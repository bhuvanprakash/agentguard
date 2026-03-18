package policy

// PolicyFile is the top-level structure of a policy.yaml file.
//
// Example policy.yaml:
//
//   version: "1"
//   default: block
//
//   agents:
//     - id: billing-agent
//       allow:
//         - tool: send_invoice
//         - tool: read_customer
//       block:
//         - tool: delete_customer
//         - tool: drop_table
//       escalate:
//         - tool: send_payment
//       spend_limit_daily_usd: 500
//       irreversible_require_human: true
//
//     - id: "*"          # wildcard — applies to all other agents
//       allow:
//         - tool: "*"    # allow all tools
//       block: []
//
// Decision values: "allow", "block", "escalate"
// "escalate" = pause, send webhook, wait for human approval

import "gopkg.in/yaml.v3"

type Decision string

const (
    DecisionAllow    Decision = "allow"
    DecisionBlock    Decision = "block"
    DecisionEscalate Decision = "escalate"
)

type WhenCondition struct {
    Arg      string  `yaml:"arg"`       // argument key to inspect
    Matches  string  `yaml:"matches"`   // glob pattern
    Equals   string  `yaml:"equals"`    // exact string match
    GT       float64 `yaml:"gt"`        // numeric greater than
    LT       float64 `yaml:"lt"`        // numeric less than
    Contains string  `yaml:"contains"`  // string contains
    Prefix   string  `yaml:"prefix"`    // string prefix
}

type ToolRule struct {
    Tool string         `yaml:"tool"`
    When *WhenCondition `yaml:"when"`
}

type RateLimitConfig struct {
    RequestsPerMinute int `yaml:"requests_per_minute"`
    Burst             int `yaml:"burst"`
}

type AgentPolicy struct {
    ID                       string           `yaml:"id"`
    Allow                    []ToolRule       `yaml:"allow"`
    Block                    []ToolRule       `yaml:"block"`
    Escalate                 []ToolRule       `yaml:"escalate"`
    SpendLimitDailyUSD       float64          `yaml:"spend_limit_daily_usd"`
    IrreversibleRequireHuman bool             `yaml:"irreversible_require_human"`
    RateLimit               *RateLimitConfig `yaml:"rate_limit"`
    BusinessHoursOnly        bool             `yaml:"business_hours_only"`
    Timezone                 string           `yaml:"timezone"`
}

type PolicyFile struct {
    Version string        `yaml:"version"`
    Default Decision      `yaml:"default"`  // what to do when no rule matches
    Agents  []AgentPolicy `yaml:"agents"`
}

// ParsePolicy parses a YAML policy file from raw bytes.
func ParsePolicy(data []byte) (*PolicyFile, error) {
    var p PolicyFile
    if err := yaml.Unmarshal(data, &p); err != nil {
        return nil, err
    }
    // Default to "block" if not specified (fail safe)
    if p.Default == "" {
        p.Default = DecisionBlock
    }
    return &p, nil
}
