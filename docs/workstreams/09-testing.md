# WS9: Testing Framework

**Priority:** P1 (High Priority)
**Estimated Effort:** 5-6 days
**Dependencies:** All implementation workstreams (WS1-WS8)

## Overview

Build a comprehensive testing framework to ensure Luminate's reliability, performance, and correctness. This includes unit tests, integration tests, load testing, chaos testing, and continuous integration pipelines.

## Objectives

1. Achieve 80%+ code coverage with unit tests
2. Implement integration tests for end-to-end workflows
3. Create mock implementations for testing
4. Build load testing suite to validate performance targets
5. Implement chaos testing scenarios
6. Set up CI/CD pipeline with automated testing
7. Generate test data for realistic scenarios

## Work Items

### 1. Unit Test Framework

#### 1.1 Test Utilities

**File:** `pkg/testutil/testutil.go`

```go
package testutil

import (
    "context"
    "fmt"
    "math/rand"
    "testing"
    "time"

    "github.com/yourusername/luminate/pkg/storage"
)

// TestContext creates a test context with timeout
func TestContext(t *testing.T) context.Context {
    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    t.Cleanup(cancel)
    return ctx
}

// AssertNoError fails the test if err is not nil
func AssertNoError(t *testing.T, err error, msg string) {
    t.Helper()
    if err != nil {
        t.Fatalf("%s: %v", msg, err)
    }
}

// AssertError fails the test if err is nil
func AssertError(t *testing.T, err error, msg string) {
    t.Helper()
    if err == nil {
        t.Fatalf("%s: expected error but got nil", msg)
    }
}

// AssertEqual fails if actual != expected
func AssertEqual(t *testing.T, expected, actual interface{}, msg string) {
    t.Helper()
    if expected != actual {
        t.Fatalf("%s: expected %v, got %v", msg, expected, actual)
    }
}

// AssertFloatEqual fails if floats are not approximately equal
func AssertFloatEqual(t *testing.T, expected, actual, epsilon float64, msg string) {
    t.Helper()
    diff := expected - actual
    if diff < 0 {
        diff = -diff
    }
    if diff > epsilon {
        t.Fatalf("%s: expected %f, got %f (epsilon=%f)", msg, expected, actual, epsilon)
    }
}

// AssertContains fails if slice doesn't contain element
func AssertContains(t *testing.T, slice []string, element string, msg string) {
    t.Helper()
    for _, item := range slice {
        if item == element {
            return
        }
    }
    t.Fatalf("%s: slice %v does not contain %s", msg, slice, element)
}
```

#### 1.2 Test Data Generator

**File:** `pkg/testutil/generator.go`

```go
package testutil

import (
    "fmt"
    "math/rand"
    "time"

    "github.com/yourusername/luminate/pkg/storage"
)

// MetricGenerator generates test metrics
type MetricGenerator struct {
    rand *rand.Rand
}

// NewMetricGenerator creates a new generator with a fixed seed
func NewMetricGenerator(seed int64) *MetricGenerator {
    return &MetricGenerator{
        rand: rand.New(rand.NewSource(seed)),
    }
}

// Generate creates n random metrics
func (g *MetricGenerator) Generate(n int, metricName string) []storage.Metric {
    metrics := make([]storage.Metric, n)
    now := time.Now()

    for i := 0; i < n; i++ {
        metrics[i] = storage.Metric{
            Name:      metricName,
            Value:     g.rand.Float64() * 1000,
            Timestamp: now.Add(-time.Duration(i) * time.Second),
            Dimensions: map[string]string{
                "endpoint": g.randomEndpoint(),
                "method":   g.randomMethod(),
                "status":   g.randomStatus(),
                "region":   g.randomRegion(),
            },
        }
    }

    return metrics
}

// GenerateTimeRange creates metrics over a time range
func (g *MetricGenerator) GenerateTimeRange(metricName string, start, end time.Time, interval time.Duration) []storage.Metric {
    var metrics []storage.Metric

    for t := start; t.Before(end); t = t.Add(interval) {
        metrics = append(metrics, storage.Metric{
            Name:      metricName,
            Value:     g.rand.Float64() * 100,
            Timestamp: t,
            Dimensions: map[string]string{
                "host": fmt.Sprintf("host-%d", g.rand.Intn(10)),
            },
        })
    }

    return metrics
}

// GenerateWithCardinality creates metrics with specific cardinality
func (g *MetricGenerator) GenerateWithCardinality(metricName string, count, cardinality int) []storage.Metric {
    metrics := make([]storage.Metric, count)
    now := time.Now()

    dimensionSets := make([]map[string]string, cardinality)
    for i := 0; i < cardinality; i++ {
        dimensionSets[i] = map[string]string{
            "service": fmt.Sprintf("svc-%d", i),
            "region":  g.randomRegion(),
        }
    }

    for i := 0; i < count; i++ {
        dimIdx := g.rand.Intn(cardinality)
        metrics[i] = storage.Metric{
            Name:       metricName,
            Value:      g.rand.Float64() * 100,
            Timestamp:  now.Add(-time.Duration(i) * time.Millisecond),
            Dimensions: dimensionSets[dimIdx],
        }
    }

    return metrics
}

// Helper methods
func (g *MetricGenerator) randomEndpoint() string {
    endpoints := []string{"/api/query", "/api/write", "/api/metrics", "/health"}
    return endpoints[g.rand.Intn(len(endpoints))]
}

func (g *MetricGenerator) randomMethod() string {
    methods := []string{"GET", "POST", "PUT", "DELETE"}
    return methods[g.rand.Intn(len(methods))]
}

func (g *MetricGenerator) randomStatus() string {
    statuses := []string{"200", "201", "400", "404", "500"}
    return statuses[g.rand.Intn(len(statuses))]
}

func (g *MetricGenerator) randomRegion() string {
    regions := []string{"us-east-1", "us-west-2", "eu-west-1", "ap-southeast-1"}
    return regions[g.rand.Intn(len(regions))]
}
```

### 2. Mock Implementations

#### 2.1 Mock Storage

**File:** `pkg/storage/mock/mock_store.go`

```go
package mock

import (
    "context"
    "sort"
    "sync"
    "time"

    "github.com/yourusername/luminate/pkg/storage"
)

// MockStore is an in-memory storage implementation for testing
type MockStore struct {
    mu      sync.RWMutex
    metrics []storage.Metric

    // Call tracking for assertions
    writeCalls  int
    queryCalls  int
    lastWriteErr error
    lastQueryErr error
}

// NewMockStore creates a new mock storage
func NewMockStore() *MockStore {
    return &MockStore{
        metrics: make([]storage.Metric, 0),
    }
}

// Write stores metrics in memory
func (m *MockStore) Write(ctx context.Context, metrics []storage.Metric) error {
    m.mu.Lock()
    defer m.mu.Unlock()

    m.writeCalls++

    if m.lastWriteErr != nil {
        return m.lastWriteErr
    }

    m.metrics = append(m.metrics, metrics...)
    return nil
}

// QueryRange returns metrics matching the query
func (m *MockStore) QueryRange(ctx context.Context, req storage.QueryRequest) ([]storage.MetricPoint, error) {
    m.mu.RLock()
    defer m.mu.RUnlock()

    m.queryCalls++

    if m.lastQueryErr != nil {
        return nil, m.lastQueryErr
    }

    var results []storage.MetricPoint

    for _, metric := range m.metrics {
        // Filter by metric name
        if metric.Name != req.MetricName {
            continue
        }

        // Filter by time range
        if metric.Timestamp.Before(req.Start) || metric.Timestamp.After(req.End) {
            continue
        }

        // Filter by dimensions
        if !matchesFilters(metric.Dimensions, req.Filters) {
            continue
        }

        results = append(results, storage.MetricPoint{
            Timestamp:  metric.Timestamp,
            Value:      metric.Value,
            Dimensions: metric.Dimensions,
        })
    }

    // Sort by timestamp
    sort.Slice(results, func(i, j int) bool {
        return results[i].Timestamp.Before(results[j].Timestamp)
    })

    return results, nil
}

// Aggregate performs aggregation on metrics
func (m *MockStore) Aggregate(ctx context.Context, req storage.AggregateRequest) ([]storage.AggregateResult, error) {
    m.mu.RLock()
    defer m.mu.RUnlock()

    if m.lastQueryErr != nil {
        return nil, m.lastQueryErr
    }

    // Simplified aggregation logic for testing
    var values []float64
    for _, metric := range m.metrics {
        if metric.Name == req.MetricName &&
            metric.Timestamp.After(req.Start) &&
            metric.Timestamp.Before(req.End) &&
            matchesFilters(metric.Dimensions, req.Filters) {
            values = append(values, metric.Value)
        }
    }

    if len(values) == 0 {
        return []storage.AggregateResult{}, nil
    }

    result := storage.AggregateResult{
        Dimensions: make(map[string]string),
    }

    switch req.Aggregation {
    case storage.AVG:
        sum := 0.0
        for _, v := range values {
            sum += v
        }
        result.Value = sum / float64(len(values))
    case storage.SUM:
        sum := 0.0
        for _, v := range values {
            sum += v
        }
        result.Value = sum
    case storage.COUNT:
        result.Value = float64(len(values))
    case storage.MIN:
        min := values[0]
        for _, v := range values {
            if v < min {
                min = v
            }
        }
        result.Value = min
    case storage.MAX:
        max := values[0]
        for _, v := range values {
            if v > max {
                max = v
            }
        }
        result.Value = max
    }

    return []storage.AggregateResult{result}, nil
}

// Rate calculates rate of change
func (m *MockStore) Rate(ctx context.Context, req storage.RateRequest) ([]storage.RatePoint, error) {
    // Simplified implementation for testing
    return []storage.RatePoint{}, nil
}

// ListMetrics returns all unique metric names
func (m *MockStore) ListMetrics(ctx context.Context) ([]string, error) {
    m.mu.RLock()
    defer m.mu.RUnlock()

    seen := make(map[string]bool)
    for _, metric := range m.metrics {
        seen[metric.Name] = true
    }

    result := make([]string, 0, len(seen))
    for name := range seen {
        result = append(result, name)
    }
    sort.Strings(result)

    return result, nil
}

// ListDimensionKeys returns all dimension keys for a metric
func (m *MockStore) ListDimensionKeys(ctx context.Context, metricName string) ([]string, error) {
    m.mu.RLock()
    defer m.mu.RUnlock()

    seen := make(map[string]bool)
    for _, metric := range m.metrics {
        if metric.Name == metricName {
            for key := range metric.Dimensions {
                seen[key] = true
            }
        }
    }

    result := make([]string, 0, len(seen))
    for key := range seen {
        result = append(result, key)
    }
    sort.Strings(result)

    return result, nil
}

// ListDimensionValues returns all values for a dimension key
func (m *MockStore) ListDimensionValues(ctx context.Context, metricName, dimensionKey string, limit int) ([]string, error) {
    m.mu.RLock()
    defer m.mu.RUnlock()

    seen := make(map[string]bool)
    for _, metric := range m.metrics {
        if metric.Name == metricName {
            if val, ok := metric.Dimensions[dimensionKey]; ok {
                seen[val] = true
            }
        }
    }

    result := make([]string, 0, len(seen))
    for val := range seen {
        result = append(result, val)
    }
    sort.Strings(result)

    if limit > 0 && len(result) > limit {
        result = result[:limit]
    }

    return result, nil
}

// DeleteBefore removes old metrics
func (m *MockStore) DeleteBefore(ctx context.Context, metricName string, before time.Time) error {
    m.mu.Lock()
    defer m.mu.Unlock()

    filtered := make([]storage.Metric, 0)
    for _, metric := range m.metrics {
        if metric.Name != metricName || metric.Timestamp.After(before) {
            filtered = append(filtered, metric)
        }
    }

    m.metrics = filtered
    return nil
}

// Health always returns healthy for mock
func (m *MockStore) Health(ctx context.Context) error {
    return nil
}

// Close is a no-op for mock
func (m *MockStore) Close() error {
    return nil
}

// Test helpers
func (m *MockStore) SetWriteError(err error) {
    m.mu.Lock()
    defer m.mu.Unlock()
    m.lastWriteErr = err
}

func (m *MockStore) SetQueryError(err error) {
    m.mu.Lock()
    defer m.mu.Unlock()
    m.lastQueryErr = err
}

func (m *MockStore) GetWriteCalls() int {
    m.mu.RLock()
    defer m.mu.RUnlock()
    return m.writeCalls
}

func (m *MockStore) GetQueryCalls() int {
    m.mu.RLock()
    defer m.mu.RUnlock()
    return m.queryCalls
}

func (m *MockStore) Clear() {
    m.mu.Lock()
    defer m.mu.Unlock()
    m.metrics = make([]storage.Metric, 0)
    m.writeCalls = 0
    m.queryCalls = 0
}

// Helper functions
func matchesFilters(dimensions, filters map[string]string) bool {
    for key, value := range filters {
        if dimensions[key] != value {
            return false
        }
    }
    return true
}
```

### 3. Integration Tests

#### 3.1 End-to-End Workflow Tests

**File:** `test/integration/e2e_test.go`

```go
package integration

import (
    "bytes"
    "context"
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "testing"
    "time"

    "github.com/yourusername/luminate/pkg/api"
    "github.com/yourusername/luminate/pkg/storage/badger"
    "github.com/yourusername/luminate/pkg/testutil"
)

func TestWriteAndQuery(t *testing.T) {
    // Setup
    store, err := badger.NewBadgerStore(badger.Config{
        Path:   t.TempDir(),
        Memory: true,
    })
    testutil.AssertNoError(t, err, "create store")
    defer store.Close()

    writeHandler := api.NewWriteHandler(store)
    queryHandler := api.NewQueryHandler(store)

    ctx := testutil.TestContext(t)

    // Step 1: Write metrics
    writeReq := api.BatchWriteRequest{
        Metrics: []api.JSONMetric{
            {
                Name:      "api_latency",
                Value:     0.150,
                Timestamp: time.Now().Unix(),
                Dimensions: map[string]string{
                    "endpoint": "/api/query",
                    "method":   "POST",
                },
            },
            {
                Name:      "api_latency",
                Value:     0.200,
                Timestamp: time.Now().Unix(),
                Dimensions: map[string]string{
                    "endpoint": "/api/query",
                    "method":   "POST",
                },
            },
        },
    }

    body, _ := json.Marshal(writeReq)
    req := httptest.NewRequest("POST", "/api/v1/write", bytes.NewReader(body))
    w := httptest.NewRecorder()

    writeHandler.HandleWrite(w, req)
    testutil.AssertEqual(t, http.StatusOK, w.Code, "write status code")

    // Step 2: Query metrics
    queryReq := api.JSONQuery{
        QueryType:  "aggregate",
        MetricName: "api_latency",
        TimeRange: api.TimeRangeSpec{
            Relative: "1h",
        },
        Aggregation: "avg",
    }

    body, _ = json.Marshal(queryReq)
    req = httptest.NewRequest("POST", "/api/v1/query", bytes.NewReader(body))
    w = httptest.NewRecorder()

    queryHandler.HandleQuery(w, req)
    testutil.AssertEqual(t, http.StatusOK, w.Code, "query status code")

    // Verify response
    var response struct {
        Results []storage.AggregateResult `json:"results"`
    }
    json.NewDecoder(w.Body).Decode(&response)

    if len(response.Results) != 1 {
        t.Fatalf("Expected 1 result, got %d", len(response.Results))
    }

    expectedAvg := (0.150 + 0.200) / 2
    testutil.AssertFloatEqual(t, expectedAvg, response.Results[0].Value, 0.001, "average value")
}

func TestDiscoveryWorkflow(t *testing.T) {
    store, _ := setupTestStore(t)
    defer store.Close()

    discoveryHandler := api.NewDiscoveryHandler(
        discovery.NewCache(store, 5*time.Minute),
    )

    // Write test data
    gen := testutil.NewMetricGenerator(42)
    metrics := gen.Generate(100, "http_requests")
    store.Write(context.Background(), metrics)

    // Test: List metrics
    req := httptest.NewRequest("GET", "/api/v1/metrics", nil)
    w := httptest.NewRecorder()

    discoveryHandler.HandleListMetrics(w, req)
    testutil.AssertEqual(t, http.StatusOK, w.Code, "list metrics status")

    var metricsResp struct {
        Metrics []string `json:"metrics"`
    }
    json.NewDecoder(w.Body).Decode(&metricsResp)
    testutil.AssertContains(t, metricsResp.Metrics, "http_requests", "metrics list")

    // Test: List dimension keys
    req = httptest.NewRequest("GET", "/api/v1/metrics/http_requests/dimensions", nil)
    w = httptest.NewRecorder()

    discoveryHandler.HandleListDimensionKeys(w, req)
    testutil.AssertEqual(t, http.StatusOK, w.Code, "list dimension keys status")
}
```

### 4. Load Testing

#### 4.1 Load Test Script (k6)

**File:** `test/load/write_load.js`

```javascript
import http from 'k6/http';
import { check, sleep } from 'k6';
import { Rate } from 'k6/metrics';

const errorRate = new Rate('errors');

export const options = {
  stages: [
    { duration: '1m', target: 100 },  // Ramp up to 100 VUs
    { duration: '3m', target: 100 },  // Stay at 100 VUs
    { duration: '1m', target: 200 },  // Ramp to 200 VUs
    { duration: '3m', target: 200 },  // Stay at 200 VUs
    { duration: '1m', target: 0 },    // Ramp down
  ],
  thresholds: {
    'http_req_duration': ['p(95)<100', 'p(99)<200'], // 95% < 100ms, 99% < 200ms
    'errors': ['rate<0.01'],                          // Error rate < 1%
  },
};

export default function () {
  const baseURL = __ENV.BASE_URL || 'http://localhost:8080';

  const payload = JSON.stringify({
    metrics: [
      {
        name: 'load_test_metric',
        value: Math.random() * 100,
        timestamp: Math.floor(Date.now() / 1000),
        dimensions: {
          host: `host-${Math.floor(Math.random() * 10)}`,
          region: 'us-east-1',
          environment: 'load-test',
        },
      },
    ],
  });

  const params = {
    headers: {
      'Content-Type': 'application/json',
    },
  };

  const res = http.post(`${baseURL}/api/v1/write`, payload, params);

  const success = check(res, {
    'status is 200': (r) => r.status === 200,
    'response time < 100ms': (r) => r.timings.duration < 100,
  });

  errorRate.add(!success);

  sleep(0.1); // 10 requests/second per VU
}
```

**File:** `test/load/query_load.js`

```javascript
import http from 'k6/http';
import { check } from 'k6';
import { Rate } from 'k6/metrics';

const errorRate = new Rate('errors');

export const options = {
  vus: 50,
  duration: '5m',
  thresholds: {
    'http_req_duration': ['p(95)<200', 'p(99)<500'],
    'errors': ['rate<0.05'],
  },
};

export default function () {
  const baseURL = __ENV.BASE_URL || 'http://localhost:8080';

  const payload = JSON.stringify({
    queryType: 'aggregate',
    metricName: 'load_test_metric',
    timeRange: { relative: '1h' },
    aggregation: 'avg',
    groupBy: ['host'],
  });

  const params = {
    headers: {
      'Content-Type': 'application/json',
    },
  };

  const res = http.post(`${baseURL}/api/v1/query`, payload, params);

  const success = check(res, {
    'status is 200': (r) => r.status === 200,
    'has results': (r) => JSON.parse(r.body).results !== undefined,
  });

  errorRate.add(!success);
}
```

#### 4.2 Load Test Runner

**File:** `test/load/run_load_tests.sh`

```bash
#!/bin/bash

set -e

BASE_URL="${BASE_URL:-http://localhost:8080}"

echo "Running load tests against $BASE_URL"

# Run write load test
echo "=== Write Load Test ==="
k6 run --env BASE_URL=$BASE_URL test/load/write_load.js

# Run query load test
echo "=== Query Load Test ==="
k6 run --env BASE_URL=$BASE_URL test/load/query_load.js

# Run mixed workload
echo "=== Mixed Workload ==="
k6 run --env BASE_URL=$BASE_URL test/load/mixed_load.js

echo "Load tests completed successfully"
```

### 5. Chaos Testing

**File:** `test/chaos/network_partition.go`

```go
package chaos

import (
    "context"
    "testing"
    "time"

    "github.com/yourusername/luminate/pkg/storage"
    "github.com/yourusername/luminate/pkg/testutil"
)

// TestNetworkPartition simulates network failures
func TestNetworkPartition(t *testing.T) {
    if testing.Short() {
        t.Skip("Skipping chaos test in short mode")
    }

    store := setupTestStore(t)
    defer store.Close()

    ctx := testutil.TestContext(t)
    gen := testutil.NewMetricGenerator(42)

    // Phase 1: Normal operation
    metrics := gen.Generate(100, "test_metric")
    err := store.Write(ctx, metrics)
    testutil.AssertNoError(t, err, "initial write")

    // Phase 2: Simulate network partition (context cancellation)
    cancelCtx, cancel := context.WithCancel(ctx)
    cancel() // Immediate cancellation

    err = store.Write(cancelCtx, metrics)
    testutil.AssertError(t, err, "write during partition")

    // Phase 3: Recovery
    err = store.Write(ctx, metrics)
    testutil.AssertNoError(t, err, "write after recovery")

    // Verify data integrity
    results, err := store.QueryRange(ctx, storage.QueryRequest{
        MetricName: "test_metric",
        Start:      time.Now().Add(-1 * time.Hour),
        End:        time.Now(),
    })
    testutil.AssertNoError(t, err, "query after recovery")

    if len(results) < 100 {
        t.Errorf("Expected at least 100 results, got %d", len(results))
    }
}

// TestHighLatency simulates slow storage
func TestHighLatency(t *testing.T) {
    if testing.Short() {
        t.Skip("Skipping chaos test in short mode")
    }

    store := setupTestStore(t)
    defer store.Close()

    gen := testutil.NewMetricGenerator(42)
    metrics := gen.Generate(1000, "latency_test")

    // Use a short timeout to simulate timeout conditions
    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
    defer cancel()

    start := time.Now()
    err := store.Write(ctx, metrics)
    duration := time.Since(start)

    // Expect timeout
    if err == nil && duration > 10*time.Millisecond {
        t.Error("Expected timeout error")
    }
}
```

### 6. CI/CD Pipeline

#### 6.1 GitHub Actions Workflow

**File:** `.github/workflows/test.yml`

```yaml
name: Tests

on:
  push:
    branches: [ main ]
  pull_request:
    branches: [ main ]

jobs:
  unit-tests:
    runs-on: ubuntu-latest
    steps:
    - uses: actions/checkout@v3

    - name: Set up Go
      uses: actions/setup-go@v4
      with:
        go-version: '1.21'

    - name: Run unit tests
      run: make test

    - name: Generate coverage report
      run: make test-coverage

    - name: Upload coverage to Codecov
      uses: codecov/codecov-action@v3
      with:
        files: ./coverage.out
        flags: unittests

  integration-tests:
    runs-on: ubuntu-latest
    services:
      clickhouse:
        image: clickhouse/clickhouse-server:latest
        ports:
          - 9000:9000
        options: --health-cmd="wget --spider http://localhost:8123/ping" --health-interval=10s --health-timeout=5s --health-retries=5

    steps:
    - uses: actions/checkout@v3

    - name: Set up Go
      uses: actions/setup-go@v4
      with:
        go-version: '1.21'

    - name: Run integration tests
      run: make test-integration
      env:
        CLICKHOUSE_HOST: localhost:9000

  load-tests:
    runs-on: ubuntu-latest
    if: github.event_name == 'push' && github.ref == 'refs/heads/main'

    steps:
    - uses: actions/checkout@v3

    - name: Build Docker image
      run: docker build -t luminate:test .

    - name: Start Luminate
      run: |
        docker run -d -p 8080:8080 --name luminate luminate:test
        sleep 10

    - name: Install k6
      run: |
        wget https://github.com/grafana/k6/releases/download/v0.46.0/k6-v0.46.0-linux-amd64.tar.gz
        tar -xzf k6-v0.46.0-linux-amd64.tar.gz
        sudo mv k6-v0.46.0-linux-amd64/k6 /usr/local/bin/

    - name: Run load tests
      run: BASE_URL=http://localhost:8080 ./test/load/run_load_tests.sh

    - name: Stop Luminate
      run: docker stop luminate

  lint:
    runs-on: ubuntu-latest
    steps:
    - uses: actions/checkout@v3

    - name: Set up Go
      uses: actions/setup-go@v4
      with:
        go-version: '1.21'

    - name: Run golangci-lint
      uses: golangci/golangci-lint-action@v3
      with:
        version: latest
```

### 7. Makefile Test Targets

**File:** `Makefile` (test section)

```makefile
.PHONY: test test-unit test-integration test-coverage test-load

# Run all unit tests
test:
	go test -v -race ./pkg/...

# Run specific package tests
test-pkg:
	go test -v -race ./pkg/$(PKG)/...

# Run integration tests
test-integration:
	go test -v -tags=integration ./test/integration/...

# Generate coverage report
test-coverage:
	go test -v -race -coverprofile=coverage.out -covermode=atomic ./pkg/...
	go tool cover -html=coverage.out -o coverage.html
	go tool cover -func=coverage.out

# Run load tests
test-load:
	./test/load/run_load_tests.sh

# Run chaos tests
test-chaos:
	go test -v -tags=chaos ./test/chaos/...

# Run all tests (unit + integration + chaos)
test-all: test test-integration test-chaos

# Watch mode (requires entr or similar)
test-watch:
	find . -name '*.go' | entr -c make test
```

## Acceptance Criteria

- [ ] 80%+ code coverage across all packages
- [ ] All unit tests pass with race detector enabled
- [ ] Integration tests cover end-to-end workflows
- [ ] Load tests validate 100K writes/sec target (ClickHouse)
- [ ] Load tests validate p95 query latency < 100ms
- [ ] Chaos tests verify graceful degradation
- [ ] CI/CD pipeline runs all tests on every PR
- [ ] Coverage reports uploaded to Codecov
- [ ] Test data generators produce realistic scenarios
- [ ] Mock implementations available for all interfaces

## Performance Benchmarks

**File:** `pkg/storage/badger/benchmark_test.go`

```go
package badger

import (
    "context"
    "testing"
    "time"

    "github.com/yourusername/luminate/pkg/testutil"
)

func BenchmarkWrite(b *testing.B) {
    store, _ := NewBadgerStore(Config{Path: b.TempDir(), Memory: true})
    defer store.Close()

    gen := testutil.NewMetricGenerator(42)
    metrics := gen.Generate(1000, "bench_metric")
    ctx := context.Background()

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        store.Write(ctx, metrics)
    }
}

func BenchmarkQueryRange(b *testing.B) {
    store, _ := NewBadgerStore(Config{Path: b.TempDir(), Memory: true})
    defer store.Close()

    gen := testutil.NewMetricGenerator(42)
    metrics := gen.GenerateTimeRange("bench_metric",
        time.Now().Add(-1*time.Hour),
        time.Now(),
        10*time.Second)

    ctx := context.Background()
    store.Write(ctx, metrics)

    req := storage.QueryRequest{
        MetricName: "bench_metric",
        Start:      time.Now().Add(-1 * time.Hour),
        End:        time.Now(),
    }

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        store.QueryRange(ctx, req)
    }
}
```

## Summary

This workstream establishes a comprehensive testing framework that ensures Luminate's reliability and performance through:
- Extensive unit and integration test coverage
- Realistic load testing scenarios
- Chaos engineering for failure modes
- Automated CI/CD pipelines
- Performance benchmarking

**Total Estimated Effort:** 5-6 days

**Dependencies:** All implementation workstreams (WS1-WS8)

**Deliverables:**
- Unit test suite with 80%+ coverage
- Integration test suite
- Mock implementations
- Load testing scripts (k6)
- Chaos testing scenarios
- CI/CD GitHub Actions workflow
- Test utilities and generators
- Performance benchmarks
