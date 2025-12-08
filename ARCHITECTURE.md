# Luminate - High-Cardinality Observability System

## Project Overview

Luminate is an observability system designed to capture high-dimensionality metrics for enhanced system observability. Unlike Prometheus and Grafana, which are not optimized for capturing customer-level or application-level metrics, Luminate is built from the ground up to handle high-cardinality data.

### Key Differentiators
- **High Cardinality Support**: Track metrics at customer, tenant, and user levels
- **Flexible Dimensions**: Support for arbitrary metadata and custom tags
- **Application-Level Metrics**: Fine-grained observability beyond infrastructure metrics

---

## Storage Backend Analysis

### The High-Cardinality Challenge

Traditional time-series databases like Prometheus struggle with high cardinality because:
- Each unique label combination creates a new series
- Indexes grow exponentially with cardinality
- Query performance degrades beyond ~10M active series
- Not designed for millions of unique customers/tenants

### Storage Options Evaluated

#### 1. ClickHouse ⭐ (Recommended for Production)

**What is ClickHouse?**
- Columnar OLAP database built for analytics workloads
- Originally developed by Yandex, now open-source
- Purpose-built for high-cardinality, high-volume data

**Why ClickHouse for High Cardinality:**

**Columnar Storage Architecture:**
```
Traditional Row Storage:
Row 1: [timestamp, metric, customer_id, value]
Row 2: [timestamp, metric, customer_id, value]
→ Reading one column = read ALL columns
→ Poor compression (mixed data types)

ClickHouse Columnar Storage:
Column timestamp:    [t1, t2, t3, ...]
Column metric:       [cpu, mem, cpu, ...]
Column customer_id:  [c1, c2, c3, ...]
Column value:        [75.5, 80.2, 90.1, ...]
→ Read only needed columns
→ 10-100x compression (same types)
→ Cardinality doesn't kill performance
```

**Pros:**
- Handles billions of rows with millions of unique dimensions
- Sub-second queries on massive datasets
- Excellent compression (10-100x)
- Fast aggregations and filtering
- Native time-series support
- Horizontal scalability
- Mature ecosystem

**Cons:**
- Eventual consistency in distributed mode
- Updates are expensive (insert-optimized)
- Requires careful schema design
- Higher operational complexity

**Real-World Performance:**
- Cloudflare: 100+ billion rows/day
- Uber: 10x compression, 200x faster queries vs Elasticsearch
- Handles millions of customers/tenants naturally

**Example Schema:**
```sql
CREATE TABLE metrics (
    timestamp DateTime64(3),
    metric_name LowCardinality(String),
    customer_id String,
    tenant_id String,
    dimensions Map(String, String),
    value Float64,
    INDEX idx_customer customer_id TYPE bloom_filter GRANULARITY 1,
    INDEX idx_tenant tenant_id TYPE bloom_filter GRANULARITY 1
) ENGINE = MergeTree()
PARTITION BY toYYYYMM(timestamp)
ORDER BY (metric_name, timestamp);
```

**Go Integration:**
```go
import "github.com/ClickHouse/clickhouse-go/v2"

conn, err := clickhouse.Open(&clickhouse.Options{
    Addr: []string{"127.0.0.1:9000"},
})

// Batch insert
batch, _ := conn.PrepareBatch(ctx, "INSERT INTO metrics")
for _, metric := range metrics {
    batch.Append(metric.Timestamp, metric.Name, metric.CustomerID, metric.Value)
}
batch.Send()
```

#### 2. BadgerDB (Recommended for Prototyping)

**What is BadgerDB?**
- Embeddable, persistent key-value store
- Written in pure Go by Dgraph Labs
- LSM tree architecture (like RocksDB)
- Runs in-process (no separate database server)

**Why Consider BadgerDB:**
- **Zero operational overhead**: No database to install/manage
- **Fast iteration**: Just import and start coding
- **Pure Go**: No CGO, easy cross-compilation
- **Simple deployment**: Single binary
- **Good performance**: Millions of writes/second

**The Catch:**
You build everything yourself:
- Time-range queries: Design key schema
- Aggregations: Write iteration code
- Indexing: Manual secondary indexes
- Query patterns: Custom code

**Example:**
```go
import "github.com/dgraph-io/badger/v4"

db, err := badger.Open(badger.DefaultOptions("/tmp/badger"))
defer db.Close()

// Write
db.Update(func(txn *badger.Txn) error {
    return txn.Set([]byte("metric:customer123:cpu"), []byte("75.5"))
})

// Read
db.View(func(txn *badger.Txn) error {
    item, err := txn.Get([]byte("metric:customer123:cpu"))
    // process item...
})
```

#### 3. Apache Druid

**Pros:**
- Real-time ingestion and querying
- Built for high-cardinality dimensions
- Excellent for rollups and pre-aggregation
- Good for streaming data

**Cons:**
- Complex architecture (ZooKeeper, deep storage, etc.)
- Steep learning curve
- Higher operational overhead
- Resource intensive

**Best for:** Real-time analytics dashboards with streaming data

#### 4. PostgreSQL with TimescaleDB

**Pros:**
- Familiar SQL interface
- TimescaleDB adds time-series optimizations
- Compression and retention policies
- Strong consistency

**Cons:**
- Not optimized for extreme cardinality
- Slower than columnar stores for analytics
- Vertical scaling limitations
- Index bloat with high cardinality

**Best for:** Lower cardinality use cases, existing Postgres infrastructure

#### 5. VictoriaMetrics

**Pros:**
- Go-based, easy to operate
- Better cardinality handling than Prometheus
- Single binary deployment
- Prometheus compatible

**Cons:**
- Still has cardinality limits
- Less flexible than true OLAP databases
- Focused on metrics, not general analytics

**Best for:** Prometheus replacement

---

## Storage Decision & Strategy

### Recommended Approach: Storage Abstraction Layer

Luminate uses a **storage abstraction interface** to support two distinct deployment scenarios. You choose **one** backend based on your operational requirements and scale - there is no migration path between them.

**Storage Abstraction:**
```go
type MetricsStore interface {
    Write(ctx context.Context, metrics []Metric) error
    Query(ctx context.Context, query Query) ([]Result, error)
}

// Option 1: BadgerDB implementation
type BadgerStore struct {
    db *badger.DB
}

// Option 2: ClickHouse implementation
type ClickHouseStore struct {
    conn clickhouse.Conn
}
```

### When to Choose Each Backend

| Use Case | BadgerDB | ClickHouse |
|----------|----------|------------|
| **Deployment** | Single binary, embedded | Distributed database cluster |
| **Operations** | Zero (runs in-process) | Moderate (manage ClickHouse) |
| **Best For** | Edge deployments, dev/test, small-scale | Production, high-scale, enterprise |
| **Scale Ceiling** | Single machine, millions of series | Petabytes, billions of series |
| **Query Performance** | Good (in-memory aggregations) | Excellent (columnar, optimized) |
| **Setup Time** | Minutes | Hours to days |
| **Infrastructure Cost** | None (embedded) | Moderate (database cluster) |
| **High Cardinality** | Manual optimization required | Native support, built-in |

### Decision Guide

**Choose BadgerDB if:**
- Running on edge devices or resource-constrained environments
- Need single-binary deployment with zero dependencies
- Development/testing environments
- Scale requirements fit on a single machine (< millions of series)
- Want zero operational complexity
- Self-hosted scenarios where you can't run external databases

**Choose ClickHouse if:**
- Production deployment with high scale requirements
- Need horizontal scalability (billions of series, petabyte scale)
- Have infrastructure team to manage ClickHouse
- Require complex queries and advanced analytics
- Need guaranteed high performance for high-cardinality queries
- Want to leverage existing ClickHouse infrastructure

### Why This Abstraction?

1. **Deployment Flexibility**: Support both embedded and distributed architectures
2. **Zero Lock-in**: Same API regardless of storage backend
3. **Clear Boundaries**: Choose based on deployment needs, not migration timeline
4. **Operational Simplicity**: Pick the right tool for your scale and environment

---

## Query Engine Abstraction Strategy

### The Challenge

BadgerDB and ClickHouse have fundamentally different query capabilities:

| Capability | BadgerDB | ClickHouse |
|-----------|----------|------------|
| **Query Language** | Key iteration (code) | SQL |
| **Filtering** | Manual (you write loops) | WHERE clauses (optimized) |
| **Aggregation** | Manual computation | Built-in (SUM, AVG, etc.) |
| **Time-range** | Key prefix scan | Indexed, partitioned |
| **Join support** | No (manual) | Yes (SQL) |
| **Performance** | Single-threaded iteration | Multi-threaded, columnar |

### Abstraction Approach: Limited Query Set ⭐

Provide a **simple, focused abstraction** that covers the most common query patterns (80% use cases) without building a complex DSL. Keep it minimal and practical:

```go
package storage

// Core abstraction - focused interface for high-cardinality observability
type MetricsStore interface {
    // === Write Operations ===
    Write(ctx context.Context, metrics []Metric) error

    // === Query Operations ===
    // Get raw metric points with simple filters
    QueryRange(ctx context.Context, req QueryRequest) ([]MetricPoint, error)

    // Aggregate metrics (AVG, SUM, COUNT, MIN, MAX, P50, P95, P99, INTEGRAL)
    Aggregate(ctx context.Context, req AggregateRequest) ([]AggregateResult, error)

    // Calculate rate of change for counter metrics (requests/sec, bytes/sec, etc.)
    Rate(ctx context.Context, req RateRequest) ([]RatePoint, error)

    // === Discovery Operations ===
    // List all metric names
    ListMetrics(ctx context.Context) ([]string, error)

    // List dimension keys for a metric
    ListDimensionKeys(ctx context.Context, metricName string) ([]string, error)

    // List dimension values for autocomplete (with limit)
    ListDimensionValues(ctx context.Context, metricName, dimensionKey string, limit int) ([]string, error)

    // === Lifecycle Operations ===
    // Delete metrics older than specified time (data retention)
    DeleteBefore(ctx context.Context, metricName string, before time.Time) error

    // Health check
    Health(ctx context.Context) error
}

// === Data Model ===

type Metric struct {
    Name       string
    Timestamp  time.Time
    Value      float64
    Dimensions map[string]string
}

// Validate metric before writing
func (m *Metric) Validate() error {
    if m.Name == "" {
        return errors.New("metric name required")
    }
    if math.IsNaN(m.Value) || math.IsInf(m.Value, 0) {
        return errors.New("metric value cannot be NaN or Inf")
    }
    // Reject timestamps too far in past (>7 days) or future (>1 hour)
    now := time.Now()
    if m.Timestamp.Before(now.Add(-7*24*time.Hour)) {
        return errors.New("timestamp too old")
    }
    if m.Timestamp.After(now.Add(1*time.Hour)) {
        return errors.New("timestamp in future")
    }
    return nil
}

// === Query Types ===

// Simple range query - get raw metric points
type QueryRequest struct {
    MetricName string
    Start      time.Time
    End        time.Time
    Filters    map[string]string  // customer_id="123", tenant_id="xyz"
    Limit      int                // Max points to return
    Timeout    time.Duration      // Query timeout (optional)
}

type MetricPoint struct {
    Timestamp  time.Time
    Value      float64
    Dimensions map[string]string
}

// Aggregate query - get computed results
type AggregateRequest struct {
    MetricName  string
    Start       time.Time
    End         time.Time
    Filters     map[string]string
    Aggregation AggregationType    // AVG, SUM, COUNT, MIN, MAX, P50, P95, P99, INTEGRAL
    GroupBy     []string           // Dimensions to group by
    Limit       int                // Top N results
    Timeout     time.Duration      // Query timeout (optional)
}

type AggregateResult struct {
    GroupKey   map[string]string  // The grouped dimensions
    Value      float64            // Aggregated value
    Count      int64              // Number of points (useful for AVG)
}

type AggregationType int

const (
    // Basic aggregations
    AVG AggregationType = iota
    SUM
    COUNT
    MIN
    MAX

    // Percentiles (latency analysis, SLO tracking)
    P50  // Median
    P95  // 95th percentile
    P99  // 99th percentile

    // Time-weighted (resource consumption)
    INTEGRAL  // Area under the curve (e.g., CPU-seconds)
)

// Rate query - calculate rate of change for counter metrics
type RateRequest struct {
    MetricName string
    Start      time.Time
    End        time.Time
    Filters    map[string]string
    Interval   time.Duration  // Bucket size (e.g., 1 minute)
    Timeout    time.Duration  // Query timeout (optional)
}

type RatePoint struct {
    Timestamp time.Time
    Rate      float64  // Change per second (ignores negative changes/resets)
}
```

**Key Design Decisions:**

1. **Four query methods**: `QueryRange()` for raw data, `Aggregate()` for statistics, `Rate()` for counters, discovery methods for UIs
2. **Simple filters**: Only equality filters (dimension="value"), no complex WHERE logic
3. **No joins**: Single metric at a time
4. **9 aggregation types**: Covers basic stats, percentiles, and time-weighted calculations
5. **Simple grouping**: Group by dimension names, no complex expressions
6. **Built-in validation**: Protect against bad data (NaN, Inf, invalid timestamps)
7. **Query limits**: Timeouts and result limits prevent expensive queries
8. **Discovery support**: Essential for building UIs and dashboards

This covers the vast majority of observability queries:
- "Show me CPU usage for customer X in the last hour" → `QueryRange()`
- "What's the average API latency per customer?" → `Aggregate(AVG)`
- "Top 10 customers by request count" → `Aggregate(COUNT)`
- "Sum of bytes transferred per tenant" → `Aggregate(SUM)`
- "p95 latency per customer" → `Aggregate(P95)`
- "Request rate over time" → `Rate()`
- "Total CPU-seconds consumed per customer" → `Aggregate(INTEGRAL)`

### Implementation Examples

**BadgerDB Implementation (Simplified):**
```go
func (b *BadgerStore) Aggregate(ctx context.Context, req AggregateRequest) ([]AggregateResult, error) {
    prefix := fmt.Sprintf("metric:%s:", req.MetricName)
    aggregators := make(map[string]*Aggregator)

    err := b.db.View(func(txn *badger.Txn) error {
        it := txn.NewIterator(badger.DefaultIteratorOptions)
        defer it.Close()

        for it.Seek([]byte(prefix)); it.ValidForPrefix([]byte(prefix)); it.Next() {
            item := it.Item()
            metric, _ := decodeMetricFromKey(item.Key())

            // Filter by time range and dimensions
            if !inTimeRange(metric.Timestamp, req.Start, req.End) {
                continue
            }
            if !matchesFilters(metric.Dimensions, req.Filters) {
                continue
            }

            // Get value and aggregate
            var value float64
            item.Value(func(val []byte) error {
                value = decodeFloat64(val)
                return nil
            })

            groupKey := buildGroupKey(metric.Dimensions, req.GroupBy)
            if agg, exists := aggregators[groupKey]; exists {
                agg.Add(value, metric.Timestamp)  // For time-weighted aggregations
            } else {
                aggregators[groupKey] = NewAggregator(req.Aggregation)
                aggregators[groupKey].Add(value, metric.Timestamp)
            }
        }
        return nil
    })

    return buildResults(aggregators, req), err
}
```

**ClickHouse Implementation:**
```go
func (c *ClickHouseStore) Aggregate(ctx context.Context, req AggregateRequest) ([]AggregateResult, error) {
    // Map aggregation type to SQL function
    aggFunc := map[AggregationType]string{
        AVG:      "AVG(value)",
        SUM:      "SUM(value)",
        COUNT:    "COUNT(*)",
        MIN:      "MIN(value)",
        MAX:      "MAX(value)",
        P50:      "quantile(0.50)(value)",
        P95:      "quantile(0.95)(value)",
        P99:      "quantile(0.99)(value)",
        INTEGRAL: "sum(value * (timestamp - lag(timestamp, 1, timestamp) OVER (ORDER BY timestamp)))",
    }[req.Aggregation]

    // Build GROUP BY clause
    groupByCols := ""
    selectCols := ""
    if len(req.GroupBy) > 0 {
        selectCols = strings.Join(req.GroupBy, ", ") + ", "
        groupByCols = "GROUP BY " + strings.Join(req.GroupBy, ", ")
    }

    // Build WHERE filters
    whereFilters := []string{"metric_name = ?", "timestamp BETWEEN ? AND ?"}
    args := []any{req.MetricName, req.Start, req.End}
    for dim, val := range req.Filters {
        whereFilters = append(whereFilters, fmt.Sprintf("dimensions['%s'] = ?", dim))
        args = append(args, val)
    }

    query := fmt.Sprintf(`
        SELECT %s%s as value, COUNT(*) as count
        FROM metrics
        WHERE %s
        %s
        ORDER BY value DESC
        LIMIT ?
    `, selectCols, aggFunc, strings.Join(whereFilters, " AND "), groupByCols)

    args = append(args, req.Limit)
    rows, err := c.conn.Query(ctx, query, args...)
    return parseAggregateResults(rows, req.GroupBy), err
}
```

### Usage Examples

**1. Get raw metric points:**
```go
// Get CPU usage for a specific customer
points, err := store.QueryRange(ctx, QueryRequest{
    MetricName: "cpu_usage",
    Start:      time.Now().Add(-1 * time.Hour),
    End:        time.Now(),
    Filters:    map[string]string{"customer_id": "cust_123"},
    Limit:      1000,
    Timeout:    10 * time.Second,
})
// Returns: []MetricPoint with timestamp, value, dimensions
```

**2. Basic aggregations:**
```go
// Average API latency per customer
results, err := store.Aggregate(ctx, AggregateRequest{
    MetricName:  "api_latency",
    Start:       time.Now().Add(-24 * time.Hour),
    End:         time.Now(),
    Aggregation: AVG,
    GroupBy:     []string{"customer_id"},
    Limit:       100,
})

// Top 10 customers by request count
results, err := store.Aggregate(ctx, AggregateRequest{
    MetricName:  "api_requests",
    Start:       time.Now().Add(-1 * time.Hour),
    End:         time.Now(),
    Aggregation: COUNT,
    GroupBy:     []string{"customer_id"},
    Limit:       10,
})

// Total bytes transferred per tenant (filtered by region)
results, err := store.Aggregate(ctx, AggregateRequest{
    MetricName:  "bytes_transferred",
    Start:       startOfDay,
    End:         endOfDay,
    Filters:     map[string]string{"region": "us-west-2"},
    Aggregation: SUM,
    GroupBy:     []string{"tenant_id"},
})
```

**3. Percentiles (latency analysis):**
```go
// p95 latency per customer (SLO tracking)
results, err := store.Aggregate(ctx, AggregateRequest{
    MetricName:  "api_latency",
    Start:       time.Now().Add(-1 * time.Hour),
    End:         time.Now(),
    Aggregation: P95,
    GroupBy:     []string{"customer_id"},
    Limit:       100,
})

// p99 latency for high-priority customers
results, err := store.Aggregate(ctx, AggregateRequest{
    MetricName:  "api_latency",
    Start:       time.Now().Add(-24 * time.Hour),
    End:         time.Now(),
    Filters:     map[string]string{"tier": "enterprise"},
    Aggregation: P99,
    GroupBy:     []string{"customer_id"},
})
```

**4. Rate calculations (counter metrics):**
```go
// Request rate per minute for a customer
rates, err := store.Rate(ctx, RateRequest{
    MetricName: "total_requests",  // Counter metric
    Start:      time.Now().Add(-1 * time.Hour),
    End:        time.Now(),
    Filters:    map[string]string{"customer_id": "cust_123"},
    Interval:   time.Minute,
})
// Returns: []RatePoint showing requests/second for each minute

// Bytes/sec over time
rates, err := store.Rate(ctx, RateRequest{
    MetricName: "total_bytes",
    Start:      time.Now().Add(-1 * time.Hour),
    End:        time.Now(),
    Interval:   5 * time.Minute,
})
```

**5. Time-weighted aggregations (resource consumption):**
```go
// Total CPU-seconds consumed per customer
results, err := store.Aggregate(ctx, AggregateRequest{
    MetricName:  "cpu_percent",
    Start:       startOfDay,
    End:         endOfDay,
    Aggregation: INTEGRAL,
    GroupBy:     []string{"customer_id"},
})
// Result value is in %-seconds, divide by 100 for CPU-seconds
```

**6. Discovery (for UIs):**
```go
// List all metrics
metrics, err := store.ListMetrics(ctx)
// Returns: ["cpu_usage", "api_latency", "bytes_transferred", ...]

// Get dimensions for a metric
dimensions, err := store.ListDimensionKeys(ctx, "api_latency")
// Returns: ["customer_id", "endpoint", "status_code", ...]

// Autocomplete dimension values
values, err := store.ListDimensionValues(ctx, "api_latency", "customer_id", 100)
// Returns: Top 100 customer IDs by frequency
```

**7. Data retention:**
```go
// Delete metrics older than 7 days
err := store.DeleteBefore(ctx, "api_latency", time.Now().Add(-7*24*time.Hour))
```

### What This Abstraction Supports

✅ **Fully Supported:**
- Time-range queries with simple equality filters
- Basic aggregations: AVG, SUM, COUNT, MIN, MAX
- Percentiles: P50, P95, P99 (latency tracking, SLO monitoring)
- Time-weighted aggregations: INTEGRAL (resource consumption)
- Rate calculations: NonNegativeDerivative (requests/sec, bytes/sec)
- Group by dimensions (high-cardinality support)
- Top N queries
- Metric discovery (list metrics, dimensions, values)
- Data retention/cleanup
- Query timeouts and limits

⚠️ **Not Covered (use backend-specific code when needed):**
- Multiple aggregations in one query
- Complex WHERE clauses (OR, NOT, ranges, regex)
- Joins across metrics
- Custom percentiles beyond P50/P95/P99
- Histograms
- Custom window functions
- Subqueries

For advanced use cases, access the native backend directly (BadgerDB or ClickHouse connection).

### Benefits of This Approach

1. **Comprehensive Coverage**: Covers 90%+ of observability use cases without building a DSL
2. **Simple & Focused**: Clear, purpose-built methods for each query type
3. **Easy to Implement**: Both BadgerDB and ClickHouse can reasonably implement this interface
4. **No DSL Complexity**: No query language to parse or translate
5. **Production-Ready**: Includes timeouts, limits, validation, discovery, and lifecycle management
6. **Testable**: Simple to mock for unit tests
7. **Clear Limitations**: Users know what's supported vs. what needs custom code
8. **Backend Flexibility**: Same API works on both storage backends

### Implementation Considerations by Backend

**BadgerDB Implementation:**
- Implement core interface: `Write()`, `QueryRange()`, `Aggregate()`, `Rate()`
- Implement discovery: `ListMetrics()`, `ListDimensionKeys()`, `ListDimensionValues()`
- Add lifecycle: `DeleteBefore()`, `Health()`
- Use in-memory aggregations (accept slower performance for complex queries)
- Design efficient key schema for time-range scans
- Focus on correctness and simplicity
- Optimize for write throughput

**ClickHouse Implementation:**
- Implement same interface using SQL queries
- Leverage ClickHouse features: native percentiles (`quantile()`), built-in aggregations
- Use materialized views for common query patterns
- Implement efficient partitioning by time
- Set up retention policies using TTL
- Optimize indexes for high-cardinality dimensions (bloom filters)
- Consider read replicas for query performance

---

## Proposed Architecture

```
┌─────────────────────────────────────────────────────────┐
│                     Clients                             │
│  (SDKs, Dashboards, Grafana, Prometheus Remote Write)  │
└─────────────────────────────────────────────────────────┘
                         ↓
┌─────────────────────────────────────────────────────────┐
│                  Ingestion API                          │
│              (HTTP/gRPC endpoints)                      │
│  - /api/v1/write (batch metrics)                       │
│  - Validation (timestamps, NaN, Inf)                   │
│  - Cardinality limits                                  │
└─────────────────────────────────────────────────────────┘
                         ↓
┌─────────────────────────────────────────────────────────┐
│              Buffer/Queue Layer                         │
│  - In-memory batching (collect 1000s of metrics)       │
│  - Background flush (every 10s or when full)           │
└─────────────────────────────────────────────────────────┘
                         ↓
┌─────────────────────────────────────────────────────────┐
│              MetricsStore Interface                     │
│                                                         │
│  Write Operations:                                     │
│    • Write(metrics []Metric)                           │
│                                                         │
│  Query Operations:                                     │
│    • QueryRange() - raw points                         │
│    • Aggregate() - stats (AVG, P95, INTEGRAL, etc.)    │
│    • Rate() - derivatives (req/sec, bytes/sec)         │
│                                                         │
│  Discovery:                                            │
│    • ListMetrics(), ListDimensionKeys/Values()         │
│                                                         │
│  Lifecycle:                                            │
│    • DeleteBefore() - retention                        │
│    • Health() - status                                 │
└─────────────────────────────────────────────────────────┘
                         ↓
        ┌────────────────┴────────────────┐
        ↓                                 ↓
┌──────────────────┐          ┌──────────────────────┐
│  BadgerDB Store  │          │  ClickHouse Store    │
│   (Phase 1)      │          │    (Phase 2)         │
├──────────────────┤          ├──────────────────────┤
│ • Embedded DB    │          │ • Columnar storage   │
│ • Key-value      │          │ • Native SQL         │
│ • In-memory agg  │          │ • Quantile funcs     │
│ • Single machine │          │ • Partitioning       │
│ • Fast dev       │          │ • Horizontal scale   │
└──────────────────┘          └──────────────────────┘
        ↓                                 ↓
┌─────────────────────────────────────────────────────────┐
│                    Query API                            │
│              (HTTP/gRPC endpoints)                      │
│  - POST /api/v1/query (unified: range, aggregate, rate)│
│  - GET  /api/v1/metrics (discovery)                    │
│  - POST /api/v1/write (ingestion)                      │
│  - GET  /api/v1/health (status)                        │
└─────────────────────────────────────────────────────────┘
                         ↓
┌─────────────────────────────────────────────────────────┐
│                  Query Clients                          │
│  (Dashboards, Grafana, Custom UIs, Alerts)             │
└─────────────────────────────────────────────────────────┘
```

---

## JSON Query Language for Dashboards

For building dashboards and panels, we need a JSON-based query language that maps to the MetricsStore interface. This allows UIs to construct queries dynamically without writing Go code.

### Query Format

```json
{
  "queryType": "aggregate|range|rate",
  "metricName": "api_latency",
  "timeRange": {
    "start": "2024-01-01T00:00:00Z",
    "end": "2024-01-01T23:59:59Z",
    "relative": "1h|24h|7d|30d"  // Alternative to absolute start/end
  },
  "filters": {
    "customer_id": "cust_123",
    "region": "us-west-2"
  },
  "aggregation": "avg|sum|count|min|max|p50|p95|p99|integral",
  "groupBy": ["customer_id", "endpoint"],
  "interval": "1m|5m|1h",  // For rate queries
  "limit": 100,
  "timeout": "30s"
}
```

### Query Examples

**1. Range Query (Raw Data Points)**
```json
{
  "queryType": "range",
  "metricName": "cpu_usage",
  "timeRange": {
    "relative": "1h"
  },
  "filters": {
    "customer_id": "cust_123"
  },
  "limit": 1000
}
```

**Maps to Go:**
```go
store.QueryRange(ctx, QueryRequest{
    MetricName: "cpu_usage",
    Start:      time.Now().Add(-1 * time.Hour),
    End:        time.Now(),
    Filters:    map[string]string{"customer_id": "cust_123"},
    Limit:      1000,
})
```

**2. Aggregate Query (Average Latency)**
```json
{
  "queryType": "aggregate",
  "metricName": "api_latency",
  "timeRange": {
    "relative": "24h"
  },
  "aggregation": "avg",
  "groupBy": ["customer_id"],
  "limit": 100
}
```

**Maps to Go:**
```go
store.Aggregate(ctx, AggregateRequest{
    MetricName:  "api_latency",
    Start:       time.Now().Add(-24 * time.Hour),
    End:         time.Now(),
    Aggregation: AVG,
    GroupBy:     []string{"customer_id"},
    Limit:       100,
})
```

**3. Percentile Query (p95 Latency)**
```json
{
  "queryType": "aggregate",
  "metricName": "api_latency",
  "timeRange": {
    "relative": "1h"
  },
  "aggregation": "p95",
  "groupBy": ["endpoint"],
  "filters": {
    "tier": "enterprise"
  },
  "limit": 50
}
```

**4. Rate Query (Requests/Second)**
```json
{
  "queryType": "rate",
  "metricName": "total_requests",
  "timeRange": {
    "relative": "1h"
  },
  "interval": "1m",
  "filters": {
    "customer_id": "cust_123"
  }
}
```

**Maps to Go:**
```go
store.Rate(ctx, RateRequest{
    MetricName: "total_requests",
    Start:      time.Now().Add(-1 * time.Hour),
    End:        time.Now(),
    Interval:   time.Minute,
    Filters:    map[string]string{"customer_id": "cust_123"},
})
```

### Dashboard Panel Configuration

A complete dashboard panel includes query + display settings:

```json
{
  "panel": {
    "id": "panel_1",
    "title": "API Latency p95 by Customer",
    "type": "timeseries|table|gauge|bar",
    "query": {
      "queryType": "aggregate",
      "metricName": "api_latency",
      "timeRange": {
        "relative": "24h"
      },
      "aggregation": "p95",
      "groupBy": ["customer_id"],
      "limit": 20
    },
    "display": {
      "unit": "ms",
      "decimals": 2,
      "threshold": {
        "warning": 200,
        "critical": 500
      },
      "legend": {
        "show": true,
        "position": "bottom"
      }
    }
  }
}
```

### Dashboard Definition

A complete dashboard with multiple panels:

```json
{
  "dashboard": {
    "id": "customer_overview",
    "title": "Customer Performance Overview",
    "tags": ["customer", "performance"],
    "timeRange": {
      "relative": "24h"
    },
    "refreshInterval": "30s",
    "panels": [
      {
        "id": "panel_1",
        "title": "Request Rate",
        "type": "timeseries",
        "gridPosition": {"x": 0, "y": 0, "w": 12, "h": 4},
        "query": {
          "queryType": "rate",
          "metricName": "total_requests",
          "timeRange": {"relative": "1h"},
          "interval": "1m",
          "groupBy": ["customer_id"]
        }
      },
      {
        "id": "panel_2",
        "title": "p95 Latency",
        "type": "timeseries",
        "gridPosition": {"x": 12, "y": 0, "w": 12, "h": 4},
        "query": {
          "queryType": "aggregate",
          "metricName": "api_latency",
          "aggregation": "p95",
          "groupBy": ["customer_id"],
          "limit": 10
        }
      },
      {
        "id": "panel_3",
        "title": "Top Customers by Request Count",
        "type": "table",
        "gridPosition": {"x": 0, "y": 4, "w": 24, "h": 4},
        "query": {
          "queryType": "aggregate",
          "metricName": "api_requests",
          "aggregation": "count",
          "groupBy": ["customer_id"],
          "limit": 20
        }
      }
    ],
    "variables": [
      {
        "name": "customer_id",
        "type": "query",
        "query": {
          "queryType": "listDimensionValues",
          "metricName": "api_requests",
          "dimension": "customer_id",
          "limit": 100
        }
      }
    ]
  }
}
```

### Template Variables

Support dynamic filtering in dashboards:

```json
{
  "variables": [
    {
      "name": "customer_id",
      "label": "Customer",
      "type": "query",
      "query": {
        "queryType": "listDimensionValues",
        "metricName": "api_latency",
        "dimension": "customer_id",
        "limit": 100
      },
      "multi": false
    },
    {
      "name": "region",
      "label": "Region",
      "type": "custom",
      "options": ["us-west-2", "us-east-1", "eu-west-1"],
      "multi": true,
      "default": "us-west-2"
    }
  ]
}
```

Use variables in queries:
```json
{
  "query": {
    "queryType": "aggregate",
    "metricName": "api_latency",
    "filters": {
      "customer_id": "$customer_id",
      "region": "$region"
    },
    "aggregation": "p95"
  }
}
```

### API Endpoints

**Simplified Single-Endpoint Design:**

```
# Core Query Endpoint (Single unified endpoint)
POST /api/v1/query
Content-Type: application/json

{
  "queryType": "aggregate",
  "metricName": "api_latency",
  "timeRange": {"relative": "1h"},
  "aggregation": "p95",
  "groupBy": ["customer_id"]
}

Response:
{
  "queryType": "aggregate",
  "results": [
    {
      "groupKey": {"customer_id": "cust_123"},
      "value": 145.5,
      "count": 1234
    },
    {
      "groupKey": {"customer_id": "cust_456"},
      "value": 98.2,
      "count": 5678
    }
  ],
  "executionTimeMs": 23
}
```

**Complete API Surface:**
```
# Queries (unified endpoint handles range, aggregate, rate)
POST /api/v1/query                                      - Execute any query type

# Discovery
GET  /api/v1/metrics                                    - List all metrics
GET  /api/v1/metrics/{name}/dimensions                  - List dimensions
GET  /api/v1/metrics/{name}/dimensions/{key}/values     - List dimension values

# Write
POST /api/v1/write                                      - Ingest metrics (batch)

# Management
POST /api/v1/admin/retention                            - Configure retention
DELETE /api/v1/admin/metrics/{name}?before={timestamp}  - Delete old data
GET  /api/v1/health                                     - Health check
```

**Why Single Endpoint?**
1. **Simpler API**: One endpoint to document, version, and secure
2. **Cleaner Client Code**: One function call for all query types
3. **Easier to Extend**: Add new query types without new routes
4. **Consistent Error Handling**: Same HTTP codes, same error format
5. **Already Designed**: JSON format has `queryType` discriminator

**Discovery Examples:**
```bash
# List all metrics
GET /api/v1/metrics
Response: ["cpu_usage", "api_latency", "bytes_transferred"]

# Get dimensions for a metric
GET /api/v1/metrics/api_latency/dimensions
Response: ["customer_id", "endpoint", "status_code"]

# Autocomplete dimension values
GET /api/v1/metrics/api_latency/dimensions/customer_id/values?limit=100
Response: ["cust_123", "cust_456", "cust_789"]
```

### Go Implementation

```go
package api

type JSONQuery struct {
    QueryType  string            `json:"queryType"`  // "aggregate", "range", "rate"
    MetricName string            `json:"metricName"`
    TimeRange  TimeRangeSpec     `json:"timeRange"`
    Filters    map[string]string `json:"filters"`
    Aggregation string           `json:"aggregation,omitempty"`  // For aggregate queries
    GroupBy    []string          `json:"groupBy,omitempty"`
    Interval   string            `json:"interval,omitempty"`     // For rate queries
    Limit      int               `json:"limit,omitempty"`
    Timeout    string            `json:"timeout,omitempty"`
}

type TimeRangeSpec struct {
    Start    *time.Time `json:"start,omitempty"`
    End      *time.Time `json:"end,omitempty"`
    Relative string     `json:"relative,omitempty"`  // "1h", "24h", "7d", "30d"
}

// Parse relative time ranges
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
        return *t.Start, *t.End, nil
    }

    return time.Time{}, time.Time{}, errors.New("invalid time range")
}

func parseRelativeTime(relative string) (time.Duration, error) {
    // "1h" -> 1 hour, "24h" -> 24 hours, "7d" -> 7 days, "30d" -> 30 days
    switch relative {
    case "1h":
        return 1 * time.Hour, nil
    case "24h":
        return 24 * time.Hour, nil
    case "7d":
        return 7 * 24 * time.Hour, nil
    case "30d":
        return 30 * 24 * time.Hour, nil
    default:
        return time.ParseDuration(relative)
    }
}

// Convert JSON query to storage query
func (jq *JSONQuery) ToStorageQuery(ctx context.Context) (interface{}, error) {
    start, end, err := jq.TimeRange.GetAbsoluteRange()
    if err != nil {
        return nil, err
    }

    timeout, _ := time.ParseDuration(jq.Timeout)

    switch jq.QueryType {
    case "range":
        return storage.QueryRequest{
            MetricName: jq.MetricName,
            Start:      start,
            End:        end,
            Filters:    jq.Filters,
            Limit:      jq.Limit,
            Timeout:    timeout,
        }, nil

    case "aggregate":
        aggType := parseAggregationType(jq.Aggregation)
        return storage.AggregateRequest{
            MetricName:  jq.MetricName,
            Start:       start,
            End:         end,
            Filters:     jq.Filters,
            Aggregation: aggType,
            GroupBy:     jq.GroupBy,
            Limit:       jq.Limit,
            Timeout:     timeout,
        }, nil

    case "rate":
        interval, _ := time.ParseDuration(jq.Interval)
        return storage.RateRequest{
            MetricName: jq.MetricName,
            Start:      start,
            End:        end,
            Filters:    jq.Filters,
            Interval:   interval,
            Timeout:    timeout,
        }, nil

    default:
        return nil, fmt.Errorf("unknown query type: %s", jq.QueryType)
    }
}

// HTTP handler
func (h *Handler) ExecuteQuery(w http.ResponseWriter, r *http.Request) {
    var jq JSONQuery
    if err := json.NewDecoder(r.Body).Decode(&jq); err != nil {
        http.Error(w, err.Error(), http.StatusBadRequest)
        return
    }

    storageQuery, err := jq.ToStorageQuery(r.Context())
    if err != nil {
        http.Error(w, err.Error(), http.StatusBadRequest)
        return
    }

    // Execute based on query type
    var result interface{}
    switch q := storageQuery.(type) {
    case storage.QueryRequest:
        result, err = h.store.QueryRange(r.Context(), q)
    case storage.AggregateRequest:
        result, err = h.store.Aggregate(r.Context(), q)
    case storage.RateRequest:
        result, err = h.store.Rate(r.Context(), q)
    }

    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }

    json.NewEncoder(w).Encode(result)
}
```

### Simplified API Client

With a single endpoint, client code is much cleaner:

```typescript
// TypeScript/JavaScript client
class LuminateClient {
  private baseURL: string;

  async query(query: JSONQuery): Promise<QueryResponse> {
    // Single endpoint for all query types
    const response = await fetch(`${this.baseURL}/api/v1/query`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(query)
    });
    return response.json();
  }

  // Discovery methods
  async listMetrics(): Promise<string[]> {
    const response = await fetch(`${this.baseURL}/api/v1/metrics`);
    return response.json();
  }

  async listDimensions(metricName: string): Promise<string[]> {
    const response = await fetch(`${this.baseURL}/api/v1/metrics/${metricName}/dimensions`);
    return response.json();
  }
}

// Usage - same function for all query types
const client = new LuminateClient('http://localhost:8080');

// Range query
const points = await client.query({
  queryType: 'range',
  metricName: 'cpu_usage',
  timeRange: { relative: '1h' }
});

// Aggregate query
const avgLatency = await client.query({
  queryType: 'aggregate',
  metricName: 'api_latency',
  timeRange: { relative: '24h' },
  aggregation: 'avg',
  groupBy: ['customer_id']
});

// Rate query
const requestRate = await client.query({
  queryType: 'rate',
  metricName: 'total_requests',
  timeRange: { relative: '1h' },
  interval: '1m'
});
```

### Benefits

1. **Single Endpoint Simplicity**: One URL for all query operations
2. **Frontend Flexibility**: UIs can construct queries without backend changes
3. **Dashboard Portability**: Dashboards defined as JSON can be exported/imported
4. **Template Variables**: Dynamic filtering for interactive dashboards
5. **Type Safety**: JSON schema validation on frontend and backend
6. **Easy Testing**: Mock queries with JSON files
7. **Version Control**: Dashboards can be checked into Git
8. **Cleaner Client Code**: No need to route to different endpoints based on query type

---

## Security & Authentication

### JWT-Based Authentication

All API endpoints (except `/api/v1/health`) require JWT authentication.

**JWT Token Format:**
```json
{
  "sub": "org_abc123",           // Organization ID
  "tenant_id": "tenant_xyz",     // Tenant ID (for multi-tenancy)
  "scopes": ["read", "write"],   // Permissions
  "exp": 1735689600,             // Expiration timestamp
  "iat": 1735603200              // Issued at timestamp
}
```

**Authentication Flow:**

1. Client obtains JWT token from auth service (out of scope for Luminate)
2. Client includes token in `Authorization` header:
   ```
   Authorization: Bearer <jwt-token>
   ```
3. Luminate validates JWT signature using shared secret or public key
4. Luminate extracts `tenant_id` from token for data isolation

**Token Validation:**
```go
package auth

import (
    "github.com/golang-jwt/jwt/v5"
)

type LuminateClaims struct {
    TenantID string   `json:"tenant_id"`
    Scopes   []string `json:"scopes"`
    jwt.RegisteredClaims
}

func ValidateToken(tokenString string, secretKey []byte) (*LuminateClaims, error) {
    token, err := jwt.ParseWithClaims(tokenString, &LuminateClaims{}, func(token *jwt.Token) (interface{}, error) {
        return secretKey, nil
    })

    if err != nil || !token.Valid {
        return nil, err
    }

    claims, ok := token.Claims.(*LuminateClaims)
    if !ok {
        return nil, errors.New("invalid claims")
    }

    return claims, nil
}
```

**Authorization Rules:**

1. **Data Isolation**: All queries automatically filter by `tenant_id` from JWT token
2. **Write Permissions**: Require `write` scope for `POST /api/v1/write`
3. **Read Permissions**: Require `read` scope for `POST /api/v1/query`
4. **Admin Permissions**: Require `admin` scope for `/api/v1/admin/*` endpoints

**Configuration:**
```yaml
auth:
  enabled: true
  jwt_secret: "your-secret-key"  # For HMAC signing
  # OR
  jwt_public_key_path: "/path/to/public.pem"  # For RSA/ECDSA
  token_expiry: "24h"
```

**Multi-Tenancy:**
- Tenant ID from JWT is automatically added to all metric dimensions: `_tenant_id: "tenant_xyz"`
- Queries are automatically scoped to tenant's data
- Prevents cross-tenant data access

---

## Rate Limiting & Quotas

Protect against abusive queries and ingestion with reasonable defaults.

### Rate Limits

**Per-Tenant Limits (Default):**
```yaml
rate_limits:
  write_requests_per_second: 100      # Max write API calls/sec
  query_requests_per_second: 50       # Max query API calls/sec
  metrics_per_second: 10000           # Max individual metrics ingested/sec
  concurrent_queries: 10              # Max parallel queries per tenant
```

**Implementation:**
- Use token bucket algorithm for smooth rate limiting
- Return `429 Too Many Requests` when limit exceeded
- Include rate limit headers in responses:
  ```
  X-RateLimit-Limit: 100
  X-RateLimit-Remaining: 45
  X-RateLimit-Reset: 1735603260
  ```

### Cardinality Limits

**Default Cardinality Constraints:**
```yaml
cardinality_limits:
  max_dimensions_per_metric: 20       # Max dimension keys per metric
  max_dimension_key_length: 128       # Max chars in dimension key
  max_dimension_value_length: 256     # Max chars in dimension value
  max_unique_series_per_metric: 1000000  # Max unique dimension combinations
  max_metrics_per_tenant: 1000        # Max distinct metric names
```

**Enforcement:**
- Validate on write, reject metrics exceeding limits with `400 Bad Request`
- Track cardinality in memory (approximate using HyperLogLog)
- Alert operators when tenants approach 80% of limits

### Query Limits

**Default Query Constraints:**
```yaml
query_limits:
  max_query_timeout: "30s"            # Absolute max query duration
  default_query_timeout: "10s"        # Default if not specified
  max_query_time_range: "90d"         # Max time span in single query
  max_result_points: 10000            # Max data points returned
  max_group_by_cardinality: 1000      # Max unique groups in GROUP BY
```

---

## Data Model & Validation

### Metric Structure

```go
type Metric struct {
    Name       string            `json:"name"`       // Required
    Timestamp  time.Time         `json:"timestamp"`  // Required (Unix milliseconds)
    Value      float64           `json:"value"`      // Required
    Dimensions map[string]string `json:"dimensions"` // Optional, max 20 keys
}
```

### Validation Rules

**Metric Name:**
- Pattern: `^[a-zA-Z_][a-zA-Z0-9_]*$` (alphanumeric + underscores, must start with letter/underscore)
- Length: 1-256 characters
- Examples: `api_latency`, `cpu_usage`, `bytes_transferred`

**Dimension Keys:**
- Pattern: `^[a-zA-Z_][a-zA-Z0-9_]*$`
- Length: 1-128 characters
- Reserved keys: `_tenant_id` (auto-added from JWT)
- Examples: `customer_id`, `region`, `endpoint`

**Dimension Values:**
- Length: 1-256 characters
- UTF-8 encoded
- No control characters

**Timestamp:**
- Precision: Milliseconds (Unix timestamp)
- Must be within range: `[now - 7 days, now + 1 hour]`
- Reject timestamps too far in past (late data) or future (clock skew)

**Value:**
- Must be finite (not NaN or Inf)
- Float64 precision

**Example Valid Metric:**
```json
{
  "name": "api_latency",
  "timestamp": 1735603200000,
  "value": 145.5,
  "dimensions": {
    "customer_id": "cust_123",
    "endpoint": "/api/users",
    "status_code": "200",
    "region": "us-west-2"
  }
}
```

---

## Batch Write API

### Endpoint Format

```
POST /api/v1/write
Content-Type: application/json
Authorization: Bearer <jwt-token>

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
        "customer_id": "cust_123",
        "host": "web-01"
      }
    }
  ]
}
```

### Batch Constraints

```yaml
batch_write:
  max_batch_size: 10000               # Max metrics per batch
  max_request_size_bytes: 10485760    # 10 MB max request body
  compression: gzip                   # Support gzip compression
```

**Compression Support:**
- Client can send `Content-Encoding: gzip` header
- Server decompresses automatically
- Reduces bandwidth by ~10x for typical metric data

### Response Format

**Success (200 OK):**
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

**Complete Failure (400 Bad Request):**
```json
{
  "error": "batch size exceeds limit",
  "max_batch_size": 10000,
  "received": 15000
}
```

---

## Error Handling

### Standardized Error Response

All errors return consistent JSON format:

```json
{
  "error": {
    "code": "INVALID_METRIC_NAME",
    "message": "Metric name must match pattern ^[a-zA-Z_][a-zA-Z0-9_]*$",
    "details": {
      "metric_name": "123-invalid",
      "pattern": "^[a-zA-Z_][a-zA-Z0-9_]*$"
    }
  }
}
```

### HTTP Status Codes

| Status | Usage |
|--------|-------|
| 200 OK | Successful query or write |
| 207 Multi-Status | Partial batch write success |
| 400 Bad Request | Invalid metric format, validation failure |
| 401 Unauthorized | Missing or invalid JWT token |
| 403 Forbidden | Valid token but insufficient permissions |
| 429 Too Many Requests | Rate limit exceeded |
| 500 Internal Server Error | Server-side error |
| 503 Service Unavailable | Storage backend unavailable |
| 504 Gateway Timeout | Query exceeded timeout |

### Error Codes

```go
const (
    ErrInvalidMetricName      = "INVALID_METRIC_NAME"
    ErrInvalidTimestamp       = "INVALID_TIMESTAMP"
    ErrInvalidValue           = "INVALID_VALUE"
    ErrDimensionLimitExceeded = "DIMENSION_LIMIT_EXCEEDED"
    ErrCardinalityExceeded    = "CARDINALITY_EXCEEDED"
    ErrRateLimitExceeded      = "RATE_LIMIT_EXCEEDED"
    ErrQueryTimeout           = "QUERY_TIMEOUT"
    ErrInvalidQuery           = "INVALID_QUERY"
    ErrUnauthorized           = "UNAUTHORIZED"
    ErrForbidden              = "FORBIDDEN"
)
```

---

## Configuration

### Configuration File Format

Use YAML for configuration with environment variable overrides:

```yaml
# config.yaml
server:
  host: "0.0.0.0"
  port: 8080
  read_timeout: "30s"
  write_timeout: "30s"
  shutdown_timeout: "10s"

auth:
  enabled: true
  jwt_secret: "${JWT_SECRET}"  # From environment variable
  token_expiry: "24h"

storage:
  backend: "badger"  # or "clickhouse"

  # BadgerDB configuration
  badger:
    data_dir: "/var/lib/luminate/data"
    value_log_max_entries: 1000000

  # ClickHouse configuration
  clickhouse:
    addresses: ["localhost:9000"]
    database: "luminate"
    username: "${CLICKHOUSE_USER}"
    password: "${CLICKHOUSE_PASSWORD}"
    max_open_conns: 10
    max_idle_conns: 5
    conn_max_lifetime: "1h"

rate_limits:
  write_requests_per_second: 100
  query_requests_per_second: 50
  metrics_per_second: 10000
  concurrent_queries: 10

cardinality_limits:
  max_dimensions_per_metric: 20
  max_dimension_key_length: 128
  max_dimension_value_length: 256
  max_unique_series_per_metric: 1000000
  max_metrics_per_tenant: 1000

query_limits:
  max_query_timeout: "30s"
  default_query_timeout: "10s"
  max_query_time_range: "90d"
  max_result_points: 10000
  max_group_by_cardinality: 1000

batch_write:
  max_batch_size: 10000
  max_request_size_bytes: 10485760  # 10 MB
  compression: true

retention:
  default_retention: "30d"          # Default retention period
  cleanup_interval: "1h"            # How often to run cleanup

logging:
  level: "info"  # debug, info, warn, error
  format: "json" # json or text
  output: "stdout"

metrics:
  enabled: true  # Expose Luminate's own metrics
  path: "/metrics"
  interval: "10s"
```

### Environment Variables

Override any config value with environment variables:

```bash
LUMINATE_SERVER_PORT=9090
LUMINATE_AUTH_JWT_SECRET=secret-key
LUMINATE_STORAGE_BACKEND=clickhouse
LUMINATE_CLICKHOUSE_ADDRESSES=host1:9000,host2:9000
```

---

## Observability (Meta-Metrics)

Luminate monitors itself and exposes metrics about its own operation.

### Self-Monitoring Metrics

**Ingestion Metrics:**
- `luminate_write_requests_total` - Total write requests received
- `luminate_metrics_ingested_total` - Total individual metrics written
- `luminate_write_errors_total` - Write failures by error type
- `luminate_write_latency_seconds` - Write request latency histogram

**Query Metrics:**
- `luminate_query_requests_total` - Total queries by type (range, aggregate, rate)
- `luminate_query_errors_total` - Query failures by error type
- `luminate_query_latency_seconds` - Query latency histogram
- `luminate_query_result_points` - Number of data points returned

**Storage Metrics:**
- `luminate_storage_size_bytes` - Total storage used
- `luminate_series_count` - Approximate unique series count
- `luminate_cardinality_by_metric` - Series count per metric name

**Rate Limiting:**
- `luminate_rate_limit_exceeded_total` - Rate limit hits by tenant
- `luminate_active_queries` - Current concurrent queries

**Health:**
- `luminate_up` - Health status (1=healthy, 0=unhealthy)
- `luminate_storage_health` - Storage backend health

### Prometheus Exposition

Expose metrics in Prometheus format at `/metrics`:

```
GET /metrics

# HELP luminate_write_requests_total Total write requests received
# TYPE luminate_write_requests_total counter
luminate_write_requests_total{tenant_id="tenant_xyz",status="success"} 12543

# HELP luminate_query_latency_seconds Query latency histogram
# TYPE luminate_query_latency_seconds histogram
luminate_query_latency_seconds_bucket{query_type="aggregate",le="0.1"} 95
luminate_query_latency_seconds_bucket{query_type="aggregate",le="0.5"} 142
```

### Health Check Endpoint

```
GET /api/v1/health

Response (200 OK):
{
  "status": "healthy",
  "version": "1.0.0",
  "storage": {
    "backend": "badger",
    "status": "healthy",
    "disk_used_bytes": 1073741824
  },
  "uptime_seconds": 86400
}

Response (503 Service Unavailable):
{
  "status": "unhealthy",
  "version": "1.0.0",
  "storage": {
    "backend": "clickhouse",
    "status": "unavailable",
    "error": "connection refused"
  },
  "uptime_seconds": 86400
}
```

---

## Deployment

### Docker Deployment

**Dockerfile:**
```dockerfile
FROM golang:1.21-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o luminate ./cmd/luminate

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /app
COPY --from=builder /app/luminate .
COPY config.yaml .

EXPOSE 8080
VOLUME ["/var/lib/luminate"]

ENTRYPOINT ["./luminate"]
CMD ["--config", "config.yaml"]
```

**Docker Run (BadgerDB):**
```bash
docker run -d \
  --name luminate \
  -p 8080:8080 \
  -v luminate-data:/var/lib/luminate \
  -e LUMINATE_AUTH_JWT_SECRET=your-secret \
  luminate:latest
```

### Kubernetes Deployment

**BadgerDB Deployment (Stateful):**
```yaml
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: luminate
spec:
  serviceName: luminate
  replicas: 1  # BadgerDB = single instance
  selector:
    matchLabels:
      app: luminate
  template:
    metadata:
      labels:
        app: luminate
    spec:
      containers:
      - name: luminate
        image: luminate:1.0.0
        ports:
        - containerPort: 8080
        env:
        - name: LUMINATE_AUTH_JWT_SECRET
          valueFrom:
            secretKeyRef:
              name: luminate-secrets
              key: jwt-secret
        - name: LUMINATE_STORAGE_BACKEND
          value: "badger"
        volumeMounts:
        - name: data
          mountPath: /var/lib/luminate
        resources:
          requests:
            memory: "2Gi"
            cpu: "1000m"
          limits:
            memory: "4Gi"
            cpu: "2000m"
  volumeClaimTemplates:
  - metadata:
      name: data
    spec:
      accessModes: ["ReadWriteOnce"]
      resources:
        requests:
          storage: 100Gi
```

**ClickHouse Deployment (Stateless):**
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: luminate
spec:
  replicas: 3  # Horizontal scaling with ClickHouse
  selector:
    matchLabels:
      app: luminate
  template:
    metadata:
      labels:
        app: luminate
    spec:
      containers:
      - name: luminate
        image: luminate:1.0.0
        ports:
        - containerPort: 8080
        env:
        - name: LUMINATE_STORAGE_BACKEND
          value: "clickhouse"
        - name: LUMINATE_CLICKHOUSE_ADDRESSES
          value: "clickhouse-svc:9000"
        - name: LUMINATE_CLICKHOUSE_PASSWORD
          valueFrom:
            secretKeyRef:
              name: clickhouse-secrets
              key: password
        resources:
          requests:
            memory: "1Gi"
            cpu: "500m"
          limits:
            memory: "2Gi"
            cpu: "1000m"
```

### Resource Requirements

**BadgerDB (Single Instance):**
- CPU: 2-4 cores
- Memory: 4-8 GB
- Disk: 100 GB - 1 TB (depending on retention)
- IOPS: 1000+ (SSD recommended)

**ClickHouse (Per Luminate Instance):**
- CPU: 1-2 cores
- Memory: 2-4 GB
- Disk: None (stateless)
- Network: Low latency to ClickHouse cluster

---

## Data Retention

### Retention Policies

**Default Retention:**
```yaml
retention:
  default_retention: "30d"      # Keep metrics for 30 days
  cleanup_interval: "1h"        # Run cleanup every hour
```

**Per-Metric Retention (Optional):**
```yaml
retention:
  policies:
    - metric_pattern: "debug_*"
      retention: "7d"            # Debug metrics: 7 days
    - metric_pattern: "api_*"
      retention: "90d"           # API metrics: 90 days
    - metric_pattern: "*"
      retention: "30d"           # Default: 30 days
```

### Implementation

**BadgerDB:**
- Run periodic cleanup job (every hour)
- Delete keys with timestamp older than retention period
- Use TTL on keys if BadgerDB supports it

**ClickHouse:**
- Use TTL on table partitions:
  ```sql
  ALTER TABLE metrics MODIFY TTL timestamp + INTERVAL 30 DAY
  ```
- Automatic partition cleanup by ClickHouse

### Retention API

```
POST /api/v1/admin/retention
Authorization: Bearer <admin-jwt-token>

{
  "metric_name": "debug_metrics",
  "retention_days": 7
}

Response (200 OK):
{
  "metric_name": "debug_metrics",
  "retention_days": 7,
  "applied_at": "2024-01-01T00:00:00Z"
}
```

---

## Next Steps

1. Set up Go project structure (`go mod init`, directory layout)
2. Implement core data model and validation
3. Build JWT authentication middleware
4. Implement storage interface for chosen backend (BadgerDB or ClickHouse)
5. Create HTTP API handlers with rate limiting
6. Add batch write endpoint with compression support
7. Build query API with unified endpoint
8. Add self-monitoring metrics (Prometheus exposition)
9. Create configuration loading and validation
10. Write deployment manifests (Docker, Kubernetes)
11. Set up retention policies and cleanup jobs

---

## References

- ClickHouse: https://clickhouse.com/
- BadgerDB: https://github.com/dgraph-io/badger
- Cloudflare ClickHouse case study: https://blog.cloudflare.com/http-analytics-for-6m-requests-per-second-using-clickhouse/
- Uber ClickHouse migration: https://www.uber.com/blog/logging/
