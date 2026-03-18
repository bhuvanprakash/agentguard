// proxy/reload.go
// POST /reload-policy
//
// Hot-reloads the policy file without restarting the server.
// Called by the Nascentist dashboard when user saves a policy.

package proxy

import (
    "encoding/json"
    "fmt"
    "io"
    "net/http"
    "os"
    "time"

    "github.com/nascentist/agentguard/auth"
    "github.com/nascentist/agentguard/policy"
)

type ReloadHandler struct {
    engine      *policy.Engine
    policyFile  string
    adminKey    string
    supabaseURL string
    supabaseKey string
    authStore   *auth.AgentAuthStore
}

func NewReloadHandler(
    engine      *policy.Engine,
    policyFile  string,
    adminKey    string,
    supabaseURL string,
    supabaseKey string,
    authStore   *auth.AgentAuthStore,
) *ReloadHandler {
    return &ReloadHandler{
        engine:      engine,
        policyFile:  policyFile,
        adminKey:    adminKey,
        supabaseURL: supabaseURL,
        supabaseKey: supabaseKey,
        authStore:   authStore,
    }
}

func (h *ReloadHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodPost {
        http.Error(w, "POST only", http.StatusMethodNotAllowed)
        return
    }

    // Auth check
    if h.adminKey != "" {
        if r.Header.Get("X-AgentGuard-Admin-Key") != h.adminKey {
            w.Header().Set("Content-Type", "application/json")
            w.WriteHeader(http.StatusUnauthorized)
            w.Write([]byte(`{"error":"unauthorized"}`))
            return
        }
    }

    // Check if a specific policy_id was requested
    var reqBody struct {
        PolicyID string `json:"policy_id"`
    }
    if r.ContentLength > 0 {
        body, _ := io.ReadAll(io.LimitReader(r.Body, 65536))
        json.Unmarshal(body, &reqBody)
    }

    if reqBody.PolicyID != "" {
        if err := h.downloadPolicyFromSupabase(reqBody.PolicyID); err != nil {
            w.Header().Set("Content-Type", "application/json")
            w.WriteHeader(http.StatusInternalServerError)
            json.NewEncoder(w).Encode(map[string]string{
                "error": fmt.Sprintf("download failed: %v", err),
            })
            return
        }
    }

    // Reload policy engine from file
    if err := h.engine.Reload(); err != nil {
        w.Header().Set("Content-Type", "application/json")
        w.WriteHeader(http.StatusInternalServerError)
        json.NewEncoder(w).Encode(map[string]string{
            "error": fmt.Sprintf("reload failed: %v", err),
        })
        return
    }

    // Reload auth store as well
    if h.authStore != nil {
        if err := h.authStore.Reload(); err != nil {
            // Non-fatal, but logged
        }
    }

    agentCount := h.engine.AgentCount()

    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(map[string]interface{}{
        "ok":          true,
        "agent_count": agentCount,
        "policy_file": h.policyFile,
        "reloaded_at": time.Now().UTC().Format(time.RFC3339),
    })
}

func (h *ReloadHandler) downloadPolicyFromSupabase(policyID string) error {
    if h.supabaseURL == "" {
        return fmt.Errorf("SUPABASE_URL not configured")
    }

    req, err := http.NewRequest("GET",
        fmt.Sprintf("%s/rest/v1/guard_policies?id=eq.%s&select=yaml_content",
            h.supabaseURL, policyID),
        nil,
    )
    if err != nil {
        return err
    }
    req.Header.Set("apikey", h.supabaseKey)
    req.Header.Set("Authorization", "Bearer "+h.supabaseKey)

    client := &http.Client{Timeout: 10 * time.Second}
    resp, err := client.Do(req)
    if err != nil {
        return err
    }
    defer resp.Body.Close()

    body, err := io.ReadAll(resp.Body)
    if err != nil {
        return err
    }

    var rows []struct {
        YAMLContent string `json:"yaml_content"`
    }
    if err := json.Unmarshal(body, &rows); err != nil {
        return fmt.Errorf("parse error: %w", err)
    }
    if len(rows) == 0 {
        return fmt.Errorf("policy %s not found in Supabase", policyID)
    }

    if err := os.WriteFile(h.policyFile, []byte(rows[0].YAMLContent), 0644); err != nil {
        return fmt.Errorf("write file: %w", err)
    }

    return nil
}
