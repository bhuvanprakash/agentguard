// notify/discord.go
// Discord escalation notifications.

package notify

import (
    "bytes"
    "encoding/json"
    "fmt"
    "net/http"
    "os"
    "time"
)

type DiscordNotifier struct {
    webhookURL   string
    dashboardURL string
}

func NewDiscordNotifier() *DiscordNotifier {
    return &DiscordNotifier{
        webhookURL:   os.Getenv("AGENTGUARD_DISCORD_WEBHOOK_URL"),
        dashboardURL: os.Getenv("AGENTGUARD_DASHBOARD_URL"),
    }
}

func (d *DiscordNotifier) Enabled() bool {
    return d.webhookURL != ""
}

func (d *DiscordNotifier) Send(e EscalationPayload) error {
    if !d.Enabled() {
        return nil
    }

    reviewURL := fmt.Sprintf(
        "%s/dashboard/agents/escalations",
        d.dashboardURL,
    )

    argsStr := "{}"
    if b, err := json.Marshal(e.Arguments); err == nil {
        argsStr = string(b)
        if len(argsStr) > 300 {
            argsStr = argsStr[:297] + "..."
        }
    }

    // Discord embed color: amber = 0xF59E0B
    payload := map[string]interface{}{
        "embeds": []interface{}{
            map[string]interface{}{
                "title":       "⚠ AgentGuard Escalation",
                "description": "A tool call requires your approval.",
                "color":       0xF59E0B,
                "fields": []interface{}{
                    map[string]interface{}{
                        "name":   "Agent",
                        "value":  fmt.Sprintf("`%s`", e.AgentID),
                        "inline": true,
                    },
                    map[string]interface{}{
                        "name":   "Tool",
                        "value":  fmt.Sprintf("`%s`", e.ToolName),
                        "inline": true,
                    },
                    map[string]interface{}{
                        "name":   "Protocol",
                        "value":  e.Protocol,
                        "inline": true,
                    },
                    map[string]interface{}{
                        "name":   "Arguments",
                        "value":  fmt.Sprintf("```json\n%s\n```", argsStr),
                        "inline": false,
                    },
                },
                "footer": map[string]interface{}{
                    "text": fmt.Sprintf(
                        "ID: %s · Review: %s",
                        e.ID, reviewURL,
                    ),
                },
                "timestamp": e.Ts.Format(time.RFC3339),
            },
        },
    }

    body, _ := json.Marshal(payload)
    req, err := http.NewRequest(
        "POST",
        d.webhookURL+"?wait=true",
        bytes.NewReader(body),
    )
    if err != nil {
        return err
    }
    req.Header.Set("Content-Type", "application/json")

    client := &http.Client{Timeout: 5 * time.Second}
    resp, err := client.Do(req)
    if err != nil {
        return err
    }
    defer resp.Body.Close()

    if resp.StatusCode < 200 || resp.StatusCode >= 300 {
        return fmt.Errorf(
            "discord webhook returned %d", resp.StatusCode,
        )
    }
    return nil
}
