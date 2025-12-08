# WS11: Dashboard UI & Visualization (Grafana Integration)

**Priority:** P1 (High Priority)
**Estimated Effort:** 3-4 days
**Dependencies:** WS3 (HTTP API Handlers), WS4 (Authentication)

## Overview

This workstream integrates Luminate with Grafana by building a Grafana datasource plugin. Instead of building a custom dashboard from scratch, we leverage Grafana's mature ecosystem to provide professional-grade dashboards, alerting, templating, and visualization capabilities.

## Why Grafana?

**Benefits:**
- ✅ **Industry Standard**: Used by millions, proven at scale (Netflix, Uber, PayPal)
- ✅ **Rich Visualization**: 20+ chart types, heatmaps, gauges, tables
- ✅ **Zero UI Development**: Grafana handles all dashboard UI
- ✅ **Advanced Features**: Alerting, annotations, templating, RBAC
- ✅ **Multi-Datasource**: Combine Luminate with Prometheus, Loki, etc.
- ✅ **Mobile Apps**: Native iOS/Android apps
- ✅ **Plugin Ecosystem**: Import thousands of existing dashboards

**vs Building Custom Dashboard:**
- Custom React dashboard: 8-10 days development + ongoing maintenance
- Grafana plugin: 3-4 days, leverages existing mature platform

## Architecture

```
┌─────────────────────────────────────────────────────────┐
│                   Browser (User)                        │
│                    Grafana UI                           │
│  - Dashboards, Panels, Queries                          │
│  - Alerting, Templating, Permissions                    │
└─────────────────────────────────────────────────────────┘
                         ↓
┌─────────────────────────────────────────────────────────┐
│           Luminate Datasource Plugin                    │
│              (TypeScript/React)                         │
│                                                         │
│  Components:                                            │
│  - QueryEditor: Build queries visually                 │
│  - ConfigEditor: Set API URL, JWT token                │
│  - Datasource: Execute queries, transform data         │
│                                                         │
│  Implements Grafana Datasource API:                    │
│  - query(): Execute queries                             │
│  - testDatasource(): Health check                      │
│  - metricFindQuery(): Variable values                  │
└─────────────────────────────────────────────────────────┘
                         ↓
┌─────────────────────────────────────────────────────────┐
│              Luminate HTTP API                          │
│  POST /api/v1/query                                     │
│  GET  /api/v1/metrics                                   │
│  GET  /api/v1/metrics/{name}/dimensions                 │
└─────────────────────────────────────────────────────────┘
```

## Work Items

### Work Item 1: Plugin Scaffolding

**Purpose:** Create Grafana plugin project structure.

**Implementation:**

```bash
# Install Grafana plugin tools
npm install -g @grafana/toolkit

# Create plugin
npx @grafana/create-plugin

# Project name: luminate-datasource
# Plugin type: datasource
# Organization: your-org

cd luminate-datasource
npm install
```

**Project Structure:**
```
luminate-datasource/
├── src/
│   ├── datasource.ts           # Main datasource logic
│   ├── ConfigEditor.tsx        # Plugin configuration UI
│   ├── QueryEditor.tsx         # Query builder UI
│   ├── types.ts                # TypeScript types
│   ├── module.ts               # Plugin entry point
│   └── plugin.json             # Plugin metadata
├── package.json
└── README.md
```

**plugin.json:**

```json
{
  "type": "datasource",
  "name": "Luminate",
  "id": "luminate-datasource",
  "metrics": true,
  "annotations": false,
  "backend": false,
  "executable": "",
  "info": {
    "description": "Datasource for Luminate high-cardinality observability",
    "author": {
      "name": "Your Organization",
      "url": "https://github.com/yourusername/luminate"
    },
    "keywords": ["metrics", "observability", "high-cardinality"],
    "version": "1.0.0",
    "updated": "2024-12-08"
  },
  "dependencies": {
    "grafanaDependency": ">=9.0.0",
    "plugins": []
  }
}
```

**Acceptance Criteria:**
- ✅ Plugin scaffolding created
- ✅ Development environment configured
- ✅ Plugin loads in Grafana
- ✅ Basic metadata defined

---

### Work Item 2: Datasource Implementation

**Purpose:** Implement core datasource logic to query Luminate API.

**Implementation:**

```typescript
// src/types.ts
import { DataQuery, DataSourceJsonData } from '@grafana/data';

export interface LuminateQuery extends DataQuery {
  queryType: 'range' | 'aggregate' | 'rate';
  metricName: string;
  aggregation?: 'avg' | 'sum' | 'count' | 'min' | 'max' | 'p50' | 'p95' | 'p99' | 'integral';
  groupBy?: string[];
  filters?: Record<string, string>;
  interval?: string;
}

export interface LuminateOptions extends DataSourceJsonData {
  apiUrl: string;
}

export interface LuminateSecureOptions {
  jwtToken?: string;
}

export interface QueryResponse {
  queryType: string;
  results: any[];
  executionTimeMs: number;
  pointsScanned?: number;
}
```

```typescript
// src/datasource.ts
import {
  DataQueryRequest,
  DataQueryResponse,
  DataSourceApi,
  DataSourceInstanceSettings,
  MutableDataFrame,
  FieldType,
  dateTime,
} from '@grafana/data';
import { getBackendSrv, getTemplateSrv } from '@grafana/runtime';
import { LuminateQuery, LuminateOptions, QueryResponse } from './types';

export class LuminateDatasource extends DataSourceApi<LuminateQuery, LuminateOptions> {
  apiUrl: string;
  jwtToken?: string;

  constructor(instanceSettings: DataSourceInstanceSettings<LuminateOptions>) {
    super(instanceSettings);
    this.apiUrl = instanceSettings.jsonData.apiUrl || 'http://localhost:8080';
    this.jwtToken = instanceSettings.secureJsonData?.jwtToken;
  }

  // Main query method
  async query(options: DataQueryRequest<LuminateQuery>): Promise<DataQueryResponse> {
    const promises = options.targets
      .filter((target) => !target.hide && target.metricName)
      .map((target) => this.executeQuery(target, options));

    const responses = await Promise.all(promises);

    return {
      data: responses.flat(),
    };
  }

  // Execute single query
  private async executeQuery(
    query: LuminateQuery,
    options: DataQueryRequest<LuminateQuery>
  ): Promise<MutableDataFrame[]> {
    const templateSrv = getTemplateSrv();

    // Build query request
    const request = {
      queryType: query.queryType,
      metricName: templateSrv.replace(query.metricName, options.scopedVars),
      timeRange: {
        start: options.range.from.toISOString(),
        end: options.range.to.toISOString(),
      },
      aggregation: query.aggregation,
      groupBy: query.groupBy || [],
      filters: this.replaceVariables(query.filters || {}, options.scopedVars),
      interval: query.interval,
      limit: 10000,
    };

    // Execute HTTP request
    const response = await this.doRequest<QueryResponse>({
      url: `${this.apiUrl}/api/v1/query`,
      method: 'POST',
      data: request,
    });

    // Transform response to Grafana data frames
    return this.transformResponse(response.data, query);
  }

  // Transform Luminate response to Grafana data frames
  private transformResponse(response: QueryResponse, query: LuminateQuery): MutableDataFrame[] {
    const frames: MutableDataFrame[] = [];

    if (query.queryType === 'aggregate') {
      // Aggregate results as table
      const frame = new MutableDataFrame({
        refId: query.refId,
        fields: [],
      });

      // Add dimension columns
      const results = response.results as Array<{ groupKey: Record<string, string>; value: number; count: number }>;
      if (results.length > 0) {
        const groupKeys = Object.keys(results[0].groupKey);
        groupKeys.forEach((key) => {
          frame.addField({ name: key, type: FieldType.string });
        });

        // Add value and count columns
        frame.addField({ name: 'value', type: FieldType.number });
        frame.addField({ name: 'count', type: FieldType.number });

        // Add data rows
        results.forEach((result) => {
          const row = groupKeys.map((key) => result.groupKey[key]);
          row.push(result.value, result.count);
          frame.add(row);
        });
      }

      frames.push(frame);
    } else if (query.queryType === 'range' || query.queryType === 'rate') {
      // Time series data
      const frame = new MutableDataFrame({
        refId: query.refId,
        fields: [
          { name: 'Time', type: FieldType.time },
          { name: 'Value', type: FieldType.number },
        ],
      });

      const results = response.results as Array<{ timestamp: string; value?: number; rate?: number }>;
      results.forEach((point) => {
        frame.add({
          Time: dateTime(point.timestamp).valueOf(),
          Value: point.value ?? point.rate ?? 0,
        });
      });

      frames.push(frame);
    }

    return frames;
  }

  // Test datasource connection
  async testDatasource() {
    try {
      const response = await this.doRequest({
        url: `${this.apiUrl}/api/v1/health`,
        method: 'GET',
      });

      if (response.status === 200) {
        return {
          status: 'success',
          message: 'Successfully connected to Luminate',
        };
      }

      return {
        status: 'error',
        message: 'Connection failed',
      };
    } catch (error: any) {
      return {
        status: 'error',
        message: `Connection error: ${error.message}`,
      };
    }
  }

  // Get metric names (for variables)
  async metricFindQuery(query: string): Promise<Array<{ text: string; value: string }>> {
    if (query === 'metrics') {
      const response = await this.doRequest<{ metrics: string[] }>({
        url: `${this.apiUrl}/api/v1/metrics`,
        method: 'GET',
      });

      return response.data.metrics.map((metric) => ({
        text: metric,
        value: metric,
      }));
    }

    // Support dimension value queries: dimensions(metric_name, dimension_key)
    const dimensionMatch = query.match(/dimensions\(([^,]+),\s*([^)]+)\)/);
    if (dimensionMatch) {
      const [, metricName, dimensionKey] = dimensionMatch;
      const response = await this.doRequest<{ values: string[] }>({
        url: `${this.apiUrl}/api/v1/metrics/${metricName}/dimensions/${dimensionKey}/values`,
        method: 'GET',
      });

      return response.data.values.map((value) => ({
        text: value,
        value,
      }));
    }

    return [];
  }

  // HTTP request wrapper with auth
  private async doRequest<T = any>(options: any): Promise<{ data: T; status: number }> {
    const headers: Record<string, string> = {
      'Content-Type': 'application/json',
    };

    if (this.jwtToken) {
      headers['Authorization'] = `Bearer ${this.jwtToken}`;
    }

    return getBackendSrv().datasourceRequest({
      ...options,
      headers,
    });
  }

  // Replace template variables in filters
  private replaceVariables(
    filters: Record<string, string>,
    scopedVars: any
  ): Record<string, string> {
    const templateSrv = getTemplateSrv();
    const result: Record<string, string> = {};

    for (const [key, value] of Object.entries(filters)) {
      result[key] = templateSrv.replace(value, scopedVars);
    }

    return result;
  }
}
```

**Acceptance Criteria:**
- ✅ Implements DataSourceApi interface
- ✅ Executes queries against Luminate API
- ✅ Transforms responses to Grafana data frames
- ✅ Handles time series and table data
- ✅ Supports template variables
- ✅ JWT authentication
- ✅ Health check works

---

### Work Item 3: Query Editor UI

**Purpose:** Visual query builder in Grafana panel editor.

**Implementation:**

```typescript
// src/QueryEditor.tsx
import React, { ChangeEvent, useState, useEffect } from 'react';
import { QueryEditorProps, SelectableValue } from '@grafana/data';
import { InlineField, Select, Input, MultiSelect, Button } from '@grafana/ui';
import { LuminateDatasource } from './datasource';
import { LuminateQuery, LuminateOptions } from './types';

type Props = QueryEditorProps<LuminateDatasource, LuminateQuery, LuminateOptions>;

export function QueryEditor({ query, onChange, onRunQuery, datasource }: Props) {
  const [metrics, setMetrics] = useState<Array<SelectableValue<string>>>([]);
  const [dimensions, setDimensions] = useState<Array<SelectableValue<string>>>([]);

  // Load metrics on mount
  useEffect(() => {
    datasource.metricFindQuery('metrics').then((results) => {
      setMetrics(results.map((r) => ({ label: r.text, value: r.value })));
    });
  }, [datasource]);

  // Load dimensions when metric changes
  useEffect(() => {
    if (query.metricName) {
      datasource
        .doRequest({
          url: `${datasource.apiUrl}/api/v1/metrics/${query.metricName}/dimensions`,
          method: 'GET',
        })
        .then((response: any) => {
          const dims = response.data.dimensions.map((d: string) => ({
            label: d,
            value: d,
          }));
          setDimensions(dims);
        });
    }
  }, [query.metricName, datasource]);

  const queryTypeOptions: Array<SelectableValue<string>> = [
    { label: 'Range (Raw Points)', value: 'range' },
    { label: 'Aggregate (Statistics)', value: 'aggregate' },
    { label: 'Rate (Change Over Time)', value: 'rate' },
  ];

  const aggregationOptions: Array<SelectableValue<string>> = [
    { label: 'Average', value: 'avg' },
    { label: 'Sum', value: 'sum' },
    { label: 'Count', value: 'count' },
    { label: 'Min', value: 'min' },
    { label: 'Max', value: 'max' },
    { label: 'P50 (Median)', value: 'p50' },
    { label: 'P95', value: 'p95' },
    { label: 'P99', value: 'p99' },
    { label: 'Integral', value: 'integral' },
  ];

  const intervalOptions: Array<SelectableValue<string>> = [
    { label: '1 minute', value: '1m' },
    { label: '5 minutes', value: '5m' },
    { label: '15 minutes', value: '15m' },
    { label: '1 hour', value: '1h' },
  ];

  const onQueryTypeChange = (value: SelectableValue<string>) => {
    onChange({ ...query, queryType: value.value as any });
    onRunQuery();
  };

  const onMetricChange = (value: SelectableValue<string>) => {
    onChange({ ...query, metricName: value.value || '' });
    onRunQuery();
  };

  const onAggregationChange = (value: SelectableValue<string>) => {
    onChange({ ...query, aggregation: value.value as any });
    onRunQuery();
  };

  const onGroupByChange = (values: Array<SelectableValue<string>>) => {
    onChange({ ...query, groupBy: values.map((v) => v.value || '') });
    onRunQuery();
  };

  const onIntervalChange = (value: SelectableValue<string>) => {
    onChange({ ...query, interval: value.value });
    onRunQuery();
  };

  return (
    <div>
      {/* Query Type */}
      <InlineField label="Query Type" labelWidth={14}>
        <Select
          options={queryTypeOptions}
          value={query.queryType}
          onChange={onQueryTypeChange}
          width={30}
        />
      </InlineField>

      {/* Metric Name */}
      <InlineField label="Metric" labelWidth={14}>
        <Select
          options={metrics}
          value={query.metricName}
          onChange={onMetricChange}
          width={40}
          placeholder="Select metric"
        />
      </InlineField>

      {/* Aggregation (for aggregate queries) */}
      {query.queryType === 'aggregate' && (
        <>
          <InlineField label="Aggregation" labelWidth={14}>
            <Select
              options={aggregationOptions}
              value={query.aggregation}
              onChange={onAggregationChange}
              width={30}
            />
          </InlineField>

          <InlineField label="Group By" labelWidth={14}>
            <MultiSelect
              options={dimensions}
              value={query.groupBy}
              onChange={onGroupByChange}
              width={40}
              placeholder="Select dimensions"
            />
          </InlineField>
        </>
      )}

      {/* Interval (for rate queries) */}
      {query.queryType === 'rate' && (
        <InlineField label="Interval" labelWidth={14}>
          <Select
            options={intervalOptions}
            value={query.interval}
            onChange={onIntervalChange}
            width={20}
          />
        </InlineField>
      )}
    </div>
  );
}
```

**Acceptance Criteria:**
- ✅ Dropdown for metric selection
- ✅ Query type selector
- ✅ Aggregation selector (for aggregate queries)
- ✅ Group by dimension selector
- ✅ Interval selector (for rate queries)
- ✅ Auto-refresh on changes
- ✅ Template variable support ($variable)

---

### Work Item 4: Configuration Editor

**Purpose:** Plugin configuration in Grafana datasource settings.

**Implementation:**

```typescript
// src/ConfigEditor.tsx
import React, { ChangeEvent } from 'react';
import { DataSourcePluginOptionsEditorProps } from '@grafana/data';
import { InlineField, Input, SecretInput } from '@grafana/ui';
import { LuminateOptions, LuminateSecureOptions } from './types';

interface Props extends DataSourcePluginOptionsEditorProps<LuminateOptions, LuminateSecureOptions> {}

export function ConfigEditor(props: Props) {
  const { onOptionsChange, options } = props;
  const { jsonData, secureJsonFields, secureJsonData } = options;

  const onApiUrlChange = (event: ChangeEvent<HTMLInputElement>) => {
    onOptionsChange({
      ...options,
      jsonData: {
        ...jsonData,
        apiUrl: event.target.value,
      },
    });
  };

  const onJWTTokenChange = (event: ChangeEvent<HTMLInputElement>) => {
    onOptionsChange({
      ...options,
      secureJsonData: {
        jwtToken: event.target.value,
      },
    });
  };

  const onResetJWTToken = () => {
    onOptionsChange({
      ...options,
      secureJsonFields: {
        ...options.secureJsonFields,
        jwtToken: false,
      },
      secureJsonData: {
        ...options.secureJsonData,
        jwtToken: '',
      },
    });
  };

  return (
    <div className="gf-form-group">
      <InlineField label="API URL" labelWidth={12} tooltip="Luminate API endpoint">
        <Input
          onChange={onApiUrlChange}
          value={jsonData.apiUrl || ''}
          placeholder="http://localhost:8080"
          width={40}
        />
      </InlineField>

      <InlineField label="JWT Token" labelWidth={12} tooltip="Authentication token">
        <SecretInput
          isConfigured={secureJsonFields?.jwtToken}
          value={secureJsonData?.jwtToken || ''}
          placeholder="Enter JWT token"
          width={40}
          onReset={onResetJWTToken}
          onChange={onJWTTokenChange}
        />
      </InlineField>
    </div>
  );
}
```

**Acceptance Criteria:**
- ✅ API URL configuration
- ✅ JWT token (secure field)
- ✅ Test datasource button works
- ✅ Settings persisted correctly

---

### Work Item 5: Plugin Module & Build

**Purpose:** Plugin entry point and build configuration.

**Implementation:**

```typescript
// src/module.ts
import { DataSourcePlugin } from '@grafana/data';
import { LuminateDatasource } from './datasource';
import { ConfigEditor } from './ConfigEditor';
import { QueryEditor } from './QueryEditor';
import { LuminateQuery, LuminateOptions } from './types';

export const plugin = new DataSourcePlugin<
  LuminateDatasource,
  LuminateQuery,
  LuminateOptions
>(LuminateDatasource)
  .setConfigEditor(ConfigEditor)
  .setQueryEditor(QueryEditor);
```

**Build & Install:**

```bash
# Build plugin
npm run build

# Development build (watch mode)
npm run dev

# Install to Grafana
# Copy dist/ folder to Grafana plugins directory
cp -r dist/ /var/lib/grafana/plugins/luminate-datasource

# Restart Grafana
sudo systemctl restart grafana-server

# Or use Docker volume
docker run -d \
  -p 3000:3000 \
  -v $(pwd)/dist:/var/lib/grafana/plugins/luminate-datasource \
  grafana/grafana:latest
```

**Package for Distribution:**

```bash
# Create signed plugin
npx @grafana/toolkit plugin:sign

# Create zip for distribution
zip -r luminate-datasource-1.0.0.zip dist/

# Publish to Grafana plugin catalog (optional)
# https://grafana.com/docs/grafana/latest/developers/plugins/publish-a-plugin/
```

**Acceptance Criteria:**
- ✅ Plugin builds successfully
- ✅ Loads in Grafana
- ✅ Appears in datasource list
- ✅ Can add datasource instance
- ✅ Ready for distribution

---

## Sample Dashboards

### Dashboard 1: System Overview

```json
{
  "dashboard": {
    "title": "Luminate System Overview",
    "panels": [
      {
        "title": "API Latency (p95)",
        "type": "timeseries",
        "targets": [
          {
            "queryType": "aggregate",
            "metricName": "api_latency",
            "aggregation": "p95",
            "groupBy": ["endpoint"]
          }
        ]
      },
      {
        "title": "Request Rate",
        "type": "timeseries",
        "targets": [
          {
            "queryType": "rate",
            "metricName": "api_requests_total",
            "interval": "1m"
          }
        ]
      },
      {
        "title": "Top Customers by Request Count",
        "type": "table",
        "targets": [
          {
            "queryType": "aggregate",
            "metricName": "api_requests_total",
            "aggregation": "count",
            "groupBy": ["customer_id"]
          }
        ]
      }
    ]
  }
}
```

### Dashboard 2: Customer Performance

```json
{
  "dashboard": {
    "title": "Customer Performance",
    "templating": {
      "list": [
        {
          "name": "customer",
          "type": "query",
          "query": "dimensions(api_latency, customer_id)",
          "multi": false
        }
      ]
    },
    "panels": [
      {
        "title": "Latency for $customer",
        "type": "timeseries",
        "targets": [
          {
            "queryType": "range",
            "metricName": "api_latency",
            "filters": {
              "customer_id": "$customer"
            }
          }
        ]
      }
    ]
  }
}
```

---

## Deployment

**Grafana Setup with Luminate:**

```bash
# 1. Deploy Grafana
kubectl apply -f - <<EOF
apiVersion: apps/v1
kind: Deployment
metadata:
  name: grafana
  namespace: luminate
spec:
  replicas: 1
  selector:
    matchLabels:
      app: grafana
  template:
    metadata:
      labels:
        app: grafana
    spec:
      containers:
      - name: grafana
        image: grafana/grafana:10.2.0
        ports:
        - containerPort: 3000
        env:
        - name: GF_SECURITY_ADMIN_PASSWORD
          value: admin
        - name: GF_PLUGINS_ALLOW_LOADING_UNSIGNED_PLUGINS
          value: luminate-datasource
        volumeMounts:
        - name: plugins
          mountPath: /var/lib/grafana/plugins
      volumes:
      - name: plugins
        configMap:
          name: grafana-plugins
---
apiVersion: v1
kind: Service
metadata:
  name: grafana
  namespace: luminate
spec:
  selector:
    app: grafana
  ports:
  - port: 3000
    targetPort: 3000
  type: LoadBalancer
EOF

# 2. Install Luminate plugin
kubectl cp dist/ luminate/grafana-pod:/var/lib/grafana/plugins/luminate-datasource

# 3. Access Grafana
kubectl port-forward svc/grafana 3000:3000 -n luminate
# Open http://localhost:3000 (admin/admin)

# 4. Add Luminate datasource
# Configuration -> Data sources -> Add data source -> Luminate
# API URL: http://luminate-svc:8080
# JWT Token: <your-token>
# Save & Test
```

---

## Summary

This workstream provides Grafana integration for Luminate:

1. **Grafana Datasource Plugin**: TypeScript plugin implementing Grafana datasource API
2. **Query Editor**: Visual query builder for Luminate queries
3. **Configuration**: Simple datasource settings (API URL, JWT token)
4. **Sample Dashboards**: Pre-built dashboards for common use cases
5. **Production Deployment**: Kubernetes manifests for Grafana + plugin

**Key Benefits:**
- ✅ Leverage mature Grafana ecosystem (vs building custom UI)
- ✅ 3-4 days development (vs 8-10 days for custom dashboard)
- ✅ Professional features out of the box (alerting, RBAC, mobile apps)
- ✅ Import thousands of existing Grafana dashboards
- ✅ Multi-datasource dashboards (combine with Prometheus, Loki)

**Next Steps**: Build plugin, test with Grafana, create default dashboards, deploy to production.
