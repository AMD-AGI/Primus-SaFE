# Alert Module TODO List

> Last Updated: 2026-01-19  
> Overall Completion: ~85-90%

## High Priority

### 1. Notification Channels
- [x] **Email Notification** (`telemetry-processor/pkg/module/alerts/notifier.go`) - Completed 2026-01-19
  - SMTP/email service integration with TLS/STARTTLS support
  - HTML email templates with responsive design
  - Configurable SMTP settings (host, port, credentials, TLS options)


### 2. Alert Routing
- [ ] **Dynamic Route Configuration Loading** (`telemetry-processor/pkg/module/alerts/router.go`)
  - Load routing rules from database instead of hardcoded defaults
  - Implement route cache with periodic refresh
  - Support route priority and fallback

- [ ] **Regex Matcher Implementation** (`telemetry-processor/pkg/module/alerts/router.go`)
  - Add regex support for label matchers (`=~` and `!~` operators)
  - Compile and cache regex patterns for performance

## Medium Priority

### 3. Log Alert Engine
- [ ] **Composite Rule Evaluation** (`telemetry-processor/pkg/module/log_alert_engine/engine.go`)
  - Implement AND/OR logic for combining multiple conditions
  - Support nested rule expressions
  - Add correlation between different log patterns

### 4. Log Alert Rule API
- [ ] **Rule Testing Logic** (`api/pkg/api/log_alert_rule.go`)
  - Implement `TestLogAlertRule` endpoint logic
  - Create temporary rule evaluation against sample logs
  - Return match results with detailed explanation

### 5. Alert Silence
- [ ] **Expression-based Silence Matching** (`telemetry-processor/pkg/module/alerts/processor.go`)
  - Implement CEL (Common Expression Language) or similar expression evaluation
  - Support complex matching conditions

## Low Priority

### 6. Alert Correlation Enhancement
- [ ] **ML-based Causal Detection** (`telemetry-processor/pkg/module/alerts/correlator.go`)
  - Replace hardcoded causal rules with learned patterns
  - Integrate with AI advisor for root cause analysis

### 7. Alert API Enhancements
- [ ] **Alert Aggregation API**
  - Add endpoint for grouped/aggregated alert views
  - Support custom grouping dimensions

- [ ] **Alert Timeline API**
  - Add endpoint for alert timeline visualization
  - Include state transitions and related events

## Completed Features

### telemetry-processor Module
- [x] Unified alert data model (metric/log/trace sources)
- [x] VMAlert webhook receiver
- [x] Log alert receiver
- [x] Trace alert receiver
- [x] Generic webhook receiver
- [x] Alert deduplication
- [x] Alert enrichment (workload/pod/node context)
- [x] Silence rule checking (label/alert_name/resource types)
- [x] Database persistence
- [x] Statistics tracking (hourly/daily)
- [x] Time-based correlation
- [x] Entity-based correlation
- [x] Cross-source correlation
- [x] Basic causal correlation (predefined rules)
- [x] Webhook notification
- [x] Slack notification
- [x] AlertManager forwarding
- [x] Email notification (SMTP with TLS/STARTTLS, HTML templates)
- [x] Log alert rule engine core
- [x] Rule caching with periodic refresh
- [x] Pattern matching (regex)
- [x] Threshold matching (sliding window)
- [x] Label selector matching

### api Module
- [x] Metric alert rule CRUD API
- [x] Metric alert rule cloning
- [x] VMRule sync to Kubernetes cluster
- [x] VMRule status query
- [x] Batch rule sync
- [x] Log alert rule CRUD API
- [x] Multi-cluster support
- [x] Batch enable/disable
- [x] Rule version management and rollback
- [x] Log alert rule cloning
- [x] Rule statistics query
- [x] Alert silence CRUD API
- [x] Multiple silence types support
- [x] Time window support
- [x] Silenced alerts history
- [x] Log alert rule templates
- [x] Alert rule advice management

## File References

| Feature | File Path |
|---------|-----------|
| Alert Models | `telemetry-processor/pkg/module/alerts/model.go` |
| Alert Processor | `telemetry-processor/pkg/module/alerts/processor.go` |
| Alert Receiver | `telemetry-processor/pkg/module/alerts/receiver.go` |
| Alert Router | `telemetry-processor/pkg/module/alerts/router.go` |
| Alert Notifier | `telemetry-processor/pkg/module/alerts/notifier.go` |
| Alert Correlator | `telemetry-processor/pkg/module/alerts/correlator.go` |
| Alert API | `telemetry-processor/pkg/module/alerts/api.go` |
| Log Alert Engine | `telemetry-processor/pkg/module/log_alert_engine/engine.go` |
| Metric Alert Rule API | `api/pkg/api/metric_alert_rule.go` |
| Log Alert Rule API | `api/pkg/api/log_alert_rule.go` |
| Alert Silence API | `api/pkg/api/alert_silence.go` |
| Log Alert Rule Template API | `api/pkg/api/log_alert_rule_template.go` |
| Alert Rule Advice API | `api/pkg/api/alert_rule_advice.go` |
