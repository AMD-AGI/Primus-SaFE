# Log Alert Rules API

The Log Alert Rules API provides operations for managing log-based alert rules. These rules analyze log streams to detect specific patterns, anomalies, or conditions and trigger alerts when matches are found.

## Endpoints

### Create Log Alert Rule

Creates a new log alert rule for monitoring log streams.

**Endpoint:** `POST /api/log-alert-rules`

**Query Parameters:**

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `cluster` | string | No | current | Target cluster name |

**Request Body:**

```json
{
  "name": "high-error-rate",
  "description": "Alert when error rate exceeds threshold",
  "enabled": true,
  "priority": 3,
  "label_selectors": [
    {
      "label": "app",
      "operator": "=",
      "value": "training-service"
    },
    {
      "label": "level",
      "operator": "=",
      "value": "error"
    }
  ],
  "match_type": "count",
  "match_config": {
    "threshold": 100,
    "window": "5m",
    "comparison": "gt"
  },
  "severity": "critical",
  "alert_template": {
    "title": "High Error Rate Detected",
    "message": "Error count exceeded {{ .threshold }} in the last {{ .window }}",
    "receivers": ["ops-team", "oncall"]
  },
  "group_by": ["app", "namespace"],
  "group_wait": 30,
  "repeat_interval": 3600,
  "route_config": {
    "receiver": "ops-team",
    "group_wait": "30s",
    "repeat_interval": "1h"
  },
  "created_by": "admin",
  "create_version": true
}
```

**Request Fields:**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | Yes | Unique rule name |
| `description` | string | No | Rule description |
| `enabled` | boolean | No | Enable/disable the rule (default: false) |
| `priority` | integer | No | Rule priority 1-10 (default: 5) |
| `label_selectors` | array | Yes | Label selectors for filtering logs |
| `match_type` | string | Yes | Match type (count, rate, regex, keyword, anomaly) |
| `match_config` | object | Yes | Match configuration based on match_type |
| `severity` | string | No | Alert severity (info, warning, critical) (default: warning) |
| `alert_template` | object | No | Alert notification template |
| `group_by` | array | No | Fields to group alerts by |
| `group_wait` | integer | No | Group wait time in seconds (default: 30) |
| `repeat_interval` | integer | No | Repeat interval in seconds (default: 3600) |
| `route_config` | object | No | Alert routing configuration |
| `created_by` | string | No | Creator username |
| `create_version` | boolean | No | Create initial version snapshot (default: false) |

**Label Selector Fields:**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `label` | string | Yes | Label key |
| `operator` | string | Yes | Operator (=, !=, =~, !~) |
| `value` | string | Yes | Label value or regex pattern |

**Match Types and Configurations:**

**1. Count Match:**
```json
{
  "match_type": "count",
  "match_config": {
    "threshold": 100,
    "window": "5m",
    "comparison": "gt"
  }
}
```

**2. Rate Match:**
```json
{
  "match_type": "rate",
  "match_config": {
    "threshold": 10.0,
    "window": "1m",
    "comparison": "gt"
  }
}
```

**3. Regex Match:**
```json
{
  "match_type": "regex",
  "match_config": {
    "pattern": "OOM|Out of memory",
    "case_sensitive": false
  }
}
```

**4. Keyword Match:**
```json
{
  "match_type": "keyword",
  "match_config": {
    "keywords": ["error", "fatal", "exception"],
    "match_all": false
  }
}
```

**5. Anomaly Detection:**
```json
{
  "match_type": "anomaly",
  "match_config": {
    "algorithm": "statistical",
    "sensitivity": "medium",
    "baseline_window": "1h"
  }
}
```

**Response:**

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "rule_id": 123,
    "cluster_name": "production-cluster"
  },
  "traceId": "trace-abc123"
}
```

**Status Codes:**
- `201 Created` - Rule created successfully
- `400 Bad Request` - Invalid request body or parameters
- `500 Internal Server Error` - Server error

**Example:**

```bash
curl -X POST "http://localhost:8080/api/log-alert-rules?cluster=production" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "high-error-rate",
    "enabled": true,
    "label_selectors": [
      {"label": "level", "operator": "=", "value": "error"}
    ],
    "match_type": "count",
    "match_config": {
      "threshold": 100,
      "window": "5m",
      "comparison": "gt"
    }
  }'
```

---

### List Log Alert Rules

Retrieves a paginated list of log alert rules with filtering support.

**Endpoint:** `GET /api/log-alert-rules`

**Query Parameters:**

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `cluster` | string | No | current | Target cluster name |
| `offset` | integer | No | 0 | Offset for pagination |
| `limit` | integer | No | 50 | Number of items to return (max: 100) |
| `enabled` | boolean | No | - | Filter by enabled status |
| `match_type` | string | No | - | Filter by match type |
| `severity` | string | No | - | Filter by severity |
| `created_by` | string | No | - | Filter by creator |
| `keyword` | string | No | - | Search by name or description |
| `priority` | integer | No | - | Filter by priority |

**Response:**

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "rules": [
      {
        "id": 123,
        "name": "high-error-rate",
        "description": "Alert when error rate exceeds threshold",
        "cluster_name": "production-cluster",
        "enabled": true,
        "priority": 3,
        "label_selectors": [...],
        "match_type": "count",
        "match_config": {...},
        "severity": "critical",
        "alert_template": {...},
        "group_by": ["app", "namespace"],
        "group_wait": 30,
        "repeat_interval": 3600,
        "route_config": {...},
        "created_at": "2024-01-15T09:00:00Z",
        "updated_at": "2024-01-15T10:30:00Z",
        "created_by": "admin",
        "updated_by": "admin"
      }
    ],
    "total": 50,
    "offset": 0,
    "limit": 50,
    "cluster_name": "production-cluster"
  },
  "traceId": "trace-abc123"
}
```

**Status Codes:**
- `200 OK` - Success
- `400 Bad Request` - Invalid parameters
- `500 Internal Server Error` - Server error

**Example:**

```bash
# List all rules
curl -X GET "http://localhost:8080/api/log-alert-rules?limit=50"

# Filter by severity and status
curl -X GET "http://localhost:8080/api/log-alert-rules?severity=critical&enabled=true"

# Search by keyword
curl -X GET "http://localhost:8080/api/log-alert-rules?keyword=error"
```

---

### List Log Alert Rules (Multi-Cluster)

Retrieves log alert rules from all configured clusters.

**Endpoint:** `GET /api/log-alert-rules/multi-cluster`

**Query Parameters:**

Same as single-cluster list endpoint (cluster_name is not used).

**Response:**

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "clusters": [
      {
        "cluster_name": "production-cluster",
        "rules": [...],
        "total": 25
      },
      {
        "cluster_name": "staging-cluster",
        "rules": [...],
        "total": 15
      },
      {
        "cluster_name": "dev-cluster",
        "rules": [],
        "total": 0,
        "error": "connection timeout"
      }
    ]
  },
  "traceId": "trace-abc123"
}
```

**Status Codes:**
- `200 OK` - Success (individual cluster errors returned in response)
- `500 Internal Server Error` - Server error

**Example:**

```bash
curl -X GET "http://localhost:8080/api/log-alert-rules/multi-cluster?limit=50"
```

---

### Get Log Alert Rule

Retrieves detailed information for a specific log alert rule.

**Endpoint:** `GET /api/log-alert-rules/:id`

**Path Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `id` | integer | Yes | Rule ID |

**Query Parameters:**

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `cluster` | string | No | current | Target cluster name |

**Response:**

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "id": 123,
    "name": "high-error-rate",
    "description": "Alert when error rate exceeds threshold",
    "cluster_name": "production-cluster",
    "enabled": true,
    "priority": 3,
    "label_selectors": [
      {
        "label": "app",
        "operator": "=",
        "value": "training-service"
      }
    ],
    "match_type": "count",
    "match_config": {
      "threshold": 100,
      "window": "5m",
      "comparison": "gt"
    },
    "severity": "critical",
    "alert_template": {
      "title": "High Error Rate Detected",
      "message": "Error count exceeded threshold"
    },
    "group_by": ["app", "namespace"],
    "group_wait": 30,
    "repeat_interval": 3600,
    "route_config": {...},
    "created_at": "2024-01-15T09:00:00Z",
    "updated_at": "2024-01-15T10:30:00Z",
    "created_by": "admin",
    "updated_by": "admin"
  },
  "traceId": "trace-abc123"
}
```

**Status Codes:**
- `200 OK` - Success
- `404 Not Found` - Rule not found
- `500 Internal Server Error` - Server error

**Example:**

```bash
curl -X GET "http://localhost:8080/api/log-alert-rules/123?cluster=production"
```

---

### Update Log Alert Rule

Updates an existing log alert rule.

**Endpoint:** `PUT /api/log-alert-rules/:id`

**Path Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `id` | integer | Yes | Rule ID |

**Query Parameters:**

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `cluster` | string | No | current | Target cluster name |

**Request Body:**

Same format as create request.

**Response:**

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "rule_id": 123
  },
  "traceId": "trace-abc123"
}
```

**Status Codes:**
- `200 OK` - Success
- `400 Bad Request` - Invalid request body
- `404 Not Found` - Rule not found
- `500 Internal Server Error` - Server error

**Example:**

```bash
curl -X PUT "http://localhost:8080/api/log-alert-rules/123?cluster=production" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "high-error-rate",
    "enabled": true,
    "match_config": {"threshold": 150, "window": "5m"},
    "updated_by": "admin",
    "create_version": true,
    "change_log": "Increased threshold to 150"
  }'
```

---

### Delete Log Alert Rule

Deletes a log alert rule.

**Endpoint:** `DELETE /api/log-alert-rules/:id`

**Path Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `id` | integer | Yes | Rule ID |

**Query Parameters:**

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `cluster` | string | No | current | Target cluster name |

**Response:**

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "message": "rule deleted successfully"
  },
  "traceId": "trace-abc123"
}
```

**Status Codes:**
- `200 OK` - Success
- `404 Not Found` - Rule not found
- `500 Internal Server Error` - Server error

**Example:**

```bash
curl -X DELETE "http://localhost:8080/api/log-alert-rules/123?cluster=production"
```

---

### Batch Update Log Alert Rules

Updates multiple log alert rules at once (currently supports enabling/disabling).

**Endpoint:** `POST /api/log-alert-rules/batch-update`

**Query Parameters:**

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `cluster` | string | No | current | Target cluster name |

**Request Body:**

```json
{
  "rule_ids": [123, 456, 789],
  "enabled": false
}
```

**Request Fields:**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `rule_ids` | array | Yes | Array of rule IDs to update |
| `enabled` | boolean | Yes | New enabled status |

**Response:**

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "updated_count": 3
  },
  "traceId": "trace-abc123"
}
```

**Status Codes:**
- `200 OK` - Success
- `400 Bad Request` - Invalid request
- `500 Internal Server Error` - Server error

**Example:**

```bash
curl -X POST "http://localhost:8080/api/log-alert-rules/batch-update" \
  -H "Content-Type: application/json" \
  -d '{
    "rule_ids": [123, 456, 789],
    "enabled": false
  }'
```

---

### Test Log Alert Rule

Tests a log alert rule against sample log data (validation only, not fully implemented).

**Endpoint:** `POST /api/log-alert-rules/test`

**Request Body:**

```json
{
  "rule": {
    "name": "test-rule",
    "label_selectors": [...],
    "match_type": "count",
    "match_config": {...}
  },
  "sample_logs": [
    {
      "timestamp": "2024-01-15T10:00:00Z",
      "level": "error",
      "message": "Connection timeout",
      "labels": {"app": "training-service"}
    }
  ]
}
```

**Response:**

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "message": "rule test not yet implemented"
  },
  "traceId": "trace-abc123"
}
```

**Status Codes:**
- `200 OK` - Success
- `400 Bad Request` - Invalid request

---

### Get Log Alert Rule Statistics

Retrieves statistics for a specific log alert rule (alert counts, trends, etc.).

**Endpoint:** `GET /api/log-alert-rules/:id/statistics`

**Path Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `id` | integer | Yes | Rule ID |

**Query Parameters:**

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `cluster` | string | No | current | Target cluster name |
| `from` | string | No | 7 days ago | Start date (YYYY-MM-DD) |
| `to` | string | No | today | End date (YYYY-MM-DD) |

**Response:**

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "summary": {
      "total_alerts": 150,
      "alert_rate": 5.5,
      "peak_time": "2024-01-15T14:00:00Z",
      "avg_alerts_per_day": 21.4
    },
    "statistics": [
      {
        "date": "2024-01-15",
        "alert_count": 25,
        "matched_logs": 1250
      },
      {
        "date": "2024-01-14",
        "alert_count": 18,
        "matched_logs": 900
      }
    ]
  },
  "traceId": "trace-abc123"
}
```

**Status Codes:**
- `200 OK` - Success
- `404 Not Found` - Rule not found
- `500 Internal Server Error` - Server error

**Example:**

```bash
curl -X GET "http://localhost:8080/api/log-alert-rules/123/statistics?from=2024-01-01&to=2024-01-31"
```

---

### Get Log Alert Rule Versions

Retrieves version history for a log alert rule.

**Endpoint:** `GET /api/log-alert-rules/:id/versions`

**Path Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `id` | integer | Yes | Rule ID |

**Query Parameters:**

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `cluster` | string | No | current | Target cluster name |

**Response:**

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "versions": [
      {
        "id": 1,
        "rule_id": 123,
        "version": 3,
        "config": {...},
        "status": "active",
        "deployed_at": "2024-01-15T10:30:00Z",
        "created_by": "admin",
        "change_log": "Increased threshold to 150"
      },
      {
        "id": 2,
        "rule_id": 123,
        "version": 2,
        "config": {...},
        "status": "archived",
        "deployed_at": "2024-01-14T15:20:00Z",
        "created_by": "admin",
        "change_log": "Updated label selectors"
      }
    ],
    "total": 3
  },
  "traceId": "trace-abc123"
}
```

**Status Codes:**
- `200 OK` - Success
- `404 Not Found` - Rule not found
- `500 Internal Server Error` - Server error

**Example:**

```bash
curl -X GET "http://localhost:8080/api/log-alert-rules/123/versions"
```

---

### Rollback Log Alert Rule

Rolls back a log alert rule to a previous version.

**Endpoint:** `POST /api/log-alert-rules/:id/rollback/:version`

**Path Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `id` | integer | Yes | Rule ID |
| `version` | integer | Yes | Target version number |

**Query Parameters:**

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `cluster` | string | No | current | Target cluster name |

**Response:**

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "message": "rule rolled back successfully"
  },
  "traceId": "trace-abc123"
}
```

**Status Codes:**
- `200 OK` - Success
- `404 Not Found` - Rule or version not found
- `500 Internal Server Error` - Server error

**Example:**

```bash
curl -X POST "http://localhost:8080/api/log-alert-rules/123/rollback/2?cluster=production"
```

---

### Clone Log Alert Rule

Clones a log alert rule to another cluster or creates a copy with a new name.

**Endpoint:** `POST /api/log-alert-rules/:id/clone`

**Path Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `id` | integer | Yes | Source rule ID |

**Query Parameters:**

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `cluster` | string | No | current | Source cluster name |

**Request Body:**

```json
{
  "new_name": "high-error-rate-staging",
  "target_cluster_name": "staging-cluster",
  "enabled": true,
  "created_by": "admin"
}
```

**Request Fields:**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `new_name` | string | Yes | New rule name |
| `target_cluster_name` | string | No | Target cluster (defaults to source cluster) |
| `enabled` | boolean | No | Initial enabled status (default: false) |
| `created_by` | string | No | Creator username |

**Response:**

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "rule_id": 456,
    "cluster_name": "staging-cluster"
  },
  "traceId": "trace-abc123"
}
```

**Status Codes:**
- `201 Created` - Rule cloned successfully
- `400 Bad Request` - Invalid request
- `404 Not Found` - Source rule not found
- `500 Internal Server Error` - Server error

**Example:**

```bash
curl -X POST "http://localhost:8080/api/log-alert-rules/123/clone" \
  -H "Content-Type: application/json" \
  -d '{
    "new_name": "high-error-rate-staging",
    "target_cluster_name": "staging-cluster",
    "enabled": true
  }'
```

---

## Match Types

### Count

Triggers an alert when the log count exceeds a threshold within a time window.

**Configuration:**
- `threshold` (integer): Count threshold
- `window` (string): Time window (e.g., "5m", "1h")
- `comparison` (string): Comparison operator (gt, gte, lt, lte, eq)

**Use Cases:**
- Error rate monitoring
- Event frequency tracking
- Threshold-based alerts

### Rate

Triggers an alert based on the rate of log messages per time unit.

**Configuration:**
- `threshold` (float): Rate threshold (logs per second)
- `window` (string): Time window for rate calculation
- `comparison` (string): Comparison operator

**Use Cases:**
- Request rate monitoring
- Traffic spike detection
- Throughput analysis

### Regex

Matches logs against a regular expression pattern.

**Configuration:**
- `pattern` (string): Regular expression pattern
- `case_sensitive` (boolean): Case-sensitive matching

**Use Cases:**
- Pattern-based detection
- Complex log parsing
- Multi-pattern matching

### Keyword

Matches logs containing specific keywords.

**Configuration:**
- `keywords` (array): List of keywords
- `match_all` (boolean): Require all keywords (AND) or any keyword (OR)

**Use Cases:**
- Simple error detection
- Keyword-based filtering
- Multi-keyword alerts

### Anomaly

Detects anomalies in log patterns using statistical or ML algorithms.

**Configuration:**
- `algorithm` (string): Algorithm type (statistical, ml, seasonal)
- `sensitivity` (string): Sensitivity level (low, medium, high)
- `baseline_window` (string): Baseline learning window

**Use Cases:**
- Unusual pattern detection
- Behavioral anomaly detection
- Predictive alerting

---

## Severity Levels

| Severity | Description | Typical Use Cases |
|----------|-------------|-------------------|
| `info` | Informational alerts | Non-critical events, audit logs |
| `warning` | Warning-level alerts | Degraded performance, threshold warnings |
| `critical` | Critical alerts | System failures, security events, data loss |

---

## Data Models

### LogAlertRule

```go
type LogAlertRule struct {
    ID             int64       // Rule ID
    Name           string      // Rule name
    Description    string      // Description
    ClusterName    string      // Target cluster
    Enabled        bool        // Enabled status
    Priority       int         // Priority (1-10)
    LabelSelectors interface{} // Label selectors
    MatchType      string      // Match type
    MatchConfig    interface{} // Match configuration
    Severity       string      // Alert severity
    AlertTemplate  interface{} // Alert template
    GroupBy        []string    // Group by fields
    GroupWait      int64       // Group wait seconds
    RepeatInterval int64       // Repeat interval seconds
    RouteConfig    interface{} // Routing configuration
    CreatedAt      time.Time   // Creation time
    UpdatedAt      time.Time   // Update time
    CreatedBy      string      // Creator
    UpdatedBy      string      // Last updater
}
```

---

## Notes

- Label selectors support regex matching using `=~` and `!~` operators
- Time windows use Go duration format: `30s`, `5m`, `1h`, `24h`
- Rule priorities range from 1 (lowest) to 10 (highest)
- Version snapshots are created when `create_version` is true
- Versions can be used to rollback rule configurations
- Multi-cluster listing aggregates rules from all configured clusters
- Statistics are calculated based on alert history in the database

---

## Error Handling

Common error responses:

```json
{
  "code": 400,
  "message": "name is required",
  "traceId": "trace-abc123"
}
```

```json
{
  "code": 400,
  "message": "match_type is required",
  "traceId": "trace-abc123"
}
```

```json
{
  "code": 404,
  "message": "rule not found",
  "traceId": "trace-abc123"
}
```

---

## Best Practices

1. **Rule Naming**: Use descriptive names indicating what the rule detects
2. **Label Selectors**: Use specific selectors to reduce false positives
3. **Thresholds**: Set appropriate thresholds based on historical data
4. **Time Windows**: Choose windows that balance responsiveness and stability
5. **Severity Assignment**: Assign severity levels consistently across rules
6. **Testing**: Test rules with sample data before enabling
7. **Versioning**: Use version snapshots for important configuration changes
8. **Grouping**: Use group_by to aggregate related alerts
9. **Repeat Interval**: Set reasonable intervals to avoid alert fatigue
10. **Documentation**: Maintain clear descriptions for each rule

---

## Integration Examples

### Python

```python
import requests

API_BASE = "http://localhost:8080/api"

def create_log_alert(name, match_type, config):
    rule = {
        "name": name,
        "enabled": True,
        "label_selectors": [
            {"label": "level", "operator": "=", "value": "error"}
        ],
        "match_type": match_type,
        "match_config": config,
        "severity": "warning",
        "created_by": "automation"
    }
    
    response = requests.post(
        f"{API_BASE}/log-alert-rules?cluster=production",
        json=rule
    )
    return response.json()

# Create error count alert
result = create_log_alert(
    "high-error-count",
    "count",
    {"threshold": 100, "window": "5m", "comparison": "gt"}
)
print(f"Rule created: {result['data']['rule_id']}")

# List rules with filters
def list_rules(severity=None, enabled=None):
    params = {}
    if severity:
        params["severity"] = severity
    if enabled is not None:
        params["enabled"] = str(enabled).lower()
    
    response = requests.get(f"{API_BASE}/log-alert-rules", params=params)
    return response.json()["data"]["rules"]

# Get critical rules
critical_rules = list_rules(severity="critical", enabled=True)
print(f"Found {len(critical_rules)} critical rules")
```

### Bash Script

```bash
#!/bin/bash
API_BASE="http://localhost:8080/api"
CLUSTER="production"

# Create log alert rule
create_log_alert() {
  curl -X POST "$API_BASE/log-alert-rules?cluster=$CLUSTER" \
    -H "Content-Type: application/json" \
    -d "{
      \"name\": \"$1\",
      \"enabled\": true,
      \"label_selectors\": [{
        \"label\": \"level\",
        \"operator\": \"=\",
        \"value\": \"error\"
      }],
      \"match_type\": \"count\",
      \"match_config\": {
        \"threshold\": $2,
        \"window\": \"5m\",
        \"comparison\": \"gt\"
      },
      \"severity\": \"$3\"
    }"
}

# List all rules
list_rules() {
  curl -s "$API_BASE/log-alert-rules?cluster=$CLUSTER&limit=100" \
    | jq '.data.rules[] | {id, name, enabled, severity}'
}

# Batch disable rules
disable_rules() {
  curl -X POST "$API_BASE/log-alert-rules/batch-update" \
    -H "Content-Type: application/json" \
    -d "{
      \"rule_ids\": [$1],
      \"enabled\": false
    }"
}

# Usage
create_log_alert "oom-detector" 10 "critical"
list_rules
disable_rules "123,456,789"
```

