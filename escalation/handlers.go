// escalation/handlers.go
// HTTP handlers for the dashboard to list, approve, and reject escalations.

package escalation

import (
    "encoding/json"
    "net/http"
    "os"
    "strings"

    "github.com/nascentist/agentguard/notify"
)

type Handler struct {
    store    *Store
    notifier *Notifier
    unified  *notify.Notifier
    adminKey string
}

func NewHandler(store *Store, notifier *Notifier, unified *notify.Notifier) *Handler {
    return &Handler{
        store:    store,
        notifier: notifier,
        unified:  unified,
        adminKey: os.Getenv("AGENTGUARD_ADMIN_KEY"),
    }
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
    mux.HandleFunc("/api/v1/escalations", h.listPending)
    mux.HandleFunc("/api/v1/escalations/resolve", h.resolve)
}

func (h *Handler) checkAuth(r *http.Request) bool {
    if h.adminKey == "" {
        return true
    }
    key := r.Header.Get("X-AgentGuard-Admin-Key")
    return key == h.adminKey
}

func (h *Handler) listPending(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodGet {
        http.Error(w, "method_not_allowed", 405)
        return
    }
    if !h.checkAuth(r) {
        http.Error(w, "unauthorized", 401)
        return
    }

    list := h.store.ListPending()
    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(list)
}

func (h *Handler) resolve(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodPost {
        http.Error(w, "method_not_allowed", 405)
        return
    }
    if !h.checkAuth(r) {
        http.Error(w, "unauthorized", 401)
        return
    }

    var req struct {
        ID     string `json:"id"`
        Status string `json:"status"`
    }
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        http.Error(w, "bad_request", 400)
        return
    }

    id     := strings.TrimSpace(req.ID)
    status := strings.ToLower(req.Status)

    if id == "" || (status != StatusApproved && status != StatusRejected) {
        http.Error(w, "invalid_params", 400)
        return
    }

    resolvedBy := r.Header.Get("X-Resolved-By")

    e, err := h.store.Resolve(id, status, resolvedBy)
    if err != nil {
        http.Error(w, `{"error":"`+err.Error()+`"}`, 400)
        return
    }

    // Fire legacy webhook
    go h.notifier.Send(e.WebhookURL, "", e)

    // (Optional) Signal unified Notifier about resolution
    // h.unified.SendResolution(...)

    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(map[string]interface{}{
        "ok":        true,
        "id":        e.ID,
        "status":    status,
        "agent_id":  e.AgentID,
        "tool_name": e.ToolName,
    })
}
