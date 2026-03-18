// src/index.ts
export { guard, AgentGuard }         from './proxy'
export { verifyWebhook }             from './verify'
export { loadPolicy, validatePolicy, simulateDecision } from './policy'
export type { EscalationItem, HealthStatus } from './proxy'
export type { PolicyFile, AgentPolicy, ToolRule, Decision } from './policy'
