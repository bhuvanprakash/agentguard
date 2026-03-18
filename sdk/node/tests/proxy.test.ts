import { guard } from '../src/proxy'
import { verifyWebhook } from '../src/verify'
import { validatePolicy, simulateDecision } from '../src/policy'
import * as crypto from 'crypto'

describe('proxy guard', () => {
  it('encodes upstream URL properly', () => {
    const url = guard('http://my-api:8080', { proxyUrl: 'http://guard:7777' })
    expect(url).toContain('guard:7777')
    expect(url).toContain(encodeURIComponent('http://my-api:8080'))
  })

  it('includes agentId if provided', () => {
    const url = guard('http://api:8080', { agentId: 'test-agent' })
    expect(url).toContain('agent_id=test-agent')
  })
})

describe('verifyWebhook', () => {
  it('verifies valid signature', () => {
    const secret = 'test-secret'
    const body = '{"event":"test"}'
    const sig = crypto.createHmac('sha256', secret).update(body).digest('hex')
    expect(verifyWebhook(body, `sha256=${sig}`, secret)).toBe(true)
  })

  it('rejects invalid signature', () => {
    expect(verifyWebhook('body', 'sha256=wrong', 'secret')).toBe(false)
  })
})

describe('policy validation and simulation', () => {
  const policy: any = {
    version: '1',
    default: 'block',
    agents: [{ id: 'test', allow: [{ tool: 'read_file' }], escalate: [{ tool: 'escalate_this' }] }]
  }

  it('validates correct policy', () => {
    expect(validatePolicy(policy)).toEqual([])
  })

  it('catches missing version', () => {
    const errors = validatePolicy({ default: 'block', agents: [] } as any)
    expect(errors[0]).toContain('version')
  })

  it('simulates decisions accurately', () => {
    expect(simulateDecision(policy, 'test', 'read_file')).toBe('allow')
    expect(simulateDecision(policy, 'test', 'escalate_this')).toBe('escalate')
    expect(simulateDecision(policy, 'test', 'unknown')).toBe('block')
    // Irreversible tools escalate regardless of rules
    expect(simulateDecision(policy, 'test', 'delete_file')).toBe('escalate')
  })
})
