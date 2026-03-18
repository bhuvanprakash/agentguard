# agentguard/policy.py
# Policy YAML loader and validator for Python.
# Useful for:
#   - Validating a policy file before deploying it
#   - Generating policy files programmatically
#   - Testing what decision would be made for a given agent+tool

from __future__ import annotations
import os
from typing import Optional


def load_policy(path: str) -> dict:
    """
    Load and parse a policy YAML file.

    Args:
        path: Path to the policy.yaml file.

    Returns:
        dict: Parsed policy data.

    Raises:
        FileNotFoundError: If the file does not exist.
        ValueError: If the YAML is invalid.
    """
    try:
        import yaml
    except ImportError:
        raise ImportError(
            "PyYAML is required: pip install pyyaml"
        )

    if not os.path.exists(path):
        raise FileNotFoundError(f"Policy file not found: {path}")

    with open(path, "r") as f:
        data = yaml.safe_load(f)

    return data or {}


def validate_policy(policy: dict) -> list[str]:
    """
    Validate a policy dict against the AgentGuard schema.

    Returns:
        list[str]: List of validation errors.
                   Empty list = valid policy.

    Example:
        policy = load_policy("./policy.yaml")
        errors = validate_policy(policy)
        if errors:
            for e in errors:
                print(f"  ✗ {e}")
        else:
            print("  ✓ Policy is valid")
    """
    errors = []

    if "version" not in policy:
        errors.append("Missing required field: version")

    default = policy.get("default", "")
    if default not in ("allow", "block", "escalate", ""):
        errors.append(
            f"Invalid default decision: '{default}'. "
            "Must be 'allow', 'block', or 'escalate'."
        )

    agents = policy.get("agents", [])
    if not isinstance(agents, list):
        errors.append("'agents' must be a list")
        return errors

    seen_ids = set()
    for i, agent in enumerate(agents):
        if not isinstance(agent, dict):
            errors.append(f"agents[{i}]: must be a dict")
            continue

        aid = agent.get("id")
        if not aid:
            errors.append(f"agents[{i}]: missing 'id' field")
        elif aid in seen_ids:
            errors.append(f"agents[{i}]: duplicate id '{aid}'")
        else:
            seen_ids.add(aid)

        # Validate rule lists
        for rule_key in ("allow", "block", "escalate"):
            rules = agent.get(rule_key, [])
            if not isinstance(rules, list):
                errors.append(
                    f"agents[{i}].{rule_key}: must be a list"
                )
                continue
            for j, rule in enumerate(rules):
                if not isinstance(rule, dict) or "tool" not in rule:
                    errors.append(
                        f"agents[{i}].{rule_key}[{j}]: "
                        "must have a 'tool' field"
                    )

        spend = agent.get("spend_limit_daily_usd")
        if spend is not None and (
            not isinstance(spend, (int, float)) or spend < 0
        ):
            errors.append(
                f"agents[{i}].spend_limit_daily_usd: "
                "must be a non-negative number"
            )

    return errors


def simulate_decision(
    policy:   dict,
    agent_id: str,
    tool:     str,
) -> str:
    """
    Simulate what decision AgentGuard would make.
    Useful for testing policies without running the Go service.

    Returns: "allow", "block", or "escalate"
    """
    # Hardcoded irreversible tools always escalate
    _IRREVERSIBLE = {
        "delete", "delete_file", "delete_record", "drop_table",
        "drop_database", "truncate", "send_payment", "transfer_funds",
        "charge_card", "send_email", "send_sms", "execute_shell",
        "run_command", "exec", "bash", "eval", "deploy",
    }
    if tool.lower() in _IRREVERSIBLE:
        return "escalate"

    agents   = policy.get("agents", [])
    default  = policy.get("default", "block")
    tool_low = tool.lower()

    # Find matching agent policy
    matched  = None
    wildcard = None
    for a in agents:
        if a.get("id") == agent_id:
            matched = a
            break
        if a.get("id") == "*":
            wildcard = a

    active = matched or wildcard
    if not active:
        return default

    def matches(pattern: str) -> bool:
        return pattern == "*" or pattern.lower() == tool_low

    # Block takes priority
    for rule in active.get("block", []):
        if matches(rule.get("tool", "")):
            return "block"

    # Then escalate
    for rule in active.get("escalate", []):
        if matches(rule.get("tool", "")):
            return "escalate"

    # Then allow
    for rule in active.get("allow", []):
        if matches(rule.get("tool", "")):
            return "allow"

    return default
