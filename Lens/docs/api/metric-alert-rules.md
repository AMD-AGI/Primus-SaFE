# Metric Alert Rules API

The Metric Alert Rules API provides operations for managing metric-based alert rules in the cluster. These rules are used to monitor Prometheus/VictoriaMetrics metrics and trigger alerts when specified conditions are met.

## Endpoints

### Create Metric Alert Rule

Creates a new metric alert rule that will be synchronized to the cluster.

**Endpoint:** `POST /api/metric-alert-rules`

**Request Body:**

```json
{
  "name": "high-gpu-utilization",
  "cluster_name": "production-cluster",
  "enabled": true,
  "description": "Alert when GPU utilization exceeds 90%",
  "labels": {
    "severity": "warning",
    "team": "infrastructure"
  },
  "namespace": "primus-lens",
  "auto_sync": true,
  "groups": [
    {
      "name": "gpu-alerts",
      "interval": "30s",
      "rules": [
        {
          "alert": "HighGPUUtilization",
          "expr": "gpu_utilization > 0.9",
          "for": "5m",
          "labels": {
            "severity": "warning"
          },
          "annotations": {
            "summary": "High GPU utilization detected",
            "description": "GPU utilization is above 90% for more than 5 minutes"
          }
        }
      ]
    }
  ],
  "resource_mapping": {
    "pod_label": "pod",
    "namespace_label": "namespace",
    "node_label": "node",
    "workload_label": "workload"
  },
  "alert_enrichment": {
    "add_labels": {
      "cluster": "production"
    },
    "add_annotations": {
      "runbook_url": "https://runbooks.example.com/gpu-alerts"
    }
  },
  "alert_grouping": {
    "group_by": ["alertname", "namespace"],
    "group_wait": "30s",
    "group_interval": "5m"
  },
  "alert_routing": {
    "receiver": "ops-team",
    "matchers": {
      "severity": "critical"
    }
  }
}
```

**Request Fields:**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | Yes | Unique name for the alert rule |
| `cluster_name` | string | Yes | Target cluster name |
| `enabled` | boolean | No | Enable/disable the rule (default: false) |
| `description` | string | No | Description of the alert rule |
| `labels` | object | No | Additional labels for the rule |
| `namespace` | string | No | VMRule namespace (default: "primus-lens") |
| `auto_sync` | boolean | No | Auto sync to cluster after creation (default: false) |
| `groups` | array | Yes | Array of rule groups |
| `resource_mapping` | object | No | Resource mapping configuration for enriching alerts with resource info |
| `alert_enrichment` | object | No | Alert enrichment configuration for adding labels/annotations |
| `alert_grouping` | object | No | Alert grouping configuration |
| `alert_routing` | object | No | Alert routing configuration |

**Rule Group Fields:**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | Yes | Group name |
| `interval` | string | No | Evaluation interval (e.g., "30s", "1m") |
| `rules` | array | Yes | Array of alert rules |

**Alert Rule Fields:**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `alert` | string | Yes | Alert name |
| `expr` | string | Yes | PromQL/MetricsQL expression |
| `for` | string | No | Duration before firing (e.g., "5m") |
| `labels` | object | No | Labels to attach to alert |
| `annotations` | object | No | Annotations (summary, description, etc.) |

**Resource Mapping Configuration:**

| Field | Type | Description |
|-------|------|-------------|
| `pod_label` | string | Label name containing pod identifier |
| `namespace_label` | string | Label name containing namespace |
| `node_label` | string | Label name containing node identifier |
| `workload_label` | string | Label name containing workload identifier |

**Alert Enrichment Configuration:**

| Field | Type | Description |
|-------|------|-------------|
| `add_labels` | object | Additional labels to add to alerts |
| `add_annotations` | object | Additional annotations to add to alerts |

**Alert Grouping Configuration:**

| Field | Type | Description |
|-------|------|-------------|
| `group_by` | array | Fields to group alerts by |
| `group_wait` | string | Wait time before sending grouped alerts (e.g., "30s") |
| `group_interval` | string | Interval for grouped alerts (e.g., "5m") |

**Alert Routing Configuration:**

| Field | Type | Description |
|-------|------|-------------|
| `receiver` | string | Target receiver for alerts |
| `matchers` | object | Label matchers for routing |

**Response:**

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "rule_id": 123,
    "message": "metric alert rule created successfully"
  },
  "traceId": "trace-abc123"
}
```

**Status Codes:**
- `201 Created` - Rule created successfully
- `400 Bad Request` - Invalid request body or parameters
- `409 Conflict` - Rule with same name already exists
- `500 Internal Server Error` - Server error

**Example:**

```bash
curl -X POST http://localhost:8080/api/metric-alert-rules \
  -H "Content-Type: application/json" \
  -d '{
    "name": "high-gpu-utilization",
    "cluster_name": "production-cluster",
    "enabled": true,
    "auto_sync": true,
    "groups": [{
      "name": "gpu-alerts",
      "interval": "30s",
      "rules": [{
        "alert": "HighGPUUtilization",
        "expr": "gpu_utilization > 0.9",
        "for": "5m"
      }]
    }]
  }'
```

---

### List Metric Alert Rules

Retrieves a paginated list of metric alert rules with filtering support.

**Endpoint:** `GET /api/metric-alert-rules`

**Query Parameters:**

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `name` | string | No | - | Filter by rule name (partial match) |
| `cluster_name` | string | No | - | Filter by cluster name |
| `enabled` | boolean | No | - | Filter by enabled status |
| `sync_status` | string | No | - | Filter by sync status (pending, synced, failed) |
| `pageNum` | integer | No | 1 | Page number |
| `pageSize` | integer | No | 20 | Number of items per page (max: 100) |

**Response:**

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "data": [
      {
        "id": 123,
        "name": "high-gpu-utilization",
        "cluster_name": "production-cluster",
        "enabled": true,
        "description": "Alert when GPU utilization exceeds 90%",
        "groups": [...],
        "labels": {
          "severity": "warning"
        },
        "sync_status": "synced",
        "sync_message": "Successfully synced to cluster",
        "last_sync_at": "2024-01-15T10:30:00Z",
        "created_at": "2024-01-15T09:00:00Z",
        "updated_at": "2024-01-15T10:30:00Z",
        "created_by": "admin",
        "updated_by": "admin"
      }
    ],
    "total": 50,
    "pageNum": 1,
    "pageSize": 20
  },
  "traceId": "trace-abc123"
}
```

**Response Fields:**

| Field | Type | Description |
|-------|------|-------------|
| `id` | integer | Rule ID |
| `name` | string | Rule name |
| `cluster_name` | string | Target cluster name |
| `enabled` | boolean | Whether the rule is enabled |
| `description` | string | Rule description |
| `groups` | array | Rule groups configuration |
| `labels` | object | Rule labels |
| `sync_status` | string | Sync status (pending, synced, failed) |
| `sync_message` | string | Sync status message |
| `last_sync_at` | string | Last sync timestamp (ISO 8601) |
| `created_at` | string | Creation timestamp |
| `updated_at` | string | Last update timestamp |
| `created_by` | string | Creator username |
| `updated_by` | string | Last updater username |

**Status Codes:**
- `200 OK` - Success
- `400 Bad Request` - Invalid parameters
- `500 Internal Server Error` - Server error

**Example:**

```bash
# List all rules
curl -X GET "http://localhost:8080/api/metric-alert-rules?pageNum=1&pageSize=20"

# Filter by cluster and status
curl -X GET "http://localhost:8080/api/metric-alert-rules?cluster_name=production-cluster&enabled=true"

# Filter by sync status
curl -X GET "http://localhost:8080/api/metric-alert-rules?sync_status=failed"
```

---

### Get Metric Alert Rule

Retrieves detailed information for a specific metric alert rule.

**Endpoint:** `GET /api/metric-alert-rules/:id`

**Path Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `id` | integer | Yes | Rule ID |

**Response:**

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "id": 123,
    "name": "high-gpu-utilization",
    "cluster_name": "production-cluster",
    "enabled": true,
    "description": "Alert when GPU utilization exceeds 90%",
    "groups": [
      {
        "name": "gpu-alerts",
        "interval": "30s",
        "rules": [
          {
            "alert": "HighGPUUtilization",
            "expr": "gpu_utilization > 0.9",
            "for": "5m",
            "labels": {
              "severity": "warning"
            },
            "annotations": {
              "summary": "High GPU utilization detected",
              "description": "GPU utilization is above 90%"
            }
          }
        ]
      }
    ],
    "labels": {
      "severity": "warning",
      "team": "infrastructure"
    },
    "sync_status": "synced",
    "sync_message": "Successfully synced to cluster",
    "last_sync_at": "2024-01-15T10:30:00Z",
    "vmrule_uid": "abc-123-def-456",
    "vmrule_status": {...},
    "created_at": "2024-01-15T09:00:00Z",
    "updated_at": "2024-01-15T10:30:00Z"
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
curl -X GET http://localhost:8080/api/metric-alert-rules/123
```

---

### Update Metric Alert Rule

Updates an existing metric alert rule.

**Endpoint:** `PUT /api/metric-alert-rules/:id`

**Path Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `id` | integer | Yes | Rule ID |

**Request Body:**

Same format as create request.

**Response:**

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "rule_id": 123,
    "message": "metric alert rule updated successfully"
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
curl -X PUT http://localhost:8080/api/metric-alert-rules/123 \
  -H "Content-Type: application/json" \
  -d '{
    "name": "high-gpu-utilization",
    "cluster_name": "production-cluster",
    "enabled": true,
    "auto_sync": true,
    "groups": [...]
  }'
```

---

### Delete Metric Alert Rule

Deletes a metric alert rule and removes it from the cluster.

**Endpoint:** `DELETE /api/metric-alert-rules/:id`

**Path Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `id` | integer | Yes | Rule ID |

**Query Parameters:**

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `namespace` | string | No | primus-lens | VMRule namespace |

**Response:**

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "message": "metric alert rule deleted successfully"
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
curl -X DELETE http://localhost:8080/api/metric-alert-rules/123
```

---

### Clone Metric Alert Rule

Clones a metric alert rule to another cluster.

**Endpoint:** `POST /api/metric-alert-rules/:id/clone`

**Path Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `id` | integer | Yes | Source rule ID |

**Request Body:**

```json
{
  "target_cluster_name": "staging-cluster",
  "new_name": "high-gpu-utilization-staging",
  "auto_sync": true
}
```

**Request Fields:**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `target_cluster_name` | string | Yes | Target cluster name |
| `new_name` | string | No | New rule name (defaults to source name) |
| `auto_sync` | boolean | No | Auto sync to target cluster (default: false) |

**Response:**

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "rule_id": 456,
    "message": "metric alert rule cloned successfully"
  },
  "traceId": "trace-abc123"
}
```

**Status Codes:**
- `201 Created` - Rule cloned successfully
- `400 Bad Request` - Invalid request
- `404 Not Found` - Source rule not found
- `409 Conflict` - Rule with same name exists in target cluster
- `500 Internal Server Error` - Server error

**Example:**

```bash
curl -X POST http://localhost:8080/api/metric-alert-rules/123/clone \
  -H "Content-Type: application/json" \
  -d '{
    "target_cluster_name": "staging-cluster",
    "auto_sync": true
  }'
```

---

### Sync Metric Alert Rules

Synchronizes metric alert rules to their target clusters.

**Endpoint:** `POST /api/metric-alert-rules/sync`

**Query Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `cluster_name` | string | Conditional | Cluster name (required if rule_ids not provided) |
| `namespace` | string | No | VMRule namespace (default: primus-lens) |

**Request Body:**

```json
{
  "rule_ids": [123, 456, 789]
}
```

**Request Fields:**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `rule_ids` | array | No | Specific rule IDs to sync (empty means sync all enabled rules in cluster) |

**Response:**

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "total": 3,
    "success": 2,
    "failed": 1,
    "message": "synced 2 rules successfully, 1 failed"
  },
  "traceId": "trace-abc123"
}
```

**Status Codes:**
- `200 OK` - Sync completed (check success/failed counts)
- `400 Bad Request` - Invalid request
- `500 Internal Server Error` - Server error

**Example:**

```bash
# Sync specific rules
curl -X POST http://localhost:8080/api/metric-alert-rules/sync \
  -H "Content-Type: application/json" \
  -d '{"rule_ids": [123, 456]}'

# Sync all enabled rules in a cluster
curl -X POST "http://localhost:8080/api/metric-alert-rules/sync?cluster_name=production-cluster"
```

---

### Get VMRule Status

Retrieves the status of a VMRule from the Kubernetes cluster.

**Endpoint:** `GET /api/metric-alert-rules/:id/status`

**Path Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `id` | integer | Yes | Rule ID |

**Query Parameters:**

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `namespace` | string | No | primus-lens | VMRule namespace |

**Response:**

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "rule_id": 123,
    "name": "high-gpu-utilization",
    "cluster_name": "production-cluster",
    "sync_status": "synced",
    "sync_message": "Successfully synced to cluster",
    "last_sync_at": "2024-01-15T10:30:00Z",
    "vmrule_status": {
      "phase": "Ready",
      "conditions": [
        {
          "type": "Ready",
          "status": "True",
          "lastTransitionTime": "2024-01-15T10:30:00Z",
          "reason": "VMRuleReconciled",
          "message": "VMRule has been successfully reconciled"
        }
      ]
    },
    "status_source": "kubernetes"
  },
  "traceId": "trace-abc123"
}
```

**Response Fields:**

| Field | Type | Description |
|-------|------|-------------|
| `vmrule_status.phase` | string | VMRule phase (Pending, Ready, Failed) |
| `vmrule_status.conditions` | array | Kubernetes condition status |
| `status_source` | string | Data source (kubernetes, database) |

**Status Codes:**
- `200 OK` - Success
- `404 Not Found` - Rule not found
- `500 Internal Server Error` - Server error

**Example:**

```bash
curl -X GET http://localhost:8080/api/metric-alert-rules/123/status
```

---

## Sync Status Values

| Status | Description |
|--------|-------------|
| `pending` | Rule created but not yet synced to cluster |
| `synced` | Rule successfully synced to cluster |
| `failed` | Sync failed (check sync_message for details) |

---

## Data Models

### MetricAlertRule

```go
type MetricAlertRule struct {
    ID              int64                   // Rule ID
    Name            string                  // Rule name
    ClusterName     string                  // Target cluster name
    Enabled         bool                    // Enabled status
    Groups          []VMRuleGroup           // Rule groups
    Description     string                  // Description
    Labels          map[string]string       // Labels
    ResourceMapping ResourceMappingConfig   // Resource mapping configuration
    AlertEnrichment AlertEnrichmentConfig   // Alert enrichment configuration
    AlertGrouping   AlertGroupingConfig     // Alert grouping configuration
    AlertRouting    AlertRoutingConfig      // Alert routing configuration
    SyncStatus      string                  // Sync status (pending, synced, failed)
    SyncMessage     string                  // Sync message
    LastSyncAt      time.Time               // Last sync time
    VMRuleUID       string                  // VMRule UID in Kubernetes
    VMRuleStatus    VMRuleStatus            // VMRule status
    CreatedAt       time.Time               // Creation time
    UpdatedAt       time.Time               // Update time
    CreatedBy       string                  // Creator
    UpdatedBy       string                  // Last updater
}
```

### ResourceMappingConfig

```go
type ResourceMappingConfig struct {
    PodLabel      string // Label name for pod identifier
    NamespaceLabel string // Label name for namespace
    NodeLabel     string // Label name for node identifier
    WorkloadLabel string // Label name for workload identifier
}
```

### AlertEnrichmentConfig

```go
type AlertEnrichmentConfig struct {
    AddLabels      map[string]string // Additional labels to add
    AddAnnotations map[string]string // Additional annotations to add
}
```

### AlertGroupingConfig

```go
type AlertGroupingConfig struct {
    GroupBy       []string // Fields to group alerts by
    GroupWait     string   // Wait time before sending (e.g., "30s")
    GroupInterval string   // Interval for grouped alerts (e.g., "5m")
}
```

### AlertRoutingConfig

```go
type AlertRoutingConfig struct {
    Receiver string            // Target receiver
    Matchers map[string]string // Label matchers for routing
}
```

### VMRuleGroup

```go
type VMRuleGroup struct {
    Name     string      // Group name
    Interval string      // Evaluation interval
    Rules    []VMRule    // Alert rules
}
```

### VMRule

```go
type VMRule struct {
    Alert       string            // Alert name
    Expr        string            // PromQL/MetricsQL expression
    For         string            // Duration before firing
    Labels      map[string]string // Labels
    Annotations map[string]string // Annotations
}
```

---

## Notes

- Rules are stored in the database and can be synced to Kubernetes as VMRule CRDs
- The `namespace` parameter refers to the Kubernetes namespace where VMRule CRDs are created
- Auto-sync automatically applies changes to the cluster when enabled
- Rule names must be unique within a cluster
- The sync process is asynchronous; check `sync_status` to verify completion
- VMRule is a VictoriaMetrics custom resource for managing alert rules
- Failed syncs can be retried using the sync endpoint

---

## Error Handling

Common error responses:

```json
{
  "code": 409,
  "message": "rule with same name already exists in this cluster",
  "traceId": "trace-abc123"
}
```

```json
{
  "code": 400,
  "message": "at least one rule group is required",
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

1. **Rule Naming**: Use descriptive names that indicate what the rule monitors
2. **Testing**: Test rules in a development cluster before deploying to production
3. **Labels**: Add appropriate labels (severity, team, etc.) for routing and filtering
4. **Intervals**: Choose appropriate evaluation intervals based on metric frequency
5. **For Duration**: Set reasonable `for` durations to avoid alert flapping
6. **Sync Management**: Use auto_sync cautiously; manual sync provides more control
7. **Monitoring**: Regularly check sync_status to ensure rules are properly deployed

---

## Integration Examples

### Python

```python
import requests

API_BASE = "http://localhost:8080/api"

def create_alert_rule(name, cluster, expr):
    rule = {
        "name": name,
        "cluster_name": cluster,
        "enabled": True,
        "auto_sync": True,
        "groups": [{
            "name": "default",
            "interval": "30s",
            "rules": [{
                "alert": name,
                "expr": expr,
                "for": "5m"
            }]
        }]
    }
    
    response = requests.post(f"{API_BASE}/metric-alert-rules", json=rule)
    return response.json()

# Create a rule
result = create_alert_rule(
    "high-gpu-temp",
    "production-cluster",
    "gpu_temperature > 85"
)
print(f"Rule created with ID: {result['data']['rule_id']}")
```

### Bash Script

```bash
#!/bin/bash
API_BASE="http://localhost:8080/api"

# Create alert rule
create_rule() {
  curl -X POST "$API_BASE/metric-alert-rules" \
    -H "Content-Type: application/json" \
    -d "{
      \"name\": \"$1\",
      \"cluster_name\": \"$2\",
      \"enabled\": true,
      \"auto_sync\": true,
      \"groups\": [{
        \"name\": \"default\",
        \"rules\": [{
          \"alert\": \"$1\",
          \"expr\": \"$3\",
          \"for\": \"5m\"
        }]
      }]
    }"
}

# List all rules
list_rules() {
  curl -s "$API_BASE/metric-alert-rules?pageSize=100" | jq '.data.data'
}

# Sync rules for a cluster
sync_cluster() {
  curl -X POST "$API_BASE/metric-alert-rules/sync?cluster_name=$1"
}

# Usage
create_rule "high-gpu-memory" "production" "gpu_memory_used > 0.9"
list_rules
sync_cluster "production"
```

