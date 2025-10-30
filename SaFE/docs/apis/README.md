# Primus-SaFE API Documentation

Welcome to the Primus-SaFE API documentation. This documentation provides complete REST API interface specifications.

## Documentation Structure

### Main Index
- [index.md](./index.md) - API overview and common instructions

### Core Business APIs

#### Workload Management
- [workload.md](./workload.md) - Workload API (training, inference, deployment)
- [cluster.md](./cluster.md) - Cluster API
- [workspace.md](./workspace.md) - Workspace API

#### Node Management
- [node.md](./node.md) - Node API
- [node-flavor.md](./node-flavor.md) - Node Flavor API
- [node-template.md](./node-template.md) - Node Template API

#### User and Security
- [user.md](./user.md) - User Management API
- [secret.md](./secret.md) - Secret Management API
- [public-key.md](./public-key.md) - Public Key Management API

#### Operations
- [fault.md](./fault.md) - Fault Injection API
- [ops-job.md](./ops-job.md) - Operational Job API
- [service.md](./service.md) - Service API
- [log.md](./log.md) - Log Query API

### Image Management APIs
- [image.md](./image.md) - Image Management API
- [image-registry.md](./image-registry.md) - Image Registry API

### Terminal Access API
- [webshell.md](./webshell.md) - WebShell API

## Quick Start

### 1. User Registration and Login

```bash
# Register user
curl -X POST http://api.example.com/api/custom/users \
  -H "Content-Type: application/json" \
  -d '{
    "name": "zhangsan",
    "password": "Password123!",
    "email": "zhangsan@example.com"
  }'

# Login to get token
curl -X POST http://api.example.com/api/custom/login \
  -H "Content-Type: application/json" \
  -d '{
    "name": "zhangsan",
    "password": "Password123!"
  }'
```

### 2. Create Workload

```bash
curl -X POST http://api.example.com/api/custom/workloads \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "displayName": "my-training-job",
    "workspaceId": "workspace-001",
    "groupVersionKind": {
      "kind": "PyTorchJob",
      "version": "v1"
    },
    "image": "pytorch/pytorch:2.0",
    "resource": {
      "cpu": "16",
      "gpu": "2",
      "memory": "64Gi",
      "replica": 1
    }
  }'
```

### 3. Query Workload Status

```bash
curl -X GET "http://api.example.com/api/custom/workloads?workspaceId=workspace-001" \
  -H "Authorization: Bearer YOUR_TOKEN"
```

## API Call Conventions

### Request Headers

```
Authorization: Bearer <token>
Content-Type: application/json
```

### Authentication Methods

**Token Authentication** (recommended for API calls):
```bash
curl -H "Authorization: Bearer YOUR_TOKEN" ...
```

**Cookie Authentication** (automatically used by Web Console):
Browser automatically carries cookies, no manual setup needed.

### Error Handling

API error response format:

```json
{
  "code": 400,
  "message": "Detailed error message"
}
```

Common error codes:
- `400` - Bad request parameters
- `401` - Unauthenticated
- `403` - Permission denied
- `404` - Resource not found
- `500` - Internal server error

## Best Practices

### 1. Resource Naming
- Use meaningful names
- Avoid special characters
- Maintain consistent naming style

### 2. Error Retry
- Network errors: recommend retry
- 4xx errors: should not retry, need to correct request
- 5xx errors: can retry, but need backoff strategy

### 3. Pagination Queries
- Use limit and offset to control data volume
- Avoid querying large amounts of data at once

### 4. Concurrency Control
- Use batch interfaces for batch operations
- Avoid large number of requests in short time

## Code Examples

### Python

```python
import requests

# Login
response = requests.post(
    'http://api.example.com/api/custom/login',
    json={'name': 'zhangsan', 'password': 'Password123!'}
)
token = response.json()['token']

# Create workload
headers = {'Authorization': f'Bearer {token}'}
workload = {
    'displayName': 'my-job',
    'workspaceId': 'workspace-001',
    'groupVersionKind': {'kind': 'PyTorchJob', 'version': 'v1'},
    'image': 'pytorch/pytorch:2.0',
    'resource': {'cpu': '16', 'gpu': '2', 'memory': '64Gi', 'replica': 1}
}
response = requests.post(
    'http://api.example.com/api/custom/workloads',
    headers=headers,
    json=workload
)
print(response.json())
```

### Go

```go
package main

import (
    "bytes"
    "encoding/json"
    "net/http"
)

type LoginRequest struct {
    Name     string `json:"name"`
    Password string `json:"password"`
}

type LoginResponse struct {
    Token string `json:"token"`
}

func main() {
    // Login
    loginReq := LoginRequest{Name: "zhangsan", Password: "Password123!"}
    body, _ := json.Marshal(loginReq)
    
    resp, _ := http.Post(
        "http://api.example.com/api/custom/login",
        "application/json",
        bytes.NewBuffer(body),
    )
    
    var loginResp LoginResponse
    json.NewDecoder(resp.Body).Decode(&loginResp)
    
    // Use token to access API
    req, _ := http.NewRequest("GET", "http://api.example.com/api/custom/workloads", nil)
    req.Header.Set("Authorization", "Bearer "+loginResp.Token)
    
    client := &http.Client{}
    client.Do(req)
}
```

## Changelog

- **2025-01** - Initial release

## Getting Help

For questions, please:
1. Check relevant sections in this documentation
2. Contact technical support: support@amd.com
3. Submit issues: [GitHub Issues](https://github.com/AMD-AIG-AIMA/SAFE/issues)

## License

Copyright (C) 2025 Advanced Micro Devices, Inc. All rights reserved.
