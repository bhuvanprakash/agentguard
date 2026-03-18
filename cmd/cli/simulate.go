// cmd/cli/simulate.go
// `agentguard simulate --agent=ID --tool=TOOL [--policy=FILE]`
//
// Simulates what decision AgentGuard would make for a
// given agent + tool combination, fully offline.
//
// Usage:
//   agentguard simulate --agent=billing-agent --tool=send_payment
//   agentguard simulate \
//     --agent=billing-agent \
//     --tool=send_payment \
//     --policy=./my-policy.yaml

package main

import (
    "fmt"
    "os"
    "strings"

    "github.com/nascentist/agentguard/policy"
    "gopkg.in/yaml.v3"
)

func runSimulate(args []string) {
    var agentID, toolName, policyFile string
    policyFile = "policy.yaml"

    for _, arg := range args {
        switch {
        case strings.HasPrefix(arg, "--agent="):
            agentID = strings.TrimPrefix(arg, "--agent=")
        case strings.HasPrefix(arg, "--tool="):
            toolName = strings.TrimPrefix(arg, "--tool=")
        case strings.HasPrefix(arg, "--policy="):
            policyFile = strings.TrimPrefix(arg, "--policy=")
        }
    }

    if agentID == "" || toolName == "" {
        fmt.Fprintln(os.Stderr, "Usage: agentguard simulate --agent=ID --tool=TOOL [--policy=FILE]")
        os.Exit(1)
    }

    // Load policy
    data, err := os.ReadFile(policyFile)
    if err != nil {
        fmt.Fprintf(os.Stderr, "✗ Cannot read policy file '%s': %v\n", policyFile, err)
        os.Exit(1)
    }

    var policyData struct {
        Version string `yaml:"version"`
        Default string `yaml:"default"`
        Agents  []struct {
            ID       string `yaml:"id"`
            Allow    []struct { Tool string `yaml:"tool"` } `yaml:"allow"`
            Block    []struct { Tool string `yaml:"tool"` } `yaml:"block"`
            Escalate []struct { Tool string `yaml:"tool"` } `yaml:"escalate"`
            SpendLimitDailyUsd float64 `yaml:"spend_limit_daily_usd"`
        } `yaml:"agents"`
    }
    if err := yaml.Unmarshal(data, &policyData); err != nil {
        fmt.Fprintf(os.Stderr, "✗ YAML parse error: %v\n", err)
        os.Exit(1)
    }

    engine, err := policy.NewEngine(policyFile)
    if err != nil {
        fmt.Fprintf(os.Stderr, "✗ Policy load error: %v\n", err)
        os.Exit(1)
    }

    // Use policy engine for simulation
    decision, _ := engine.Evaluate(agentID, toolName, nil)

    // Symbols per decision
    symbols := map[string]string{
        "allow":    "✓",
        "block":    "✗",
        "escalate": "⚠",
    }
    colors := map[string]string{
        "allow":    "\033[32m", // green
        "block":    "\033[31m", // red
        "escalate": "\033[33m", // yellow
    }
    reset := "\033[0m"

    sym   := symbols[string(decision)]
    color := colors[string(decision)]

    fmt.Printf("\nSimulating decision:\n")
    fmt.Printf("  Policy:  %s\n", policyFile)
    fmt.Printf("  Agent:   %s\n", agentID)
    fmt.Printf("  Tool:    %s\n", toolName)
    fmt.Println()
    fmt.Printf("  Decision: %s%s %s%s\n",
        color, sym, strings.ToUpper(string(decision)), reset)
    fmt.Println()

    // Advice
    switch string(decision) {
    case "allow":
        fmt.Println("  The tool call would be forwarded to the upstream API.")
    case "block":
        fmt.Println("  The tool call would be rejected immediately.")
        fmt.Println("  The agent receives an error response.")
    case "escalate":
        fmt.Println("  The tool call would be paused for human approval.")
        fmt.Println("  A notification is sent to the dashboard.")
        fmt.Println("  The agent receives a 202 pending response.")
    }
    fmt.Println()
}
