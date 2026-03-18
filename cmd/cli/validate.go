// cmd/cli/validate.go
// `agentguard validate [policy.yaml]`
//
// Validates a policy YAML file offline — no proxy needed.
// Exits 0 on success, 1 on validation errors.
//
// Usage:
//   agentguard validate                    ← validates ./policy.yaml
//   agentguard validate /path/to/policy.yaml
//   agentguard validate --strict           ← also warn on missing agent IDs

package main

import (
    "fmt"
    "os"
    "strings"

    "gopkg.in/yaml.v3"
)

func runValidate(args []string) {
    policyFile := "policy.yaml"
    strict     := false

    for _, arg := range args {
        if arg == "--strict" {
            strict = true
        } else if !strings.HasPrefix(arg, "-") {
            policyFile = arg
        }
    }

    // Check file exists
    if _, err := os.Stat(policyFile); os.IsNotExist(err) {
        fmt.Fprintf(os.Stderr, "✗ Policy file not found: %s\n", policyFile)
        os.Exit(1)
    }

    // Parse YAML
    data, err := os.ReadFile(policyFile)
    if err != nil {
        fmt.Fprintf(os.Stderr, "✗ Cannot read file: %v\n", err)
        os.Exit(1)
    }

    var policy map[string]interface{}
    if err := yaml.Unmarshal(data, &policy); err != nil {
        fmt.Fprintf(os.Stderr, "✗ YAML parse error: %v\n", err)
        fmt.Println()
        fmt.Println("Tip: YAML is indentation-sensitive.")
        fmt.Println("     Use 2 spaces, not tabs.")
        os.Exit(1)
    }

    errors   := []string{}
    warnings := []string{}

    // Required fields
    if _, ok := policy["version"]; !ok {
        errors = append(errors, "Missing required field: version")
    }

    // Default decision
    if def, ok := policy["default"].(string); ok {
        if def != "allow" && def != "block" && def != "escalate" {
            errors = append(errors,
                fmt.Sprintf("Invalid default: '%s' "+
                    "(must be allow|block|escalate)", def))
        }
    } else if _, ok := policy["default"]; ok {
        errors = append(errors, "Invalid type for 'default': must be a string")
    } else {
        warnings = append(warnings,
            "No 'default' set — will use 'block' as fallback")
    }

    // Agents
    agents, ok := policy["agents"].([]interface{})
    if !ok && policy["agents"] != nil {
        errors = append(errors, "'agents' must be a list")
    }

    seenIDs := map[string]bool{}
    for i, raw := range agents {
        agent, ok := raw.(map[string]interface{})
        if !ok {
            errors = append(errors,
                fmt.Sprintf("agents[%d]: must be a map", i))
            continue
        }

        id, hasID := agent["id"].(string)
        if !hasID || id == "" {
            errors = append(errors,
                fmt.Sprintf("agents[%d]: missing 'id' field", i))
        } else if seenIDs[id] {
            errors = append(errors,
                fmt.Sprintf("agents[%d]: duplicate id '%s'", i, id))
        } else {
            seenIDs[id] = true
        }

        // Validate rule lists
        for _, key := range []string{"allow", "block", "escalate"} {
            rules, hasRules := agent[key]
            if !hasRules {
                continue
            }
            ruleList, ok := rules.([]interface{})
            if !ok {
                errors = append(errors,
                    fmt.Sprintf("agents[%d].%s: must be a list", i, key))
                continue
            }
            for j, rule := range ruleList {
                ruleMap, ok := rule.(map[string]interface{})
                if !ok {
                    errors = append(errors,
                        fmt.Sprintf("agents[%d].%s[%d]: "+
                            "must be a map with 'tool' field", i, key, j))
                    continue
                }
                if _, ok := ruleMap["tool"]; !ok {
                    errors = append(errors,
                        fmt.Sprintf("agents[%d].%s[%d]: "+
                            "missing 'tool' field", i, key, j))
                }
            }
        }

        // Spend limit
        if spend, ok := agent["spend_limit_daily_usd"]; ok {
            switch v := spend.(type) {
            case int:
                if v < 0 {
                    errors = append(errors,
                        fmt.Sprintf("agents[%d].spend_limit_daily_usd: "+
                            "must be >= 0", i))
                }
            case float64:
                if v < 0 {
                    errors = append(errors,
                        fmt.Sprintf("agents[%d].spend_limit_daily_usd: "+
                            "must be >= 0", i))
                }
            default:
                errors = append(errors,
                    fmt.Sprintf("agents[%d].spend_limit_daily_usd: "+
                        "must be a number", i))
            }
        }

        if strict && id != "*" {
            hasRules := agent["allow"] != nil ||
                agent["block"] != nil ||
                agent["escalate"] != nil
            if !hasRules {
                warnings = append(warnings,
                    fmt.Sprintf("agents[%d] ('%s'): "+
                        "no rules defined — all decisions use default", i, id))
            }
        }
    }

    // Print results
    fmt.Printf("Validating: %s\n\n", policyFile)

    if len(errors) == 0 && len(warnings) == 0 {
        fmt.Printf("✓ Policy is valid\n")
        fmt.Printf("  %d agent(s) defined\n", len(agents))
        os.Exit(0)
    }

    if len(errors) > 0 {
        fmt.Printf("✗ %d error(s) found:\n\n", len(errors))
        for _, e := range errors {
            fmt.Printf("  ✗ %s\n", e)
        }
        fmt.Println()
    }

    if len(warnings) > 0 {
        fmt.Printf("⚠ %d warning(s):\n\n", len(warnings))
        for _, w := range warnings {
            fmt.Printf("  ⚠ %s\n", w)
        }
        fmt.Println()
    }

    if len(errors) > 0 {
        os.Exit(1)
    }
    os.Exit(0)
}
