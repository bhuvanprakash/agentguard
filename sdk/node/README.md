# AgentGuard Node.js SDK

A protocol-neutral enforcement proxy client for AI agents.

## Installation

```bash
npm install agentguard
```

## Quickstart

```typescript
import { guard } from 'agentguard'
import { MCPClient } from '@modelcontextprotocol/sdk'

// Before:
// const client = new MCPClient('http://my-mcp-server:8080')

// After: wrap your upstream URL with AgentGuard
const client = new MCPClient(guard('http://my-mcp-server:8080'))
```

## Verify Webhooks

```typescript
import { verifyWebhook } from 'agentguard'

// Example in Express:
app.post('/webhook', express.raw({type: '*/*'}), (req, res) => {
  const valid = verifyWebhook(
    req.body,
    req.headers['x-agentguard-signature'],
    process.env.AGENTGUARD_WEBHOOK_SECRET
  )
  if (!valid) return res.status(403).send('Forbidden')
  res.send('ok')
})
```
