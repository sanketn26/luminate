# WS1: Storage Backend - BadgerDB Implementation

**Priority:** P0 (Critical Path)
**Estimated Effort:** 8-10 days
**Dependencies:** None
**Owner:** Backend Team

## Overview

Implement the complete BadgerDB storage backend that satisfies the `MetricsStore` interface. BadgerDB is an embedded LSM-tree based key-value store that provides the foundation for development and single-instance deployments.

## Objectives

- Implement all `MetricsStore` interface methods
- Achieve 10,000+ metrics/second write throughput
- Support query p95 latency < 500ms
- Handle up to 1M unique series per metric
- Provide data retention and cleanup

## Work Items

### 1. Key Schema Design

**Effort:** 1 day
**File:** `pkg/storage/badger/schema.go`

#### Technical Specification

Design an efficient key schema that enables:
- Fast time-range scans
- Efficient filtering by dimensions
- Compact key size

**Recommended Schema:**
```
Key Format: metric:<metric_name>:<timestamp_ms>:<dimension_hash>
Value Format: <float64_value>|<json_dimensions>

Examples:
metric:api_latency:1735603200000:a3f5b2c1 -> 145.5|{"customer_id":"cust_123","endpoint":"/api/users"}
metric:cpu_usage:1735603201000:d2e4f6a8 -> 75.2|{"customer_id":"cust_123","host":"web-01"}
```

**Dimension Hash Function:**
```go
// pkg/storage/badger/schema.go
package badger

import (
    "crypto/sha256"
    "encoding/hex"
    "encoding/json"
    "fmt"
    "sort"
    "time"
)

// MetricKey represents the key structure
type MetricKey struct {
    MetricName string
    Timestamp  time.Time
    Dimensions map[string]string
}

// Encode creates a BadgerDB key
func (k *MetricKey) Encode() []byte {
    hash := dimensionHash(k.Dimensions)
    timestampMs := k.Timestamp.UnixMilli()
    key := fmt.Sprintf("metric:%s:%d:%s", k.MetricName, timestampMs, hash)
    return []byte(key)
}

// dimensionHash creates a deterministic hash of dimensions
func dimensionHash(dims map[string]string) string {
    // Sort keys for deterministic hash
    keys := make([]string, 0, len(dims))
    for k := range dims {
        keys = append(keys, k)
    }
    sort.Strings(keys)

    // Build canonical JSON
    canonical := make([]string, 0, len(keys))
    for _, k := range keys {
        canonical = append(canonical, fmt.Sprintf("%s=%s", k, dims[k]))
    }

    // SHA256 hash, take first 8 chars
    h := sha256.New()
    for _, s := range canonical {
        h.Write([]byte(s))
    }
    return hex.EncodeToString(h.Sum(nil))[:16]
}

// EncodeValue creates the value part
func EncodeValue(value float64, dimensions map[string]string) ([]byte, error) {
    dimJSON, err := json.Marshal(dimensions)
    if err != nil {
        return nil, err
    }

    // Format: float64 bytes + '|' + JSON
    valueBytes := make([]byte, 8)
    binary.LittleEndian.PutUint64(valueBytes, math.Float64bits(value))

    result := append(valueBytes, '|')
    result = append(result, dimJSON...)
    return result, nil
}

// DecodeValue extracts value and dimensions
func DecodeValue(data []byte) (float64, map[string]string, error) {
    if len(data) < 10 { // 8 bytes + '|' + at least {}
        return 0, nil, errors.New("invalid value format")
    }

    // Extract float64
    valueBits := binary.LittleEndian.Uint64(data[:8])
    value := math.Float64frombits(valueBits)

    // Extract dimensions JSON
    dimJSON := data[9:] // Skip 8 bytes + '|'
    var dimensions map[string]string
    if err := json.Unmarshal(dimJSON, &dimensions); err != nil {
        return 0, nil, err
    }

    return value, dimensions, nil
}
```

#### Implementation Steps

1. Create `pkg/storage/badger/schema.go`
2. Implement `MetricKey` struct and `Encode()` method
3. Implement `dimensionHash()` function
4. Implement `EncodeValue()` and `DecodeValue()` functions
5. Write unit tests for encoding/decoding
6. Benchmark key encoding performance

#### Tests

```go
// pkg/storage/badger/schema_test.go
func TestMetricKeyEncoding(t *testing.T) {
    key := &MetricKey{
        MetricName: "api_latency",
        Timestamp:  time.Unix(1735603200, 0),
        Dimensions: map[string]string{
            "customer_id": "cust_123",
            "endpoint":    "/api/users",
        },
    }

    encoded := key.Encode()
    assert.Contains(t, string(encoded), "metric:api_latency:")

    // Test deterministic hashing
    encoded2 := key.Encode()
    assert.Equal(t, encoded, encoded2)
}

func TestDimensionHashDeterministic(t *testing.T) {
    dims := map[string]string{"b": "2", "a": "1", "c": "3"}
    hash1 := dimensionHash(dims)
    hash2 := dimensionHash(dims)
    assert.Equal(t, hash1, hash2)
}
```

#### Acceptance Criteria

- [ ] Keys are lexicographically ordered by metric name, then timestamp
- [ ] Dimension hash is deterministic (same dimensions = same hash)
- [ ] Encoding/decoding roundtrip preserves all data
- [ ] Unit tests pass with 100% coverage
- [ ] Benchmark shows <1µs per encode operation

---

### 2. Store Initialization and Configuration

**Effort:** 0.5 days
**File:** `pkg/storage/badger/store.go`

#### Technical Specification

```go
// pkg/storage/badger/store.go
package badger

import (
    "context"
    "github.com/dgraph-io/badger/v4"
    "github.com/yourusername/luminate/pkg/config"
    "github.com/yourusername/luminate/pkg/storage"
)

type Store struct {
    db     *badger.DB
    config config.BadgerConfig
}

// NewStore creates a new BadgerDB storage backend
func NewStore(cfg config.BadgerConfig) (*Store, error) {
    opts := badger.DefaultOptions(cfg.DataDir).
        WithValueLogMaxEntries(uint32(cfg.ValueLogMaxEntries)).
        WithNumVersionsToKeep(1). // We don't need versioning
        WithCompactL0OnClose(true).
        WithNumCompactors(2).
        WithNumLevelZeroTables(2).
        WithNumLevelZeroTablesStall(4)

    db, err := badger.Open(opts)
    if err != nil {
        return nil, fmt.Errorf("failed to open badger: %w", err)
    }

    store := &Store{
        db:     db,
        config: cfg,
    }

    // Start background goroutines
    go store.runGarbageCollection()

    return store, nil
}

// Close closes the database
func (s *Store) Close() error {
    return s.db.Close()
}

// runGarbageCollection periodically runs value log GC
func (s *Store) runGarbageCollection() {
    ticker := time.NewTicker(5 * time.Minute)
    defer ticker.Stop()

    for range ticker.C {
        for {
            err := s.db.RunValueLogGC(0.5) // Discard 50% or more
            if err != nil {
                break // No more GC needed
            }
        }
    }
}
```

#### Implementation Steps

1. Create `pkg/storage/badger/store.go`
2. Implement `NewStore()` with optimized BadgerDB options
3. Implement `Close()` method
4. Add garbage collection goroutine
5. Add logging for operations
6. Test with different configurations

#### Acceptance Criteria

- [ ] Store initializes successfully with valid config
- [ ] Store creates data directory if missing
- [ ] Garbage collection runs periodically
- [ ] Close() cleanly shuts down database
- [ ] Error handling for invalid paths

---

### 3. Write Operations

**Effort:** 1.5 days
**File:** `pkg/storage/badger/write.go`

#### Technical Specification

```go
// pkg/storage/badger/write.go
package badger

import (
    "context"
    "github.com/dgraph-io/badger/v4"
    "github.com/yourusername/luminate/pkg/models"
)

// Write stores metrics in BadgerDB
func (s *Store) Write(ctx context.Context, metrics []models.Metric) error {
    if len(metrics) == 0 {
        return nil
    }

    // Use WriteBatch for atomic writes
    wb := s.db.NewWriteBatch()
    defer wb.Cancel()

    for _, metric := range metrics {
        // Validate metric
        if err := metric.Validate(); err != nil {
            return fmt.Errorf("invalid metric %s: %w", metric.Name, err)
        }

        // Encode key and value
        key := &MetricKey{
            MetricName: metric.Name,
            Timestamp:  metric.Timestamp,
            Dimensions: metric.Dimensions,
        }

        keyBytes := key.Encode()
        valueBytes, err := EncodeValue(metric.Value, metric.Dimensions)
        if err != nil {
            return fmt.Errorf("failed to encode value: %w", err)
        }

        // Add to batch
        if err := wb.Set(keyBytes, valueBytes); err != nil {
            return fmt.Errorf("failed to add to batch: %w", err)
        }
    }

    // Commit batch
    if err := wb.Flush(); err != nil {
        return fmt.Errorf("failed to commit batch: %w", err)
    }

    return nil
}
```

#### Implementation Steps

1. Implement `Write()` method with batch support
2. Add validation for each metric
3. Use `WriteBatch` for atomic writes
4. Add error handling and logging
5. Write unit tests
6. Benchmark write throughput

#### Tests

```go
func TestWriteMetrics(t *testing.T) {
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
}
```

#### Acceptance Criteria

- [ ] Successfully writes single metric
- [ ] Successfully writes batch of metrics
- [ ] Validates all metrics before writing
- [ ] Atomic writes (all or nothing)
- [ ] Achieves 10,000+ metrics/second throughput
- [ ] Tests pass with edge cases (empty batch, invalid metrics)

---

### 4. QueryRange Implementation

**Effort:** 1 day
**File:** `pkg/storage/badger/query.go`

#### Technical Specification

```go
// pkg/storage/badger/query.go
package badger

import (
    "context"
    "github.com/dgraph-io/badger/v4"
    "github.com/yourusername/luminate/pkg/models"
    "github.com/yourusername/luminate/pkg/storage"
)

// QueryRange retrieves raw metric points
func (s *Store) QueryRange(ctx context.Context, req storage.QueryRequest) ([]models.MetricPoint, error) {
    results := make([]models.MetricPoint, 0)

    err := s.db.View(func(txn *badger.Txn) error {
        opts := badger.DefaultIteratorOptions
        opts.PrefetchValues = true
        opts.PrefetchSize = 100

        it := txn.NewIterator(opts)
        defer it.Close()

        // Create prefix for metric name
        prefix := []byte(fmt.Sprintf("metric:%s:", req.MetricName))
        startKey := []byte(fmt.Sprintf("metric:%s:%d:", req.MetricName, req.Start.UnixMilli()))
        endKey := []byte(fmt.Sprintf("metric:%s:%d:", req.MetricName, req.End.UnixMilli()))

        for it.Seek(startKey); it.ValidForPrefix(prefix); it.Next() {
            // Check if we've exceeded time range
            if bytes.Compare(it.Item().Key(), endKey) > 0 {
                break
            }

            // Check context cancellation
            select {
            case <-ctx.Done():
                return ctx.Err()
            default:
            }

            // Decode key to get timestamp
            timestamp, err := extractTimestampFromKey(it.Item().Key())
            if err != nil {
                continue
            }

            // Check time range
            if timestamp.Before(req.Start) || timestamp.After(req.End) {
                continue
            }

            // Decode value
            var value float64
            var dimensions map[string]string
            err = it.Item().Value(func(val []byte) error {
                v, d, err := DecodeValue(val)
                value = v
                dimensions = d
                return err
            })
            if err != nil {
                continue
            }

            // Apply dimension filters
            if !matchesFilters(dimensions, req.Filters) {
                continue
            }

            // Add to results
            results = append(results, models.MetricPoint{
                Timestamp:  timestamp,
                Value:      value,
                Dimensions: dimensions,
            })

            // Check limit
            if req.Limit > 0 && len(results) >= req.Limit {
                break
            }
        }

        return nil
    })

    return results, err
}

// matchesFilters checks if dimensions match the filter criteria
func matchesFilters(dimensions map[string]string, filters map[string]string) bool {
    for filterKey, filterValue := range filters {
        if dimensions[filterKey] != filterValue {
            return false
        }
    }
    return true
}

// extractTimestampFromKey parses timestamp from key
func extractTimestampFromKey(key []byte) (time.Time, error) {
    // Key format: metric:<name>:<timestamp>:<hash>
    parts := bytes.Split(key, []byte(":"))
    if len(parts) < 3 {
        return time.Time{}, errors.New("invalid key format")
    }

    timestampMs, err := strconv.ParseInt(string(parts[2]), 10, 64)
    if err != nil {
        return time.Time{}, err
    }

    return time.UnixMilli(timestampMs), nil
}
```

#### Implementation Steps

1. Implement `QueryRange()` with iterator
2. Add prefix scan optimization
3. Implement filter matching
4. Add limit handling
5. Handle context cancellation
6. Write comprehensive tests
7. Benchmark query performance

#### Tests

```go
func TestQueryRange(t *testing.T) {
    store := setupTestStore(t)
    defer store.Close()

    // Write test data
    now := time.Now()
    metrics := []models.Metric{
        {Name: "cpu", Timestamp: now.Add(-2 * time.Minute), Value: 70, Dimensions: map[string]string{"host": "web-1"}},
        {Name: "cpu", Timestamp: now.Add(-1 * time.Minute), Value: 80, Dimensions: map[string]string{"host": "web-1"}},
        {Name: "cpu", Timestamp: now, Value: 90, Dimensions: map[string]string{"host": "web-1"}},
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
}

func TestQueryRangeWithFilters(t *testing.T) {
    store := setupTestStore(t)
    defer store.Close()

    // Write data for multiple hosts
    now := time.Now()
    metrics := []models.Metric{
        {Name: "cpu", Timestamp: now, Value: 70, Dimensions: map[string]string{"host": "web-1"}},
        {Name: "cpu", Timestamp: now, Value: 80, Dimensions: map[string]string{"host": "web-2"}},
    }
    store.Write(context.Background(), metrics)

    // Query with filter
    results, err := store.QueryRange(context.Background(), storage.QueryRequest{
        MetricName: "cpu",
        Start:      now.Add(-1 * time.Minute),
        End:        now.Add(1 * time.Minute),
        Filters:    map[string]string{"host": "web-1"},
    })

    assert.NoError(t, err)
    assert.Len(t, results, 1)
    assert.Equal(t, 70.0, results[0].Value)
}
```

#### Acceptance Criteria

- [ ] Returns metrics within time range
- [ ] Applies dimension filters correctly
- [ ] Respects limit parameter
- [ ] Handles context cancellation
- [ ] Query latency < 100ms for 10K points
- [ ] Tests cover edge cases (empty results, no matches)

---

### 5. Aggregate Operations

**Effort:** 2.5 days
**File:** `pkg/storage/badger/aggregate.go`

#### Technical Specification

Implement all 9 aggregation types: AVG, SUM, COUNT, MIN, MAX, P50, P95, P99, INTEGRAL.

```go
// pkg/storage/badger/aggregate.go
package badger

import (
    "context"
    "math"
    "sort"
    "github.com/yourusername/luminate/pkg/models"
    "github.com/yourusername/luminate/pkg/storage"
)

// Aggregate computes aggregated results
func (s *Store) Aggregate(ctx context.Context, req storage.AggregateRequest) ([]models.AggregateResult, error) {
    // First, get all raw points
    points, err := s.QueryRange(ctx, storage.QueryRequest{
        MetricName: req.MetricName,
        Start:      req.Start,
        End:        req.End,
        Filters:    req.Filters,
    })
    if err != nil {
        return nil, err
    }

    // Group by specified dimensions
    groups := groupByDimensions(points, req.GroupBy)

    // Apply aggregation to each group
    results := make([]models.AggregateResult, 0, len(groups))
    for groupKey, groupPoints := range groups {
        value, count := computeAggregation(groupPoints, req.Aggregation)

        results = append(results, models.AggregateResult{
            GroupKey: groupKey,
            Value:    value,
            Count:    count,
        })
    }

    // Sort by value descending
    sort.Slice(results, func(i, j int) bool {
        return results[i].Value > results[j].Value
    })

    // Apply limit
    if req.Limit > 0 && len(results) > req.Limit {
        results = results[:req.Limit]
    }

    return results, nil
}

// groupByDimensions groups points by specified dimension keys
func groupByDimensions(points []models.MetricPoint, groupBy []string) map[map[string]string][]models.MetricPoint {
    groups := make(map[string][]models.MetricPoint)

    for _, point := range points {
        // Build group key from specified dimensions
        groupKey := make(map[string]string)
        for _, dim := range groupBy {
            if val, exists := point.Dimensions[dim]; exists {
                groupKey[dim] = val
            }
        }

        // Serialize group key for map
        keyStr := serializeGroupKey(groupKey)
        groups[keyStr] = append(groups[keyStr], point)
    }

    // Convert back to map[map[string]string]
    result := make(map[map[string]string][]models.MetricPoint)
    for keyStr, points := range groups {
        groupKey := deserializeGroupKey(keyStr)
        result[groupKey] = points
    }

    return result
}

// computeAggregation applies the aggregation function
func computeAggregation(points []models.MetricPoint, aggType storage.AggregationType) (float64, int64) {
    if len(points) == 0 {
        return 0, 0
    }

    count := int64(len(points))

    switch aggType {
    case storage.AVG:
        sum := 0.0
        for _, p := range points {
            sum += p.Value
        }
        return sum / float64(count), count

    case storage.SUM:
        sum := 0.0
        for _, p := range points {
            sum += p.Value
        }
        return sum, count

    case storage.COUNT:
        return float64(count), count

    case storage.MIN:
        min := points[0].Value
        for _, p := range points {
            if p.Value < min {
                min = p.Value
            }
        }
        return min, count

    case storage.MAX:
        max := points[0].Value
        for _, p := range points {
            if p.Value > max {
                max = p.Value
            }
        }
        return max, count

    case storage.P50, storage.P95, storage.P99:
        return computePercentile(points, aggType), count

    case storage.INTEGRAL:
        return computeIntegral(points), count

    default:
        return 0, 0
    }
}

// computePercentile calculates percentile values
func computePercentile(points []models.MetricPoint, aggType storage.AggregationType) float64 {
    if len(points) == 0 {
        return 0
    }

    // Extract and sort values
    values := make([]float64, len(points))
    for i, p := range points {
        values[i] = p.Value
    }
    sort.Float64s(values)

    var percentile float64
    switch aggType {
    case storage.P50:
        percentile = 0.50
    case storage.P95:
        percentile = 0.95
    case storage.P99:
        percentile = 0.99
    }

    // Calculate index
    index := int(math.Ceil(percentile * float64(len(values)))) - 1
    if index < 0 {
        index = 0
    }
    if index >= len(values) {
        index = len(values) - 1
    }

    return values[index]
}

// computeIntegral calculates time-weighted integral (area under curve)
func computeIntegral(points []models.MetricPoint) float64 {
    if len(points) < 2 {
        return 0
    }

    // Sort by timestamp
    sort.Slice(points, func(i, j int) bool {
        return points[i].Timestamp.Before(points[j].Timestamp)
    })

    integral := 0.0
    for i := 1; i < len(points); i++ {
        // Trapezoidal rule
        dt := points[i].Timestamp.Sub(points[i-1].Timestamp).Seconds()
        avgValue := (points[i].Value + points[i-1].Value) / 2
        integral += avgValue * dt
    }

    return integral
}
```

#### Implementation Steps

1. Implement `Aggregate()` method
2. Implement `groupByDimensions()` for GROUP BY support
3. Implement `computeAggregation()` with all types
4. Implement `computePercentile()` for P50/P95/P99
5. Implement `computeIntegral()` for time-weighted aggregation
6. Write comprehensive tests for each aggregation type
7. Benchmark aggregation performance

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

    results, err := store.Aggregate(context.Background(), storage.AggregateRequest{
        MetricName:  "latency",
        Start:       now.Add(-1 * time.Minute),
        End:         now.Add(1 * time.Minute),
        Aggregation: storage.AVG,
        GroupBy:     []string{"customer"},
    })

    assert.NoError(t, err)
    assert.Len(t, results, 1)
    assert.Equal(t, 200.0, results[0].Value) // (100+200+300)/3
}

func TestAggregateP95(t *testing.T) {
    store := setupTestStore(t)
    defer store.Close()

    now := time.Now()
    // Create 100 data points
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

    results, err := store.Aggregate(context.Background(), storage.AggregateRequest{
        MetricName:  "latency",
        Start:       now.Add(-1 * time.Minute),
        End:         now.Add(1 * time.Minute),
        Aggregation: storage.P95,
        GroupBy:     []string{"customer"},
    })

    assert.NoError(t, err)
    assert.Len(t, results, 1)
    assert.InDelta(t, 95.0, results[0].Value, 1.0) // p95 should be ~95
}

func TestAggregateIntegral(t *testing.T) {
    store := setupTestStore(t)
    defer store.Close()

    now := time.Now()
    metrics := []models.Metric{
        {Name: "cpu", Timestamp: now, Value: 50, Dimensions: map[string]string{"host": "web-1"}},
        {Name: "cpu", Timestamp: now.Add(10 * time.Second), Value: 100, Dimensions: map[string]string{"host": "web-1"}},
    }
    store.Write(context.Background(), metrics)

    results, err := store.Aggregate(context.Background(), storage.AggregateRequest{
        MetricName:  "cpu",
        Start:       now.Add(-1 * time.Minute),
        End:         now.Add(1 * time.Minute),
        Aggregation: storage.INTEGRAL,
        GroupBy:     []string{"host"},
    })

    assert.NoError(t, err)
    assert.Len(t, results, 1)
    // Integral = (50+100)/2 * 10 = 750 %-seconds
    assert.InDelta(t, 750.0, results[0].Value, 1.0)
}
```

#### Acceptance Criteria

- [ ] All 9 aggregation types work correctly
- [ ] GROUP BY produces correct groups
- [ ] P95/P99 calculated accurately
- [ ] INTEGRAL uses trapezoidal rule
- [ ] Results sorted by value descending
- [ ] Limit applied correctly
- [ ] Tests cover all aggregation types
- [ ] Aggregation latency < 500ms for 10K points

---

### 6. Rate Calculations

**Effort:** 1 day
**File:** `pkg/storage/badger/rate.go`

#### Technical Specification

```go
// pkg/storage/badger/rate.go
package badger

import (
    "context"
    "time"
    "github.com/yourusername/luminate/pkg/models"
    "github.com/yourusername/luminate/pkg/storage"
)

// Rate calculates rate of change for counter metrics
func (s *Store) Rate(ctx context.Context, req storage.RateRequest) ([]models.RatePoint, error) {
    // Get raw points
    points, err := s.QueryRange(ctx, storage.QueryRequest{
        MetricName: req.MetricName,
        Start:      req.Start,
        End:        req.End,
        Filters:    req.Filters,
    })
    if err != nil {
        return nil, err
    }

    if len(points) < 2 {
        return []models.RatePoint{}, nil
    }

    // Sort by timestamp
    sort.Slice(points, func(i, j int) bool {
        return points[i].Timestamp.Before(points[j].Timestamp)
    })

    // Group into time buckets
    buckets := groupIntoBuckets(points, req.Start, req.End, req.Interval)

    // Calculate rate for each bucket
    results := make([]models.RatePoint, 0, len(buckets))
    for _, bucket := range buckets {
        rate := calculateBucketRate(bucket)
        results = append(results, models.RatePoint{
            Timestamp: bucket.Timestamp,
            Rate:      rate,
        })
    }

    return results, nil
}

type bucket struct {
    Timestamp time.Time
    Points    []models.MetricPoint
}

// groupIntoBuckets groups points into time intervals
func groupIntoBuckets(points []models.MetricPoint, start, end time.Time, interval time.Duration) []bucket {
    buckets := make([]bucket, 0)

    currentBucket := start
    for currentBucket.Before(end) {
        b := bucket{
            Timestamp: currentBucket,
            Points:    []models.MetricPoint{},
        }

        nextBucket := currentBucket.Add(interval)
        for _, p := range points {
            if !p.Timestamp.Before(currentBucket) && p.Timestamp.Before(nextBucket) {
                b.Points = append(b.Points, p)
            }
        }

        if len(b.Points) > 0 {
            buckets = append(buckets, b)
        }

        currentBucket = nextBucket
    }

    return buckets
}

// calculateBucketRate calculates rate for a bucket
func calculateBucketRate(b bucket) float64 {
    if len(b.Points) < 2 {
        return 0
    }

    // Use first and last point in bucket
    first := b.Points[0]
    last := b.Points[len(b.Points)-1]

    // Calculate change (handle counter resets)
    change := last.Value - first.Value
    if change < 0 {
        // Counter reset, ignore
        return 0
    }

    // Calculate time difference in seconds
    dt := last.Timestamp.Sub(first.Timestamp).Seconds()
    if dt == 0 {
        return 0
    }

    // Rate = change / time
    return change / dt
}
```

#### Implementation Steps

1. Implement `Rate()` method
2. Implement time bucket grouping
3. Implement rate calculation (handle counter resets)
4. Write tests for various scenarios
5. Handle edge cases (single point, no data, counter resets)

#### Tests

```go
func TestRate(t *testing.T) {
    store := setupTestStore(t)
    defer store.Close()

    now := time.Now()
    metrics := []models.Metric{
        {Name: "requests", Timestamp: now, Value: 100, Dimensions: map[string]string{"host": "web-1"}},
        {Name: "requests", Timestamp: now.Add(1 * time.Minute), Value: 160, Dimensions: map[string]string{"host": "web-1"}},
    }
    store.Write(context.Background(), metrics)

    results, err := store.Rate(context.Background(), storage.RateRequest{
        MetricName: "requests",
        Start:      now.Add(-1 * time.Minute),
        End:        now.Add(2 * time.Minute),
        Interval:   1 * time.Minute,
    })

    assert.NoError(t, err)
    assert.Len(t, results, 1)
    assert.InDelta(t, 1.0, results[0].Rate, 0.01) // 60 requests / 60 seconds = 1 req/sec
}

func TestRateCounterReset(t *testing.T) {
    store := setupTestStore(t)
    defer store.Close()

    now := time.Now()
    metrics := []models.Metric{
        {Name: "requests", Timestamp: now, Value: 100, Dimensions: map[string]string{"host": "web-1"}},
        {Name: "requests", Timestamp: now.Add(1 * time.Minute), Value: 10, Dimensions: map[string]string{"host": "web-1"}}, // Reset
    }
    store.Write(context.Background(), metrics)

    results, err := store.Rate(context.Background(), storage.RateRequest{
        MetricName: "requests",
        Start:      now.Add(-1 * time.Minute),
        End:        now.Add(2 * time.Minute),
        Interval:   1 * time.Minute,
    })

    assert.NoError(t, err)
    assert.Len(t, results, 1)
    assert.Equal(t, 0.0, results[0].Rate) // Should ignore negative change
}
```

#### Acceptance Criteria

- [ ] Calculates correct rate (change per second)
- [ ] Groups into time buckets correctly
- [ ] Handles counter resets (ignores negative changes)
- [ ] Returns empty for insufficient data
- [ ] Tests cover edge cases

---

### 7. Discovery Operations

**Effort:** 1 day
**File:** `pkg/storage/badger/discovery.go`

#### Technical Specification

```go
// pkg/storage/badger/discovery.go
package badger

import (
    "context"
    "strings"
    "github.com/dgraph-io/badger/v4"
)

// ListMetrics returns all unique metric names
func (s *Store) ListMetrics(ctx context.Context) ([]string, error) {
    metrics := make(map[string]bool)

    err := s.db.View(func(txn *badger.Txn) error {
        opts := badger.DefaultIteratorOptions
        opts.PrefetchValues = false // Only need keys

        it := txn.NewIterator(opts)
        defer it.Close()

        prefix := []byte("metric:")
        for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
            key := string(it.Item().Key())
            parts := strings.Split(key, ":")
            if len(parts) >= 2 {
                metricName := parts[1]
                metrics[metricName] = true
            }
        }

        return nil
    })

    if err != nil {
        return nil, err
    }

    // Convert to slice
    result := make([]string, 0, len(metrics))
    for name := range metrics {
        result = append(result, name)
    }
    sort.Strings(result)

    return result, nil
}

// ListDimensionKeys returns all dimension keys for a metric
func (s *Store) ListDimensionKeys(ctx context.Context, metricName string) ([]string, error) {
    keys := make(map[string]bool)

    err := s.db.View(func(txn *badger.Txn) error {
        opts := badger.DefaultIteratorOptions
        it := txn.NewIterator(opts)
        defer it.Close()

        prefix := []byte(fmt.Sprintf("metric:%s:", metricName))

        for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
            var dimensions map[string]string
            err := it.Item().Value(func(val []byte) error {
                _, dims, err := DecodeValue(val)
                dimensions = dims
                return err
            })
            if err != nil {
                continue
            }

            for key := range dimensions {
                keys[key] = true
            }
        }

        return nil
    })

    if err != nil {
        return nil, err
    }

    result := make([]string, 0, len(keys))
    for key := range keys {
        result = append(result, key)
    }
    sort.Strings(result)

    return result, nil
}

// ListDimensionValues returns dimension values for autocomplete
func (s *Store) ListDimensionValues(ctx context.Context, metricName, dimensionKey string, limit int) ([]string, error) {
    values := make(map[string]int) // value -> count

    err := s.db.View(func(txn *badger.Txn) error {
        opts := badger.DefaultIteratorOptions
        it := txn.NewIterator(opts)
        defer it.Close()

        prefix := []byte(fmt.Sprintf("metric:%s:", metricName))

        for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
            var dimensions map[string]string
            err := it.Item().Value(func(val []byte) error {
                _, dims, err := DecodeValue(val)
                dimensions = dims
                return err
            })
            if err != nil {
                continue
            }

            if value, exists := dimensions[dimensionKey]; exists {
                values[value]++
            }
        }

        return nil
    })

    if err != nil {
        return nil, err
    }

    // Sort by frequency
    type valueCount struct {
        Value string
        Count int
    }
    sorted := make([]valueCount, 0, len(values))
    for val, count := range values {
        sorted = append(sorted, valueCount{Value: val, Count: count})
    }
    sort.Slice(sorted, func(i, j int) bool {
        return sorted[i].Count > sorted[j].Count
    })

    // Return top N
    result := make([]string, 0, limit)
    for i := 0; i < len(sorted) && i < limit; i++ {
        result = append(result, sorted[i].Value)
    }

    return result, nil
}
```

#### Implementation Steps

1. Implement `ListMetrics()`
2. Implement `ListDimensionKeys()`
3. Implement `ListDimensionValues()` with frequency sorting
4. Optimize for performance (consider caching)
5. Write tests
6. Document API

#### Acceptance Criteria

- [ ] ListMetrics returns all unique metric names
- [ ] ListDimensionKeys returns all dimension keys for a metric
- [ ] ListDimensionValues returns top N values by frequency
- [ ] Results are sorted appropriately
- [ ] Performance acceptable for UI autocomplete
- [ ] Tests validate correctness

---

### 8. Data Retention and Cleanup

**Effort:** 1 day
**File:** `pkg/storage/badger/retention.go`

#### Technical Specification

```go
// pkg/storage/badger/retention.go
package badger

import (
    "context"
    "time"
    "github.com/dgraph-io/badger/v4"
)

// DeleteBefore removes metrics older than the specified time
func (s *Store) DeleteBefore(ctx context.Context, metricName string, before time.Time) error {
    wb := s.db.NewWriteBatch()
    defer wb.Cancel()

    err := s.db.View(func(txn *badger.Txn) error {
        opts := badger.DefaultIteratorOptions
        opts.PrefetchValues = false // Don't need values

        it := txn.NewIterator(opts)
        defer it.Close()

        prefix := []byte(fmt.Sprintf("metric:%s:", metricName))
        endKey := []byte(fmt.Sprintf("metric:%s:%d:", metricName, before.UnixMilli()))

        for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
            key := it.Item().Key()

            // Stop if we've reached keys newer than 'before'
            if bytes.Compare(key, endKey) >= 0 {
                break
            }

            // Mark for deletion
            if err := wb.Delete(key); err != nil {
                return err
            }
        }

        return nil
    })

    if err != nil {
        return err
    }

    // Commit deletions
    return wb.Flush()
}

// RunCleanup runs periodic cleanup based on retention policy
func (s *Store) RunCleanup(ctx context.Context, retention time.Duration) error {
    cutoff := time.Now().Add(-retention)

    // Get all metrics
    metrics, err := s.ListMetrics(ctx)
    if err != nil {
        return err
    }

    // Delete old data for each metric
    for _, metricName := range metrics {
        if err := s.DeleteBefore(ctx, metricName, cutoff); err != nil {
            return fmt.Errorf("failed to cleanup %s: %w", metricName, err)
        }
    }

    return nil
}
```

#### Implementation Steps

1. Implement `DeleteBefore()` method
2. Implement `RunCleanup()` for periodic cleanup
3. Add background goroutine for automatic cleanup
4. Write tests for deletion logic
5. Test with large datasets
6. Benchmark cleanup performance

#### Acceptance Criteria

- [ ] DeleteBefore removes only old metrics
- [ ] RunCleanup processes all metrics
- [ ] Cleanup runs periodically in background
- [ ] Large deletions performant
- [ ] Tests validate correct deletion

---

### 9. Health Checks

**Effort:** 0.5 days
**File:** `pkg/storage/badger/health.go`

#### Technical Specification

```go
// pkg/storage/badger/health.go
package badger

import (
    "context"
    "os"
)

// Health checks if the storage backend is healthy
func (s *Store) Health(ctx context.Context) error {
    // Check if database is accessible
    if s.db == nil {
        return errors.New("database not initialized")
    }

    // Try a simple read operation
    err := s.db.View(func(txn *badger.Txn) error {
        // Just getting an iterator validates DB access
        it := txn.NewIterator(badger.DefaultIteratorOptions)
        defer it.Close()
        return nil
    })

    if err != nil {
        return fmt.Errorf("database unhealthy: %w", err)
    }

    // Check disk space
    diskUsage, err := s.getDiskUsage()
    if err != nil {
        return fmt.Errorf("failed to check disk usage: %w", err)
    }

    if diskUsage > 0.95 { // 95% full
        return fmt.Errorf("disk usage critical: %.1f%%", diskUsage*100)
    }

    return nil
}

// getDiskUsage returns disk usage percentage
func (s *Store) getDiskUsage() (float64, error) {
    // Get directory info
    info, err := os.Stat(s.config.DataDir)
    if err != nil {
        return 0, err
    }

    // This is a simplified check - in production, use syscall.Statfs
    // to get actual filesystem stats
    _ = info

    return 0.5, nil // Placeholder
}
```

#### Acceptance Criteria

- [ ] Health() returns nil when healthy
- [ ] Health() returns error when database inaccessible
- [ ] Health() checks disk space
- [ ] Tests validate health check behavior

---

## Testing Strategy

### Unit Tests
- Each function has dedicated test
- Cover happy path and edge cases
- Use table-driven tests where appropriate
- Mock external dependencies

### Integration Tests
- Test full write → query cycle
- Test with realistic data volumes
- Test concurrent operations
- Test cleanup and retention

### Performance Tests
- Benchmark write throughput (target: 10K+/sec)
- Benchmark query latency (target: <500ms for 10K points)
- Benchmark aggregation performance
- Memory profiling

### Test Data Generators
```go
func generateTestMetrics(count int) []models.Metric {
    metrics := make([]models.Metric, count)
    now := time.Now()

    for i := 0; i < count; i++ {
        metrics[i] = models.Metric{
            Name:      fmt.Sprintf("metric_%d", i%10),
            Timestamp: now.Add(time.Duration(i) * time.Second),
            Value:     float64(rand.Intn(100)),
            Dimensions: map[string]string{
                "customer_id": fmt.Sprintf("cust_%d", rand.Intn(100)),
                "region":      fmt.Sprintf("region_%d", rand.Intn(5)),
            },
        }
    }

    return metrics
}
```

## Acceptance Criteria Summary

### Functional
- [ ] All MetricsStore interface methods implemented
- [ ] All 9 aggregation types work correctly
- [ ] Rate calculations handle counter resets
- [ ] Discovery APIs return correct results
- [ ] Data retention works as configured

### Performance
- [ ] Write throughput: 10,000+ metrics/second
- [ ] Query latency (p95): < 500ms for 10K points
- [ ] Aggregation latency: < 1s for 100K points
- [ ] Memory usage: < 2GB for 1M series

### Quality
- [ ] Unit test coverage: 80%+
- [ ] Integration tests pass
- [ ] Benchmarks meet targets
- [ ] No data races (go test -race passes)
- [ ] Documentation complete

## Dependencies

**Go Packages:**
```go
require (
    github.com/dgraph-io/badger/v4 v4.2.0
)
```

## Rollout Plan

1. **Day 1-2:** Key schema and store initialization
2. **Day 3-4:** Write and QueryRange operations
3. **Day 5-7:** Aggregate operations (all 9 types)
4. **Day 8:** Rate calculations
5. **Day 9:** Discovery operations
6. **Day 10:** Retention and health checks
7. **Day 11:** Integration testing and performance tuning

## Success Metrics

- ✅ Write throughput benchmarks pass
- ✅ Query latency benchmarks pass
- ✅ All unit tests pass with 80%+ coverage
- ✅ Integration tests validate end-to-end flows
- ✅ No memory leaks or goroutine leaks
- ✅ Documentation complete and reviewed

---

**Status:** Ready for Implementation
**Last Updated:** 2024-12-08
