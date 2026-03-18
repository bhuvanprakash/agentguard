# agentguard/proxy.py
# Core AgentGuard proxy wrapper.
# guard() is a one-line drop-in replacement.
# Instead of pointing your MCP/HTTP client at your real API,
# you point it at AgentGuard which enforces your policy first.
# How it works:
#   guard("http://real-api:8080")
#   → returns "http://localhost:7777"
#   → sets X-AgentGuard-Upstream header so AgentGuard knows
#     where to forward allowed requests
# The real API URL is passed as a header, not in the URL.
# This means your code doesn't change at all — just swap the URL.

import os
import urllib.parse
import hashlib
import hmac as hmac_lib
import time
from typing import Optional


# Default AgentGuard proxy URL
_DEFAULT_PROXY = os.environ.get("AGENTGUARD_URL", "http://localhost:7777")


def guard(
    upstream_url: str,
    proxy_url:    Optional[str]  = None,
    agent_id:     Optional[str]  = None,
) -> str:
    """
    Wrap an upstream URL with AgentGuard enforcement.

    Returns the AgentGuard proxy URL. Pass this to any
    MCP or HTTP client instead of the real upstream URL.

    Args:
        upstream_url: The real API URL you want to call.
        proxy_url:    AgentGuard URL (default: AGENTGUARD_URL env
                      or http://localhost:7777)
        agent_id:     Optional agent identifier for policy matching.

    Returns:
        str: The AgentGuard proxy URL to use instead.

    Example:
        from agentguard import guard
        from mcp import MCPClient

        # Before:
        client = MCPClient("http://my-mcp-server:8080")

        # After (one word change):
        client = MCPClient(guard("http://my-mcp-server:8080"))
    """
    base = (proxy_url or _DEFAULT_PROXY).rstrip("/")

    # Encode upstream as query param so the proxy knows
    # where to forward (alternative: use X-Upstream header)
    encoded = urllib.parse.quote(upstream_url, safe="")
    url     = f"{base}?upstream={encoded}"

    if agent_id:
        url += f"&agent_id={urllib.parse.quote(agent_id)}"

    return url


class AgentGuard:
    """
    AgentGuard client with configuration.
    Use this when you need more control than guard() provides.

    Example:
        ag = AgentGuard(
            proxy_url="http://agentguard:7777",
            agent_id="billing-agent",
            admin_key="secret",
        )

        # Wrap a URL
        client = MCPClient(ag.wrap("http://my-api:8080"))

        # List pending escalations
        escalations = ag.list_escalations()

        # Approve an escalation
        ag.approve("escalation-id")
    """

    def __init__(
        self,
        proxy_url:       Optional[str] = None,
        agent_id:        Optional[str] = None,
        signing_secret:  Optional[str] = None,
        admin_key:       Optional[str] = None,
    ):
        self.proxy_url = (proxy_url or _DEFAULT_PROXY).rstrip("/")
        self.agent_id  = agent_id
        self.signing_secret = signing_secret or os.environ.get("AGENTGUARD_SIGNING_SECRET", "")
        self.admin_key = admin_key or os.environ.get("AGENTGUARD_ADMIN_KEY", "")

    def _sign_request(
        self,
        body:      bytes,
        timestamp: str,
    ) -> str:
        """
        Build HMAC-SHA256 signature over canonical string.
        Returns "sha256=<hex>"
        """
        body_hash = hashlib.sha256(body).hexdigest()
        canonical = f"{self.agent_id}\n{timestamp}\n{body_hash}"
        sig = hmac_lib.new(
            self.signing_secret.encode(),
            canonical.encode(),
            hashlib.sha256,
        ).hexdigest()
        return f"sha256={sig}"

    def signed_headers(self, body: bytes) -> dict:
        """
        Returns the auth headers to add to every request.
        If no signing_secret configured, returns empty dict.
        (backward compatible — dev mode still works)
        """
        if not self.signing_secret or not self.agent_id:
            return {}

        ts = str(int(time.time()))
        sig = self._sign_request(body, ts)
        return {
            "X-Agent-ID":              self.agent_id,
            "X-AgentGuard-Timestamp":  ts,
            "X-AgentGuard-Signature":  sig,
        }

    def wrap_request(
        self,
        body:    bytes,
        headers: dict = None,
    ) -> dict:
        """
        Returns a complete headers dict ready to attach to
        any HTTP request to the AgentGuard proxy.
        """
        base = {
            "Content-Type": "application/json",
            "X-Agent-ID":   self.agent_id,
        }
        if headers:
            base.update(headers)
        base.update(self.signed_headers(body))
        return base

    def wrap(self, upstream_url: str) -> str:
        """Wrap an upstream URL. Same as guard() but uses instance config."""
        return guard(upstream_url, self.proxy_url, self.agent_id)

    def list_escalations(self) -> list:
        """Fetch pending escalations from AgentGuard."""
        import urllib.request
        import json

        req = urllib.request.Request(
            f"{self.proxy_url}/escalations",
            headers={"X-AgentGuard-Admin-Key": self.admin_key},
        )
        try:
            with urllib.request.urlopen(req, timeout=5) as resp:
                data = json.loads(resp.read())
                return data.get("escalations", [])
        except Exception:
            return []

    def approve(self, escalation_id: str) -> bool:
        """Approve a pending escalation."""
        return self._resolve(escalation_id, "approve")

    def reject(self, escalation_id: str) -> bool:
        """Reject a pending escalation."""
        return self._resolve(escalation_id, "reject")

    def health(self) -> dict:
        """Check AgentGuard health."""
        import urllib.request
        import json
        try:
            with urllib.request.urlopen(
                f"{self.proxy_url}/health", timeout=3
            ) as resp:
                return json.loads(resp.read())
        except Exception as e:
            return {"status": "unreachable", "error": str(e)}

    def _resolve(self, escalation_id: str, action: str) -> bool:
        import urllib.request
        req = urllib.request.Request(
            f"{self.proxy_url}/escalations/{escalation_id}/{action}",
            data=b"",
            method="POST",
            headers={"X-AgentGuard-Admin-Key": self.admin_key},
        )
        try:
            with urllib.request.urlopen(req, timeout=5) as resp:
                return resp.status == 200
        except Exception:
            return False
