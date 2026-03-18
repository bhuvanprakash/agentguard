-- Local SQLite schema for AgentGuard audit log.
-- This runs once at startup to create the table if it doesn't exist.
-- All agent decisions are written here locally.
-- A background goroutine also ships rows to Supabase.

CREATE TABLE IF NOT EXISTS guard_logs (
    id           TEXT PRIMARY KEY,  -- UUID
    ts           TEXT NOT NULL,     -- RFC3339 timestamp
    agent_id     TEXT NOT NULL,     -- who called
    tool_name    TEXT NOT NULL,     -- what they called
    protocol     TEXT NOT NULL,     -- mcp | a2a | atxp | unknown
    decision     TEXT NOT NULL,     -- allow | block | escalate
    policy_rule  TEXT,              -- which rule matched (or "default"/"irreversible")
    latency_ms   INTEGER,           -- how long the policy check took
    upstream_ms  INTEGER,           -- how long the upstream took (if forwarded)
    upstream_status INTEGER,        -- HTTP status from upstream (if forwarded)
    arguments    TEXT,              -- JSON of tool arguments (truncated to 500 chars)
    synced       INTEGER DEFAULT 0  -- 0 = not yet shipped to Supabase
);

CREATE INDEX IF NOT EXISTS idx_guard_logs_agent_id ON guard_logs(agent_id);
CREATE INDEX IF NOT EXISTS idx_guard_logs_ts ON guard_logs(ts);
CREATE INDEX IF NOT EXISTS idx_guard_logs_decision ON guard_logs(decision);
CREATE INDEX IF NOT EXISTS idx_guard_logs_synced ON guard_logs(synced);
