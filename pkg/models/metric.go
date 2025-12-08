package models

import (
	"errors"
	"math"
	"time"
)

// Metric represents a single metric data point with dimensions
type Metric struct {
	Name       string            `json:"name"`
	Timestamp  time.Time         `json:"timestamp"`
	Value      float64           `json:"value"`
	Dimensions map[string]string `json:"dimensions"`
}

// Validate checks if the metric is valid according to Luminate rules
func (m *Metric) Validate() error {
	if m.Name == "" {
		return errors.New("metric name required")
	}

	if !isValidMetricName(m.Name) {
		return errors.New("metric name must match pattern ^[a-zA-Z_][a-zA-Z0-9_]*$")
	}

	if math.IsNaN(m.Value) || math.IsInf(m.Value, 0) {
		return errors.New("metric value cannot be NaN or Inf")
	}

	// Reject timestamps too far in past (>7 days) or future (>1 hour)
	now := time.Now()
	if m.Timestamp.Before(now.Add(-7 * 24 * time.Hour)) {
		return errors.New("timestamp too old (>7 days)")
	}
	if m.Timestamp.After(now.Add(1 * time.Hour)) {
		return errors.New("timestamp in future (>1 hour)")
	}

	// Validate dimensions
	if len(m.Dimensions) > 20 {
		return errors.New("max 20 dimensions allowed per metric")
	}

	for key, value := range m.Dimensions {
		if !isValidDimensionKey(key) {
			return errors.New("dimension key must match pattern ^[a-zA-Z_][a-zA-Z0-9_]*$")
		}
		if len(key) > 128 {
			return errors.New("dimension key exceeds 128 characters")
		}
		if len(value) > 256 {
			return errors.New("dimension value exceeds 256 characters")
		}
	}

	return nil
}

// isValidMetricName checks if metric name matches pattern ^[a-zA-Z_][a-zA-Z0-9_]*$
func isValidMetricName(name string) bool {
	if len(name) == 0 || len(name) > 256 {
		return false
	}

	first := name[0]
	if !((first >= 'a' && first <= 'z') || (first >= 'A' && first <= 'Z') || first == '_') {
		return false
	}

	for i := 1; i < len(name); i++ {
		c := name[i]
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_') {
			return false
		}
	}

	return true
}

// isValidDimensionKey checks if dimension key matches pattern ^[a-zA-Z_][a-zA-Z0-9_]*$
func isValidDimensionKey(key string) bool {
	return isValidMetricName(key) // Same validation rules
}

// MetricPoint represents a single data point returned from queries
type MetricPoint struct {
	Timestamp  time.Time         `json:"timestamp"`
	Value      float64           `json:"value"`
	Dimensions map[string]string `json:"dimensions"`
}

// AggregateResult represents the result of an aggregation query
type AggregateResult struct {
	GroupKey map[string]string `json:"groupKey"`
	Value    float64           `json:"value"`
	Count    int64             `json:"count"`
}

// RatePoint represents a rate calculation data point
type RatePoint struct {
	Timestamp time.Time `json:"timestamp"`
	Rate      float64   `json:"rate"`
}
