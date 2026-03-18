package protocol

// MCP (Model Context Protocol) parser.
//
// MCP tool calls arrive as JSON-RPC 2.0 over HTTP POST.
// AgentGuard needs to extract:
//   - The tool name (method: "tools/call", params.name)
//   - The agent ID (from X-Agent-ID header or params.agentId)
//   - The arguments (params.arguments — for logging only)
//
// MCP JSON-RPC format:
// {
//   "jsonrpc": "2.0",
//   "id": "req-123",
//   "method": "tools/call",
//   "params": {
//     "name": "send_payment",
//     "arguments": { "amount": 500, "currency": "INR" }
//   }
// }
//
// AgentGuard does NOT modify the MCP message.
// It reads it, makes a decision, then either:
//   - Forwards it unchanged (allow)
//   - Returns a JSON-RPC error (block)
//   - Returns a JSON-RPC "pending human approval" response (escalate)

import (
    "bytes"
    "encoding/json"
    "io"
    "net/http"
)

type MCPRequest struct {
    JSONRPC string          `json:"jsonrpc"`
    ID      interface{}     `json:"id"`
    Method  string          `json:"method"`
    Params  MCPToolCallParams `json:"params"`
}

type MCPToolCallParams struct {
    Name      string                 `json:"name"`
    Arguments map[string]interface{} `json:"arguments"`
    AgentID   string                 `json:"agentId"` // optional
}

type AgentCall struct {
    Protocol  string                 // "mcp", "a2a", "atxp", "unknown"
    AgentID   string                 // who is making the call
    ToolName  string                 // what tool is being called
    Arguments map[string]interface{} // what arguments
    RawBody   []byte                 // original request body (for forwarding)
    RequestID interface{}            // for JSON-RPC correlation
}

// ParseMCP attempts to parse an HTTP request as an MCP tool call.
// Returns (call, true) if it's a valid MCP tools/call request.
// Returns (nil, false) if it's not MCP format.
//
// IMPORTANT: This reads req.Body completely and replaces it
// so downstream handlers can still read it.
func ParseMCP(req *http.Request, agentIDHeader string) (*AgentCall, bool) {
    if req.Method != http.MethodPost {
        return nil, false
    }

    body, err := io.ReadAll(io.LimitReader(req.Body, 1<<20)) // 1MB limit
    if err != nil {
        return nil, false
    }
    // Restore body for forwarding
    req.Body = io.NopCloser(bytes.NewReader(body))

    var rpc MCPRequest
    if err := json.Unmarshal(body, &rpc); err != nil {
        return nil, false
    }
    if rpc.Method != "tools/call" || rpc.Params.Name == "" {
        return nil, false
    }

    // Agent ID: header > params.agentId > "unknown"
    agentID := agentIDHeader
    if agentID == "" {
        agentID = rpc.Params.AgentID
    }
    if agentID == "" {
        agentID = "unknown"
    }

    return &AgentCall{
        Protocol:  "mcp",
        AgentID:   agentID,
        ToolName:  rpc.Params.Name,
        Arguments: rpc.Params.Arguments,
        RawBody:   body,
        RequestID: rpc.ID,
    }, true
}

// BlockResponse returns an MCP-compatible JSON-RPC error response.
func MCPBlockResponse(requestID interface{}, toolName string) []byte {
    resp := map[string]interface{}{
        "jsonrpc": "2.0",
        "id":      requestID,
        "error": map[string]interface{}{
            "code":    -32600,
            "message": "AgentGuard: tool call blocked by policy",
            "data": map[string]interface{}{
                "tool":   toolName,
                "reason": "This tool is blocked by your AgentGuard policy.",
                "docs":   "https://nascentist.ai/docs/agentguard/policies",
            },
        },
    }
    data, _ := json.Marshal(resp)
    return data
}

// EscalateResponse returns an MCP-compatible response indicating
// the call is pending human approval.
func MCPEscalateResponse(requestID interface{}, toolName string) []byte {
    resp := map[string]interface{}{
        "jsonrpc": "2.0",
        "id":      requestID,
        "result": map[string]interface{}{
            "status":  "pending_approval",
            "tool":    toolName,
            "message": "AgentGuard: this action requires human approval.",
            "docs":    "https://nascentist.ai/docs/agentguard/escalation",
        },
    }
    data, _ := json.Marshal(resp)
    return data
}

// MCPEscalateResponseWithID returns an escalate response
// with the escalation ID included for agent polling.
func MCPEscalateResponseWithID(
    requestID   interface{},
    toolName    string,
    escalationID string,
) []byte {
    resp := map[string]interface{}{
        "jsonrpc": "2.0",
        "id":      requestID,
        "result": map[string]interface{}{
            "status":        "pending_approval",
            "tool":          toolName,
            "escalation_id": escalationID,
            "message":       "AgentGuard: awaiting human approval.",
            "poll_url":      "/escalations/" + escalationID,
            "docs":          "https://nascentist.ai/docs/agentguard/escalation",
        },
    }
    data, _ := json.Marshal(resp)
    return data
}
