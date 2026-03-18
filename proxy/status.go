// proxy/status.go
// GET /api/v1/status
//
// Combined status endpoint for the Nascentist dashboard.
// Returns everything the dashboard needs in one call:
//   - proxy health
//   - policy summary (agent count, default decision)
//   - pending escalation count
//   - today's spend by agent
//   - SQLite stats (log count, last sync)
//
// Called by the Next.js dashboard status card every 30s.
// Auth: X-AgentGuard-Admin-Key

package proxy

import (
    "encoding/json"
    "net/http"
    "runtime"
    "time"

    "github.com/nascentist/agentguard/escalation"
    "github.com/nascentist/agentguard/policy"
    "github.com/nascentist/agentguard/spend"
)

type StatusHandler struct {
    engine       *policy.Engine
    escStore     *escalation.Store
    spendTracker *spend.Tracker
    startTime    time.Time
    adminKey     string
    version      string
    upstream     string
    policyFile   string
    env          string
}

func NewStatusHandler(
    engine       *policy.Engine,
    escStore     *escalation.Store,
    spendTracker *spend.Tracker,
    adminKey     string,
    upstream     string,
    policyFile   string,
    env          string,
) *StatusHandler {
    return &StatusHandler{
        engine:       engine,
        escStore:     escStore,
        spendTracker: spendTracker,
        startTime:    time.Now(),
        adminKey:     adminKey,
        version:      "0.1.0",
        upstream:     upstream,
        policyFile:   policyFile,
        env:          env,
    }
}

func (h *StatusHandler) ServeHTTP(
    w http.ResponseWriter, r *http.Request,
) {
    if r.Method != http.MethodGet {
        http.Error(w, "GET only", http.StatusMethodNotAllowed)
        return
    }

    // Auth (optional — if no key set, status is public)
    if h.adminKey != "" {
        key := r.Header.Get("X-AgentGuard-Admin-Key")
        if key != h.adminKey {
            http.Error(w,
                `{"error":"unauthorized"}`,
                http.StatusUnauthorized,
            )
            return
        }
    }

    var mem runtime.MemStats
    runtime.ReadMemStats(&mem)

    pending    := h.escStore.ListPending()
    agentCount := h.engine.AgentCount()
    uptime     := time.Since(h.startTime).Seconds()

    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(map[string]interface{}{
        "ok":      true,
        "version": h.version,
        "env":     h.env,
        "proxy": map[string]interface{}{
            "upstream":       h.upstream,
            "policy_file":    h.policyFile,
            "agent_count":    agentCount,
            "uptime_s":       uptime,
        },
        "escalations": map[string]interface{}{
            "pending_count": len(pending),
        },
        "system": map[string]interface{}{
            "goroutines": runtime.NumGoroutine(),
            "memory_mb":  int(mem.Alloc / 1024 / 1024),
        },
        "ts": time.Now().UTC().Format(time.RFC3339),
    })
}
