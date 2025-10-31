# Primus-SaFE API Documentation

## Overview

Primus-SaFE is a Kubernetes-based AI workload management platform that provides comprehensive cluster management, workspace management, and workload scheduling capabilities.

## API Base URL

```
http://<server-address>
```

## Authentication

Most APIs require authentication. The following authentication methods are supported:

- **Token Authentication**: Add `Authorization: Bearer <token>` in request header
- **Cookie Authentication**: Used by Web Console, cookies are managed automatically

## API Modules

### 1. Core Business APIs

Base Path: `/api/v1`

- [Workload API](./workload.md) - Manage various workloads (training jobs, inference services, deployments, etc.)
- [Cluster API](./cluster.md) - Kubernetes cluster creation and management
- [Workspace API](./workspace.md) - Workspace creation and resource quota management
- [Node API](./node.md) - Node registration, management and monitoring
- [NodeFlavor API](./node-flavor.md) - Node flavor configuration management
- [NodeTemplate API](./node-template.md) - Node environment template management
- [User API](./user.md) - User management and authentication
- [Secret API](./secret.md) - SSH and image registry secret management
- [Fault API](./fault.md) - Fault injection and management
- [OpsJob API](./ops-job.md) - Operational job management
- [PublicKey API](./public-key.md) - SSH public key management
- [Log API](./log.md) - Log query interfaces

### 2. Image Management APIs

Base Path: `/api/v1`

- [Image API](./image.md) - Image management and import
- [Image Registry API](./image-registry.md) - Image registry configuration

### 3. WebShell API

Base Path: `/api/v1`

- [WebShell API](./webshell.md) - Web terminal interface

## Common Response Format

### Success Response

custom content defined by the API, with an HTTP status code of 200.

### Error Response

```json
{
  "errorCode": "Primus.xxx",
  "errorMessage": ""
}
```

## Common HTTP Status Codes

- `200 OK` - Request successful
- `201 Created` - Resource created successfully
- `400 Bad Request` - Invalid request parameters
- `401 Unauthorized` - Not authenticated or authentication failed
- `403 Forbidden` - Permission denied
- `404 Not Found` - Resource not found
- `409 Conflict` - Resource conflict
- `500 Internal Server Error` - Internal server error

## Data Type Conventions

### Time Format

All time fields use RFC3339 format: `2006-01-02T15:04:05`

### Resource Units

- **CPU**: Number of cores, e.g., `"128"` means 128 cores
- **GPU**: Number of cards, e.g., `"8"` means 8 cards
- **Memory**: Byte units, e.g., `"128Gi"` means 128GB memory
- **Storage**: Byte units, e.g., `"50Gi"` means 50GB storage

### Pagination Parameters

- `limit`: Number of records per page, default 100
- `offset`: Number of records to skip, default 0

## Version History

- **v1.0** (2025-10) - Initial release

## Contact

For questions, please contact:
- Project Repository: [GitHub](https://github.com/AMD-AGI/Primus-SaFE)
