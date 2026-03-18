// cmd/cli/escalate.go
// `agentguard escalations list|approve|reject`
//
// Manages pending escalations from the terminal.
// Requires a running AgentGuard proxy.

package main

import (
    "encoding/json"
    "fmt"
    "io"
    "net/http"
    "os"
    "strings"
    "time"
)

func runEscalations(subCmd string, args []string) {
    url      := getEnvOrDefault("AGENTGUARD_URL", "http://localhost:7777")
    adminKey := os.Getenv("AGENTGUARD_ADMIN_KEY")

    for _, arg := range args {
        if strings.HasPrefix(arg, "--url=") {
            url = strings.TrimPrefix(arg, "--url=")
        }
    }

    switch subCmd {
    case "list":
        escalationsList(url, adminKey)
    case "approve":
        if len(args) == 0 {
            fmt.Fprintln(os.Stderr, "Usage: agentguard escalations approve <ID>")
            os.Exit(1)
        }
        id := args[len(args)-1]
        if strings.HasPrefix(id, "--") {
            fmt.Fprintln(os.Stderr, "Error: provide escalation ID")
            os.Exit(1)
        }
        escalationResolve(url, adminKey, id, "approve")
    case "reject":
        if len(args) == 0 {
            fmt.Fprintln(os.Stderr, "Usage: agentguard escalations reject <ID>")
            os.Exit(1)
        }
        id := args[len(args)-1]
        escalationResolve(url, adminKey, id, "reject")
    default:
        fmt.Fprintf(os.Stderr,
            "Unknown escalations subcommand: %s\n"+
                "Use: list | approve | reject\n", subCmd)
        os.Exit(1)
    }
}

func escalationsList(baseURL, adminKey string) {
    req, _ := http.NewRequest("GET",
        baseURL+"/escalations", nil)
    req.Header.Set("X-AgentGuard-Admin-Key", adminKey)

    client := &http.Client{Timeout: 5 * time.Second}
    resp, err := client.Do(req)
    if err != nil {
        fmt.Fprintf(os.Stderr,
            "✗ Cannot connect to AgentGuard at %s\n"+
                "  Is it running? Try: agentguard serve\n\n"+
                "  Error: %v\n",
            baseURL, err)
        os.Exit(1)
    }
    defer resp.Body.Close()

    body, _ := io.ReadAll(resp.Body)
    if resp.StatusCode == 401 {
        fmt.Fprintln(os.Stderr, "✗ Unauthorized. Set AGENTGUARD_ADMIN_KEY env var.")
        os.Exit(1)
    }

    var result struct {
        Escalations []struct {
            ID         string  `json:"id"`
            AgentID    string  `json:"agent_id"`
            ToolName   string  `json:"tool_name"`
            Protocol   string  `json:"protocol"`
            Ts         string  `json:"ts"`
            ExpiresAt  string  `json:"expires_at"`
            AgeMinutes float64 `json:"age_minutes"`
        } `json:"escalations"`
        Count int `json:"count"`
    }
    json.Unmarshal(body, &result)

    if result.Count == 0 {
        fmt.Println("No pending escalations. ✓")
        return
    }

    fmt.Printf("%d pending escalation(s):\n\n", result.Count)
    fmt.Printf("  %-38s  %-20s  %-20s  %-8s  %s\n",
        "ID", "AGENT", "TOOL", "PROTOCOL", "AGE")
    fmt.Println("  " + strings.Repeat("─", 100))

    for _, e := range result.Escalations {
        shortID := e.ID
        if len(shortID) > 36 {
            shortID = shortID[:36]
        }

        age := ""
        if e.AgeMinutes < 1 {
            age = "just now"
        } else if e.AgeMinutes < 60 {
            age = fmt.Sprintf("%.0fm ago", e.AgeMinutes)
        } else {
            age = fmt.Sprintf("%.0fh ago", e.AgeMinutes/60)
        }

        fmt.Printf("  %-38s  %-20s  %-20s  %-8s  %s\n",
            shortID,
            truncate(e.AgentID, 20),
            truncate(e.ToolName, 20),
            e.Protocol,
            age,
        )
    }

    fmt.Println()
    fmt.Println("Approve: agentguard escalations approve <ID>")
    fmt.Println("Reject:  agentguard escalations reject  <ID>")
}

func escalationResolve(baseURL, adminKey, id, action string) {
    req, _ := http.NewRequest("POST",
        fmt.Sprintf("%s/escalations/%s/%s", baseURL, id, action),
        nil,
    )
    req.Header.Set("X-AgentGuard-Admin-Key", adminKey)

    client := &http.Client{Timeout: 5 * time.Second}
    resp, err := client.Do(req)
    if err != nil {
        fmt.Fprintf(os.Stderr, "✗ Cannot connect to AgentGuard: %v\n", err)
        os.Exit(1)
    }
    defer resp.Body.Close()

    body, _ := io.ReadAll(resp.Body)

    if resp.StatusCode == 401 {
        fmt.Fprintln(os.Stderr, "✗ Unauthorized. Set AGENTGUARD_ADMIN_KEY env var.")
        os.Exit(1)
    }

    if resp.StatusCode != 200 {
        fmt.Fprintf(os.Stderr, "✗ Failed (%d): %s\n", resp.StatusCode, string(body))
        os.Exit(1)
    }

    sym := "✓"
    if action == "reject" {
        sym = "✗"
    }
    fmt.Printf("%s Escalation %s\n  ID: %s\n", sym, action+"d", id)
}

func truncate(s string, n int) string {
    if len(s) <= n {
        return s
    }
    return s[:n-1] + "…"
}
