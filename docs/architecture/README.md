# Architecture Documentation

This directory contains high-level architecture and design documents for Luminate.

## Documents

### [Overview](overview.md)
System architecture overview, key design decisions, and architectural principles.

**Topics covered:**
- System components and their interactions
- Storage abstraction layer design
- Scalability architecture
- Technology stack choices

### [Storage](storage.md)
Deep dive into storage backend options and trade-offs.

**Topics covered:**
- BadgerDB vs ClickHouse comparison
- High-cardinality challenges
- Storage schema design
- Performance characteristics
- When to use each backend

### [Data Model](data-model.md)
Metric structure, validation rules, and data constraints.

**Topics covered:**
- Metric schema
- Dimension structure
- Validation rules and constraints
- Aggregation types
- Time-series data model

### [API Design](api-design.md)
API design principles and patterns.

**Topics covered:**
- Unified query endpoint design
- REST conventions
- Request/response formats
- Error handling patterns
- Versioning strategy

### [Security](security.md)
Security architecture and multi-tenancy design.

**Topics covered:**
- JWT-based authentication
- Multi-tenancy isolation
- Scope-based authorization
- Rate limiting strategy
- Security best practices

## Key Architectural Decisions

### 1. Storage Abstraction Layer
**Decision:** Single `MetricsStore` interface with two implementations (BadgerDB, ClickHouse)

**Rationale:**
- Enable development with zero dependencies (BadgerDB)
- Scale to production with ClickHouse
- Swap backends without code changes
- Test easily with mock implementations

### 2. High-Cardinality Support
**Decision:** Use ClickHouse for production workloads

**Rationale:**
- Columnar storage handles millions of unique dimensions
- Excellent compression (10-100x)
- Sub-second queries on billions of rows
- Proven at scale (Cloudflare, Uber)

### 3. Multi-Tenancy via Dimensions
**Decision:** Inject `_tenant_id` as a dimension, not a separate table

**Rationale:**
- Simple schema (single metrics table)
- Automatic filtering via dimension filters
- No cross-tenant data leakage
- Easy to understand and debug

### 4. Unified Query Endpoint
**Decision:** Single POST `/api/v1/query` with `queryType` discriminator

**Rationale:**
- Consistent API surface
- Easier to version and evolve
- Simpler error handling
- Better for GraphQL-style queries

### 5. JWT Authentication
**Decision:** Stateless JWT tokens with tenant claims

**Rationale:**
- No session storage required
- Horizontal scaling without shared state
- Standard industry practice
- Easy integration with external auth systems

## Architecture Principles

1. **Simplicity Over Complexity**
   - Single metrics table per backend
   - No joins required
   - Simple filters (equality only)

2. **Performance by Default**
   - Batch writes encouraged
   - Efficient storage formats
   - Optimized query patterns

3. **Graceful Degradation**
   - Continue serving reads if writes fail
   - Degraded mode when dependencies are unhealthy
   - Clear error messages

4. **Observable by Design**
   - Internal Prometheus metrics
   - Structured logging
   - Health check endpoints
   - Distributed tracing ready

5. **Secure by Default**
   - Authentication required in production
   - Tenant isolation enforced
   - Rate limiting enabled
   - Input validation on all requests

## System Components

```
┌─────────────────────────────────────────────────────────┐
│                     Load Balancer                        │
└────────────────────┬────────────────────────────────────┘
                     │
        ┌────────────┴────────────┬──────────────┐
        │                         │              │
   ┌────▼────┐               ┌────▼────┐    ┌────▼────┐
   │ Luminate│               │ Luminate│    │ Luminate│
   │  Pod 1  │               │  Pod 2  │    │  Pod N  │
   └────┬────┘               └────┬────┘    └────┬────┘
        │                         │              │
        └────────────┬────────────┴──────────────┘
                     │
            ┌────────▼─────────┐
            │   ClickHouse     │
            │     Cluster      │
            │  (4 nodes:       │
            │   2 shards x     │
            │   2 replicas)    │
            └──────────────────┘
```

## Data Flow

### Write Path
1. Client sends batch write request
2. JWT authentication validates tenant
3. Rate limiter checks quota
4. Metrics validated against schema
5. `_tenant_id` dimension injected
6. Batch written to storage backend
7. Response returned to client

### Query Path
1. Client sends query request
2. JWT authentication extracts tenant
3. Rate limiter checks quota
4. Query parameters validated
5. Tenant filter automatically added
6. Storage executes query
7. Results returned to client

## Scaling Strategy

### Horizontal Scaling (ClickHouse Backend)
- **Stateless Luminate pods:** 3-10 replicas via HPA
- **Trigger:** 70% CPU or 80% memory
- **Anti-affinity:** Spread across nodes
- **Zero-downtime updates:** Rolling deployment

### Vertical Scaling (BadgerDB Backend)
- Single pod with increased resources
- Suitable for development/testing
- Not recommended for production

## Performance Characteristics

| Metric | BadgerDB | ClickHouse |
|--------|----------|------------|
| Write Throughput | 10K/sec | 100K/sec per pod |
| Query Latency (p95) | <500ms | <100ms |
| Max Series | 1M | Billions |
| Scaling | Vertical | Horizontal |
| Dependencies | None | ClickHouse cluster |

## Technology Stack

- **Language:** Go 1.21+
- **Storage:** BadgerDB (embedded), ClickHouse (distributed)
- **Coordination:** Zookeeper (for ClickHouse replication)
- **Container:** Docker
- **Orchestration:** Kubernetes
- **Monitoring:** Prometheus, Grafana
- **Authentication:** JWT
- **Testing:** Go test, k6 (load testing)

## Related Documents

- **[../WORKSTREAMS.md](../WORKSTREAMS.md)** - Implementation plan
- **[../../ARCHITECTURE.md](../../ARCHITECTURE.md)** - Original architecture document
- **[../../CLAUDE.md](../../CLAUDE.md)** - Project overview

---

**Last Updated:** 2024-12-09
