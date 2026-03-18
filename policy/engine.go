package policy

// Engine holds a loaded policy and evaluates decisions.
//
// Usage:
//   engine, err := NewEngine("./policy.yaml")
//   decision := engine.Evaluate("billing-agent", "delete_customer")
//   // returns DecisionEscalate (hardcoded irreversible)

import (
    "fmt"
    "os"
    "strings"
    "sync"
)

type Engine struct {
    mu     sync.RWMutex
    policy *PolicyFile
    path   string
}

// NewEngine loads a policy YAML file and returns an Engine.
// The Engine is safe for concurrent use.
func NewEngine(policyFilePath string) (*Engine, error) {
    e := &Engine{path: policyFilePath}
    if err := e.Reload(); err != nil {
        return nil, err
    }
    return e, nil
}

// Reload reads and parses the policy file.
// Call this to hot-reload policies without restarting.
func (e *Engine) Reload() error {
    data, err := os.ReadFile(e.path)
    if err != nil {
        return fmt.Errorf("policy: cannot read %s: %w", e.path, err)
    }
    p, err := ParsePolicy(data)
    if err != nil {
        return fmt.Errorf("policy: cannot parse %s: %w", e.path, err)
    }
    e.mu.Lock()
    e.policy = p
    e.mu.Unlock()
    return nil
}

// Evaluate decides what to do with a given agent+tool combination.
//
// Decision priority (highest to lowest):
//   1. IrreversibleTools hardcoded list → always escalate
//   2. Exact agent ID match in policy   → use its rules
//   3. Wildcard agent "*" in policy     → use its rules
//   4. Policy default                   → use default decision
func (e *Engine) Evaluate(agentID, toolName string, args map[string]interface{}) (Decision, string) {
    e.mu.RLock()
    defer e.mu.RUnlock()

    // Priority 1: hardcoded irreversible list
    if IsIrreversible(toolName) {
        return DecisionEscalate, "irreversible_hardcoded"
    }

    toolLower := strings.ToLower(toolName)
    _ = toolLower

    // Find matching agent policy (exact, then wildcard)
    var matchedPolicy *AgentPolicy
    var wildcardPolicy *AgentPolicy

    for i := range e.policy.Agents {
        ap := &e.policy.Agents[i]
        if ap.ID == agentID {
            matchedPolicy = ap
            break
        }
        if ap.ID == "*" {
            wildcardPolicy = ap
        }
    }

    active := matchedPolicy
    if active == nil {
        active = wildcardPolicy
    }
    if active == nil {
        return e.policy.Default, "default_no_agent"
    }

    // Check business hours if restricted
    if active.BusinessHoursOnly && !IsBusinessHours(active.Timezone) {
        return DecisionEscalate, "outside_business_hours"
    }

    // Check block rules first (most restrictive)
    for _, rule := range active.Block {
        if MatchesGlob(rule.Tool, toolName) && EvaluateWhen(rule.When, args) {
            return DecisionBlock, "block_rule"
        }
    }

    // Check escalate rules
    for _, rule := range active.Escalate {
        if MatchesGlob(rule.Tool, toolName) && EvaluateWhen(rule.When, args) {
            return DecisionEscalate, "escalate_rule"
        }
    }

    // Check allow rules
    for _, rule := range active.Allow {
        if MatchesGlob(rule.Tool, toolName) && EvaluateWhen(rule.When, args) {
            return DecisionAllow, "allow_rule"
        }
    }

    // Nothing matched — use default
    return e.policy.Default, "default_matched_agent"
}

// matchesTool matches a tool name against a rule pattern.
// Supports "*" wildcard (matches anything).
func matchesTool(pattern, toolName string) bool {
    if pattern == "*" {
        return true
    }
    return strings.ToLower(pattern) == toolName
}

// GetAgentPolicy returns the policy for a specific agent ID.
// Returns nil if no policy is found (use default).
func (e *Engine) GetAgentPolicy(agentID string) *AgentPolicy {
    e.mu.RLock()
    defer e.mu.RUnlock()
    for i := range e.policy.Agents {
        if e.policy.Agents[i].ID == agentID {
            return &e.policy.Agents[i]
        }
    }
    return nil
}

// AgentCount returns the number of agents in the loaded policy.
func (e *Engine) AgentCount() int {
    e.mu.RLock()
    defer e.mu.RUnlock()
    return len(e.policy.Agents)
}

// GetAgents returns a copy of the currently loaded agent policies.
// Used for reconfiguring the rate limiter.
func (e *Engine) GetAgents() []AgentPolicy {
    e.mu.RLock()
    defer e.mu.RUnlock()
    // return a shallow copy
    agents := make([]AgentPolicy, len(e.policy.Agents))
    copy(agents, e.policy.Agents)
    return agents
}
