# WS5: Rate Limiting & Validation

**Priority:** P1 (High Priority)
**Estimated Effort:** 3-4 days
**Dependencies:** WS4 (Authentication & Security)

## Overview

This workstream implements rate limiting, cardinality tracking, and input validation to protect Luminate from abuse and ensure system stability. The implementation uses token bucket algorithm for smooth rate limiting, HyperLogLog for cardinality estimation, and comprehensive validation rules to prevent malformed data.

## Architecture

```
┌─────────────────────────────────────────────────────────┐
│              Incoming HTTP Request                      │
└─────────────────────────────────────────────────────────┘
                         ↓
┌─────────────────────────────────────────────────────────┐
│           Authentication Middleware                     │
│         (Extract tenant_id from JWT)                    │
└─────────────────────────────────────────────────────────┘
                         ↓
┌─────────────────────────────────────────────────────────┐
│            Rate Limiting Middleware                     │
│  1. Check request rate (requests/second)                │
│  2. Check metric rate (metrics/second)                  │
│  3. Check concurrent queries                            │
│  4. Return 429 if limit exceeded                        │
└─────────────────────────────────────────────────────────┘
                         ↓
┌─────────────────────────────────────────────────────────┐
│          Cardinality Tracking Middleware                │
│  1. Track unique series (HyperLogLog)                   │
│  2. Check cardinality limits                            │
│  3. Warn/reject if approaching limits                   │
└─────────────────────────────────────────────────────────┘
                         ↓
┌─────────────────────────────────────────────────────────┐
│            Request Validation Middleware                │
│  1. Validate metric names, dimension keys/values        │
│  2. Check timestamp ranges                              │
│  3. Validate numeric values (NaN, Inf)                  │
│  4. Enforce dimension limits                            │
└─────────────────────────────────────────────────────────┘
                         ↓
┌─────────────────────────────────────────────────────────┐
│                   API Handlers                          │
│            (Process validated request)                  │
└─────────────────────────────────────────────────────────┘
```

## Rate Limiting Strategy

**Token Bucket Algorithm:**
- Tokens added at fixed rate (e.g., 100/second)
- Each request consumes 1 token
- If no tokens available, request rejected with 429
- Allows bursts up to bucket capacity
- Smooth rate limiting over time

**Per-Tenant Limits:**
- Write requests: 100 requests/second
- Query requests: 50 requests/second
- Metrics ingested: 10,000 metrics/second
- Concurrent queries: 10 parallel queries

## Work Items

### Work Item 1: Token Bucket Rate Limiter

**Purpose:** Implement token bucket algorithm for smooth rate limiting.

**Technical Specification:**

**Implementation:**

```go
// pkg/ratelimit/token_bucket.go
package ratelimit

import (
    "sync"
    "time"
)

// TokenBucket implements token bucket rate limiting algorithm
type TokenBucket struct {
    mu sync.Mutex

    capacity     float64   // Max tokens
    tokens       float64   // Current tokens
    refillRate   float64   // Tokens per second
    lastRefill   time.Time // Last refill timestamp
}

// NewTokenBucket creates a new token bucket
// capacity: max burst size
// refillRate: tokens added per second
func NewTokenBucket(capacity, refillRate float64) *TokenBucket {
    return &TokenBucket{
        capacity:   capacity,
        tokens:     capacity, // Start full
        refillRate: refillRate,
        lastRefill: time.Now(),
    }
}

// Allow checks if n tokens are available and consumes them
func (tb *TokenBucket) Allow(n float64) bool {
    tb.mu.Lock()
    defer tb.mu.Unlock()

    // Refill tokens based on elapsed time
    now := time.Now()
    elapsed := now.Sub(tb.lastRefill).Seconds()
    tb.tokens += elapsed * tb.refillRate

    // Cap at capacity
    if tb.tokens > tb.capacity {
        tb.tokens = tb.capacity
    }

    tb.lastRefill = now

    // Check if enough tokens available
    if tb.tokens >= n {
        tb.tokens -= n
        return true
    }

    return false
}

// AllowN is a convenience method for Allow(1)
func (tb *TokenBucket) AllowN() bool {
    return tb.Allow(1.0)
}

// Remaining returns the number of tokens currently available
func (tb *TokenBucket) Remaining() float64 {
    tb.mu.Lock()
    defer tb.mu.Unlock()

    // Calculate tokens without modifying state
    now := time.Now()
    elapsed := now.Sub(tb.lastRefill).Seconds()
    tokens := tb.tokens + (elapsed * tb.refillRate)

    if tokens > tb.capacity {
        tokens = tb.capacity
    }

    return tokens
}

// Reset resets the bucket to full capacity
func (tb *TokenBucket) Reset() {
    tb.mu.Lock()
    defer tb.mu.Unlock()

    tb.tokens = tb.capacity
    tb.lastRefill = time.Now()
}
```

**Tests:**

```go
// pkg/ratelimit/token_bucket_test.go
package ratelimit

import (
    "testing"
    "time"

    "github.com/stretchr/testify/assert"
)

func TestTokenBucket_Allow(t *testing.T) {
    // 10 tokens capacity, 5 tokens/second refill
    tb := NewTokenBucket(10, 5)

    // Initially full, should allow 10 requests
    for i := 0; i < 10; i++ {
        assert.True(t, tb.AllowN(), "request %d should be allowed", i+1)
    }

    // 11th request should be denied (empty bucket)
    assert.False(t, tb.AllowN(), "request 11 should be denied")
}

func TestTokenBucket_Refill(t *testing.T) {
    // 10 tokens capacity, 10 tokens/second refill
    tb := NewTokenBucket(10, 10)

    // Consume all tokens
    for i := 0; i < 10; i++ {
        tb.AllowN()
    }

    // Should be denied immediately
    assert.False(t, tb.AllowN())

    // Wait 1 second for refill (10 tokens should be added)
    time.Sleep(1 * time.Second)

    // Should allow 10 more requests
    for i := 0; i < 10; i++ {
        assert.True(t, tb.AllowN(), "request after refill %d should be allowed", i+1)
    }

    // Should be denied again
    assert.False(t, tb.AllowN())
}

func TestTokenBucket_PartialConsumption(t *testing.T) {
    // 100 tokens capacity, 10 tokens/second refill
    tb := NewTokenBucket(100, 10)

    // Consume 50 tokens at once
    assert.True(t, tb.Allow(50))

    // Should have ~50 tokens remaining
    remaining := tb.Remaining()
    assert.InDelta(t, 50.0, remaining, 1.0)

    // Try to consume 60 (should fail, only ~50 available)
    assert.False(t, tb.Allow(60))

    // Consume remaining 50 (should succeed)
    assert.True(t, tb.Allow(50))
}

func TestTokenBucket_Reset(t *testing.T) {
    tb := NewTokenBucket(10, 5)

    // Consume all tokens
    for i := 0; i < 10; i++ {
        tb.AllowN()
    }

    assert.False(t, tb.AllowN())

    // Reset
    tb.Reset()

    // Should allow 10 requests again
    for i := 0; i < 10; i++ {
        assert.True(t, tb.AllowN())
    }
}

func BenchmarkTokenBucket_Allow(b *testing.B) {
    tb := NewTokenBucket(1000000, 100000)

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        tb.AllowN()
    }
}
```

**Acceptance Criteria:**
- ✅ Implements token bucket algorithm correctly
- ✅ Allows bursts up to capacity
- ✅ Refills tokens at specified rate
- ✅ Thread-safe (uses mutex)
- ✅ Performs well under load (< 1μs per call)

---

### Work Item 2: Per-Tenant Rate Limiter

**Purpose:** Track rate limits per tenant with different limits for different operations.

**Implementation:**

```go
// pkg/ratelimit/tenant_limiter.go
package ratelimit

import (
    "sync"
    "time"
)

// TenantLimits defines rate limits for a tenant
type TenantLimits struct {
    WriteRequestsPerSecond float64 // Max write API calls/sec
    QueryRequestsPerSecond float64 // Max query API calls/sec
    MetricsPerSecond       float64 // Max individual metrics ingested/sec
    MaxConcurrentQueries   int     // Max parallel queries
}

// DefaultLimits returns default rate limits
func DefaultLimits() TenantLimits {
    return TenantLimits{
        WriteRequestsPerSecond: 100,
        QueryRequestsPerSecond: 50,
        MetricsPerSecond:       10000,
        MaxConcurrentQueries:   10,
    }
}

// TenantRateLimiter manages rate limits for multiple tenants
type TenantRateLimiter struct {
    mu sync.RWMutex

    // Token buckets per tenant
    writeRequests map[string]*TokenBucket
    queryRequests map[string]*TokenBucket
    metricsIngest map[string]*TokenBucket

    // Concurrent query tracking
    concurrentQueries map[string]int

    // Default limits
    defaultLimits TenantLimits

    // Custom limits per tenant (optional)
    customLimits map[string]TenantLimits
}

// NewTenantRateLimiter creates a new multi-tenant rate limiter
func NewTenantRateLimiter(defaultLimits TenantLimits) *TenantRateLimiter {
    return &TenantRateLimiter{
        writeRequests:     make(map[string]*TokenBucket),
        queryRequests:     make(map[string]*TokenBucket),
        metricsIngest:     make(map[string]*TokenBucket),
        concurrentQueries: make(map[string]int),
        defaultLimits:     defaultLimits,
        customLimits:      make(map[string]TenantLimits),
    }
}

// SetCustomLimits sets custom limits for a specific tenant
func (trl *TenantRateLimiter) SetCustomLimits(tenantID string, limits TenantLimits) {
    trl.mu.Lock()
    defer trl.mu.Unlock()

    trl.customLimits[tenantID] = limits

    // Reset existing buckets to apply new limits
    delete(trl.writeRequests, tenantID)
    delete(trl.queryRequests, tenantID)
    delete(trl.metricsIngest, tenantID)
}

// getLimits returns limits for tenant (custom or default)
func (trl *TenantRateLimiter) getLimits(tenantID string) TenantLimits {
    trl.mu.RLock()
    defer trl.mu.RUnlock()

    if limits, ok := trl.customLimits[tenantID]; ok {
        return limits
    }
    return trl.defaultLimits
}

// getOrCreateBucket gets or creates a token bucket for tenant
func (trl *TenantRateLimiter) getOrCreateBucket(tenantID string, buckets map[string]*TokenBucket, capacity, refillRate float64) *TokenBucket {
    trl.mu.Lock()
    defer trl.mu.Unlock()

    if bucket, ok := buckets[tenantID]; ok {
        return bucket
    }

    bucket := NewTokenBucket(capacity, refillRate)
    buckets[tenantID] = bucket
    return bucket
}

// AllowWriteRequest checks if tenant can make write request
func (trl *TenantRateLimiter) AllowWriteRequest(tenantID string) bool {
    limits := trl.getLimits(tenantID)
    bucket := trl.getOrCreateBucket(tenantID, trl.writeRequests, limits.WriteRequestsPerSecond*2, limits.WriteRequestsPerSecond)
    return bucket.AllowN()
}

// AllowQueryRequest checks if tenant can make query request
func (trl *TenantRateLimiter) AllowQueryRequest(tenantID string) bool {
    limits := trl.getLimits(tenantID)
    bucket := trl.getOrCreateBucket(tenantID, trl.queryRequests, limits.QueryRequestsPerSecond*2, limits.QueryRequestsPerSecond)
    return bucket.AllowN()
}

// AllowMetrics checks if tenant can ingest n metrics
func (trl *TenantRateLimiter) AllowMetrics(tenantID string, count int) bool {
    limits := trl.getLimits(tenantID)
    bucket := trl.getOrCreateBucket(tenantID, trl.metricsIngest, limits.MetricsPerSecond*2, limits.MetricsPerSecond)
    return bucket.Allow(float64(count))
}

// AcquireQuerySlot attempts to acquire a concurrent query slot
func (trl *TenantRateLimiter) AcquireQuerySlot(tenantID string) bool {
    trl.mu.Lock()
    defer trl.mu.Unlock()

    limits := trl.getLimits(tenantID)
    current := trl.concurrentQueries[tenantID]

    if current >= limits.MaxConcurrentQueries {
        return false
    }

    trl.concurrentQueries[tenantID]++
    return true
}

// ReleaseQuerySlot releases a concurrent query slot
func (trl *TenantRateLimiter) ReleaseQuerySlot(tenantID string) {
    trl.mu.Lock()
    defer trl.mu.Unlock()

    if trl.concurrentQueries[tenantID] > 0 {
        trl.concurrentQueries[tenantID]--
    }
}

// GetStats returns rate limit statistics for a tenant
func (trl *TenantRateLimiter) GetStats(tenantID string) map[string]interface{} {
    trl.mu.RLock()
    defer trl.mu.RUnlock()

    stats := make(map[string]interface{})

    if bucket, ok := trl.writeRequests[tenantID]; ok {
        stats["write_requests_remaining"] = bucket.Remaining()
    }

    if bucket, ok := trl.queryRequests[tenantID]; ok {
        stats["query_requests_remaining"] = bucket.Remaining()
    }

    if bucket, ok := trl.metricsIngest[tenantID]; ok {
        stats["metrics_ingest_remaining"] = bucket.Remaining()
    }

    stats["concurrent_queries"] = trl.concurrentQueries[tenantID]

    return stats
}

// Cleanup removes inactive tenant buckets (call periodically)
func (trl *TenantRateLimiter) Cleanup(maxIdleTime time.Duration) {
    trl.mu.Lock()
    defer trl.mu.Unlock()

    now := time.Now()

    // Helper to check if bucket is idle
    isIdle := func(bucket *TokenBucket) bool {
        return now.Sub(bucket.lastRefill) > maxIdleTime
    }

    // Clean up write request buckets
    for tenantID, bucket := range trl.writeRequests {
        if isIdle(bucket) {
            delete(trl.writeRequests, tenantID)
        }
    }

    // Clean up query request buckets
    for tenantID, bucket := range trl.queryRequests {
        if isIdle(bucket) {
            delete(trl.queryRequests, tenantID)
        }
    }

    // Clean up metrics ingest buckets
    for tenantID, bucket := range trl.metricsIngest {
        if isIdle(bucket) {
            delete(trl.metricsIngest, tenantID)
        }
    }
}
```

**Tests:**

```go
// pkg/ratelimit/tenant_limiter_test.go
package ratelimit

import (
    "testing"
    "time"

    "github.com/stretchr/testify/assert"
)

func TestTenantRateLimiter_WriteRequests(t *testing.T) {
    limits := TenantLimits{
        WriteRequestsPerSecond: 10,
        QueryRequestsPerSecond: 10,
        MetricsPerSecond:       100,
        MaxConcurrentQueries:   5,
    }

    limiter := NewTenantRateLimiter(limits)

    // Should allow 20 write requests (burst = 2x rate)
    for i := 0; i < 20; i++ {
        assert.True(t, limiter.AllowWriteRequest("tenant_1"))
    }

    // 21st should be denied
    assert.False(t, limiter.AllowWriteRequest("tenant_1"))

    // Different tenant should not be affected
    assert.True(t, limiter.AllowWriteRequest("tenant_2"))
}

func TestTenantRateLimiter_MetricsIngestion(t *testing.T) {
    limits := TenantLimits{
        WriteRequestsPerSecond: 100,
        QueryRequestsPerSecond: 50,
        MetricsPerSecond:       1000,
        MaxConcurrentQueries:   10,
    }

    limiter := NewTenantRateLimiter(limits)

    // Should allow 2000 metrics total (burst = 2x rate)
    assert.True(t, limiter.AllowMetrics("tenant_1", 1000))
    assert.True(t, limiter.AllowMetrics("tenant_1", 1000))

    // Next batch should be denied
    assert.False(t, limiter.AllowMetrics("tenant_1", 100))
}

func TestTenantRateLimiter_ConcurrentQueries(t *testing.T) {
    limits := TenantLimits{
        WriteRequestsPerSecond: 100,
        QueryRequestsPerSecond: 50,
        MetricsPerSecond:       10000,
        MaxConcurrentQueries:   3,
    }

    limiter := NewTenantRateLimiter(limits)

    // Should allow 3 concurrent queries
    assert.True(t, limiter.AcquireQuerySlot("tenant_1"))
    assert.True(t, limiter.AcquireQuerySlot("tenant_1"))
    assert.True(t, limiter.AcquireQuerySlot("tenant_1"))

    // 4th should be denied
    assert.False(t, limiter.AcquireQuerySlot("tenant_1"))

    // Release one slot
    limiter.ReleaseQuerySlot("tenant_1")

    // Should allow one more
    assert.True(t, limiter.AcquireQuerySlot("tenant_1"))
}

func TestTenantRateLimiter_CustomLimits(t *testing.T) {
    defaultLimits := TenantLimits{
        WriteRequestsPerSecond: 10,
        QueryRequestsPerSecond: 10,
        MetricsPerSecond:       100,
        MaxConcurrentQueries:   5,
    }

    limiter := NewTenantRateLimiter(defaultLimits)

    // Set custom limits for enterprise tenant
    enterpriseLimits := TenantLimits{
        WriteRequestsPerSecond: 1000,
        QueryRequestsPerSecond: 500,
        MetricsPerSecond:       100000,
        MaxConcurrentQueries:   50,
    }

    limiter.SetCustomLimits("tenant_enterprise", enterpriseLimits)

    // Enterprise tenant should have higher limits
    for i := 0; i < 100; i++ {
        assert.True(t, limiter.AllowWriteRequest("tenant_enterprise"))
    }

    // Regular tenant should still have default limits
    for i := 0; i < 20; i++ {
        limiter.AllowWriteRequest("tenant_regular")
    }

    assert.False(t, limiter.AllowWriteRequest("tenant_regular"))
}

func TestTenantRateLimiter_GetStats(t *testing.T) {
    limits := DefaultLimits()
    limiter := NewTenantRateLimiter(limits)

    // Make some requests
    limiter.AllowWriteRequest("tenant_1")
    limiter.AllowQueryRequest("tenant_1")
    limiter.AcquireQuerySlot("tenant_1")

    stats := limiter.GetStats("tenant_1")

    assert.NotNil(t, stats)
    assert.Contains(t, stats, "concurrent_queries")
    assert.Equal(t, 1, stats["concurrent_queries"])
}
```

**Acceptance Criteria:**
- ✅ Per-tenant rate limiting for write/query requests
- ✅ Per-tenant metrics ingestion rate limiting
- ✅ Concurrent query slot management
- ✅ Support for custom limits per tenant (e.g., enterprise tier)
- ✅ Statistics API for monitoring
- ✅ Cleanup of inactive tenants

---

### Work Item 3: Rate Limiting Middleware

**Purpose:** HTTP middleware that enforces rate limits and returns appropriate headers.

**Implementation:**

```go
// pkg/ratelimit/middleware.go
package ratelimit

import (
    "net/http"
    "strconv"
    "time"

    "github.com/yourusername/luminate/pkg/auth"
    "github.com/yourusername/luminate/pkg/models"
)

// RateLimitMiddleware enforces rate limits
type RateLimitMiddleware struct {
    limiter *TenantRateLimiter
}

// NewRateLimitMiddleware creates a new rate limiting middleware
func NewRateLimitMiddleware(limiter *TenantRateLimiter) *RateLimitMiddleware {
    return &RateLimitMiddleware{limiter: limiter}
}

// Middleware returns HTTP middleware for rate limiting
func (rlm *RateLimitMiddleware) Middleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // Get tenant ID from context (set by auth middleware)
        tenantID, ok := auth.GetTenantIDFromContext(r.Context())
        if !ok {
            // If no tenant ID, skip rate limiting (shouldn't happen after auth)
            next.ServeHTTP(w, r)
            return
        }

        // Check rate limit based on endpoint
        var allowed bool
        var remaining float64

        switch {
        case r.URL.Path == "/api/v1/write":
            allowed = rlm.limiter.AllowWriteRequest(tenantID)
            if bucket, ok := rlm.limiter.writeRequests[tenantID]; ok {
                remaining = bucket.Remaining()
            }

        case r.URL.Path == "/api/v1/query":
            allowed = rlm.limiter.AllowQueryRequest(tenantID)
            if bucket, ok := rlm.limiter.queryRequests[tenantID]; ok {
                remaining = bucket.Remaining()
            }

        default:
            // No rate limit for other endpoints
            next.ServeHTTP(w, r)
            return
        }

        // Add rate limit headers
        limits := rlm.limiter.getLimits(tenantID)
        w.Header().Set("X-RateLimit-Limit", strconv.FormatFloat(limits.WriteRequestsPerSecond, 'f', 0, 64))
        w.Header().Set("X-RateLimit-Remaining", strconv.FormatFloat(remaining, 'f', 0, 64))
        w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(time.Now().Add(1*time.Second).Unix(), 10))

        if !allowed {
            writeRateLimitError(w, tenantID)
            return
        }

        next.ServeHTTP(w, r)
    })
}

// ConcurrentQueryMiddleware manages concurrent query slots
func (rlm *RateLimitMiddleware) ConcurrentQueryMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        tenantID, ok := auth.GetTenantIDFromContext(r.Context())
        if !ok {
            next.ServeHTTP(w, r)
            return
        }

        // Try to acquire query slot
        if !rlm.limiter.AcquireQuerySlot(tenantID) {
            writeRateLimitError(w, tenantID)
            return
        }

        // Release slot when done
        defer rlm.limiter.ReleaseQuerySlot(tenantID)

        next.ServeHTTP(w, r)
    })
}

// writeRateLimitError writes a 429 Too Many Requests response
func writeRateLimitError(w http.ResponseWriter, tenantID string) {
    w.Header().Set("Content-Type", "application/json")
    w.Header().Set("Retry-After", "1") // Retry after 1 second
    w.WriteHeader(http.StatusTooManyRequests)

    response := map[string]interface{}{
        "error": map[string]interface{}{
            "code":    models.ErrCodeRateLimitExceeded,
            "message": "rate limit exceeded",
            "details": map[string]interface{}{
                "tenant_id":   tenantID,
                "retry_after": 1,
            },
        },
    }

    json.NewEncoder(w).Encode(response)
}
```

**Tests:**

```go
// pkg/ratelimit/middleware_test.go
package ratelimit

import (
    "context"
    "net/http"
    "net/http/httptest"
    "testing"

    "github.com/stretchr/testify/assert"
    "github.com/yourusername/luminate/pkg/auth"
)

func TestRateLimitMiddleware_AllowedRequests(t *testing.T) {
    limits := TenantLimits{
        WriteRequestsPerSecond: 10,
        QueryRequestsPerSecond: 10,
        MetricsPerSecond:       100,
        MaxConcurrentQueries:   5,
    }

    limiter := NewTenantRateLimiter(limits)
    middleware := NewRateLimitMiddleware(limiter)

    handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusOK)
    })

    // Set tenant ID in context
    ctx := context.WithValue(context.Background(), auth.TenantIDContextKey, "tenant_1")

    // Should allow first 20 requests (burst)
    for i := 0; i < 20; i++ {
        req := httptest.NewRequest("POST", "/api/v1/write", nil).WithContext(ctx)
        w := httptest.NewRecorder()

        middleware.Middleware(handler).ServeHTTP(w, req)

        assert.Equal(t, http.StatusOK, w.Code)
        assert.NotEmpty(t, w.Header().Get("X-RateLimit-Limit"))
        assert.NotEmpty(t, w.Header().Get("X-RateLimit-Remaining"))
    }

    // 21st should be rate limited
    req := httptest.NewRequest("POST", "/api/v1/write", nil).WithContext(ctx)
    w := httptest.NewRecorder()

    middleware.Middleware(handler).ServeHTTP(w, req)

    assert.Equal(t, http.StatusTooManyRequests, w.Code)
    assert.Equal(t, "1", w.Header().Get("Retry-After"))
}

func TestConcurrentQueryMiddleware_MaxQueries(t *testing.T) {
    limits := TenantLimits{
        WriteRequestsPerSecond: 100,
        QueryRequestsPerSecond: 50,
        MetricsPerSecond:       10000,
        MaxConcurrentQueries:   2,
    }

    limiter := NewTenantRateLimiter(limits)
    middleware := NewRateLimitMiddleware(limiter)

    handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusOK)
    })

    ctx := context.WithValue(context.Background(), auth.TenantIDContextKey, "tenant_1")

    // Simulate 2 concurrent queries (acquire without releasing)
    for i := 0; i < 2; i++ {
        limiter.AcquireQuerySlot("tenant_1")
    }

    // 3rd query should be denied
    req := httptest.NewRequest("POST", "/api/v1/query", nil).WithContext(ctx)
    w := httptest.NewRecorder()

    middleware.ConcurrentQueryMiddleware(handler).ServeHTTP(w, req)

    assert.Equal(t, http.StatusTooManyRequests, w.Code)
}
```

**Acceptance Criteria:**
- ✅ Enforces rate limits on write and query endpoints
- ✅ Returns 429 Too Many Requests when limit exceeded
- ✅ Includes rate limit headers (X-RateLimit-*)
- ✅ Includes Retry-After header
- ✅ Manages concurrent query slots
- ✅ Per-tenant rate limiting

---

### Work Item 4: Cardinality Tracking with HyperLogLog

**Purpose:** Track unique metric series to prevent cardinality explosion.

**Implementation:**

```go
// pkg/cardinality/hyperloglog.go
package cardinality

import (
    "crypto/md5"
    "encoding/binary"
    "math"
    "sync"
)

// HyperLogLog implements probabilistic cardinality estimation
// Space: ~12 KB for ±2% error
type HyperLogLog struct {
    mu sync.RWMutex

    precision uint8    // Number of bits for register index (typically 14)
    m         uint32   // Number of registers (2^precision)
    registers []uint8  // Register array
    alphaMM   float64  // Bias correction constant
}

// NewHyperLogLog creates a new HyperLogLog counter
// precision: 10-16 (higher = more accurate, more memory)
// precision=14 -> ~12 KB memory, ±2% error
func NewHyperLogLog(precision uint8) *HyperLogLog {
    if precision < 4 || precision > 18 {
        precision = 14 // Default
    }

    m := uint32(1 << precision)

    // Calculate alpha constant for bias correction
    var alphaMM float64
    switch m {
    case 16:
        alphaMM = 0.673 * float64(m) * float64(m)
    case 32:
        alphaMM = 0.697 * float64(m) * float64(m)
    case 64:
        alphaMM = 0.709 * float64(m) * float64(m)
    default:
        alphaMM = (0.7213 / (1 + 1.079/float64(m))) * float64(m) * float64(m)
    }

    return &HyperLogLog{
        precision: precision,
        m:         m,
        registers: make([]uint8, m),
        alphaMM:   alphaMM,
    }
}

// Add adds an element to the set
func (hll *HyperLogLog) Add(value string) {
    hll.mu.Lock()
    defer hll.mu.Unlock()

    // Hash the value
    hash := md5.Sum([]byte(value))
    hashInt := binary.BigEndian.Uint64(hash[:8])

    // Extract register index (first p bits)
    j := hashInt >> (64 - hll.precision)

    // Extract remaining bits
    w := hashInt << hll.precision

    // Count leading zeros + 1
    rho := uint8(1)
    for w&(1<<63) == 0 && rho <= 64-hll.precision {
        rho++
        w <<= 1
    }

    // Update register (take maximum)
    if rho > hll.registers[j] {
        hll.registers[j] = rho
    }
}

// Count estimates the cardinality
func (hll *HyperLogLog) Count() uint64 {
    hll.mu.RLock()
    defer hll.mu.RUnlock()

    // Calculate raw estimate
    var sum float64
    for _, val := range hll.registers {
        sum += 1.0 / math.Pow(2.0, float64(val))
    }

    estimate := hll.alphaMM / sum

    // Apply bias correction for small and large estimates
    if estimate <= 2.5*float64(hll.m) {
        // Small range correction
        zeros := 0
        for _, val := range hll.registers {
            if val == 0 {
                zeros++
            }
        }
        if zeros != 0 {
            estimate = float64(hll.m) * math.Log(float64(hll.m)/float64(zeros))
        }
    } else if estimate > (1.0/30.0)*math.Pow(2.0, 32) {
        // Large range correction
        estimate = -math.Pow(2.0, 32) * math.Log(1.0-estimate/math.Pow(2.0, 32))
    }

    return uint64(estimate)
}

// Merge merges another HyperLogLog into this one
func (hll *HyperLogLog) Merge(other *HyperLogLog) error {
    if hll.precision != other.precision {
        return errors.New("cannot merge HyperLogLogs with different precision")
    }

    hll.mu.Lock()
    defer hll.mu.Unlock()

    other.mu.RLock()
    defer other.mu.RUnlock()

    for i, val := range other.registers {
        if val > hll.registers[i] {
            hll.registers[i] = val
        }
    }

    return nil
}

// Reset clears all registers
func (hll *HyperLogLog) Reset() {
    hll.mu.Lock()
    defer hll.mu.Unlock()

    for i := range hll.registers {
        hll.registers[i] = 0
    }
}
```

**Cardinality Tracker:**

```go
// pkg/cardinality/tracker.go
package cardinality

import (
    "fmt"
    "sync"
    "time"
)

// CardinalityTracker tracks unique series per metric
type CardinalityTracker struct {
    mu sync.RWMutex

    // HyperLogLog counters per metric
    counters map[string]*HyperLogLog

    // Cardinality limits
    maxSeriesPerMetric uint64
    maxMetrics         int

    // Warnings
    warningThreshold float64 // Warn at 80% of limit
}

// NewCardinalityTracker creates a new cardinality tracker
func NewCardinalityTracker(maxSeriesPerMetric uint64, maxMetrics int) *CardinalityTracker {
    return &CardinalityTracker{
        counters:           make(map[string]*HyperLogLog),
        maxSeriesPerMetric: maxSeriesPerMetric,
        maxMetrics:         maxMetrics,
        warningThreshold:   0.8,
    }
}

// Track tracks a metric series (metric_name + dimensions hash)
func (ct *CardinalityTracker) Track(metricName string, dimensionsHash string) error {
    ct.mu.Lock()
    defer ct.mu.Unlock()

    // Check metric count limit
    if len(ct.counters) >= ct.maxMetrics {
        if _, exists := ct.counters[metricName]; !exists {
            return fmt.Errorf("max metrics limit reached: %d", ct.maxMetrics)
        }
    }

    // Get or create HyperLogLog for this metric
    hll, ok := ct.counters[metricName]
    if !ok {
        hll = NewHyperLogLog(14) // ~12 KB, ±2% error
        ct.counters[metricName] = hll
    }

    // Add series to HyperLogLog
    hll.Add(dimensionsHash)

    // Check cardinality limit
    cardinality := hll.Count()
    if cardinality > ct.maxSeriesPerMetric {
        return fmt.Errorf("cardinality limit exceeded for metric %s: %d > %d",
            metricName, cardinality, ct.maxSeriesPerMetric)
    }

    // Warn if approaching limit
    if float64(cardinality) > float64(ct.maxSeriesPerMetric)*ct.warningThreshold {
        // TODO: Log warning or emit metric
    }

    return nil
}

// GetCardinality returns estimated cardinality for a metric
func (ct *CardinalityTracker) GetCardinality(metricName string) uint64 {
    ct.mu.RLock()
    defer ct.mu.RUnlock()

    if hll, ok := ct.counters[metricName]; ok {
        return hll.Count()
    }

    return 0
}

// GetAllCardinalities returns cardinality estimates for all metrics
func (ct *CardinalityTracker) GetAllCardinalities() map[string]uint64 {
    ct.mu.RLock()
    defer ct.mu.RUnlock()

    result := make(map[string]uint64, len(ct.counters))
    for metricName, hll := range ct.counters {
        result[metricName] = hll.Count()
    }

    return result
}

// Reset clears all cardinality data
func (ct *CardinalityTracker) Reset() {
    ct.mu.Lock()
    defer ct.mu.Unlock()

    ct.counters = make(map[string]*HyperLogLog)
}

// ResetMetric clears cardinality data for a specific metric
func (ct *CardinalityTracker) ResetMetric(metricName string) {
    ct.mu.Lock()
    defer ct.mu.Unlock()

    delete(ct.counters, metricName)
}
```

**Tests:**

```go
// pkg/cardinality/hyperloglog_test.go
package cardinality

import (
    "fmt"
    "testing"

    "github.com/stretchr/testify/assert"
)

func TestHyperLogLog_Accuracy(t *testing.T) {
    hll := NewHyperLogLog(14)

    // Add 10,000 unique values
    for i := 0; i < 10000; i++ {
        hll.Add(fmt.Sprintf("value_%d", i))
    }

    estimate := hll.Count()

    // Should be within ±2% of actual (200 error margin)
    assert.InDelta(t, 10000, estimate, 200)
}

func TestHyperLogLog_Duplicates(t *testing.T) {
    hll := NewHyperLogLog(14)

    // Add same value 1000 times
    for i := 0; i < 1000; i++ {
        hll.Add("same_value")
    }

    estimate := hll.Count()

    // Should estimate ~1 unique value
    assert.InDelta(t, 1, estimate, 1)
}

func TestCardinalityTracker_LimitEnforcement(t *testing.T) {
    tracker := NewCardinalityTracker(100, 10)

    // Track 100 unique series for metric (should succeed)
    for i := 0; i < 100; i++ {
        err := tracker.Track("cpu_usage", fmt.Sprintf("hash_%d", i))
        assert.NoError(t, err)
    }

    // Track 101st series (should fail)
    err := tracker.Track("cpu_usage", "hash_101")
    assert.Error(t, err)
    assert.Contains(t, err.Error(), "cardinality limit exceeded")
}

func TestCardinalityTracker_MetricLimit(t *testing.T) {
    tracker := NewCardinalityTracker(1000, 3)

    // Create 3 metrics (should succeed)
    tracker.Track("metric_1", "hash_1")
    tracker.Track("metric_2", "hash_2")
    tracker.Track("metric_3", "hash_3")

    // Create 4th metric (should fail)
    err := tracker.Track("metric_4", "hash_4")
    assert.Error(t, err)
    assert.Contains(t, err.Error(), "max metrics limit reached")
}

func BenchmarkHyperLogLog_Add(b *testing.B) {
    hll := NewHyperLogLog(14)

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        hll.Add(fmt.Sprintf("value_%d", i))
    }
}

func BenchmarkHyperLogLog_Count(b *testing.B) {
    hll := NewHyperLogLog(14)

    // Pre-populate
    for i := 0; i < 10000; i++ {
        hll.Add(fmt.Sprintf("value_%d", i))
    }

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        hll.Count()
    }
}
```

**Acceptance Criteria:**
- ✅ Implements HyperLogLog with ±2% accuracy
- ✅ Tracks cardinality per metric
- ✅ Enforces max series per metric limit
- ✅ Enforces max metrics per tenant limit
- ✅ Memory efficient (~12 KB per metric)
- ✅ Thread-safe
- ✅ Warns at 80% of limits

---

### Work Item 5: Input Validation

**Purpose:** Comprehensive validation of metric data before storage.

**Implementation:**

```go
// pkg/validation/validator.go
package validation

import (
    "fmt"
    "math"
    "regexp"
    "time"

    "github.com/yourusername/luminate/pkg/models"
)

var (
    // Metric name: alphanumeric + underscore, must start with letter/underscore
    metricNameRegex = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

    // Dimension key: same as metric name
    dimensionKeyRegex = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

    // Reserved dimension keys
    reservedKeys = map[string]bool{
        "_tenant_id": true,
        "_internal":  true,
    }
)

// ValidationLimits defines validation constraints
type ValidationLimits struct {
    MaxMetricNameLength       int
    MaxDimensionKeyLength     int
    MaxDimensionValueLength   int
    MaxDimensionsPerMetric    int
    TimestampMaxPastDelta     time.Duration
    TimestampMaxFutureDelta   time.Duration
}

// DefaultValidationLimits returns default validation limits
func DefaultValidationLimits() ValidationLimits {
    return ValidationLimits{
        MaxMetricNameLength:     256,
        MaxDimensionKeyLength:   128,
        MaxDimensionValueLength: 256,
        MaxDimensionsPerMetric:  20,
        TimestampMaxPastDelta:   7 * 24 * time.Hour,  // 7 days
        TimestampMaxFutureDelta: 1 * time.Hour,       // 1 hour
    }
}

// Validator validates metric data
type Validator struct {
    limits ValidationLimits
}

// NewValidator creates a new validator
func NewValidator(limits ValidationLimits) *Validator {
    return &Validator{limits: limits}
}

// ValidateMetric validates a single metric
func (v *Validator) ValidateMetric(metric *models.Metric) error {
    // Validate metric name
    if err := v.ValidateMetricName(metric.Name); err != nil {
        return err
    }

    // Validate timestamp
    if err := v.ValidateTimestamp(metric.Timestamp); err != nil {
        return err
    }

    // Validate value
    if err := v.ValidateValue(metric.Value); err != nil {
        return err
    }

    // Validate dimensions
    if err := v.ValidateDimensions(metric.Dimensions); err != nil {
        return err
    }

    return nil
}

// ValidateMetricName validates metric name format and length
func (v *Validator) ValidateMetricName(name string) error {
    if name == "" {
        return models.NewAPIError(models.ErrCodeInvalidMetricName, "metric name cannot be empty", nil)
    }

    if len(name) > v.limits.MaxMetricNameLength {
        return models.NewAPIError(models.ErrCodeInvalidMetricName,
            fmt.Sprintf("metric name too long (max %d characters)", v.limits.MaxMetricNameLength),
            map[string]interface{}{
                "metric_name": name,
                "length":      len(name),
                "max_length":  v.limits.MaxMetricNameLength,
            })
    }

    if !metricNameRegex.MatchString(name) {
        return models.NewAPIError(models.ErrCodeInvalidMetricName,
            "metric name must match pattern ^[a-zA-Z_][a-zA-Z0-9_]*$",
            map[string]interface{}{
                "metric_name": name,
                "pattern":     "^[a-zA-Z_][a-zA-Z0-9_]*$",
            })
    }

    return nil
}

// ValidateTimestamp validates timestamp is within allowed range
func (v *Validator) ValidateTimestamp(timestamp time.Time) error {
    now := time.Now()

    // Check if too old
    if timestamp.Before(now.Add(-v.limits.TimestampMaxPastDelta)) {
        return models.NewAPIError(models.ErrCodeInvalidTimestamp,
            fmt.Sprintf("timestamp too old (max %v in past)", v.limits.TimestampMaxPastDelta),
            map[string]interface{}{
                "timestamp":   timestamp,
                "max_past":    v.limits.TimestampMaxPastDelta.String(),
            })
    }

    // Check if too far in future
    if timestamp.After(now.Add(v.limits.TimestampMaxFutureDelta)) {
        return models.NewAPIError(models.ErrCodeInvalidTimestamp,
            fmt.Sprintf("timestamp in future (max %v ahead)", v.limits.TimestampMaxFutureDelta),
            map[string]interface{}{
                "timestamp":   timestamp,
                "max_future":  v.limits.TimestampMaxFutureDelta.String(),
            })
    }

    return nil
}

// ValidateValue validates metric value is finite
func (v *Validator) ValidateValue(value float64) error {
    if math.IsNaN(value) {
        return models.NewAPIError(models.ErrCodeInvalidValue, "metric value cannot be NaN", nil)
    }

    if math.IsInf(value, 0) {
        return models.NewAPIError(models.ErrCodeInvalidValue, "metric value cannot be Inf", nil)
    }

    return nil
}

// ValidateDimensions validates all dimensions
func (v *Validator) ValidateDimensions(dimensions map[string]string) error {
    if len(dimensions) > v.limits.MaxDimensionsPerMetric {
        return models.NewAPIError(models.ErrCodeTooManyDimensions,
            fmt.Sprintf("too many dimensions (max %d)", v.limits.MaxDimensionsPerMetric),
            map[string]interface{}{
                "dimension_count": len(dimensions),
                "max_dimensions":  v.limits.MaxDimensionsPerMetric,
            })
    }

    for key, value := range dimensions {
        // Validate dimension key
        if err := v.ValidateDimensionKey(key); err != nil {
            return err
        }

        // Validate dimension value
        if err := v.ValidateDimensionValue(key, value); err != nil {
            return err
        }
    }

    return nil
}

// ValidateDimensionKey validates dimension key format
func (v *Validator) ValidateDimensionKey(key string) error {
    if key == "" {
        return models.NewAPIError(models.ErrCodeInvalidDimensionKey, "dimension key cannot be empty", nil)
    }

    if len(key) > v.limits.MaxDimensionKeyLength {
        return models.NewAPIError(models.ErrCodeInvalidDimensionKey,
            fmt.Sprintf("dimension key too long (max %d characters)", v.limits.MaxDimensionKeyLength),
            map[string]interface{}{
                "dimension_key": key,
                "length":        len(key),
                "max_length":    v.limits.MaxDimensionKeyLength,
            })
    }

    if !dimensionKeyRegex.MatchString(key) {
        return models.NewAPIError(models.ErrCodeInvalidDimensionKey,
            "dimension key must match pattern ^[a-zA-Z_][a-zA-Z0-9_]*$",
            map[string]interface{}{
                "dimension_key": key,
                "pattern":       "^[a-zA-Z_][a-zA-Z0-9_]*$",
            })
    }

    if reservedKeys[key] {
        return models.NewAPIError(models.ErrCodeInvalidDimensionKey,
            fmt.Sprintf("dimension key '%s' is reserved", key),
            map[string]interface{}{"dimension_key": key})
    }

    return nil
}

// ValidateDimensionValue validates dimension value length
func (v *Validator) ValidateDimensionValue(key, value string) error {
    if value == "" {
        return models.NewAPIError(models.ErrCodeInvalidDimensionValue,
            "dimension value cannot be empty",
            map[string]interface{}{"dimension_key": key})
    }

    if len(value) > v.limits.MaxDimensionValueLength {
        return models.NewAPIError(models.ErrCodeInvalidDimensionValue,
            fmt.Sprintf("dimension value too long (max %d characters)", v.limits.MaxDimensionValueLength),
            map[string]interface{}{
                "dimension_key": key,
                "value_length":  len(value),
                "max_length":    v.limits.MaxDimensionValueLength,
            })
    }

    return nil
}
```

**Tests:**

```go
// pkg/validation/validator_test.go
package validation

import (
    "math"
    "strings"
    "testing"
    "time"

    "github.com/stretchr/testify/assert"
    "github.com/yourusername/luminate/pkg/models"
)

func TestValidateMetricName_Valid(t *testing.T) {
    validator := NewValidator(DefaultValidationLimits())

    validNames := []string{
        "cpu_usage",
        "api_latency",
        "_internal_metric",
        "metric123",
        "CamelCaseMetric",
    }

    for _, name := range validNames {
        t.Run(name, func(t *testing.T) {
            err := validator.ValidateMetricName(name)
            assert.NoError(t, err)
        })
    }
}

func TestValidateMetricName_Invalid(t *testing.T) {
    validator := NewValidator(DefaultValidationLimits())

    invalidNames := []string{
        "",                     // Empty
        "123_metric",           // Starts with number
        "metric-name",          // Contains hyphen
        "metric.name",          // Contains dot
        "metric name",          // Contains space
        strings.Repeat("a", 257), // Too long
    }

    for _, name := range invalidNames {
        t.Run(name, func(t *testing.T) {
            err := validator.ValidateMetricName(name)
            assert.Error(t, err)
        })
    }
}

func TestValidateTimestamp_Valid(t *testing.T) {
    validator := NewValidator(DefaultValidationLimits())

    validTimestamps := []time.Time{
        time.Now(),
        time.Now().Add(-1 * time.Hour),
        time.Now().Add(-24 * time.Hour),
        time.Now().Add(30 * time.Minute),
    }

    for _, ts := range validTimestamps {
        err := validator.ValidateTimestamp(ts)
        assert.NoError(t, err)
    }
}

func TestValidateTimestamp_Invalid(t *testing.T) {
    validator := NewValidator(DefaultValidationLimits())

    invalidTimestamps := []time.Time{
        time.Now().Add(-8 * 24 * time.Hour), // Too old
        time.Now().Add(2 * time.Hour),       // Too far in future
    }

    for _, ts := range invalidTimestamps {
        err := validator.ValidateTimestamp(ts)
        assert.Error(t, err)
    }
}

func TestValidateValue_Valid(t *testing.T) {
    validator := NewValidator(DefaultValidationLimits())

    validValues := []float64{
        0.0,
        100.5,
        -50.2,
        math.MaxFloat64,
        -math.MaxFloat64,
    }

    for _, val := range validValues {
        err := validator.ValidateValue(val)
        assert.NoError(t, err)
    }
}

func TestValidateValue_Invalid(t *testing.T) {
    validator := NewValidator(DefaultValidationLimits())

    invalidValues := []float64{
        math.NaN(),
        math.Inf(1),
        math.Inf(-1),
    }

    for _, val := range invalidValues {
        err := validator.ValidateValue(val)
        assert.Error(t, err)
    }
}

func TestValidateDimensions_Valid(t *testing.T) {
    validator := NewValidator(DefaultValidationLimits())

    validDimensions := map[string]string{
        "customer_id": "cust_123",
        "region":      "us-west-2",
        "endpoint":    "/api/users",
    }

    err := validator.ValidateDimensions(validDimensions)
    assert.NoError(t, err)
}

func TestValidateDimensions_TooMany(t *testing.T) {
    validator := NewValidator(DefaultValidationLimits())

    // Create 21 dimensions (max is 20)
    dimensions := make(map[string]string)
    for i := 0; i < 21; i++ {
        dimensions[fmt.Sprintf("dim_%d", i)] = "value"
    }

    err := validator.ValidateDimensions(dimensions)
    assert.Error(t, err)
}

func TestValidateDimensionKey_Reserved(t *testing.T) {
    validator := NewValidator(DefaultValidationLimits())

    err := validator.ValidateDimensionKey("_tenant_id")
    assert.Error(t, err)
    assert.Contains(t, err.Error(), "reserved")
}
```

**Acceptance Criteria:**
- ✅ Validates metric name format and length
- ✅ Validates timestamp range (7 days past, 1 hour future)
- ✅ Validates value is finite (not NaN or Inf)
- ✅ Validates dimension count (max 20)
- ✅ Validates dimension key format and length
- ✅ Validates dimension value length
- ✅ Rejects reserved dimension keys

---

## Integration with Router

```go
// pkg/api/router.go (updated with rate limiting and validation)
func NewRouter(store storage.MetricsStore, authMiddleware *auth.AuthMiddleware, rateLimiter *ratelimit.TenantRateLimiter) *chi.Mux {
    r := chi.NewRouter()

    // Global middleware
    r.Use(RecoveryMiddleware)
    r.Use(LoggingMiddleware)
    r.Use(auth.SecurityHeadersMiddleware)
    r.Use(CORSMiddleware)

    // Rate limiting middleware
    rateLimitMiddleware := ratelimit.NewRateLimitMiddleware(rateLimiter)

    // Protected API routes
    r.Route("/api/v1", func(r chi.Router) {
        r.Use(authMiddleware.Middleware) // Auth first
        r.Use(rateLimitMiddleware.Middleware) // Then rate limiting

        // Write endpoint
        r.Post("/write", writeHandler.HandleWrite)

        // Query endpoint (with concurrent query limiting)
        r.With(rateLimitMiddleware.ConcurrentQueryMiddleware).
            Post("/query", queryHandler.HandleQuery)
    })

    return r
}
```

---

## Summary

This workstream provides complete rate limiting, cardinality tracking, and validation:

1. **Token Bucket Rate Limiter**: Smooth rate limiting with burst support
2. **Per-Tenant Rate Limiting**: Separate limits for write/query requests and metrics ingestion
3. **Rate Limiting Middleware**: HTTP middleware with rate limit headers
4. **HyperLogLog Cardinality Tracking**: Memory-efficient cardinality estimation (±2% accuracy)
5. **Input Validation**: Comprehensive validation of metric names, timestamps, values, and dimensions

**Next Steps**: After completing this workstream, proceed to [WS7: Internal Metrics](07-internal-metrics.md) to add Prometheus metrics for self-monitoring, or [WS8: Health Checks & Discovery](08-health-discovery.md) for comprehensive health checks.
