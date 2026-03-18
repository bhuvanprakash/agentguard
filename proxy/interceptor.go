package proxy

// Interceptor is the core of AgentGuard.
// Every request passes through here before reaching upstream.
//
// Flow:
//   1. Detect protocol (MCP? A2A? ATXP? Unknown?)
//   2. Parse agent ID + tool name
//   3. (Optional) fetch agent context from Nascentist memory
//   4. Evaluate policy (allow / block / escalate)
//   5. Log decision to audit logger
//   6. Either forward to upstream or return block/escalate response

import (
    "encoding/json"
    "fmt"
    "net/http"
    "net/http/httputil"
    "net/url"
    "time"

    "github.com/nascentist/agentguard/audit"
    "github.com/nascentist/agentguard/memory"
    "github.com/nascentist/agentguard/policy"
    "github.com/nascentist/agentguard/protocol"
    "github.com/nascentist/agentguard/escalation"
    "github.com/nascentist/agentguard/spend"
    "github.com/nascentist/agentguard/auth"
)

type Interceptor struct {
    upstream      *httputil.ReverseProxy
    engine        *policy.Engine
    logger        *audit.Logger
    memory        *memory.Client
    escStore      *escalation.Store
    escNotifier   *escalation.Notifier
    spendTracker  *spend.Tracker
    rateLimiter   *RateLimiter
    authStore     *auth.AgentAuthStore
    webhookURL    string
    webhookSecret string
}

func NewInterceptor(
    upstreamURL   string,
    engine        *policy.Engine,
    logger        *audit.Logger,
    mem           *memory.Client,
    escStore      *escalation.Store,
    escNotifier   *escalation.Notifier,
    spendTracker  *spend.Tracker,
    rateLimiter   *RateLimiter,
    authStore     *auth.AgentAuthStore,
    webhookURL    string,
    webhookSecret string,
) (*Interceptor, error) {
    target, err := url.Parse(upstreamURL)
    if err != nil {
        return nil, fmt.Errorf("interceptor: invalid upstream URL: %w", err)
    }

    rp := &httputil.ReverseProxy{
        Director: func(req *http.Request) {
            req.URL.Scheme = target.Scheme
            req.URL.Host = target.Host
            req.Host = target.Host
            // Add header so upstream knows it went through AgentGuard
            req.Header.Set("X-AgentGuard-Version", "1")
            req.Header.Set("X-AgentGuard-Decision", "allow")
        },
        ModifyResponse: func(resp *http.Response) error {
            // Strip backend server info from response
            resp.Header.Del("Server")
            resp.Header.Del("X-Powered-By")
            return nil
        },
        ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
            http.Error(w,
                `{"error":{"code":"upstream_error","message":"Upstream unavailable"}}`,
                http.StatusBadGateway,
            )
        },
    }

    return &Interceptor{
        upstream:      rp,
        engine:        engine,
        logger:        logger,
        memory:        mem,
        escStore:      escStore,
        escNotifier:   escNotifier,
        spendTracker:  spendTracker,
        rateLimiter:   rateLimiter,
        authStore:     authStore,
        webhookURL:    webhookURL,
        webhookSecret: webhookSecret,
    }, nil
}

// ServeHTTP handles every incoming request.
func (i *Interceptor) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    start := time.Now()

    // Extract agent ID from header (standard across all protocols)
    agentIDHeader := ExtractAgentID(r)

    // Auth check (before rate limit and policy)
    if i.authStore != nil {
        authResult := i.authStore.CheckRequest(r, agentIDHeader)
        if !authResult.Allowed {
            StandardResponseHeaders(w, "block")
            w.Header().Set("Content-Type", "application/json")
            w.WriteHeader(http.StatusUnauthorized)
            // Error response body
            json.NewEncoder(w).Encode(map[string]interface{}{
                "error": map[string]interface{}{
                    "code":    -32600,
                    "message": "AgentGuard: authentication failed",
                    "data": map[string]string{
                        "agent_id": agentIDHeader,
                        "reason":   authResult.Reason,
                    },
                },
            })

            // Log the auth failure
            go i.logger.Log(audit.LogEntry{
                AgentID:    agentIDHeader,
                Decision:   "block",
                PolicyRule: "auth_failed",
            })
            return
        }
    }

    // ── 1. Detect protocol and parse call ─────────────────────
    var call *protocol.AgentCall
    var parsed bool

    if call, parsed = protocol.ParseMCP(r, agentIDHeader); parsed {
        // matched MCP
    } else if call, parsed = protocol.ParseA2A(r, agentIDHeader); parsed {
        // matched A2A
    } else if call, parsed = protocol.ParseATXP(r, agentIDHeader); parsed {
        // matched ATXP
    } else {
        // Unknown protocol — forward directly without policy check
        i.upstream.ServeHTTP(w, r)
        return
    }

    // ── 1b. Rate limit check ──────────────────────────────────
    if !i.rateLimiter.Allow(call.AgentID) {
        StandardResponseHeaders(w, "block")
        w.Header().Set("Retry-After", "60")
        w.Header().Set("Content-Type", "application/json")
        w.WriteHeader(http.StatusTooManyRequests)
        body := []byte(`{"error":{"code":-32600,"message":"AgentGuard: rate limit exceeded","data":{"agent_id":"` + call.AgentID + `","reason":"rate_limit"}}}`)
        if call.Protocol == "mcp" {
            // MCP standard error for rate limiting
            body = []byte(`{"jsonrpc":"2.0","id":` + fmt.Sprintf("%v", call.RequestID) + `,"error":{"code":-32005,"message":"Rate limit exceeded"}}`)
        }
        w.Write(body)

        // Log the rate-limit block
        go i.logger.Log(audit.LogEntry{
            AgentID:    call.AgentID,
            ToolName:   call.ToolName,
            Protocol:   call.Protocol,
            Decision:   "block",
            PolicyRule: "rate_limit",
            LatencyMs:  0,
        })
        return
    }

    // ── 2. (Optional) fetch agent context ─────────────────────
    // This adds ~50ms max — acceptable for agent calls
    // which are themselves 200ms-2000ms
    _ = i.memory.GetContext(r.Context(), call.AgentID, call.ToolName)

    // ── 2b. Spend limit check ─────────────────────────────────
    // If the agent policy has spend_limit_daily_usd set,
    // check if they've exceeded it before evaluating further.
    agentPolicy := i.engine.GetAgentPolicy(call.AgentID)
    decision := policy.DecisionAllow
    policyRule := "yaml_rule"

    if agentPolicy != nil && agentPolicy.SpendLimitDailyUSD > 0 {
        if i.spendTracker.ExceedsLimit(
            call.AgentID,
            agentPolicy.SpendLimitDailyUSD,
        ) {
            // Override decision to block — spend limit exceeded
            decision = policy.DecisionBlock
            policyRule = "spend_limit_exceeded"
        }
    }

    // ── 3. Evaluate policy ────────────────────────────────────
    policyStart := time.Now()
    if decision == policy.DecisionAllow {
        decision, policyRule = i.engine.Evaluate(call.AgentID, call.ToolName, call.Arguments)
    }
    policyLatency := time.Since(policyStart).Milliseconds()

    if policyRule == "yaml_rule" && policy.IsIrreversible(call.ToolName) {
        policyRule = "irreversible_hardcoded"
    }

    // ── 4. Log decision ───────────────────────────────────────
    logEntry := audit.LogEntry{
        AgentID:    call.AgentID,
        ToolName:   call.ToolName,
        Protocol:   call.Protocol,
        Decision:   string(decision),
        PolicyRule: policyRule,
        LatencyMs:  policyLatency,
        Arguments:  call.Arguments,
    }
    defer func() {
        _ = i.logger.Log(logEntry)
    }()

    StandardResponseHeaders(w, string(decision))

    // ── 5. Act on decision ────────────────────────────────────
    switch decision {

    case policy.DecisionBlock:
        w.Header().Set("Content-Type", "application/json")
        w.Header().Set("X-AgentGuard-Decision", "block")
        w.WriteHeader(http.StatusForbidden)

        var body []byte
        if call.Protocol == "mcp" {
            body = protocol.MCPBlockResponse(call.RequestID, call.ToolName)
        } else {
            body = []byte(fmt.Sprintf(
                `{"error":{"code":"blocked","message":"AgentGuard blocked: %s","tool":"%s"}}`,
                call.ToolName, call.ToolName,
            ))
        }
        w.Write(body)
        return

    case policy.DecisionEscalate:
        // Create escalation record
        escID := audit.NewID() // Use the original logic, assuming audit package has something, else just make it random
        
        esc := &escalation.Escalation{
            ID:         escID,
            AgentID:    call.AgentID,
            ToolName:   call.ToolName,
            Protocol:   call.Protocol,
            Arguments:  call.Arguments,
            WebhookURL: i.webhookURL,
            GuardLogID: escID, // same ID for log correlation
        }
        if storeErr := i.escStore.Create(esc); storeErr != nil {
            // Non-fatal — still return escalate response
        }

        // Fire webhook asynchronously
        i.escNotifier.Send(i.webhookURL, i.webhookSecret, esc)

        // Immediate sync to Supabase for realtime UI update
        go i.logger.ImmediateSync(&logEntry)

        // Return 202 with escalation ID so agent can poll
        w.Header().Set("Content-Type",        "application/json")
        w.Header().Set("X-AgentGuard-Decision", "escalate")
        w.Header().Set("X-Escalation-ID",      escID)
        w.WriteHeader(http.StatusAccepted)

        var body []byte
        if call.Protocol == "mcp" {
            body = protocol.MCPEscalateResponseWithID(
                call.RequestID, call.ToolName, escID)
        } else {
            body = []byte(`{"status":"pending_approval","escalation_id":"` +
                escID + `","tool":"` + call.ToolName + `"}`)
        }
        w.Write(body)
        return

    case policy.DecisionAllow:
        // For ATXP calls, record spend amount
        if call.Protocol == "atxp" {
            if amount, ok := call.Arguments["amount"].(map[string]interface{}); ok {
                if val, ok := amount["value"].(string); ok {
                    var usd float64
                    fmt.Sscanf(val, "%f", &usd)
                    i.spendTracker.Add(call.AgentID, usd)
                }
            }
        } else {
            // Count non-ATXP calls at $0.001 each for spend tracking
            i.spendTracker.Add(call.AgentID, 0.001)
        }

        // Forward to upstream
        w.Header().Set("X-AgentGuard-Decision", "allow")
        upstreamStart := time.Now()
        i.upstream.ServeHTTP(w, r)
        _ = time.Since(upstreamStart).Milliseconds()
        _ = start
        return
    }
}
