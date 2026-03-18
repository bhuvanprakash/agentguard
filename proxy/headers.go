// proxy/headers.go
// Standardized header extraction for AgentGuard.
//
// Agent identity can come from multiple headers.
// This file defines the canonical extraction order so
// the dashboard and Go service agree on agent_id values.
//
// Priority order (first non-empty wins):
//   1. X-Agent-ID           ← preferred (explicit)
//   2. X-AgentGuard-Agent   ← alternate spelling
//   3. Authorization bearer  ← extract from nsc_live_ key
//   4. "anonymous"          ← fallback

package proxy

import (
    "net/http"
    "strings"
)

// ExtractAgentID returns the canonical agent identifier
// from the request. This must match what the dashboard
// filters expect.
func ExtractAgentID(r *http.Request) string {
    // 1. Explicit header
    if id := r.Header.Get("X-Agent-ID"); id != "" {
        return sanitizeAgentID(id)
    }
    // 2. Alternate header
    if id := r.Header.Get("X-AgentGuard-Agent"); id != "" {
        return sanitizeAgentID(id)
    }
    // 3. Extract from nsc_live_ API key
    if auth := r.Header.Get("Authorization"); auth != "" {
        if strings.HasPrefix(auth, "Bearer nsc_live_") {
            key := strings.TrimPrefix(auth, "Bearer ")
            // Use first 20 chars of key as agent ID
            // (stable, deterministic, not the full secret)
            if len(key) > 20 {
                return "key_" + key[10:20]
            }
            return "key_" + key
        }
        if strings.HasPrefix(auth, "Bearer ") {
            token := strings.TrimPrefix(auth, "Bearer ")
            if len(token) > 12 {
                return "token_" + token[:8]
            }
        }
    }
    // 4. Fallback
    return "anonymous"
}

// ExtractRequestID returns a stable request identifier
// for log correlation. Checks standard headers.
func ExtractRequestID(r *http.Request) string {
    if id := r.Header.Get("X-Request-ID"); id != "" {
        return id
    }
    if id := r.Header.Get("X-Correlation-ID"); id != "" {
        return id
    }
    return ""
}

func sanitizeAgentID(id string) string {
    // Remove whitespace, limit to 64 chars
    id = strings.TrimSpace(id)
    if len(id) > 64 {
        return id[:64]
    }
    return id
}

// StandardResponseHeaders sets headers AgentGuard adds to
// every response so clients can identify it.
func StandardResponseHeaders(
    w       http.ResponseWriter,
    decision string,
) {
    w.Header().Set("X-AgentGuard",          "1.0")
    w.Header().Set("X-AgentGuard-Decision", decision)
    w.Header().Set("Server",                "AgentGuard/1.0")
}
