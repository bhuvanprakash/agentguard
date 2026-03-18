// src/policy.ts
// Policy loader and simulator for Node.js/TypeScript.

import { readFileSync } from 'fs'

export interface ToolRule {
  tool: string
}

export interface AgentPolicy {
  id:                        string
  allow?:                    ToolRule[]
  block?:                    ToolRule[]
  escalate?:                 ToolRule[]
  spend_limit_daily_usd?:    number
  irreversible_require_human?: boolean
}

export interface PolicyFile {
  version: string
  default: 'allow' | 'block' | 'escalate'
  agents:  AgentPolicy[]
}

export type Decision = 'allow' | 'block' | 'escalate'

const IRREVERSIBLE_TOOLS = new Set([
  'delete', 'delete_file', 'delete_record', 'drop_table',
  'drop_database', 'truncate', 'send_payment', 'transfer_funds',
  'charge_card', 'send_email', 'send_sms', 'execute_shell',
  'run_command', 'exec', 'bash', 'eval', 'deploy',
])

/**
 * Load a policy YAML file.
 * Requires 'js-yaml': npm install js-yaml
 */
export function loadPolicy(path: string): PolicyFile {
  let yaml: typeof import('js-yaml')
  try {
    yaml = require('js-yaml')
  } catch {
    throw new Error('js-yaml is required: npm install js-yaml')
  }
  const content = readFileSync(path, 'utf-8')
  return yaml.load(content) as PolicyFile
}

/**
 * Validate a policy object. Returns array of error strings.
 * Empty array = valid.
 */
export function validatePolicy(policy: PolicyFile): string[] {
  const errors: string[] = []

  if (!policy.version) errors.push('Missing required field: version')

  if (policy.default && !['allow','block','escalate'].includes(policy.default)) {
    errors.push(`Invalid default: '${policy.default}'`)
  }

  if (!Array.isArray(policy.agents)) {
    errors.push("'agents' must be an array")
    return errors
  }

  const seenIds = new Set<string>()
  policy.agents.forEach((agent, i) => {
    if (!agent.id) {
      errors.push(`agents[${i}]: missing 'id'`)
    } else if (seenIds.has(agent.id)) {
      errors.push(`agents[${i}]: duplicate id '${agent.id}'`)
    } else {
      seenIds.add(agent.id)
    }

    for (const key of ['allow', 'block', 'escalate'] as const) {
      const rules = agent[key]
      if (rules !== undefined && !Array.isArray(rules)) {
        errors.push(`agents[${i}].${key}: must be an array`)
      }
    }
  })

  return errors
}

/**
 * Simulate what decision AgentGuard would make.
 * Useful for unit-testing your policies.
 *
 * @example
 * const policy = loadPolicy('./policy.yaml')
 * expect(simulateDecision(policy, 'billing-agent', 'read_invoice'))
 *   .toBe('allow')
 */
export function simulateDecision(
  policy:  PolicyFile,
  agentId: string,
  tool:    string,
): Decision {
  const toolLow = tool.toLowerCase()

  // Hardcoded irreversible = always escalate
  if (IRREVERSIBLE_TOOLS.has(toolLow)) return 'escalate'

  const matched  = policy.agents.find(a => a.id === agentId)
  const wildcard = policy.agents.find(a => a.id === '*')
  const active   = matched ?? wildcard

  if (!active) return policy.default ?? 'block'

  const matches = (pattern: string) =>
    pattern === '*' || pattern.toLowerCase() === toolLow

  for (const r of active.block   ?? []) if (matches(r.tool)) return 'block'
  for (const r of active.escalate ?? []) if (matches(r.tool)) return 'escalate'
  for (const r of active.allow   ?? []) if (matches(r.tool)) return 'allow'

  return policy.default ?? 'block'
}
