package storage

import (
	"context"
	"time"

	"github.com/yourusername/luminate/pkg/models"
)

// MetricsStore defines the interface for storing and querying metrics
type MetricsStore interface {
	// Write operations
	Write(ctx context.Context, metrics []models.Metric) error

	// Query operations
	QueryRange(ctx context.Context, req QueryRequest) ([]models.MetricPoint, error)
	Aggregate(ctx context.Context, req AggregateRequest) ([]models.AggregateResult, error)
	Rate(ctx context.Context, req RateRequest) ([]models.RatePoint, error)

	// Discovery operations
	ListMetrics(ctx context.Context) ([]string, error)
	ListDimensionKeys(ctx context.Context, metricName string) ([]string, error)
	ListDimensionValues(ctx context.Context, metricName, dimensionKey string, limit int) ([]string, error)

	// Lifecycle operations
	DeleteBefore(ctx context.Context, metricName string, before time.Time) error
	Health(ctx context.Context) error
	Close() error
}

// QueryRequest represents a simple range query
type QueryRequest struct {
	MetricName string
	Start      time.Time
	End        time.Time
	Filters    map[string]string
	Limit      int
	Timeout    time.Duration
}

// AggregateRequest represents an aggregation query
type AggregateRequest struct {
	MetricName  string
	Start       time.Time
	End         time.Time
	Filters     map[string]string
	Aggregation AggregationType
	GroupBy     []string
	Limit       int
	Timeout     time.Duration
}

// RateRequest represents a rate calculation query
type RateRequest struct {
	MetricName string
	Start      time.Time
	End        time.Time
	Filters    map[string]string
	Interval   time.Duration
	Timeout    time.Duration
}

// AggregationType defines the type of aggregation
type AggregationType int

const (
	AVG AggregationType = iota
	SUM
	COUNT
	MIN
	MAX
	P50
	P95
	P99
	INTEGRAL
)

// String returns the string representation of AggregationType
func (a AggregationType) String() string {
	switch a {
	case AVG:
		return "avg"
	case SUM:
		return "sum"
	case COUNT:
		return "count"
	case MIN:
		return "min"
	case MAX:
		return "max"
	case P50:
		return "p50"
	case P95:
		return "p95"
	case P99:
		return "p99"
	case INTEGRAL:
		return "integral"
	default:
		return "unknown"
	}
}

// ParseAggregationType parses a string into AggregationType
func ParseAggregationType(s string) (AggregationType, error) {
	switch s {
	case "avg":
		return AVG, nil
	case "sum":
		return SUM, nil
	case "count":
		return COUNT, nil
	case "min":
		return MIN, nil
	case "max":
		return MAX, nil
	case "p50":
		return P50, nil
	case "p95":
		return P95, nil
	case "p99":
		return P99, nil
	case "integral":
		return INTEGRAL, nil
	default:
		return 0, nil
	}
}
