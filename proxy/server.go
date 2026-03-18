package proxy

import (
    "fmt"
    "log"
    "net/http"
    "time"

    "github.com/nascentist/agentguard/audit"
    "github.com/nascentist/agentguard/config"
    "github.com/nascentist/agentguard/health"
    "github.com/nascentist/agentguard/memory"
    "github.com/nascentist/agentguard/policy"
    "github.com/nascentist/agentguard/spend"
    "github.com/nascentist/agentguard/escalation"
    "github.com/nascentist/agentguard/notify"
    "github.com/nascentist/agentguard/auth"
)

// StartServer builds all components and starts the HTTP server.
// This is called once from main.go.
func StartServer(cfg *config.Config) error {
    // Policy engine
    engine, err := policy.NewEngine(cfg.PolicyFile)
    if err != nil {
        return fmt.Errorf("server: policy engine: %w", err)
    }
    log.Printf("[agentguard] Policy loaded from %s", cfg.PolicyFile)

    // Rate limiter
    rateLimiter := NewRateLimiter()
    rateLimiter.UpdateFromPolicy(engine.GetAgents())
    log.Printf("[agentguard] Rate limiter initialized")


    // Audit logger
    logger, err := audit.NewLogger(cfg.SQLitePath, cfg.SupabaseURL, cfg.SupabaseKey)
    if err != nil {
        return fmt.Errorf("server: audit logger: %w", err)
    }
    log.Printf("[agentguard] Audit log: %s", cfg.SQLitePath)

    // Memory client (optional)
    mem := memory.NewClient(cfg.NascentistMemURL, cfg.NascentistKey)

    // Spend tracker
    spendTracker, err := spend.NewTracker(logger.DB())
    if err != nil {
        return fmt.Errorf("server: spend tracker: %w", err)
    }
    log.Printf("[agentguard] Spend tracker initialized")

    // Agent Auth Store
    authStore := auth.NewAgentAuthStore(
        cfg.SupabaseURL,
        cfg.SupabaseKey,
        cfg.Env == "production", // require auth in production
    )
    log.Printf("[agentguard] Agent auth store initialized")

    // Notification channels (Slack/Discord)
    notifier := notify.NewNotifier()

    // Escalation store
    escStore, err := escalation.NewStore(logger.DB(), cfg.SupabaseURL, cfg.SupabaseKey, notifier)
    if err != nil {
        return fmt.Errorf("server: escalation store: %w", err)
    }
    log.Printf("[agentguard] Escalation store initialized")

    // Legacy Escalation notifier (keep for backward compat if any)
    escNotifier := escalation.NewNotifier(cfg.GuardBaseURL, cfg.DashboardURL)

    // Escalation HTTP handlers
    escHandler := escalation.NewHandler(escStore, escNotifier, notifier)

    // Interceptor
    interceptor, err := NewInterceptor(
        cfg.UpstreamURL, engine, logger, mem,
        escStore, escNotifier, spendTracker, rateLimiter,
        authStore,
        cfg.WebhookURL, cfg.WebhookSecret,
    )
    if err != nil {
        return fmt.Errorf("server: interceptor: %w", err)
    }

    // Routes
    mux := http.NewServeMux()

    // Setup escalation routing handlers
    escHandler.RegisterRoutes(mux)

    // Health check — does not go through interceptor
    mux.HandleFunc("/health", health.Handler(cfg, engine))

    // Policy reload — handled by reload.go
    reloadHandler := NewReloadHandler(
        engine,
        cfg.PolicyFile,
        cfg.AdminKey,
        cfg.SupabaseURL,
        cfg.SupabaseKey,
        authStore, // Added authStore as per instruction
    )
    mux.Handle("/reload-policy", reloadHandler)

    // Status handler for dashboard health check
    statusHandler := NewStatusHandler(
        engine, escStore, spendTracker,
        cfg.AdminKey,
        cfg.UpstreamURL,
        cfg.PolicyFile,
        cfg.Env,
    )
    mux.Handle("/api/v1/status", statusHandler)

    // All other requests go through the interceptor
    mux.Handle("/", interceptor)

    handler := Chain(
        mux,
        RecoveryMiddleware,
        TimeoutMiddleware(45*time.Second),
        RequestIDMiddleware,
        CORSMiddleware,
    )

    server := &http.Server{
        Addr:         ":" + cfg.Port,
        Handler:      handler,
        ReadTimeout:  30 * time.Second,
        WriteTimeout: 60 * time.Second,
        IdleTimeout:  120 * time.Second,
    }

    log.Printf("[agentguard] Listening on :%s → upstream: %s",
        cfg.Port, cfg.UpstreamURL)
    return RunWithGracefulShutdown(server, logger)
}
