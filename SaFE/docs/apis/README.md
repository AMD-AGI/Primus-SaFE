# Primus-SaFE API Documentation

Welcome to the Primus-SaFE API documentation. This documentation provides complete REST API interface specifications.

## Documentation Structure

### Main Index
- [index.md](./index.md) - API overview and common instructions

### Core Business APIs

#### Resource Management
- [workload.md](./workload.md) - Workload API
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
curl -X POST http://api.example.com/api/v1/users \
  -H "Content-Type: application/json" \
  -d '{
    "name": "zhangsan",
    "password": "Password123!",
    "email": "zhangsan@example.com"
  }'

# Login to get token
curl -X POST http://api.example.com/api/v1/login \
  -H "Content-Type: application/json" \
  -d '{
    "name": "zhangsan",
    "password": "Password123!"
  }'
```

### 2. Create Workload

```bash
curl -X POST http://api.example.com/api/v1/workloads \
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
    "entryPoint": "YmFzaCBydW4uc2gK",
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
curl -X GET "http://api.example.com/api/v1/workloads?workspaceId=workspace-001" \
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
  "errorCode": "Primus.xxx",
  "errorMessage": ""
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
- Use DNS-compliant names: letters, numbers, and hyphens only
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


## Changelog

- **2025-10** - Initial release

## Getting Help

For questions, please:
1. Check relevant sections in this documentation
2. Submit issues: [GitHub Issues](https://github.com/AMD-AGI/Primus-SaFE)

## License

Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.