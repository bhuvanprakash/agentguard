# AgentGuard Launch Copy

Use these posts on launch day. Edit before posting.

***

## Reddit — r/LocalLLaMA

**Title:**
AgentGuard — I built an open-source proxy that blocks dangerous
AI agent tool calls before they execute (MCP + A2A + ATXP)

**Body:**
Hey r/LocalLLaMA,

I've been building local AI agents using MCP servers and kept
running into the same problem: the agent would try to do something
destructive (delete files, send API calls I didn't expect) and
there was no easy way to stop it without rewriting my agent code.

So I built AgentGuard. It's a Go proxy that sits between your
agent and your MCP server. Every tool call passes through it and
is checked against a YAML policy before it executes.

**How it works:**

```yaml
# policy.yaml
version: "1"
default: block

agents:
  - id: my-agent
    allow:
      - tool: read_file
    block:
      - tool: delete_file
    escalate:
      - tool: send_payment  # pauses for your approval
```

```python
from agentguard import guard
from mcp import MCPClient

# One line change
client = MCPClient(guard("http://my-mcp-server:8080"))
```

**Features:**
- Works with MCP, A2A, and ATXP (all three agent protocols)
- Policy-as-YAML — readable, version-controlled
- Escalation flow: dangerous actions pause and wait for
  human approval before executing
- Daily spend limits per agent (for ATXP payment calls)
- Go binary, <1ms policy overhead, handles 10k concurrent agents
- MIT license, fully self-hostable

**GitHub:** https://github.com/nascentist/agentguard

Would love feedback especially from people running production
MCP setups. What tool patterns are you seeing that need blocking?

***

## Hacker News

**Title:**
Show HN: AgentGuard – open-source AI agent enforcement proxy
(MCP/A2A/ATXP)

**Body:**
I built an enforcement proxy for AI agents that intercepts tool
calls before they execute.

The problem: AI agents using MCP or A2A protocols can call any
tool they're given — including destructive ones. There's no
standard interception layer between the agent and the tool.

AgentGuard is a Go proxy that solves this. It sits in front of
your MCP server and checks every tools/call request against a
YAML policy. Decisions are allow, block, or escalate (human
approval required).

One-line integration:
```python
client = MCPClient(guard("http://my-mcp-server:8080"))
```

Built in Go for performance (<1ms overhead), single binary,
MIT license. Python and Node.js SDKs included.

https://github.com/nascentist/agentguard

Happy to answer questions about the protocol parsing or
the escalation flow design.

***

## A2A GitHub Discussions

**Title:**
AgentGuard: an enforcement proxy for A2A agents — looking for
protocol feedback

**Body:**
Hi A2A community,

I've been working on an enforcement proxy called AgentGuard that
intercepts A2A messages/send and tasks/create requests before
they reach the real service.

It works by sitting in front of the A2A server and checking
each request against a policy:

- allow → forward to real service
- block → return error response
- escalate → pause, notify human, wait for approval

I'm particularly interested in feedback from this community on:

1. Are there A2A message types I'm not parsing correctly?
2. What's the right response format when blocking a task?
   (Currently returning 403 with JSON error body)
3. Is there an existing standard for agent policy enforcement
   that I should align with?

The full source is MIT at:
https://github.com/nascentist/agentguard

The A2A parsing is in:
https://github.com/nascentist/agentguard/blob/main/protocol/a2a.go

Would love a review from people who know the spec better than I do.

***

## Product Hunt Tagline options (pick one):

1. "Stop your AI agents from deleting your database"
2. "A policy layer for AI agents — allow, block, or escalate any tool call"
3. "MCP + A2A + ATXP enforcement proxy — built in Go, MIT license"

## Product Hunt Description:

AgentGuard is an open-source enforcement proxy for AI agents.

When your agent calls delete_database() or send_payment(amount=50000),
AgentGuard intercepts the call before it reaches the real API and
checks it against your YAML policy.

Three decisions:
→ allow: forward to real API
→ block: reject immediately
→ escalate: pause, send you an alert, wait for your approval

Works with MCP (Model Context Protocol), A2A (Agent-to-Agent),
and ATXP (Agent Transaction Protocol).

One-line integration:
```
client = MCPClient(guard("http://my-api:8080"))
```

Fully open source (MIT). Self-host with a single Go binary.
Managed cloud at nascentist.ai.
