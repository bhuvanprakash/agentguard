package protocol

// ATXP (Agent Transaction Protocol) parser.
//
// ATXP payment intents arrive as HTTP POST with a
// special header: X-ATXP-Version or a payment body shape.
//
// ATXP requirePayment format:
// POST /atxp/payment
// Headers: X-ATXP-Version: 1
// {
//   "agentId": "billing-agent",
//   "tool": "send_invoice",
//   "amount": { "value": "0.005", "currency": "USD" },
//   "callbackUrl": "https://..."
// }

import (
    "bytes"
    "encoding/json"
    "io"
    "net/http"
    "strings"
)

type ATXPPayment struct {
    AgentID     string                 `json:"agentId"`
    Tool        string                 `json:"tool"`
    Amount      map[string]interface{} `json:"amount"`
    CallbackURL string                 `json:"callbackUrl"`
}

// ParseATXP attempts to parse an HTTP request as an ATXP payment.
func ParseATXP(req *http.Request, agentIDHeader string) (*AgentCall, bool) {
    isATXP := req.Header.Get("X-ATXP-Version") != "" ||
        strings.Contains(req.URL.Path, "/atxp/")

    if !isATXP || req.Method != http.MethodPost {
        return nil, false
    }

    body, err := io.ReadAll(io.LimitReader(req.Body, 1<<20))
    if err != nil {
        return nil, false
    }
    req.Body = io.NopCloser(bytes.NewReader(body))

    var payment ATXPPayment
    if err := json.Unmarshal(body, &payment); err != nil {
        return nil, false
    }

    agentID := agentIDHeader
    if agentID == "" {
        agentID = payment.AgentID
    }
    if agentID == "" {
        agentID = "unknown-atxp"
    }

    toolName := payment.Tool
    if toolName == "" {
        toolName = "require_payment" // default ATXP action
    }

    return &AgentCall{
        Protocol:  "atxp",
        AgentID:   agentID,
        ToolName:  toolName,
        Arguments: map[string]interface{}{"amount": payment.Amount},
        RawBody:   body,
        RequestID: nil,
    }, true
}
