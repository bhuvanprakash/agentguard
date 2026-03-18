// policy/advanced.go
// Advanced policy rule evaluation:
//   1. Argument inspection — match tool call arguments
//   2. Time-based rules   — business hours restrictions
//   3. Pattern matching   — glob patterns for tool names

package policy

import (
    "fmt"
    "path/filepath"
    "strconv"
    "strings"
    "time"
)

// EvaluateWhen checks if a WhenCondition is satisfied
// by the given arguments map.
// Returns true if condition is met (rule should apply).
// Returns true if no condition (rule always applies).
func EvaluateWhen(
    when *WhenCondition,
    args map[string]interface{},
) bool {
    if when == nil {
        return true // no condition = always applies
    }

    val, ok := args[when.Arg]
    if !ok {
        // Argument not present — condition not met
        return false
    }

    strVal := fmt.Sprintf("%v", val)

    if when.Matches != "" {
        matched, err := filepath.Match(when.Matches, strVal)
        if err != nil {
            return false
        }
        return matched
    }

    if when.Equals != "" {
        return strVal == when.Equals
    }

    if when.Contains != "" {
        return strings.Contains(strVal, when.Contains)
    }

    if when.Prefix != "" {
        return strings.HasPrefix(strVal, when.Prefix)
    }

    if when.GT != 0 || when.LT != 0 {
        numVal, err := strconv.ParseFloat(strVal, 64)
        if err != nil {
            return false
        }
        if when.GT != 0 && numVal <= when.GT {
            return false
        }
        if when.LT != 0 && numVal >= when.LT {
            return false
        }
        return true
    }

    return true
}

// IsBusinessHours returns true if the current time in
// the given timezone is within business hours (Mon-Fri, 9-18).
// If timezone is empty, uses UTC.
func IsBusinessHours(timezone string) bool {
    loc := time.UTC
    if timezone != "" {
        if l, err := time.LoadLocation(timezone); err == nil {
            loc = l
        }
    }

    now := time.Now().In(loc)
    weekday := now.Weekday()
    hour    := now.Hour()

    // Monday–Friday, 9am–6pm
    isWeekday := weekday >= time.Monday &&
        weekday <= time.Friday
    isDayHours := hour >= 9 && hour < 18

    return isWeekday && isDayHours
}

// MatchesGlob checks if a tool name matches a pattern.
// Supports:
//   "*"            → matches anything
//   "delete_*"     → prefix glob
//   "*_payment"    → suffix glob
//   "read_file"    → exact match
func MatchesGlob(pattern, toolName string) bool {
    if pattern == "*" {
        return true
    }
    if !strings.ContainsAny(pattern, "*?") {
        return strings.EqualFold(pattern, toolName)
    }
    matched, err := filepath.Match(
        strings.ToLower(pattern),
        strings.ToLower(toolName),
    )
    if err != nil {
        return false
    }
    return matched
}
