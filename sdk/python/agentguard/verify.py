# agentguard/verify.py
# Webhook signature verification.
# When AgentGuard delivers a webhook to your URL,
# it includes an X-AgentGuard-Signature: sha256=<hex> header.
# Use verify_webhook() to confirm the payload is authentic.
# Usage (FastAPI example):
#   from agentguard import verify_webhook
# @app.post("/webhooks/agentguard")
#   async def handle_escalation(
#       request: Request,
#       x_agentguard_signature: str = Header(None)
#   ):
#       body = await request.body()
#       if not verify_webhook(body, x_agentguard_signature, SECRET):
#           raise HTTPException(403, "Invalid signature")
#       payload = await request.json()
# handle payload...

import hashlib
import hmac


def verify_webhook(
    body:      bytes,
    signature: str,
    secret:    str,
) -> bool:
    """
    Verify an AgentGuard webhook signature.

    Args:
        body:      Raw request body bytes (before JSON parsing)
        signature: Value of X-AgentGuard-Signature header
        secret:    Your AGENTGUARD_WEBHOOK_SECRET

    Returns:
        bool: True if signature is valid, False otherwise.

    Security:
        Uses timing-safe comparison to prevent timing attacks.
    """
    if not signature or not secret:
        return False

    # Strip "sha256=" prefix
    if signature.startswith("sha256="):
        signature = signature[7:]

    expected = hmac.new(
        secret.encode("utf-8"),
        body,
        hashlib.sha256,
    ).hexdigest()

    # Timing-safe comparison
    return hmac.compare_digest(expected, signature)
