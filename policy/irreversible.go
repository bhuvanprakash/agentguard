package policy

import "strings"

// IrreversibleTools is a hardcoded list of tool names that are
// ALWAYS escalated for human approval, regardless of what the
// YAML policy says.
//
// These are tools whose effects cannot be undone:
//   - Deleting data
//   - Sending payments
//   - Sending emails to external users
//   - Dropping tables or databases
//   - Executing shell commands
//
// This list is Nascentist's opinion on what "dangerous" means.
// Users can ADD to this list but cannot REMOVE from it via YAML.
// (Removing requires a code change and PR — intentional friction.)
//
// WHY hardcode this?
//   A YAML file can be misconfigured accidentally.
//   A junior dev can typo "escalate" as "allow".
//   This list is the last line of defense.

var IrreversibleTools = map[string]bool{
    // Data deletion
    "delete":           true,
    "delete_file":      true,
    "delete_record":    true,
    "drop_table":       true,
    "drop_database":    true,
    "truncate":         true,
    "truncate_table":   true,
    "destroy":          true,
    "purge":            true,
    "remove_user":      true,

    // Payments
    "send_payment":     true,
    "transfer_funds":   true,
    "charge_card":      true,
    "refund":           true,
    "require_payment":  true, // ATXP

    // External communication
    "send_email":       true,
    "send_sms":         true,
    "send_webhook":     true,
    "post_to_slack":    true,
    "notify_external":  true,

    // Code execution
    "execute_shell":    true,
    "run_command":      true,
    "exec":             true,
    "bash":             true,
    "eval":             true,

    // Infrastructure
    "deploy":           true,
    "shutdown_server":  true,
    "terminate_instance": true,
    "scale_down":       true,
}

// IsIrreversible returns true if the tool name is in the
// hardcoded irreversible list.
// Comparison is case-insensitive.
func IsIrreversible(toolName string) bool {
    return IrreversibleTools[strings.ToLower(toolName)]
}
