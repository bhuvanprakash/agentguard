// proxy/middleware.go
// Production-grade HTTP middleware for AgentGuard.
//
// Middleware chain (applied in order):
//   1. RecoveryMiddleware   — catches panics, returns 500
//   2. TimeoutMiddleware    — kills slow upstream calls
//   3. RequestIDMiddleware  — attaches request ID to context
//   4. LoggingMiddleware    — structured request logging
//   5. CORSMiddleware       — for browser-based agents
//
// Usage in server.go:
//   handler := Chain(
//     mux,
//     RecoveryMiddleware,
//     TimeoutMiddleware(30*time.Second),
//     RequestIDMiddleware,
//     CORSMiddleware,
//   )

package proxy

import (
    "context"
    "encoding/json"
    "fmt"
    "log/slog"
    "net/http"
    "runtime/debug"
    "time"

    "github.com/google/uuid"
)

type contextKey string

const CtxRequestID contextKey = "request_id"

// Middleware is a standard http.Handler wrapper.
type Middleware func(http.Handler) http.Handler

// Chain applies middlewares in order (first = outermost).
func Chain(h http.Handler, middlewares ...Middleware) http.Handler {
    for i := len(middlewares) - 1; i >= 0; i-- {
        h = middlewares[i](h)
    }
    return h
}

// RecoveryMiddleware catches panics and returns a structured
// 500 JSON error instead of crashing the server.
func RecoveryMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(
        w http.ResponseWriter, r *http.Request,
    ) {
        defer func() {
            if rec := recover(); rec != nil {
                stack := debug.Stack()
                slog.Error("panic recovered",
                    "panic", fmt.Sprintf("%v", rec),
                    "stack", string(stack),
                    "method", r.Method,
                    "path", r.URL.Path,
                )
                w.Header().Set("Content-Type", "application/json")
                w.WriteHeader(http.StatusInternalServerError)
                json.NewEncoder(w).Encode(map[string]interface{}{
                    "error": map[string]interface{}{
                        "code":    -32603,
                        "message": "Internal server error",
                    },
                })
            }
        }()
        next.ServeHTTP(w, r)
    })
}

// TimeoutMiddleware cancels the request context after d.
// Prevents slow upstream APIs from blocking proxy goroutines.
func TimeoutMiddleware(d time.Duration) Middleware {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(
            w http.ResponseWriter, r *http.Request,
        ) {
            ctx, cancel := context.WithTimeout(r.Context(), d)
            defer cancel()
            next.ServeHTTP(w, r.WithContext(ctx))
        })
    }
}

// RequestIDMiddleware attaches a unique request ID to the
// request context and adds it to the response header.
func RequestIDMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(
        w http.ResponseWriter, r *http.Request,
    ) {
        id := r.Header.Get("X-Request-ID")
        if id == "" {
            id = uuid.New().String()
        }
        ctx := context.WithValue(r.Context(), CtxRequestID, id)
        w.Header().Set("X-Request-ID", id)
        next.ServeHTTP(w, r.WithContext(ctx))
    })
}

// CORSMiddleware adds permissive CORS headers.
// Required when browser-based agent UIs call AgentGuard directly.
func CORSMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(
        w http.ResponseWriter, r *http.Request,
    ) {
        w.Header().Set("Access-Control-Allow-Origin",  "*")
        w.Header().Set("Access-Control-Allow-Methods",
            "GET, POST, PUT, DELETE, OPTIONS")
        w.Header().Set("Access-Control-Allow-Headers",
            "Content-Type, Authorization, "+
                "X-Agent-ID, X-AgentGuard-Admin-Key, "+
                "X-Request-ID")

        if r.Method == http.MethodOptions {
            w.WriteHeader(http.StatusNoContent)
            return
        }
        next.ServeHTTP(w, r)
    })
}
