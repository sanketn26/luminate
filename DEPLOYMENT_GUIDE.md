# Luminate Deployment Guide

## Overview

Luminate supports two deployment strategies:

1. **Horizontally Scalable (ClickHouse)** - Production-ready, auto-scaling
2. **Single Instance (BadgerDB)** - Development, edge, embedded scenarios

## Horizontal Scaling Architecture (ClickHouse)

### Key Components

```
┌──────────────────────────────────────────────────────┐
│                  Load Balancer                       │
│              (Kubernetes Ingress)                    │
└──────────────────────────────────────────────────────┘
                       ↓
┌──────────────────────────────────────────────────────┐
│          Kubernetes Service (ClusterIP)              │
│          Round-robin load balancing                  │
└──────────────────────────────────────────────────────┘
                       ↓
┌──────────────────────────────────────────────────────┐
│        Luminate Pods (3-10 replicas)                 │
│  ┌─────────┐ ┌─────────┐ ┌─────────┐ ┌─────────┐   │
│  │  Pod 1  │ │  Pod 2  │ │  Pod 3  │ │  Pod N  │   │
│  │ 250m CPU│ │ 250m CPU│ │ 250m CPU│ │ 250m CPU│   │
│  │ 512Mi   │ │ 512Mi   │ │ 512Mi   │ │ 512Mi   │   │
│  └─────────┘ └─────────┘ └─────────┘ └─────────┘   │
│                                                      │
│  • Stateless (no local data)                        │
│  • Auto-scaling via HPA                             │
│  • Anti-affinity (spread across nodes)             │
│  • Graceful shutdown (30s)                          │
└──────────────────────────────────────────────────────┘
                       ↓
┌──────────────────────────────────────────────────────┐
│          ClickHouse Cluster                          │
│          (External or managed service)               │
│                                                      │
│  • Columnar storage                                 │
│  • High-cardinality support                         │
│  • 10-100x compression                              │
│  • Billions of metrics                              │
└──────────────────────────────────────────────────────┘
```

## Quick Start: Horizontally Scalable Deployment

### Prerequisites

- Kubernetes cluster (1.21+)
- kubectl configured
- ClickHouse instance or managed service
- Container registry access

### Step 1: Build and Push Image

```bash
# Set your registry
export DOCKER_REGISTRY=docker.io
export DOCKER_REPO=yourusername

# Build and push
make docker-push
```

### Step 2: Setup ClickHouse

**Option A: Managed Service (Recommended)**
- Use ClickHouse Cloud, Altinity.Cloud, or AWS ClickHouse
- Note the connection details (host, port, credentials)

**Option B: Self-Hosted**
```bash
# Run locally for testing
make clickhouse-local

# Setup schema
make clickhouse-setup
```

### Step 3: Configure Kubernetes Secrets

Edit [deployments/kubernetes/secret.yaml](deployments/kubernetes/secret.yaml):

```yaml
stringData:
  jwt-secret: "your-strong-secret-here"
  clickhouse-password: "your-clickhouse-password"
```

**IMPORTANT**: Use strong, randomly generated secrets in production!

### Step 4: Update ConfigMap

Edit [deployments/kubernetes/configmap.yaml](deployments/kubernetes/configmap.yaml):

```yaml
storage:
  backend: "clickhouse"
  clickhouse:
    addresses: ["your-clickhouse-host:9000"]
    database: "luminate"
    username: "your-username"
```

### Step 5: Deploy

```bash
# Deploy everything
make k8s-deploy

# This creates:
# - Namespace: luminate
# - ConfigMap: luminate-config
# - Secret: luminate-secrets
# - Deployment: luminate (3 replicas initially)
# - Service: luminate-svc
# - HPA: luminate-hpa (auto-scaling)
```

### Step 6: Verify Deployment

```bash
# Check status
make k8s-status

# Expected output:
# NAME                       READY   STATUS    RESTARTS   AGE
# pod/luminate-xxx-yyy       1/1     Running   0          1m
# pod/luminate-xxx-zzz       1/1     Running   0          1m
# pod/luminate-xxx-www       1/1     Running   0          1m

# Check HPA
kubectl get hpa -n luminate

# Expected output:
# NAME           REFERENCE             TARGETS         MINPODS   MAXPODS   REPLICAS
# luminate-hpa   Deployment/luminate   20%/70%, 30%/80%   3         10        3

# View logs
make k8s-logs
```

### Step 7: Expose Service

**Option A: Ingress (Recommended)**

Edit [deployments/kubernetes/ingress.yaml](deployments/kubernetes/ingress.yaml):

```yaml
spec:
  tls:
  - hosts:
    - luminate.yourdomain.com
  rules:
  - host: luminate.yourdomain.com
```

Deploy ingress:
```bash
kubectl apply -f deployments/kubernetes/ingress.yaml
```

**Option B: LoadBalancer**

```bash
kubectl patch svc luminate-svc -n luminate -p '{"spec":{"type":"LoadBalancer"}}'
kubectl get svc luminate-svc -n luminate
```

## Auto-Scaling Configuration

The HPA configuration is in [deployments/kubernetes/hpa.yaml](deployments/kubernetes/hpa.yaml):

```yaml
spec:
  minReplicas: 3      # Always run at least 3 pods
  maxReplicas: 10     # Maximum 10 pods

  metrics:
  - type: Resource
    resource:
      name: cpu
      target:
        averageUtilization: 70    # Scale up at 70% CPU

  - type: Resource
    resource:
      name: memory
      target:
        averageUtilization: 80    # Scale up at 80% memory
```

### Scaling Behavior

**Scale Up:**
- Triggered when CPU > 70% or memory > 80%
- Immediate response (0s stabilization)
- Max increase: 100% (double) or 2 pods per 15 seconds
- Example: 3 → 6 → 10 pods in under 1 minute

**Scale Down:**
- Triggered when metrics below threshold
- Wait 5 minutes before scaling down (prevents flapping)
- Max decrease: 50% or 1 pod per 60 seconds
- Example: 10 → 9 → 8 → 7... (gradual)

### Testing Auto-Scaling

Generate load to trigger scaling:

```bash
# Install hey (HTTP load generator)
go install github.com/rakyll/hey@latest

# Generate load
hey -z 5m -c 50 -q 100 \
  -H "Authorization: Bearer your-jwt-token" \
  -m POST \
  -D test-metrics.json \
  http://luminate.yourdomain.com/api/v1/write

# Watch scaling in real-time
watch kubectl get hpa,pods -n luminate
```

## Resource Planning

### Per-Pod Resources

```yaml
requests:           # Guaranteed resources
  cpu: 250m         # 0.25 cores
  memory: 512Mi     # 512 MiB

limits:             # Maximum allowed
  cpu: 1000m        # 1 core
  memory: 2Gi       # 2 GiB
```

### Cluster Sizing

**Small Deployment (< 100K metrics/sec)**
- Pods: 3 replicas
- Total: 750m CPU, 1.5Gi memory
- Node: 2 cores, 4Gi memory per node

**Medium Deployment (100K - 500K metrics/sec)**
- Pods: 5-7 replicas
- Total: 1.25-1.75 cores, 2.5-3.5Gi memory
- Node: 4 cores, 8Gi memory per node

**Large Deployment (> 500K metrics/sec)**
- Pods: 8-10 replicas
- Total: 2-2.5 cores, 4-5Gi memory
- Node: 8 cores, 16Gi memory per node

## High Availability Features

### 1. Pod Anti-Affinity

Pods prefer to run on different nodes:

```yaml
affinity:
  podAntiAffinity:
    preferredDuringSchedulingIgnoredDuringExecution:
    - weight: 100
      podAffinityTerm:
        topologyKey: kubernetes.io/hostname
```

Result: Spread across availability zones for fault tolerance

### 2. Health Checks

**Liveness Probe** (restart if unhealthy):
```yaml
livenessProbe:
  httpGet:
    path: /api/v1/health
    port: 8080
  initialDelaySeconds: 30
  periodSeconds: 10
```

**Readiness Probe** (remove from load balancer if not ready):
```yaml
readinessProbe:
  httpGet:
    path: /api/v1/health
    port: 8080
  initialDelaySeconds: 10
  periodSeconds: 5
```

**Startup Probe** (allow slow startup):
```yaml
startupProbe:
  httpGet:
    path: /api/v1/health
    port: 8080
  failureThreshold: 30
  periodSeconds: 5
```

### 3. Graceful Shutdown

```yaml
lifecycle:
  preStop:
    exec:
      command: ["/bin/sh", "-c", "sleep 15"]

terminationGracePeriodSeconds: 30
```

Flow:
1. Pod receives SIGTERM
2. PreStop hook: sleep 15s (allow load balancer to update)
3. Application handles in-flight requests (up to 30s total)
4. Pod terminates

### 4. Rolling Updates

```yaml
strategy:
  type: RollingUpdate
  rollingUpdate:
    maxSurge: 1           # Max 1 extra pod during update
    maxUnavailable: 0     # Always keep all pods available
```

Result: Zero-downtime deployments

## Monitoring

### Prometheus Metrics

All pods expose metrics at `/metrics`:

```bash
# Port-forward to a pod
kubectl port-forward -n luminate pod/luminate-xxx-yyy 8080:8080

# Scrape metrics
curl http://localhost:8080/metrics
```

**Key metrics:**
- `luminate_write_requests_total` - Total write requests
- `luminate_query_requests_total` - Total queries
- `luminate_query_latency_seconds` - Query latency histogram
- `luminate_rate_limit_exceeded_total` - Rate limit hits

### Pod Annotations

Pods are annotated for Prometheus auto-discovery:

```yaml
annotations:
  prometheus.io/scrape: "true"
  prometheus.io/port: "8080"
  prometheus.io/path: "/metrics"
```

## Troubleshooting

### Pods Not Starting

```bash
# Check pod status
kubectl get pods -n luminate

# View events
kubectl describe pod luminate-xxx-yyy -n luminate

# Check logs
kubectl logs luminate-xxx-yyy -n luminate

# Common issues:
# - ImagePullBackOff: Wrong image name or registry credentials
# - CrashLoopBackOff: Check logs for startup errors
# - Pending: Insufficient resources on cluster
```

### HPA Not Scaling

```bash
# Check HPA status
kubectl describe hpa luminate-hpa -n luminate

# Check metrics server
kubectl top pods -n luminate

# Common issues:
# - Metrics server not installed
# - Resource requests not set
# - Insufficient cluster capacity
```

### Can't Connect to ClickHouse

```bash
# Check connectivity from pod
kubectl exec -it luminate-xxx-yyy -n luminate -- /bin/sh
wget -O- http://clickhouse-host:8123/ping

# Check credentials
kubectl get secret luminate-secrets -n luminate -o yaml

# Common issues:
# - Wrong ClickHouse address in ConfigMap
# - Invalid credentials in Secret
# - Network policy blocking access
```

## Comparison: ClickHouse vs BadgerDB

| Feature | ClickHouse (Scalable) | BadgerDB (Single) |
|---------|----------------------|-------------------|
| Replicas | 3-10 (auto-scaling) | 1 (fixed) |
| Stateless | Yes | No (StatefulSet) |
| Storage | External ClickHouse | Local SSD (100GB) |
| Write throughput | 100K+/sec per pod | 10K+/sec total |
| Cardinality | Billions | Millions |
| Ops complexity | Medium (manage ClickHouse) | Low (embedded) |
| Best for | Production, high scale | Dev, edge, embedded |

## Next Steps

1. **Setup monitoring:**
   - Configure Prometheus to scrape metrics
   - Create Grafana dashboards
   - Setup alerts for SLOs

2. **Test scaling:**
   - Generate load to test HPA
   - Verify zero-downtime rolling updates
   - Test pod failure recovery

3. **Production hardening:**
   - Setup NetworkPolicies
   - Configure PodSecurityPolicies
   - Enable audit logging
   - Setup backup/restore for ClickHouse

4. **Implement application:**
   - Complete storage backend implementations
   - Add API handlers
   - Write tests
   - Performance tuning

## Resources

- [Kubernetes HPA Documentation](https://kubernetes.io/docs/tasks/run-application/horizontal-pod-autoscale/)
- [ClickHouse Documentation](https://clickhouse.com/docs)
- [Prometheus Operator](https://github.com/prometheus-operator/prometheus-operator)
