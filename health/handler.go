package health

import (
    "encoding/json"
    "net/http"
    "runtime"
    "time"

    "github.com/nascentist/agentguard/config"
    "github.com/nascentist/agentguard/policy"
)

var startTime = time.Now()

func Handler(cfg *config.Config, engine *policy.Engine) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        var m runtime.MemStats
        runtime.ReadMemStats(&m)

        resp := map[string]interface{}{
            "status":      "ok",
            "version":     "1.0.0",
            "uptime_s":    time.Since(startTime).Seconds(),
            "environment": cfg.Env,
            "upstream":    cfg.UpstreamURL,
            "policy_file": cfg.PolicyFile,
            "memory_mb":   m.Alloc / 1024 / 1024,
            "goroutines":  runtime.NumGoroutine(),
            "ts":          time.Now().UTC().Format(time.RFC3339),
        }

        w.Header().Set("Content-Type", "application/json")
        w.WriteHeader(http.StatusOK)
        json.NewEncoder(w).Encode(resp)
    }
}
