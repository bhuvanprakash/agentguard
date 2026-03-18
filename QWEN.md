# AgentGuard — Project Context

## Project Overview

**AgentGuard** is an open-source, protocol-neutral enforcement proxy for AI agents. It sits between AI agents and real-world APIs/tools, intercepting every tool call and checking it against a YAML policy before execution.

### Core Purpose
- **Security Layer**: Prevents AI agents from executing destructive actions (file deletion, payments, database drops) without approval
- **Policy Enforcement**: Decisions are `allow`, `block`, or `escalate` (pause for human approval)
- **Protocol Support**: MCP (Model Context Protocol), A2A (Agent-to-Agent), ATXP (Agent Transaction), and generic HTTP

### Architecture
```
Agent → AgentGuard (policy check) → Real API
                    ↓
            allow | block | escalate
```

### Key Components

| Directory | Purpose |
|-----------|---------|
| `proxy/` | Core HTTP interception engine, middleware, rate limiting |
| `policy/` | YAML policy parsing, rule evaluation, irreversible tool list |
| `auth/` | HMAC-SHA256 request signing, agent registration, Supabase-backed secret store |
| `audit/` | SQLite audit logging with async Supabase sync |
| `escalation/` | Escalation management, approval/rejection workflows |
| `spend/` | Daily spend limit tracking per agent |
| `memory/` | Nascentist memory integration for context-aware decisions |
| `notify/` | Slack/Discord webhook notifications |
| `sdk/` | Python and Node.js client libraries |
| `config/` | Environment variable loading and validation |
| `protocol/` | MCP, A2A, ATXP request parsing |

### Technology Stack
- **Language**: Go 1.22
- **Database**: SQLite (local) + Supabase (cloud sync)
- **Deployment**: Docker, Fly.io
- **Auth**: HMAC-SHA256 with replay protection (±300s timestamp window)

---

## Building and Running

### Prerequisites
- Go 1.22+
- Docker (optional, for containerized deployment)
- Supabase project (for audit sync and agent registration)

### Environment Variables

**Required:**
```bash
AGENTGUARD_UPSTREAM_URL=http://your-mcp-server:8080
SUPABASE_URL=https://xxx.supabase.co
SUPABASE_SERVICE_KEY=eyJ...
```

**Optional:**
```bash
AGENTGUARD_PORT=7777
AGENTGUARD_POLICY_FILE=./policy.yaml
AGENTGUARD_SQLITE_PATH=./audit.db
AGENTGUARD_ADMIN_KEY=your_admin_key
AGENTGUARD_WEBHOOK_URL=https://your-webhook.com
AGENTGUARD_WEBHOOK_SECRET=hmac_secret
NASCENTIST_MEMORY_URL=https://nascentist.ai
```

See `.env.example` for full reference.

### Quick Start

```bash
# Run directly
make run

# Or with environment
AGENTGUARD_UPSTREAM_URL=http://localhost:8080 \
SUPABASE_URL=https://xxx.supabase.co \
SUPABASE_SERVICE_KEY=eyJ... \
go run main.go

# Build binary
make build
# Output: bin/agentguard (~8MB static binary)

# Docker
make docker
docker run -p 7777:7777 --env-file .env agentguard:latest
```

### Testing

```bash
# Run all tests
make test

# SDK tests
make test-python    # Python SDK
make test-node      # Node SDK

# Smoke tests (end-to-end)
make smoke-test

# Individual curl tests
make test-block     # Test MCP block decision
make test-allow     # Test MCP allow decision
make test-escalate  # Test escalation flow
make health         # Health check
make reload         # Hot-reload policy
```

### Policy Management

```bash
# Validate policy syntax
make validate-policy

# Hot-reload without restart
curl -X POST http://localhost:7777/reload-policy \
  -H "X-AgentGuard-Admin-Key: $AGENTGUARD_ADMIN_KEY"
```

---

## Policy Reference

### Example `policy.yaml`

```yaml
version: "1"
default: block

agents:
  - id: billing-agent
    allow:
      - tool: read_invoice
      - tool: read_customer
    block:
      - tool: delete_customer
      - tool: drop_table
    escalate:
      - tool: send_payment
    spend_limit_daily_usd: 500
    business_hours_only: true
    timezone: "America/New_York"

  # Wildcard for unlisted agents
  - id: "*"
    allow:
      - tool: read_file
    block:
      - tool: "*"
```

### Decision Priority (highest → lowest)
1. **Hardcoded irreversible tools** → always escalate (`delete`, `drop_table`, `send_payment`, `execute_shell`, `send_email`, `deploy`)
2. **Agent `block` rules** → block
3. **Agent `escalate` rules** → escalate
4. **Agent `allow` rules** → allow
5. **Policy `default`** → fallback

### Tool Rules with Conditions

```yaml
escalate:
  - tool: send_payment
    when:
      arg: amount
      gt: 1000  # escalate only if amount > 1000
```

Condition types: `matches` (glob), `equals`, `gt`, `lt`, `contains`, `prefix`

---

## API Endpoints

| Endpoint | Method | Auth | Description |
|----------|--------|------|-------------|
| `/health` | GET | — | Health check + stats |
| `/reload-policy` | POST | Admin key | Hot-reload policy |
| `/escalations` | GET | Admin key | List pending escalations |
| `/escalations/:id` | GET | Admin key | Get escalation details |
| `/escalations/:id/approve` | POST | Admin key | Approve escalation |
| `/escalations/:id/reject` | POST | Admin key | Reject escalation |
| `/api/v1/status` | GET | Admin key | Dashboard status |

**Auth Headers:**
- Admin endpoints: `X-AgentGuard-Admin-Key`
- Agent auth: `X-AgentGuard-Signature` + `X-AgentGuard-Timestamp` (HMAC) or `X-AgentGuard-Key` (key_only mode)

---

## Development Conventions

### Code Style
- Go standard formatting (`go fmt`)
- Package-level comments for exported types/functions
- Error handling with wrapped context: `fmt.Errorf("context: %w", err)`
- Logging with `log` package, prefix: `[agentguard]`

### Testing Practices
- Unit tests in `*_test.go` files alongside source
- End-to-end smoke tests in `scripts/smoke-test.sh`
- SDK tests use `pytest` (Python) and `jest`/`npm test` (Node)

### Git Workflow
- Descriptive commit messages
- Tag releases: `make release VERSION=0.1.0`
- PRs should update docs if features change

### Project Structure Conventions
- Each package is self-contained with clear responsibilities
- `proxy/` handles HTTP, `policy/` handles YAML evaluation
- No circular dependencies between packages
- SQLite schema in `audit/schema.sql`, auto-applied at startup

---

## Key Files

| File | Purpose |
|------|---------|
| `main.go` | Entry point, loads config, starts proxy server |
| `Makefile` | Build, test, and deployment commands |
| `policy.yaml` | Example policy configuration |
| `Dockerfile` | Multi-stage build for static binary |
| `fly.toml` | Fly.io deployment configuration |
| `CONTRIBUTING.md` | Contribution guidelines |
| `LAUNCH.md` | Launch announcement templates |

---

## Common Workflows

### Add a new irreversible tool
Edit `policy/irreversible.go` to add tool name patterns to the hardcoded list.

### Test a decision locally
```bash
make simulate AGENT=billing-agent TOOL=send_payment
```

### Debug policy evaluation
Check `serve.log` for decision logs with matched rules.

### Build SDKs for release
```bash
make build-sdks
# Outputs: sdk/python/dist/*.whl, sdk/node/dist/*.tgz
```

### Install CLI globally
```bash
make install-cli
# Installs to /usr/local/bin/agentguard
```

---

## External Dependencies

| Dependency | Purpose |
|------------|---------|
| `gopkg.in/yaml.v3` | YAML policy parsing |
| `github.com/joho/godotenv` | `.env` file loading |
| `github.com/mattn/go-sqlite3` | Local audit logging (CGO) |
| `github.com/google/uuid` | UUID generation for log IDs |

---

## Notes

- **CGO Required**: SQLite binding requires CGO. Build with `CGO_ENABLED=1`
- **Static Binary**: Docker build includes `build-base` for CGO compilation
- **Replay Protection**: Timestamps must be within ±300 seconds
- **Rate Limiting**: Configurable per-agent via `rate_limit` in policy
- **Business Hours**: Optional `business_hours_only: true` with timezone support
