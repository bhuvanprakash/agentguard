package spend

// DailySpendTracker tracks cumulative USD spend per agent per day.
//
// How spend is tracked:
//   ATXP calls include an amount in the payload.
//   Other protocol calls are counted at $0.001 per tool call
//   (configurable per-agent in policy YAML).
//
// Auto-block: if an agent exceeds spend_limit_daily_usd,
// ALL subsequent calls from that agent are blocked
// for the rest of that calendar day (UTC).
//
// The tracker uses an in-memory map refreshed at midnight UTC.
// It also persists to SQLite so restarts don't reset spend.

import (
    "database/sql"
    "fmt"
    "sync"
    "time"
)

type AgentSpend struct {
    AgentID   string
    Date      string  // YYYY-MM-DD UTC
    TotalUSD  float64
    CallCount int
}

type Tracker struct {
    mu      sync.RWMutex
    db      *sql.DB
    today   string                     // current UTC date YYYY-MM-DD
    spends  map[string]*AgentSpend     // key: "agentID:date"
}

func NewTracker(db *sql.DB) (*Tracker, error) {
    _, err := db.Exec(`
        CREATE TABLE IF NOT EXISTS agent_spend (
            agent_id   TEXT NOT NULL,
            date       TEXT NOT NULL,
            total_usd  REAL NOT NULL DEFAULT 0,
            call_count INTEGER NOT NULL DEFAULT 0,
            PRIMARY KEY (agent_id, date)
        );
    `)
    if err != nil {
        return nil, fmt.Errorf("spend: cannot create table: %w", err)
    }

    t := &Tracker{
        db:     db,
        today:  todayUTC(),
        spends: make(map[string]*AgentSpend),
    }
    t.loadToday()

    // Midnight reset goroutine
    go t.midnightReset()

    return t, nil
}

// Add records a spend amount for an agent.
// Returns the new total for today.
func (t *Tracker) Add(agentID string, amountUSD float64) float64 {
    t.mu.Lock()
    defer t.mu.Unlock()

    key := agentID + ":" + t.today
    if t.spends[key] == nil {
        t.spends[key] = &AgentSpend{
            AgentID: agentID,
            Date:    t.today,
        }
    }
    t.spends[key].TotalUSD  += amountUSD
    t.spends[key].CallCount++
    total := t.spends[key].TotalUSD

    // Persist asynchronously
    go t.persist(agentID, t.today, total, t.spends[key].CallCount)

    return total
}

// GetToday returns the total spend for an agent today.
func (t *Tracker) GetToday(agentID string) float64 {
    t.mu.RLock()
    defer t.mu.RUnlock()
    key := agentID + ":" + t.today
    if s := t.spends[key]; s != nil {
        return s.TotalUSD
    }
    return 0
}

// ExceedsLimit returns true if the agent has exceeded their limit.
func (t *Tracker) ExceedsLimit(agentID string, limitUSD float64) bool {
    if limitUSD <= 0 {
        return false // no limit set
    }
    return t.GetToday(agentID) >= limitUSD
}

func (t *Tracker) persist(agentID, date string, totalUSD float64, count int) {
    t.db.Exec(`
        INSERT INTO agent_spend (agent_id, date, total_usd, call_count)
        VALUES (?, ?, ?, ?)
        ON CONFLICT(agent_id, date)
        DO UPDATE SET total_usd = ?, call_count = ?`,
        agentID, date, totalUSD, count,
        totalUSD, count,
    )
}

func (t *Tracker) loadToday() {
    rows, err := t.db.Query(`
        SELECT agent_id, date, total_usd, call_count
        FROM agent_spend WHERE date = ?`, t.today,
    )
    if err != nil {
        return
    }
    defer rows.Close()
    for rows.Next() {
        var s AgentSpend
        rows.Scan(&s.AgentID, &s.Date, &s.TotalUSD, &s.CallCount)
        t.spends[s.AgentID+":"+s.Date] = &s
    }
}

func (t *Tracker) midnightReset() {
    for {
        now    := time.Now().UTC()
        next   := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 5, 0, time.UTC)
        timer  := time.NewTimer(time.Until(next))
        <-timer.C

        t.mu.Lock()
        t.today = todayUTC()
        t.spends = make(map[string]*AgentSpend)
        t.mu.Unlock()
        t.loadToday()
    }
}

func todayUTC() string {
    return time.Now().UTC().Format("2006-01-02")
}
