# AgentGuard Python SDK

A protocol-neutral enforcement proxy client for AI agents.

## Installation

```bash
pip install agentguard
```

## Quickstart

```python
from agentguard import guard
from mcp import MCPClient

# Before:
# client = MCPClient("http://my-mcp-server:8080")

# After: wrap your upstream URL with AgentGuard
client = MCPClient(guard("http://my-mcp-server:8080"))
```

## Verify Webhooks

```python
from agentguard import verify_webhook

def handle_webhook(body_bytes: bytes, signature: str):
    if not verify_webhook(body_bytes, signature, "my-secret"):
        raise ValueError("Invalid signature")
    print("Valid webhook!")
```
