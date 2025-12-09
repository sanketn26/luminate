# WS8: Health Checks & Discovery

**Priority:** P2 (Medium Priority)
**Estimated Effort:** 2-3 days
**Dependencies:** WS1 (Storage Backend), WS3 (HTTP API Handlers)

## Overview

Implement comprehensive health check endpoints and metric discovery APIs to support Kubernetes liveness/readiness probes, service monitoring, and dynamic metric exploration in UIs like Grafana.

## Objectives

1. Implement Kubernetes-compatible health check endpoints
2. Provide metric discovery APIs for dynamic dashboards
3. Cache discovery results to minimize storage load
4. Support graceful degradation when dependencies fail
5. Enable deep health checks for troubleshooting

## Work Items

### 1. Health Check Models

#### 1.1 Health Status Types

**File:** `pkg/health/types.go`

```go
package health

import (
    "time"
)

// Status represents the health status of a component
type Status string

const (
    StatusHealthy   Status = "healthy"
    StatusDegraded  Status = "degraded"
    StatusUnhealthy Status = "unhealthy"
)

// ComponentHealth represents the health of a single component
type ComponentHealth struct {
    Name      string                 `json:"name"`
    Status    Status                 `json:"status"`
    Message   string                 `json:"message,omitempty"`
    Details   map[string]interface{} `json:"details,omitempty"`
    Timestamp time.Time              `json:"timestamp"`
    Duration  time.Duration          `json:"duration_ms"`
}

// OverallHealth represents the aggregate health of all components
type OverallHealth struct {
    Status     Status            `json:"status"`
    Components []ComponentHealth `json:"components"`
    Timestamp  time.Time         `json:"timestamp"`
}

// IsHealthy returns true if the status is healthy
func (s Status) IsHealthy() bool {
    return s == StatusHealthy
}

// IsDegraded returns true if the status is degraded
func (s Status) IsDegraded() bool {
    return s == StatusDegraded
}

// IsUnhealthy returns true if the status is unhealthy
func (s Status) IsUnhealthy() bool {
    return s == StatusUnhealthy
}
```

### 2. Health Check Interface

**File:** `pkg/health/checker.go`

```go
package health

import (
    "context"
)

// Checker defines the interface for health checking a component
type Checker interface {
    // Check performs a health check and returns the component health
    Check(ctx context.Context) ComponentHealth
}

// CheckerFunc is a function adapter for the Checker interface
type CheckerFunc func(ctx context.Context) ComponentHealth

func (f CheckerFunc) Check(ctx context.Context) ComponentHealth {
    return f(ctx)
}
```

### 3. Storage Health Checker

**File:** `pkg/health/storage_checker.go`

```go
package health

import (
    "context"
    "time"

    "github.com/yourusername/luminate/pkg/storage"
)

// StorageChecker checks the health of a storage backend
type StorageChecker struct {
    store storage.MetricsStore
    name  string
}

// NewStorageChecker creates a new storage health checker
func NewStorageChecker(store storage.MetricsStore, name string) *StorageChecker {
    return &StorageChecker{
        store: store,
        name:  name,
    }
}

// Check performs a health check on the storage backend
func (c *StorageChecker) Check(ctx context.Context) ComponentHealth {
    start := time.Now()

    health := ComponentHealth{
        Name:      c.name,
        Timestamp: start,
        Details:   make(map[string]interface{}),
    }

    // Create a timeout context for the health check
    checkCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
    defer cancel()

    // Perform health check
    err := c.store.Health(checkCtx)
    health.Duration = time.Since(start)

    if err == nil {
        health.Status = StatusHealthy
        health.Message = "Storage backend is healthy"
        return health
    }

    // Check if it's a timeout
    if checkCtx.Err() == context.DeadlineExceeded {
        health.Status = StatusUnhealthy
        health.Message = "Storage health check timed out"
        health.Details["error"] = "timeout after 5s"
        return health
    }

    // Other errors
    health.Status = StatusUnhealthy
    health.Message = "Storage backend is unhealthy"
    health.Details["error"] = err.Error()

    return health
}
```

### 4. Health Manager

**File:** `pkg/health/manager.go`

```go
package health

import (
    "context"
    "sync"
    "time"
)

// Manager coordinates health checks across all components
type Manager struct {
    checkers map[string]Checker
    mu       sync.RWMutex
}

// NewManager creates a new health check manager
func NewManager() *Manager {
    return &Manager{
        checkers: make(map[string]Checker),
    }
}

// Register adds a new health checker
func (m *Manager) Register(name string, checker Checker) {
    m.mu.Lock()
    defer m.mu.Unlock()
    m.checkers[name] = checker
}

// Check performs health checks on all registered components
func (m *Manager) Check(ctx context.Context) OverallHealth {
    m.mu.RLock()
    checkers := make(map[string]Checker, len(m.checkers))
    for k, v := range m.checkers {
        checkers[k] = v
    }
    m.mu.RUnlock()

    // Run all checks in parallel
    results := make(chan ComponentHealth, len(checkers))
    for name, checker := range checkers {
        go func(n string, c Checker) {
            results <- c.Check(ctx)
        }(name, checker)
    }

    // Collect results
    components := make([]ComponentHealth, 0, len(checkers))
    for i := 0; i < len(checkers); i++ {
        components = append(components, <-results)
    }

    // Determine overall status
    overall := OverallHealth{
        Components: components,
        Timestamp:  time.Now(),
        Status:     StatusHealthy,
    }

    for _, comp := range components {
        if comp.Status == StatusUnhealthy {
            overall.Status = StatusUnhealthy
            break
        }
        if comp.Status == StatusDegraded && overall.Status == StatusHealthy {
            overall.Status = StatusDegraded
        }
    }

    return overall
}

// Liveness performs a simple liveness check (process is running)
func (m *Manager) Liveness(ctx context.Context) bool {
    // Liveness is always true if we can respond
    return true
}

// Readiness checks if the service is ready to accept traffic
func (m *Manager) Readiness(ctx context.Context) bool {
    health := m.Check(ctx)
    // Ready if status is healthy or degraded (not unhealthy)
    return !health.Status.IsUnhealthy()
}

// Startup checks if the service has completed initialization
func (m *Manager) Startup(ctx context.Context) bool {
    health := m.Check(ctx)
    // Started if all components are at least degraded
    return !health.Status.IsUnhealthy()
}
```

### 5. Discovery Cache

**File:** `pkg/discovery/cache.go`

```go
package discovery

import (
    "context"
    "sync"
    "time"

    "github.com/yourusername/luminate/pkg/storage"
)

// Cache provides cached metric discovery results
type Cache struct {
    store storage.MetricsStore
    ttl   time.Duration

    mu              sync.RWMutex
    metrics         []string
    metricsExpiry   time.Time
    dimensionKeys   map[string]cacheEntry
    dimensionValues map[string]cacheEntry
}

type cacheEntry struct {
    data   []string
    expiry time.Time
}

// NewCache creates a new discovery cache
func NewCache(store storage.MetricsStore, ttl time.Duration) *Cache {
    return &Cache{
        store:           store,
        ttl:             ttl,
        dimensionKeys:   make(map[string]cacheEntry),
        dimensionValues: make(map[string]cacheEntry),
    }
}

// ListMetrics returns all metric names (cached)
func (c *Cache) ListMetrics(ctx context.Context) ([]string, error) {
    c.mu.RLock()
    if time.Now().Before(c.metricsExpiry) {
        defer c.mu.RUnlock()
        return c.metrics, nil
    }
    c.mu.RUnlock()

    // Cache miss, fetch from storage
    metrics, err := c.store.ListMetrics(ctx)
    if err != nil {
        return nil, err
    }

    c.mu.Lock()
    c.metrics = metrics
    c.metricsExpiry = time.Now().Add(c.ttl)
    c.mu.Unlock()

    return metrics, nil
}

// ListDimensionKeys returns all dimension keys for a metric (cached)
func (c *Cache) ListDimensionKeys(ctx context.Context, metricName string) ([]string, error) {
    c.mu.RLock()
    if entry, ok := c.dimensionKeys[metricName]; ok && time.Now().Before(entry.expiry) {
        defer c.mu.RUnlock()
        return entry.data, nil
    }
    c.mu.RUnlock()

    // Cache miss
    keys, err := c.store.ListDimensionKeys(ctx, metricName)
    if err != nil {
        return nil, err
    }

    c.mu.Lock()
    c.dimensionKeys[metricName] = cacheEntry{
        data:   keys,
        expiry: time.Now().Add(c.ttl),
    }
    c.mu.Unlock()

    return keys, nil
}

// ListDimensionValues returns all values for a dimension key (cached)
func (c *Cache) ListDimensionValues(ctx context.Context, metricName, dimensionKey string, limit int) ([]string, error) {
    cacheKey := metricName + ":" + dimensionKey

    c.mu.RLock()
    if entry, ok := c.dimensionValues[cacheKey]; ok && time.Now().Before(entry.expiry) {
        defer c.mu.RUnlock()
        // Apply limit
        if limit > 0 && len(entry.data) > limit {
            return entry.data[:limit], nil
        }
        return entry.data, nil
    }
    c.mu.RUnlock()

    // Cache miss
    values, err := c.store.ListDimensionValues(ctx, metricName, dimensionKey, limit)
    if err != nil {
        return nil, err
    }

    c.mu.Lock()
    c.dimensionValues[cacheKey] = cacheEntry{
        data:   values,
        expiry: time.Now().Add(c.ttl),
    }
    c.mu.Unlock()

    return values, nil
}

// Invalidate clears all cached data
func (c *Cache) Invalidate() {
    c.mu.Lock()
    defer c.mu.Unlock()

    c.metrics = nil
    c.metricsExpiry = time.Time{}
    c.dimensionKeys = make(map[string]cacheEntry)
    c.dimensionValues = make(map[string]cacheEntry)
}

// InvalidateMetric clears cached data for a specific metric
func (c *Cache) InvalidateMetric(metricName string) {
    c.mu.Lock()
    defer c.mu.Unlock()

    delete(c.dimensionKeys, metricName)

    // Remove all dimension value entries for this metric
    for key := range c.dimensionValues {
        if len(key) > len(metricName) && key[:len(metricName)] == metricName {
            delete(c.dimensionValues, key)
        }
    }
}
```

### 6. Health Check HTTP Handlers

**File:** `pkg/api/health_handlers.go`

```go
package api

import (
    "encoding/json"
    "net/http"
    "time"

    "github.com/yourusername/luminate/pkg/health"
)

// HealthHandler provides HTTP handlers for health checks
type HealthHandler struct {
    manager *health.Manager
}

// NewHealthHandler creates a new health handler
func NewHealthHandler(manager *health.Manager) *HealthHandler {
    return &HealthHandler{
        manager: manager,
    }
}

// HandleHealth returns detailed health status
func (h *HealthHandler) HandleHealth(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()

    // Set a timeout for the health check
    ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
    defer cancel()

    overall := h.manager.Check(ctx)

    // Set status code based on health
    statusCode := http.StatusOK
    if overall.Status == health.StatusUnhealthy {
        statusCode = http.StatusServiceUnavailable
    } else if overall.Status == health.StatusDegraded {
        statusCode = http.StatusOK // Still accepting traffic
    }

    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(statusCode)
    json.NewEncoder(w).Encode(overall)
}

// HandleLiveness returns liveness probe status (Kubernetes)
func (h *HealthHandler) HandleLiveness(w http.ResponseWriter, r *http.Request) {
    ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
    defer cancel()

    if h.manager.Liveness(ctx) {
        w.WriteHeader(http.StatusOK)
        w.Write([]byte("OK"))
    } else {
        w.WriteHeader(http.StatusServiceUnavailable)
        w.Write([]byte("NOT OK"))
    }
}

// HandleReadiness returns readiness probe status (Kubernetes)
func (h *HealthHandler) HandleReadiness(w http.ResponseWriter, r *http.Request) {
    ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
    defer cancel()

    if h.manager.Readiness(ctx) {
        w.WriteHeader(http.StatusOK)
        w.Write([]byte("READY"))
    } else {
        w.WriteHeader(http.StatusServiceUnavailable)
        w.Write([]byte("NOT READY"))
    }
}

// HandleStartup returns startup probe status (Kubernetes)
func (h *HealthHandler) HandleStartup(w http.ResponseWriter, r *http.Request) {
    ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
    defer cancel()

    if h.manager.Startup(ctx) {
        w.WriteHeader(http.StatusOK)
        w.Write([]byte("STARTED"))
    } else {
        w.WriteHeader(http.StatusServiceUnavailable)
        w.Write([]byte("NOT STARTED"))
    }
}
```

### 7. Discovery HTTP Handlers

**File:** `pkg/api/discovery_handlers.go`

```go
package api

import (
    "encoding/json"
    "net/http"
    "strconv"

    "github.com/yourusername/luminate/pkg/discovery"
)

// DiscoveryHandler provides HTTP handlers for metric discovery
type DiscoveryHandler struct {
    cache *discovery.Cache
}

// NewDiscoveryHandler creates a new discovery handler
func NewDiscoveryHandler(cache *discovery.Cache) *DiscoveryHandler {
    return &DiscoveryHandler{
        cache: cache,
    }
}

// HandleListMetrics returns all metric names
// GET /api/v1/metrics
func (h *DiscoveryHandler) HandleListMetrics(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()

    metrics, err := h.cache.ListMetrics(ctx)
    if err != nil {
        respondError(w, http.StatusInternalServerError, "DISCOVERY_FAILED", err)
        return
    }

    response := struct {
        Metrics []string `json:"metrics"`
        Count   int      `json:"count"`
    }{
        Metrics: metrics,
        Count:   len(metrics),
    }

    respondJSON(w, http.StatusOK, response)
}

// HandleListDimensionKeys returns all dimension keys for a metric
// GET /api/v1/metrics/{metric_name}/dimensions
func (h *DiscoveryHandler) HandleListDimensionKeys(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()

    // Extract metric name from URL
    metricName := extractPathParam(r.URL.Path, "/api/v1/metrics/", "/dimensions")
    if metricName == "" {
        respondError(w, http.StatusBadRequest, "INVALID_REQUEST",
            fmt.Errorf("metric name is required"))
        return
    }

    keys, err := h.cache.ListDimensionKeys(ctx, metricName)
    if err != nil {
        respondError(w, http.StatusInternalServerError, "DISCOVERY_FAILED", err)
        return
    }

    response := struct {
        MetricName string   `json:"metricName"`
        Dimensions []string `json:"dimensions"`
        Count      int      `json:"count"`
    }{
        MetricName: metricName,
        Dimensions: keys,
        Count:      len(keys),
    }

    respondJSON(w, http.StatusOK, response)
}

// HandleListDimensionValues returns all values for a dimension key
// GET /api/v1/metrics/{metric_name}/dimensions/{dimension_key}/values?limit=100
func (h *DiscoveryHandler) HandleListDimensionValues(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()

    // Extract parameters
    metricName := extractPathParam(r.URL.Path, "/api/v1/metrics/", "/dimensions/")
    dimensionKey := extractLastPathParam(r.URL.Path)

    if metricName == "" || dimensionKey == "" {
        respondError(w, http.StatusBadRequest, "INVALID_REQUEST",
            fmt.Errorf("metric name and dimension key are required"))
        return
    }

    // Parse limit
    limit := 100 // default
    if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
        if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
            limit = l
        }
    }

    values, err := h.cache.ListDimensionValues(ctx, metricName, dimensionKey, limit)
    if err != nil {
        respondError(w, http.StatusInternalServerError, "DISCOVERY_FAILED", err)
        return
    }

    response := struct {
        MetricName   string   `json:"metricName"`
        DimensionKey string   `json:"dimensionKey"`
        Values       []string `json:"values"`
        Count        int      `json:"count"`
        Limit        int      `json:"limit"`
    }{
        MetricName:   metricName,
        DimensionKey: dimensionKey,
        Values:       values,
        Count:        len(values),
        Limit:        limit,
    }

    respondJSON(w, http.StatusOK, response)
}

// Helper functions
func extractPathParam(path, prefix, suffix string) string {
    if !strings.HasPrefix(path, prefix) {
        return ""
    }
    path = path[len(prefix):]

    if suffix != "" {
        idx := strings.Index(path, suffix)
        if idx == -1 {
            return ""
        }
        path = path[:idx]
    }

    return path
}

func extractLastPathParam(path string) string {
    parts := strings.Split(path, "/")
    if len(parts) > 0 {
        return parts[len(parts)-1]
    }
    return ""
}
```

### 8. Integration into Main Server

**File:** `cmd/luminate/main.go` (additions)

```go
package main

import (
    "context"
    "log"
    "net/http"
    "time"

    "github.com/yourusername/luminate/pkg/api"
    "github.com/yourusername/luminate/pkg/discovery"
    "github.com/yourusername/luminate/pkg/health"
)

func main() {
    // ... existing setup ...

    // Initialize health manager
    healthManager := health.NewManager()

    // Register storage health checker
    storageChecker := health.NewStorageChecker(store, "storage")
    healthManager.Register("storage", storageChecker)

    // Initialize discovery cache (5 minute TTL)
    discoveryCache := discovery.NewCache(store, 5*time.Minute)

    // Create handlers
    healthHandler := api.NewHealthHandler(healthManager)
    discoveryHandler := api.NewDiscoveryHandler(discoveryCache)

    // Setup HTTP router
    mux := http.NewServeMux()

    // Health check endpoints
    mux.HandleFunc("/api/v1/health", healthHandler.HandleHealth)
    mux.HandleFunc("/healthz", healthHandler.HandleLiveness)
    mux.HandleFunc("/readyz", healthHandler.HandleReadiness)
    mux.HandleFunc("/startupz", healthHandler.HandleStartup)

    // Discovery endpoints
    mux.HandleFunc("/api/v1/metrics", discoveryHandler.HandleListMetrics)
    mux.HandleFunc("/api/v1/metrics/", discoveryHandler.HandleListDimensionKeys)

    // ... other endpoints ...

    log.Println("Starting Luminate on :8080")
    if err := http.ListenAndServe(":8080", mux); err != nil {
        log.Fatal(err)
    }
}
```

## Testing

### Unit Tests

**File:** `pkg/health/manager_test.go`

```go
package health

import (
    "context"
    "testing"
    "time"
)

func TestHealthManager(t *testing.T) {
    manager := NewManager()

    // Register healthy checker
    manager.Register("component1", CheckerFunc(func(ctx context.Context) ComponentHealth {
        return ComponentHealth{
            Name:      "component1",
            Status:    StatusHealthy,
            Message:   "OK",
            Timestamp: time.Now(),
        }
    }))

    // Register unhealthy checker
    manager.Register("component2", CheckerFunc(func(ctx context.Context) ComponentHealth {
        return ComponentHealth{
            Name:      "component2",
            Status:    StatusUnhealthy,
            Message:   "Failed",
            Timestamp: time.Now(),
        }
    }))

    ctx := context.Background()
    overall := manager.Check(ctx)

    if overall.Status != StatusUnhealthy {
        t.Errorf("Expected overall status to be unhealthy, got %s", overall.Status)
    }

    if len(overall.Components) != 2 {
        t.Errorf("Expected 2 components, got %d", len(overall.Components))
    }
}

func TestReadiness(t *testing.T) {
    manager := NewManager()

    // Healthy system
    manager.Register("storage", CheckerFunc(func(ctx context.Context) ComponentHealth {
        return ComponentHealth{
            Name:      "storage",
            Status:    StatusHealthy,
            Timestamp: time.Now(),
        }
    }))

    if !manager.Readiness(context.Background()) {
        t.Error("Expected system to be ready")
    }

    // Degraded but ready
    manager = NewManager()
    manager.Register("storage", CheckerFunc(func(ctx context.Context) ComponentHealth {
        return ComponentHealth{
            Name:      "storage",
            Status:    StatusDegraded,
            Timestamp: time.Now(),
        }
    }))

    if !manager.Readiness(context.Background()) {
        t.Error("Expected degraded system to still be ready")
    }

    // Unhealthy, not ready
    manager = NewManager()
    manager.Register("storage", CheckerFunc(func(ctx context.Context) ComponentHealth {
        return ComponentHealth{
            Name:      "storage",
            Status:    StatusUnhealthy,
            Timestamp: time.Now(),
        }
    }))

    if manager.Readiness(context.Background()) {
        t.Error("Expected unhealthy system to not be ready")
    }
}
```

**File:** `pkg/discovery/cache_test.go`

```go
package discovery

import (
    "context"
    "testing"
    "time"

    "github.com/yourusername/luminate/pkg/storage"
    "github.com/yourusername/luminate/pkg/storage/badger"
)

func TestDiscoveryCache(t *testing.T) {
    // Create in-memory BadgerDB
    store, err := badger.NewBadgerStore(badger.Config{
        Path:   t.TempDir(),
        Memory: true,
    })
    if err != nil {
        t.Fatal(err)
    }
    defer store.Close()

    // Write some metrics
    ctx := context.Background()
    metrics := []storage.Metric{
        {
            Name:      "api_latency",
            Value:     0.150,
            Timestamp: time.Now(),
            Dimensions: map[string]string{
                "endpoint": "/api/query",
                "method":   "POST",
            },
        },
    }
    store.Write(ctx, metrics)

    // Create cache with 1 second TTL
    cache := NewCache(store, 1*time.Second)

    // First call - cache miss
    result1, err := cache.ListMetrics(ctx)
    if err != nil {
        t.Fatal(err)
    }
    if len(result1) != 1 || result1[0] != "api_latency" {
        t.Errorf("Expected [api_latency], got %v", result1)
    }

    // Second call - cache hit (should be fast)
    start := time.Now()
    result2, err := cache.ListMetrics(ctx)
    duration := time.Since(start)
    if err != nil {
        t.Fatal(err)
    }
    if duration > 1*time.Millisecond {
        t.Error("Cache hit took too long")
    }
    if len(result2) != 1 {
        t.Error("Expected cached result")
    }

    // Wait for cache expiry
    time.Sleep(1100 * time.Millisecond)

    // Third call - cache expired
    result3, err := cache.ListMetrics(ctx)
    if err != nil {
        t.Fatal(err)
    }
    if len(result3) != 1 {
        t.Error("Expected result after cache expiry")
    }
}

func TestDimensionKeysCache(t *testing.T) {
    store, _ := setupTestStore(t)
    defer store.Close()

    cache := NewCache(store, 5*time.Second)
    ctx := context.Background()

    keys, err := cache.ListDimensionKeys(ctx, "api_latency")
    if err != nil {
        t.Fatal(err)
    }

    if len(keys) < 2 {
        t.Error("Expected at least 2 dimension keys")
    }

    // Should contain endpoint and method
    hasEndpoint := false
    hasMethod := false
    for _, key := range keys {
        if key == "endpoint" {
            hasEndpoint = true
        }
        if key == "method" {
            hasMethod = true
        }
    }

    if !hasEndpoint || !hasMethod {
        t.Error("Expected endpoint and method dimension keys")
    }
}
```

### Integration Tests

**File:** `pkg/api/health_handlers_test.go`

```go
package api

import (
    "context"
    "net/http"
    "net/http/httptest"
    "testing"

    "github.com/yourusername/luminate/pkg/health"
)

func TestHealthHandler(t *testing.T) {
    manager := health.NewManager()
    manager.Register("test", health.CheckerFunc(func(ctx context.Context) health.ComponentHealth {
        return health.ComponentHealth{
            Name:   "test",
            Status: health.StatusHealthy,
        }
    }))

    handler := NewHealthHandler(manager)

    req := httptest.NewRequest("GET", "/api/v1/health", nil)
    w := httptest.NewRecorder()

    handler.HandleHealth(w, req)

    if w.Code != http.StatusOK {
        t.Errorf("Expected status 200, got %d", w.Code)
    }

    // Verify JSON response
    contentType := w.Header().Get("Content-Type")
    if contentType != "application/json" {
        t.Errorf("Expected application/json, got %s", contentType)
    }
}

func TestReadinessHandler(t *testing.T) {
    manager := health.NewManager()
    manager.Register("test", health.CheckerFunc(func(ctx context.Context) health.ComponentHealth {
        return health.ComponentHealth{
            Name:   "test",
            Status: health.StatusHealthy,
        }
    }))

    handler := NewHealthHandler(manager)

    req := httptest.NewRequest("GET", "/readyz", nil)
    w := httptest.NewRecorder()

    handler.HandleReadiness(w, req)

    if w.Code != http.StatusOK {
        t.Errorf("Expected status 200, got %d", w.Code)
    }

    if w.Body.String() != "READY" {
        t.Errorf("Expected READY, got %s", w.Body.String())
    }
}
```

## Kubernetes Probes Configuration

**File:** `deployments/kubernetes/deployment.yaml` (additions)

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: luminate
spec:
  template:
    spec:
      containers:
      - name: luminate
        image: luminate:latest
        ports:
        - containerPort: 8080
          name: http

        # Liveness probe - restart if process is stuck
        livenessProbe:
          httpGet:
            path: /healthz
            port: http
          initialDelaySeconds: 10
          periodSeconds: 10
          timeoutSeconds: 5
          failureThreshold: 3

        # Readiness probe - remove from load balancer if not ready
        readinessProbe:
          httpGet:
            path: /readyz
            port: http
          initialDelaySeconds: 5
          periodSeconds: 5
          timeoutSeconds: 3
          failureThreshold: 2

        # Startup probe - allow time for initialization
        startupProbe:
          httpGet:
            path: /startupz
            port: http
          initialDelaySeconds: 0
          periodSeconds: 5
          timeoutSeconds: 3
          failureThreshold: 12  # 60 seconds to start
```

## Acceptance Criteria

- [ ] `/api/v1/health` endpoint returns detailed health status
- [ ] `/healthz` liveness probe always returns 200 when process is responsive
- [ ] `/readyz` readiness probe returns 503 when dependencies are unhealthy
- [ ] `/startupz` startup probe tracks initialization status
- [ ] Health checks timeout after 5 seconds
- [ ] Discovery cache reduces storage load by >90% for repeated requests
- [ ] Cache TTL is configurable (default 5 minutes)
- [ ] `/api/v1/metrics` lists all metric names
- [ ] `/api/v1/metrics/{name}/dimensions` lists dimension keys
- [ ] `/api/v1/metrics/{name}/dimensions/{key}/values` lists values with limit
- [ ] All endpoints have comprehensive unit tests
- [ ] Integration tests verify Kubernetes probe compatibility

## Performance Considerations

1. **Health Check Timeout**: 5 seconds prevents slow checks from blocking restarts
2. **Parallel Checks**: All component checks run concurrently
3. **Discovery Cache**: 5-minute TTL balances freshness and load
4. **Cache Invalidation**: Automatic on write operations
5. **Graceful Degradation**: Degraded status allows traffic while investigating issues

## Summary

This workstream implements production-ready health checks and metric discovery:
- Kubernetes-compatible liveness/readiness/startup probes
- Detailed component health status for troubleshooting
- Efficient caching for discovery APIs
- Foundation for dynamic Grafana dashboards

**Total Estimated Effort:** 2-3 days

**Dependencies:** WS1 (Storage Backend), WS3 (HTTP API Handlers)

**Deliverables:**
- Health check manager and checkers
- Discovery cache with TTL
- HTTP handlers for all probe types
- Comprehensive test coverage
- Kubernetes probe configurations
