# Service API

Service-related interfaces provide log query and other functions.

## API List

### 1. Get Service Logs

Query system service logs.

**Endpoint**: `POST /api/custom/service/:name/logs`

**Authentication Required**: Yes

**Path Parameters**:
- `name`: Service name


---

## Supported Services

- `primus-safe-apiserver`
- `primus-safe-resource-manager`
- `primus-safe-job-manager`
- `primus-safe-webhooks`
