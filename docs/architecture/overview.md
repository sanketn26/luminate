# System Architecture Overview

## Introduction

Luminate is a high-cardinality observability system designed to capture and query metrics with millions of unique dimension combinations. Unlike traditional time-series databases optimized for infrastructure monitoring, Luminate is purpose-built for application-level observability at customer, tenant, and user granularity.

## Design Philosophy

### Core Principles

1. **High Cardinality as First-Class Concern**
   - Support millions of unique dimension combinations
   - No artificial limits on customer/tenant tracking
   - Efficient storage and querying at scale

2. **Storage Flexibility**
   - Abstract storage behind a unified interface
   - Support both embedded (BadgerDB) and distributed (ClickHouse) backends
   - Swap storage without application code changes

3. **Horizontal Scalability**
   - Stateless application pods
   - Scale from 3 to 10+ replicas automatically
   - Linear performance with pod count (ClickHouse backend)

4. **Multi-Tenancy from Day One**
   - JWT-based tenant isolation
   - Automatic tenant filtering on all queries
   - Per-tenant rate limits and quotas

5. **Developer Experience**
   - Zero dependencies for local development (BadgerDB)
   - Simple API with unified query endpoint
   - Comprehensive error messages

## System Architecture

### High-Level Components

```
┌─────────────────────────────────────────────────────────────────┐
│                         Client Applications                      │
│          (Services, dashboards, monitoring tools)                │
└────────────────────────────┬────────────────────────────────────┘
                             │ HTTPS/JWT
                ┌────────────▼───────────┐
                │   Load Balancer/       │
                │   Ingress Controller   │
                └────────────┬───────────┘
                             │
        ┌────────────────────┼────────────────────┐
        │                    │                    │
  ┌─────▼──────┐      ┌─────▼──────┐      ┌─────▼──────┐
  │  Luminate   │      │  Luminate   │      │  Luminate   │
  │   Pod 1     │      │   Pod 2     │      │   Pod N     │
  │             │      │             │      │             │
  │ ┌─────────┐ │      │ ┌─────────┐ │      │ ┌─────────┐ │
  │ │  Auth   │ │      │ │  Auth   │ │      │ │  Auth   │ │
  │ │Middleware│ │      │ │Middleware│ │      │ │Middleware│ │
  │ └────┬────┘ │      │ └────┬────┘ │      │ └────┬────┘ │
  │      │      │      │      │      │      │      │      │
  │ ┌────▼────┐ │      │ ┌────▼────┐ │      │ ┌────▼────┐ │
  │ │  Rate   │ │      │ │  Rate   │ │      │ │  Rate   │ │
  │ │ Limiter │ │      │ │ Limiter │ │      │ │ Limiter │ │
  │ └────┬────┘ │      │ └────┬────┘ │      │ └────┬────┘ │
  │      │      │      │      │      │      │      │      │
  │ ┌────▼────┐ │      │ ┌────▼────┐ │      │ ┌────▼────┐ │
  │ │   API   │ │      │ │   API   │ │      │ │   API   │ │
  │ │Handlers │ │      │ │Handlers │ │      │ │Handlers │ │
  │ └────┬────┘ │      │ └────┬────┘ │      │ └────┬────┘ │
  │      │      │      │      │      │      │      │      │
  │ ┌────▼────┐ │      │ ┌────▼────┐ │      │ ┌────▼────┐ │
  │ │ Storage │ │      │ │ Storage │ │      │ │ Storage │ │
  │ │Interface│ │      │ │Interface│ │      │ │Interface│ │
  │ └────┬────┘ │      │ └────┬────┘ │      │ └────┬────┘ │
  └──────┼──────┘      └──────┼──────┘      └──────┼──────┘
         │                    │                    │
         └────────────────────┼────────────────────┘
                              │
                    ┌─────────▼──────────┐
                    │   Storage Backend   │
                    │                     │
                    │  ┌──────────────┐  │
                    │  │  ClickHouse  │  │
                    │  │   Cluster    │  │
                    │  │              │  │
                    │  │ • Shard 1    │  │
                    │  │   - Replica 1│  │
                    │  │   - Replica 2│  │
                    │  │ • Shard 2    │  │
                    │  │   - Replica 1│  │
                    │  │   - Replica 2│  │
                    │  └──────────────┘  │
                    │                     │
                    │  OR                 │
                    │                     │
                    │  ┌──────────────┐  │
                    │  │  BadgerDB    │  │
                    │  │  (embedded)  │  │
                    │  └──────────────┘  │
                    └─────────────────────┘
```

### Component Responsibilities

#### 1. Load Balancer / Ingress
- **Role:** Distribute traffic across Luminate pods
- **Technology:** Kubernetes Service (LoadBalancer or Ingress)
- **Features:**
  - TLS termination
  - Session affinity (optional)
  - Health check integration

#### 2. Luminate Pods (Stateless)
Each pod contains:

**Auth Middleware:**
- Validate JWT tokens
- Extract tenant ID from claims
- Verify scopes (read, write, admin)
- Inject tenant context

**Rate Limiter:**
- Token bucket per tenant
- HyperLogLog cardinality tracking
- Per-tenant quotas:
  - 100 write requests/sec
  - 50 query requests/sec
  - 10,000 metrics/sec

**API Handlers:**
- Unified query endpoint (POST `/api/v1/query`)
- Batch write endpoint (POST `/api/v1/write`)
- Discovery endpoints (GET `/api/v1/metrics/*`)
- Health checks (GET `/healthz`, `/readyz`, `/startupz`)

**Storage Interface:**
- Abstract storage operations
- Automatic tenant filtering
- Connection pooling

#### 3. Storage Backend

**BadgerDB (Development/Single-Node):**
- Embedded LSM-tree key-value store
- Zero external dependencies
- Single-process operation
- Suitable for: Development, testing, small deployments

**ClickHouse (Production/Scale):**
- Distributed columnar database
- Horizontal scalability
- High-cardinality optimization
- Suitable for: Production, high-traffic environments

## Request Flow

### Write Request Flow

```
1. Client → Load Balancer
   POST /api/v1/write
   Authorization: Bearer <JWT>
   Body: { "metrics": [...] }

2. Load Balancer → Luminate Pod (selected)

3. Luminate Pod:
   a. Auth Middleware
      - Validate JWT signature
      - Extract tenant_id: "acme-corp"
      - Verify 'write' scope

   b. Rate Limiter
      - Check tenant quota: 100 req/sec
      - Check metrics quota: 10K metrics/sec
      - Update HyperLogLog cardinality

   c. Validation
      - Validate metric names (regex)
      - Validate dimension keys/values
      - Check dimension count (<= 20)
      - Validate timestamps (7 days past - 1 hour future)

   d. Tenant Injection
      - Add "_tenant_id: acme-corp" to each metric

   e. Storage Write
      - Batch insert to ClickHouse
      - OR write to BadgerDB

4. Response → Client
   200 OK { "accepted": 100, "rejected": 0 }
```

### Query Request Flow

```
1. Client → Load Balancer
   POST /api/v1/query
   Authorization: Bearer <JWT>
   Body: {
     "queryType": "aggregate",
     "metricName": "api_latency",
     "aggregation": "p95",
     "timeRange": { "relative": "1h" },
     "groupBy": ["endpoint"]
   }

2. Load Balancer → Luminate Pod

3. Luminate Pod:
   a. Auth Middleware
      - Validate JWT
      - Extract tenant_id: "acme-corp"
      - Verify 'read' scope

   b. Rate Limiter
      - Check query quota: 50 req/sec
      - Allow request

   c. Query Construction
      - Add tenant filter: _tenant_id = "acme-corp"
      - Parse time range: now - 1 hour to now
      - Build storage query

   d. Storage Query
      - ClickHouse: SELECT quantile(0.95)(value) ... GROUP BY endpoint
      - BadgerDB: Scan KV pairs, compute in-memory

   e. Result Formatting
      - Convert to JSON response

4. Response → Client
   200 OK {
     "results": [
       { "endpoint": "/api/query", "value": 0.145 },
       { "endpoint": "/api/write", "value": 0.089 }
     ]
   }
```

## Storage Abstraction Layer

### Interface Design

```go
type MetricsStore interface {
    // Write operations
    Write(ctx context.Context, metrics []Metric) error

    // Query operations
    QueryRange(ctx context.Context, req QueryRequest) ([]MetricPoint, error)
    Aggregate(ctx context.Context, req AggregateRequest) ([]AggregateResult, error)
    Rate(ctx context.Context, req RateRequest) ([]RatePoint, error)

    // Discovery
    ListMetrics(ctx context.Context) ([]string, error)
    ListDimensionKeys(ctx context.Context, metricName string) ([]string, error)
    ListDimensionValues(ctx context.Context, metricName, dimensionKey string, limit int) ([]string, error)

    // Lifecycle
    DeleteBefore(ctx context.Context, metricName string, before time.Time) error
    Health(ctx context.Context) error
    Close() error
}
```

### Implementation Strategy

**BadgerDB:**
- Key format: `metric:<name>:<timestamp_ms>:<dimension_hash>`
- Manual aggregations (in-memory scan)
- Prefix scans for time ranges
- Efficient for single-node deployments

**ClickHouse:**
- Table: `metrics` with MergeTree engine
- Native SQL aggregations
- Bloom filter indexes
- Partitioning by month
- Suitable for cluster deployments

### Benefits of Abstraction

1. **Flexibility:** Swap backends without code changes
2. **Testing:** Mock interface for unit tests
3. **Evolution:** Add new backends (e.g., TimescaleDB)
4. **Simplicity:** Single API surface for all operations

## Scalability Architecture

### Horizontal Scaling (ClickHouse)

**Auto-Scaling Configuration:**
```yaml
HorizontalPodAutoscaler:
  minReplicas: 3
  maxReplicas: 10
  metrics:
    - CPU: 70%
    - Memory: 80%
```

**Scaling Behavior:**
- Start with 3 replicas for high availability
- Scale up at 70% CPU utilization
- Scale down after 5 minutes of low utilization
- Max 10 replicas to prevent over-provisioning

**Performance at Scale:**
- 3 pods: ~300K metrics/sec
- 5 pods: ~500K metrics/sec
- 10 pods: ~1M metrics/sec

### Vertical Scaling (BadgerDB)

**Resource Allocation:**
```yaml
resources:
  requests:
    cpu: "1"
    memory: "2Gi"
  limits:
    cpu: "2"
    memory: "4Gi"
```

**Performance:**
- Single pod: ~10K metrics/sec
- Suitable for development/testing
- Not recommended for production

## Multi-Tenancy Architecture

### Tenant Isolation Strategy

**JWT Claims:**
```json
{
  "tenant_id": "acme-corp",
  "org_id": "org_123",
  "scopes": ["read", "write"],
  "tier": "enterprise"
}
```

**Dimension Injection:**
```go
// Original metric
{
  "name": "api_latency",
  "value": 0.150,
  "dimensions": {
    "endpoint": "/api/query"
  }
}

// After tenant injection
{
  "name": "api_latency",
  "value": 0.150,
  "dimensions": {
    "endpoint": "/api/query",
    "_tenant_id": "acme-corp"  // ← Injected automatically
  }
}
```

**Query Filtering:**
All queries automatically include:
```sql
WHERE dimensions['_tenant_id'] = 'acme-corp'
```

**Benefits:**
- Zero chance of cross-tenant data access
- Simple schema (single table)
- Easy to audit and debug
- Natural fit with dimensional model

## Observability of Luminate Itself

### Internal Metrics (Prometheus)

Luminate exports its own metrics:
- `luminate_http_requests_total` - Request rates by endpoint
- `luminate_http_request_duration_seconds` - Request latency
- `luminate_storage_writes_total` - Write operations
- `luminate_storage_query_duration_seconds` - Query latency
- `luminate_rate_limit_rejected_total` - Rate limit hits

### Health Checks

**Kubernetes Probes:**
- **Liveness:** `/healthz` - Process is responsive
- **Readiness:** `/readyz` - Ready to accept traffic
- **Startup:** `/startupz` - Initialization complete

### Logging

Structured JSON logs with:
- Request ID for tracing
- Tenant ID for filtering
- Error codes for alerting
- Duration for performance analysis

## Technology Choices

| Component | Technology | Rationale |
|-----------|-----------|-----------|
| Language | Go 1.21+ | Performance, concurrency, simple deployment |
| Embedded Storage | BadgerDB | Zero dependencies, LSM-tree, Go-native |
| Distributed Storage | ClickHouse | Best-in-class for high-cardinality OLAP |
| Authentication | JWT | Stateless, standard, easy integration |
| Monitoring | Prometheus | Industry standard, excellent Go support |
| Visualization | Grafana | Universal dashboarding, plugin ecosystem |
| Container | Docker | Standard containerization |
| Orchestration | Kubernetes | Auto-scaling, self-healing, declarative |

## Security Considerations

1. **Authentication:** Required for all production deployments
2. **Authorization:** Scope-based (read, write, admin)
3. **Tenant Isolation:** Automatic via dimension injection
4. **Rate Limiting:** Prevent abuse and ensure fairness
5. **Input Validation:** All inputs validated before processing
6. **TLS:** HTTPS recommended for production

## Performance Targets

| Metric | Target | Backend |
|--------|--------|---------|
| Write Throughput | 10K/sec | BadgerDB |
| Write Throughput | 100K/sec per pod | ClickHouse |
| Query Latency (p95) | <500ms | BadgerDB |
| Query Latency (p95) | <100ms | ClickHouse |
| Max Unique Series | 1M | BadgerDB |
| Max Unique Series | Billions | ClickHouse |

## Deployment Models

### Development
- Single BadgerDB instance
- In-memory mode
- No external dependencies

### Staging
- 2-3 Luminate pods
- Small ClickHouse cluster (2 nodes)
- Reduced resource allocation

### Production
- 3-10 Luminate pods (auto-scaled)
- ClickHouse cluster (4+ nodes)
- Full monitoring and alerting
- Automated backups

## Future Extensibility

The architecture supports future enhancements:
1. **Additional Storage Backends** (e.g., TimescaleDB, M3DB)
2. **Query Language** (PromQL-like syntax)
3. **Distributed Tracing** (OpenTelemetry integration)
4. **Anomaly Detection** (ML-based alerting)
5. **Data Federation** (Multi-region queries)

---

**See Also:**
- [Storage Architecture](storage.md)
- [Data Model](data-model.md)
- [Security Architecture](security.md)
