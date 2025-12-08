# WS3: HTTP API Handlers

**Priority:** P0 (Critical Path)
**Estimated Effort:** 5-7 days
**Dependencies:** WS1 (Storage Backend), WS2 (Core Data Models)

## Overview

This workstream implements all HTTP API endpoints for Luminate's observability system. The API provides a unified query interface, batch write capabilities, discovery endpoints, health checks, and admin operations. The implementation focuses on clean error handling, proper middleware integration, and production-ready request/response patterns.

## Architecture

```
┌─────────────────────────────────────────────────────────┐
│                   HTTP Router (Chi)                     │
└─────────────────────────────────────────────────────────┘
                         ↓
┌─────────────────────────────────────────────────────────┐
│                  Middleware Chain                       │
│  Recovery → Logger → RateLimit → Auth → CORS           │
└─────────────────────────────────────────────────────────┘
                         ↓
┌─────────────────────────────────────────────────────────┐
│                   API Handlers                          │
│                                                         │
│  POST   /api/v1/query          - Unified query         │
│  POST   /api/v1/write          - Batch write           │
│  GET    /api/v1/metrics        - List metrics          │
│  GET    /api/v1/metrics/{name}/dimensions              │
│  GET    /api/v1/metrics/{name}/dimensions/{key}/values │
│  GET    /api/v1/health         - Health check          │
│  POST   /api/v1/admin/retention                        │
│  DELETE /api/v1/admin/metrics/{name}                   │
│  GET    /metrics               - Prometheus metrics    │
└─────────────────────────────────────────────────────────┘
                         ↓
┌─────────────────────────────────────────────────────────┐
│              MetricsStore Interface                     │
│  (BadgerDB or ClickHouse Implementation)               │
└─────────────────────────────────────────────────────────┘
```

## Work Items

### Work Item 1: Unified Query Endpoint

**Endpoint:** `POST /api/v1/query`

**Purpose:** Single endpoint that handles all query types (range, aggregate, rate) based on `queryType` discriminator in JSON request body.

**Technical Specification:**

**Request Format:**
```json
{
  "queryType": "aggregate|range|rate",
  "metricName": "api_latency",
  "timeRange": {
    "start": "2024-01-01T00:00:00Z",
    "end": "2024-01-01T23:59:59Z",
    "relative": "1h|24h|7d|30d"
  },
  "filters": {
    "customer_id": "cust_123",
    "region": "us-west-2"
  },
  "aggregation": "avg|sum|count|min|max|p50|p95|p99|integral",
  "groupBy": ["customer_id", "endpoint"],
  "interval": "1m|5m|1h",
  "limit": 100,
  "timeout": "30s"
}
```

**Response Format:**
```json
{
  "queryType": "aggregate",
  "results": [
    {
      "groupKey": {"customer_id": "cust_123"},
      "value": 145.5,
      "count": 1234
    }
  ],
  "executionTimeMs": 23,
  "pointsScanned": 5678
}
```

**Implementation:**

```go
// pkg/api/query.go
package api

import (
    "context"
    "encoding/json"
    "net/http"
    "time"

    "github.com/yourusername/luminate/pkg/models"
    "github.com/yourusername/luminate/pkg/storage"
)

// JSONQuery represents the unified query request
type JSONQuery struct {
    QueryType   string            `json:"queryType"`   // "aggregate", "range", "rate"
    MetricName  string            `json:"metricName"`
    TimeRange   TimeRangeSpec     `json:"timeRange"`
    Filters     map[string]string `json:"filters,omitempty"`
    Aggregation string            `json:"aggregation,omitempty"` // For aggregate queries
    GroupBy     []string          `json:"groupBy,omitempty"`
    Interval    string            `json:"interval,omitempty"` // For rate queries
    Limit       int               `json:"limit,omitempty"`
    Timeout     string            `json:"timeout,omitempty"`
}

// TimeRangeSpec supports both absolute and relative time ranges
type TimeRangeSpec struct {
    Start    *time.Time `json:"start,omitempty"`
    End      *time.Time `json:"end,omitempty"`
    Relative string     `json:"relative,omitempty"` // "1h", "24h", "7d", "30d"
}

// GetAbsoluteRange converts relative or absolute time range to start/end
func (t *TimeRangeSpec) GetAbsoluteRange() (time.Time, time.Time, error) {
    if t.Relative != "" {
        duration, err := parseRelativeTime(t.Relative)
        if err != nil {
            return time.Time{}, time.Time{}, err
        }
        end := time.Now()
        start := end.Add(-duration)
        return start, end, nil
    }

    if t.Start != nil && t.End != nil {
        if t.End.Before(*t.Start) {
            return time.Time{}, time.Time{}, models.ErrInvalidTimeRange
        }
        return *t.Start, *t.End, nil
    }

    return time.Time{}, time.Time{}, models.ErrInvalidTimeRange
}

// parseRelativeTime converts relative time strings to duration
func parseRelativeTime(relative string) (time.Duration, error) {
    switch relative {
    case "1h":
        return 1 * time.Hour, nil
    case "24h":
        return 24 * time.Hour, nil
    case "7d":
        return 7 * 24 * time.Hour, nil
    case "30d":
        return 30 * 24 * time.Hour, nil
    case "90d":
        return 90 * 24 * time.Hour, nil
    default:
        // Try parsing as Go duration (e.g., "2h30m")
        return time.ParseDuration(relative)
    }
}

// Validate validates the query request
func (jq *JSONQuery) Validate() error {
    if jq.QueryType == "" {
        return models.NewAPIError(models.ErrCodeInvalidQuery, "queryType is required", nil)
    }

    if jq.MetricName == "" {
        return models.NewAPIError(models.ErrCodeInvalidQuery, "metricName is required", nil)
    }

    // Validate metric name format
    if !models.IsValidMetricName(jq.MetricName) {
        return models.NewAPIError(models.ErrCodeInvalidMetricName,
            "invalid metric name format",
            map[string]interface{}{"metric_name": jq.MetricName})
    }

    // Validate time range
    _, _, err := jq.TimeRange.GetAbsoluteRange()
    if err != nil {
        return err
    }

    // Validate query type specific fields
    switch jq.QueryType {
    case "aggregate":
        if jq.Aggregation == "" {
            return models.NewAPIError(models.ErrCodeInvalidQuery,
                "aggregation is required for aggregate queries", nil)
        }
        if !isValidAggregation(jq.Aggregation) {
            return models.NewAPIError(models.ErrCodeInvalidQuery,
                "invalid aggregation type",
                map[string]interface{}{"aggregation": jq.Aggregation})
        }
    case "rate":
        if jq.Interval == "" {
            return models.NewAPIError(models.ErrCodeInvalidQuery,
                "interval is required for rate queries", nil)
        }
    case "range":
        // No additional validation
    default:
        return models.NewAPIError(models.ErrCodeInvalidQuery,
            "invalid queryType",
            map[string]interface{}{"queryType": jq.QueryType})
    }

    return nil
}

// ToStorageQuery converts JSON query to storage-specific query
func (jq *JSONQuery) ToStorageQuery(ctx context.Context) (interface{}, error) {
    start, end, err := jq.TimeRange.GetAbsoluteRange()
    if err != nil {
        return nil, err
    }

    // Parse timeout
    timeout := 10 * time.Second // Default
    if jq.Timeout != "" {
        timeout, err = time.ParseDuration(jq.Timeout)
        if err != nil {
            return nil, models.NewAPIError(models.ErrCodeInvalidQuery,
                "invalid timeout format",
                map[string]interface{}{"timeout": jq.Timeout})
        }
    }

    // Set default limit if not specified
    limit := jq.Limit
    if limit == 0 {
        limit = 1000 // Default limit
    }

    switch jq.QueryType {
    case "range":
        return storage.QueryRequest{
            MetricName: jq.MetricName,
            Start:      start,
            End:        end,
            Filters:    jq.Filters,
            Limit:      limit,
            Timeout:    timeout,
        }, nil

    case "aggregate":
        aggType, err := parseAggregationType(jq.Aggregation)
        if err != nil {
            return nil, err
        }
        return storage.AggregateRequest{
            MetricName:  jq.MetricName,
            Start:       start,
            End:         end,
            Filters:     jq.Filters,
            Aggregation: aggType,
            GroupBy:     jq.GroupBy,
            Limit:       limit,
            Timeout:     timeout,
        }, nil

    case "rate":
        interval, err := time.ParseDuration(jq.Interval)
        if err != nil {
            return nil, models.NewAPIError(models.ErrCodeInvalidQuery,
                "invalid interval format",
                map[string]interface{}{"interval": jq.Interval})
        }
        return storage.RateRequest{
            MetricName: jq.MetricName,
            Start:      start,
            End:        end,
            Filters:    jq.Filters,
            Interval:   interval,
            Timeout:    timeout,
        }, nil

    default:
        return nil, models.NewAPIError(models.ErrCodeInvalidQuery,
            "unknown query type",
            map[string]interface{}{"queryType": jq.QueryType})
    }
}

// parseAggregationType converts string to AggregationType
func parseAggregationType(agg string) (storage.AggregationType, error) {
    switch agg {
    case "avg":
        return storage.AVG, nil
    case "sum":
        return storage.SUM, nil
    case "count":
        return storage.COUNT, nil
    case "min":
        return storage.MIN, nil
    case "max":
        return storage.MAX, nil
    case "p50":
        return storage.P50, nil
    case "p95":
        return storage.P95, nil
    case "p99":
        return storage.P99, nil
    case "integral":
        return storage.INTEGRAL, nil
    default:
        return 0, models.NewAPIError(models.ErrCodeInvalidQuery,
            "invalid aggregation type",
            map[string]interface{}{"aggregation": agg})
    }
}

// isValidAggregation checks if aggregation type is valid
func isValidAggregation(agg string) bool {
    valid := []string{"avg", "sum", "count", "min", "max", "p50", "p95", "p99", "integral"}
    for _, v := range valid {
        if agg == v {
            return true
        }
    }
    return false
}

// QueryResponse is the unified response format
type QueryResponse struct {
    QueryType       string      `json:"queryType"`
    Results         interface{} `json:"results"`
    ExecutionTimeMs int64       `json:"executionTimeMs"`
    PointsScanned   int64       `json:"pointsScanned,omitempty"`
}

// QueryHandler handles the unified query endpoint
type QueryHandler struct {
    store storage.MetricsStore
}

// NewQueryHandler creates a new query handler
func NewQueryHandler(store storage.MetricsStore) *QueryHandler {
    return &QueryHandler{store: store}
}

// HandleQuery processes the unified query endpoint
func (h *QueryHandler) HandleQuery(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()
    startTime := time.Now()

    // Parse request
    var jq JSONQuery
    if err := json.NewDecoder(r.Body).Decode(&jq); err != nil {
        writeError(w, http.StatusBadRequest, models.ErrCodeInvalidQuery,
            "invalid JSON request", map[string]interface{}{"error": err.Error()})
        return
    }

    // Validate request
    if err := jq.Validate(); err != nil {
        apiErr, ok := err.(*models.APIError)
        if ok {
            writeError(w, http.StatusBadRequest, apiErr.Code, apiErr.Message, apiErr.Details)
        } else {
            writeError(w, http.StatusBadRequest, models.ErrCodeInvalidQuery, err.Error(), nil)
        }
        return
    }

    // Convert to storage query
    storageQuery, err := jq.ToStorageQuery(ctx)
    if err != nil {
        apiErr, ok := err.(*models.APIError)
        if ok {
            writeError(w, http.StatusBadRequest, apiErr.Code, apiErr.Message, apiErr.Details)
        } else {
            writeError(w, http.StatusBadRequest, models.ErrCodeInvalidQuery, err.Error(), nil)
        }
        return
    }

    // Execute query based on type
    var result interface{}
    var pointsScanned int64

    switch q := storageQuery.(type) {
    case storage.QueryRequest:
        points, err := h.store.QueryRange(ctx, q)
        if err != nil {
            handleStorageError(w, err)
            return
        }
        result = points
        pointsScanned = int64(len(points))

    case storage.AggregateRequest:
        aggResults, err := h.store.Aggregate(ctx, q)
        if err != nil {
            handleStorageError(w, err)
            return
        }
        result = aggResults

    case storage.RateRequest:
        rates, err := h.store.Rate(ctx, q)
        if err != nil {
            handleStorageError(w, err)
            return
        }
        result = rates
        pointsScanned = int64(len(rates))
    }

    // Build response
    executionTime := time.Since(startTime).Milliseconds()
    response := QueryResponse{
        QueryType:       jq.QueryType,
        Results:         result,
        ExecutionTimeMs: executionTime,
        PointsScanned:   pointsScanned,
    }

    // Write response
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(http.StatusOK)
    json.NewEncoder(w).Encode(response)
}
```

**Tests:**

```go
// pkg/api/query_test.go
package api

import (
    "bytes"
    "context"
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "testing"
    "time"

    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/mock"
    "github.com/yourusername/luminate/pkg/storage"
)

// MockMetricsStore for testing
type MockMetricsStore struct {
    mock.Mock
}

func (m *MockMetricsStore) QueryRange(ctx context.Context, req storage.QueryRequest) ([]storage.MetricPoint, error) {
    args := m.Called(ctx, req)
    return args.Get(0).([]storage.MetricPoint), args.Error(1)
}

func (m *MockMetricsStore) Aggregate(ctx context.Context, req storage.AggregateRequest) ([]storage.AggregateResult, error) {
    args := m.Called(ctx, req)
    return args.Get(0).([]storage.AggregateResult), args.Error(1)
}

func (m *MockMetricsStore) Rate(ctx context.Context, req storage.RateRequest) ([]storage.RatePoint, error) {
    args := m.Called(ctx, req)
    return args.Get(0).([]storage.RatePoint), args.Error(1)
}

// Implement other interface methods...
func (m *MockMetricsStore) Write(ctx context.Context, metrics []storage.Metric) error {
    args := m.Called(ctx, metrics)
    return args.Error(0)
}

func (m *MockMetricsStore) ListMetrics(ctx context.Context) ([]string, error) {
    args := m.Called(ctx)
    return args.Get(0).([]string), args.Error(1)
}

func (m *MockMetricsStore) ListDimensionKeys(ctx context.Context, metricName string) ([]string, error) {
    args := m.Called(ctx, metricName)
    return args.Get(0).([]string), args.Error(1)
}

func (m *MockMetricsStore) ListDimensionValues(ctx context.Context, metricName, dimensionKey string, limit int) ([]string, error) {
    args := m.Called(ctx, metricName, dimensionKey, limit)
    return args.Get(0).([]string), args.Error(1)
}

func (m *MockMetricsStore) DeleteBefore(ctx context.Context, metricName string, before time.Time) error {
    args := m.Called(ctx, metricName, before)
    return args.Error(0)
}

func (m *MockMetricsStore) Health(ctx context.Context) error {
    args := m.Called(ctx)
    return args.Error(0)
}

func (m *MockMetricsStore) Close() error {
    args := m.Called()
    return args.Error(0)
}

func TestHandleQuery_RangeQuery(t *testing.T) {
    mockStore := new(MockMetricsStore)
    handler := NewQueryHandler(mockStore)

    // Mock data
    expectedPoints := []storage.MetricPoint{
        {
            Timestamp:  time.Now(),
            Value:      100.5,
            Dimensions: map[string]string{"customer_id": "cust_123"},
        },
    }

    mockStore.On("QueryRange", mock.Anything, mock.MatchedBy(func(req storage.QueryRequest) bool {
        return req.MetricName == "cpu_usage"
    })).Return(expectedPoints, nil)

    // Create request
    query := JSONQuery{
        QueryType:  "range",
        MetricName: "cpu_usage",
        TimeRange: TimeRangeSpec{
            Relative: "1h",
        },
        Limit: 1000,
    }

    body, _ := json.Marshal(query)
    req := httptest.NewRequest("POST", "/api/v1/query", bytes.NewReader(body))
    req.Header.Set("Content-Type", "application/json")

    // Execute
    w := httptest.NewRecorder()
    handler.HandleQuery(w, req)

    // Assert
    assert.Equal(t, http.StatusOK, w.Code)

    var response QueryResponse
    json.NewDecoder(w.Body).Decode(&response)

    assert.Equal(t, "range", response.QueryType)
    assert.NotNil(t, response.Results)
    assert.Greater(t, response.ExecutionTimeMs, int64(0))

    mockStore.AssertExpectations(t)
}

func TestHandleQuery_AggregateQuery(t *testing.T) {
    mockStore := new(MockMetricsStore)
    handler := NewQueryHandler(mockStore)

    // Mock data
    expectedResults := []storage.AggregateResult{
        {
            GroupKey: map[string]string{"customer_id": "cust_123"},
            Value:    145.5,
            Count:    1234,
        },
    }

    mockStore.On("Aggregate", mock.Anything, mock.MatchedBy(func(req storage.AggregateRequest) bool {
        return req.MetricName == "api_latency" && req.Aggregation == storage.AVG
    })).Return(expectedResults, nil)

    // Create request
    query := JSONQuery{
        QueryType:   "aggregate",
        MetricName:  "api_latency",
        TimeRange:   TimeRangeSpec{Relative: "24h"},
        Aggregation: "avg",
        GroupBy:     []string{"customer_id"},
        Limit:       100,
    }

    body, _ := json.Marshal(query)
    req := httptest.NewRequest("POST", "/api/v1/query", bytes.NewReader(body))
    req.Header.Set("Content-Type", "application/json")

    // Execute
    w := httptest.NewRecorder()
    handler.HandleQuery(w, req)

    // Assert
    assert.Equal(t, http.StatusOK, w.Code)

    var response QueryResponse
    json.NewDecoder(w.Body).Decode(&response)

    assert.Equal(t, "aggregate", response.QueryType)
    mockStore.AssertExpectations(t)
}

func TestHandleQuery_InvalidQueryType(t *testing.T) {
    mockStore := new(MockMetricsStore)
    handler := NewQueryHandler(mockStore)

    query := JSONQuery{
        QueryType:  "invalid",
        MetricName: "cpu_usage",
        TimeRange:  TimeRangeSpec{Relative: "1h"},
    }

    body, _ := json.Marshal(query)
    req := httptest.NewRequest("POST", "/api/v1/query", bytes.NewReader(body))

    w := httptest.NewRecorder()
    handler.HandleQuery(w, req)

    assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestTimeRangeSpec_Relative(t *testing.T) {
    tests := []struct {
        name     string
        relative string
        wantDur  time.Duration
        wantErr  bool
    }{
        {"1 hour", "1h", 1 * time.Hour, false},
        {"24 hours", "24h", 24 * time.Hour, false},
        {"7 days", "7d", 7 * 24 * time.Hour, false},
        {"30 days", "30d", 30 * 24 * time.Hour, false},
        {"custom", "2h30m", 2*time.Hour + 30*time.Minute, false},
        {"invalid", "invalid", 0, true},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            tr := TimeRangeSpec{Relative: tt.relative}
            start, end, err := tr.GetAbsoluteRange()

            if tt.wantErr {
                assert.Error(t, err)
            } else {
                assert.NoError(t, err)
                assert.InDelta(t, tt.wantDur.Seconds(), end.Sub(start).Seconds(), 1.0)
            }
        })
    }
}
```

**Acceptance Criteria:**
- ✅ Single endpoint handles all three query types
- ✅ Supports both relative ("1h", "24h") and absolute time ranges
- ✅ Validates all required fields based on query type
- ✅ Returns consistent JSON response format
- ✅ Includes execution time in response
- ✅ Proper error handling with specific error codes
- ✅ Unit tests with 80%+ coverage

---

### Work Item 2: Batch Write Endpoint

**Endpoint:** `POST /api/v1/write`

**Purpose:** Ingest metrics in batches with validation, compression support, and partial success handling.

**Technical Specification:**

**Request Format:**
```json
{
  "metrics": [
    {
      "name": "api_latency",
      "timestamp": 1735603200000,
      "value": 145.5,
      "dimensions": {
        "customer_id": "cust_123",
        "endpoint": "/api/users"
      }
    },
    {
      "name": "cpu_usage",
      "timestamp": 1735603200000,
      "value": 75.2,
      "dimensions": {
        "customer_id": "cust_123"
      }
    }
  ]
}
```

**Response Format:**
```json
{
  "accepted": 2,
  "rejected": 0,
  "errors": []
}
```

**Partial Success (207 Multi-Status):**
```json
{
  "accepted": 1,
  "rejected": 1,
  "errors": [
    {
      "index": 1,
      "metric": "cpu_usage",
      "error": "timestamp too old"
    }
  ]
}
```

**Implementation:**

```go
// pkg/api/write.go
package api

import (
    "compress/gzip"
    "encoding/json"
    "io"
    "net/http"
    "time"

    "github.com/yourusername/luminate/pkg/models"
    "github.com/yourusername/luminate/pkg/storage"
)

const (
    MaxBatchSize       = 10000
    MaxRequestBodySize = 10 * 1024 * 1024 // 10 MB
)

// BatchWriteRequest represents a batch write request
type BatchWriteRequest struct {
    Metrics []models.Metric `json:"metrics"`
}

// BatchWriteResponse represents the write response
type BatchWriteResponse struct {
    Accepted int                `json:"accepted"`
    Rejected int                `json:"rejected"`
    Errors   []MetricWriteError `json:"errors,omitempty"`
}

// MetricWriteError represents an error for a specific metric
type MetricWriteError struct {
    Index  int    `json:"index"`
    Metric string `json:"metric"`
    Error  string `json:"error"`
}

// WriteHandler handles metric ingestion
type WriteHandler struct {
    store storage.MetricsStore
}

// NewWriteHandler creates a new write handler
func NewWriteHandler(store storage.MetricsStore) *WriteHandler {
    return &WriteHandler{store: store}
}

// HandleWrite processes batch write requests
func (h *WriteHandler) HandleWrite(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()

    // Check request size
    if r.ContentLength > MaxRequestBodySize {
        writeError(w, http.StatusRequestEntityTooLarge, models.ErrCodeBatchTooLarge,
            "request body too large",
            map[string]interface{}{
                "max_size_bytes": MaxRequestBodySize,
                "received_bytes": r.ContentLength,
            })
        return
    }

    // Handle gzip compression
    var reader io.Reader = r.Body
    if r.Header.Get("Content-Encoding") == "gzip" {
        gzipReader, err := gzip.NewReader(r.Body)
        if err != nil {
            writeError(w, http.StatusBadRequest, models.ErrCodeInvalidRequest,
                "invalid gzip encoding", map[string]interface{}{"error": err.Error()})
            return
        }
        defer gzipReader.Close()
        reader = gzipReader
    }

    // Parse request
    var req BatchWriteRequest
    decoder := json.NewDecoder(reader)
    if err := decoder.Decode(&req); err != nil {
        writeError(w, http.StatusBadRequest, models.ErrCodeInvalidRequest,
            "invalid JSON request", map[string]interface{}{"error": err.Error()})
        return
    }

    // Validate batch size
    if len(req.Metrics) == 0 {
        writeError(w, http.StatusBadRequest, models.ErrCodeInvalidRequest,
            "metrics array cannot be empty", nil)
        return
    }

    if len(req.Metrics) > MaxBatchSize {
        writeError(w, http.StatusBadRequest, models.ErrCodeBatchTooLarge,
            "batch size exceeds limit",
            map[string]interface{}{
                "max_batch_size": MaxBatchSize,
                "received":       len(req.Metrics),
            })
        return
    }

    // Validate and filter metrics
    validMetrics := make([]storage.Metric, 0, len(req.Metrics))
    var errors []MetricWriteError

    for i, metric := range req.Metrics {
        if err := metric.Validate(); err != nil {
            errors = append(errors, MetricWriteError{
                Index:  i,
                Metric: metric.Name,
                Error:  err.Error(),
            })
            continue
        }

        // Convert to storage metric
        validMetrics = append(validMetrics, storage.Metric{
            Name:       metric.Name,
            Timestamp:  metric.Timestamp,
            Value:      metric.Value,
            Dimensions: metric.Dimensions,
        })
    }

    // Write valid metrics
    var writeErr error
    if len(validMetrics) > 0 {
        writeErr = h.store.Write(ctx, validMetrics)
    }

    // Build response
    response := BatchWriteResponse{
        Accepted: len(validMetrics),
        Rejected: len(errors),
        Errors:   errors,
    }

    // Determine status code
    statusCode := http.StatusOK
    if writeErr != nil {
        // Complete failure
        statusCode = http.StatusInternalServerError
        writeError(w, statusCode, models.ErrCodeStorageError,
            "failed to write metrics", map[string]interface{}{"error": writeErr.Error()})
        return
    } else if len(errors) > 0 && len(validMetrics) > 0 {
        // Partial success
        statusCode = http.StatusMultiStatus
    } else if len(errors) > 0 {
        // All rejected
        statusCode = http.StatusBadRequest
    }

    // Write response
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(statusCode)
    json.NewEncoder(w).Encode(response)
}
```

**Tests:**

```go
// pkg/api/write_test.go
package api

import (
    "bytes"
    "compress/gzip"
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "testing"
    "time"

    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/mock"
    "github.com/yourusername/luminate/pkg/models"
)

func TestHandleWrite_Success(t *testing.T) {
    mockStore := new(MockMetricsStore)
    handler := NewWriteHandler(mockStore)

    mockStore.On("Write", mock.Anything, mock.MatchedBy(func(metrics []storage.Metric) bool {
        return len(metrics) == 2
    })).Return(nil)

    // Create request
    req := BatchWriteRequest{
        Metrics: []models.Metric{
            {
                Name:       "api_latency",
                Timestamp:  time.Now(),
                Value:      145.5,
                Dimensions: map[string]string{"customer_id": "cust_123"},
            },
            {
                Name:       "cpu_usage",
                Timestamp:  time.Now(),
                Value:      75.2,
                Dimensions: map[string]string{"customer_id": "cust_123"},
            },
        },
    }

    body, _ := json.Marshal(req)
    httpReq := httptest.NewRequest("POST", "/api/v1/write", bytes.NewReader(body))
    httpReq.Header.Set("Content-Type", "application/json")

    // Execute
    w := httptest.NewRecorder()
    handler.HandleWrite(w, httpReq)

    // Assert
    assert.Equal(t, http.StatusOK, w.Code)

    var response BatchWriteResponse
    json.NewDecoder(w.Body).Decode(&response)

    assert.Equal(t, 2, response.Accepted)
    assert.Equal(t, 0, response.Rejected)
    assert.Empty(t, response.Errors)

    mockStore.AssertExpectations(t)
}

func TestHandleWrite_PartialSuccess(t *testing.T) {
    mockStore := new(MockMetricsStore)
    handler := NewWriteHandler(mockStore)

    mockStore.On("Write", mock.Anything, mock.MatchedBy(func(metrics []storage.Metric) bool {
        return len(metrics) == 1 // Only one valid metric
    })).Return(nil)

    // Create request with one valid and one invalid metric
    req := BatchWriteRequest{
        Metrics: []models.Metric{
            {
                Name:       "api_latency",
                Timestamp:  time.Now(),
                Value:      145.5,
                Dimensions: map[string]string{"customer_id": "cust_123"},
            },
            {
                Name:       "cpu_usage",
                Timestamp:  time.Now().Add(-8 * 24 * time.Hour), // Too old!
                Value:      75.2,
                Dimensions: map[string]string{"customer_id": "cust_123"},
            },
        },
    }

    body, _ := json.Marshal(req)
    httpReq := httptest.NewRequest("POST", "/api/v1/write", bytes.NewReader(body))
    httpReq.Header.Set("Content-Type", "application/json")

    // Execute
    w := httptest.NewRecorder()
    handler.HandleWrite(w, httpReq)

    // Assert
    assert.Equal(t, http.StatusMultiStatus, w.Code)

    var response BatchWriteResponse
    json.NewDecoder(w.Body).Decode(&response)

    assert.Equal(t, 1, response.Accepted)
    assert.Equal(t, 1, response.Rejected)
    assert.Len(t, response.Errors, 1)
    assert.Equal(t, 1, response.Errors[0].Index)

    mockStore.AssertExpectations(t)
}

func TestHandleWrite_GzipCompression(t *testing.T) {
    mockStore := new(MockMetricsStore)
    handler := NewWriteHandler(mockStore)

    mockStore.On("Write", mock.Anything, mock.Anything).Return(nil)

    // Create compressed request
    req := BatchWriteRequest{
        Metrics: []models.Metric{
            {
                Name:       "api_latency",
                Timestamp:  time.Now(),
                Value:      145.5,
                Dimensions: map[string]string{"customer_id": "cust_123"},
            },
        },
    }

    body, _ := json.Marshal(req)

    // Compress body
    var buf bytes.Buffer
    gzipWriter := gzip.NewWriter(&buf)
    gzipWriter.Write(body)
    gzipWriter.Close()

    httpReq := httptest.NewRequest("POST", "/api/v1/write", &buf)
    httpReq.Header.Set("Content-Type", "application/json")
    httpReq.Header.Set("Content-Encoding", "gzip")

    // Execute
    w := httptest.NewRecorder()
    handler.HandleWrite(w, httpReq)

    // Assert
    assert.Equal(t, http.StatusOK, w.Code)
    mockStore.AssertExpectations(t)
}

func TestHandleWrite_BatchTooLarge(t *testing.T) {
    mockStore := new(MockMetricsStore)
    handler := NewWriteHandler(mockStore)

    // Create oversized batch
    metrics := make([]models.Metric, MaxBatchSize+1)
    for i := range metrics {
        metrics[i] = models.Metric{
            Name:      "test_metric",
            Timestamp: time.Now(),
            Value:     float64(i),
        }
    }

    req := BatchWriteRequest{Metrics: metrics}
    body, _ := json.Marshal(req)

    httpReq := httptest.NewRequest("POST", "/api/v1/write", bytes.NewReader(body))
    httpReq.Header.Set("Content-Type", "application/json")

    // Execute
    w := httptest.NewRecorder()
    handler.HandleWrite(w, httpReq)

    // Assert
    assert.Equal(t, http.StatusBadRequest, w.Code)
}
```

**Acceptance Criteria:**
- ✅ Accepts batches up to 10,000 metrics
- ✅ Supports gzip compression (Content-Encoding: gzip)
- ✅ Validates all metrics before writing
- ✅ Returns partial success (207) when some metrics fail validation
- ✅ Enforces 10 MB max request body size
- ✅ Includes detailed error information for rejected metrics
- ✅ Handles write failures gracefully

---

### Work Item 3: Discovery Endpoints

**Purpose:** Enable UI/dashboard builders to discover available metrics, dimensions, and dimension values.

**Endpoints:**
1. `GET /api/v1/metrics` - List all metrics
2. `GET /api/v1/metrics/{name}/dimensions` - List dimension keys
3. `GET /api/v1/metrics/{name}/dimensions/{key}/values` - List dimension values

**Implementation:**

```go
// pkg/api/discovery.go
package api

import (
    "encoding/json"
    "net/http"
    "strconv"

    "github.com/go-chi/chi/v5"
    "github.com/yourusername/luminate/pkg/models"
    "github.com/yourusername/luminate/pkg/storage"
)

// DiscoveryHandler handles metric discovery endpoints
type DiscoveryHandler struct {
    store storage.MetricsStore
}

// NewDiscoveryHandler creates a new discovery handler
func NewDiscoveryHandler(store storage.MetricsStore) *DiscoveryHandler {
    return &DiscoveryHandler{store: store}
}

// ListMetricsResponse represents the list metrics response
type ListMetricsResponse struct {
    Metrics []string `json:"metrics"`
    Count   int      `json:"count"`
}

// HandleListMetrics lists all metric names
func (h *DiscoveryHandler) HandleListMetrics(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()

    metrics, err := h.store.ListMetrics(ctx)
    if err != nil {
        handleStorageError(w, err)
        return
    }

    response := ListMetricsResponse{
        Metrics: metrics,
        Count:   len(metrics),
    }

    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(http.StatusOK)
    json.NewEncoder(w).Encode(response)
}

// ListDimensionsResponse represents the list dimensions response
type ListDimensionsResponse struct {
    MetricName string   `json:"metricName"`
    Dimensions []string `json:"dimensions"`
    Count      int      `json:"count"`
}

// HandleListDimensions lists dimension keys for a metric
func (h *DiscoveryHandler) HandleListDimensions(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()
    metricName := chi.URLParam(r, "name")

    if metricName == "" {
        writeError(w, http.StatusBadRequest, models.ErrCodeInvalidRequest,
            "metric name is required", nil)
        return
    }

    dimensions, err := h.store.ListDimensionKeys(ctx, metricName)
    if err != nil {
        handleStorageError(w, err)
        return
    }

    response := ListDimensionsResponse{
        MetricName: metricName,
        Dimensions: dimensions,
        Count:      len(dimensions),
    }

    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(http.StatusOK)
    json.NewEncoder(w).Encode(response)
}

// ListDimensionValuesResponse represents the list dimension values response
type ListDimensionValuesResponse struct {
    MetricName   string   `json:"metricName"`
    DimensionKey string   `json:"dimensionKey"`
    Values       []string `json:"values"`
    Count        int      `json:"count"`
    Limit        int      `json:"limit"`
}

// HandleListDimensionValues lists dimension values for autocomplete
func (h *DiscoveryHandler) HandleListDimensionValues(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()
    metricName := chi.URLParam(r, "name")
    dimensionKey := chi.URLParam(r, "key")

    if metricName == "" || dimensionKey == "" {
        writeError(w, http.StatusBadRequest, models.ErrCodeInvalidRequest,
            "metric name and dimension key are required", nil)
        return
    }

    // Parse limit from query params
    limit := 100 // Default
    if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
        parsedLimit, err := strconv.Atoi(limitStr)
        if err == nil && parsedLimit > 0 && parsedLimit <= 1000 {
            limit = parsedLimit
        }
    }

    values, err := h.store.ListDimensionValues(ctx, metricName, dimensionKey, limit)
    if err != nil {
        handleStorageError(w, err)
        return
    }

    response := ListDimensionValuesResponse{
        MetricName:   metricName,
        DimensionKey: dimensionKey,
        Values:       values,
        Count:        len(values),
        Limit:        limit,
    }

    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(http.StatusOK)
    json.NewEncoder(w).Encode(response)
}
```

**Tests:**

```go
// pkg/api/discovery_test.go
package api

import (
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "testing"

    "github.com/go-chi/chi/v5"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/mock"
)

func TestHandleListMetrics(t *testing.T) {
    mockStore := new(MockMetricsStore)
    handler := NewDiscoveryHandler(mockStore)

    expectedMetrics := []string{"cpu_usage", "api_latency", "memory_usage"}
    mockStore.On("ListMetrics", mock.Anything).Return(expectedMetrics, nil)

    req := httptest.NewRequest("GET", "/api/v1/metrics", nil)
    w := httptest.NewRecorder()

    handler.HandleListMetrics(w, req)

    assert.Equal(t, http.StatusOK, w.Code)

    var response ListMetricsResponse
    json.NewDecoder(w.Body).Decode(&response)

    assert.Equal(t, 3, response.Count)
    assert.ElementsMatch(t, expectedMetrics, response.Metrics)

    mockStore.AssertExpectations(t)
}

func TestHandleListDimensions(t *testing.T) {
    mockStore := new(MockMetricsStore)
    handler := NewDiscoveryHandler(mockStore)

    expectedDimensions := []string{"customer_id", "endpoint", "status_code"}
    mockStore.On("ListDimensionKeys", mock.Anything, "api_latency").
        Return(expectedDimensions, nil)

    // Setup Chi router for URL params
    r := chi.NewRouter()
    r.Get("/api/v1/metrics/{name}/dimensions", handler.HandleListDimensions)

    req := httptest.NewRequest("GET", "/api/v1/metrics/api_latency/dimensions", nil)
    w := httptest.NewRecorder()

    r.ServeHTTP(w, req)

    assert.Equal(t, http.StatusOK, w.Code)

    var response ListDimensionsResponse
    json.NewDecoder(w.Body).Decode(&response)

    assert.Equal(t, "api_latency", response.MetricName)
    assert.Equal(t, 3, response.Count)
    assert.ElementsMatch(t, expectedDimensions, response.Dimensions)

    mockStore.AssertExpectations(t)
}

func TestHandleListDimensionValues(t *testing.T) {
    mockStore := new(MockMetricsStore)
    handler := NewDiscoveryHandler(mockStore)

    expectedValues := []string{"cust_123", "cust_456", "cust_789"}
    mockStore.On("ListDimensionValues", mock.Anything, "api_latency", "customer_id", 100).
        Return(expectedValues, nil)

    r := chi.NewRouter()
    r.Get("/api/v1/metrics/{name}/dimensions/{key}/values", handler.HandleListDimensionValues)

    req := httptest.NewRequest("GET", "/api/v1/metrics/api_latency/dimensions/customer_id/values", nil)
    w := httptest.NewRecorder()

    r.ServeHTTP(w, req)

    assert.Equal(t, http.StatusOK, w.Code)

    var response ListDimensionValuesResponse
    json.NewDecoder(w.Body).Decode(&response)

    assert.Equal(t, "api_latency", response.MetricName)
    assert.Equal(t, "customer_id", response.DimensionKey)
    assert.Equal(t, 3, response.Count)
    assert.Equal(t, 100, response.Limit)
    assert.ElementsMatch(t, expectedValues, response.Values)

    mockStore.AssertExpectations(t)
}

func TestHandleListDimensionValues_CustomLimit(t *testing.T) {
    mockStore := new(MockMetricsStore)
    handler := NewDiscoveryHandler(mockStore)

    mockStore.On("ListDimensionValues", mock.Anything, "api_latency", "customer_id", 50).
        Return([]string{"cust_123"}, nil)

    r := chi.NewRouter()
    r.Get("/api/v1/metrics/{name}/dimensions/{key}/values", handler.HandleListDimensionValues)

    req := httptest.NewRequest("GET", "/api/v1/metrics/api_latency/dimensions/customer_id/values?limit=50", nil)
    w := httptest.NewRecorder()

    r.ServeHTTP(w, req)

    assert.Equal(t, http.StatusOK, w.Code)

    var response ListDimensionValuesResponse
    json.NewDecoder(w.Body).Decode(&response)

    assert.Equal(t, 50, response.Limit)

    mockStore.AssertExpectations(t)
}
```

**Acceptance Criteria:**
- ✅ List all metric names endpoint works
- ✅ List dimensions for metric endpoint works
- ✅ List dimension values with limit endpoint works
- ✅ Supports custom limit (default 100, max 1000)
- ✅ Returns consistent JSON response format
- ✅ Proper error handling for invalid metric names

---

### Work Item 4: Health Check Endpoint

**Endpoint:** `GET /api/v1/health`

**Purpose:** Kubernetes liveness/readiness probes and monitoring.

**Implementation:**

```go
// pkg/api/health.go
package api

import (
    "encoding/json"
    "net/http"
    "time"

    "github.com/yourusername/luminate/pkg/storage"
)

// HealthHandler handles health check requests
type HealthHandler struct {
    store     storage.MetricsStore
    startTime time.Time
}

// NewHealthHandler creates a new health handler
func NewHealthHandler(store storage.MetricsStore) *HealthHandler {
    return &HealthHandler{
        store:     store,
        startTime: time.Now(),
    }
}

// HealthResponse represents the health check response
type HealthResponse struct {
    Status        string        `json:"status"` // "healthy" or "unhealthy"
    Version       string        `json:"version"`
    UptimeSeconds int64         `json:"uptimeSeconds"`
    Storage       StorageHealth `json:"storage"`
}

// StorageHealth represents storage backend health
type StorageHealth struct {
    Backend string `json:"backend"` // "badger" or "clickhouse"
    Status  string `json:"status"`  // "healthy" or "unhealthy"
    Error   string `json:"error,omitempty"`
}

// HandleHealth processes health check requests
func (h *HealthHandler) HandleHealth(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()

    // Check storage health
    storageErr := h.store.Health(ctx)
    storageStatus := "healthy"
    storageErrorMsg := ""

    if storageErr != nil {
        storageStatus = "unhealthy"
        storageErrorMsg = storageErr.Error()
    }

    // Overall status
    overallStatus := "healthy"
    if storageStatus == "unhealthy" {
        overallStatus = "unhealthy"
    }

    // Build response
    response := HealthResponse{
        Status:        overallStatus,
        Version:       "1.0.0", // TODO: Inject from build
        UptimeSeconds: int64(time.Since(h.startTime).Seconds()),
        Storage: StorageHealth{
            Backend: "badger", // TODO: Get from config
            Status:  storageStatus,
            Error:   storageErrorMsg,
        },
    }

    // Determine HTTP status code
    statusCode := http.StatusOK
    if overallStatus == "unhealthy" {
        statusCode = http.StatusServiceUnavailable
    }

    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(statusCode)
    json.NewEncoder(w).Encode(response)
}
```

**Tests:**

```go
// pkg/api/health_test.go
package api

import (
    "encoding/json"
    "errors"
    "net/http"
    "net/http/httptest"
    "testing"

    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/mock"
)

func TestHandleHealth_Healthy(t *testing.T) {
    mockStore := new(MockMetricsStore)
    handler := NewHealthHandler(mockStore)

    mockStore.On("Health", mock.Anything).Return(nil)

    req := httptest.NewRequest("GET", "/api/v1/health", nil)
    w := httptest.NewRecorder()

    handler.HandleHealth(w, req)

    assert.Equal(t, http.StatusOK, w.Code)

    var response HealthResponse
    json.NewDecoder(w.Body).Decode(&response)

    assert.Equal(t, "healthy", response.Status)
    assert.Equal(t, "healthy", response.Storage.Status)
    assert.Empty(t, response.Storage.Error)
    assert.Greater(t, response.UptimeSeconds, int64(0))

    mockStore.AssertExpectations(t)
}

func TestHandleHealth_Unhealthy(t *testing.T) {
    mockStore := new(MockMetricsStore)
    handler := NewHealthHandler(mockStore)

    mockStore.On("Health", mock.Anything).Return(errors.New("storage unavailable"))

    req := httptest.NewRequest("GET", "/api/v1/health", nil)
    w := httptest.NewRecorder()

    handler.HandleHealth(w, req)

    assert.Equal(t, http.StatusServiceUnavailable, w.Code)

    var response HealthResponse
    json.NewDecoder(w.Body).Decode(&response)

    assert.Equal(t, "unhealthy", response.Status)
    assert.Equal(t, "unhealthy", response.Storage.Status)
    assert.Equal(t, "storage unavailable", response.Storage.Error)

    mockStore.AssertExpectations(t)
}
```

**Acceptance Criteria:**
- ✅ Returns 200 OK when healthy
- ✅ Returns 503 Service Unavailable when unhealthy
- ✅ Checks storage backend health
- ✅ Includes uptime in response
- ✅ Includes version information
- ✅ Works without authentication (public endpoint)

---

### Work Item 5: Admin Endpoints

**Endpoints:**
1. `POST /api/v1/admin/retention` - Configure retention policy
2. `DELETE /api/v1/admin/metrics/{name}` - Delete old metrics

**Implementation:**

```go
// pkg/api/admin.go
package api

import (
    "encoding/json"
    "net/http"
    "time"

    "github.com/go-chi/chi/v5"
    "github.com/yourusername/luminate/pkg/models"
    "github.com/yourusername/luminate/pkg/storage"
)

// AdminHandler handles admin operations
type AdminHandler struct {
    store storage.MetricsStore
}

// NewAdminHandler creates a new admin handler
func NewAdminHandler(store storage.MetricsStore) *AdminHandler {
    return &AdminHandler{store: store}
}

// RetentionRequest represents a retention policy request
type RetentionRequest struct {
    MetricName    string `json:"metricName"`
    RetentionDays int    `json:"retentionDays"`
}

// RetentionResponse represents the retention policy response
type RetentionResponse struct {
    MetricName    string    `json:"metricName"`
    RetentionDays int       `json:"retentionDays"`
    AppliedAt     time.Time `json:"appliedAt"`
}

// HandleSetRetention configures retention policy (placeholder)
func (h *AdminHandler) HandleSetRetention(w http.ResponseWriter, r *http.Request) {
    var req RetentionRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        writeError(w, http.StatusBadRequest, models.ErrCodeInvalidRequest,
            "invalid JSON request", map[string]interface{}{"error": err.Error()})
        return
    }

    if req.MetricName == "" {
        writeError(w, http.StatusBadRequest, models.ErrCodeInvalidRequest,
            "metricName is required", nil)
        return
    }

    if req.RetentionDays <= 0 || req.RetentionDays > 365 {
        writeError(w, http.StatusBadRequest, models.ErrCodeInvalidRequest,
            "retentionDays must be between 1 and 365",
            map[string]interface{}{"retention_days": req.RetentionDays})
        return
    }

    // TODO: Store retention policy in configuration
    // For now, just return success

    response := RetentionResponse{
        MetricName:    req.MetricName,
        RetentionDays: req.RetentionDays,
        AppliedAt:     time.Now(),
    }

    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(http.StatusOK)
    json.NewEncoder(w).Encode(response)
}

// DeleteMetricsRequest for query params
type DeleteMetricsRequest struct {
    Before time.Time
}

// DeleteMetricsResponse represents the delete response
type DeleteMetricsResponse struct {
    MetricName   string    `json:"metricName"`
    DeletedCount int64     `json:"deletedCount,omitempty"`
    DeletedAt    time.Time `json:"deletedAt"`
}

// HandleDeleteMetrics deletes old metrics
func (h *AdminHandler) HandleDeleteMetrics(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()
    metricName := chi.URLParam(r, "name")

    if metricName == "" {
        writeError(w, http.StatusBadRequest, models.ErrCodeInvalidRequest,
            "metric name is required", nil)
        return
    }

    // Parse 'before' timestamp from query params
    beforeStr := r.URL.Query().Get("before")
    if beforeStr == "" {
        writeError(w, http.StatusBadRequest, models.ErrCodeInvalidRequest,
            "'before' query parameter is required (RFC3339 timestamp)", nil)
        return
    }

    before, err := time.Parse(time.RFC3339, beforeStr)
    if err != nil {
        writeError(w, http.StatusBadRequest, models.ErrCodeInvalidRequest,
            "invalid 'before' timestamp format (use RFC3339)",
            map[string]interface{}{"before": beforeStr, "error": err.Error()})
        return
    }

    // Delete metrics
    err = h.store.DeleteBefore(ctx, metricName, before)
    if err != nil {
        handleStorageError(w, err)
        return
    }

    response := DeleteMetricsResponse{
        MetricName: metricName,
        DeletedAt:  time.Now(),
    }

    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(http.StatusOK)
    json.NewEncoder(w).Encode(response)
}
```

**Tests:**

```go
// pkg/api/admin_test.go
package api

import (
    "bytes"
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "testing"
    "time"

    "github.com/go-chi/chi/v5"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/mock"
)

func TestHandleSetRetention(t *testing.T) {
    mockStore := new(MockMetricsStore)
    handler := NewAdminHandler(mockStore)

    req := RetentionRequest{
        MetricName:    "debug_metrics",
        RetentionDays: 7,
    }

    body, _ := json.Marshal(req)
    httpReq := httptest.NewRequest("POST", "/api/v1/admin/retention", bytes.NewReader(body))
    httpReq.Header.Set("Content-Type", "application/json")

    w := httptest.NewRecorder()
    handler.HandleSetRetention(w, httpReq)

    assert.Equal(t, http.StatusOK, w.Code)

    var response RetentionResponse
    json.NewDecoder(w.Body).Decode(&response)

    assert.Equal(t, "debug_metrics", response.MetricName)
    assert.Equal(t, 7, response.RetentionDays)
}

func TestHandleDeleteMetrics(t *testing.T) {
    mockStore := new(MockMetricsStore)
    handler := NewAdminHandler(mockStore)

    before := time.Now().Add(-7 * 24 * time.Hour)
    mockStore.On("DeleteBefore", mock.Anything, "old_metrics", mock.MatchedBy(func(t time.Time) bool {
        return t.Unix() == before.Unix()
    })).Return(nil)

    r := chi.NewRouter()
    r.Delete("/api/v1/admin/metrics/{name}", handler.HandleDeleteMetrics)

    httpReq := httptest.NewRequest("DELETE",
        "/api/v1/admin/metrics/old_metrics?before="+before.Format(time.RFC3339), nil)

    w := httptest.NewRecorder()
    r.ServeHTTP(w, httpReq)

    assert.Equal(t, http.StatusOK, w.Code)

    var response DeleteMetricsResponse
    json.NewDecoder(w.Body).Decode(&response)

    assert.Equal(t, "old_metrics", response.MetricName)

    mockStore.AssertExpectations(t)
}
```

**Acceptance Criteria:**
- ✅ Set retention policy endpoint works
- ✅ Delete old metrics endpoint works
- ✅ Validates retention days (1-365)
- ✅ Validates timestamp format (RFC3339)
- ✅ Requires admin authentication
- ✅ Returns clear success/error responses

---

### Work Item 6: Error Handling & Middleware

**Purpose:** Standardized error responses and request/response middleware.

**Implementation:**

```go
// pkg/api/errors.go
package api

import (
    "encoding/json"
    "net/http"

    "github.com/yourusername/luminate/pkg/models"
    "github.com/yourusername/luminate/pkg/storage"
)

// ErrorResponse represents a standardized error response
type ErrorResponse struct {
    Error ErrorDetail `json:"error"`
}

// ErrorDetail contains error details
type ErrorDetail struct {
    Code    string                 `json:"code"`
    Message string                 `json:"message"`
    Details map[string]interface{} `json:"details,omitempty"`
}

// writeError writes a standardized error response
func writeError(w http.ResponseWriter, statusCode int, code, message string, details map[string]interface{}) {
    response := ErrorResponse{
        Error: ErrorDetail{
            Code:    code,
            Message: message,
            Details: details,
        },
    }

    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(statusCode)
    json.NewEncoder(w).Encode(response)
}

// handleStorageError converts storage errors to HTTP responses
func handleStorageError(w http.ResponseWriter, err error) {
    // Check if it's a timeout
    if err == storage.ErrQueryTimeout {
        writeError(w, http.StatusGatewayTimeout, models.ErrCodeQueryTimeout,
            "query execution timeout", nil)
        return
    }

    // Check if it's a storage unavailable error
    if err == storage.ErrStorageUnavailable {
        writeError(w, http.StatusServiceUnavailable, models.ErrCodeStorageUnavailable,
            "storage backend unavailable", nil)
        return
    }

    // Generic storage error
    writeError(w, http.StatusInternalServerError, models.ErrCodeStorageError,
        "storage operation failed",
        map[string]interface{}{"error": err.Error()})
}

// pkg/api/middleware.go
package api

import (
    "log"
    "net/http"
    "time"
)

// LoggingMiddleware logs HTTP requests
func LoggingMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        start := time.Now()

        // Wrap ResponseWriter to capture status code
        ww := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

        next.ServeHTTP(ww, r)

        duration := time.Since(start)
        log.Printf("[%s] %s %s - %d (%v)",
            r.Method, r.URL.Path, r.RemoteAddr, ww.statusCode, duration)
    })
}

// responseWriter wraps http.ResponseWriter to capture status code
type responseWriter struct {
    http.ResponseWriter
    statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
    rw.statusCode = code
    rw.ResponseWriter.WriteHeader(code)
}

// RecoveryMiddleware recovers from panics
func RecoveryMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        defer func() {
            if err := recover(); err != nil {
                log.Printf("PANIC: %v", err)
                writeError(w, http.StatusInternalServerError, models.ErrCodeInternalError,
                    "internal server error", nil)
            }
        }()

        next.ServeHTTP(w, r)
    })
}

// CORSMiddleware adds CORS headers
func CORSMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Access-Control-Allow-Origin", "*") // TODO: Configure allowed origins
        w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
        w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

        if r.Method == "OPTIONS" {
            w.WriteHeader(http.StatusOK)
            return
        }

        next.ServeHTTP(w, r)
    })
}
```

**Tests:**

```go
// pkg/api/middleware_test.go
package api

import (
    "net/http"
    "net/http/httptest"
    "testing"

    "github.com/stretchr/testify/assert"
)

func TestLoggingMiddleware(t *testing.T) {
    handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusOK)
    })

    middleware := LoggingMiddleware(handler)

    req := httptest.NewRequest("GET", "/test", nil)
    w := httptest.NewRecorder()

    middleware.ServeHTTP(w, req)

    assert.Equal(t, http.StatusOK, w.Code)
}

func TestRecoveryMiddleware(t *testing.T) {
    handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        panic("test panic")
    })

    middleware := RecoveryMiddleware(handler)

    req := httptest.NewRequest("GET", "/test", nil)
    w := httptest.NewRecorder()

    middleware.ServeHTTP(w, req)

    assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestCORSMiddleware(t *testing.T) {
    handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusOK)
    })

    middleware := CORSMiddleware(handler)

    req := httptest.NewRequest("OPTIONS", "/test", nil)
    w := httptest.NewRecorder()

    middleware.ServeHTTP(w, req)

    assert.Equal(t, http.StatusOK, w.Code)
    assert.Equal(t, "*", w.Header().Get("Access-Control-Allow-Origin"))
}
```

**Acceptance Criteria:**
- ✅ Standardized error response format
- ✅ Logging middleware logs all requests
- ✅ Recovery middleware catches panics
- ✅ CORS middleware adds proper headers
- ✅ Storage errors mapped to HTTP status codes
- ✅ Consistent error codes across all endpoints

---

### Work Item 7: Router Setup

**Purpose:** Configure HTTP router with all endpoints and middleware chain.

**Implementation:**

```go
// pkg/api/router.go
package api

import (
    "github.com/go-chi/chi/v5"
    "github.com/go-chi/chi/v5/middleware"
    "github.com/yourusername/luminate/pkg/storage"
)

// NewRouter creates the HTTP router with all endpoints
func NewRouter(store storage.MetricsStore) *chi.Mux {
    r := chi.NewRouter()

    // Global middleware
    r.Use(RecoveryMiddleware)
    r.Use(LoggingMiddleware)
    r.Use(CORSMiddleware)
    r.Use(middleware.RequestID)
    r.Use(middleware.RealIP)
    r.Use(middleware.Compress(5)) // Gzip compression

    // Initialize handlers
    queryHandler := NewQueryHandler(store)
    writeHandler := NewWriteHandler(store)
    discoveryHandler := NewDiscoveryHandler(store)
    healthHandler := NewHealthHandler(store)
    adminHandler := NewAdminHandler(store)

    // Public endpoints (no auth required)
    r.Get("/api/v1/health", healthHandler.HandleHealth)

    // API v1 routes (require auth - will be added in WS4)
    r.Route("/api/v1", func(r chi.Router) {
        // Core endpoints
        r.Post("/query", queryHandler.HandleQuery)
        r.Post("/write", writeHandler.HandleWrite)

        // Discovery endpoints
        r.Get("/metrics", discoveryHandler.HandleListMetrics)
        r.Get("/metrics/{name}/dimensions", discoveryHandler.HandleListDimensions)
        r.Get("/metrics/{name}/dimensions/{key}/values", discoveryHandler.HandleListDimensionValues)

        // Admin endpoints (require admin scope - will be added in WS4)
        r.Route("/admin", func(r chi.Router) {
            r.Post("/retention", adminHandler.HandleSetRetention)
            r.Delete("/metrics/{name}", adminHandler.HandleDeleteMetrics)
        })
    })

    return r
}
```

**Tests:**

```go
// pkg/api/router_test.go
package api

import (
    "net/http"
    "net/http/httptest"
    "testing"

    "github.com/stretchr/testify/assert"
)

func TestRouter_HealthEndpoint(t *testing.T) {
    mockStore := new(MockMetricsStore)
    mockStore.On("Health", mock.Anything).Return(nil)

    router := NewRouter(mockStore)

    req := httptest.NewRequest("GET", "/api/v1/health", nil)
    w := httptest.NewRecorder()

    router.ServeHTTP(w, req)

    assert.Equal(t, http.StatusOK, w.Code)
}

func TestRouter_NotFound(t *testing.T) {
    mockStore := new(MockMetricsStore)
    router := NewRouter(mockStore)

    req := httptest.NewRequest("GET", "/nonexistent", nil)
    w := httptest.NewRecorder()

    router.ServeHTTP(w, req)

    assert.Equal(t, http.StatusNotFound, w.Code)
}
```

**Acceptance Criteria:**
- ✅ All endpoints registered correctly
- ✅ Middleware chain applied in correct order
- ✅ Health endpoint is public (no auth)
- ✅ API endpoints require auth (placeholder for WS4)
- ✅ Admin endpoints require admin scope (placeholder for WS4)
- ✅ 404 handling for unknown routes

---

## Integration Testing

```go
// pkg/api/integration_test.go
package api

import (
    "bytes"
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "testing"
    "time"

    "github.com/stretchr/testify/assert"
    "github.com/yourusername/luminate/pkg/models"
    "github.com/yourusername/luminate/pkg/storage"
)

func TestAPI_WriteAndQuery_Integration(t *testing.T) {
    // Use in-memory mock store
    mockStore := setupMockStore()
    router := NewRouter(mockStore)

    // Step 1: Write metrics
    writeReq := BatchWriteRequest{
        Metrics: []models.Metric{
            {
                Name:       "api_latency",
                Timestamp:  time.Now(),
                Value:      145.5,
                Dimensions: map[string]string{"customer_id": "cust_123"},
            },
        },
    }

    body, _ := json.Marshal(writeReq)
    req := httptest.NewRequest("POST", "/api/v1/write", bytes.NewReader(body))
    req.Header.Set("Content-Type", "application/json")

    w := httptest.NewRecorder()
    router.ServeHTTP(w, req)

    assert.Equal(t, http.StatusOK, w.Code)

    // Step 2: Query metrics
    queryReq := JSONQuery{
        QueryType:   "aggregate",
        MetricName:  "api_latency",
        TimeRange:   TimeRangeSpec{Relative: "1h"},
        Aggregation: "avg",
        GroupBy:     []string{"customer_id"},
    }

    body, _ = json.Marshal(queryReq)
    req = httptest.NewRequest("POST", "/api/v1/query", bytes.NewReader(body))
    req.Header.Set("Content-Type", "application/json")

    w = httptest.NewRecorder()
    router.ServeHTTP(w, req)

    assert.Equal(t, http.StatusOK, w.Code)

    var queryResp QueryResponse
    json.NewDecoder(w.Body).Decode(&queryResp)

    assert.Equal(t, "aggregate", queryResp.QueryType)
    assert.NotNil(t, queryResp.Results)
}
```

---

## Performance Targets

- **Write Endpoint**: Handle 10,000 metrics/second per pod
- **Query Endpoint**: p95 latency < 100ms for aggregate queries
- **Throughput**: Support 100 concurrent requests
- **Memory**: < 1 GB per pod under normal load
- **Error Rate**: < 0.1% for valid requests

---

## Documentation

### API Documentation (OpenAPI/Swagger)

Create `docs/api/openapi.yaml`:

```yaml
openapi: 3.0.0
info:
  title: Luminate Observability API
  version: 1.0.0
  description: High-cardinality metrics API

servers:
  - url: http://localhost:8080/api/v1

paths:
  /query:
    post:
      summary: Execute unified query
      requestBody:
        required: true
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/JSONQuery'
      responses:
        '200':
          description: Query results
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/QueryResponse'

  /write:
    post:
      summary: Write metrics batch
      requestBody:
        required: true
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/BatchWriteRequest'
      responses:
        '200':
          description: All metrics accepted
        '207':
          description: Partial success

components:
  schemas:
    JSONQuery:
      type: object
      required:
        - queryType
        - metricName
        - timeRange
      properties:
        queryType:
          type: string
          enum: [range, aggregate, rate]
        metricName:
          type: string
        timeRange:
          $ref: '#/components/schemas/TimeRangeSpec'
```

---

## Summary

This workstream provides complete HTTP API implementation for Luminate with:

1. **Unified Query Endpoint**: Single endpoint for range, aggregate, and rate queries
2. **Batch Write Endpoint**: High-throughput metric ingestion with compression
3. **Discovery Endpoints**: Metric/dimension exploration for UIs
4. **Health Check Endpoint**: Kubernetes probes and monitoring
5. **Admin Endpoints**: Retention and cleanup operations
6. **Error Handling**: Standardized error responses
7. **Middleware**: Logging, recovery, CORS

**Next Steps**: After completing this workstream, proceed to [WS4: Authentication & Security](04-authentication.md) to add JWT authentication and multi-tenancy support.
