package escalation

// Store manages pending escalations in SQLite (local fast)
// and syncs them to Supabase (cloud, for dashboard).
//
// An escalation is created when policy.Evaluate() returns
// DecisionEscalate. It sits in "pending" until:
//   a) A human approves it  → original request is forwarded
//   b) A human rejects it   → block response is returned
//   c) 24h passes           → auto-expires
//
// The calling agent is BLOCKED waiting for approval.
// This means the agent's HTTP connection stays open until
// the human acts (or the request times out).
//
// To avoid holding HTTP connections open for hours,
// AgentGuard returns 202 Accepted immediately and expects
// the agent framework to poll for result.
// (Most MCP/A2A clients retry on 202 automatically.)

import (
    "bytes"
    "database/sql"
    "encoding/json"
    "fmt"
    "net/http"
    "sync"
    "time"

    "github.com/nascentist/agentguard/notify"
    _ "github.com/mattn/go-sqlite3"
)

// Status values for an escalation
const (
    StatusPending  = "pending"
    StatusApproved = "approved"
    StatusRejected = "rejected"
    StatusExpired  = "expired"
)

type Escalation struct {
    ID          string
    Ts          time.Time
    AgentID     string
    ToolName    string
    Protocol    string
    Arguments   map[string]interface{}
    Status      string
    WebhookURL  string
    GuardLogID  string
    ExpiresAt   time.Time
}

type Store struct {
    mu          sync.RWMutex
    db          *sql.DB
    supabaseURL string
    supabaseKey string
    // In-memory pending map for fast lookup by ID
    // (avoids SQLite read on every poll)
    pending     map[string]*Escalation
    notifier    *notify.Notifier
}

func NewStore(db *sql.DB, supabaseURL, supabaseKey string, notifier *notify.Notifier) (*Store, error) {
    s := &Store{
        db:          db,
        supabaseURL: supabaseURL,
        supabaseKey: supabaseKey,
        pending:     make(map[string]*Escalation),
        notifier:    notifier,
    }

    // Create SQLite escalation table
    _, err := db.Exec(`
        CREATE TABLE IF NOT EXISTS escalations (
            id           TEXT PRIMARY KEY,
            ts           TEXT NOT NULL,
            agent_id     TEXT NOT NULL,
            tool_name    TEXT NOT NULL,
            protocol     TEXT NOT NULL,
            arguments    TEXT,
            status       TEXT NOT NULL DEFAULT 'pending',
            webhook_url  TEXT,
            guard_log_id TEXT,
            expires_at   TEXT NOT NULL,
            resolved_at  TEXT,
            resolved_by  TEXT
        );
        CREATE INDEX IF NOT EXISTS esc_status
            ON escalations(status);
    `)
    if err != nil {
        return nil, fmt.Errorf("escalation: cannot create table: %w", err)
    }

    // Reload pending escalations from SQLite on startup
    s.reloadFromDB()

    // Start expiry ticker
    go s.expireLoop()

    return s, nil
}

// Create inserts a new pending escalation and returns its ID.
func (s *Store) Create(e *Escalation) error {
    e.Ts        = time.Now().UTC()
    e.Status    = StatusPending
    e.ExpiresAt = e.Ts.Add(24 * time.Hour)

    argsJSON, _ := json.Marshal(e.Arguments)

    _, err := s.db.Exec(`
        INSERT INTO escalations
          (id, ts, agent_id, tool_name, protocol, arguments,
           status, webhook_url, guard_log_id, expires_at)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
        e.ID, e.Ts.Format(time.RFC3339Nano),
        e.AgentID, e.ToolName, e.Protocol,
        string(argsJSON), StatusPending,
        e.WebhookURL, e.GuardLogID,
        e.ExpiresAt.Format(time.RFC3339Nano),
    )
    if err != nil {
        return err
    }

    s.mu.Lock()
    s.pending[e.ID] = e
    s.mu.Unlock()

    // Async sync to Supabase
    go s.syncOneToSupabase(e)

    // Notify Slack/Discord
    if s.notifier != nil {
        s.notifier.SendEscalation(notify.EscalationEvent{
            ID:        e.ID,
            AgentID:   e.AgentID,
            ToolName:  e.ToolName,
            Protocol:  e.Protocol,
            Arguments: e.Arguments,
            Ts:        e.Ts,
            ExpiresAt: e.ExpiresAt,
        })
    }

    return nil
}

// Resolve marks an escalation as approved or rejected.
// Returns the escalation so the caller can act on it.
func (s *Store) Resolve(id, status, resolvedBy string) (*Escalation, error) {
    if status != StatusApproved && status != StatusRejected {
        return nil, fmt.Errorf("invalid status: %s", status)
    }

    s.mu.Lock()
    e, exists := s.pending[id]
    if !exists {
        s.mu.Unlock()
        // Try loading from DB
        e = s.loadFromDB(id)
        if e == nil {
            return nil, fmt.Errorf("escalation not found: %s", id)
        }
        s.mu.Lock()
    }

    if e.Status != StatusPending {
        s.mu.Unlock()
        return nil, fmt.Errorf("escalation already %s", e.Status)
    }

    e.Status = status
    delete(s.pending, id)
    s.mu.Unlock()

    now := time.Now().UTC().Format(time.RFC3339Nano)
    _, err := s.db.Exec(`
        UPDATE escalations
        SET status = ?, resolved_at = ?
        WHERE id = ?`,
        status, now, id,
    )
    if err != nil {
        return e, err
    }

    // Sync resolution to Supabase
    go s.updateSupabaseStatus(id, status, resolvedBy, now)

    return e, nil
}

// Get returns an escalation by ID (checks memory first, then DB).
func (s *Store) Get(id string) *Escalation {
    s.mu.RLock()
    e := s.pending[id]
    s.mu.RUnlock()
    if e != nil {
        return e
    }
    return s.loadFromDB(id)
}

// ListPending returns all pending escalations.
func (s *Store) ListPending() []*Escalation {
    s.mu.RLock()
    defer s.mu.RUnlock()
    list := make([]*Escalation, 0, len(s.pending))
    for _, e := range s.pending {
        list = append(list, e)
    }
    return list
}

// ── Internal helpers ──────────────────────────────────────────

func (s *Store) expireLoop() {
    ticker := time.NewTicker(5 * time.Minute)
    defer ticker.Stop()
    for range ticker.C {
        now := time.Now().UTC()
        s.mu.Lock()
        for id, e := range s.pending {
            if e.ExpiresAt.Before(now) {
                e.Status = StatusExpired
                delete(s.pending, id)
                go s.db.Exec(
                    `UPDATE escalations SET status = 'expired' WHERE id = ?`, id)
                go s.updateSupabaseStatus(id, StatusExpired, "", "")
            }
        }
        s.mu.Unlock()
    }
}

func (s *Store) reloadFromDB() {
    rows, err := s.db.Query(`
        SELECT id, ts, agent_id, tool_name, protocol,
               arguments, webhook_url, guard_log_id, expires_at
        FROM escalations WHERE status = 'pending'
    `)
    if err != nil {
        return
    }
    defer rows.Close()

    s.mu.Lock()
    defer s.mu.Unlock()

    for rows.Next() {
        var e Escalation
        var tsStr, expiresStr, argsStr string
        rows.Scan(
            &e.ID, &tsStr, &e.AgentID, &e.ToolName, &e.Protocol,
            &argsStr, &e.WebhookURL, &e.GuardLogID, &expiresStr,
        )
        e.Ts, _        = time.Parse(time.RFC3339Nano, tsStr)
        e.ExpiresAt, _ = time.Parse(time.RFC3339Nano, expiresStr)
        json.Unmarshal([]byte(argsStr), &e.Arguments)
        e.Status = StatusPending
        s.pending[e.ID] = &e
    }
}

func (s *Store) loadFromDB(id string) *Escalation {
    var e Escalation
    var tsStr, expiresStr, argsStr string
    err := s.db.QueryRow(`
        SELECT id, ts, agent_id, tool_name, protocol,
               arguments, status, webhook_url, guard_log_id, expires_at
        FROM escalations WHERE id = ?`, id,
    ).Scan(
        &e.ID, &tsStr, &e.AgentID, &e.ToolName, &e.Protocol,
        &argsStr, &e.Status, &e.WebhookURL, &e.GuardLogID, &expiresStr,
    )
    if err != nil {
        return nil
    }
    e.Ts, _        = time.Parse(time.RFC3339Nano, tsStr)
    e.ExpiresAt, _ = time.Parse(time.RFC3339Nano, expiresStr)
    json.Unmarshal([]byte(argsStr), &e.Arguments)
    return &e
}

func (s *Store) syncOneToSupabase(e *Escalation) {
    if s.supabaseURL == "" {
        return
    }
    argsJSON, _ := json.Marshal(e.Arguments)
    body, _ := json.Marshal(map[string]interface{}{
        "id":          e.ID,
        "ts":          e.Ts.Format(time.RFC3339),
        "agent_id":    e.AgentID,
        "tool_name":   e.ToolName,
        "protocol":    e.Protocol,
        "arguments":   string(argsJSON),
        "status":      StatusPending,
        "webhook_url": e.WebhookURL,
        "guard_log_id": e.GuardLogID,
        "expires_at":  e.ExpiresAt.Format(time.RFC3339),
    })

    req, _ := http.NewRequest("POST",
        s.supabaseURL+"/rest/v1/guard_escalations",
        bytes.NewReader(body),
    )
    req.Header.Set("apikey", s.supabaseKey)
    req.Header.Set("Authorization", "Bearer "+s.supabaseKey)
    req.Header.Set("Content-Type", "application/json")
    req.Header.Set("Prefer", "resolution=merge-duplicates")

    client := &http.Client{Timeout: 5 * time.Second}
    resp, err := client.Do(req)
    if err == nil {
        resp.Body.Close()
    }
}

func (s *Store) updateSupabaseStatus(id, status, resolvedBy, resolvedAt string) {
    if s.supabaseURL == "" {
        return
    }
    update := map[string]interface{}{"status": status}
    if resolvedAt != "" {
        update["resolved_at"] = resolvedAt
    }
    if resolvedBy != "" {
        update["resolved_by"] = resolvedBy
    }
    body, _ := json.Marshal(update)
    req, _ := http.NewRequest("PATCH",
        s.supabaseURL+"/rest/v1/guard_escalations?id=eq."+id,
        bytes.NewReader(body),
    )
    req.Header.Set("apikey", s.supabaseKey)
    req.Header.Set("Authorization", "Bearer "+s.supabaseKey)
    req.Header.Set("Content-Type", "application/json")

    client := &http.Client{Timeout: 5 * time.Second}
    resp, err := client.Do(req)
    if err == nil {
        resp.Body.Close()
    }
}
