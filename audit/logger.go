package audit

// Logger writes AgentGuard decisions to SQLite (fast, local)
// and ships them to Supabase in the background (cloud, dashboard).
//
// Design:
//   - SQLite writes are synchronous (must complete before response)
//   - Supabase writes are async (goroutine, non-blocking)
//   - If Supabase is down, logs stay in SQLite with synced=0
//   - A background worker retries unsynced rows every 60 seconds
//
// Every intercepted request — allowed, blocked, or escalated —
// gets a log entry. This is the immutable audit trail.

import (
    "bytes"
    "context"
    "crypto/rand"
    "database/sql"
    "encoding/hex"
    "encoding/json"
    "fmt"
    "net/http"
    "os"
    "time"

    _ "github.com/mattn/go-sqlite3"
)

type LogEntry struct {
    AgentID        string
    ToolName       string
    Protocol       string
    Decision       string
    PolicyRule     string
    LatencyMs      int64
    UpstreamMs     int64
    UpstreamStatus int
    Arguments      map[string]interface{}
}

type Logger struct {
    db           *sql.DB
    supabaseURL  string
    supabaseKey  string
    syncInterval time.Duration
}

func NewLogger(sqlitePath, supabaseURL, supabaseKey string) (*Logger, error) {
    db, err := sql.Open("sqlite3", sqlitePath)
    if err != nil {
        return nil, fmt.Errorf("audit: cannot open sqlite: %w", err)
    }

    // Create tables
    schema, err := os.ReadFile("audit/schema.sql")
    if err != nil {
        // Fallback: inline schema if file not found
        schema = []byte(`CREATE TABLE IF NOT EXISTS guard_logs (
            id TEXT PRIMARY KEY, ts TEXT NOT NULL,
            agent_id TEXT NOT NULL, tool_name TEXT NOT NULL,
            protocol TEXT NOT NULL, decision TEXT NOT NULL,
            policy_rule TEXT, latency_ms INTEGER, upstream_ms INTEGER,
            upstream_status INTEGER, arguments TEXT, synced INTEGER DEFAULT 0
        );`)
    }
    if _, err := db.Exec(string(schema)); err != nil {
        return nil, fmt.Errorf("audit: cannot create schema: %w", err)
    }

    syncInterval := 30 * time.Second
    if os.Getenv("AGENTGUARD_ENV") == "development" {
        syncInterval = 10 * time.Second
    }

    l := &Logger{
        db:           db,
        supabaseURL:  supabaseURL,
        supabaseKey:  supabaseKey,
        syncInterval: syncInterval,
    }

    // Start background Supabase sync worker
    go l.syncWorker()

    return l, nil
}

func (l *Logger) Log(entry LogEntry) error {
    id := NewID()
    ts := time.Now().UTC().Format(time.RFC3339Nano)

    argsJSON := "{}"
    if entry.Arguments != nil {
        b, _ := json.Marshal(entry.Arguments)
        s := string(b)
        if len(s) > 500 {
            s = s[:500] + "...[truncated]"
        }
        argsJSON = s
    }

    _, err := l.db.Exec(`
        INSERT INTO guard_logs
          (id, ts, agent_id, tool_name, protocol, decision,
           policy_rule, latency_ms, upstream_ms, upstream_status,
           arguments, synced)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0)`,
        id, ts,
        entry.AgentID, entry.ToolName, entry.Protocol,
        entry.Decision, entry.PolicyRule,
        entry.LatencyMs, entry.UpstreamMs, entry.UpstreamStatus,
        argsJSON,
    )
    return err
}

// syncWorker runs in a goroutine, ships unsynced rows to Supabase.
func (l *Logger) syncWorker() {
    ticker := time.NewTicker(l.syncInterval)
    defer ticker.Stop()
    for range ticker.C {
        l.syncToSupabase()
    }
}

// ImmediateSync immediately writes an entry to Supabase bypassing the sync queue.
func (l *Logger) ImmediateSync(entry *LogEntry) {
    go l.syncOneEntry(entry)
}

func (l *Logger) syncOneEntry(entry *LogEntry) {
    if l.supabaseURL == "" || l.supabaseKey == "" {
        return
    }

    // Convert args to JSON
    argsJSON := "{}"
    if entry.Arguments != nil {
        b, _ := json.Marshal(entry.Arguments)
        argsJSON = string(b)
    }

    // Reuse the exact same payload shape as batch
    batch := []map[string]interface{}{{
        "id":             NewID(),
        "ts":             time.Now().UTC().Format(time.RFC3339Nano),
        "agent_id":       entry.AgentID,
        "tool_name":      entry.ToolName,
        "protocol":       entry.Protocol,
        "decision":       entry.Decision,
        "policy_rule":    entry.PolicyRule,
        "latency_ms":     entry.LatencyMs,
        "upstream_ms":    entry.UpstreamMs,
        "upstream_status": entry.UpstreamStatus,
        "arguments":      argsJSON,
    }}

    body, _ := json.Marshal(batch)
    req, err := http.NewRequest("POST",
        l.supabaseURL+"/rest/v1/guard_logs",
        bytes.NewReader(body),
    )
    if err != nil {
        return
    }

    req.Header.Set("apikey", l.supabaseKey)
    req.Header.Set("Authorization", "Bearer "+l.supabaseKey)
    req.Header.Set("Content-Type", "application/json")
    req.Header.Set("Prefer", "return=minimal")

    client := &http.Client{Timeout: 10 * time.Second}
    resp, err := client.Do(req)
    if err == nil {
        resp.Body.Close()
    }
}

func (l *Logger) syncToSupabase() {
    if l.supabaseURL == "" || l.supabaseKey == "" {
        return
    }

    rows, err := l.db.Query(`
        SELECT id, ts, agent_id, tool_name, protocol, decision,
               policy_rule, latency_ms, upstream_ms,
               upstream_status, arguments
        FROM guard_logs WHERE synced = 0 LIMIT 100
    `)
    if err != nil {
        return
    }
    defer rows.Close()

    var batch []map[string]interface{}
    var ids []string

    for rows.Next() {
        var row struct {
            ID, Ts, AgentID, ToolName, Protocol,
            Decision, PolicyRule, Arguments string
            LatencyMs, UpstreamMs, UpstreamStatus int64
        }
        if err := rows.Scan(
            &row.ID, &row.Ts, &row.AgentID, &row.ToolName,
            &row.Protocol, &row.Decision, &row.PolicyRule,
            &row.LatencyMs, &row.UpstreamMs, &row.UpstreamStatus,
            &row.Arguments,
        ); err != nil {
            continue
        }
        batch = append(batch, map[string]interface{}{
            "id": row.ID, "ts": row.Ts,
            "agent_id": row.AgentID, "tool_name": row.ToolName,
            "protocol": row.Protocol, "decision": row.Decision,
            "policy_rule": row.PolicyRule,
            "latency_ms": row.LatencyMs,
            "upstream_ms": row.UpstreamMs,
            "upstream_status": row.UpstreamStatus,
            "arguments": row.Arguments,
        })
        ids = append(ids, row.ID)
    }

    if len(batch) == 0 {
        return
    }

    body, _ := json.Marshal(batch)
    req, _ := http.NewRequest("POST",
        l.supabaseURL+"/rest/v1/guard_logs",
        bytes.NewReader(body),
    )
    req.Header.Set("apikey", l.supabaseKey)
    req.Header.Set("Authorization", "Bearer "+l.supabaseKey)
    req.Header.Set("Content-Type", "application/json")
    req.Header.Set("Prefer", "resolution=merge-duplicates")

    client := &http.Client{Timeout: 10 * time.Second}
    resp, err := client.Do(req)
    if err != nil || resp.StatusCode >= 300 {
        return
    }
    resp.Body.Close()

    // Mark rows as synced
    for _, id := range ids {
        l.db.Exec(`UPDATE guard_logs SET synced = 1 WHERE id = ?`, id)
    }
}

func NewID() string {
    b := make([]byte, 16)
    rand.Read(b)
    return hex.EncodeToString(b)
}

func (l *Logger) DB() *sql.DB {
    return l.db
}

// FlushSync forces an immediate full sync to Supabase.
// Called on graceful shutdown. Blocks until done or ctx expires.
// Modified to work with the existing SQLite sync mechanism.
func (l *Logger) FlushSync(ctx context.Context) error {
    if l.supabaseURL == "" {
        return nil
    }
    
    // Sync remaining unsynced items before shutdown
    for i := 0; i < 5; i++ {
        l.syncToSupabase()
    }
    return nil
}
