# WS10: Production Deployment

**Priority:** P1 (High Priority)
**Estimated Effort:** 4-5 days
**Dependencies:** WS6 (ClickHouse Backend), WS9 (Testing Framework)

## Overview

Prepare Luminate for production deployment with ClickHouse cluster setup, Kubernetes configuration tuning, monitoring infrastructure, alerting rules, backup/restore procedures, and operational runbooks.

## Objectives

1. Deploy production-grade ClickHouse cluster
2. Optimize Kubernetes manifests for production
3. Implement secrets management
4. Set up comprehensive monitoring (Prometheus + Grafana)
5. Configure alerting rules and escalation
6. Establish backup and restore procedures
7. Create operational runbooks
8. Conduct load testing in staging environment

## Work Items

### 1. ClickHouse Cluster Setup

#### 1.1 ClickHouse Cluster Configuration

**File:** `deployments/clickhouse/config.xml`

```xml
<?xml version="1.0"?>
<clickhouse>
    <!-- Cluster configuration -->
    <remote_servers>
        <luminate_cluster>
            <!-- Shard 1 -->
            <shard>
                <replica>
                    <host>clickhouse-0.clickhouse-headless</host>
                    <port>9000</port>
                </replica>
                <replica>
                    <host>clickhouse-1.clickhouse-headless</host>
                    <port>9000</port>
                </replica>
            </shard>
            <!-- Shard 2 -->
            <shard>
                <replica>
                    <host>clickhouse-2.clickhouse-headless</host>
                    <port>9000</port>
                </replica>
                <replica>
                    <host>clickhouse-3.clickhouse-headless</host>
                    <port>9000</port>
                </replica>
            </shard>
        </luminate_cluster>
    </remote_servers>

    <!-- Zookeeper for replication -->
    <zookeeper>
        <node>
            <host>zookeeper-0.zookeeper-headless</host>
            <port>2181</port>
        </node>
        <node>
            <host>zookeeper-1.zookeeper-headless</host>
            <port>2181</port>
        </node>
        <node>
            <host>zookeeper-2.zookeeper-headless</host>
            <port>2181</port>
        </node>
    </zookeeper>

    <!-- Macros for distributed DDL -->
    <macros>
        <shard from_env="SHARD_ID"/>
        <replica from_env="REPLICA_ID"/>
    </macros>

    <!-- Performance settings -->
    <max_connections>1000</max_connections>
    <max_concurrent_queries>500</max_concurrent_queries>
    <max_table_size_to_drop>0</max_table_size_to_drop>

    <!-- Storage configuration -->
    <path>/var/lib/clickhouse/</path>
    <tmp_path>/var/lib/clickhouse/tmp/</tmp_path>

    <!-- Logging -->
    <logger>
        <level>information</level>
        <log>/var/log/clickhouse-server/clickhouse-server.log</log>
        <errorlog>/var/log/clickhouse-server/clickhouse-server.err.log</errorlog>
        <size>1000M</size>
        <count>10</count>
    </logger>

    <!-- Query log -->
    <query_log>
        <database>system</database>
        <table>query_log</table>
        <partition_by>toYYYYMM(event_date)</partition_by>
        <flush_interval_milliseconds>7500</flush_interval_milliseconds>
    </query_log>

    <!-- Metrics export for Prometheus -->
    <prometheus>
        <endpoint>/metrics</endpoint>
        <port>9363</port>
        <metrics>true</metrics>
        <events>true</events>
        <asynchronous_metrics>true</asynchronous_metrics>
    </prometheus>
</clickhouse>
```

#### 1.2 ClickHouse StatefulSet

**File:** `deployments/kubernetes/production/clickhouse-statefulset.yaml`

```yaml
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: clickhouse
  namespace: luminate-prod
spec:
  serviceName: clickhouse-headless
  replicas: 4  # 2 shards x 2 replicas
  selector:
    matchLabels:
      app: clickhouse
  template:
    metadata:
      labels:
        app: clickhouse
    spec:
      affinity:
        podAntiAffinity:
          requiredDuringSchedulingIgnoredDuringExecution:
          - labelSelector:
              matchExpressions:
              - key: app
                operator: In
                values:
                - clickhouse
            topologyKey: "kubernetes.io/hostname"

      containers:
      - name: clickhouse
        image: clickhouse/clickhouse-server:23.8
        ports:
        - containerPort: 9000
          name: native
        - containerPort: 8123
          name: http
        - containerPort: 9363
          name: metrics

        env:
        - name: SHARD_ID
          valueFrom:
            fieldRef:
              fieldPath: metadata.labels['statefulset.kubernetes.io/pod-name']
        - name: REPLICA_ID
          valueFrom:
            fieldRef:
              fieldPath: metadata.labels['statefulset.kubernetes.io/pod-name']

        volumeMounts:
        - name: data
          mountPath: /var/lib/clickhouse
        - name: config
          mountPath: /etc/clickhouse-server/config.d/
        - name: users
          mountPath: /etc/clickhouse-server/users.d/

        resources:
          requests:
            cpu: "2"
            memory: "8Gi"
          limits:
            cpu: "4"
            memory: "16Gi"

        livenessProbe:
          httpGet:
            path: /ping
            port: http
          initialDelaySeconds: 30
          periodSeconds: 10

        readinessProbe:
          httpGet:
            path: /ping
            port: http
          initialDelaySeconds: 10
          periodSeconds: 5

      volumes:
      - name: config
        configMap:
          name: clickhouse-config
      - name: users
        secret:
          secretName: clickhouse-users

  volumeClaimTemplates:
  - metadata:
      name: data
    spec:
      accessModes: [ "ReadWriteOnce" ]
      storageClassName: "fast-ssd"
      resources:
        requests:
          storage: 500Gi
```

#### 1.3 Zookeeper Deployment

**File:** `deployments/kubernetes/production/zookeeper-statefulset.yaml`

```yaml
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: zookeeper
  namespace: luminate-prod
spec:
  serviceName: zookeeper-headless
  replicas: 3
  selector:
    matchLabels:
      app: zookeeper
  template:
    metadata:
      labels:
        app: zookeeper
    spec:
      containers:
      - name: zookeeper
        image: zookeeper:3.8
        ports:
        - containerPort: 2181
          name: client
        - containerPort: 2888
          name: follower
        - containerPort: 3888
          name: election

        env:
        - name: ZOO_MY_ID
          valueFrom:
            fieldRef:
              fieldPath: metadata.labels['statefulset.kubernetes.io/pod-name']
        - name: ZOO_SERVERS
          value: "server.1=zookeeper-0.zookeeper-headless:2888:3888;2181 server.2=zookeeper-1.zookeeper-headless:2888:3888;2181 server.3=zookeeper-2.zookeeper-headless:2888:3888;2181"

        volumeMounts:
        - name: data
          mountPath: /data

        resources:
          requests:
            cpu: "500m"
            memory: "2Gi"
          limits:
            cpu: "1"
            memory: "4Gi"

  volumeClaimTemplates:
  - metadata:
      name: data
    spec:
      accessModes: [ "ReadWriteOnce" ]
      resources:
        requests:
          storage: 50Gi
```

### 2. Production Luminate Deployment

#### 2.1 Optimized Deployment Manifest

**File:** `deployments/kubernetes/production/luminate-deployment.yaml`

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: luminate
  namespace: luminate-prod
  labels:
    app: luminate
    version: v1.0.0
spec:
  replicas: 5
  strategy:
    type: RollingUpdate
    rollingUpdate:
      maxSurge: 1
      maxUnavailable: 0  # Zero-downtime deployments

  selector:
    matchLabels:
      app: luminate

  template:
    metadata:
      labels:
        app: luminate
        version: v1.0.0
      annotations:
        prometheus.io/scrape: "true"
        prometheus.io/port: "8080"
        prometheus.io/path: "/metrics"

    spec:
      # Anti-affinity: spread pods across nodes
      affinity:
        podAntiAffinity:
          preferredDuringSchedulingIgnoredDuringExecution:
          - weight: 100
            podAffinityTerm:
              labelSelector:
                matchExpressions:
                - key: app
                  operator: In
                  values:
                  - luminate
              topologyKey: "kubernetes.io/hostname"

      # Use service account for RBAC
      serviceAccountName: luminate

      # Security context
      securityContext:
        runAsNonRoot: true
        runAsUser: 1000
        fsGroup: 1000

      containers:
      - name: luminate
        image: ghcr.io/yourusername/luminate:v1.0.0
        imagePullPolicy: IfNotPresent

        ports:
        - containerPort: 8080
          name: http
          protocol: TCP

        env:
        # Storage backend
        - name: LUMINATE_STORAGE_BACKEND
          value: "clickhouse"

        # ClickHouse connection
        - name: LUMINATE_CLICKHOUSE_HOSTS
          value: "clickhouse-0.clickhouse-headless:9000,clickhouse-1.clickhouse-headless:9000,clickhouse-2.clickhouse-headless:9000,clickhouse-3.clickhouse-headless:9000"

        - name: LUMINATE_CLICKHOUSE_DATABASE
          value: "luminate"

        - name: LUMINATE_CLICKHOUSE_USERNAME
          valueFrom:
            secretKeyRef:
              name: clickhouse-credentials
              key: username

        - name: LUMINATE_CLICKHOUSE_PASSWORD
          valueFrom:
            secretKeyRef:
              name: clickhouse-credentials
              key: password

        # JWT secret
        - name: JWT_SECRET
          valueFrom:
            secretKeyRef:
              name: luminate-secrets
              key: jwt-secret

        # Rate limits
        - name: LUMINATE_RATELIMIT_WRITE_RPS
          value: "100"
        - name: LUMINATE_RATELIMIT_QUERY_RPS
          value: "50"
        - name: LUMINATE_RATELIMIT_METRICS_PER_SEC
          value: "10000"

        # Resource limits
        resources:
          requests:
            cpu: "1"
            memory: "2Gi"
          limits:
            cpu: "2"
            memory: "4Gi"

        # Health probes
        livenessProbe:
          httpGet:
            path: /healthz
            port: http
          initialDelaySeconds: 15
          periodSeconds: 10
          timeoutSeconds: 5
          failureThreshold: 3

        readinessProbe:
          httpGet:
            path: /readyz
            port: http
          initialDelaySeconds: 10
          periodSeconds: 5
          timeoutSeconds: 3
          failureThreshold: 2

        startupProbe:
          httpGet:
            path: /startupz
            port: http
          initialDelaySeconds: 0
          periodSeconds: 5
          timeoutSeconds: 3
          failureThreshold: 12

        # Graceful shutdown
        lifecycle:
          preStop:
            exec:
              command: ["/bin/sh", "-c", "sleep 15"]

---
apiVersion: v1
kind: Service
metadata:
  name: luminate-svc
  namespace: luminate-prod
spec:
  type: LoadBalancer
  selector:
    app: luminate
  ports:
  - port: 80
    targetPort: 8080
    protocol: TCP
    name: http
  sessionAffinity: None

---
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: luminate-hpa
  namespace: luminate-prod
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: luminate
  minReplicas: 5
  maxReplicas: 20
  metrics:
  - type: Resource
    resource:
      name: cpu
      target:
        type: Utilization
        averageUtilization: 70
  - type: Resource
    resource:
      name: memory
      target:
        type: Utilization
        averageUtilization: 80
  behavior:
    scaleDown:
      stabilizationWindowSeconds: 300
      policies:
      - type: Percent
        value: 50
        periodSeconds: 60
    scaleUp:
      stabilizationWindowSeconds: 60
      policies:
      - type: Percent
        value: 100
        periodSeconds: 60
```

### 3. Secrets Management

#### 3.1 Sealed Secrets (GitOps-friendly)

**File:** `deployments/kubernetes/production/secrets/luminate-sealed-secret.yaml`

```yaml
apiVersion: bitnami.com/v1alpha1
kind: SealedSecret
metadata:
  name: luminate-secrets
  namespace: luminate-prod
spec:
  encryptedData:
    jwt-secret: AgBXXXXXXXXXXXXXXXX  # Encrypted with kubeseal
```

**File:** `deployments/kubernetes/production/secrets/clickhouse-sealed-secret.yaml`

```yaml
apiVersion: bitnami.com/v1alpha1
kind: SealedSecret
metadata:
  name: clickhouse-credentials
  namespace: luminate-prod
spec:
  encryptedData:
    username: AgBXXXXXXXXXXXXXXXX
    password: AgBYYYYYYYYYYYYYYYY
```

#### 3.2 External Secrets Operator (AWS Secrets Manager)

**File:** `deployments/kubernetes/production/external-secrets.yaml`

```yaml
apiVersion: external-secrets.io/v1beta1
kind: SecretStore
metadata:
  name: aws-secretsmanager
  namespace: luminate-prod
spec:
  provider:
    aws:
      service: SecretsManager
      region: us-east-1
      auth:
        jwt:
          serviceAccountRef:
            name: luminate

---
apiVersion: external-secrets.io/v1beta1
kind: ExternalSecret
metadata:
  name: luminate-secrets
  namespace: luminate-prod
spec:
  refreshInterval: 1h
  secretStoreRef:
    name: aws-secretsmanager
    kind: SecretStore

  target:
    name: luminate-secrets
    creationPolicy: Owner

  data:
  - secretKey: jwt-secret
    remoteRef:
      key: luminate/jwt-secret

  - secretKey: clickhouse-password
    remoteRef:
      key: luminate/clickhouse-password
```

### 4. Monitoring Setup

#### 4.1 Prometheus Configuration

**File:** `deployments/prometheus/prometheus-config.yaml`

```yaml
global:
  scrape_interval: 15s
  evaluation_interval: 15s

scrape_configs:
  # Luminate pods
  - job_name: 'luminate'
    kubernetes_sd_configs:
    - role: pod
      namespaces:
        names:
        - luminate-prod
    relabel_configs:
    - source_labels: [__meta_kubernetes_pod_annotation_prometheus_io_scrape]
      action: keep
      regex: true
    - source_labels: [__meta_kubernetes_pod_annotation_prometheus_io_path]
      action: replace
      target_label: __metrics_path__
      regex: (.+)
    - source_labels: [__address__, __meta_kubernetes_pod_annotation_prometheus_io_port]
      action: replace
      regex: ([^:]+)(?::\d+)?;(\d+)
      replacement: $1:$2
      target_label: __address__

  # ClickHouse metrics
  - job_name: 'clickhouse'
    static_configs:
    - targets:
      - clickhouse-0.clickhouse-headless:9363
      - clickhouse-1.clickhouse-headless:9363
      - clickhouse-2.clickhouse-headless:9363
      - clickhouse-3.clickhouse-headless:9363

  # Kubernetes node metrics
  - job_name: 'kubernetes-nodes'
    kubernetes_sd_configs:
    - role: node
    relabel_configs:
    - action: labelmap
      regex: __meta_kubernetes_node_label_(.+)
```

#### 4.2 Alerting Rules

**File:** `deployments/prometheus/alerts/luminate-alerts.yaml`

```yaml
groups:
- name: luminate
  interval: 30s
  rules:

  # High error rate
  - alert: LuminateHighErrorRate
    expr: |
      rate(luminate_http_requests_total{status=~"5.."}[5m]) > 0.05
    for: 5m
    labels:
      severity: critical
      component: luminate
    annotations:
      summary: "High error rate detected"
      description: "Error rate is {{ $value | humanizePercentage }} (threshold: 5%)"
      runbook_url: https://wiki.company.com/runbooks/luminate-high-error-rate

  # High write latency
  - alert: LuminateHighWriteLatency
    expr: |
      histogram_quantile(0.95,
        rate(luminate_storage_write_duration_seconds_bucket[5m])
      ) > 0.1
    for: 10m
    labels:
      severity: warning
      component: luminate
    annotations:
      summary: "Storage write latency is high"
      description: "p95 write latency is {{ $value }}s (threshold: 100ms)"
      runbook_url: https://wiki.company.com/runbooks/luminate-high-latency

  # High query latency
  - alert: LuminateHighQueryLatency
    expr: |
      histogram_quantile(0.95,
        rate(luminate_storage_query_duration_seconds_bucket[5m])
      ) > 0.5
    for: 10m
    labels:
      severity: warning
      component: luminate
    annotations:
      summary: "Query latency is high"
      description: "p95 query latency is {{ $value }}s (threshold: 500ms)"

  # Rate limit rejections
  - alert: LuminateHighRateLimitRejections
    expr: |
      rate(luminate_rate_limit_rejected_total[5m]) > 10
    for: 5m
    labels:
      severity: warning
      component: luminate
    annotations:
      summary: "High rate limit rejection rate"
      description: "Tenant {{ $labels.tenant_id }} is being rate limited at {{ $value }} req/s"

  # Storage unhealthy
  - alert: LuminateStorageUnhealthy
    expr: |
      up{job="luminate"} == 0
    for: 2m
    labels:
      severity: critical
      component: storage
    annotations:
      summary: "Storage backend is unhealthy"
      description: "Storage health check failing for {{ $labels.instance }}"
      runbook_url: https://wiki.company.com/runbooks/luminate-storage-down

  # ClickHouse alerts
  - alert: ClickHouseHighCPU
    expr: |
      rate(process_cpu_seconds_total{job="clickhouse"}[5m]) > 0.8
    for: 10m
    labels:
      severity: warning
      component: clickhouse
    annotations:
      summary: "ClickHouse CPU usage is high"
      description: "CPU usage is {{ $value | humanizePercentage }} on {{ $labels.instance }}"

  - alert: ClickHouseDiskSpaceLow
    expr: |
      (node_filesystem_avail_bytes{mountpoint="/var/lib/clickhouse"} /
       node_filesystem_size_bytes{mountpoint="/var/lib/clickhouse"}) < 0.2
    for: 5m
    labels:
      severity: critical
      component: clickhouse
    annotations:
      summary: "ClickHouse disk space low"
      description: "Only {{ $value | humanizePercentage }} disk space remaining"

  # Pod restarts
  - alert: LuminatePodRestarts
    expr: |
      rate(kube_pod_container_status_restarts_total{namespace="luminate-prod"}[15m]) > 0
    for: 5m
    labels:
      severity: warning
      component: kubernetes
    annotations:
      summary: "Pod is restarting"
      description: "Pod {{ $labels.pod }} is restarting frequently"
```

### 5. Backup and Restore

#### 5.1 ClickHouse Backup Script

**File:** `scripts/backup-clickhouse.sh`

```bash
#!/bin/bash

set -euo pipefail

# Configuration
CLICKHOUSE_HOST="${CLICKHOUSE_HOST:-clickhouse-0.clickhouse-headless}"
CLICKHOUSE_PORT="${CLICKHOUSE_PORT:-9000}"
BACKUP_BUCKET="${BACKUP_BUCKET:-s3://luminate-backups}"
BACKUP_RETENTION_DAYS="${BACKUP_RETENTION_DAYS:-30}"
TIMESTAMP=$(date +%Y%m%d_%H%M%S)

# Database to backup
DATABASE="luminate"
BACKUP_DIR="/tmp/clickhouse-backup-${TIMESTAMP}"

echo "[$(date)] Starting ClickHouse backup..."

# Create backup directory
mkdir -p "${BACKUP_DIR}"

# Freeze tables (creates hard links for backup)
clickhouse-client --host="${CLICKHOUSE_HOST}" --port="${CLICKHOUSE_PORT}" --query="
    SYSTEM FREEZE;
"

# Copy frozen data
rsync -av /var/lib/clickhouse/shadow/ "${BACKUP_DIR}/"

# Upload to S3
aws s3 sync "${BACKUP_DIR}" "${BACKUP_BUCKET}/${TIMESTAMP}/" \
    --storage-class INTELLIGENT_TIERING

# Cleanup local backup
rm -rf "${BACKUP_DIR}"

# Delete old backups (retention policy)
CUTOFF_DATE=$(date -d "${BACKUP_RETENTION_DAYS} days ago" +%Y%m%d)
aws s3 ls "${BACKUP_BUCKET}/" | awk '{print $2}' | while read -r backup_dir; do
    backup_date=$(echo "${backup_dir}" | cut -d_ -f1)
    if [[ "${backup_date}" < "${CUTOFF_DATE}" ]]; then
        echo "Deleting old backup: ${backup_dir}"
        aws s3 rm "${BACKUP_BUCKET}/${backup_dir}" --recursive
    fi
done

echo "[$(date)] Backup completed successfully"
```

#### 5.2 Restore Script

**File:** `scripts/restore-clickhouse.sh`

```bash
#!/bin/bash

set -euo pipefail

BACKUP_TIMESTAMP="${1:?Usage: $0 <backup_timestamp>}"
CLICKHOUSE_HOST="${CLICKHOUSE_HOST:-clickhouse-0.clickhouse-headless}"
BACKUP_BUCKET="${BACKUP_BUCKET:-s3://luminate-backups}"
RESTORE_DIR="/tmp/clickhouse-restore-${BACKUP_TIMESTAMP}"

echo "[$(date)] Starting restore from backup: ${BACKUP_TIMESTAMP}"

# Download backup from S3
mkdir -p "${RESTORE_DIR}"
aws s3 sync "${BACKUP_BUCKET}/${BACKUP_TIMESTAMP}/" "${RESTORE_DIR}/"

# Stop ClickHouse (if running locally)
# systemctl stop clickhouse-server

# Restore data
rsync -av "${RESTORE_DIR}/" /var/lib/clickhouse/

# Start ClickHouse
# systemctl start clickhouse-server

# Verify restore
clickhouse-client --host="${CLICKHOUSE_HOST}" --query="
    SELECT count() FROM luminate.metrics;
"

rm -rf "${RESTORE_DIR}"

echo "[$(date)] Restore completed successfully"
```

#### 5.3 Backup CronJob

**File:** `deployments/kubernetes/production/backup-cronjob.yaml`

```yaml
apiVersion: batch/v1
kind: CronJob
metadata:
  name: clickhouse-backup
  namespace: luminate-prod
spec:
  schedule: "0 2 * * *"  # Daily at 2 AM UTC
  concurrencyPolicy: Forbid
  successfulJobsHistoryLimit: 3
  failedJobsHistoryLimit: 3

  jobTemplate:
    spec:
      template:
        spec:
          serviceAccountName: backup-sa

          containers:
          - name: backup
            image: ghcr.io/yourusername/luminate-backup:latest
            command: ["/scripts/backup-clickhouse.sh"]

            env:
            - name: CLICKHOUSE_HOST
              value: "clickhouse-0.clickhouse-headless"
            - name: BACKUP_BUCKET
              value: "s3://luminate-backups"
            - name: BACKUP_RETENTION_DAYS
              value: "30"

            volumeMounts:
            - name: scripts
              mountPath: /scripts

          restartPolicy: OnFailure

          volumes:
          - name: scripts
            configMap:
              name: backup-scripts
              defaultMode: 0755
```

### 6. Operational Runbooks

#### 6.1 High Error Rate Runbook

**File:** `docs/runbooks/high-error-rate.md`

```markdown
# Runbook: High Error Rate

## Alert
- **Name**: LuminateHighErrorRate
- **Severity**: Critical
- **Threshold**: Error rate > 5% for 5 minutes

## Investigation Steps

### 1. Check Error Distribution
```bash
# Query Prometheus for error breakdown
kubectl port-forward -n luminate-prod svc/prometheus 9090:9090

# Visit http://localhost:9090 and run:
rate(luminate_http_requests_total{status=~"5.."}[5m]) by (status, path)
```

### 2. Check Recent Deployments
```bash
# List recent deployments
kubectl rollout history deployment/luminate -n luminate-prod

# Check pod status
kubectl get pods -n luminate-prod -l app=luminate
```

### 3. Check ClickHouse Health
```bash
# Test ClickHouse connectivity
kubectl exec -it clickhouse-0 -n luminate-prod -- \
    clickhouse-client --query="SELECT 1"

# Check ClickHouse logs
kubectl logs -n luminate-prod clickhouse-0 --tail=100
```

### 4. Check Application Logs
```bash
# View Luminate logs
kubectl logs -n luminate-prod -l app=luminate --tail=100 | grep ERROR

# Follow logs in real-time
kubectl logs -n luminate-prod -l app=luminate -f
```

## Mitigation Steps

### If caused by bad deployment:
```bash
# Rollback to previous version
kubectl rollout undo deployment/luminate -n luminate-prod

# Verify rollback
kubectl rollout status deployment/luminate -n luminate-prod
```

### If caused by ClickHouse issues:
```bash
# Restart ClickHouse pod
kubectl delete pod clickhouse-0 -n luminate-prod

# Wait for pod to be ready
kubectl wait --for=condition=ready pod/clickhouse-0 -n luminate-prod --timeout=300s
```

### If caused by resource exhaustion:
```bash
# Scale up deployment
kubectl scale deployment/luminate -n luminate-prod --replicas=10

# Monitor scaling progress
watch kubectl get pods -n luminate-prod -l app=luminate
```

## Escalation
- Escalate to on-call engineer if errors persist after mitigation
- Page database team if ClickHouse is unhealthy
- Create incident in PagerDuty/Opsgenie

## Post-Incident
- Document root cause
- Update monitoring thresholds if needed
- Implement preventive measures
```

### 7. Load Testing in Staging

#### 7.1 Staging Environment Setup

**File:** `deployments/kubernetes/staging/kustomization.yaml`

```yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization

namespace: luminate-staging

resources:
- ../production/clickhouse-statefulset.yaml
- ../production/luminate-deployment.yaml
- ../production/zookeeper-statefulset.yaml

patchesStrategicMerge:
- staging-patches.yaml

configMapGenerator:
- name: luminate-config
  literals:
  - ENVIRONMENT=staging

replicas:
- name: luminate
  count: 3  # Fewer replicas in staging
- name: clickhouse
  count: 2  # Smaller ClickHouse cluster
```

#### 7.2 Load Test Execution Script

**File:** `scripts/run-staging-load-test.sh`

```bash
#!/bin/bash

set -euo pipefail

STAGING_URL="${STAGING_URL:-https://luminate-staging.example.com}"
DURATION="${DURATION:-10m}"
VUS="${VUS:-200}"

echo "Running load test against ${STAGING_URL}"
echo "Duration: ${DURATION}, VUs: ${VUS}"

# Run write load test
echo "=== Write Load Test ==="
k6 run --env BASE_URL="${STAGING_URL}" \
    --vus "${VUS}" \
    --duration "${DURATION}" \
    --out cloud \
    test/load/write_load.js

# Run query load test
echo "=== Query Load Test ==="
k6 run --env BASE_URL="${STAGING_URL}" \
    --vus $((VUS / 2)) \
    --duration "${DURATION}" \
    --out cloud \
    test/load/query_load.js

# Check for regressions
echo "=== Checking for performance regressions ==="
k6 run --env BASE_URL="${STAGING_URL}" \
    --vus "${VUS}" \
    --duration "5m" \
    --out cloud \
    --thresholds="http_req_duration{p(95)}<100" \
    test/load/write_load.js

if [ $? -ne 0 ]; then
    echo "ERROR: Performance regression detected!"
    exit 1
fi

echo "Load test completed successfully"
```

## Acceptance Criteria

- [ ] ClickHouse cluster deployed with 4 nodes (2 shards x 2 replicas)
- [ ] Zookeeper ensemble running with 3 nodes
- [ ] Luminate deployment configured for production (5-20 replicas)
- [ ] Secrets managed via Sealed Secrets or External Secrets Operator
- [ ] Prometheus scraping all components
- [ ] Alerting rules configured and tested
- [ ] Daily automated backups to S3
- [ ] Backup retention policy enforced (30 days)
- [ ] Restore procedure tested and documented
- [ ] Runbooks created for common incidents
- [ ] Load testing passed in staging (100K+ writes/sec, p95 < 100ms)
- [ ] Zero-downtime deployment verified
- [ ] HPA configured and tested

## Deployment Checklist

### Pre-Deployment
- [ ] Review all configuration changes
- [ ] Test in staging environment
- [ ] Notify stakeholders of deployment window
- [ ] Verify backup is recent and valid
- [ ] Check monitoring dashboards are accessible

### Deployment
- [ ] Deploy ClickHouse cluster
- [ ] Deploy Zookeeper ensemble
- [ ] Create database schema
- [ ] Deploy Luminate application
- [ ] Verify health checks pass
- [ ] Run smoke tests
- [ ] Monitor error rates and latency

### Post-Deployment
- [ ] Verify metrics are being collected
- [ ] Test sample write and query requests
- [ ] Confirm alerts are working
- [ ] Update documentation
- [ ] Schedule load testing
- [ ] Conduct post-deployment review

## Summary

This workstream prepares Luminate for production deployment with enterprise-grade infrastructure:
- High-availability ClickHouse cluster with replication
- Secure secrets management
- Comprehensive monitoring and alerting
- Automated backups with retention policies
- Operational runbooks for incident response
- Load testing validation in staging

**Total Estimated Effort:** 4-5 days

**Dependencies:** WS6 (ClickHouse Backend), WS9 (Testing Framework)

**Deliverables:**
- Production ClickHouse cluster configuration
- Optimized Kubernetes manifests
- Secrets management setup
- Prometheus + Grafana monitoring stack
- Alerting rules and escalation policies
- Backup/restore automation
- Operational runbooks
- Staging load test validation
