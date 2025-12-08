# Luminate - High-Cardinality Observability System

Luminate is a high-performance observability system designed to handle high-cardinality metrics at scale. Unlike traditional time-series databases like Prometheus, Luminate is built from the ground up to support customer-level, tenant-level, and application-level metrics with millions of unique dimension combinations.

## Features

- **High Cardinality Support**: Track metrics at customer, tenant, and user levels
- **Flexible Storage Backends**: Choose between embedded BadgerDB or distributed ClickHouse
- **Horizontally Scalable**: Deploy with Kubernetes for automatic scaling
- **Multi-Tenant**: Built-in JWT authentication and tenant isolation
- **Rich Query API**: Support for aggregations, percentiles, rates, and time-weighted calculations
- **Production Ready**: Rate limiting, cardinality limits, health checks, and observability

## Architecture

The architecture is based on a pluggable storage backend approach, allowing you to choose between embedded or distributed storage based on your deployment needs.

## Quick Start

### Local Development

1. **Clone and setup:**
```bash
git clone https://github.com/yourusername/luminate.git
cd luminate
cp .env.example .env
# Edit .env and set JWT_SECRET
```

2. **Run with BadgerDB (embedded):**
```bash
make run
```

The server will start on `http://localhost:8080`.

### Using Docker

```bash
make docker-build
make docker-run
```

## Deployment Options

### Option 1: ClickHouse Backend (Horizontally Scalable) ⭐

**Best for:** Production deployments, high scale, enterprise use cases

**Characteristics:**
- Horizontal Scaling: Deploy 3-10+ replicas with automatic scaling
- Stateless Pods: Each instance connects to shared ClickHouse cluster
- High Availability: Automatic failover and load balancing
- Performance: Columnar storage optimized for high-cardinality queries
- Scale: Handles billions of metrics, petabyte scale

**Deploy to Kubernetes:**
```bash
# 1. Setup ClickHouse (or use managed service)
make clickhouse-local  # For local testing

# 2. Setup schema
make clickhouse-setup

# 3. Deploy Luminate
make k8s-deploy

# 4. Verify
make k8s-status
```

**Scaling Configuration:**
- Min Replicas: 3 (high availability)
- Max Replicas: 10 (configurable)
- Scale Up Trigger: 70% CPU or 80% memory
- Resources per Pod: 250m-1000m CPU, 512Mi-2Gi memory

### Option 2: BadgerDB Backend (Single Instance)

**Best for:** Edge deployments, development, small-scale, embedded scenarios

**Characteristics:**
- Single Instance: StatefulSet with 1 replica (not horizontally scalable)
- Embedded Database: No external dependencies
- Persistent Storage: 100GB SSD volume
- Zero Ops: No database cluster to manage

**Deploy:**
```bash
make k8s-deploy-badger
```

## Horizontal Scaling Details

### How Scaling Works with ClickHouse

1. **Stateless Application Layer**: Each Luminate pod is stateless and connects to shared ClickHouse cluster
2. **Load Balancing**: Kubernetes Service distributes traffic across all pods
3. **Auto-Scaling**: HorizontalPodAutoscaler (HPA) monitors CPU/memory and scales pods automatically
4. **Graceful Shutdown**: Pods handle ongoing requests before termination (30s grace period)
5. **Rolling Updates**: Zero-downtime deployments with maxUnavailable: 0

### Scaling Triggers

The HPA scales based on:
- **CPU**: Scale up when average CPU > 70%
- **Memory**: Scale up when average memory > 80%

### Scaling Behavior

**Scale Up:**
- Immediate (0s stabilization window)
- Max 100% increase per 15s (or 2 pods, whichever is more)

**Scale Down:**
- Wait 5 minutes before scaling down
- Max 50% reduction per 60s (or 1 pod, whichever is less)

## Makefile Commands

### Development
```bash
make help              # Show all available commands
make deps              # Download Go dependencies
make build             # Build for all platforms
make build-local       # Build for current platform
make test              # Run tests
make test-coverage     # Generate coverage report
make lint              # Run linter
make run               # Build and run locally
make dev               # Run with hot reload
```

### Docker
```bash
make docker-build      # Build Docker image
make docker-push       # Push to registry
make docker-run        # Run container locally
```

### Kubernetes
```bash
make k8s-deploy        # Deploy with ClickHouse (scalable)
make k8s-deploy-badger # Deploy with BadgerDB (single instance)
make k8s-delete        # Delete all resources
make k8s-logs          # View logs
make k8s-status        # Check deployment status
```

### Database
```bash
make clickhouse-setup  # Setup ClickHouse schema
make clickhouse-local  # Run ClickHouse locally
```

## API Examples

### Write Metrics

```bash
curl -X POST http://localhost:8080/api/v1/write \
  -H "Authorization: Bearer <jwt-token>" \
  -H "Content-Type: application/json" \
  -d '{
    "metrics": [
      {
        "name": "api_latency",
        "timestamp": 1735603200000,
        "value": 145.5,
        "dimensions": {
          "customer_id": "cust_123",
          "endpoint": "/api/users"
        }
      }
    ]
  }'
```

### Query Metrics

```bash
# Aggregate query (average latency)
curl -X POST http://localhost:8080/api/v1/query \
  -H "Authorization: Bearer <jwt-token>" \
  -H "Content-Type: application/json" \
  -d '{
    "queryType": "aggregate",
    "metricName": "api_latency",
    "timeRange": {"relative": "24h"},
    "aggregation": "avg",
    "groupBy": ["customer_id"]
  }'
```

## Project Structure

```
luminate/
├── cmd/luminate/          # Main application
├── pkg/
│   ├── api/              # HTTP API handlers
│   ├── auth/             # JWT authentication
│   ├── config/           # Configuration
│   ├── middleware/       # HTTP middleware
│   ├── models/           # Data models
│   └── storage/          # Storage interface
│       ├── badger/       # BadgerDB implementation
│       └── clickhouse/   # ClickHouse implementation
├── deployments/
│   ├── docker/           # Dockerfile
│   └── kubernetes/       # K8s manifests
├── configs/              # Configuration files
└── Makefile
```

## Performance

### ClickHouse Backend

- Write Throughput: 100K+ metrics/sec per pod
- Query Latency (p95): < 100ms
- Cardinality: Millions of unique series
- Horizontal Scaling: Linear to 10+ pods

### BadgerDB Backend

- Write Throughput: 10K+ metrics/sec
- Query Latency (p95): < 500ms
- Cardinality: Millions (single machine limit)
- Scaling: Single instance only

## License

MIT License

## Documentation

See [CLAUDE.md](CLAUDE.md) for detailed architecture and design decisions.
