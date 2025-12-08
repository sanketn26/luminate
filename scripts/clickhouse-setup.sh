#!/bin/bash
# ClickHouse schema setup script

set -e

CLICKHOUSE_HOST=${CLICKHOUSE_HOST:-localhost}
CLICKHOUSE_PORT=${CLICKHOUSE_PORT:-9000}
CLICKHOUSE_DB=${CLICKHOUSE_DB:-luminate}
CLICKHOUSE_USER=${CLICKHOUSE_USER:-default}
CLICKHOUSE_PASSWORD=${CLICKHOUSE_PASSWORD:-}

echo "Setting up ClickHouse schema..."

# Create database
clickhouse-client --host=$CLICKHOUSE_HOST --port=$CLICKHOUSE_PORT \
  --user=$CLICKHOUSE_USER --password=$CLICKHOUSE_PASSWORD \
  --query="CREATE DATABASE IF NOT EXISTS $CLICKHOUSE_DB"

# Create metrics table
clickhouse-client --host=$CLICKHOUSE_HOST --port=$CLICKHOUSE_PORT \
  --user=$CLICKHOUSE_USER --password=$CLICKHOUSE_PASSWORD \
  --database=$CLICKHOUSE_DB \
  --query="
CREATE TABLE IF NOT EXISTS metrics (
    timestamp DateTime64(3),
    metric_name LowCardinality(String),
    value Float64,
    dimensions Map(String, String),
    INDEX idx_metric_name metric_name TYPE bloom_filter GRANULARITY 1
) ENGINE = MergeTree()
PARTITION BY toYYYYMM(timestamp)
ORDER BY (metric_name, timestamp)
TTL timestamp + INTERVAL 30 DAY
SETTINGS index_granularity = 8192
"

echo "ClickHouse schema setup complete!"
