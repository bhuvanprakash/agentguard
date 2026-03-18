package protocol

// A2A (Agent-to-Agent Protocol) parser.
//
// A2A requests arrive as HTTP POST to /messages/send or /tasks/create.
// AgentGuard extracts the task type as the "tool name" for policy eval.
//
// A2A message format:
// POST /messages/send
// {
//   "role": "user",
//   "content": [{ "type": "text", "text": "..." }],
//   "taskId": "task-123",
//   "contextId": "ctx-456",
//   "metadata": {
//     "agentId": "researcher-agent",
//     "taskType": "web_search"
//   }
// }

import (
    "bytes"
    "encoding/json"
    "io"
    "net/http"
    "strings"
)

type A2AMessage struct {
    Role      string                 `json:"role"`
    Content   []interface{}          `json:"content"`
    TaskID    string                 `json:"taskId"`
    ContextID string                 `json:"contextId"`
    Metadata  map[string]interface{} `json:"metadata"`
}

// ParseA2A attempts to parse an HTTP request as an A2A message.
// Returns (call, true) if it's a valid A2A request.
func ParseA2A(req *http.Request, agentIDHeader string) (*AgentCall, bool) {
    path := req.URL.Path
    isA2A := strings.Contains(path, "/messages/send") ||
        strings.Contains(path, "/tasks/create") ||
        strings.Contains(path, "/tasks/send")

    if !isA2A || req.Method != http.MethodPost {
        return nil, false
    }

    body, err := io.ReadAll(io.LimitReader(req.Body, 1<<20))
    if err != nil {
        return nil, false
    }
    req.Body = io.NopCloser(bytes.NewReader(body))

    var msg A2AMessage
    if err := json.Unmarshal(body, &msg); err != nil {
        return nil, false
    }

    // Extract agentId and taskType from metadata
    agentID := agentIDHeader
    taskType := "a2a_task"

    if msg.Metadata != nil {
        if aid, ok := msg.Metadata["agentId"].(string); ok && aid != "" {
            if agentID == "" {
                agentID = aid
            }
        }
        if tt, ok := msg.Metadata["taskType"].(string); ok && tt != "" {
            taskType = tt
        }
    }

    if agentID == "" {
        agentID = "unknown-a2a"
    }

    return &AgentCall{
        Protocol:  "a2a",
        AgentID:   agentID,
        ToolName:  taskType,
        Arguments: msg.Metadata,
        RawBody:   body,
        RequestID: msg.TaskID,
    }, true
}
