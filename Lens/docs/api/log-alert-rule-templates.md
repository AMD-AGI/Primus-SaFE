# Log Alert Rule Templates API

The Log Alert Rule Templates API provides operations for managing predefined log alert rule templates. Templates allow users to quickly create log alert rules from pre-configured patterns for common monitoring scenarios.

## Endpoints

### List Log Alert Rule Templates

Retrieves a list of available log alert rule templates.

**Endpoint:** `GET /api/log-alert-rule-templates`

**Query Parameters:**

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `category` | string | No | - | Filter by template category |
| `cluster` | string | No | current | Target cluster name |

**Response:**

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "templates": [
      {
        "id": 1,
        "name": "High Error Rate",
        "category": "errors",
        "description": "Detects when error log count exceeds threshold",
        "template_config": {
          "label_selectors": [
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
          "severity": "warning",
          "alert_template": {
            "title": "High Error Rate Detected",
            "message": "Error count exceeded threshold in the last 5 minutes"
          }
        },
        "tags": ["errors", "monitoring", "production"],
        "is_builtin": true,
        "usage_count": 45,
        "created_at": "2024-01-01T00:00:00Z",
        "created_by": "system"
      },
      {
        "id": 2,
        "name": "OOM Killer",
        "category": "system",
        "description": "Detects Out of Memory (OOM) events",
        "template_config": {
          "label_selectors": [
            {
              "label": "source",
              "operator": "=",
              "value": "kernel"
            }
          ],
          "match_type": "regex",
          "match_config": {
            "pattern": "Out of memory|OOM killer|oom-kill",
            "case_sensitive": false
          },
          "severity": "critical",
          "alert_template": {
            "title": "OOM Event Detected",
            "message": "Out of memory condition detected on {{ .node }}"
          }
        },
        "tags": ["oom", "memory", "system", "critical"],
        "is_builtin": true,
        "usage_count": 32,
        "created_at": "2024-01-01T00:00:00Z",
        "created_by": "system"
      }
    ],
    "total": 15
  },
  "traceId": "trace-abc123"
}
```

**Response Fields:**

| Field | Type | Description |
|-------|------|-------------|
| `id` | integer | Template ID |
| `name` | string | Template name |
| `category` | string | Template category (errors, system, security, performance, etc.) |
| `description` | string | Template description |
| `template_config` | object | Template configuration (rule definition) |
| `tags` | array | Tags for categorization and search |
| `is_builtin` | boolean | Whether it's a built-in template |
| `usage_count` | integer | Number of times template has been used |
| `created_at` | string | Creation timestamp (ISO 8601) |
| `created_by` | string | Creator username |

**Status Codes:**
- `200 OK` - Success
- `500 Internal Server Error` - Server error

**Example:**

```bash
# List all templates
curl -X GET http://localhost:8080/api/log-alert-rule-templates

# Filter by category
curl -X GET "http://localhost:8080/api/log-alert-rule-templates?category=errors"
```

---

### Get Log Alert Rule Template

Retrieves detailed information for a specific template.

**Endpoint:** `GET /api/log-alert-rule-templates/:id`

**Path Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `id` | integer | Yes | Template ID |

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
    "id": 1,
    "name": "High Error Rate",
    "category": "errors",
    "description": "Detects when error log count exceeds threshold. Useful for monitoring application health and identifying issues early.",
    "template_config": {
      "label_selectors": [
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
      "severity": "warning",
      "alert_template": {
        "title": "High Error Rate Detected",
        "message": "Error count exceeded {{ .threshold }} in the last {{ .window }}",
        "receivers": ["ops-team"]
      }
    },
    "tags": ["errors", "monitoring", "production"],
    "is_builtin": true,
    "usage_count": 45,
    "created_at": "2024-01-01T00:00:00Z",
    "updated_at": "2024-01-01T00:00:00Z",
    "created_by": "system"
  },
  "traceId": "trace-abc123"
}
```

**Status Codes:**
- `200 OK` - Success
- `404 Not Found` - Template not found
- `500 Internal Server Error` - Server error

**Example:**

```bash
curl -X GET http://localhost:8080/api/log-alert-rule-templates/1
```

---

### Create Log Alert Rule Template

Creates a new custom log alert rule template.

**Endpoint:** `POST /api/log-alert-rule-templates`

**Query Parameters:**

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `cluster` | string | No | current | Target cluster name |

**Request Body:**

```json
{
  "name": "High Request Latency",
  "category": "performance",
  "description": "Detects when API request latency exceeds threshold",
  "template_config": {
    "label_selectors": [
      {
        "label": "component",
        "operator": "=",
        "value": "api"
      }
    ],
    "match_type": "regex",
    "match_config": {
      "pattern": "latency=(\\d+)ms",
      "threshold": 1000,
      "case_sensitive": false
    },
    "severity": "warning",
    "alert_template": {
      "title": "High API Latency",
      "message": "Request latency exceeded 1000ms"
    }
  },
  "tags": ["performance", "latency", "api"],
  "created_by": "admin"
}
```

**Request Fields:**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | Yes | Template name |
| `category` | string | Yes | Template category |
| `description` | string | No | Template description |
| `template_config` | object | Yes | Template configuration |
| `tags` | array | No | Tags for categorization |
| `created_by` | string | No | Creator username |

**Response:**

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "template_id": 10
  },
  "traceId": "trace-abc123"
}
```

**Status Codes:**
- `201 Created` - Template created successfully
- `400 Bad Request` - Invalid request body
- `500 Internal Server Error` - Server error

**Example:**

```bash
curl -X POST http://localhost:8080/api/log-alert-rule-templates \
  -H "Content-Type: application/json" \
  -d '{
    "name": "High Request Latency",
    "category": "performance",
    "template_config": {
      "label_selectors": [
        {"label": "component", "operator": "=", "value": "api"}
      ],
      "match_type": "count",
      "match_config": {
        "threshold": 100,
        "window": "5m",
        "comparison": "gt"
      }
    },
    "tags": ["performance", "api"]
  }'
```

---

### Delete Log Alert Rule Template

Deletes a custom log alert rule template. Built-in templates cannot be deleted.

**Endpoint:** `DELETE /api/log-alert-rule-templates/:id`

**Path Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `id` | integer | Yes | Template ID |

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
    "message": "template deleted successfully"
  },
  "traceId": "trace-abc123"
}
```

**Status Codes:**
- `200 OK` - Success
- `403 Forbidden` - Cannot delete built-in template
- `404 Not Found` - Template not found
- `500 Internal Server Error` - Server error

**Example:**

```bash
curl -X DELETE http://localhost:8080/api/log-alert-rule-templates/10
```

---

### Create Rule from Template (Instantiate)

Creates a new log alert rule based on a template.

**Endpoint:** `POST /api/log-alert-rule-templates/:id/instantiate`

**Path Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `id` | integer | Yes | Template ID |

**Query Parameters:**

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `cluster` | string | No | current | Target cluster name |

**Request Body:**

```json
{
  "name": "prod-api-high-error-rate",
  "description": "High error rate alert for production API",
  "enabled": true,
  "priority": 3,
  "overrides": {
    "label_selectors": [
      {
        "label": "environment",
        "operator": "=",
        "value": "production"
      },
      {
        "label": "level",
        "operator": "=",
        "value": "error"
      }
    ],
    "severity": "critical",
    "priority": 1,
    "group_by": ["service", "namespace"]
  },
  "created_by": "admin"
}
```

**Request Fields:**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | Yes | New rule name |
| `description` | string | No | Rule description |
| `enabled` | boolean | No | Enable the rule (default: false) |
| `priority` | integer | No | Rule priority 1-10 (default: 5) |
| `overrides` | object | No | Override template configuration |
| `created_by` | string | No | Creator username |

**Override Fields:**

The `overrides` object can override any field from the template configuration:
- `label_selectors` - Replace or add label selectors
- `severity` - Override alert severity
- `priority` - Override rule priority
- `group_by` - Override grouping fields
- Any other fields from `match_config` or `alert_template`

**Response:**

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "rule_id": 234,
    "cluster_name": "production-cluster"
  },
  "traceId": "trace-abc123"
}
```

**Status Codes:**
- `201 Created` - Rule created successfully
- `400 Bad Request` - Invalid request
- `404 Not Found` - Template not found
- `500 Internal Server Error` - Server error

**Example:**

```bash
curl -X POST http://localhost:8080/api/log-alert-rule-templates/1/instantiate \
  -H "Content-Type: application/json" \
  -d '{
    "name": "prod-high-error-rate",
    "enabled": true,
    "priority": 1,
    "overrides": {
      "label_selectors": [
        {"label": "environment", "operator": "=", "value": "production"}
      ],
      "severity": "critical"
    }
  }'
```

---

## Template Categories

Common template categories include:

| Category | Description | Example Templates |
|----------|-------------|-------------------|
| `errors` | Error detection and monitoring | High error rate, exception detection |
| `system` | System-level events | OOM killer, disk full, kernel panics |
| `security` | Security-related events | Failed login attempts, unauthorized access |
| `performance` | Performance monitoring | High latency, slow queries |
| `network` | Network-related events | Connection timeouts, DNS failures |
| `application` | Application-specific events | Service crashes, health check failures |
| `database` | Database events | Slow queries, connection pool exhaustion |
| `kubernetes` | Kubernetes events | Pod evictions, node failures |

---

## Built-in Templates

### Error Monitoring Templates

**1. High Error Rate**
- Detects when error log count exceeds threshold
- Category: `errors`
- Match Type: `count`
- Severity: `warning`

**2. Critical Errors**
- Detects critical or fatal error messages
- Category: `errors`
- Match Type: `regex`
- Severity: `critical`

**3. Exception Tracking**
- Detects unhandled exceptions in application logs
- Category: `errors`
- Match Type: `regex`
- Severity: `warning`

### System Templates

**4. OOM Killer**
- Detects Out of Memory (OOM) events
- Category: `system`
- Match Type: `regex`
- Severity: `critical`

**5. Disk Space Alert**
- Detects disk space warnings
- Category: `system`
- Match Type: `keyword`
- Severity: `warning`

**6. Kernel Panic**
- Detects kernel panic events
- Category: `system`
- Match Type: `regex`
- Severity: `critical`

### Security Templates

**7. Failed Login Attempts**
- Detects multiple failed authentication attempts
- Category: `security`
- Match Type: `count`
- Severity: `warning`

**8. Unauthorized Access**
- Detects unauthorized access attempts
- Category: `security`
- Match Type: `regex`
- Severity: `critical`

### Performance Templates

**9. High Latency**
- Detects requests with high latency
- Category: `performance`
- Match Type: `regex`
- Severity: `warning`

**10. Slow Query**
- Detects slow database queries
- Category: `database`
- Match Type: `regex`
- Severity: `warning`

---

## Data Models

### LogAlertRuleTemplate

```go
type LogAlertRuleTemplate struct {
    ID             int64       // Template ID
    Name           string      // Template name
    Category       string      // Template category
    Description    string      // Description
    TemplateConfig interface{} // Template configuration
    Tags           []string    // Tags
    IsBuiltin      bool        // Is built-in template
    UsageCount     int         // Usage counter
    CreatedAt      time.Time   // Creation time
    UpdatedAt      time.Time   // Update time
    CreatedBy      string      // Creator
}
```

### Template Configuration Structure

```go
type TemplateConfig struct {
    LabelSelectors []LabelSelector      // Label selectors
    MatchType      string               // Match type
    MatchConfig    interface{}          // Match configuration
    Severity       string               // Alert severity
    AlertTemplate  AlertTemplate        // Alert template
    GroupBy        []string             // Group by fields (optional)
    RouteConfig    map[string]interface{} // Route config (optional)
}
```

---

## Notes

- Built-in templates cannot be modified or deleted
- Custom templates can be created, modified, and deleted
- Templates support the same match types as log alert rules
- The `overrides` parameter allows customization during instantiation
- Usage count is automatically incremented when a template is used
- Templates are cluster-scoped (can be different per cluster)
- Template configurations follow the same structure as log alert rules

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
  "message": "category is required",
  "traceId": "trace-abc123"
}
```

```json
{
  "code": 403,
  "message": "cannot delete builtin template",
  "traceId": "trace-abc123"
}
```

```json
{
  "code": 404,
  "message": "template not found",
  "traceId": "trace-abc123"
}
```

---

## Best Practices

1. **Template Design**
   - Keep templates generic and reusable
   - Use clear, descriptive names
   - Provide comprehensive descriptions
   - Include usage examples in descriptions

2. **Categorization**
   - Use standard categories for consistency
   - Apply relevant tags for discoverability
   - Group related templates together

3. **Configuration**
   - Use reasonable default thresholds
   - Make templates easily customizable via overrides
   - Include sensible default alert messages
   - Consider different severity levels

4. **Instantiation**
   - Always override label selectors to match your environment
   - Adjust thresholds based on your workload
   - Use descriptive names for instantiated rules
   - Set appropriate priorities

5. **Maintenance**
   - Regularly review template effectiveness
   - Update templates based on feedback
   - Remove unused custom templates
   - Monitor template usage statistics

---

## Integration Examples

### Python

```python
import requests

API_BASE = "http://localhost:8080/api"

def list_templates(category=None):
    """List available templates"""
    params = {}
    if category:
        params["category"] = category
    
    response = requests.get(
        f"{API_BASE}/log-alert-rule-templates",
        params=params
    )
    return response.json()["data"]["templates"]

def create_rule_from_template(template_id, name, overrides=None):
    """Create a rule from a template"""
    data = {
        "name": name,
        "enabled": True,
        "overrides": overrides or {}
    }
    
    response = requests.post(
        f"{API_BASE}/log-alert-rule-templates/{template_id}/instantiate",
        json=data
    )
    return response.json()

# List all error templates
error_templates = list_templates(category="errors")
print(f"Found {len(error_templates)} error templates")

# Create rule from template with customization
rule = create_rule_from_template(
    template_id=1,
    name="prod-high-error-rate",
    overrides={
        "label_selectors": [
            {"label": "environment", "operator": "=", "value": "production"},
            {"label": "level", "operator": "=", "value": "error"}
        ],
        "severity": "critical",
        "priority": 1
    }
)
print(f"Created rule: {rule['data']['rule_id']}")

# Create custom template
def create_template(name, category, config):
    template = {
        "name": name,
        "category": category,
        "description": f"Custom {name} template",
        "template_config": config,
        "tags": [category, "custom"],
        "created_by": "automation"
    }
    
    response = requests.post(
        f"{API_BASE}/log-alert-rule-templates",
        json=template
    )
    return response.json()

# Create a custom performance template
perf_config = {
    "label_selectors": [
        {"label": "component", "operator": "=", "value": "api"}
    ],
    "match_type": "count",
    "match_config": {
        "threshold": 1000,
        "window": "5m",
        "comparison": "gt"
    },
    "severity": "warning"
}

custom_template = create_template(
    "API Request Spike",
    "performance",
    perf_config
)
print(f"Created template: {custom_template['data']['template_id']}")
```

### Bash Script

```bash
#!/bin/bash
API_BASE="http://localhost:8080/api"

# List all templates
list_templates() {
  curl -s "$API_BASE/log-alert-rule-templates" \
    | jq '.data.templates[] | {id, name, category, usage_count}'
}

# Get template details
get_template() {
  curl -s "$API_BASE/log-alert-rule-templates/$1" \
    | jq '.data'
}

# Create rule from template
instantiate_template() {
  TEMPLATE_ID=$1
  RULE_NAME=$2
  
  curl -X POST "$API_BASE/log-alert-rule-templates/$TEMPLATE_ID/instantiate" \
    -H "Content-Type: application/json" \
    -d "{
      \"name\": \"$RULE_NAME\",
      \"enabled\": true,
      \"overrides\": {
        \"severity\": \"critical\",
        \"priority\": 1
      }
    }"
}

# Create custom template
create_template() {
  curl -X POST "$API_BASE/log-alert-rule-templates" \
    -H "Content-Type: application/json" \
    -d '{
      "name": "'"$1"'",
      "category": "'"$2"'",
      "template_config": {
        "label_selectors": [
          {"label": "level", "operator": "=", "value": "error"}
        ],
        "match_type": "count",
        "match_config": {
          "threshold": 100,
          "window": "5m",
          "comparison": "gt"
        }
      },
      "tags": ["'"$2"'", "custom"]
    }'
}

# Usage examples
echo "Listing all templates:"
list_templates

echo -e "\nGetting template 1 details:"
get_template 1

echo -e "\nCreating rule from template:"
instantiate_template 1 "prod-high-error-rate"

echo -e "\nCreating custom template:"
create_template "My Custom Alert" "custom"
```

### Go

```go
package main

import (
    "bytes"
    "encoding/json"
    "fmt"
    "io/ioutil"
    "net/http"
)

const apiBase = "http://localhost:8080/api"

type TemplateResponse struct {
    Code    int    `json:"code"`
    Message string `json:"message"`
    Data    struct {
        Templates []Template `json:"templates"`
    } `json:"data"`
}

type Template struct {
    ID          int64  `json:"id"`
    Name        string `json:"name"`
    Category    string `json:"category"`
    Description string `json:"description"`
    UsageCount  int    `json:"usage_count"`
}

func listTemplates(category string) ([]Template, error) {
    url := fmt.Sprintf("%s/log-alert-rule-templates", apiBase)
    if category != "" {
        url += "?category=" + category
    }
    
    resp, err := http.Get(url)
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()
    
    var result TemplateResponse
    if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
        return nil, err
    }
    
    return result.Data.Templates, nil
}

func instantiateTemplate(templateID int64, name string, overrides map[string]interface{}) error {
    data := map[string]interface{}{
        "name":      name,
        "enabled":   true,
        "overrides": overrides,
    }
    
    jsonData, _ := json.Marshal(data)
    url := fmt.Sprintf("%s/log-alert-rule-templates/%d/instantiate", apiBase, templateID)
    
    resp, err := http.Post(url, "application/json", bytes.NewBuffer(jsonData))
    if err != nil {
        return err
    }
    defer resp.Body.Close()
    
    body, _ := ioutil.ReadAll(resp.Body)
    fmt.Printf("Response: %s\n", body)
    
    return nil
}

func main() {
    // List error templates
    templates, err := listTemplates("errors")
    if err != nil {
        panic(err)
    }
    
    fmt.Printf("Found %d error templates:\n", len(templates))
    for _, t := range templates {
        fmt.Printf("- %s (ID: %d, Used: %d times)\n", t.Name, t.ID, t.UsageCount)
    }
    
    // Create rule from template
    overrides := map[string]interface{}{
        "severity": "critical",
        "priority": 1,
    }
    
    err = instantiateTemplate(1, "prod-high-error-rate", overrides)
    if err != nil {
        panic(err)
    }
}
```

---

## Use Cases

### Rapid Deployment
Quickly deploy standard monitoring rules across multiple clusters using templates.

### Consistency
Ensure consistent alert configurations across teams and environments.

### Best Practices
Share proven alert patterns through built-in and custom templates.

### Onboarding
Help new users get started with pre-configured monitoring patterns.

### Standardization
Establish organizational standards for common monitoring scenarios.

---

## Future Enhancements

Planned features for future releases:

1. **Template Versioning**: Track template changes over time
2. **Template Sharing**: Share templates across clusters
3. **Template Marketplace**: Community-contributed templates
4. **Template Testing**: Validate templates with sample data
5. **Template Metrics**: Detailed usage analytics and effectiveness
6. **Template Recommendations**: AI-powered template suggestions
7. **Template Import/Export**: Backup and restore templates
8. **Template Inheritance**: Create templates based on other templates

