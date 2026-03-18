-- ══════════════════════════════════════════════════════════
-- COMPLETE RLS SETUP FOR ALL AGENTGUARD TABLES
-- Run this after all previous migrations.
-- Safe to run multiple times (uses IF NOT EXISTS / OR REPLACE).
-- ══════════════════════════════════════════════════════════

-- ── 1. guard_logs ─────────────────────────────────────────

ALTER TABLE public.guard_logs
  ENABLE ROW LEVEL SECURITY;

-- Drop existing policies first (safe re-run)
DROP POLICY IF EXISTS "service_all_logs"   ON public.guard_logs;
DROP POLICY IF EXISTS "owner_read_logs"    ON public.guard_logs;

-- Service role can do everything (Go service uses this key)
CREATE POLICY "service_all_logs"
  ON public.guard_logs
  FOR ALL
  TO service_role
  USING (true)
  WITH CHECK (true);

-- Authenticated dashboard users can only READ their own logs
-- (guard_logs does not have user_id yet — we add it below)
-- For now: anon cannot read, service role can do all.

-- ── 2. guard_policies ─────────────────────────────────────

ALTER TABLE public.guard_policies
  ENABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS "service_all_policies"   ON public.guard_policies;
DROP POLICY IF EXISTS "owner_select_policies"  ON public.guard_policies;
DROP POLICY IF EXISTS "owner_insert_policies"  ON public.guard_policies;
DROP POLICY IF EXISTS "owner_update_policies"  ON public.guard_policies;
DROP POLICY IF EXISTS "owner_delete_policies"  ON public.guard_policies;

-- Service role: full access
CREATE POLICY "service_all_policies"
  ON public.guard_policies
  FOR ALL
  TO service_role
  USING (true)
  WITH CHECK (true);

-- Authenticated user: own policies only
CREATE POLICY "owner_select_policies"
  ON public.guard_policies
  FOR SELECT
  TO authenticated
  USING (user_id = auth.uid());

CREATE POLICY "owner_insert_policies"
  ON public.guard_policies
  FOR INSERT
  TO authenticated
  WITH CHECK (user_id = auth.uid());

CREATE POLICY "owner_update_policies"
  ON public.guard_policies
  FOR UPDATE
  TO authenticated
  USING (user_id = auth.uid())
  WITH CHECK (user_id = auth.uid());

CREATE POLICY "owner_delete_policies"
  ON public.guard_policies
  FOR DELETE
  TO authenticated
  USING (user_id = auth.uid());

-- ── 3. guard_escalations ──────────────────────────────────

ALTER TABLE public.guard_escalations
  ENABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS "service_all_escalations"  ON public.guard_escalations;
DROP POLICY IF EXISTS "owner_read_escalations"   ON public.guard_escalations;

-- Service role: full access (Go service uses this)
CREATE POLICY "service_all_escalations"
  ON public.guard_escalations
  FOR ALL
  TO service_role
  USING (true)
  WITH CHECK (true);

-- ── 4. Add user_id to guard_logs (if not already present) ─

ALTER TABLE public.guard_logs
  ADD COLUMN IF NOT EXISTS user_id UUID
  REFERENCES public.profiles(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS guard_logs_user_id
  ON public.guard_logs(user_id);

-- Now authenticated users can read their own logs
DROP POLICY IF EXISTS "owner_read_logs" ON public.guard_logs;
CREATE POLICY "owner_read_logs"
  ON public.guard_logs
  FOR SELECT
  TO authenticated
  USING (
    user_id = auth.uid()
    OR
    -- Also allow service role read (already covered above)
    -- This allows dashboard to read logs belonging to user
    user_id IS NULL  -- backward compat: old logs without user_id
  );

-- ── 5. Trigger: auto-expire stale escalations ─────────────

CREATE OR REPLACE FUNCTION public.expire_stale_escalations()
RETURNS void
LANGUAGE plpgsql
SECURITY DEFINER
AS $$
BEGIN
  UPDATE public.guard_escalations
  SET status = 'expired'
  WHERE status = 'pending'
    AND expires_at < now();
END;
$$;

-- ── 6. Trigger: update guard_policies.updated_at ──────────

CREATE OR REPLACE FUNCTION public.set_guard_policy_updated_at()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
  NEW.updated_at = now();
  RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS guard_policy_updated_at_trigger
  ON public.guard_policies;

CREATE TRIGGER guard_policy_updated_at_trigger
  BEFORE UPDATE ON public.guard_policies
  FOR EACH ROW
  EXECUTE FUNCTION public.set_guard_policy_updated_at();

-- ── 7. Realtime: enable for guard tables ──────────────────
-- (allows Next.js to use supabase.channel() for live updates)

ALTER PUBLICATION supabase_realtime
  ADD TABLE public.guard_escalations;

ALTER PUBLICATION supabase_realtime
  ADD TABLE public.guard_logs;

-- ── 8. Useful views ───────────────────────────────────────

CREATE OR REPLACE VIEW public.guard_stats_today AS
SELECT
  date_trunc('day', ts) AS day,
  COUNT(*)                                              AS total,
  COUNT(*) FILTER (WHERE decision = 'allow')            AS allowed,
  COUNT(*) FILTER (WHERE decision = 'block')            AS blocked,
  COUNT(*) FILTER (WHERE decision = 'escalate')         AS escalated,
  COUNT(DISTINCT agent_id)                              AS unique_agents,
  AVG(latency_ms)                                       AS avg_latency_ms,
  MAX(latency_ms)                                       AS max_latency_ms
FROM public.guard_logs
WHERE ts >= date_trunc('day', now())
GROUP BY 1;

CREATE OR REPLACE VIEW public.guard_agent_summary AS
SELECT
  agent_id,
  COUNT(*)                                          AS total_calls,
  COUNT(*) FILTER (WHERE decision = 'block')        AS blocked_calls,
  COUNT(*) FILTER (WHERE decision = 'escalate')     AS escalated_calls,
  MAX(ts)                                           AS last_seen,
  COUNT(DISTINCT tool_name)                         AS unique_tools
FROM public.guard_logs
GROUP BY agent_id
ORDER BY total_calls DESC;
