package config

// Config holds all runtime configuration for AgentGuard.
// All values come from environment variables.
// Call Load() once at startup. Panic if required vars missing.
//
// ENV vars:
//   AGENTGUARD_PORT          Port to listen on (default: 7777)
//   AGENTGUARD_UPSTREAM_URL  URL to forward allowed requests to
//   AGENTGUARD_POLICY_FILE   Path to YAML policy file
//   AGENTGUARD_SQLITE_PATH   Path to SQLite audit DB (default: ./audit.db)
//   SUPABASE_URL             Your Supabase project URL
//   SUPABASE_SERVICE_KEY     Supabase service role key (for audit writes)
//   NASCENTIST_MEMORY_URL    URL of Nascentist memory API
//   NASCENTIST_INTERNAL_KEY  Internal API key for memory calls
//   AGENTGUARD_ENV           "development" or "production"

import (
    "fmt"
    "os"

    "github.com/joho/godotenv"
)

type Config struct {
    Port             string
    UpstreamURL      string
    PolicyFile       string
    SQLitePath       string
    SupabaseURL      string
    SupabaseKey      string
    NascentistMemURL string
    NascentistKey    string
    Env              string
    GuardBaseURL     string  // e.g. "http://agentguard:7777"
    DashboardURL     string  // e.g. "https://nascentist.ai"
    WebhookURL       string  // user's webhook URL (optional)
    WebhookSecret    string  // HMAC secret for webhook signing
    AdminKey         string  // key for approve/reject endpoints
}

func Load() (*Config, error) {
    // Load .env file in development (ignore error in production)
    _ = godotenv.Load()

    cfg := &Config{
        Port:             getEnv("AGENTGUARD_PORT", "7777"),
        UpstreamURL:      mustGetEnv("AGENTGUARD_UPSTREAM_URL"),
        PolicyFile:       getEnv("AGENTGUARD_POLICY_FILE", "./policy.yaml"),
        SQLitePath:       getEnv("AGENTGUARD_SQLITE_PATH", "./audit.db"),
        SupabaseURL:      mustGetEnv("SUPABASE_URL"),
        SupabaseKey:      mustGetEnv("SUPABASE_SERVICE_KEY"),
        NascentistMemURL: getEnv("NASCENTIST_MEMORY_URL", ""),
        NascentistKey:    getEnv("NASCENTIST_INTERNAL_KEY", ""),
        Env:              getEnv("AGENTGUARD_ENV", "development"),
        GuardBaseURL:     getEnv("AGENTGUARD_BASE_URL", "http://localhost:7777"),
        DashboardURL:     getEnv("AGENTGUARD_DASHBOARD_URL", "http://localhost:3000"),
        WebhookURL:       getEnv("AGENTGUARD_WEBHOOK_URL", ""),
        WebhookSecret:    getEnv("AGENTGUARD_WEBHOOK_SECRET", ""),
        AdminKey:         getEnv("AGENTGUARD_ADMIN_KEY", ""),
    }

    return cfg, nil
}

func getEnv(key, fallback string) string {
    if v := os.Getenv(key); v != "" {
        return v
    }
    return fallback
}

func mustGetEnv(key string) string {
    v := os.Getenv(key)
    if v == "" {
        panic(fmt.Sprintf("[agentguard] FATAL: required env var %s is not set", key))
    }
    return v
}
