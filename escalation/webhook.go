package escalation

// Webhook notifier — sends an HTTP POST to the user's
// configured webhook URL when an escalation is created.
//
// Webhook payload:
// {
//   "event":        "agentguard.escalation.created",
//   "escalation_id": "abc123",
//   "agent_id":      "billing-agent",
//   "tool_name":     "send_payment",
//   "protocol":      "mcp",
//   "arguments":     { "amount": 500 },
//   "expires_at":    "2026-03-19T02:00:00Z",
//   "approve_url":   "http://agentguard:7777/escalations/abc123/approve",
//   "reject_url":    "http://agentguard:7777/escalations/abc123/reject",
//   "dashboard_url": "https://nascentist.ai/dashboard/agents/escalations"
// }
//
// Delivery:
//   3 attempts with exponential backoff (1s, 2s, 4s).
//   If all fail, escalation stays pending — user can still
//   approve/reject from the dashboard.
//
// Security:
//   Each webhook POST is signed with HMAC-SHA256 using
//   the user's webhook secret (stored in Supabase profiles).
//   Header: X-AgentGuard-Signature: sha256=<hex>

import (
    "bytes"
    "crypto/hmac"
    "crypto/sha256"
    "encoding/hex"
    "encoding/json"
    "fmt"
    "net/http"
    "os"
    "time"
)

type WebhookPayload struct {
    Event         string                 `json:"event"`
    EscalationID  string                 `json:"escalation_id"`
    AgentID       string                 `json:"agent_id"`
    ToolName      string                 `json:"tool_name"`
    Protocol      string                 `json:"protocol"`
    Arguments     map[string]interface{} `json:"arguments"`
    ExpiresAt     string                 `json:"expires_at"`
    ApproveURL    string                 `json:"approve_url"`
    RejectURL     string                 `json:"reject_url"`
    DashboardURL  string                 `json:"dashboard_url"`
    Ts            string                 `json:"ts"`
}

type Notifier struct {
    guardBaseURL    string // e.g. "http://agentguard:7777"
    dashboardURL    string // e.g. "https://nascentist.ai"
    httpClient      *http.Client
}

func NewNotifier(guardBaseURL, dashboardURL string) *Notifier {
    return &Notifier{
        guardBaseURL: guardBaseURL,
        dashboardURL: dashboardURL,
        httpClient: &http.Client{
            Timeout: 10 * time.Second,
        },
    }
}

// Send delivers the webhook with retries.
// Non-blocking: caller does NOT need to wait for delivery.
func (n *Notifier) Send(
    webhookURL  string,
    webhookSecret string,
    e           *Escalation,
) {
    if webhookURL == "" {
        // Still notify email service directly
        go n.notifyEmailService(e)
        return
    }
    go n.sendWithRetry(webhookURL, webhookSecret, e, 3)
}

func (n *Notifier) sendWithRetry(
    url, secret string,
    e           *Escalation,
    maxAttempts int,
) {
    payload := WebhookPayload{
        Event:        "agentguard.escalation.created",
        EscalationID: e.ID,
        AgentID:      e.AgentID,
        ToolName:     e.ToolName,
        Protocol:     e.Protocol,
        Arguments:    e.Arguments,
        ExpiresAt:    e.ExpiresAt.Format(time.RFC3339),
        ApproveURL:   fmt.Sprintf("%s/escalations/%s/approve", n.guardBaseURL, e.ID),
        RejectURL:    fmt.Sprintf("%s/escalations/%s/reject",  n.guardBaseURL, e.ID),
        DashboardURL: n.dashboardURL + "/dashboard/agents/escalations",
        Ts:           time.Now().UTC().Format(time.RFC3339),
    }

    body, err := json.Marshal(payload)
    if err != nil {
        return
    }

    for attempt := 1; attempt <= maxAttempts; attempt++ {
        if err := n.doPost(url, secret, body); err == nil {
            break // success
        }
        if attempt < maxAttempts {
            delay := time.Duration(attempt) * time.Second
            time.Sleep(delay)
        }
    }
    
    // Also notify the Nascentist email service
    go n.notifyEmailService(e)
}

func (n *Notifier) notifyEmailService(e *Escalation) {
    internalSecret := os.Getenv("INTERNAL_WEBHOOK_SECRET")
    dashURL        := n.dashboardURL

    body, _ := json.Marshal(map[string]interface{}{
        "escalation_id": e.ID,
        "agent_id":      e.AgentID,
        "tool_name":     e.ToolName,
        "protocol":      e.Protocol,
        "expires_at":    e.ExpiresAt.Format(time.RFC3339),
    })

    req, _ := http.NewRequest("POST",
        dashURL+"/api/v1/guard/escalations/notify",
        bytes.NewReader(body),
    )
    req.Header.Set("Content-Type",      "application/json")
    req.Header.Set("X-Internal-Secret", internalSecret)

    client := &http.Client{Timeout: 5 * time.Second}
    resp, err := client.Do(req)
    if err == nil {
        resp.Body.Close()
    }
}

func (n *Notifier) doPost(url, secret string, body []byte) error {
    req, err := http.NewRequest("POST", url, bytes.NewReader(body))
    if err != nil {
        return err
    }

    req.Header.Set("Content-Type", "application/json")
    req.Header.Set("User-Agent", "AgentGuard/1.0")

    // HMAC signature
    if secret != "" {
        mac := hmac.New(sha256.New, []byte(secret))
        mac.Write(body)
        sig := "sha256=" + hex.EncodeToString(mac.Sum(nil))
        req.Header.Set("X-AgentGuard-Signature", sig)
    }

    resp, err := n.httpClient.Do(req)
    if err != nil {
        return err
    }
    defer resp.Body.Close()

    if resp.StatusCode < 200 || resp.StatusCode >= 300 {
        return fmt.Errorf("webhook returned %d", resp.StatusCode)
    }
    return nil
}
