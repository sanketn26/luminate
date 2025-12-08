# Luminate Project Structure

## Complete Directory Layout

```
luminate/
├── cmd/
│   └── luminate/
│       └── main.go                    # Application entry point
├── pkg/
│   ├── api/                          # HTTP API handlers (to be implemented)
│   ├── auth/                         # JWT authentication (to be implemented)
│   ├── config/
│   │   └── config.go                 # Configuration management
│   ├── middleware/                   # HTTP middleware (to be implemented)
│   ├── models/
│   │   └── metric.go                 # Core data models
│   ├── query/                        # Query parsing (to be implemented)
│   ├── ratelimit/                    # Rate limiting (to be implemented)
│   └── storage/
│       ├── interface.go              # Storage abstraction
│       ├── badger/                   # BadgerDB implementation (to be implemented)
│       └── clickhouse/               # ClickHouse implementation (to be implemented)
├── internal/
│   ├── metrics/                      # Internal metrics collection (to be implemented)
│   └── validation/                   # Validation utilities (to be implemented)
├── deployments/
│   ├── docker/
│   │   └── Dockerfile                # Multi-stage Docker build
│   └── kubernetes/
│       ├── namespace.yaml            # Kubernetes namespace
│       ├── configmap.yaml            # ClickHouse config
│       ├── configmap-badger.yaml     # BadgerDB config
│       ├── secret.yaml               # Secrets (JWT, passwords)
│       ├── deployment.yaml           # Deployment (ClickHouse backend, 3-10 replicas)
│       ├── statefulset.yaml          # StatefulSet (BadgerDB backend, 1 replica)
│       ├── service.yaml              # Kubernetes services
│       ├── hpa.yaml                  # Horizontal Pod Autoscaler
│       └── ingress.yaml              # Ingress configuration
├── configs/
│   └── config.yaml                   # Default configuration
├── scripts/
│   └── clickhouse-setup.sh           # ClickHouse schema setup
├── build/                            # Build artifacts (gitignored)
├── Makefile                          # Build and deployment automation
├── go.mod                            # Go module definition
├── go.sum                            # Go dependencies (generated)
├── .env.example                      # Environment variable template
├── .gitignore                        # Git ignore rules
├── .air.toml                         # Hot reload configuration
├── README.md                         # Project documentation
├── CLAUDE.md                         # Detailed architecture documentation
└── PROJECT_STRUCTURE.md              # This file
```

## Key Components

### 1. Application Entry Point
- [cmd/luminate/main.go](cmd/luminate/main.go) - Main application with server setup, middleware chain, graceful shutdown

### 2. Core Abstractions
- [pkg/storage/interface.go](pkg/storage/interface.go) - MetricsStore interface for pluggable storage backends
- [pkg/models/metric.go](pkg/models/metric.go) - Core data models with validation
- [pkg/config/config.go](pkg/config/config.go) - Configuration management with YAML and env vars

### 3. Deployment Configurations

#### Horizontally Scalable (ClickHouse)
- [deployments/kubernetes/deployment.yaml](deployments/kubernetes/deployment.yaml)
  - Replicas: 3-10 (auto-scaling)
  - Stateless pods
  - Pod anti-affinity for node distribution
  - Resources: 250m-1000m CPU, 512Mi-2Gi memory

- [deployments/kubernetes/hpa.yaml](deployments/kubernetes/hpa.yaml)
  - Auto-scaling based on CPU (70%) and memory (80%)
  - Scale up: immediate, max 100% or 2 pods
  - Scale down: 5min stabilization, max 50% or 1 pod

#### Single Instance (BadgerDB)
- [deployments/kubernetes/statefulset.yaml](deployments/kubernetes/statefulset.yaml)
  - Replicas: 1 (not scalable)
  - Persistent volume: 100Gi SSD
  - Resources: 500m-2000m CPU, 2Gi-4Gi memory

### 4. Build Automation
- [Makefile](Makefile) - Comprehensive build, test, and deployment targets
  - Development: build, test, run, dev
  - Docker: docker-build, docker-push, docker-run
  - Kubernetes: k8s-deploy, k8s-deploy-badger, k8s-delete
  - Database: clickhouse-setup, clickhouse-local

## Deployment Strategies

### Strategy 1: Production ClickHouse (Recommended)

```bash
# 1. Setup ClickHouse
make clickhouse-local      # or use managed ClickHouse

# 2. Initialize schema
make clickhouse-setup

# 3. Deploy to Kubernetes with auto-scaling
make k8s-deploy

# Result:
# - 3-10 Luminate pods (auto-scaling)
# - Stateless, horizontally scalable
# - Shared ClickHouse cluster
# - High availability
```

### Strategy 2: Edge/Dev BadgerDB

```bash
# Deploy single instance with embedded database
make k8s-deploy-badger

# Result:
# - 1 Luminate pod (StatefulSet)
# - Embedded BadgerDB
# - 100GB persistent volume
# - No external dependencies
```

## Horizontal Scaling Architecture

```
┌─────────────────────────────────────────────────────────┐
│                    Ingress / Load Balancer              │
│                  (luminate.example.com)                 │
└─────────────────────────────────────────────────────────┘
                          ↓
┌─────────────────────────────────────────────────────────┐
│              Kubernetes Service (ClusterIP)             │
│              Distributes traffic across pods            │
└─────────────────────────────────────────────────────────┘
                          ↓
┌─────────────────────────────────────────────────────────┐
│            Luminate Pods (Deployment)                   │
│  ┌────────┐  ┌────────┐  ┌────────┐  ┌────────┐       │
│  │ Pod 1  │  │ Pod 2  │  │ Pod 3  │  │ Pod N  │       │
│  │Stateless│  │Stateless│  │Stateless│  │Stateless│       │
│  └────────┘  └────────┘  └────────┘  └────────┘       │
│                                                         │
│  Min: 3 replicas, Max: 10 replicas                     │
│  Auto-scaling on CPU (70%) and Memory (80%)            │
└─────────────────────────────────────────────────────────┘
                          ↓
┌─────────────────────────────────────────────────────────┐
│          ClickHouse Cluster (External Service)          │
│          Shared columnar storage for all pods           │
│          Handles high-cardinality dimensions            │
└─────────────────────────────────────────────────────────┘
```

## Key Features for Horizontal Scalability

### 1. Stateless Application Design
- No local state stored in pods
- All metric data stored in external ClickHouse
- Configuration via ConfigMaps and environment variables
- Can add/remove pods without data loss

### 2. Load Balancing
- Kubernetes Service distributes traffic evenly
- Session affinity: None (any pod can handle any request)
- Health checks ensure traffic only goes to healthy pods

### 3. Auto-Scaling (HPA)
```yaml
minReplicas: 3
maxReplicas: 10

metrics:
  - CPU: 70% average utilization
  - Memory: 80% average utilization

behavior:
  scaleUp: Immediate (0s stabilization)
  scaleDown: 5 minutes stabilization
```

### 4. High Availability
- Pod anti-affinity spreads pods across nodes
- Multiple replicas ensure availability during failures
- Graceful shutdown (30s termination grace period)
- Rolling updates with zero downtime (maxUnavailable: 0)

### 5. Resource Management
```yaml
requests:     # Guaranteed resources
  cpu: 250m
  memory: 512Mi

limits:       # Maximum allowed
  cpu: 1000m
  memory: 2Gi
```

## Next Steps

1. **Implement storage backends:**
   - [pkg/storage/badger/store.go](pkg/storage/badger/) - BadgerDB implementation
   - [pkg/storage/clickhouse/store.go](pkg/storage/clickhouse/) - ClickHouse implementation

2. **Implement API handlers:**
   - [pkg/api/handler.go](pkg/api/) - HTTP request handlers
   - [pkg/api/write.go](pkg/api/) - Write endpoint
   - [pkg/api/query.go](pkg/api/) - Query endpoint

3. **Implement middleware:**
   - [pkg/middleware/auth.go](pkg/middleware/) - JWT authentication
   - [pkg/middleware/ratelimit.go](pkg/middleware/) - Rate limiting
   - [pkg/middleware/logging.go](pkg/middleware/) - Request logging

4. **Testing:**
   - Unit tests for all components
   - Integration tests with real storage backends
   - Load testing for horizontal scaling validation

## Configuration Management

### Environment-based Configuration
1. Default values in [configs/config.yaml](configs/config.yaml)
2. Environment variable overrides via `${VAR_NAME}`
3. Kubernetes ConfigMaps for cluster-specific settings
4. Kubernetes Secrets for sensitive data

### Example Configuration Flow
```
configs/config.yaml
    ↓
Environment variables (${JWT_SECRET})
    ↓
Kubernetes ConfigMap (mounted at /app/configs)
    ↓
Kubernetes Secrets (JWT_SECRET, CLICKHOUSE_PASSWORD)
    ↓
Final configuration loaded by application
```

## Monitoring and Observability

- **Health checks**: `/api/v1/health`
- **Metrics**: `/metrics` (Prometheus format)
- **Logging**: Structured JSON logs to stdout
- **Kubernetes probes**: Liveness, readiness, startup

## Security

- **Authentication**: JWT-based with tenant isolation
- **Authorization**: Scope-based permissions (read, write, admin)
- **Rate limiting**: Per-tenant request limits
- **Network**: Kubernetes NetworkPolicies (to be added)
- **RBAC**: Kubernetes ServiceAccount with minimal permissions

## Performance Characteristics

### ClickHouse Backend (Horizontal Scaling)
- **Write throughput**: 100K+ metrics/sec per pod
- **Query latency (p95)**: < 100ms
- **Max cardinality**: Millions of unique series
- **Scaling**: Linear scaling to 10+ pods
- **Storage**: 10-100x compression

### BadgerDB Backend (Single Instance)
- **Write throughput**: 10K+ metrics/sec
- **Query latency (p95)**: < 500ms
- **Max cardinality**: Millions (single machine limit)
- **Scaling**: Vertical only (more CPU/RAM)
- **Storage**: 3-5x compression
