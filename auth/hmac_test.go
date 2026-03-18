// auth/hmac_test.go

package auth_test

import (
    "bytes"
    "net/http"
    "net/http/httptest"
    "strconv"
    "testing"
    "time"

    "github.com/nascentist/agentguard/auth"
)

func TestGenerateSigningSecret(t *testing.T) {
    raw, hash, prefix, err := auth.GenerateSigningSecret()
    if err != nil {
        t.Fatalf("GenerateSigningSecret failed: %v", err)
    }
    if len(raw) < 60 {
        t.Errorf("secret too short: %d", len(raw))
    }
    if hash == raw {
        t.Error("hash must differ from raw")
    }
    if hash == "" {
        t.Error("hash must not be empty")
    }
    if len(prefix) < 10 {
        t.Errorf("prefix too short: %s", prefix)
    }
    t.Logf("raw=%s hash=%s prefix=%s", raw[:12]+"...", hash[:12]+"...", prefix)
}

func TestSignAndVerify(t *testing.T) {
    secret  := "agk_test_secret_for_unit_tests_only_not_real"
    agentID := "test-agent"
    ts      := strconv.FormatInt(time.Now().Unix(), 10)
    body    := []byte(`{"jsonrpc":"2.0","method":"tools/call","id":"1"}`)

    sig := auth.Sign(secret, agentID, ts, body)

    req := httptest.NewRequest(
        "POST", "/", bytes.NewReader(body),
    )
    req.Header.Set("X-Agent-ID",              agentID)
    req.Header.Set(auth.HeaderTimestamp,       ts)
    req.Header.Set(auth.HeaderSignature,       sig)
    req.Header.Set("Content-Type",             "application/json")

    result := auth.Verify(req, secret, agentID)
    if !result.Valid {
        t.Errorf("Expected valid, got: %s", result.Reason)
    }
}

func TestVerifyReplayAttack(t *testing.T) {
    secret  := "agk_test_secret_for_unit_tests_only_not_real"
    agentID := "test-agent"
    // Timestamp 10 minutes in the past = replay
    oldTS := strconv.FormatInt(
        time.Now().Unix()-600, 10,
    )
    body := []byte(`{"jsonrpc":"2.0","method":"tools/call"}`)
    sig  := auth.Sign(secret, agentID, oldTS, body)

    req := httptest.NewRequest("POST", "/",
        bytes.NewReader(body))
    req.Header.Set(auth.HeaderTimestamp, oldTS)
    req.Header.Set(auth.HeaderSignature, sig)

    result := auth.Verify(req, secret, agentID)
    if result.Valid {
        t.Error("Expected invalid for replayed timestamp")
    }
    if result.Reason == "" {
        t.Error("Expected reason for failure")
    }
    t.Logf("Correct rejection: %s", result.Reason)
}

func TestVerifyTamperedBody(t *testing.T) {
    secret  := "agk_test_secret_for_unit_tests_only_not_real"
    agentID := "test-agent"
    ts      := strconv.FormatInt(time.Now().Unix(), 10)
    origBody    := []byte(`{"amount":100}`)
    tamperedBody := []byte(`{"amount":99999}`)

    // Sign original body
    sig := auth.Sign(secret, agentID, ts, origBody)

    // Send tampered body with original signature
    req := httptest.NewRequest("POST", "/",
        bytes.NewReader(tamperedBody))
    req.Header.Set(auth.HeaderTimestamp, ts)
    req.Header.Set(auth.HeaderSignature, sig)

    result := auth.Verify(req, secret, agentID)
    if result.Valid {
        t.Error("Expected invalid for tampered body")
    }
    t.Logf("Correct rejection: %s", result.Reason)
}

func TestVerifyMissingHeaders(t *testing.T) {
    body := []byte(`{"test":true}`)

    // Missing both headers
    req := httptest.NewRequest("POST", "/",
        bytes.NewReader(body))

    result := auth.Verify(req, "secret", "agent")
    if result.Valid {
        t.Error("Expected invalid for missing headers")
    }
    t.Logf("Correct rejection: %s", result.Reason)
}

func TestHashSecretDeterministic(t *testing.T) {
    h1 := auth.HashSecret("agk_test")
    h2 := auth.HashSecret("agk_test")
    if h1 != h2 {
        t.Error("Hash must be deterministic")
    }
    if len(h1) != 64 {
        t.Errorf("SHA-256 hex must be 64 chars, got %d", len(h1))
    }
}
