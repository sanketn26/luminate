# Quick Start Guide

Get Luminate up and running in 5 minutes with zero dependencies.

## Prerequisites

- Go 1.21 or higher
- make (optional, but recommended)

No Docker, no ClickHouse, no external services required!

## Step 1: Clone and Build

```bash
# Clone the repository
git clone https://github.com/yourusername/luminate.git
cd luminate

# Build
make build-local

# Or without make:
go build -o luminate ./cmd/luminate
```

## Step 2: Start Luminate

```bash
# Start with BadgerDB (embedded, zero dependencies)
./luminate

# Or with make:
make run
```

You should see:
```
2024-12-09T10:00:00Z INFO Starting Luminate
2024-12-09T10:00:00Z INFO Storage backend: badger
2024-12-09T10:00:00Z INFO Server listening on :8080
```

## Step 3: Write Your First Metric

```bash
# Write a metric
curl -X POST http://localhost:8080/api/v1/write \
  -H "Content-Type: application/json" \
  -d '{
    "metrics": [
      {
        "name": "my_first_metric",
        "value": 42.5,
        "timestamp": '$(date +%s)',
        "dimensions": {
          "environment": "dev",
          "service": "quickstart"
        }
      }
    ]
  }'
```

Response:
```json
{
  "accepted": 1,
  "rejected": 0,
  "errors": []
}
```

## Step 4: Query the Metric

### Get raw data points

```bash
curl -X POST http://localhost:8080/api/v1/query \
  -H "Content-Type: application/json" \
  -d '{
    "queryType": "range",
    "metricName": "my_first_metric",
    "timeRange": {
      "relative": "1h"
    }
  }'
```

Response:
```json
{
  "results": [
    {
      "timestamp": "2024-12-09T10:00:00Z",
      "value": 42.5,
      "dimensions": {
        "environment": "dev",
        "service": "quickstart"
      }
    }
  ]
}
```

### Aggregate data

```bash
curl -X POST http://localhost:8080/api/v1/query \
  -H "Content-Type: application/json" \
  -d '{
    "queryType": "aggregate",
    "metricName": "my_first_metric",
    "aggregation": "avg",
    "timeRange": {
      "relative": "1h"
    },
    "groupBy": ["environment"]
  }'
```

Response:
```json
{
  "results": [
    {
      "dimensions": {
        "environment": "dev"
      },
      "value": 42.5
    }
  ]
}
```

## Step 5: Discover Metrics

### List all metrics

```bash
curl http://localhost:8080/api/v1/metrics
```

Response:
```json
{
  "metrics": ["my_first_metric"],
  "count": 1
}
```

### List dimensions for a metric

```bash
curl http://localhost:8080/api/v1/metrics/my_first_metric/dimensions
```

Response:
```json
{
  "metricName": "my_first_metric",
  "dimensions": ["environment", "service"],
  "count": 2
}
```

## Step 6: Check Health

```bash
curl http://localhost:8080/api/v1/health
```

Response:
```json
{
  "status": "healthy",
  "components": [
    {
      "name": "storage",
      "status": "healthy",
      "message": "Storage backend is healthy",
      "timestamp": "2024-12-09T10:00:00Z",
      "duration_ms": 2
    }
  ],
  "timestamp": "2024-12-09T10:00:00Z"
}
```

## Example: Tracking API Latency

Let's track API latency for multiple endpoints:

```bash
# Write latency metrics for different endpoints
curl -X POST http://localhost:8080/api/v1/write \
  -H "Content-Type: application/json" \
  -d '{
    "metrics": [
      {
        "name": "api_latency",
        "value": 0.145,
        "timestamp": '$(date +%s)',
        "dimensions": {
          "endpoint": "/api/users",
          "method": "GET",
          "status": "200"
        }
      },
      {
        "name": "api_latency",
        "value": 0.089,
        "timestamp": '$(date +%s)',
        "dimensions": {
          "endpoint": "/api/products",
          "method": "GET",
          "status": "200"
        }
      },
      {
        "name": "api_latency",
        "value": 0.523,
        "timestamp": '$(date +%s)',
        "dimensions": {
          "endpoint": "/api/orders",
          "method": "POST",
          "status": "201"
        }
      }
    ]
  }'
```

### Calculate p95 latency by endpoint

```bash
curl -X POST http://localhost:8080/api/v1/query \
  -H "Content-Type: application/json" \
  -d '{
    "queryType": "aggregate",
    "metricName": "api_latency",
    "aggregation": "p95",
    "timeRange": {
      "relative": "1h"
    },
    "groupBy": ["endpoint"]
  }'
```

Response:
```json
{
  "results": [
    {
      "dimensions": {"endpoint": "/api/users"},
      "value": 0.145
    },
    {
      "dimensions": {"endpoint": "/api/products"},
      "value": 0.089
    },
    {
      "dimensions": {"endpoint": "/api/orders"},
      "value": 0.523
    }
  ]
}
```

## Aggregation Types

Luminate supports 9 aggregation types:

```bash
# Average
"aggregation": "avg"

# Sum
"aggregation": "sum"

# Count
"aggregation": "count"

# Min/Max
"aggregation": "min"
"aggregation": "max"

# Percentiles
"aggregation": "p50"  # Median
"aggregation": "p95"
"aggregation": "p99"

# Time-weighted (for resources like CPU-seconds)
"aggregation": "integral"
```

## Time Range Formats

### Relative (recommended for most use cases)

```json
{
  "timeRange": {
    "relative": "1h"   // 1 hour ago to now
  }
}
```

Supported values: `1h`, `24h`, `7d`, `30d`

### Absolute

```json
{
  "timeRange": {
    "start": "2024-12-09T00:00:00Z",
    "end": "2024-12-09T23:59:59Z"
  }
}
```

## Configuration

Luminate uses sensible defaults. To customize, create `config.yaml`:

```yaml
server:
  port: 8080

storage:
  backend: "badger"  # or "clickhouse"
  badger:
    path: "./data"

# Authentication (disabled by default)
auth:
  enabled: false
  # jwt_secret: "your-secret-here"

# Rate limiting (permissive by default)
rate_limiting:
  enabled: true
  write_requests_per_sec: 1000
  query_requests_per_sec: 500
```

Start with custom config:
```bash
./luminate -config config.yaml
```

Or use environment variables:
```bash
LUMINATE_SERVER_PORT=9090 ./luminate
```

## What's Next?

### Development
- [Development Setup](development.md) - Set up for contributing
- [API Reference](api-reference.md) - Complete API documentation
- [Configuration Reference](configuration.md) - All config options

### Production
- [Production Deployment](../operations/deployment.md) - Deploy with ClickHouse
- [Monitoring Guide](../operations/monitoring.md) - Set up Prometheus + Grafana
- [Backup & Restore](../operations/backup-restore.md) - Data protection

### Advanced
- [Architecture Overview](../architecture/overview.md) - System design
- [Performance Tuning](performance-tuning.md) - Optimize for your workload

## Common Issues

### Port already in use
```bash
# Check what's using port 8080
lsof -i :8080

# Use a different port
LUMINATE_SERVER_PORT=9090 ./luminate
```

### Permission denied on data directory
```bash
# Create data directory
mkdir -p ./data
chmod 755 ./data
```

### Metric validation errors
Check that:
- Metric names match `^[a-zA-Z_][a-zA-Z0-9_]*$`
- Max 20 dimensions per metric
- Timestamps within [now - 7 days, now + 1 hour]
- Values are finite (no NaN or Inf)

## Getting Help

- Check [Troubleshooting Guide](troubleshooting.md)
- Review [API Reference](api-reference.md)
- Open an issue on GitHub

---

**Congratulations!** You've successfully:
- ✅ Started Luminate locally
- ✅ Written metrics
- ✅ Queried data
- ✅ Discovered metrics
- ✅ Checked system health

Next: Try [connecting Grafana](../workstreams/11-dashboard-ui.md) for visualization!
