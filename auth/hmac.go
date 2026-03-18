// auth/hmac.go
// HMAC-SHA256 request signing and verification for AgentGuard.
//
// ── How it works ─────────────────────────────────────────────
//
// When a developer registers an agent in the Nascentist
// dashboard, they receive a signing secret:
//   agk_xK3mP9...  (64 hex chars)
//
// The agent signs EVERY request using HMAC-SHA256 over
// a canonical string that includes:
//   - The agent ID
//   - The request timestamp (Unix seconds, string)
//   - The raw request body (prevents tampering)
//
// Canonical string format:
//   "{agent_id}\n{timestamp}\n{body_sha256_hex}"
//
// The agent sends two extra headers:
//   X-AgentGuard-Timestamp: 1742000000
//   X-AgentGuard-Signature: sha256=<hex>
//
// The proxy verifies:
//   1. Timestamp is within ±300 seconds (replay protection)
//   2. HMAC matches (tamper protection)
//   3. Agent ID is registered (identity verification)
//
// ── SDK usage (Python) ───────────────────────────────────────
//   from agentguard import AgentGuard
//   guard = AgentGuard(
//     agent_id="billing-agent",
//     signing_secret="agk_xK3mP9...",
//   )
//   # All requests are auto-signed
//
// ── SDK usage (Node.js) ──────────────────────────────────────
//   import { AgentGuard } from 'agentguard'
//   const guard = new AgentGuard({
//     agentId: 'billing-agent',
//     signingSecret: 'agk_xK3mP9...',
//   })

package auth

import (
    "bytes"
    "crypto/hmac"
    "crypto/rand"
    "crypto/sha256"
    "encoding/hex"
    "fmt"
    "io"
    "net/http"
    "strconv"
    "strings"
    "time"
)

const (
    // Maximum allowed clock drift between agent and proxy.
    // 5 minutes is generous but prevents replay attacks.
    MaxTimestampSkewSeconds = 300

    // Header names
    HeaderTimestamp = "X-AgentGuard-Timestamp"
    HeaderSignature = "X-AgentGuard-Signature"

    // Secret prefix — allows users to identify their keys
    SecretPrefix = "agk_"
)

// GenerateSigningSecret creates a new random signing secret.
// Returns both the raw secret (shown once) and its SHA-256 hash
// (stored in Supabase guard_agents.secret_hash).
//
// Format: agk_ + 60 random hex chars
// Example: agk_3f8a2c1e9b4d7e0f1a2b3c4d5e6f7a8b9c0d1e2f3a4b5c6d7e8f9a0b1c2d3e4f
func GenerateSigningSecret() (raw string, hash string, prefix string, err error) {
    b := make([]byte, 30)
    if _, err = rand.Read(b); err != nil {
        return
    }
    raw    = SecretPrefix + hex.EncodeToString(b)
    hash   = HashSecret(raw)
    prefix = raw[:12] + "..."
    return
}

// HashSecret returns the SHA-256 hex digest of a secret.
// Used to compare incoming secrets without storing them plaintext.
func HashSecret(secret string) string {
    h := sha256.Sum256([]byte(secret))
    return hex.EncodeToString(h[:])
}

// BuildCanonicalString builds the string that is signed.
// Format: "{agent_id}\n{timestamp}\n{body_hash}"
// Where body_hash is SHA-256 hex of the raw request body.
func BuildCanonicalString(
    agentID   string,
    timestamp string,
    body      []byte,
) string {
    bodyHash := sha256.Sum256(body)
    return fmt.Sprintf(
        "%s\n%s\n%s",
        agentID,
        timestamp,
        hex.EncodeToString(bodyHash[:]),
    )
}

// Sign creates an HMAC-SHA256 signature over the canonical string.
// Returns the signature in the format: "sha256=<hex>"
func Sign(
    secret    string,
    agentID   string,
    timestamp string,
    body      []byte,
) string {
    canonical := BuildCanonicalString(agentID, timestamp, body)
    mac := hmac.New(sha256.New, []byte(secret))
    mac.Write([]byte(canonical))
    return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

// VerificationResult is returned by Verify().
type VerificationResult struct {
    Valid   bool
    AgentID string
    Reason  string  // populated when Valid=false
}

// Verify checks the HMAC signature on an incoming request.
// It reads the request body (and restores it for downstream use).
//
// Returns a VerificationResult indicating whether the
// signature is valid, and why it failed if not.
func Verify(
    r       *http.Request,
    secret  string,  // raw signing secret from agent registration
    agentID string,  // expected agent ID
) VerificationResult {
    // Read timestamp header
    tsStr := r.Header.Get(HeaderTimestamp)
    if tsStr == "" {
        return VerificationResult{
            Valid:  false,
            Reason: "missing X-AgentGuard-Timestamp header",
        }
    }

    // Validate timestamp — prevent replay attacks
    ts, err := strconv.ParseInt(tsStr, 10, 64)
    if err != nil {
        return VerificationResult{
            Valid:  false,
            Reason: "invalid timestamp format",
        }
    }

    now  := time.Now().Unix()
    skew := now - ts
    if skew < 0 {
        skew = -skew
    }
    if skew > MaxTimestampSkewSeconds {
        return VerificationResult{
            Valid: false,
            Reason: fmt.Sprintf(
                "timestamp too old or too far in future (skew %ds > %ds)",
                skew, MaxTimestampSkewSeconds,
            ),
        }
    }

    // Read signature header
    sigHeader := r.Header.Get(HeaderSignature)
    if sigHeader == "" {
        return VerificationResult{
            Valid:  false,
            Reason: "missing X-AgentGuard-Signature header",
        }
    }
    if !strings.HasPrefix(sigHeader, "sha256=") {
        return VerificationResult{
            Valid:  false,
            Reason: "signature must start with sha256=",
        }
    }

    // Read body (and restore it for downstream handlers)
    body, err := io.ReadAll(r.Body)
    if err != nil {
        return VerificationResult{
            Valid:  false,
            Reason: "failed to read request body",
        }
    }
    r.Body = io.NopCloser(bytes.NewReader(body))

    // Compute expected signature
    expected := Sign(secret, agentID, tsStr, body)

    // Timing-safe comparison
    expectedBytes, _ := hex.DecodeString(
        strings.TrimPrefix(expected, "sha256="),
    )
    actualBytes, err := hex.DecodeString(
        strings.TrimPrefix(sigHeader, "sha256="),
    )
    if err != nil || !hmac.Equal(actualBytes, expectedBytes) {
        return VerificationResult{
            Valid:  false,
            Reason: "signature mismatch",
            AgentID: agentID,
        }
    }

    return VerificationResult{
        Valid:   true,
        AgentID: agentID,
    }
}
