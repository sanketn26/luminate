# Operations Documentation

Production operations, deployment, monitoring, and troubleshooting guides for Luminate.

## Quick Links

### Deployment
- **[Deployment Guide](deployment.md)** - Step-by-step deployment procedures
- **[Scaling Guide](scaling.md)** - Horizontal and vertical scaling strategies

### Monitoring & Alerting
- **[Monitoring Guide](monitoring.md)** - Prometheus, Grafana, and metrics
- **[Alerting Guide](alerting.md)** - Alert rules and escalation policies

### Data Management
- **[Backup & Restore](backup-restore.md)** - Data protection procedures
- **[Retention Policies](retention.md)** - Data lifecycle management

### Incident Response
- **[Runbooks](runbooks/)** - Step-by-step troubleshooting guides

## Documentation Overview

### Deployment Guide
**Audience:** DevOps engineers, SREs

Complete guide to deploying Luminate in production with ClickHouse.

**Topics:**
- Prerequisites and requirements
- ClickHouse cluster setup
- Kubernetes manifests
- Secrets management
- Load testing and validation
- Rollback procedures

### Monitoring Guide
**Audience:** Operations teams

Set up comprehensive monitoring for Luminate.

**Topics:**
- Prometheus configuration
- Grafana dashboards
- Key metrics to watch
- Performance indicators
- Capacity planning

### Backup & Restore
**Audience:** Database administrators, SREs

Protect your data with automated backups.

**Topics:**
- Backup strategy
- Automated backup jobs
- Restore procedures
- Point-in-time recovery
- Disaster recovery

### Runbooks
**Audience:** On-call engineers

Quick reference guides for common incidents.

**Available runbooks:**
- [High Error Rate](runbooks/high-error-rate.md)
- [High Latency](runbooks/high-latency.md)
- [Storage Issues](runbooks/storage-issues.md)
- [Scaling Events](runbooks/scaling.md)

## Production Checklist

Before deploying to production, ensure:

### Infrastructure
- [ ] ClickHouse cluster deployed (minimum 4 nodes)
- [ ] Zookeeper ensemble running (3 nodes)
- [ ] Kubernetes cluster meets requirements
- [ ] Load balancer configured
- [ ] TLS certificates provisioned

### Security
- [ ] JWT secret configured
- [ ] Secrets stored in vault/sealed secrets
- [ ] Network policies applied
- [ ] RBAC configured
- [ ] TLS enabled for external traffic

### Monitoring
- [ ] Prometheus scraping Luminate
- [ ] Grafana dashboards imported
- [ ] Alerting rules configured
- [ ] PagerDuty/Opsgenie integration set up
- [ ] Log aggregation configured

### Data Protection
- [ ] Automated backups configured
- [ ] Backup retention policy set
- [ ] Restore procedure tested
- [ ] Disaster recovery plan documented

### Performance
- [ ] Load testing completed in staging
- [ ] Auto-scaling tested
- [ ] Resource limits tuned
- [ ] Query performance validated

### Operations
- [ ] Runbooks reviewed by team
- [ ] On-call rotation established
- [ ] Incident management process defined
- [ ] Change management process in place

## Operational Metrics

Monitor these key metrics:

### Application Metrics
- `luminate_http_requests_total` - Request rate
- `luminate_http_request_duration_seconds` - Latency
- `luminate_storage_writes_total` - Write throughput
- `luminate_storage_query_duration_seconds` - Query performance
- `luminate_rate_limit_rejected_total` - Rate limiting

### System Metrics
- `luminate_goroutines` - Concurrent operations
- `luminate_memory_usage_bytes` - Memory consumption
- `process_cpu_seconds_total` - CPU usage

### ClickHouse Metrics
- `ClickHouseMetrics_Query` - Active queries
- `ClickHouseMetrics_BackgroundPoolTask` - Background tasks
- `ClickHouseAsyncMetrics_TotalRowsOfMergeTreeTables` - Data size

## SLA Targets

### Availability
- **Production:** 99.9% uptime (8.76 hours downtime/year)
- **Staging:** 99% uptime

### Performance
- **Write Latency (p95):** < 100ms
- **Query Latency (p95):** < 200ms
- **Write Throughput:** 100K+ metrics/sec per pod

### Data Durability
- **RPO (Recovery Point Objective):** < 1 hour
- **RTO (Recovery Time Objective):** < 4 hours

## Maintenance Windows

### Weekly Maintenance
- **When:** Saturday 2-4 AM UTC
- **What:** Minor updates, configuration changes
- **Impact:** Zero downtime (rolling updates)

### Monthly Maintenance
- **When:** First Sunday 2-6 AM UTC
- **What:** Major version updates, cluster maintenance
- **Impact:** Possible brief disruption (<5 minutes)

### Emergency Maintenance
- **When:** As needed for critical security/stability
- **Process:** Escalate to VP Engineering for approval

## Contact Information

### On-Call Rotation
- **Primary:** See PagerDuty schedule
- **Secondary:** See PagerDuty schedule
- **Escalation:** Engineering Manager → VP Engineering

### Support Channels
- **Slack:** #luminate-ops
- **Email:** luminate-ops@company.com
- **PagerDuty:** luminate-production service

### Vendor Support
- **ClickHouse:** Altinity Support (Enterprise)
- **Kubernetes:** Cloud provider support
- **Grafana:** Grafana Labs (if using Cloud)

## Change Management

### Change Request Process
1. Create change request in Jira
2. Document:
   - What's changing
   - Why it's changing
   - Rollback plan
   - Testing performed
3. Get approval from engineering manager
4. Schedule during maintenance window (unless emergency)
5. Execute with runbook
6. Validate and close ticket

### Rollback Criteria
Rollback immediately if:
- Error rate > 5% for 5 minutes
- p95 latency > 500ms for 10 minutes
- Data corruption detected
- Security vulnerability introduced

## Disaster Recovery

### Scenarios

**1. ClickHouse Node Failure**
- **Impact:** Reduced capacity, no data loss
- **Action:** Kubernetes restarts pod automatically
- **Recovery Time:** 2-5 minutes

**2. ClickHouse Cluster Failure**
- **Impact:** Service outage
- **Action:** Restore from backup
- **Recovery Time:** 2-4 hours

**3. Data Center Failure**
- **Impact:** Complete outage
- **Action:** Failover to secondary region (if configured)
- **Recovery Time:** 30-60 minutes

**4. Data Corruption**
- **Impact:** Incorrect query results
- **Action:** Point-in-time restore
- **Recovery Time:** 2-4 hours

## Capacity Planning

### Growth Metrics to Monitor
- Metrics written per day
- Unique series count
- Storage size growth
- Query volume

### Scaling Triggers
- **Storage:** When 70% full, add shards
- **Compute:** When CPU > 70%, scale horizontally
- **Memory:** When memory > 80%, increase pod limits

### Forecasting
- Review capacity quarterly
- Project growth based on 6-month trends
- Plan hardware/cloud resources 2 quarters ahead

---

**Last Updated:** 2024-12-09
