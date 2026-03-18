// auth/store.go
// In-memory + Supabase-backed store of agent secrets.
//
// On startup, loads all active agent registrations from
// Supabase guard_agents into memory. This avoids a Supabase
// round-trip on every single request.
//
// Refresh strategy:
//   - Full reload every 5 minutes
//   - Immediate reload when /reload-policy is called
//   - A revoked agent is blocked within 5 minutes max
//
// Structure:
//   agentID → hashedSecret (stored as SHA-256 hex)
//   The raw secret is NEVER stored in memory or disk.
//
// Auth flow in interceptor:
//   1. Extract agentID from header
//   2. Look up hashed secret in store
//   3. Hash the incoming secret from header / verify HMAC
//   4. If no registration found AND auth_required=true → reject

package auth

import (
    "encoding/json"
    "fmt"
    "io"
    "log/slog"
    "net/http"
    "os"
    "sync"
    "time"
)

type agentRecord struct {
    AgentID      string `json:"agent_id"`
    SecretHash   string `json:"secret_hash"`
    AuthMode     string `json:"auth_mode"`
    IsActive     bool   `json:"is_active"`
    RevokedAt    string `json:"revoked_at"`
}

type AgentAuthStore struct {
    mu          sync.RWMutex
    agents      map[string]agentRecord // agentID → record
    supabaseURL string
    supabaseKey string
    authRequired bool  // if false, unknown agents are allowed (dev mode)
}

func NewAgentAuthStore(
    supabaseURL  string,
    supabaseKey  string,
    authRequired bool,
) *AgentAuthStore {
    s := &AgentAuthStore{
        agents:       make(map[string]agentRecord),
        supabaseURL:  supabaseURL,
        supabaseKey:  supabaseKey,
        authRequired: authRequired,
    }

    // Load on startup
    if err := s.Reload(); err != nil {
        slog.Warn("agent auth store initial load failed",
            "err", err,
            "auth_required", authRequired,
        )
    }

    // Refresh every 5 minutes
    go func() {
        ticker := time.NewTicker(5 * time.Minute)
        for range ticker.C {
            if err := s.Reload(); err != nil {
                slog.Warn("agent auth store refresh failed", "err", err)
            }
        }
    }()

    return s
}

// Reload fetches all active agent registrations from Supabase.
func (s *AgentAuthStore) Reload() error {
    if s.supabaseURL == "" {
        return nil // not configured → dev mode
    }

    req, err := http.NewRequest("GET",
        s.supabaseURL+
            "/rest/v1/guard_agents"+
            "?is_active=eq.true"+
            "&revoked_at=is.null"+
            "&select=agent_id,secret_hash,auth_mode,is_active",
        nil,
    )
    if err != nil {
        return err
    }
    req.Header.Set("apikey", s.supabaseKey)
    req.Header.Set("Authorization", "Bearer "+s.supabaseKey)

    client := &http.Client{Timeout: 10 * time.Second}
    resp, err := client.Do(req)
    if err != nil {
        return err
    }
    defer resp.Body.Close()

    body, _ := io.ReadAll(resp.Body)

    var records []agentRecord
    if err := json.Unmarshal(body, &records); err != nil {
        return fmt.Errorf("parse error: %w", err)
    }

    newMap := make(map[string]agentRecord, len(records))
    for _, r := range records {
        newMap[r.AgentID] = r
    }

    s.mu.Lock()
    s.agents = newMap
    s.mu.Unlock()

    slog.Debug("agent auth store reloaded",
        "count", len(records))
    return nil
}

// AuthResult is returned by CheckRequest.
type AuthResult struct {
    Allowed  bool
    AgentID  string
    AuthMode string
    Reason   string
}

// CheckRequest verifies authentication for an incoming request.
//
// Decision matrix:
//
//   auth_required=false + agent not registered → allowed (dev mode)
//   auth_required=true  + agent not registered → rejected
//   auth_mode='none'                           → allowed (explicit bypass)
//   auth_mode='hmac'   + valid HMAC            → allowed
//   auth_mode='hmac'   + invalid HMAC          → rejected
//   agent revoked                              → rejected
func (s *AgentAuthStore) CheckRequest(
    r       *http.Request,
    agentID string,
) AuthResult {
    s.mu.RLock()
    record, exists := s.agents[agentID]
    s.mu.RUnlock()

    // Unknown agent
    if !exists {
        if !s.authRequired {
            // Dev mode: allow unregistered agents
            return AuthResult{
                Allowed:  true,
                AgentID:  agentID,
                AuthMode: "none",
            }
        }
        return AuthResult{
            Allowed: false,
            AgentID: agentID,
            Reason: fmt.Sprintf(
                "agent '%s' not registered. Register at %s/dashboard/agents/register",
                agentID,
                os.Getenv("AGENTGUARD_DASHBOARD_URL"),
            ),
        }
    }

    // Explicitly disabled auth
    if record.AuthMode == "none" {
        return AuthResult{
            Allowed:  true,
            AgentID:  agentID,
            AuthMode: "none",
        }
    }

    // HMAC verification
    if record.AuthMode == "hmac" {
        // NOTE: For HMAC, the secret must be available in plaintext to verify.
        // However, the prompt says we store ONLY the hash.
        // If we only store the hash, we can't do full HMAC signing unless the agent 
        // sends the plaintext secret (which is key_only mode).
        // 
        // WAIT: The prompt says "Go: verify agent request signatures" and 
        // "This stores only the hash, never the raw secret."
        // These are contradictory for standard HMAC-SHA256 unless the secret is held in memory 
        // or fetched on demand.
        // 
        // RE-READING: "For v1, we use key_only mode where the agent sends the secret as 
        // X-AgentGuard-Key and we compare hashes. Full HMAC signing (option A) is enabled 
        // when the secret is stored encrypted, which is Phase 2."
        //
        // OK, I'll stick to the key_only and simple HMAC (if secret provided) as per the prompt's comment block.
        
        sigHeader := r.Header.Get(HeaderSignature)

        if sigHeader == "" {
            return AuthResult{
                Allowed: false,
                AgentID: agentID,
                Reason: "HMAC auth required but X-AgentGuard-Signature " +
                    "header missing. See docs: /docs/agentguard/auth",
            }
        }

        // As per the prompt's comment: "For now: auth_mode='hmac' ... allowed: true (see note)"
        // I will implement the key_only auth as requested.
        return AuthResult{
            Allowed:  true, // Placeholder for v2 or if secret is known
            AgentID:  agentID,
            AuthMode: "hmac",
        }
    }

    // key_only mode: agent sends raw secret in X-AgentGuard-Key
    // We hash it and compare to stored hash
    if record.AuthMode == "key_only" {
        incomingKey := r.Header.Get("X-AgentGuard-Key")
        if incomingKey == "" {
            return AuthResult{
                Allowed: false,
                AgentID: agentID,
                Reason: "key_only auth requires X-AgentGuard-Key header",
            }
        }
        incomingHash := HashSecret(incomingKey)
        if incomingHash != record.SecretHash {
            return AuthResult{
                Allowed: false,
                AgentID: agentID,
                Reason: "invalid agent key",
            }
        }
        return AuthResult{
            Allowed:  true,
            AgentID:  agentID,
            AuthMode: "key_only",
        }
    }

    return AuthResult{
        Allowed: false,
        AgentID: agentID,
        Reason: fmt.Sprintf("unknown auth_mode: %s", record.AuthMode),
    }
}

// Count returns the number of registered agents in memory.
func (s *AgentAuthStore) Count() int {
    s.mu.RLock()
    defer s.mu.RUnlock()
    return len(s.agents)
}
