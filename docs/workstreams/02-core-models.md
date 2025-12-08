# WS2: Core Data Models

**Priority:** P0 (Critical Path)
**Estimated Effort:** 2-3 days
**Dependencies:** None
**Owner:** Backend Team

## Overview

Enhance and complete the core data models with comprehensive validation, serialization, and error handling. This workstream establishes the foundation for all data flowing through the system.

## Objectives

- Complete metric validation with all edge cases
- Define all request/response models
- Implement error types and codes
- Add JSON serialization/deserialization
- Achieve 100% test coverage for models

## Work Items

### 1. Enhanced Metric Validation

**Effort:** 0.5 days
**File:** `pkg/models/metric.go`

#### Current State

Basic validation exists in [pkg/models/metric.go](../pkg/models/metric.go:23-59).

#### Enhancements Needed

```go
// pkg/models/metric.go
package models

import (
	"errors"
	"fmt"
	"math"
	"regexp"
	"time"
	"unicode/utf8"
)

var (
	// Validation regexes
	metricNameRegex = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)
	dimensionKeyRegex = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

	// Reserved dimension keys
	reservedKeys = map[string]bool{
		"_tenant_id": true,
		"_internal":  true,
	}

	// Validation errors
	ErrEmptyMetricName       = errors.New("metric name cannot be empty")
	ErrInvalidMetricName     = errors.New("metric name must match pattern ^[a-zA-Z_][a-zA-Z0-9_]*$")
	ErrMetricNameTooLong     = errors.New("metric name exceeds 256 characters")
	ErrInvalidValue          = errors.New("metric value must be finite (not NaN or Inf)")
	ErrTimestampTooOld       = errors.New("timestamp too old (>7 days)")
	ErrTimestampInFuture     = errors.New("timestamp in future (>1 hour)")
	ErrTooManyDimensions     = errors.New("max 20 dimensions allowed per metric")
	ErrInvalidDimensionKey   = errors.New("dimension key must match pattern ^[a-zA-Z_][a-zA-Z0-9_]*$")
	ErrReservedDimensionKey  = errors.New("dimension key is reserved")
	ErrDimensionKeyTooLong   = errors.New("dimension key exceeds 128 characters")
	ErrDimensionValueTooLong = errors.New("dimension value exceeds 256 characters")
	ErrDimensionValueInvalid = errors.New("dimension value contains control characters")
)

// Metric represents a single metric data point with dimensions
type Metric struct {
	Name       string            `json:"name"`
	Timestamp  time.Time         `json:"timestamp"`
	Value      float64           `json:"value"`
	Dimensions map[string]string `json:"dimensions"`
}

// Validate performs comprehensive validation
func (m *Metric) Validate() error {
	// Validate name
	if err := m.validateName(); err != nil {
		return err
	}

	// Validate timestamp
	if err := m.validateTimestamp(); err != nil {
		return err
	}

	// Validate value
	if err := m.validateValue(); err != nil {
		return err
	}

	// Validate dimensions
	if err := m.validateDimensions(); err != nil {
		return err
	}

	return nil
}

func (m *Metric) validateName() error {
	if m.Name == "" {
		return ErrEmptyMetricName
	}

	if len(m.Name) > 256 {
		return ErrMetricNameTooLong
	}

	if !metricNameRegex.MatchString(m.Name) {
		return ErrInvalidMetricName
	}

	return nil
}

func (m *Metric) validateTimestamp() error {
	now := time.Now()

	// Check if too old (>7 days)
	if m.Timestamp.Before(now.Add(-7 * 24 * time.Hour)) {
		return ErrTimestampTooOld
	}

	// Check if in future (>1 hour)
	if m.Timestamp.After(now.Add(1 * time.Hour)) {
		return ErrTimestampInFuture
	}

	return nil
}

func (m *Metric) validateValue() error {
	if math.IsNaN(m.Value) || math.IsInf(m.Value, 0) {
		return ErrInvalidValue
	}

	return nil
}

func (m *Metric) validateDimensions() error {
	if len(m.Dimensions) > 20 {
		return ErrTooManyDimensions
	}

	for key, value := range m.Dimensions {
		// Validate key
		if err := validateDimensionKey(key); err != nil {
			return fmt.Errorf("dimension key '%s': %w", key, err)
		}

		// Validate value
		if err := validateDimensionValue(value); err != nil {
			return fmt.Errorf("dimension '%s' value: %w", key, err)
		}
	}

	return nil
}

func validateDimensionKey(key string) error {
	if key == "" {
		return errors.New("dimension key cannot be empty")
	}

	if len(key) > 128 {
		return ErrDimensionKeyTooLong
	}

	if !dimensionKeyRegex.MatchString(key) {
		return ErrInvalidDimensionKey
	}

	if reservedKeys[key] {
		return ErrReservedDimensionKey
	}

	return nil
}

func validateDimensionValue(value string) error {
	if value == "" {
		return errors.New("dimension value cannot be empty")
	}

	if len(value) > 256 {
		return ErrDimensionValueTooLong
	}

	// Check for control characters
	if !utf8.ValidString(value) {
		return ErrDimensionValueInvalid
	}

	for _, r := range value {
		if r < 32 && r != '\t' && r != '\n' && r != '\r' {
			return ErrDimensionValueInvalid
		}
	}

	return nil
}

// ValidateMany validates multiple metrics efficiently
func ValidateMany(metrics []Metric) error {
	for i, metric := range metrics {
		if err := metric.Validate(); err != nil {
			return fmt.Errorf("metric %d (%s): %w", i, metric.Name, err)
		}
	}
	return nil
}
```

#### Tests

```go
// pkg/models/metric_test.go
package models

import (
	"math"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestMetricValidation_ValidMetric(t *testing.T) {
	metric := Metric{
		Name:      "api_latency",
		Timestamp: time.Now(),
		Value:     145.5,
		Dimensions: map[string]string{
			"customer_id": "cust_123",
			"endpoint":    "/api/users",
		},
	}

	err := metric.Validate()
	assert.NoError(t, err)
}

func TestMetricValidation_EmptyName(t *testing.T) {
	metric := Metric{
		Name:      "",
		Timestamp: time.Now(),
		Value:     100,
	}

	err := metric.Validate()
	assert.ErrorIs(t, err, ErrEmptyMetricName)
}

func TestMetricValidation_InvalidNamePattern(t *testing.T) {
	tests := []string{
		"123invalid",      // Starts with number
		"invalid-name",    // Contains hyphen
		"invalid.name",    // Contains dot
		"invalid name",    // Contains space
	}

	for _, name := range tests {
		metric := Metric{
			Name:      name,
			Timestamp: time.Now(),
			Value:     100,
		}

		err := metric.Validate()
		assert.ErrorIs(t, err, ErrInvalidMetricName, "name: %s", name)
	}
}

func TestMetricValidation_NameTooLong(t *testing.T) {
	metric := Metric{
		Name:      strings.Repeat("a", 257),
		Timestamp: time.Now(),
		Value:     100,
	}

	err := metric.Validate()
	assert.ErrorIs(t, err, ErrMetricNameTooLong)
}

func TestMetricValidation_InvalidValues(t *testing.T) {
	tests := []struct {
		name  string
		value float64
	}{
		{"NaN", math.NaN()},
		{"PositiveInf", math.Inf(1)},
		{"NegativeInf", math.Inf(-1)},
	}

	for _, tt := range tests {
		metric := Metric{
			Name:      "test_metric",
			Timestamp: time.Now(),
			Value:     tt.value,
		}

		err := metric.Validate()
		assert.ErrorIs(t, err, ErrInvalidValue, "test: %s", tt.name)
	}
}

func TestMetricValidation_TimestampTooOld(t *testing.T) {
	metric := Metric{
		Name:      "test_metric",
		Timestamp: time.Now().Add(-8 * 24 * time.Hour), // 8 days old
		Value:     100,
	}

	err := metric.Validate()
	assert.ErrorIs(t, err, ErrTimestampTooOld)
}

func TestMetricValidation_TimestampInFuture(t *testing.T) {
	metric := Metric{
		Name:      "test_metric",
		Timestamp: time.Now().Add(2 * time.Hour), // 2 hours in future
		Value:     100,
	}

	err := metric.Validate()
	assert.ErrorIs(t, err, ErrTimestampInFuture)
}

func TestMetricValidation_TooManyDimensions(t *testing.T) {
	dimensions := make(map[string]string)
	for i := 0; i < 21; i++ {
		dimensions[fmt.Sprintf("dim%d", i)] = "value"
	}

	metric := Metric{
		Name:       "test_metric",
		Timestamp:  time.Now(),
		Value:      100,
		Dimensions: dimensions,
	}

	err := metric.Validate()
	assert.ErrorIs(t, err, ErrTooManyDimensions)
}

func TestMetricValidation_InvalidDimensionKey(t *testing.T) {
	metric := Metric{
		Name:      "test_metric",
		Timestamp: time.Now(),
		Value:     100,
		Dimensions: map[string]string{
			"invalid-key": "value",
		},
	}

	err := metric.Validate()
	assert.ErrorIs(t, err, ErrInvalidDimensionKey)
}

func TestMetricValidation_ReservedDimensionKey(t *testing.T) {
	metric := Metric{
		Name:      "test_metric",
		Timestamp: time.Now(),
		Value:     100,
		Dimensions: map[string]string{
			"_tenant_id": "tenant_123",
		},
	}

	err := metric.Validate()
	assert.ErrorIs(t, err, ErrReservedDimensionKey)
}

func TestMetricValidation_DimensionValueTooLong(t *testing.T) {
	metric := Metric{
		Name:      "test_metric",
		Timestamp: time.Now(),
		Value:     100,
		Dimensions: map[string]string{
			"key": strings.Repeat("a", 257),
		},
	}

	err := metric.Validate()
	assert.ErrorIs(t, err, ErrDimensionValueTooLong)
}

func TestMetricValidation_DimensionValueControlCharacters(t *testing.T) {
	metric := Metric{
		Name:      "test_metric",
		Timestamp: time.Now(),
		Value:     100,
		Dimensions: map[string]string{
			"key": "value\x00with\x01control",
		},
	}

	err := metric.Validate()
	assert.ErrorIs(t, err, ErrDimensionValueInvalid)
}

func BenchmarkMetricValidation(b *testing.B) {
	metric := Metric{
		Name:      "api_latency",
		Timestamp: time.Now(),
		Value:     145.5,
		Dimensions: map[string]string{
			"customer_id": "cust_123",
			"endpoint":    "/api/users",
			"method":      "GET",
			"status":      "200",
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = metric.Validate()
	}
}
```

#### Acceptance Criteria

- [ ] All validation rules implemented
- [ ] Specific error types for each validation failure
- [ ] 100% test coverage
- [ ] Validation benchmark < 1µs per metric
- [ ] Documentation for each validation rule

---

### 2. Query Request/Response Models

**Effort:** 1 day
**File:** `pkg/models/query.go`

#### Technical Specification

```go
// pkg/models/query.go
package models

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// TimeRangeSpec represents a time range (absolute or relative)
type TimeRangeSpec struct {
	Start    *time.Time `json:"start,omitempty"`
	End      *time.Time `json:"end,omitempty"`
	Relative string     `json:"relative,omitempty"` // e.g., "1h", "24h", "7d"
}

// GetAbsoluteRange converts to absolute time range
func (t *TimeRangeSpec) GetAbsoluteRange() (time.Time, time.Time, error) {
	if t.Relative != "" {
		duration, err := parseRelativeDuration(t.Relative)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("invalid relative time: %w", err)
		}

		end := time.Now()
		start := end.Add(-duration)
		return start, end, nil
	}

	if t.Start != nil && t.End != nil {
		if t.End.Before(*t.Start) {
			return time.Time{}, time.Time{}, errors.New("end time must be after start time")
		}
		return *t.Start, *t.End, nil
	}

	return time.Time{}, time.Time{}, errors.New("must specify either absolute range (start/end) or relative time")
}

func parseRelativeDuration(relative string) (time.Duration, error) {
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
		// Try parsing as duration
		return time.ParseDuration(relative)
	}
}

// JSONQuery represents a unified query request
type JSONQuery struct {
	QueryType   string            `json:"queryType"`   // "range", "aggregate", "rate"
	MetricName  string            `json:"metricName"`
	TimeRange   TimeRangeSpec     `json:"timeRange"`
	Filters     map[string]string `json:"filters,omitempty"`
	Aggregation string            `json:"aggregation,omitempty"` // For aggregate queries
	GroupBy     []string          `json:"groupBy,omitempty"`
	Interval    string            `json:"interval,omitempty"` // For rate queries
	Limit       int               `json:"limit,omitempty"`
	Timeout     string            `json:"timeout,omitempty"`
}

// Validate checks if the query is valid
func (q *JSONQuery) Validate() error {
	if q.MetricName == "" {
		return errors.New("metric name required")
	}

	if q.QueryType == "" {
		return errors.New("query type required")
	}

	switch q.QueryType {
	case "range", "aggregate", "rate":
		// Valid
	default:
		return fmt.Errorf("invalid query type: %s", q.QueryType)
	}

	// Validate time range
	if _, _, err := q.TimeRange.GetAbsoluteRange(); err != nil {
		return fmt.Errorf("invalid time range: %w", err)
	}

	// Type-specific validation
	switch q.QueryType {
	case "aggregate":
		if q.Aggregation == "" {
			return errors.New("aggregation required for aggregate query")
		}
	case "rate":
		if q.Interval == "" {
			return errors.New("interval required for rate query")
		}
	}

	return nil
}

// QueryResponse represents the unified query response
type QueryResponse struct {
	QueryType       string           `json:"queryType"`
	Results         interface{}      `json:"results"`
	ExecutionTimeMs int64            `json:"executionTimeMs"`
	PointsScanned   int64            `json:"pointsScanned,omitempty"`
}

// BatchWriteRequest represents a batch of metrics to write
type BatchWriteRequest struct {
	Metrics []Metric `json:"metrics"`
}

// Validate validates the batch write request
func (b *BatchWriteRequest) Validate() error {
	if len(b.Metrics) == 0 {
		return errors.New("no metrics provided")
	}

	if len(b.Metrics) > 10000 {
		return errors.New("batch exceeds maximum size of 10,000 metrics")
	}

	return ValidateMany(b.Metrics)
}

// BatchWriteResponse represents the response to a batch write
type BatchWriteResponse struct {
	Accepted int                `json:"accepted"`
	Rejected int                `json:"rejected"`
	Errors   []MetricWriteError `json:"errors,omitempty"`
}

// MetricWriteError represents an error writing a specific metric
type MetricWriteError struct {
	Index      int    `json:"index"`
	MetricName string `json:"metricName"`
	Error      string `json:"error"`
}
```

#### Tests

```go
// pkg/models/query_test.go
package models

func TestTimeRangeSpec_Relative(t *testing.T) {
	spec := TimeRangeSpec{Relative: "1h"}

	start, end, err := spec.GetAbsoluteRange()
	assert.NoError(t, err)
	assert.WithinDuration(t, time.Now().Add(-1*time.Hour), start, 1*time.Second)
	assert.WithinDuration(t, time.Now(), end, 1*time.Second)
}

func TestTimeRangeSpec_Absolute(t *testing.T) {
	now := time.Now()
	startTime := now.Add(-1 * time.Hour)

	spec := TimeRangeSpec{
		Start: &startTime,
		End:   &now,
	}

	start, end, err := spec.GetAbsoluteRange()
	assert.NoError(t, err)
	assert.Equal(t, startTime, start)
	assert.Equal(t, now, end)
}

func TestTimeRangeSpec_InvalidEndBeforeStart(t *testing.T) {
	now := time.Now()
	future := now.Add(1 * time.Hour)

	spec := TimeRangeSpec{
		Start: &future,
		End:   &now,
	}

	_, _, err := spec.GetAbsoluteRange()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "end time must be after start time")
}

func TestJSONQuery_Validate(t *testing.T) {
	query := JSONQuery{
		QueryType:  "aggregate",
		MetricName: "api_latency",
		TimeRange:  TimeRangeSpec{Relative: "1h"},
		Aggregation: "avg",
		GroupBy:    []string{"customer_id"},
	}

	err := query.Validate()
	assert.NoError(t, err)
}

func TestJSONQuery_ValidateErrors(t *testing.T) {
	tests := []struct {
		name    string
		query   JSONQuery
		wantErr string
	}{
		{
			name:    "missing metric name",
			query:   JSONQuery{QueryType: "range"},
			wantErr: "metric name required",
		},
		{
			name:    "missing query type",
			query:   JSONQuery{MetricName: "test"},
			wantErr: "query type required",
		},
		{
			name: "invalid query type",
			query: JSONQuery{
				QueryType:  "invalid",
				MetricName: "test",
				TimeRange:  TimeRangeSpec{Relative: "1h"},
			},
			wantErr: "invalid query type",
		},
		{
			name: "aggregate without aggregation",
			query: JSONQuery{
				QueryType:  "aggregate",
				MetricName: "test",
				TimeRange:  TimeRangeSpec{Relative: "1h"},
			},
			wantErr: "aggregation required",
		},
		{
			name: "rate without interval",
			query: JSONQuery{
				QueryType:  "rate",
				MetricName: "test",
				TimeRange:  TimeRangeSpec{Relative: "1h"},
			},
			wantErr: "interval required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.query.Validate()
			assert.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestBatchWriteRequest_Validate(t *testing.T) {
	metrics := []Metric{
		{
			Name:      "test_metric",
			Timestamp: time.Now(),
			Value:     100,
		},
	}

	req := BatchWriteRequest{Metrics: metrics}
	err := req.Validate()
	assert.NoError(t, err)
}

func TestBatchWriteRequest_ValidateEmpty(t *testing.T) {
	req := BatchWriteRequest{Metrics: []Metric{}}
	err := req.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no metrics provided")
}

func TestBatchWriteRequest_ValidateTooLarge(t *testing.T) {
	metrics := make([]Metric, 10001)
	req := BatchWriteRequest{Metrics: metrics}

	err := req.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "exceeds maximum size")
}
```

#### Acceptance Criteria

- [ ] All request/response models defined
- [ ] JSON serialization works correctly
- [ ] Validation comprehensive
- [ ] Tests cover all scenarios
- [ ] Documentation for each model

---

### 3. Error Types and Codes

**Effort:** 0.5 days
**File:** `pkg/models/errors.go`

#### Technical Specification

```go
// pkg/models/errors.go
package models

import (
	"fmt"
	"net/http"
)

// ErrorCode represents a structured error code
type ErrorCode string

const (
	// Validation errors (4xx)
	ErrCodeInvalidRequest        ErrorCode = "INVALID_REQUEST"
	ErrCodeInvalidMetricName     ErrorCode = "INVALID_METRIC_NAME"
	ErrCodeInvalidTimestamp      ErrorCode = "INVALID_TIMESTAMP"
	ErrCodeInvalidValue          ErrorCode = "INVALID_VALUE"
	ErrCodeDimensionLimitExceeded ErrorCode = "DIMENSION_LIMIT_EXCEEDED"
	ErrCodeBatchSizeExceeded     ErrorCode = "BATCH_SIZE_EXCEEDED"
	ErrCodeInvalidQuery          ErrorCode = "INVALID_QUERY"

	// Authorization errors (401, 403)
	ErrCodeUnauthorized ErrorCode = "UNAUTHORIZED"
	ErrCodeForbidden    ErrorCode = "FORBIDDEN"

	// Rate limiting errors (429)
	ErrCodeRateLimitExceeded     ErrorCode = "RATE_LIMIT_EXCEEDED"
	ErrCodeCardinalityExceeded   ErrorCode = "CARDINALITY_EXCEEDED"

	// Server errors (5xx)
	ErrCodeInternalError   ErrorCode = "INTERNAL_ERROR"
	ErrCodeStorageError    ErrorCode = "STORAGE_ERROR"
	ErrCodeQueryTimeout    ErrorCode = "QUERY_TIMEOUT"
	ErrCodeServiceUnavailable ErrorCode = "SERVICE_UNAVAILABLE"
)

// APIError represents a structured API error
type APIError struct {
	Code       ErrorCode              `json:"code"`
	Message    string                 `json:"message"`
	Details    map[string]interface{} `json:"details,omitempty"`
	StatusCode int                    `json:"-"` // HTTP status code
}

// Error implements the error interface
func (e *APIError) Error() string {
	return fmt.Sprintf("[%s] %s", e.Code, e.Message)
}

// ErrorResponse is the JSON error response format
type ErrorResponse struct {
	Error APIError `json:"error"`
}

// NewAPIError creates a new API error
func NewAPIError(code ErrorCode, message string, details map[string]interface{}) *APIError {
	statusCode := getHTTPStatusCode(code)

	return &APIError{
		Code:       code,
		Message:    message,
		Details:    details,
		StatusCode: statusCode,
	}
}

// getHTTPStatusCode maps error codes to HTTP status codes
func getHTTPStatusCode(code ErrorCode) int {
	switch code {
	case ErrCodeInvalidRequest, ErrCodeInvalidMetricName, ErrCodeInvalidTimestamp,
		ErrCodeInvalidValue, ErrCodeDimensionLimitExceeded, ErrCodeBatchSizeExceeded,
		ErrCodeInvalidQuery:
		return http.StatusBadRequest

	case ErrCodeUnauthorized:
		return http.StatusUnauthorized

	case ErrCodeForbidden:
		return http.StatusForbidden

	case ErrCodeRateLimitExceeded, ErrCodeCardinalityExceeded:
		return http.StatusTooManyRequests

	case ErrCodeQueryTimeout:
		return http.StatusGatewayTimeout

	case ErrCodeServiceUnavailable:
		return http.StatusServiceUnavailable

	case ErrCodeInternalError, ErrCodeStorageError:
		return http.StatusInternalServerError

	default:
		return http.StatusInternalServerError
	}
}

// Common error constructors
func InvalidMetricError(metricName string, reason error) *APIError {
	return NewAPIError(
		ErrCodeInvalidMetricName,
		"Invalid metric",
		map[string]interface{}{
			"metric_name": metricName,
			"reason":      reason.Error(),
		},
	)
}

func RateLimitError(limit int, window string) *APIError {
	return NewAPIError(
		ErrCodeRateLimitExceeded,
		"Rate limit exceeded",
		map[string]interface{}{
			"limit":  limit,
			"window": window,
		},
	)
}

func QueryTimeoutError(timeout string) *APIError {
	return NewAPIError(
		ErrCodeQueryTimeout,
		"Query execution exceeded timeout",
		map[string]interface{}{
			"timeout": timeout,
		},
	)
}

func StorageError(err error) *APIError {
	return NewAPIError(
		ErrCodeStorageError,
		"Storage backend error",
		map[string]interface{}{
			"error": err.Error(),
		},
	)
}
```

#### Tests

```go
// pkg/models/errors_test.go
package models

func TestAPIError_Error(t *testing.T) {
	err := NewAPIError(ErrCodeInvalidMetricName, "test error", nil)
	assert.Equal(t, "[INVALID_METRIC_NAME] test error", err.Error())
}

func TestGetHTTPStatusCode(t *testing.T) {
	tests := []struct {
		code       ErrorCode
		wantStatus int
	}{
		{ErrCodeInvalidRequest, http.StatusBadRequest},
		{ErrCodeUnauthorized, http.StatusUnauthorized},
		{ErrCodeForbidden, http.StatusForbidden},
		{ErrCodeRateLimitExceeded, http.StatusTooManyRequests},
		{ErrCodeQueryTimeout, http.StatusGatewayTimeout},
		{ErrCodeInternalError, http.StatusInternalServerError},
	}

	for _, tt := range tests {
		status := getHTTPStatusCode(tt.code)
		assert.Equal(t, tt.wantStatus, status, "code: %s", tt.code)
	}
}

func TestInvalidMetricError(t *testing.T) {
	err := InvalidMetricError("test_metric", errors.New("invalid name"))

	assert.Equal(t, ErrCodeInvalidMetricName, err.Code)
	assert.Equal(t, http.StatusBadRequest, err.StatusCode)
	assert.Contains(t, err.Details, "metric_name")
	assert.Contains(t, err.Details, "reason")
}
```

#### Acceptance Criteria

- [ ] All error codes defined
- [ ] Consistent error structure
- [ ] HTTP status codes correct
- [ ] Helper constructors for common errors
- [ ] Tests validate error structure

---

## Integration Testing

### Integration Test Suite

**File:** `pkg/models/integration_test.go`

```go
package models_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/yourusername/luminate/pkg/models"
)

func TestMetric_JSONRoundtrip(t *testing.T) {
	metric := models.Metric{
		Name:      "api_latency",
		Timestamp: time.Now().Truncate(time.Millisecond),
		Value:     145.5,
		Dimensions: map[string]string{
			"customer_id": "cust_123",
			"endpoint":    "/api/users",
		},
	}

	// Marshal to JSON
	data, err := json.Marshal(metric)
	assert.NoError(t, err)

	// Unmarshal from JSON
	var decoded models.Metric
	err = json.Unmarshal(data, &decoded)
	assert.NoError(t, err)

	// Verify
	assert.Equal(t, metric.Name, decoded.Name)
	assert.Equal(t, metric.Value, decoded.Value)
	assert.Equal(t, metric.Dimensions, decoded.Dimensions)
}

func TestJSONQuery_JSONRoundtrip(t *testing.T) {
	query := models.JSONQuery{
		QueryType:   "aggregate",
		MetricName:  "api_latency",
		TimeRange:   models.TimeRangeSpec{Relative: "1h"},
		Aggregation: "p95",
		GroupBy:     []string{"customer_id"},
		Limit:       100,
	}

	// Marshal to JSON
	data, err := json.Marshal(query)
	assert.NoError(t, err)

	// Unmarshal from JSON
	var decoded models.JSONQuery
	err = json.Unmarshal(data, &decoded)
	assert.NoError(t, err)

	// Verify
	assert.Equal(t, query.QueryType, decoded.QueryType)
	assert.Equal(t, query.MetricName, decoded.MetricName)
	assert.Equal(t, query.Aggregation, decoded.Aggregation)
}
```

## Documentation

### API Documentation

Create comprehensive documentation for all models:

**File:** `pkg/models/README.md`

```markdown
# Data Models

## Metric

Represents a single metric data point.

### Validation Rules

- **Name**: Must match `^[a-zA-Z_][a-zA-Z0-9_]*$`, 1-256 characters
- **Timestamp**: Must be within `[now - 7 days, now + 1 hour]`
- **Value**: Must be finite (no NaN or Inf)
- **Dimensions**: Max 20, keys match name pattern, values UTF-8 1-256 chars

### Example

\`\`\`json
{
  "name": "api_latency",
  "timestamp": "2024-01-01T12:00:00Z",
  "value": 145.5,
  "dimensions": {
    "customer_id": "cust_123",
    "endpoint": "/api/users"
  }
}
\`\`\`

## Query Models

See full documentation in ARCHITECTURE.md.
```

## Acceptance Criteria Summary

### Functional
- [ ] All models implemented
- [ ] Comprehensive validation
- [ ] Error types defined
- [ ] JSON serialization works

### Quality
- [ ] 100% test coverage
- [ ] All edge cases tested
- [ ] Benchmarks pass
- [ ] Documentation complete

## Dependencies

**Go Packages:**
```go
require (
	github.com/stretchr/testify v1.8.4
)
```

## Rollout Plan

1. **Day 1:** Enhanced metric validation
2. **Day 2:** Query request/response models
3. **Day 3:** Error types and integration tests

---

**Status:** Ready for Implementation
**Last Updated:** 2024-12-08
