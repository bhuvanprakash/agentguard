// notify/notifier.go
// Unified notifier that sends to all enabled channels.

package notify

import (
    "log/slog"
    "time"
)

type EscalationEvent struct {
    ID        string
    AgentID   string
    ToolName  string
    Protocol  string
    Arguments map[string]interface{}
    Ts        time.Time
    ExpiresAt time.Time
}

type Notifier struct {
    slack   *SlackNotifier
    discord *DiscordNotifier
}

func NewNotifier() *Notifier {
    return &Notifier{
        slack:   NewSlackNotifier(),
        discord: NewDiscordNotifier(),
    }
}

func (n *Notifier) SendEscalation(e EscalationEvent) {
    p := EscalationPayload{
        ID:        e.ID,
        AgentID:   e.AgentID,
        ToolName:  e.ToolName,
        Protocol:  e.Protocol,
        Arguments: e.Arguments,
        Ts:        e.Ts,
        ExpiresAt: e.ExpiresAt,
    }

    if n.slack.Enabled() {
        go func() {
            if err := n.slack.Send(p); err != nil {
                slog.Warn("slack notify failed", "err", err)
            }
        }()
    }

    if n.discord.Enabled() {
        go func() {
            if err := n.discord.Send(p); err != nil {
                slog.Warn("discord notify failed", "err", err)
            }
        }()
    }
}
