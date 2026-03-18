# AgentGuard

**A protocol-neutral enforcement proxy for AI agents.**

AgentGuard sits between your AI agent and the real world.
Every MCP tool call, A2A task delegation, and ATXP payment
passes through AgentGuard first — checked against your policy
before anything executes.

```
Your Agent → AgentGuard → Real API
                ↓
          Policy Check
          allow | block | escalate
```

## Why AgentGuard?

AI agents are being given real tools: file deletion, payment
processing, email sending, database queries. When they make
mistakes — and they will — the damage is real and often
irreversible.

**AgentGuard adds one enforcement layer you control.**

- **Protocol-neutral**: works with MCP, A2A, ATXP, and any
  HTTP-based agent framework
- **Policy-as-YAML**: readable by any developer, not just DevOps
- **Open source core**: MIT license, self-hostable, no vendor lock-in
- **Built in Go**: single 8MB binary, <1ms policy overhead,
  handles 10,000 concurrent agent connections

***

## Quickstart (5 minutes)

### 1. Run AgentGuard

```bash
docker run -p 7777:7777 \
  -e AGENTGUARD_UPSTREAM_URL=http://your-mcp-server:8080 \
  -e SUPABASE_URL=https://xxx.supabase.co \
  -e SUPABASE_SERVICE_KEY=eyJ... \
  -v $(pwd)/policy.yaml:/app/policy.yaml \
  nascentist/agentguard:latest
```

### 2. Create a policy

```yaml
# policy.yaml
version: "1"
default: block

agents:
  - id: my-agent
    allow:
      - tool: read_file
      - tool: search
    block:
      - tool: delete_file
      - tool: execute_shell
    escalate:
      - tool: send_payment   # needs human approval
```

### 3. Point your agent at AgentGuard

**Python**
```bash
pip install agentguard
```
```python
from agentguard import AgentGuard
from mcp import MCPClient

# Initialize with signing secret for security
guard = AgentGuard(
    agent_id="my-agent",
    signing_secret="agk_xK3m..." 
)

client = MCPClient(guard.wrap("http://my-mcp-server:8080"))
```

**Node.js**
```bash
npm install agentguard
```
```typescript
import { AgentGuard } from 'agentguard'
import { MCPClient } from '@modelcontextprotocol/sdk'

const guard = new AgentGuard({
    agentId: 'my-agent',
    signingSecret: process.env.AGENTGUARD_SECRET,
})

const client = new MCPClient(guard.wrap('http://my-mcp-server:8080'))
```

***

## 🔐 Security & Authentication

AgentGuard prevents **agent impersonation** using `HMAC-SHA256` request signing. 

1. **Register your agent** in the [Nascentist Dashboard](https://nascentist.ai/dashboard/agents/register) to receive a signing secret (`agk_...`).
2. **Configure your agent SDK** with this secret. 
3. **The Proxy verifies** every request. If the signature is missing or incorrect, AgentGuard returns `401 Unauthorized`.

**Replay Protection**: All signed requests include a `X-AgentGuard-Timestamp`. The proxy rejects any request with a clock skew > 300 seconds.

***

That is it. Every tool call your agent makes now goes through
AgentGuard and is checked against your policy before executing.

***

## How It Works

```
Agent calls tools/call { name: "delete_file", ... }
         │
         ▼
  AgentGuard receives the call
         │
         ├─ Check hardcoded irreversible list
         │   (delete, drop_table, send_payment, etc.)
         │   → always escalate, cannot be overridden
         │
         ├─ Look up agent ID in policy.yaml
         │   → match to allow/block/escalate rule
         │
         ├─ Check daily spend limit
         │   → auto-block if exceeded
         │
         └─ Decision:
              allow    → forward to real API
              block    → return error immediately
              escalate → store, notify you, wait for approval
```

***

## Policy Reference

```yaml
version: "1"

# Default when no rule matches an agent+tool
# Safe default: "block" (deny unknown actions)
default: block

agents:

  # Specific agent rules
  - id: billing-agent
    allow:
      - tool: read_invoice
      - tool: read_customer
    block:
      - tool: delete_customer
      - tool: drop_table
    escalate:
      - tool: send_invoice     # pauses for human approval
    spend_limit_daily_usd: 500

  # Wildcard: applies to all unlisted agents
  - id: "*"
    allow:
      - tool: read_file
    block:
      - tool: "*"              # block everything else
```

**Decision priority (highest → lowest)**
1. Hardcoded irreversible tools → always escalate
2. Agent `block` rules → block
3. Agent `escalate` rules → escalate
4. Agent `allow` rules → allow
5. Policy `default` → fallback

**Hardcoded irreversible tools** (cannot be overridden):
`delete`, `drop_table`, `send_payment`, `execute_shell`,
`send_email`, `deploy`, and others.
[Full list →](https://nascentist.ai/docs/agentguard/policies)

***

## Escalation Flow

When a tool call is escalated:

1. AgentGuard returns `202 Accepted` with an `escalation_id`
2. Your webhook URL receives a POST notification (HMAC-signed)
3. You receive an email alert
4. You approve or reject from the dashboard

```bash
# Approve via CLI
curl -X POST http://localhost:7777/escalations/<id>/approve \
  -H "X-AgentGuard-Admin-Key: $AGENTGUARD_ADMIN_KEY"
```

Or approve from the [Nascentist dashboard →](https://nascentist.ai/dashboard/agents/escalations)

***

## API Reference

| Endpoint | Method | Description |
|---|---|---|
| `/health` | GET | Service health + stats |
| `/reload-policy` | POST | Hot-reload policy without restart |
| `/escalations` | GET | List pending escalations |
| `/escalations/:id` | GET | Get escalation status |
| `/escalations/:id/approve` | POST | Approve an escalation |
| `/escalations/:id/reject` | POST | Reject an escalation |

**Auth**: `X-AgentGuard-Admin-Key` header for escalation endpoints.

***

## Environment Variables

| Variable | Required | Description |
|---|---|---|
| `AGENTGUARD_UPSTREAM_URL` | ✅ | URL to forward allowed requests |
| `SUPABASE_URL` | ✅ | Supabase project URL |
| `SUPABASE_SERVICE_KEY` | ✅ | Supabase service role key |
| `AGENTGUARD_PORT` | — | Port (default: 7777) |
| `AGENTGUARD_POLICY_FILE` | — | Policy YAML path (default: ./policy.yaml) |
| `AGENTGUARD_ADMIN_KEY` | — | Key for approve/reject endpoints |
| `AGENTGUARD_WEBHOOK_URL` | — | URL to notify on escalation |
| `AGENTGUARD_WEBHOOK_SECRET` | — | HMAC secret for webhook signing |
| `NASCENTIST_MEMORY_URL` | — | Nascentist memory API URL |

***

## Supported Protocols

| Protocol | Support | Notes |
|---|---|---|
| **MCP** (Model Context Protocol) | ✅ Full | JSON-RPC `tools/call` interception |
| **A2A** (Agent-to-Agent) | ✅ Full | `/messages/send`, `/tasks/create` |
| **ATXP** (Agent Transaction) | ✅ Full | `requirePayment` with spend tracking |
| **Generic HTTP** | ✅ | Pass-through with audit logging |

***

## Self-Hosting

```bash
# Build from source
git clone https://github.com/nascentist/agentguard
cd agentguard
make build
# Binary: bin/agentguard (~8MB, no dependencies)

# Or Docker
make docker
docker run -p 7777:7777 --env-file .env agentguard:latest
```

***

## Cloud Tier (Nascentist)

Self-hosting is always free (MIT license).

For teams that want managed AgentGuard with:
- Dashboard: [nascentist.ai/dashboard/agents](https://nascentist.ai/dashboard/agents)
- Policy editor in browser
- Email + webhook alerts
- Multi-agent spend analytics

[See pricing →](https://nascentist.ai/dashboard/billing)

***

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md).

The most valuable contributions right now:
- Additional protocol support (OpenAI function calling, LangChain tools)
- More examples in `examples/`
- Integration tests
- Bug reports with reproduction steps

***

## License

MIT — see [LICENSE](LICENSE).

Built by [Nascentist](https://nascentist.ai).
