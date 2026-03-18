// notify/slack.go
// Slack escalation notifications.

package notify

import (
    "bytes"
    "encoding/json"
    "fmt"
    "net/http"
    "os"
    "time"
)

type SlackNotifier struct {
    webhookURL   string
    dashboardURL string
}

func NewSlackNotifier() *SlackNotifier {
    return &SlackNotifier{
        webhookURL:   os.Getenv("AGENTGUARD_SLACK_WEBHOOK_URL"),
        dashboardURL: os.Getenv("AGENTGUARD_DASHBOARD_URL"),
    }
}

func (s *SlackNotifier) Enabled() bool {
    return s.webhookURL != ""
}

type EscalationPayload struct {
    ID        string                 `json:"id"`
    AgentID   string                 `json:"agent_id"`
    ToolName  string                 `json:"tool_name"`
    Protocol  string                 `json:"protocol"`
    Arguments map[string]interface{} `json:"arguments"`
    Ts        time.Time              `json:"ts"`
    ExpiresAt time.Time              `json:"expires_at"`
}

func (s *SlackNotifier) Send(e EscalationPayload) error {
    if !s.Enabled() {
        return nil
    }

    approveURL := fmt.Sprintf(
        "%s/dashboard/agents/escalations",
        s.dashboardURL,
    )
    rejectURL := fmt.Sprintf(
        "%s/dashboard/agents/escalations",
        s.dashboardURL,
    )

    argsStr := "{}"
    if b, err := json.Marshal(e.Arguments); err == nil {
        argsStr = string(b)
        if len(argsStr) > 200 {
            argsStr = argsStr[:197] + "..."
        }
    }

    payload := map[string]interface{}{
        "blocks": []interface{}{
            map[string]interface{}{
                "type": "header",
                "text": map[string]interface{}{
                    "type": "plain_text",
                    "text": "⚠ AgentGuard Escalation",
                },
            },
            map[string]interface{}{"type": "divider"},
            map[string]interface{}{
                "type": "section",
                "fields": []interface{}{
                    map[string]interface{}{
                        "type": "mrkdwn",
                        "text": fmt.Sprintf("*Agent*\n`%s`", e.AgentID),
                    },
                    map[string]interface{}{
                        "type": "mrkdwn",
                        "text": fmt.Sprintf("*Tool*\n`%s`", e.ToolName),
                    },
                    map[string]interface{}{
                        "type": "mrkdwn",
                        "text": fmt.Sprintf("*Protocol*\n%s", e.Protocol),
                    },
                    map[string]interface{}{
                        "type": "mrkdwn",
                        "text": fmt.Sprintf(
                            "*Expires*\n%s",
                            e.ExpiresAt.Format("15:04 UTC"),
                        ),
                    },
                },
            },
            map[string]interface{}{
                "type": "section",
                "text": map[string]interface{}{
                    "type": "mrkdwn",
                    "text": fmt.Sprintf(
                        "*Arguments*\n```%s```", argsStr,
                    ),
                },
            },
            map[string]interface{}{
                "type": "actions",
                "elements": []interface{}{
                    map[string]interface{}{
                        "type":  "button",
                        "style": "primary",
                        "text": map[string]interface{}{
                            "type": "plain_text",
                            "text": "✓ Review",
                        },
                        "url": approveURL,
                    },
                    map[string]interface{}{
                        "type":  "button",
                        "style": "danger",
                        "text": map[string]interface{}{
                            "type": "plain_text",
                            "text": "✗ Reject",
                        },
                        "url": rejectURL,
                    },
                },
            },
            map[string]interface{}{
                "type": "context",
                "elements": []interface{}{
                    map[string]interface{}{
                        "type": "mrkdwn",
                        "text": fmt.Sprintf(
                            "ID: `%s` · AgentGuard v0.1.0",
                            e.ID,
                        ),
                    },
                },
            },
        },
    }

    body, _ := json.Marshal(payload)
    req, err := http.NewRequest(
        "POST", s.webhookURL,
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

    if resp.StatusCode != 200 && resp.StatusCode != 201 {
        return fmt.Errorf("slack webhook returned %d", resp.StatusCode)
    }
    return nil
}
