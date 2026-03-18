# agentguard/__init__.py
# AgentGuard Python SDK
# Usage:
#   from agentguard import guard, verify_webhook
# Wrap any MCP/HTTP client URL
#   client = MCPClient(guard("http://my-api:8080"))
# Or use the longer form:
#   from agentguard import AgentGuard
#   ag = AgentGuard(proxy_url="http://localhost:7777")
#   client = MCPClient(ag.wrap("http://my-api:8080"))

from .proxy  import guard, AgentGuard
from .verify import verify_webhook
from .policy import load_policy, validate_policy

__version__ = "0.1.0"
__all__     = [
    "guard",
    "AgentGuard",
    "verify_webhook",
    "load_policy",
    "validate_policy",
]
