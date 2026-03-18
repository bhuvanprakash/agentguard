package memory

// Client calls Nascentist's existing memory API to get
// agent context before evaluating policies.
//
// This is what makes AgentGuard context-aware:
//   "billing-agent has already spent $490 today (limit $500)"
//   "billing-agent attempted delete_customer 3 times this hour"
//
// If Nascentist memory is not configured, AgentGuard works
// fine without it — policies still apply, just without history.
//
// Nascentist memory API endpoint used:
//   POST /api/v1/memory/search
//   Body: { "query": "<agentId> <toolName>", "limit": 5 }
//   Auth: Authorization: Bearer <NASCENTIST_INTERNAL_KEY>

import (
    "bytes"
    "context"
    "encoding/json"
    "fmt"
    "net/http"
    "time"
)

type Client struct {
    baseURL string
    apiKey  string
    http    *http.Client
}

type Memory struct {
    Key   string `json:"key"`
    Value string `json:"value"`
    Type  string `json:"type"`
}

type AgentContext struct {
    Memories    []Memory
    Available   bool   // false if memory API is unreachable
    ContextText string // formatted context for logging
}

func NewClient(baseURL, apiKey string) *Client {
    return &Client{
        baseURL: baseURL,
        apiKey:  apiKey,
        http: &http.Client{
            Timeout: 500 * time.Millisecond, // must be fast — on hot path
        },
    }
}

// GetContext fetches agent memories relevant to a tool call.
// Never blocks longer than 500ms — returns empty context on timeout.
func (c *Client) GetContext(
    ctx context.Context,
    agentID, toolName string,
) AgentContext {
    if c.baseURL == "" || c.apiKey == "" {
        return AgentContext{Available: false}
    }

    query := fmt.Sprintf("agent:%s tool:%s", agentID, toolName)
    body, _ := json.Marshal(map[string]interface{}{
        "query": query,
        "limit": 5,
    })

    req, err := http.NewRequestWithContext(ctx, "POST",
        c.baseURL+"/api/v1/memory/search",
        bytes.NewReader(body),
    )
    if err != nil {
        return AgentContext{Available: false}
    }
    req.Header.Set("Authorization", "Bearer "+c.apiKey)
    req.Header.Set("Content-Type", "application/json")

    resp, err := c.http.Do(req)
    if err != nil || resp.StatusCode != 200 {
        return AgentContext{Available: false}
    }
    defer resp.Body.Close()

    var result struct {
        Memories []Memory `json:"memories"`
    }
    if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
        return AgentContext{Available: false}
    }

    contextText := ""
    for _, m := range result.Memories {
        contextText += fmt.Sprintf("- %s: %s\n", m.Key, m.Value)
    }

    return AgentContext{
        Memories:    result.Memories,
        Available:   true,
        ContextText: contextText,
    }
}
