// cmd/cli/health.go
// `agentguard health [--url=URL]`
//
// Checks a running AgentGuard proxy's health.

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

func runHealth(args []string) {
    url := getEnvOrDefault("AGENTGUARD_URL", "http://localhost:7777")

    for _, arg := range args {
        if strings.HasPrefix(arg, "--url=") {
            url = strings.TrimPrefix(arg, "--url=")
        }
    }

    client := &http.Client{Timeout: 5 * time.Second}
    start  := time.Now()
    resp, err := client.Get(url + "/health")
    elapsed := time.Since(start)

    if err != nil {
        fmt.Fprintf(os.Stderr,
            "✗ Cannot reach AgentGuard at %s\n"+
                "  Is it running? Try: agentguard serve\n\n"+
                "  Error: %v\n",
            url, err)
        os.Exit(1)
    }
    defer resp.Body.Close()

    body, _ := io.ReadAll(resp.Body)

    var h struct {
        Status      string  `json:"status"`
        Version     string  `json:"version"`
        UptimeS     float64 `json:"uptime_s"`
        Environment string  `json:"environment"`
        Goroutines  int     `json:"goroutines"`
        MemoryMB    int     `json:"memory_mb"`
        PolicyFile  string  `json:"policy_file"`
        Upstream    string  `json:"upstream"`
    }
    json.Unmarshal(body, &h)

    sym := "✓"
    if h.Status != "ok" {
        sym = "✗"
    }

    // Format uptime
    uptime := ""
    if h.UptimeS < 60 {
        uptime = fmt.Sprintf("%.0fs", h.UptimeS)
    } else if h.UptimeS < 3600 {
        uptime = fmt.Sprintf("%.0fm", h.UptimeS/60)
    } else {
        uptime = fmt.Sprintf("%.1fh", h.UptimeS/3600)
    }

    fmt.Printf("\n%s AgentGuard is %s\n\n", sym, strings.ToUpper(h.Status))
    fmt.Printf("  URL:         %s\n", url)
    fmt.Printf("  Version:     %s\n", h.Version)
    fmt.Printf("  Upstream:    %s\n", h.Upstream)
    fmt.Printf("  Policy:      %s\n", h.PolicyFile)
    fmt.Printf("  Environment: %s\n", h.Environment)
    fmt.Printf("  Uptime:      %s\n", uptime)
    fmt.Printf("  Goroutines:  %d\n", h.Goroutines)
    fmt.Printf("  Memory:      %dMB\n", h.MemoryMB)
    fmt.Printf("  Latency:     %s\n", elapsed.Round(time.Millisecond))
    fmt.Println()

    if h.Status != "ok" {
        os.Exit(1)
    }
}
