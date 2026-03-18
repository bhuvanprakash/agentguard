# tests/test_proxy.py

import pytest
from agentguard.proxy  import guard, AgentGuard
from agentguard.verify import verify_webhook
from agentguard.policy import validate_policy, simulate_decision


class TestGuard:
    def test_guard_returns_proxy_url(self):
        url = guard("http://my-api:8080",
                    proxy_url="http://agentguard:7777")
        assert "agentguard:7777" in url
        assert "my-api" in url   # upstream encoded in URL

    def test_guard_uses_env_default(self, monkeypatch):
        monkeypatch.setenv("AGENTGUARD_URL", "http://custom:9999")
        import importlib, agentguard.proxy as m
        importlib.reload(m)
        url = m.guard("http://upstream:8080")
        assert "9999" in url

    def test_guard_with_agent_id(self):
        url = guard("http://api:8080",
                    proxy_url="http://guard:7777",
                    agent_id="billing-agent")
        assert "billing-agent" in url


class TestWebhookVerify:
    def test_valid_signature(self):
        import hashlib, hmac
        secret  = "test-secret"
        body    = b'{"event":"agentguard.escalation.created"}'
        sig     = "sha256=" + hmac.new(
            secret.encode(), body, hashlib.sha256
        ).hexdigest()
        assert verify_webhook(body, sig, secret) is True

    def test_invalid_signature(self):
        assert verify_webhook(
            b"body", "sha256=wrong", "secret"
        ) is False

    def test_empty_signature(self):
        assert verify_webhook(b"body", "", "secret") is False

    def test_empty_secret(self):
        assert verify_webhook(b"body", "sha256=abc", "") is False


class TestPolicy:
    def test_valid_policy(self):
        policy = {
            "version": "1",
            "default": "block",
            "agents": [
                {
                    "id": "test-agent",
                    "allow": [{"tool": "read_file"}],
                    "block": [{"tool": "delete_file"}],
                }
            ]
        }
        errors = validate_policy(policy)
        assert errors == []

    def test_missing_version(self):
        errors = validate_policy({"agents": []})
        assert any("version" in e for e in errors)

    def test_invalid_default(self):
        errors = validate_policy({
            "version": "1",
            "default": "INVALID",
            "agents": []
        })
        assert any("default" in e for e in errors)

    def test_duplicate_agent_ids(self):
        errors = validate_policy({
            "version": "1",
            "agents": [
                {"id": "same", "allow": []},
                {"id": "same", "allow": []},
            ]
        })
        assert any("duplicate" in e for e in errors)


class TestSimulate:
    POLICY = {
        "version": "1",
        "default": "block",
        "agents": [
            {
                "id": "test",
                "allow":    [{"tool": "read_file"}],
                "block":    [{"tool": "bad_tool"}],
                "escalate": [{"tool": "risky_tool"}],
            }
        ]
    }

    def test_allow(self):
        assert simulate_decision(
            self.POLICY, "test", "read_file") == "allow"

    def test_block(self):
        assert simulate_decision(
            self.POLICY, "test", "bad_tool") == "block"

    def test_escalate(self):
        assert simulate_decision(
            self.POLICY, "test", "risky_tool") == "escalate"

    def test_irreversible_always_escalates(self):
        assert simulate_decision(
            self.POLICY, "test", "delete_file") == "escalate"

    def test_default_block(self):
        assert simulate_decision(
            self.POLICY, "test", "unknown_tool") == "block"
