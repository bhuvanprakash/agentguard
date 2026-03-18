package main

// AgentGuard — Protocol-neutral AI agent enforcement proxy.
// Built for Nascentist by Bhuvan Prakash.
//
// Usage:
//   AGENTGUARD_UPSTREAM_URL=http://localhost:8080 \
//   SUPABASE_URL=https://xxx.supabase.co \
//   SUPABASE_SERVICE_KEY=eyJ... \
//   go run main.go
//
// Or with Docker:
//   docker build -t agentguard .
//   docker run -p 7777:7777 --env-file .env agentguard

import (
    "log"

    "github.com/nascentist/agentguard/config"
    "github.com/nascentist/agentguard/proxy"
)

func main() {
    log.SetFlags(log.LstdFlags | log.Lshortfile)
    log.Println("[agentguard] Starting AgentGuard v1.0.0")

    cfg, err := config.Load()
    if err != nil {
        log.Fatalf("[agentguard] Config error: %v", err)
    }

    if err := proxy.StartServer(cfg); err != nil {
        log.Fatalf("[agentguard] Server error: %v", err)
    }
}
