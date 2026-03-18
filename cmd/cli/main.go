// cmd/cli/main.go
// AgentGuard CLI — standalone binary for terminal users.
//
// Build:
//   go build -o bin/agentguard ./cmd/cli
//
// Usage:
//   agentguard serve               ← start the proxy server
//   agentguard validate [file]     ← validate a policy YAML file
//   agentguard simulate            ← simulate a policy decision
//   agentguard escalations list    ← list pending escalations
//   agentguard escalations approve ← approve an escalation
//   agentguard escalations reject  ← reject an escalation
//   agentguard version             ← print version
//   agentguard health              ← check a running proxy
//
// The CLI does NOT require the Go service to be running
// for validate/simulate. Those run fully offline.
// escalations/health/serve DO require a running proxy or config.

package main

import (
    "fmt"
    "os"
)

const version = "0.1.0"

var usage = `AgentGuard — AI agent enforcement proxy

USAGE
  agentguard <command> [flags]

COMMANDS
  serve                   Start the AgentGuard proxy server
  validate [policy.yaml]  Validate a policy YAML file (offline)
  simulate                Simulate a policy decision (offline)
  escalations list        List pending escalations
  escalations approve ID  Approve a pending escalation
  escalations reject  ID  Reject a pending escalation
  health                  Check a running proxy's health
  version                 Print version

EXAMPLES
  agentguard serve
  agentguard validate ./policy.yaml
  agentguard simulate --agent=billing-agent --tool=send_payment
  agentguard escalations list
  agentguard escalations approve abc-123
  agentguard health --url=http://localhost:7777

ENVIRONMENT
  AGENTGUARD_URL       URL of running proxy (default: http://localhost:7777)
  AGENTGUARD_ADMIN_KEY Admin key for escalation endpoints

Learn more: https://nascentist.ai/docs/agentguard
`

func main() {
    if len(os.Args) < 2 {
        fmt.Print(usage)
        os.Exit(0)
    }

    cmd := os.Args[1]

    switch cmd {
    case "serve":
        runServe(os.Args[2:])

    case "validate":
        runValidate(os.Args[2:])

    case "simulate":
        runSimulate(os.Args[2:])

    case "escalations":
        if len(os.Args) < 3 {
            fmt.Println("Usage: agentguard escalations <list|approve|reject>")
            os.Exit(1)
        }
        runEscalations(os.Args[2], os.Args[3:])

    case "health":
        runHealth(os.Args[2:])

    case "version", "--version", "-v":
        fmt.Printf("agentguard version %s\n", version)

    case "help", "--help", "-h":
        fmt.Print(usage)

    default:
        fmt.Fprintf(os.Stderr, "Unknown command: %s\n\n%s", cmd, usage)
        os.Exit(1)
    }
}

// getEnvOrDefault reads an env var with fallback.
func getEnvOrDefault(key, fallback string) string {
    if v := os.Getenv(key); v != "" {
        return v
    }
    return fallback
}
