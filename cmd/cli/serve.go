// cmd/cli/serve.go
// `agentguard serve [--port=PORT] [--policy=FILE]`
//
// Starts the AgentGuard proxy server from the CLI.
// Wraps the proxy package from the main service.

package main

import (
    "fmt"
    "os"
    "strings"

    "github.com/nascentist/agentguard/config"
    "github.com/nascentist/agentguard/proxy"
)

func runServe(args []string) {
    // Override config with CLI flags
    for _, arg := range args {
        switch {
        case strings.HasPrefix(arg, "--port="):
            os.Setenv("AGENTGUARD_PORT",
                strings.TrimPrefix(arg, "--port="))
        case strings.HasPrefix(arg, "--policy="):
            os.Setenv("AGENTGUARD_POLICY_FILE",
                strings.TrimPrefix(arg, "--policy="))
        case strings.HasPrefix(arg, "--upstream="):
            os.Setenv("AGENTGUARD_UPSTREAM_URL",
                strings.TrimPrefix(arg, "--upstream="))
        case arg == "--help" || arg == "-h":
            fmt.Println(`Usage: agentguard serve [flags]

Flags:
  --port=PORT        Port to listen on (default: 7777)
  --policy=FILE      Policy YAML file (default: ./policy.yaml)
  --upstream=URL     Upstream URL to proxy to

Environment (alternative to flags):
  AGENTGUARD_PORT
  AGENTGUARD_POLICY_FILE
  AGENTGUARD_UPSTREAM_URL
  SUPABASE_URL
  SUPABASE_SERVICE_KEY`)
            os.Exit(0)
        }
    }

    cfg, err := config.Load()
    if err != nil {
        fmt.Fprintf(os.Stderr, "✗ Config error: %v\n", err)
        os.Exit(1)
    }

    fmt.Printf("AgentGuard v%s starting on port %s\n", version, cfg.Port)
    fmt.Printf("  Upstream:   %s\n", cfg.UpstreamURL)
    fmt.Printf("  Policy:     %s\n", cfg.PolicyFile)
    fmt.Printf("  SQLite:     %s\n", cfg.SQLitePath)
    fmt.Printf("  Supabase:   %s\n", cfg.SupabaseURL)
    fmt.Println()

    if err := proxy.StartServer(cfg); err != nil {
        fmt.Fprintf(os.Stderr, "✗ Server error: %v\n", err)
        os.Exit(1)
    }
}
