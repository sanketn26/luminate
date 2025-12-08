# WS6: Storage Backend - ClickHouse Implementation

**Priority:** P1 (High Priority)
**Estimated Effort:** 6-8 days
**Dependencies:** WS1 (BadgerDB for reference), WS2 (Core Models)
**Owner:** Backend Team

## Overview

Implement the production-ready ClickHouse storage backend that enables horizontal scaling. ClickHouse's columnar architecture and native support for high-cardinality data makes it ideal for production deployments handling billions of metrics.

## Objectives

- Implement all `MetricsStore` interface methods using ClickHouse
- Achieve 100,000+ metrics/second write throughput per pod
- Support query p95 latency < 100ms
- Handle billions of unique series across millions of tenants
- Enable horizontal scaling with stateless architecture

## Background: Why ClickHouse?

### Columnar Storage Advantage

```
Traditional Row Storage (e.g., BadgerDB):
Row 1: [timestamp=t1, metric=cpu, customer=c1, value=70.5]
Row 2: [timestamp=t2, metric=cpu, customer=c1, value=75.2]
→ Reading one column requires reading ALL columns
→ Poor compression (mixed data types)

ClickHouse Columnar Storage:
Column timestamp:    [t1, t2, t3, t4, ...]  ← Compressed together
Column metric_name:  [cpu, cpu, mem, cpu, ...] ← LowCardinality compression
Column customer_id:  [c1, c1, c2, c1, ...]  ← Bloom filter index
Column value:        [70.5, 75.2, 80.1, ...] ← Delta encoding
→ Read only needed columns
→ 10-100x compression
→ Cardinality doesn't kill performance
```

### Real-World Performance
- **Cloudflare**: 100+ billion rows/day, sub-second queries
- **Uber**: 10x compression, 200x faster than Elasticsearch
- **Yandex**: Petabyte-scale analytics

## Work Items

### 1. Connection Management and Configuration

**Effort:** 1 day
**File:** `pkg/storage/clickhouse/store.go`

#### Technical Specification

```go
// pkg/storage/clickhouse/store.go
package clickhouse

import (
	"context"
	"fmt"
	"time"

	clickhouse "github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/yourusername/luminate/pkg/config"
	"github.com/yourusername/luminate/pkg/storage"
)

type Store struct {
	conn   driver.Conn
	config config.ClickHouseConfig
}

// NewStore creates a new ClickHouse storage backend
func NewStore(cfg config.ClickHouseConfig) (*Store, error) {
	// Build connection options
	opts := &clickhouse.Options{
		Addr: cfg.Addresses,
		Auth: clickhouse.Auth{
			Database: cfg.Database,
			Username: cfg.Username,
			Password: cfg.Password,
		},
		Settings: clickhouse.Settings{
			"max_execution_time":             30,  // 30 second timeout
			"max_memory_usage":               10000000000, // 10GB
			"readonly":                       0,
			"max_block_size":                 10000,
			"max_insert_block_size":          100000,
			"insert_quorum":                  0,  // No quorum for performance
			"insert_quorum_timeout":          0,
			"select_sequential_consistency":  0,  // Eventual consistency OK
		},
		DialTimeout:      5 * time.Second,
		MaxOpenConns:     cfg.MaxOpenConns,
		MaxIdleConns:     cfg.MaxIdleConns,
		ConnMaxLifetime:  cfg.ConnMaxLifetime,
		ConnOpenStrategy: clickhouse.ConnOpenInOrder,
		BlockBufferSize:  10,
	}

	// Connect
	conn, err := clickhouse.Open(opts)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to ClickHouse: %w", err)
	}

	// Ping to verify connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := conn.Ping(ctx); err != nil {
		return nil, fmt.Errorf("failed to ping ClickHouse: %w", err)
	}

	store := &Store{
		conn:   conn,
		config: cfg,
	}

	// Initialize schema
	if err := store.initializeSchema(ctx); err != nil {
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	return store, nil
}

// Close closes the ClickHouse connection
func (s *Store) Close() error {
	return s.conn.Close()
}

// initializeSchema creates tables and indexes if they don't exist
func (s *Store) initializeSchema(ctx context.Context) error {
	// Create metrics table
	query := `
	CREATE TABLE IF NOT EXISTS metrics (
		timestamp DateTime64(3),
		metric_name LowCardinality(String),
		value Float64,
		dimensions Map(String, String),
		INDEX idx_metric metric_name TYPE bloom_filter GRANULARITY 1
	) ENGINE = MergeTree()
	PARTITION BY toYYYYMM(timestamp)
	ORDER BY (metric_name, timestamp)
	TTL timestamp + INTERVAL 30 DAY
	SETTINGS index_granularity = 8192
	`

	if err := s.conn.Exec(ctx, query); err != nil {
		return fmt.Errorf("failed to create metrics table: %w", err)
	}

	// Create materialized view for cardinality tracking (optional optimization)
	cardinalityView := `
	CREATE MATERIALIZED VIEW IF NOT EXISTS metrics_cardinality
	ENGINE = SummingMergeTree()
	PARTITION BY toYYYYMM(date)
	ORDER BY (metric_name, dimension_key, dimension_value)
	AS SELECT
		toDate(timestamp) as date,
		metric_name,
		arrayJoin(mapKeys(dimensions)) as dimension_key,
		dimensions[dimension_key] as dimension_value,
		count() as count
	FROM metrics
	GROUP BY date, metric_name, dimension_key, dimension_value
	`

	if err := s.conn.Exec(ctx, cardinalityView); err != nil {
		// Log but don't fail - materialized view is optional
		fmt.Printf("Warning: failed to create cardinality view: %v\n", err)
	}

	return nil
}
```

#### Schema Design Decisions

**Table Structure:**
- `DateTime64(3)`: Millisecond precision timestamps
- `LowCardinality(String)`: Automatic dictionary encoding for metric names (huge compression)
- `Map(String, String)`: Native map type for dimensions (better than JSON)
- `Float64`: Standard metric values

**Engine: MergeTree**
- Optimized for inserts and range scans
- Background merging of data parts
- Supports TTL for automatic data deletion

**Partitioning:**
- `PARTITION BY toYYYYMM(timestamp)`: Monthly partitions
- Enables efficient data deletion (drop entire partition)
- Parallelizes queries across partitions

**Ordering:**
- `ORDER BY (metric_name, timestamp)`: Primary key
- Data sorted on disk for fast range scans
- Bloom filter index on metric_name for fast lookups

**TTL:**
- `TTL timestamp + INTERVAL 30 DAY`: Automatic cleanup after 30 days
- Runs in background, no manual intervention

#### Tests

```go
// pkg/storage/clickhouse/store_test.go
package clickhouse

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/yourusername/luminate/pkg/config"
)

func TestNewStore(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	cfg := config.ClickHouseConfig{
		Addresses:       []string{"localhost:9000"},
		Database:        "luminate_test",
		Username:        "default",
		Password:        "",
		MaxOpenConns:    5,
		MaxIdleConns:    2,
		ConnMaxLifetime: time.Hour,
	}

	store, err := NewStore(cfg)
	assert.NoError(t, err)
	assert.NotNil(t, store)

	defer store.Close()

	// Verify connection
	err = store.Health(context.Background())
	assert.NoError(t, err)
}
```

#### Acceptance Criteria

- [ ] Connection pool configured optimally
- [ ] Schema creates successfully
- [ ] Ping verifies connectivity
- [ ] Graceful error handling for connection failures
- [ ] Integration tests pass with real ClickHouse

---

### 2. Batch Write Operations

**Effort:** 1.5 days
**File:** `pkg/storage/clickhouse/write.go`

#### Technical Specification

```go
// pkg/storage/clickhouse/write.go
package clickhouse

import (
	"context"
	"fmt"

	"github.com/yourusername/luminate/pkg/models"
)

// Write stores metrics in ClickHouse using batch insert
func (s *Store) Write(ctx context.Context, metrics []models.Metric) error {
	if len(metrics) == 0 {
		return nil
	}

	// Validate all metrics before writing
	if err := models.ValidateMany(metrics); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	// Prepare batch
	batch, err := s.conn.PrepareBatch(ctx, "INSERT INTO metrics")
	if err != nil {
		return fmt.Errorf("failed to prepare batch: %w", err)
	}

	// Append all metrics to batch
	for _, metric := range metrics {
		err := batch.Append(
			metric.Timestamp,
			metric.Name,
			metric.Value,
			metric.Dimensions,
		)
		if err != nil {
			return fmt.Errorf("failed to append metric %s: %w", metric.Name, err)
		}
	}

	// Send batch
	if err := batch.Send(); err != nil {
		return fmt.Errorf("failed to send batch: %w", err)
	}

	return nil
}

// WriteBatched writes metrics in smaller batches (for very large inputs)
func (s *Store) WriteBatched(ctx context.Context, metrics []models.Metric, batchSize int) error {
	for i := 0; i < len(metrics); i += batchSize {
		end := i + batchSize
		if end > len(metrics) {
			end = len(metrics)
		}

		batch := metrics[i:end]
		if err := s.Write(ctx, batch); err != nil {
			return fmt.Errorf("failed to write batch %d-%d: %w", i, end, err)
		}
	}

	return nil
}
```

#### Performance Optimization

**Batch Size:**
- ClickHouse optimizes for batches of 10K-100K rows
- Use `PrepareBatch()` for native protocol efficiency
- Single network round-trip per batch

**Compression:**
- Native protocol uses LZ4 compression automatically
- 10x+ reduction in network bandwidth
- Configurable via connection settings

**Async Inserts (Optional):**
```go
// For even higher throughput, use async inserts
Settings: clickhouse.Settings{
	"async_insert": 1,
	"wait_for_async_insert": 0,  // Don't wait for insert to complete
	"async_insert_max_data_size": 10485760,  // 10MB buffer
	"async_insert_busy_timeout_ms": 1000,    // Flush every 1s
}
```

#### Tests

```go
func TestWrite(t *testing.T) {
	store := setupTestStore(t)
	defer store.Close()

	metrics := []models.Metric{
		{
			Name:      "api_latency",
			Timestamp: time.Now(),
			Value:     145.5,
			Dimensions: map[string]string{"customer_id": "cust_123"},
		},
	}

	err := store.Write(context.Background(), metrics)
	assert.NoError(t, err)

	// Verify write
	results, err := store.QueryRange(context.Background(), storage.QueryRequest{
		MetricName: "api_latency",
		Start:      time.Now().Add(-1 * time.Minute),
		End:        time.Now().Add(1 * time.Minute),
	})
	assert.NoError(t, err)
	assert.Len(t, results, 1)
}

func BenchmarkWrite(b *testing.B) {
	store := setupTestStore(b)
	defer store.Close()

	metrics := generateTestMetrics(1000)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		store.Write(context.Background(), metrics)
	}
	// Target: 100K+ metrics/second
}
```

#### Acceptance Criteria

- [ ] Successfully writes single metric
- [ ] Successfully writes large batches (10K+ metrics)
- [ ] Validates all metrics before writing
- [ ] Achieves 100,000+ metrics/second throughput
- [ ] Handles ClickHouse connection errors gracefully
- [ ] Tests pass with various batch sizes

---

### 3. QueryRange Implementation

**Effort:** 1 day
**File:** `pkg/storage/clickhouse/query.go`

#### Technical Specification

```go
// pkg/storage/clickhouse/query.go
package clickhouse

import (
	"context"
	"fmt"
	"strings"

	"github.com/yourusername/luminate/pkg/models"
	"github.com/yourusername/luminate/pkg/storage"
)

// QueryRange retrieves raw metric points
func (s *Store) QueryRange(ctx context.Context, req storage.QueryRequest) ([]models.MetricPoint, error) {
	// Build WHERE clause
	whereClauses := []string{
		"metric_name = ?",
		"timestamp BETWEEN ? AND ?",
	}
	args := []interface{}{req.MetricName, req.Start, req.End}

	// Add dimension filters
	for key, value := range req.Filters {
		whereClauses = append(whereClauses, fmt.Sprintf("dimensions['%s'] = ?", key))
		args = append(args, value)
	}

	// Build query
	query := fmt.Sprintf(`
		SELECT 
			timestamp,
			value,
			dimensions
		FROM metrics
		WHERE %s
		ORDER BY timestamp ASC
		LIMIT ?
	`, strings.Join(whereClauses, " AND "))

	limit := req.Limit
	if limit == 0 {
		limit = 10000 // Default limit
	}
	args = append(args, limit)

	// Apply timeout
	if req.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, req.Timeout)
		defer cancel()
	}

	// Execute query
	rows, err := s.conn.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query failed: %w", err)
	}
	defer rows.Close()

	// Parse results
	results := make([]models.MetricPoint, 0, limit)
	for rows.Next() {
		var point models.MetricPoint
		if err := rows.Scan(&point.Timestamp, &point.Value, &point.Dimensions); err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}
		results = append(results, point)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("row iteration error: %w", err)
	}

	return results, nil
}
```

#### Query Optimization

**Index Usage:**
- Bloom filter on `metric_name` provides fast lookups
- Range scan on `timestamp` uses primary key ordering
- Map access `dimensions['key']` is optimized by ClickHouse

**Partition Pruning:**
- ClickHouse automatically prunes partitions based on timestamp
- Query only touches relevant monthly partitions

**Parallel Execution:**
- ClickHouse parallelizes across multiple cores
- Multiple partitions processed concurrently

#### Tests

```go
func TestQueryRange(t *testing.T) {
	store := setupTestStore(t)
	defer store.Close()

	// Write test data
	now := time.Now()
	metrics := []models.Metric{
		{Name: "cpu", Timestamp: now.Add(-2 * time.Minute), Value: 70},
		{Name: "cpu", Timestamp: now.Add(-1 * time.Minute), Value: 80},
		{Name: "cpu", Timestamp: now, Value: 90},
	}
	store.Write(context.Background(), metrics)

	// Query
	results, err := store.QueryRange(context.Background(), storage.QueryRequest{
		MetricName: "cpu",
		Start:      now.Add(-3 * time.Minute),
		End:        now.Add(1 * time.Minute),
	})

	assert.NoError(t, err)
	assert.Len(t, results, 3)
	assert.Equal(t, 70.0, results[0].Value)
	assert.Equal(t, 90.0, results[2].Value)
}

func BenchmarkQueryRange(b *testing.B) {
	store := setupTestStore(b)
	defer store.Close()

	// Pre-populate data
	populateTestData(store, 100000) // 100K points

	now := time.Now()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		store.QueryRange(context.Background(), storage.QueryRequest{
			MetricName: "test_metric",
			Start:      now.Add(-1 * time.Hour),
			End:        now,
			Limit:      10000,
		})
	}
	// Target: p95 latency < 100ms
}
```

#### Acceptance Criteria

- [ ] Returns metrics within time range
- [ ] Applies dimension filters correctly
- [ ] Respects limit parameter
- [ ] Handles context timeout
- [ ] Query latency < 100ms for 10K points
- [ ] Tests cover edge cases

---

### 4. Aggregate Operations with Native SQL

**Effort:** 2 days
**File:** `pkg/storage/clickhouse/aggregate.go`

#### Technical Specification

```go
// pkg/storage/clickhouse/aggregate.go
package clickhouse

import (
	"context"
	"fmt"
	"strings"

	"github.com/yourusername/luminate/pkg/models"
	"github.com/yourusername/luminate/pkg/storage"
)

// Aggregate computes aggregated results using ClickHouse native functions
func (s *Store) Aggregate(ctx context.Context, req storage.AggregateRequest) ([]models.AggregateResult, error) {
	// Map aggregation type to SQL function
	aggFunc, err := getAggregationFunction(req.Aggregation)
	if err != nil {
		return nil, err
	}

	// Build SELECT clause with GROUP BY
	selectCols := []string{}
	for _, dim := range req.GroupBy {
		selectCols = append(selectCols, fmt.Sprintf("dimensions['%s'] as %s", dim, dim))
	}

	selectClause := ""
	if len(selectCols) > 0 {
		selectClause = strings.Join(selectCols, ", ") + ", "
	}

	// Build WHERE clause
	whereClauses := []string{
		"metric_name = ?",
		"timestamp BETWEEN ? AND ?",
	}
	args := []interface{}{req.MetricName, req.Start, req.End}

	for key, value := range req.Filters {
		whereClauses = append(whereClauses, fmt.Sprintf("dimensions['%s'] = ?", key))
		args = append(args, value)
	}

	// Build GROUP BY clause
	groupByClause := ""
	if len(req.GroupBy) > 0 {
		groupByClause = "GROUP BY " + strings.Join(req.GroupBy, ", ")
	}

	// Build complete query
	query := fmt.Sprintf(`
		SELECT 
			%s
			%s as value,
			COUNT(*) as count
		FROM metrics
		WHERE %s
		%s
		ORDER BY value DESC
		LIMIT ?
	`, selectClause, aggFunc, strings.Join(whereClauses, " AND "), groupByClause)

	limit := req.Limit
	if limit == 0 {
		limit = 100
	}
	args = append(args, limit)

	// Apply timeout
	if req.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, req.Timeout)
		defer cancel()
	}

	// Execute query
	rows, err := s.conn.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("aggregate query failed: %w", err)
	}
	defer rows.Close()

	// Parse results
	results := make([]models.AggregateResult, 0)
	for rows.Next() {
		result := models.AggregateResult{
			GroupKey: make(map[string]string),
		}

		// Prepare scan targets
		scanTargets := []interface{}{}
		
		// Add group key targets
		for _, dim := range req.GroupBy {
			var dimValue string
			scanTargets = append(scanTargets, &dimValue)
			defer func(d string, dv *string) {
				result.GroupKey[d] = *dv
			}(dim, &dimValue)
		}

		// Add value and count
		scanTargets = append(scanTargets, &result.Value, &result.Count)

		if err := rows.Scan(scanTargets...); err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}

		results = append(results, result)
	}

	return results, nil
}

// getAggregationFunction maps storage.AggregationType to ClickHouse SQL
func getAggregationFunction(aggType storage.AggregationType) (string, error) {
	switch aggType {
	case storage.AVG:
		return "AVG(value)", nil
	case storage.SUM:
		return "SUM(value)", nil
	case storage.COUNT:
		return "COUNT(*)", nil
	case storage.MIN:
		return "MIN(value)", nil
	case storage.MAX:
		return "MAX(value)", nil
	case storage.P50:
		return "quantile(0.50)(value)", nil
	case storage.P95:
		return "quantile(0.95)(value)", nil
	case storage.P99:
		return "quantile(0.99)(value)", nil
	case storage.INTEGRAL:
		// Time-weighted integral using window functions
		return `
			SUM(value * (
				toUnixTimestamp64Milli(timestamp) - 
				lagInFrame(toUnixTimestamp64Milli(timestamp), 1, toUnixTimestamp64Milli(timestamp)) 
				OVER (ORDER BY timestamp)
			)) / 1000.0
		`, nil
	default:
		return "", fmt.Errorf("unsupported aggregation type: %v", aggType)
	}
}
```

#### ClickHouse Aggregation Functions

**Native Quantiles:**
- `quantile(0.95)(value)`: Uses reservoir sampling, highly efficient
- Better than sorting: O(n) instead of O(n log n)
- Configurable precision via `quantile(level)(x)` variants

**Window Functions (INTEGRAL):**
- `lagInFrame()`: Access previous row value
- `OVER (ORDER BY timestamp)`: Define window
- Calculates trapezoidal rule in single pass

**Performance:**
- All aggregations run in parallel across cores
- Columnar format means only value column is read
- Pre-aggregation via materialized views (optional)

#### Tests

```go
func TestAggregateAVG(t *testing.T) {
	store := setupTestStore(t)
	defer store.Close()

	now := time.Now()
	metrics := []models.Metric{
		{Name: "latency", Timestamp: now, Value: 100, Dimensions: map[string]string{"customer": "c1"}},
		{Name: "latency", Timestamp: now, Value: 200, Dimensions: map[string]string{"customer": "c1"}},
		{Name: "latency", Timestamp: now, Value: 300, Dimensions: map[string]string{"customer": "c1"}},
	}
	store.Write(context.Background(), metrics)

	// Wait for data to be visible
	time.Sleep(100 * time.Millisecond)

	results, err := store.Aggregate(context.Background(), storage.AggregateRequest{
		MetricName:  "latency",
		Start:       now.Add(-1 * time.Minute),
		End:         now.Add(1 * time.Minute),
		Aggregation: storage.AVG,
		GroupBy:     []string{"customer"},
	})

	assert.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Equal(t, 200.0, results[0].Value)
}

func TestAggregateP95(t *testing.T) {
	store := setupTestStore(t)
	defer store.Close()

	now := time.Now()
	metrics := make([]models.Metric, 100)
	for i := 0; i < 100; i++ {
		metrics[i] = models.Metric{
			Name:      "latency",
			Timestamp: now,
			Value:     float64(i),
			Dimensions: map[string]string{"customer": "c1"},
		}
	}
	store.Write(context.Background(), metrics)

	time.Sleep(100 * time.Millisecond)

	results, err := store.Aggregate(context.Background(), storage.AggregateRequest{
		MetricName:  "latency",
		Start:       now.Add(-1 * time.Minute),
		End:         now.Add(1 * time.Minute),
		Aggregation: storage.P95,
		GroupBy:     []string{"customer"},
	})

	assert.NoError(t, err)
	assert.Len(t, results, 1)
	assert.InDelta(t, 95.0, results[0].Value, 1.0)
}

func BenchmarkAggregate(b *testing.B) {
	store := setupTestStore(b)
	defer store.Close()

	// Pre-populate 1M points
	populateTestData(store, 1000000)

	now := time.Now()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		store.Aggregate(context.Background(), storage.AggregateRequest{
			MetricName:  "test_metric",
			Start:       now.Add(-1 * time.Hour),
			End:         now,
			Aggregation: storage.P95,
			GroupBy:     []string{"customer_id"},
			Limit:       100,
		})
	}
	// Target: < 100ms for 1M points
}
```

#### Acceptance Criteria

- [ ] All 9 aggregation types work correctly
- [ ] GROUP BY produces correct groups
- [ ] Native quantile functions used for percentiles
- [ ] INTEGRAL uses window functions
- [ ] Query latency < 100ms for 1M points
- [ ] Tests cover all aggregation types

---

### 5. Rate Calculations

**Effort:** 0.5 days
**File:** `pkg/storage/clickhouse/rate.go`

#### Technical Specification

```go
// pkg/storage/clickhouse/rate.go
package clickhouse

import (
	"context"
	"fmt"
	"strings"

	"github.com/yourusername/luminate/pkg/models"
	"github.com/yourusername/luminate/pkg/storage"
)

// Rate calculates rate of change using ClickHouse window functions
func (s *Store) Rate(ctx context.Context, req storage.RateRequest) ([]models.RatePoint, error) {
	// Build WHERE clause
	whereClauses := []string{
		"metric_name = ?",
		"timestamp BETWEEN ? AND ?",
	}
	args := []interface{}{req.MetricName, req.Start, req.End}

	for key, value := range req.Filters {
		whereClauses = append(whereClauses, fmt.Sprintf("dimensions['%s'] = ?", key))
		args = append(args, value)
	}

	// Calculate rate using window functions
	query := fmt.Sprintf(`
		WITH bucketed AS (
			SELECT
				toStartOfInterval(timestamp, INTERVAL ? SECOND) as bucket,
				value,
				timestamp
			FROM metrics
			WHERE %s
			ORDER BY timestamp
		)
		SELECT
			bucket,
			(MAX(value) - MIN(value)) / 
			(toUnixTimestamp64Milli(MAX(timestamp)) - toUnixTimestamp64Milli(MIN(timestamp))) * 1000.0 as rate
		FROM bucketed
		GROUP BY bucket
		HAVING rate >= 0  -- Ignore counter resets
		ORDER BY bucket
	`, strings.Join(whereClauses, " AND "))

	args = append([]interface{}{int(req.Interval.Seconds())}, args...)

	// Execute
	rows, err := s.conn.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("rate query failed: %w", err)
	}
	defer rows.Close()

	// Parse results
	results := make([]models.RatePoint, 0)
	for rows.Next() {
		var point models.RatePoint
		if err := rows.Scan(&point.Timestamp, &point.Rate); err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}
		results = append(results, point)
	}

	return results, nil
}
```

#### Acceptance Criteria

- [ ] Calculates correct rate (change per second)
- [ ] Groups into time buckets
- [ ] Handles counter resets (negative changes ignored)
- [ ] Tests pass

---

### 6. Discovery Operations

**Effort:** 0.5 days
**File:** `pkg/storage/clickhouse/discovery.go`

#### Technical Specification

```go
// pkg/storage/clickhouse/discovery.go
package clickhouse

import (
	"context"
	"fmt"
)

// ListMetrics returns all unique metric names
func (s *Store) ListMetrics(ctx context.Context) ([]string, error) {
	query := `
		SELECT DISTINCT metric_name
		FROM metrics
		ORDER BY metric_name
	`

	rows, err := s.conn.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list metrics failed: %w", err)
	}
	defer rows.Close()

	results := make([]string, 0)
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		results = append(results, name)
	}

	return results, nil
}

// ListDimensionKeys returns all dimension keys for a metric
func (s *Store) ListDimensionKeys(ctx context.Context, metricName string) ([]string, error) {
	query := `
		SELECT DISTINCT arrayJoin(mapKeys(dimensions)) as key
		FROM metrics
		WHERE metric_name = ?
		ORDER BY key
	`

	rows, err := s.conn.Query(ctx, query, metricName)
	if err != nil {
		return nil, fmt.Errorf("list dimension keys failed: %w", err)
	}
	defer rows.Close()

	results := make([]string, 0)
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return nil, err
		}
		results = append(results, key)
	}

	return results, nil
}

// ListDimensionValues returns top N dimension values by frequency
func (s *Store) ListDimensionValues(ctx context.Context, metricName, dimensionKey string, limit int) ([]string, error) {
	query := `
		SELECT 
			dimensions[?] as value,
			COUNT(*) as count
		FROM metrics
		WHERE metric_name = ?
		  AND has(mapKeys(dimensions), ?)
		GROUP BY value
		ORDER BY count DESC
		LIMIT ?
	`

	rows, err := s.conn.Query(ctx, query, dimensionKey, metricName, dimensionKey, limit)
	if err != nil {
		return nil, fmt.Errorf("list dimension values failed: %w", err)
	}
	defer rows.Close()

	results := make([]string, 0)
	for rows.Next() {
		var value string
		var count uint64
		if err := rows.Scan(&value, &count); err != nil {
			return nil, err
		}
		results = append(results, value)
	}

	return results, nil
}
```

#### Acceptance Criteria

- [ ] ListMetrics returns all unique names
- [ ] ListDimensionKeys uses arrayJoin for map keys
- [ ] ListDimensionValues sorted by frequency
- [ ] Queries are efficient (use indexes)

---

### 7. Data Retention with TTL

**Effort:** 0.5 days
**File:** `pkg/storage/clickhouse/retention.go`

#### Technical Specification

```go
// pkg/storage/clickhouse/retention.go
package clickhouse

import (
	"context"
	"fmt"
	"time"
)

// DeleteBefore removes metrics older than specified time
func (s *Store) DeleteBefore(ctx context.Context, metricName string, before time.Time) error {
	// Use ALTER TABLE MODIFY TTL for efficient deletion
	// Or use DELETE for immediate removal (slower)
	query := `
		DELETE FROM metrics
		WHERE metric_name = ?
		  AND timestamp < ?
	`

	err := s.conn.Exec(ctx, query, metricName, before)
	if err != nil {
		return fmt.Errorf("delete failed: %w", err)
	}

	// Optimize table to reclaim space
	optimizeQuery := "OPTIMIZE TABLE metrics FINAL"
	if err := s.conn.Exec(ctx, optimizeQuery); err != nil {
		// Log but don't fail - optimization is best-effort
		fmt.Printf("Warning: optimize failed: %v\n", err)
	}

	return nil
}

// UpdateRetention updates TTL for automatic cleanup
func (s *Store) UpdateRetention(ctx context.Context, retention time.Duration) error {
	days := int(retention.Hours() / 24)

	query := fmt.Sprintf(`
		ALTER TABLE metrics 
		MODIFY TTL timestamp + INTERVAL %d DAY
	`, days)

	return s.conn.Exec(ctx, query)
}
```

#### TTL vs DELETE

**TTL (Recommended):**
- Automatic cleanup in background
- Efficient (drops entire partitions)
- No query overhead
- Set once, runs forever

**DELETE (Manual):**
- Immediate removal
- Creates mutations (rewrite data)
- Slower, uses resources
- Use for one-off deletions

#### Acceptance Criteria

- [ ] DeleteBefore works correctly
- [ ] UpdateRetention modifies TTL
- [ ] Space reclaimed after deletion
- [ ] Tests validate retention

---

### 8. Health Checks

**Effort:** 0.5 days
**File:** `pkg/storage/clickhouse/health.go`

#### Technical Specification

```go
// pkg/storage/clickhouse/health.go
package clickhouse

import (
	"context"
	"fmt"
)

// Health checks if ClickHouse is accessible and healthy
func (s *Store) Health(ctx context.Context) error {
	// Ping connection
	if err := s.conn.Ping(ctx); err != nil {
		return fmt.Errorf("ping failed: %w", err)
	}

	// Check if tables exist
	query := `
		SELECT count() 
		FROM system.tables 
		WHERE database = ? AND name = 'metrics'
	`

	var count uint64
	row := s.conn.QueryRow(ctx, query, s.config.Database)
	if err := row.Scan(&count); err != nil {
		return fmt.Errorf("table check failed: %w", err)
	}

	if count == 0 {
		return fmt.Errorf("metrics table does not exist")
	}

	// Check if we can write (optional)
	testQuery := "SELECT 1"
	if err := s.conn.QueryRow(ctx, testQuery).Scan(&count); err != nil {
		return fmt.Errorf("query test failed: %w", err)
	}

	return nil
}

// GetStorageSize returns total storage used
func (s *Store) GetStorageSize(ctx context.Context) (uint64, error) {
	query := `
		SELECT SUM(bytes)
		FROM system.parts
		WHERE database = ? AND table = 'metrics' AND active
	`

	var size uint64
	row := s.conn.QueryRow(ctx, query, s.config.Database)
	if err := row.Scan(&size); err != nil {
		return 0, err
	}

	return size, nil
}
```

#### Acceptance Criteria

- [ ] Health() verifies connectivity
- [ ] Health() checks table existence
- [ ] GetStorageSize returns accurate size
- [ ] Tests validate health checks

---

## Testing Strategy

### Unit Tests
- Test SQL generation logic
- Test error handling
- Test timeout handling

### Integration Tests
- Requires real ClickHouse instance
- Use Docker for CI/CD
- Test full write → query cycle

### Performance Tests
```bash
# Start ClickHouse
docker run -d --name clickhouse-test \
  -p 9000:9000 -p 8123:8123 \
  clickhouse/clickhouse-server

# Run benchmarks
go test -bench=. -benchmem ./pkg/storage/clickhouse
```

### Load Testing
```go
func TestHighVolumeWrites(t *testing.T) {
	store := setupTestStore(t)
	defer store.Close()

	// Write 1M metrics
	batchSize := 10000
	for i := 0; i < 100; i++ {
		metrics := generateTestMetrics(batchSize)
		err := store.Write(context.Background(), metrics)
		assert.NoError(t, err)
	}

	// Verify
	count, err := store.countMetrics(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, 1000000, count)
}
```

## Optimization Strategies

### 1. Materialized Views (Optional)

For frequently queried aggregations:

```sql
CREATE MATERIALIZED VIEW metrics_hourly
ENGINE = SummingMergeTree()
PARTITION BY toYYYYMM(timestamp)
ORDER BY (metric_name, customer_id, toStartOfHour(timestamp))
AS SELECT
    metric_name,
    dimensions['customer_id'] as customer_id,
    toStartOfHour(timestamp) as timestamp,
    AVG(value) as avg_value,
    quantile(0.95)(value) as p95_value,
    COUNT(*) as count
FROM metrics
GROUP BY metric_name, customer_id, timestamp
```

### 2. Distributed Tables (Multi-Node)

```sql
CREATE TABLE metrics_distributed AS metrics
ENGINE = Distributed(cluster, database, metrics, rand())
```

### 3. Compression Codecs

```sql
value Float64 CODEC(Delta, LZ4)  -- Delta encoding + LZ4
```

## Acceptance Criteria Summary

### Functional
- [ ] All MetricsStore methods implemented
- [ ] All aggregations use native SQL functions
- [ ] Discovery APIs optimized
- [ ] Retention with TTL configured

### Performance
- [ ] Write: 100K+ metrics/second per pod
- [ ] Query (p95): < 100ms for typical queries
- [ ] Aggregate (p95): < 100ms for 1M points
- [ ] Horizontal scaling validated

### Quality
- [ ] Integration tests pass
- [ ] Benchmarks meet targets
- [ ] Documentation complete
- [ ] Migration guide from BadgerDB

## Rollout Plan

1. **Day 1-2:** Connection management and schema
2. **Day 3-4:** Write operations and QueryRange
3. **Day 5-6:** Aggregate operations
4. **Day 7:** Rate, discovery, retention
5. **Day 8:** Performance tuning and optimization

---

**Status:** Ready for Implementation
**Last Updated:** 2024-12-08
