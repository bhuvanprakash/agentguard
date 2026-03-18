-- AgentGuard tables — added to existing Nascentist Supabase

-- ── guard_logs ────────────────────────────────────────────────
-- Cloud copy of the local SQLite audit log.
-- Synced by the background worker in audit/logger.go.
-- Used by the Nascentist dashboard to show agent activity.

CREATE TABLE IF NOT EXISTS public.guard_logs (
    id               TEXT PRIMARY KEY,
    ts               TIMESTAMPTZ NOT NULL,
    agent_id         TEXT NOT NULL,
    tool_name        TEXT NOT NULL,
    protocol         TEXT NOT NULL CHECK (protocol IN ('mcp','a2a','atxp','unknown')),
    decision         TEXT NOT NULL CHECK (decision IN ('allow','block','escalate')),
    policy_rule      TEXT,
    latency_ms       INTEGER,
    upstream_ms      INTEGER,
    upstream_status  INTEGER,
    arguments        TEXT,              -- JSON string, truncated to 500 chars
    created_at       TIMESTAMPTZ DEFAULT now()
);

CREATE INDEX IF NOT EXISTS guard_logs_agent_id
    ON public.guard_logs(agent_id);

CREATE INDEX IF NOT EXISTS guard_logs_decision
    ON public.guard_logs(decision, created_at DESC);

CREATE INDEX IF NOT EXISTS guard_logs_ts
    ON public.guard_logs(ts DESC);

-- ── guard_policies ────────────────────────────────────────────
-- Stores policy files per user (for hosted cloud tier).
-- The Go service pulls the active policy from here.

CREATE TABLE IF NOT EXISTS public.guard_policies (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    UUID NOT NULL REFERENCES public.profiles(id) ON DELETE CASCADE,
    name       TEXT NOT NULL,
    yaml_content TEXT NOT NULL,
    is_active  BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMPTZ DEFAULT now(),
    updated_at TIMESTAMPTZ DEFAULT now()
);

CREATE INDEX IF NOT EXISTS guard_policies_user_id
    ON public.guard_policies(user_id);

-- ── RLS ───────────────────────────────────────────────────────
ALTER TABLE public.guard_logs     ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.guard_policies ENABLE ROW LEVEL SECURITY;

-- Users see only their own logs (by agent_id prefix user_id)
-- For now, service role only (direct inserts from Go service)
CREATE POLICY "service_insert_guard_logs"
    ON public.guard_logs FOR INSERT
    WITH CHECK (true);

CREATE POLICY "service_select_guard_logs"
    ON public.guard_logs FOR SELECT
    USING (true);   -- dashboard queries use service key

CREATE POLICY "guard_policies_own"
    ON public.guard_policies FOR ALL
    USING (auth.uid() = user_id)
    WITH CHECK (auth.uid() = user_id);
