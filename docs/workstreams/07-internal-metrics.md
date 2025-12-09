# WS7: Internal Metrics

**Priority:** P2 (Medium Priority)
**Estimated Effort:** 3-4 days
**Dependencies:** WS3 (HTTP API Handlers)

## Overview

Implement comprehensive internal metrics using Prometheus to monitor Luminate's own performance, health, and resource utilization. This enables observability of the observability system itself and provides critical operational insights.

## Objectives

1. Expose Prometheus metrics at `/metrics` endpoint
2. Track request patterns (rate, latency, errors)
3. Monitor storage backend performance
4. Track rate limiting and quota enforcement
5. Measure cardinality growth
6. Provide ready-to-use Grafana dashboards

## Work Items

### 1. Prometheus Client Setup

#### 1.1 Dependencies

```go
// go.mod additions
require (
    github.com/prometheus/client_golang v1.17.0
)
```

#### 1.2 Metrics Registry

**File:** `pkg/metrics/registry.go`

```go
package metrics

import (
    "github.com/prometheus/client_golang/prometheus"
    "github.com/prometheus/client_golang/prometheus/promauto"
)

// Registry holds all Prometheus metrics for Luminate
type Registry struct {
    // HTTP Metrics
    HTTPRequestsTotal       *prometheus.CounterVec
    HTTPRequestDuration     *prometheus.HistogramVec
    HTTPRequestSize         *prometheus.HistogramVec
    HTTPResponseSize        *prometheus.HistogramVec
    HTTPActiveRequests      *prometheus.GaugeVec

    // Storage Metrics
    StorageWritesTotal      *prometheus.CounterVec
    StorageWriteDuration    *prometheus.HistogramVec
    StorageWriteBatchSize   *prometheus.HistogramVec
    StorageQueriesTotal     *prometheus.CounterVec
    StorageQueryDuration    *prometheus.HistogramVec
    StorageErrors           *prometheus.CounterVec
    StorageSize             *prometheus.GaugeVec
    StorageCardinality      *prometheus.GaugeVec

    // Rate Limiting Metrics
    RateLimitHits           *prometheus.CounterVec
    RateLimitAllowed        *prometheus.CounterVec
    RateLimitRejected       *prometheus.CounterVec
    RateLimitTokensAvailable *prometheus.GaugeVec

    // Validation Metrics
    ValidationErrors        *prometheus.CounterVec
    MetricsDropped          *prometheus.CounterVec

    // Tenant Metrics
    TenantRequests          *prometheus.CounterVec
    TenantDataSize          *prometheus.GaugeVec
    TenantCardinality       *prometheus.GaugeVec

    // System Metrics
    GoRoutines              prometheus.Gauge
    MemoryUsage             *prometheus.GaugeVec
}

// NewRegistry creates and registers all Prometheus metrics
func NewRegistry() *Registry {
    reg := &Registry{
        // HTTP request metrics
        HTTPRequestsTotal: promauto.NewCounterVec(
            prometheus.CounterOpts{
                Name: "luminate_http_requests_total",
                Help: "Total number of HTTP requests",
            },
            []string{"method", "path", "status", "tenant_id"},
        ),

        HTTPRequestDuration: promauto.NewHistogramVec(
            prometheus.HistogramOpts{
                Name:    "luminate_http_request_duration_seconds",
                Help:    "HTTP request latency distributions",
                Buckets: []float64{.001, .005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10},
            },
            []string{"method", "path", "tenant_id"},
        ),

        HTTPRequestSize: promauto.NewHistogramVec(
            prometheus.HistogramOpts{
                Name:    "luminate_http_request_size_bytes",
                Help:    "HTTP request size in bytes",
                Buckets: prometheus.ExponentialBuckets(100, 10, 8), // 100B to 10MB
            },
            []string{"method", "path"},
        ),

        HTTPResponseSize: promauto.NewHistogramVec(
            prometheus.HistogramOpts{
                Name:    "luminate_http_response_size_bytes",
                Help:    "HTTP response size in bytes",
                Buckets: prometheus.ExponentialBuckets(100, 10, 8),
            },
            []string{"method", "path"},
        ),

        HTTPActiveRequests: promauto.NewGaugeVec(
            prometheus.GaugeOpts{
                Name: "luminate_http_active_requests",
                Help: "Number of active HTTP requests",
            },
            []string{"method", "path"},
        ),

        // Storage write metrics
        StorageWritesTotal: promauto.NewCounterVec(
            prometheus.CounterOpts{
                Name: "luminate_storage_writes_total",
                Help: "Total number of write operations",
            },
            []string{"backend", "tenant_id", "status"},
        ),

        StorageWriteDuration: promauto.NewHistogramVec(
            prometheus.HistogramOpts{
                Name:    "luminate_storage_write_duration_seconds",
                Help:    "Storage write operation duration",
                Buckets: []float64{.0001, .0005, .001, .005, .01, .05, .1, .5, 1},
            },
            []string{"backend"},
        ),

        StorageWriteBatchSize: promauto.NewHistogramVec(
            prometheus.HistogramOpts{
                Name:    "luminate_storage_write_batch_size",
                Help:    "Number of metrics in each write batch",
                Buckets: []float64{1, 10, 50, 100, 500, 1000, 5000, 10000},
            },
            []string{"backend"},
        ),

        // Storage query metrics
        StorageQueriesTotal: promauto.NewCounterVec(
            prometheus.CounterOpts{
                Name: "luminate_storage_queries_total",
                Help: "Total number of query operations",
            },
            []string{"backend", "query_type", "tenant_id", "status"},
        ),

        StorageQueryDuration: promauto.NewHistogramVec(
            prometheus.HistogramOpts{
                Name:    "luminate_storage_query_duration_seconds",
                Help:    "Storage query operation duration",
                Buckets: []float64{.001, .005, .01, .025, .05, .1, .25, .5, 1, 2, 5},
            },
            []string{"backend", "query_type"},
        ),

        StorageErrors: promauto.NewCounterVec(
            prometheus.CounterOpts{
                Name: "luminate_storage_errors_total",
                Help: "Total number of storage errors",
            },
            []string{"backend", "operation", "error_type"},
        ),

        StorageSize: promauto.NewGaugeVec(
            prometheus.GaugeOpts{
                Name: "luminate_storage_size_bytes",
                Help: "Total storage size in bytes",
            },
            []string{"backend", "metric_name"},
        ),

        StorageCardinality: promauto.NewGaugeVec(
            prometheus.GaugeOpts{
                Name: "luminate_storage_cardinality",
                Help: "Number of unique metric series",
            },
            []string{"backend", "metric_name"},
        ),

        // Rate limiting metrics
        RateLimitHits: promauto.NewCounterVec(
            prometheus.CounterOpts{
                Name: "luminate_rate_limit_hits_total",
                Help: "Total number of rate limit checks",
            },
            []string{"tenant_id", "limit_type"},
        ),

        RateLimitAllowed: promauto.NewCounterVec(
            prometheus.CounterOpts{
                Name: "luminate_rate_limit_allowed_total",
                Help: "Total number of requests allowed by rate limiter",
            },
            []string{"tenant_id", "limit_type"},
        ),

        RateLimitRejected: promauto.NewCounterVec(
            prometheus.CounterOpts{
                Name: "luminate_rate_limit_rejected_total",
                Help: "Total number of requests rejected by rate limiter",
            },
            []string{"tenant_id", "limit_type"},
        ),

        RateLimitTokensAvailable: promauto.NewGaugeVec(
            prometheus.GaugeOpts{
                Name: "luminate_rate_limit_tokens_available",
                Help: "Number of tokens currently available in rate limiter",
            },
            []string{"tenant_id", "limit_type"},
        ),

        // Validation metrics
        ValidationErrors: promauto.NewCounterVec(
            prometheus.CounterOpts{
                Name: "luminate_validation_errors_total",
                Help: "Total number of validation errors",
            },
            []string{"error_type", "tenant_id"},
        ),

        MetricsDropped: promauto.NewCounterVec(
            prometheus.CounterOpts{
                Name: "luminate_metrics_dropped_total",
                Help: "Total number of metrics dropped",
            },
            []string{"reason", "tenant_id"},
        ),

        // Tenant metrics
        TenantRequests: promauto.NewCounterVec(
            prometheus.CounterOpts{
                Name: "luminate_tenant_requests_total",
                Help: "Total requests per tenant",
            },
            []string{"tenant_id", "operation"},
        ),

        TenantDataSize: promauto.NewGaugeVec(
            prometheus.GaugeOpts{
                Name: "luminate_tenant_data_size_bytes",
                Help: "Data size per tenant in bytes",
            },
            []string{"tenant_id"},
        ),

        TenantCardinality: promauto.NewGaugeVec(
            prometheus.GaugeOpts{
                Name: "luminate_tenant_cardinality",
                Help: "Number of unique series per tenant",
            },
            []string{"tenant_id"},
        ),

        // System metrics
        GoRoutines: promauto.NewGauge(
            prometheus.GaugeOpts{
                Name: "luminate_goroutines",
                Help: "Number of goroutines",
            },
        ),

        MemoryUsage: promauto.NewGaugeVec(
            prometheus.GaugeOpts{
                Name: "luminate_memory_usage_bytes",
                Help: "Memory usage in bytes",
            },
            []string{"type"}, // heap_alloc, heap_sys, stack_sys, etc.
        ),
    }

    return reg
}
```

### 2. Metrics Middleware

#### 2.1 HTTP Metrics Middleware

**File:** `pkg/middleware/metrics.go`

```go
package middleware

import (
    "net/http"
    "strconv"
    "time"

    "github.com/yourusername/luminate/pkg/auth"
    "github.com/yourusername/luminate/pkg/metrics"
)

// MetricsMiddleware tracks HTTP request metrics
func MetricsMiddleware(reg *metrics.Registry) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            start := time.Now()

            // Track request size
            if r.ContentLength > 0 {
                reg.HTTPRequestSize.WithLabelValues(
                    r.Method,
                    r.URL.Path,
                ).Observe(float64(r.ContentLength))
            }

            // Track active requests
            reg.HTTPActiveRequests.WithLabelValues(
                r.Method,
                r.URL.Path,
            ).Inc()
            defer reg.HTTPActiveRequests.WithLabelValues(
                r.Method,
                r.URL.Path,
            ).Dec()

            // Wrap response writer to capture status code and size
            wrapped := &metricsResponseWriter{
                ResponseWriter: w,
                statusCode:     http.StatusOK,
                bytesWritten:   0,
            }

            // Process request
            next.ServeHTTP(wrapped, r)

            // Record metrics
            duration := time.Since(start).Seconds()
            tenantID := getTenantID(r)

            reg.HTTPRequestsTotal.WithLabelValues(
                r.Method,
                r.URL.Path,
                strconv.Itoa(wrapped.statusCode),
                tenantID,
            ).Inc()

            reg.HTTPRequestDuration.WithLabelValues(
                r.Method,
                r.URL.Path,
                tenantID,
            ).Observe(duration)

            reg.HTTPResponseSize.WithLabelValues(
                r.Method,
                r.URL.Path,
            ).Observe(float64(wrapped.bytesWritten))
        })
    }
}

// metricsResponseWriter wraps http.ResponseWriter to capture metrics
type metricsResponseWriter struct {
    http.ResponseWriter
    statusCode   int
    bytesWritten int
}

func (w *metricsResponseWriter) WriteHeader(statusCode int) {
    w.statusCode = statusCode
    w.ResponseWriter.WriteHeader(statusCode)
}

func (w *metricsResponseWriter) Write(b []byte) (int, error) {
    n, err := w.ResponseWriter.Write(b)
    w.bytesWritten += n
    return n, err
}

func getTenantID(r *http.Request) string {
    tenantID, ok := auth.GetTenantIDFromContext(r.Context())
    if !ok {
        return "unknown"
    }
    return tenantID
}
```

### 3. Storage Instrumentation

#### 3.1 Instrumented Storage Wrapper

**File:** `pkg/storage/instrumented.go`

```go
package storage

import (
    "context"
    "time"

    "github.com/yourusername/luminate/pkg/metrics"
)

// InstrumentedStore wraps a MetricsStore with Prometheus instrumentation
type InstrumentedStore struct {
    store   MetricsStore
    metrics *metrics.Registry
    backend string // "badger" or "clickhouse"
}

// NewInstrumentedStore creates a new instrumented storage backend
func NewInstrumentedStore(store MetricsStore, reg *metrics.Registry, backend string) *InstrumentedStore {
    return &InstrumentedStore{
        store:   store,
        metrics: reg,
        backend: backend,
    }
}

// Write instruments the Write operation
func (s *InstrumentedStore) Write(ctx context.Context, metrics []Metric) error {
    start := time.Now()

    // Track batch size
    s.metrics.StorageWriteBatchSize.WithLabelValues(s.backend).Observe(float64(len(metrics)))

    // Perform write
    err := s.store.Write(ctx, metrics)

    // Record metrics
    duration := time.Since(start).Seconds()
    s.metrics.StorageWriteDuration.WithLabelValues(s.backend).Observe(duration)

    status := "success"
    if err != nil {
        status = "error"
        s.metrics.StorageErrors.WithLabelValues(
            s.backend,
            "write",
            categorizeError(err),
        ).Inc()
    }

    tenantID := getTenantIDFromMetrics(metrics)
    s.metrics.StorageWritesTotal.WithLabelValues(
        s.backend,
        tenantID,
        status,
    ).Inc()

    return err
}

// QueryRange instruments the QueryRange operation
func (s *InstrumentedStore) QueryRange(ctx context.Context, req QueryRequest) ([]MetricPoint, error) {
    return s.instrumentQuery(ctx, "range", func() (interface{}, error) {
        return s.store.QueryRange(ctx, req)
    })
}

// Aggregate instruments the Aggregate operation
func (s *InstrumentedStore) Aggregate(ctx context.Context, req AggregateRequest) ([]AggregateResult, error) {
    return s.instrumentQuery(ctx, "aggregate", func() (interface{}, error) {
        return s.store.Aggregate(ctx, req)
    })
}

// Rate instruments the Rate operation
func (s *InstrumentedStore) Rate(ctx context.Context, req RateRequest) ([]RatePoint, error) {
    return s.instrumentQuery(ctx, "rate", func() (interface{}, error) {
        return s.store.Rate(ctx, req)
    })
}

// instrumentQuery is a helper to instrument query operations
func (s *InstrumentedStore) instrumentQuery(ctx context.Context, queryType string, fn func() (interface{}, error)) (interface{}, error) {
    start := time.Now()

    result, err := fn()

    duration := time.Since(start).Seconds()
    s.metrics.StorageQueryDuration.WithLabelValues(
        s.backend,
        queryType,
    ).Observe(duration)

    status := "success"
    if err != nil {
        status = "error"
        s.metrics.StorageErrors.WithLabelValues(
            s.backend,
            "query_"+queryType,
            categorizeError(err),
        ).Inc()
    }

    tenantID := "unknown" // Extract from context if available
    s.metrics.StorageQueriesTotal.WithLabelValues(
        s.backend,
        queryType,
        tenantID,
        status,
    ).Inc()

    return result, err
}

// ListMetrics instruments the ListMetrics operation
func (s *InstrumentedStore) ListMetrics(ctx context.Context) ([]string, error) {
    return s.store.ListMetrics(ctx)
}

// ListDimensionKeys instruments the ListDimensionKeys operation
func (s *InstrumentedStore) ListDimensionKeys(ctx context.Context, metricName string) ([]string, error) {
    return s.store.ListDimensionKeys(ctx, metricName)
}

// ListDimensionValues instruments the ListDimensionValues operation
func (s *InstrumentedStore) ListDimensionValues(ctx context.Context, metricName, dimensionKey string, limit int) ([]string, error) {
    return s.store.ListDimensionValues(ctx, metricName, dimensionKey, limit)
}

// DeleteBefore instruments the DeleteBefore operation
func (s *InstrumentedStore) DeleteBefore(ctx context.Context, metricName string, before time.Time) error {
    return s.store.DeleteBefore(ctx, metricName, before)
}

// Health instruments the Health operation
func (s *InstrumentedStore) Health(ctx context.Context) error {
    return s.store.Health(ctx)
}

// Close instruments the Close operation
func (s *InstrumentedStore) Close() error {
    return s.store.Close()
}

// Helper functions
func categorizeError(err error) string {
    // Categorize errors for better metrics
    if err == nil {
        return "none"
    }

    switch {
    case err == context.DeadlineExceeded:
        return "timeout"
    case err == context.Canceled:
        return "canceled"
    default:
        return "internal"
    }
}

func getTenantIDFromMetrics(metrics []Metric) string {
    if len(metrics) == 0 {
        return "unknown"
    }
    if tenantID, ok := metrics[0].Dimensions["_tenant_id"]; ok {
        return tenantID
    }
    return "unknown"
}
```

### 4. Rate Limiter Instrumentation

**File:** `pkg/ratelimit/instrumented.go`

```go
package ratelimit

import (
    "github.com/yourusername/luminate/pkg/metrics"
)

// InstrumentedRateLimiter wraps a rate limiter with Prometheus metrics
type InstrumentedRateLimiter struct {
    limiter   *RateLimiter
    metrics   *metrics.Registry
    tenantID  string
    limitType string
}

// NewInstrumentedRateLimiter creates a new instrumented rate limiter
func NewInstrumentedRateLimiter(limiter *RateLimiter, reg *metrics.Registry, tenantID, limitType string) *InstrumentedRateLimiter {
    return &InstrumentedRateLimiter{
        limiter:   limiter,
        metrics:   reg,
        tenantID:  tenantID,
        limitType: limitType,
    }
}

// Allow checks if the request is allowed and records metrics
func (rl *InstrumentedRateLimiter) Allow(n float64) bool {
    rl.metrics.RateLimitHits.WithLabelValues(
        rl.tenantID,
        rl.limitType,
    ).Inc()

    allowed := rl.limiter.Allow(n)

    if allowed {
        rl.metrics.RateLimitAllowed.WithLabelValues(
            rl.tenantID,
            rl.limitType,
        ).Inc()
    } else {
        rl.metrics.RateLimitRejected.WithLabelValues(
            rl.tenantID,
            rl.limitType,
        ).Inc()
    }

    // Update available tokens gauge
    rl.metrics.RateLimitTokensAvailable.WithLabelValues(
        rl.tenantID,
        rl.limitType,
    ).Set(rl.limiter.tokens)

    return allowed
}
```

### 5. Background Metrics Collector

**File:** `pkg/metrics/collector.go`

```go
package metrics

import (
    "context"
    "runtime"
    "time"

    "github.com/yourusername/luminate/pkg/storage"
)

// Collector periodically collects system and storage metrics
type Collector struct {
    registry *Registry
    store    storage.MetricsStore
    interval time.Duration
    done     chan struct{}
}

// NewCollector creates a new metrics collector
func NewCollector(reg *Registry, store storage.MetricsStore, interval time.Duration) *Collector {
    return &Collector{
        registry: reg,
        store:    store,
        interval: interval,
        done:     make(chan struct{}),
    }
}

// Start begins collecting metrics in the background
func (c *Collector) Start(ctx context.Context) {
    ticker := time.NewTicker(c.interval)
    defer ticker.Stop()

    for {
        select {
        case <-ticker.C:
            c.collect(ctx)
        case <-c.done:
            return
        case <-ctx.Done():
            return
        }
    }
}

// Stop stops the background collector
func (c *Collector) Stop() {
    close(c.done)
}

// collect gathers all metrics
func (c *Collector) collect(ctx context.Context) {
    c.collectSystemMetrics()
    c.collectStorageMetrics(ctx)
}

// collectSystemMetrics gathers Go runtime metrics
func (c *Collector) collectSystemMetrics() {
    // Goroutines
    c.registry.GoRoutines.Set(float64(runtime.NumGoroutine()))

    // Memory stats
    var memStats runtime.MemStats
    runtime.ReadMemStats(&memStats)

    c.registry.MemoryUsage.WithLabelValues("heap_alloc").Set(float64(memStats.HeapAlloc))
    c.registry.MemoryUsage.WithLabelValues("heap_sys").Set(float64(memStats.HeapSys))
    c.registry.MemoryUsage.WithLabelValues("heap_idle").Set(float64(memStats.HeapIdle))
    c.registry.MemoryUsage.WithLabelValues("heap_inuse").Set(float64(memStats.HeapInuse))
    c.registry.MemoryUsage.WithLabelValues("stack_sys").Set(float64(memStats.StackSys))
    c.registry.MemoryUsage.WithLabelValues("mallocs").Set(float64(memStats.Mallocs))
    c.registry.MemoryUsage.WithLabelValues("frees").Set(float64(memStats.Frees))
}

// collectStorageMetrics gathers storage-specific metrics
func (c *Collector) collectStorageMetrics(ctx context.Context) {
    // List all metrics
    metrics, err := c.store.ListMetrics(ctx)
    if err != nil {
        return
    }

    // For each metric, estimate cardinality
    for _, metricName := range metrics {
        // This would require a custom storage method to get cardinality
        // For now, we'll skip or implement in storage backends
    }
}
```

### 6. Metrics HTTP Handler

**File:** `pkg/api/metrics_handler.go`

```go
package api

import (
    "net/http"

    "github.com/prometheus/client_golang/prometheus/promhttp"
)

// MetricsHandler returns the Prometheus metrics HTTP handler
func MetricsHandler() http.Handler {
    return promhttp.Handler()
}
```

### 7. Integration into Main Server

**File:** `cmd/luminate/main.go` (additions)

```go
package main

import (
    "context"
    "log"
    "net/http"
    "time"

    "github.com/prometheus/client_golang/prometheus/promhttp"
    "github.com/yourusername/luminate/pkg/metrics"
    "github.com/yourusername/luminate/pkg/middleware"
    "github.com/yourusername/luminate/pkg/storage"
)

func main() {
    // ... existing setup ...

    // Initialize metrics registry
    metricsRegistry := metrics.NewRegistry()

    // Wrap storage with instrumentation
    instrumentedStore := storage.NewInstrumentedStore(
        store,
        metricsRegistry,
        "badger", // or "clickhouse"
    )

    // Start background metrics collector
    collector := metrics.NewCollector(metricsRegistry, instrumentedStore, 30*time.Second)
    go collector.Start(context.Background())
    defer collector.Stop()

    // Setup HTTP router
    mux := http.NewServeMux()

    // Prometheus metrics endpoint
    mux.Handle("/metrics", promhttp.Handler())

    // API endpoints (with metrics middleware)
    apiMux := http.NewServeMux()
    // ... register API handlers ...

    // Apply middleware chain
    handler := middleware.RecoveryMiddleware(
        middleware.LoggingMiddleware(
            middleware.MetricsMiddleware(metricsRegistry)(
                middleware.RateLimitMiddleware(
                    middleware.AuthMiddleware(apiMux),
                ),
            ),
        ),
    )

    mux.Handle("/api/", handler)

    // Start server
    log.Println("Starting Luminate on :8080")
    if err := http.ListenAndServe(":8080", mux); err != nil {
        log.Fatal(err)
    }
}
```

## Testing

### Unit Tests

**File:** `pkg/metrics/registry_test.go`

```go
package metrics

import (
    "testing"

    "github.com/prometheus/client_golang/prometheus"
    "github.com/prometheus/client_golang/prometheus/testutil"
)

func TestRegistry(t *testing.T) {
    reg := NewRegistry()

    // Test HTTP request counter
    reg.HTTPRequestsTotal.WithLabelValues("GET", "/api/v1/query", "200", "tenant1").Inc()
    reg.HTTPRequestsTotal.WithLabelValues("GET", "/api/v1/query", "200", "tenant1").Inc()

    count := testutil.CollectAndCount(reg.HTTPRequestsTotal)
    if count != 1 {
        t.Errorf("Expected 1 metric, got %d", count)
    }

    value := testutil.ToFloat64(reg.HTTPRequestsTotal.WithLabelValues("GET", "/api/v1/query", "200", "tenant1"))
    if value != 2.0 {
        t.Errorf("Expected counter value 2.0, got %f", value)
    }
}

func TestStorageWriteDuration(t *testing.T) {
    reg := NewRegistry()

    reg.StorageWriteDuration.WithLabelValues("badger").Observe(0.005)
    reg.StorageWriteDuration.WithLabelValues("badger").Observe(0.010)
    reg.StorageWriteDuration.WithLabelValues("badger").Observe(0.003)

    count := testutil.CollectAndCount(reg.StorageWriteDuration)
    if count != 1 {
        t.Errorf("Expected 1 metric, got %d", count)
    }
}

func TestRateLimitMetrics(t *testing.T) {
    reg := NewRegistry()

    reg.RateLimitAllowed.WithLabelValues("tenant1", "write").Inc()
    reg.RateLimitRejected.WithLabelValues("tenant1", "write").Inc()
    reg.RateLimitRejected.WithLabelValues("tenant1", "write").Inc()

    allowed := testutil.ToFloat64(reg.RateLimitAllowed.WithLabelValues("tenant1", "write"))
    rejected := testutil.ToFloat64(reg.RateLimitRejected.WithLabelValues("tenant1", "write"))

    if allowed != 1.0 {
        t.Errorf("Expected 1 allowed request, got %f", allowed)
    }
    if rejected != 2.0 {
        t.Errorf("Expected 2 rejected requests, got %f", rejected)
    }
}
```

### Integration Tests

**File:** `pkg/metrics/collector_test.go`

```go
package metrics

import (
    "context"
    "testing"
    "time"

    "github.com/yourusername/luminate/pkg/storage/badger"
)

func TestCollector(t *testing.T) {
    // Create in-memory BadgerDB
    store, err := badger.NewBadgerStore(badger.Config{
        Path:   t.TempDir(),
        Memory: true,
    })
    if err != nil {
        t.Fatal(err)
    }
    defer store.Close()

    reg := NewRegistry()
    collector := NewCollector(reg, store, 100*time.Millisecond)

    ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
    defer cancel()

    go collector.Start(ctx)

    // Wait for at least one collection cycle
    time.Sleep(150 * time.Millisecond)

    // Verify system metrics were collected
    goroutines := testutil.ToFloat64(reg.GoRoutines)
    if goroutines == 0 {
        t.Error("Expected goroutines metric to be set")
    }

    heapAlloc := testutil.ToFloat64(reg.MemoryUsage.WithLabelValues("heap_alloc"))
    if heapAlloc == 0 {
        t.Error("Expected heap_alloc metric to be set")
    }
}
```

## Grafana Dashboard

### Dashboard JSON

**File:** `deployments/grafana/luminate-overview.json`

```json
{
  "dashboard": {
    "title": "Luminate Overview",
    "uid": "luminate-overview",
    "timezone": "browser",
    "panels": [
      {
        "title": "Request Rate",
        "type": "graph",
        "targets": [
          {
            "expr": "rate(luminate_http_requests_total[5m])",
            "legendFormat": "{{method}} {{path}} ({{status}})"
          }
        ],
        "gridPos": {"x": 0, "y": 0, "w": 12, "h": 8}
      },
      {
        "title": "Request Latency (p95)",
        "type": "graph",
        "targets": [
          {
            "expr": "histogram_quantile(0.95, rate(luminate_http_request_duration_seconds_bucket[5m]))",
            "legendFormat": "{{method}} {{path}}"
          }
        ],
        "gridPos": {"x": 12, "y": 0, "w": 12, "h": 8}
      },
      {
        "title": "Write Throughput",
        "type": "graph",
        "targets": [
          {
            "expr": "rate(luminate_storage_writes_total{status=\"success\"}[5m])",
            "legendFormat": "{{backend}}"
          }
        ],
        "gridPos": {"x": 0, "y": 8, "w": 12, "h": 8}
      },
      {
        "title": "Storage Write Latency",
        "type": "graph",
        "targets": [
          {
            "expr": "histogram_quantile(0.95, rate(luminate_storage_write_duration_seconds_bucket[5m]))",
            "legendFormat": "{{backend}} p95"
          },
          {
            "expr": "histogram_quantile(0.99, rate(luminate_storage_write_duration_seconds_bucket[5m]))",
            "legendFormat": "{{backend}} p99"
          }
        ],
        "gridPos": {"x": 12, "y": 8, "w": 12, "h": 8}
      },
      {
        "title": "Rate Limit Rejections",
        "type": "graph",
        "targets": [
          {
            "expr": "rate(luminate_rate_limit_rejected_total[5m])",
            "legendFormat": "{{tenant_id}} {{limit_type}}"
          }
        ],
        "gridPos": {"x": 0, "y": 16, "w": 12, "h": 8}
      },
      {
        "title": "Storage Cardinality",
        "type": "graph",
        "targets": [
          {
            "expr": "luminate_storage_cardinality",
            "legendFormat": "{{metric_name}}"
          }
        ],
        "gridPos": {"x": 12, "y": 16, "w": 12, "h": 8}
      },
      {
        "title": "Memory Usage",
        "type": "graph",
        "targets": [
          {
            "expr": "luminate_memory_usage_bytes",
            "legendFormat": "{{type}}"
          }
        ],
        "gridPos": {"x": 0, "y": 24, "w": 12, "h": 8}
      },
      {
        "title": "Goroutines",
        "type": "graph",
        "targets": [
          {
            "expr": "luminate_goroutines",
            "legendFormat": "goroutines"
          }
        ],
        "gridPos": {"x": 12, "y": 24, "w": 12, "h": 8}
      }
    ]
  }
}
```

## Acceptance Criteria

- [ ] Prometheus `/metrics` endpoint exposes all defined metrics
- [ ] HTTP request metrics track rate, latency, and status codes
- [ ] Storage write/query operations are instrumented
- [ ] Rate limiting decisions are tracked per tenant
- [ ] Background collector reports system metrics every 30s
- [ ] Grafana dashboard displays key operational metrics
- [ ] Metrics have proper labels for filtering (tenant_id, backend, etc.)
- [ ] Histogram buckets cover expected latency ranges
- [ ] Memory usage metrics include heap, stack, and GC stats
- [ ] Unit tests verify metric registration and updates

## Performance Considerations

1. **Metric Cardinality**: Limit label combinations to avoid metric explosion
   - Max ~1000 unique tenants
   - Avoid unbounded labels (e.g., don't use metric_name as label for all metrics)

2. **Collection Overhead**: Metrics should add <1ms latency per request
   - Use pre-allocated label sets
   - Avoid string concatenation in hot paths

3. **Background Collection**: 30-second interval balances freshness and overhead
   - System metrics: Cheap to collect
   - Storage cardinality: Expensive, cache results

## Deployment

### Kubernetes ServiceMonitor

**File:** `deployments/kubernetes/servicemonitor.yaml`

```yaml
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: luminate
  namespace: luminate
spec:
  selector:
    matchLabels:
      app: luminate
  endpoints:
  - port: http
    path: /metrics
    interval: 30s
```

### Prometheus Rules

**File:** `deployments/prometheus/rules.yaml`

```yaml
groups:
- name: luminate
  interval: 30s
  rules:
  - alert: HighErrorRate
    expr: rate(luminate_http_requests_total{status=~"5.."}[5m]) > 0.05
    for: 5m
    annotations:
      summary: "High error rate detected"

  - alert: HighWriteLatency
    expr: histogram_quantile(0.95, rate(luminate_storage_write_duration_seconds_bucket[5m])) > 0.1
    for: 10m
    annotations:
      summary: "Storage write latency is high"

  - alert: RateLimitRejections
    expr: rate(luminate_rate_limit_rejected_total[5m]) > 10
    for: 5m
    annotations:
      summary: "High rate limit rejection rate for {{$labels.tenant_id}}"
```

## Summary

This workstream implements comprehensive Prometheus metrics for Luminate, enabling:
- Real-time performance monitoring
- Tenant-level observability
- Proactive alerting on anomalies
- Capacity planning insights
- Debugging production issues

**Total Estimated Effort:** 3-4 days

**Dependencies:** WS3 (HTTP API Handlers)

**Deliverables:**
- Prometheus metrics registry
- Instrumented storage and HTTP layers
- Background metrics collector
- Grafana dashboard template
- Prometheus alerting rules
- Complete test coverage
