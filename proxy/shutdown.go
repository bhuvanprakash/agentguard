// proxy/shutdown.go
// Graceful shutdown for AgentGuard proxy.
//
// On SIGTERM or SIGINT:
//   1. Stop accepting new connections
//   2. Wait up to 30s for in-flight requests to complete
//   3. Flush SQLite audit log to Supabase
//   4. Exit cleanly
//
// This prevents data loss when deploying new versions.

package proxy

import (
    "context"
    "log/slog"
    "net/http"
    "os"
    "os/signal"
    "syscall"
    "time"

    "github.com/nascentist/agentguard/audit"
)

// RunWithGracefulShutdown wraps http.Server.ListenAndServe
// with SIGTERM/SIGINT handling and a shutdown deadline.
func RunWithGracefulShutdown(
    srv    *http.Server,
    logger *audit.Logger,
) error {
    // Start serving in background
    errCh := make(chan error, 1)
    go func() {
        slog.Info("AgentGuard listening", "addr", srv.Addr)
        if err := srv.ListenAndServe(); err != nil &&
            err != http.ErrServerClosed {
            errCh <- err
        }
        close(errCh)
    }()

    // Wait for signal or server error
    quit := make(chan os.Signal, 1)
    signal.Notify(quit, syscall.SIGTERM, syscall.SIGINT)

    select {
    case err := <-errCh:
        return err

    case sig := <-quit:
        slog.Info("shutdown signal received",
            "signal", sig.String())

        // 30 second shutdown window
        ctx, cancel := context.WithTimeout(
            context.Background(), 30*time.Second,
        )
        defer cancel()

        slog.Info("stopping server, draining connections...")
        if err := srv.Shutdown(ctx); err != nil {
            slog.Error("shutdown error", "err", err)
        }

        // Final Supabase sync before exit
        slog.Info("flushing audit log to Supabase...")
        flushCtx, flushCancel := context.WithTimeout(
            context.Background(), 15*time.Second,
        )
        defer flushCancel()

        if err := logger.FlushSync(flushCtx); err != nil {
            slog.Warn("flush warning", "err", err)
        }

        slog.Info("AgentGuard stopped cleanly")
        return nil
    }
}
