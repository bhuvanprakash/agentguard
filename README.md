<div align="center">
  <img src="https://nascentist.ai/images/agentguard-logo.png" alt="AgentGuard Logo" width="120" />
  <h1>AgentGuard</h1>
  <p><strong>The Protocol-Neutral Enforcement Proxy for AI Agents</strong></p>

  <p>
    <a href="https://github.com/bhuvanprakash/agentguard/actions"><img src="https://img.shields.io/github/actions/workflow/status/bhuvanprakash/agentguard/test.yml?branch=main&style=flat-square" alt="Build Status" /></a>
    <a href="https://github.com/bhuvanprakash/agentguard/releases"><img src="https://img.shields.io/github/v/release/bhuvanprakash/agentguard?style=flat-square&color=blue" alt="Latest Release" /></a>
    <a href="LICENSE"><img src="https://img.shields.io/badge/license-Apache%202.0-orange?style=flat-square" alt="License" /></a>
    <a href="https://nascentist.ai/docs/agentguard"><img src="https://img.shields.io/badge/docs-nascentist.ai-green?style=flat-square" alt="Documentation" /></a>
  </p>
</div>

***

## 🛡️ Secure Your AI Agents in Production

AgentGuard is a high-performance security proxy that sits between your AI agent and the real world. Every tool call, task delegation, and payment transaction is intercepted and validated against your custom policy before execution.

### ✨ Key Features

- **⚡ Blazing Fast**: Built in Go, adding <1ms overhead to your agent pipeline.
- **🔌 Protocol Agnostic**: Native support for **MCP** (Model Context Protocol), **A2A**, and **ATXP**.
- **📜 Policy-as-Code**: Human-readable YAML policies to allow, block, or escalate sensitive actions.
- **🙋 Human-in-the-Loop**: Seamlessly pause risky tool calls for manual approval via Dashboard or Slack.
- **💰 Spend Control**: Set granular daily USD limits and token-bucket rate limits per agent.
- **🔒 Zero Trust**: HMAC-SHA256 request signing ensures agents can't spoof their identity.

---

## 🚀 Quickstart

### 1. Launch with Docker

```bash
docker run -p 7777:7777 \
  -e AGENTGUARD_UPSTREAM_URL=http://your-api:8080 \
  -v $(pwd)/policy.yaml:/app/policy.yaml \
  bhuvanprakash/agentguard:latest
```

### 2. Define Your Policy

```yaml
version: "1"
default: block

agents:
  - id: billing-agent
    allow:
      - tool: read_invoice
    escalate:
      - tool: send_payment   # Requires human approval
    block:
      - tool: delete_record
    spend_limit_daily_usd: 50.00
```

---

## 🧠 How it Works

AgentGuard intercepts JSON-RPC and REST calls from your agents. It evaluates the "Tool/Call" intent against your rules:

1.  **Identity Verification**: Validates the `X-Agent-HMAC` signature.
2.  **Safety Check**: Intercepts "Irreversible Tools" (like `drop_table`).
3.  **Policy Evaluation**: Matches the agent ID and tool name.
4.  **Enforcement**: 
    - `ALLOW`: Forwards directly to the upstream.
    - `BLOCK`: Returns a `403 Forbidden`.
    - `ESCALATE`: Returns `202 Accepted` and waits for approval.

---

## 🛠️ SDK Support

| Language | Package | Install |
| :--- | :--- | :--- |
| **Node.js** | `agentguard` | `npm install agentguard` |
| **Python** | `agentguard` | `pip install agentguard` |

---

## 📊 Dashboard & Monitoring

While the proxy is fully standalone, you can connect it to the **[Nascentist Dashboard](https://nascentist.com)** for:
- 📈 Real-time spend analytics
- 📱 Mobile approval notifications
- 🛠️ Drag-and-drop policy editor
- 🔐 Managed agent secret storage

---

## 📜 License

Distributed under the **Apache License 2.0**. See `LICENSE` for more information.

---

<div align="center">
  <p>Built by <b>Bhuvan Prakash</b> for <a href="https://nascentist.com">nascentist.com</a></p>
  <p>
    <a href="https://nascentist.ai/docs/agentguard">Documentation</a> •
    <a href="https://github.com/bhuvanprakash/agentguard/issues">Report Bug</a> •
    <a href="https://github.com/bhuvanprakash/agentguard/pulls">Request Feature</a>
  </p>
</div>
