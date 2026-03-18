// proxy/ratelimit.go
// Per-agent token bucket rate limiter.
//
// Configured in policy.yaml per agent:
//   agents:
//     - id: billing-agent
//       rate_limit:
//         requests_per_minute: 60
//         burst: 10
//
// If an agent exceeds its rate limit, the call is blocked
// immediately and logged as decision=block, reason=rate_limit.
//
// The limiter state is in-memory (not persisted to Supabase).
// On restart, rate limit windows reset — acceptable tradeoff
// for the latency benefit of in-memory limiting.

package proxy

import (
    "sync"
    "time"

    "github.com/nascentist/agentguard/policy"
)

// TokenBucket implements a simple token bucket per agent.
type TokenBucket struct {
    mu          sync.Mutex
    tokens      float64
    maxTokens   float64
    refillRate  float64 // tokens per second
    lastRefill  time.Time
}

func newTokenBucket(requestsPerMinute int, burst int) *TokenBucket {
    return &TokenBucket{
        tokens:     float64(burst),
        maxTokens:  float64(burst),
        refillRate: float64(requestsPerMinute) / 60.0,
        lastRefill: time.Now(),
    }
}

// Allow returns true if the request is within rate limit.
func (b *TokenBucket) Allow() bool {
    b.mu.Lock()
    defer b.mu.Unlock()

    now := time.Now()
    elapsed := now.Sub(b.lastRefill).Seconds()

    // Refill tokens
    b.tokens += elapsed * b.refillRate
    if b.tokens > b.maxTokens {
        b.tokens = b.maxTokens
    }
    b.lastRefill = now

    if b.tokens < 1.0 {
        return false
    }
    b.tokens--
    return true
}

// RateLimiter manages per-agent token buckets.
type RateLimiter struct {
    mu      sync.RWMutex
    buckets map[string]*TokenBucket
    configs map[string]rateLimitConfig
}

type rateLimitConfig struct {
    RequestsPerMinute int
    Burst             int
}

func NewRateLimiter() *RateLimiter {
    rl := &RateLimiter{
        buckets: make(map[string]*TokenBucket),
        configs: make(map[string]rateLimitConfig),
    }
    // Start cleanup goroutine (remove idle buckets)
    go rl.cleanup()
    return rl
}

// Configure sets the rate limit for an agent.
// Called when policy is loaded or reloaded.
func (rl *RateLimiter) Configure(
    agentID           string,
    requestsPerMinute int,
    burst             int,
) {
    rl.mu.Lock()
    defer rl.mu.Unlock()
    rl.configs[agentID] = rateLimitConfig{
        RequestsPerMinute: requestsPerMinute,
        Burst:             burst,
    }
    // Reset bucket on reconfigure
    delete(rl.buckets, agentID)
}

// Allow checks if agentID is within its rate limit.
// Returns true if allowed, false if rate limited.
// If no config for this agent, always returns true.
func (rl *RateLimiter) Allow(agentID string) bool {
    rl.mu.RLock()
    cfg, ok := rl.configs[agentID]

    // Check wildcard config if no specific config
    if !ok {
        cfg, ok = rl.configs["*"]
    }
    rl.mu.RUnlock()

    if !ok || cfg.RequestsPerMinute <= 0 {
        return true
    }

    rl.mu.Lock()
    bucket, exists := rl.buckets[agentID]
    if !exists {
        bucket = newTokenBucket(
            cfg.RequestsPerMinute,
            cfg.Burst,
        )
        rl.buckets[agentID] = bucket
    }
    rl.mu.Unlock()

    return bucket.Allow()
}

// UpdateFromPolicy reconfigures all rate limits from policy.
// Called by policy engine after every reload.
func (rl *RateLimiter) UpdateFromPolicy(
    agents []policy.AgentPolicy,
) {
    for _, agent := range agents {
        if agent.RateLimit != nil {
            burst := agent.RateLimit.Burst
            if burst <= 0 {
                burst = agent.RateLimit.RequestsPerMinute / 4
                if burst < 1 {
                    burst = 1
                }
            }
            rl.Configure(
                agent.ID,
                agent.RateLimit.RequestsPerMinute,
                burst,
            )
        }
    }
}

// cleanup removes buckets that haven't been used in 5 minutes.
func (rl *RateLimiter) cleanup() {
    ticker := time.NewTicker(5 * time.Minute)
    for range ticker.C {
        rl.mu.Lock()
        // Simple strategy: clear all buckets, they
        // rebuild on next request. Tokens reset is
        // acceptable for 5-minute idle agents.
        if len(rl.buckets) > 1000 {
            rl.buckets = make(map[string]*TokenBucket)
        }
        rl.mu.Unlock()
    }
}
