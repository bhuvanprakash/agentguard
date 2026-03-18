// Zero dependencies — works in Node.js, Deno, Bun, Edge runtimes.

import crypto from 'crypto'

const DEFAULT_PROXY =
  process.env.AGENTGUARD_URL ?? 'http://localhost:7777'

/**
 * Wrap an upstream URL with AgentGuard enforcement.
 *
 * Returns the AgentGuard proxy URL.
 * Pass this to any MCP or HTTP client instead of the real URL.
 *
 * @example
 * // Before
 * const client = new MCPClient('http://my-mcp-server:8080')
 *
 * // After (one word change)
 * const client = new MCPClient(guard('http://my-mcp-server:8080'))
 */
export function guard(
  upstreamUrl: string,
  options?: {
    proxyUrl?: string
    agentId?:  string
  }
): string {
  const base = (options?.proxyUrl ?? DEFAULT_PROXY).replace(/\/$/, '')
  const encoded = encodeURIComponent(upstreamUrl)
  let url = `${base}?upstream=${encoded}`
  if (options?.agentId) {
    url += `&agent_id=${encodeURIComponent(options.agentId)}`
  }
  return url
}

export interface EscalationItem {
  id:          string
  agent_id:    string
  tool_name:   string
  protocol:    string
  arguments:   Record<string, unknown>
  ts:          string
  expires_at:  string
  age_minutes: number
}

export interface HealthStatus {
  status:      string
  version:     string
  uptime_s:    number
  environment: string
  goroutines:  number
}

/**
 * AgentGuard client with full API access.
 *
 * @example
 * const ag = new AgentGuard({
 *   proxyUrl:  'http://agentguard:7777',
 *   agentId:   'billing-agent',
 *   adminKey:  process.env.AGENTGUARD_ADMIN_KEY,
 * })
 *
 * const client = new MCPClient(ag.wrap('http://my-api:8080'))
 * const pending = await ag.listEscalations()
 * await ag.approve(pending.id)
 */
export class AgentGuard {
  private proxyUrl: string
  private agentId?: string
  private signingSecret: string
  private adminKey: string

  constructor(options?: {
    proxyUrl?: string
    agentId?:  string
    signingSecret?: string
    adminKey?: string
  }) {
    this.proxyUrl = (options?.proxyUrl ?? DEFAULT_PROXY).replace(/\/$/, '')
    this.agentId  = options?.agentId
    this.signingSecret = options?.signingSecret ?? process.env.AGENTGUARD_SIGNING_SECRET ?? ''
    this.adminKey = options?.adminKey
      ?? process.env.AGENTGUARD_ADMIN_KEY
      ?? ''
  }

  /**
   * Build HMAC-SHA256 signature over canonical string.
   * Returns "sha256=<hex>"
   */
  private signRequest(body: string, timestamp: string): string {
    const bodyHash = crypto
      .createHash('sha256')
      .update(body)
      .digest('hex')

    const canonical =
      `${this.agentId}\n${timestamp}\n${bodyHash}`

    const sig = crypto
      .createHmac('sha256', this.signingSecret)
      .update(canonical)
      .digest('hex')

    return `sha256=${sig}`
  }

  /**
   * Returns auth headers for a request body.
   * Returns empty object if no signing secret (dev mode).
   */
  signedHeaders(body: string): Record<string, string> {
    if (!this.signingSecret || !this.agentId) return {}

    const ts  = String(Math.floor(Date.now() / 1000))
    const sig = this.signRequest(body, ts)

    return {
      'X-Agent-ID':             this.agentId,
      'X-AgentGuard-Timestamp': ts,
      'X-AgentGuard-Signature': sig,
    }
  }

  /**
   * Make a signed fetch call through AgentGuard.
   * Automatically adds auth headers.
   *
   * Usage:
   *   const result = await guard.fetch({
   *     method:  'tools/call',
   *     params:  { name: 'read_file', arguments: {} },
   *   })
   */
  async fetch(payload: {
    method:  string
    params?: Record<string, unknown>
    id?:     string | number
  }): Promise<Response> {
    const body = JSON.stringify({
      jsonrpc: '2.0',
      id:      payload.id ?? crypto.randomUUID(),
      method:  payload.method,
      params:  payload.params ?? {},
    })

    return globalThis.fetch(this.proxyUrl, {
      method:  'POST',
      headers: {
        'Content-Type': 'application/json',
        ...this.signedHeaders(body),
      },
      body,
    })
  }

  /** Wrap an upstream URL with this instance's config. */
  wrap(upstreamUrl: string): string {
    return guard(upstreamUrl, {
      proxyUrl: this.proxyUrl,
      agentId:  this.agentId,
    })
  }

  /** List pending escalations. */
  async listEscalations(): Promise<EscalationItem[]> {
    try {
      const res = await fetch(`${this.proxyUrl}/escalations`, {
        headers: { 'X-AgentGuard-Admin-Key': this.adminKey },
      })
      const data = await res.json() as { escalations: EscalationItem[] }
      return data.escalations ?? []
    } catch {
      return []
    }
  }

  /** Approve a pending escalation. */
  async approve(escalationId: string): Promise<boolean> {
    return this._resolve(escalationId, 'approve')
  }

  /** Reject a pending escalation. */
  async reject(escalationId: string): Promise<boolean> {
    return this._resolve(escalationId, 'reject')
  }

  /** Check AgentGuard health. */
  async health(): Promise<HealthStatus | null> {
    try {
      const res = await fetch(`${this.proxyUrl}/health`)
      return await res.json() as HealthStatus
    } catch {
      return null
    }
  }

  private async _resolve(
    id:     string,
    action: 'approve' | 'reject',
  ): Promise<boolean> {
    try {
      const res = await fetch(
        `${this.proxyUrl}/escalations/${id}/${action}`,
        {
          method:  'POST',
          headers: { 'X-AgentGuard-Admin-Key': this.adminKey },
        }
      )
      return res.ok
    } catch {
      return false
    }
  }
}
