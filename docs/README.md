# Luminate Documentation

Welcome to the Luminate documentation! This directory contains comprehensive technical documentation for the Luminate high-cardinality observability system.

## 📚 Documentation Structure

```
docs/
├── README.md                    # This file - documentation index
├── WORKSTREAMS.md              # Complete implementation plan overview
│
├── architecture/               # Architecture and design documents
│   ├── README.md              # Architecture documentation index
│   ├── overview.md            # System architecture overview
│   ├── storage.md             # Storage backend comparison
│   ├── data-model.md          # Data model and validation
│   ├── api-design.md          # API design principles
│   └── security.md            # Security architecture
│
├── workstreams/               # Detailed implementation workstreams
│   ├── 01-storage-badgerdb.md
│   ├── 02-core-models.md
│   ├── 03-api-handlers.md
│   ├── 04-authentication.md
│   ├── 05-rate-limiting.md
│   ├── 06-storage-clickhouse.md
│   ├── 07-internal-metrics.md
│   ├── 08-health-discovery.md
│   ├── 09-testing.md
│   ├── 10-production-deployment.md
│   └── 11-dashboard-ui.md
│
├── operations/                # Operational documentation
│   ├── README.md             # Operations guide index
│   ├── deployment.md         # Deployment procedures
│   ├── monitoring.md         # Monitoring and alerting
│   ├── backup-restore.md     # Backup and restore procedures
│   └── runbooks/             # Incident response runbooks
│       ├── high-error-rate.md
│       ├── high-latency.md
│       ├── storage-issues.md
│       └── scaling.md
│
└── guides/                   # User and developer guides
    ├── README.md            # Guides index
    ├── quickstart.md        # Quick start guide
    ├── development.md       # Development setup
    ├── configuration.md     # Configuration reference
    └── api-reference.md     # API reference
```

## 🚀 Quick Start

**New to Luminate?** Start here:
1. Read the [Architecture Overview](architecture/overview.md) to understand the system design
2. Review [WORKSTREAMS.md](WORKSTREAMS.md) for the implementation plan
3. Follow the [Quick Start Guide](guides/quickstart.md) to get running locally
4. Explore individual [workstreams](workstreams/) for detailed implementation

## 📖 Documentation Categories

### Architecture Documentation

High-level design documents explaining **why** decisions were made:
- **[Architecture Overview](architecture/overview.md)** - System architecture and design principles
- **[Storage Architecture](architecture/storage.md)** - BadgerDB vs ClickHouse comparison
- **[Data Model](architecture/data-model.md)** - Metric structure and validation rules
- **[API Design](architecture/api-design.md)** - API patterns and conventions
- **[Security Architecture](architecture/security.md)** - Authentication and multi-tenancy

### Implementation Workstreams

Detailed **how-to** guides for implementing each component:

#### Phase 1: Foundation (Weeks 1-2)
- **[WS1: Storage Backend - BadgerDB](workstreams/01-storage-badgerdb.md)** - Embedded KV storage implementation
- **[WS2: Core Data Models](workstreams/02-core-models.md)** - Data validation and serialization

#### Phase 2: API Layer (Weeks 3-4)
- **[WS3: HTTP API Handlers](workstreams/03-api-handlers.md)** - REST API endpoints
- **[WS4: Authentication & Security](workstreams/04-authentication.md)** - JWT and multi-tenancy
- **[WS5: Rate Limiting & Validation](workstreams/05-rate-limiting.md)** - Rate limits and cardinality tracking

#### Phase 3: Advanced Storage (Weeks 5-6)
- **[WS6: Storage Backend - ClickHouse](workstreams/06-storage-clickhouse.md)** - Production-scale storage

#### Phase 4: Observability & UI (Week 7)
- **[WS7: Internal Metrics](workstreams/07-internal-metrics.md)** - Prometheus instrumentation
- **[WS8: Health Checks & Discovery](workstreams/08-health-discovery.md)** - Health probes and metric discovery
- **[WS11: Dashboard UI](workstreams/11-dashboard-ui.md)** - Grafana datasource plugin

#### Phase 5: Testing & Deployment (Week 8)
- **[WS9: Testing Framework](workstreams/09-testing.md)** - Unit, integration, load testing
- **[WS10: Production Deployment](workstreams/10-production-deployment.md)** - K8s deployment and operations

### Operational Documentation

Production operations and troubleshooting:
- **[Deployment Guide](operations/deployment.md)** - Step-by-step deployment procedures
- **[Monitoring Guide](operations/monitoring.md)** - Metrics, alerts, and dashboards
- **[Backup & Restore](operations/backup-restore.md)** - Data protection procedures
- **[Runbooks](operations/runbooks/)** - Incident response guides

### User & Developer Guides

Practical guides for using and developing Luminate:
- **[Quick Start](guides/quickstart.md)** - Get up and running in 5 minutes
- **[Development Setup](guides/development.md)** - Local development environment
- **[Configuration Reference](guides/configuration.md)** - All configuration options
- **[API Reference](guides/api-reference.md)** - Complete API documentation

## 🎯 Key Concepts

### Storage Backends

Luminate supports two storage backends via a unified interface:

| Feature | BadgerDB | ClickHouse |
|---------|----------|------------|
| **Use Case** | Development, single-node | Production, horizontal scaling |
| **Throughput** | 10K+ metrics/sec | 100K+ metrics/sec per pod |
| **Scaling** | Vertical only | Horizontal (3-10+ pods) |
| **Dependencies** | None (embedded) | Requires ClickHouse cluster |
| **Query Performance** | p95 < 500ms | p95 < 100ms |

### Data Model

Luminate captures metrics with flexible dimensions:

```go
{
  "name": "api_latency",
  "value": 0.150,
  "timestamp": 1701234567890,
  "dimensions": {
    "endpoint": "/api/query",
    "method": "POST",
    "customer_id": "acme-corp",
    "region": "us-east-1"
  }
}
```

**Validation Rules:**
- Metric names: `^[a-zA-Z_][a-zA-Z0-9_]*$` (1-256 chars)
- Max 20 dimensions per metric
- Timestamp window: [now - 7 days, now + 1 hour]
- Values must be finite (no NaN/Inf)

### Aggregation Types

Nine aggregation types supported:
- **Basic:** AVG, SUM, COUNT, MIN, MAX
- **Percentiles:** P50, P95, P99
- **Time-weighted:** INTEGRAL (for resource consumption)

### Multi-Tenancy

JWT-based tenant isolation:
- Automatic `_tenant_id` dimension injection
- All queries filtered by tenant
- Per-tenant rate limits and quotas

## 📊 Performance Targets

### BadgerDB Backend
- **Write:** 10,000+ metrics/sec
- **Query:** p95 < 500ms
- **Scaling:** Vertical only

### ClickHouse Backend
- **Write:** 100,000+ metrics/sec per pod
- **Query:** p95 < 100ms
- **Scaling:** Horizontal (3-10 pods with HPA)
- **Auto-scaling trigger:** 70% CPU, 80% memory

## 🔧 Development

### Prerequisites
- Go 1.21+
- Docker & Docker Compose
- kubectl (for Kubernetes deployment)
- make

### Build & Test

```bash
# Build
make build-local

# Run tests
make test

# Run with coverage
make test-coverage

# Run integration tests
make test-integration

# Run load tests
make test-load
```

### Local Development

```bash
# Start with BadgerDB (no dependencies)
make run

# Start with ClickHouse (requires Docker)
docker-compose up -d clickhouse
LUMINATE_STORAGE_BACKEND=clickhouse make run
```

## 🚢 Deployment

### Quick Deploy (BadgerDB)

```bash
# Single-node deployment with embedded storage
make k8s-deploy-badger
```

### Production Deploy (ClickHouse)

```bash
# Multi-node deployment with ClickHouse cluster
make k8s-deploy

# Verify deployment
make k8s-status

# View logs
make k8s-logs
```

See [Deployment Guide](operations/deployment.md) for detailed instructions.

## 📈 Monitoring

Luminate exposes Prometheus metrics at `/metrics`:
- HTTP request rates and latency
- Storage write/query performance
- Rate limiting decisions
- System resources (CPU, memory, goroutines)

See [Monitoring Guide](operations/monitoring.md) for dashboard setup.

## 🔐 Security

- **Authentication:** JWT-based with configurable secret
- **Authorization:** Scope-based (read, write, admin)
- **Multi-tenancy:** Automatic tenant isolation
- **Rate Limiting:** Per-tenant quotas
- **TLS:** Optional HTTPS support

See [Security Architecture](architecture/security.md) for details.

## 📝 Contributing

See individual workstream documents for implementation guidelines. Each workstream includes:
- Technical specifications
- Implementation steps
- Code examples
- Test requirements
- Acceptance criteria

## 🔗 Related Documentation

- **[CLAUDE.md](../CLAUDE.md)** - Project overview and common commands
- **[ARCHITECTURE.md](../ARCHITECTURE.md)** - Original architecture design document
- **[PROJECT_STRUCTURE.md](../PROJECT_STRUCTURE.md)** - Codebase organization
- **[DEPLOYMENT_GUIDE.md](../DEPLOYMENT_GUIDE.md)** - Kubernetes deployment guide

## 📧 Support

For questions or issues:
1. Check the [runbooks](operations/runbooks/) for common issues
2. Review the [API reference](guides/api-reference.md)
3. Open an issue on GitHub

---

**Last Updated:** 2024-12-09
**Version:** 1.0
**Status:** Complete Implementation Documentation
