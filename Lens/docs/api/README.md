# Primus-Lens API Documentation

This directory contains comprehensive API documentation for the Primus-Lens RESTful API service. The API provides endpoints for cluster monitoring, node management, workload tracking, and GPU resource management.

## API Groups

The Primus-Lens API is organized into the following groups:

### [Clusters API](./clusters.md)
Cluster-level operations including overview, GPU statistics, and consumer information.

**Endpoints:**
- `GET /api/clusters/overview` - Get cluster overview with node statistics and resource utilization
- `GET /api/clusters/consumers` - List GPU consumers (workloads) with pagination
- `GET /api/clusters/gpuHeatmap` - Get GPU heatmap with top K power/utilization/temperature data

### [Nodes API](./nodes.md)
Node-level operations for GPU nodes, device information, and metrics.

**Endpoints:**
- `GET /api/nodes` - List all GPU nodes with filtering and pagination
- `GET /api/nodes/:name` - Get detailed information for a specific node
- `GET /api/nodes/:name/gpuDevices` - Get GPU devices for a specific node
- `GET /api/nodes/:name/gpuMetrics` - Get GPU metrics history for a node
- `GET /api/nodes/:name/workloads` - List workloads running on a node
- `GET /api/nodes/:name/workloadsHistory` - Get historical workload data for a node
- `GET /api/nodes/gpuAllocation` - Get GPU allocation information across all nodes
- `GET /api/nodes/gpuUtilization` - Get cluster-wide GPU utilization statistics
- `GET /api/nodes/gpuUtilizationHistory` - Get historical GPU utilization data

### [Workloads API](./workloads.md)
Workload management including listing, details, hierarchy, and metrics.

**Endpoints:**
- `GET /api/workloads` - List workloads with filtering and pagination
- `GET /api/workloads/:uid` - Get detailed information for a specific workload
- `GET /api/workloads/:uid/hierarchy` - Get workload hierarchy (parent-child relationships)
- `GET /api/workloads/:uid/metrics` - Get metrics for a specific workload
- `GET /api/workloads/:uid/trainingPerformance` - Get training performance data for AI/ML workloads
- `GET /api/workloadMetadata` - Get metadata (namespaces, kinds) for workload filtering

### [Storage API](./storage.md)
Storage statistics and management operations.

**Endpoints:**
- `GET /api/storage/stat` - Get storage statistics including capacity and usage

## Base URL

All API endpoints are relative to the base URL of your Primus-Lens API service:

```
http://<api-server-host>:<port>/api
```

Default port: `8080`

## Authentication

Currently, the Primus-Lens API does not require authentication. This may change in future releases. Ensure your API service is properly secured through network policies or a reverse proxy.

## Request Format

All requests should include the appropriate `Content-Type` header:

```
Content-Type: application/json
```

## Response Format

All API responses follow a standard format:

### Success Response

```json
{
  "code": 0,
  "message": "success",
  "data": { /* response data */ },
  "traceId": "trace-id-here"
}
```

### Error Response

```json
{
  "code": <error_code>,
  "message": "<error_message>",
  "traceId": "trace-id-here"
}
```

**Common Error Codes:**
- `400` - Bad Request (invalid parameters)
- `404` - Not Found (resource does not exist)
- `500` - Internal Server Error

## Pagination

List endpoints support pagination using the following query parameters:

- `pageNum` (integer): Page number (default: 1)
- `pageSize` (integer): Number of items per page (default: 10)

**Example:**
```
GET /api/nodes?pageNum=1&pageSize=20
```

**Paginated Response:**
```json
{
  "code": 0,
  "data": {
    "data": [ /* array of items */ ],
    "total": 100
  }
}
```

## Time Parameters

Many endpoints accept time range parameters for querying historical data:

- `start` (int64): Start timestamp in Unix seconds or milliseconds (depending on endpoint)
- `end` (int64): End timestamp in Unix seconds or milliseconds
- `step` (int): Query resolution in seconds (default: 60)

**Example:**
```
GET /api/nodes/node-1/gpuMetrics?start=1609459200&end=1609545600&step=300
```

## Filtering

List endpoints support filtering using query parameters. Supported filters vary by endpoint:

- `name` - Filter by name (partial match)
- `namespace` - Filter by namespace
- `kind` - Filter by resource kind
- `status` - Filter by status

**Example:**
```
GET /api/workloads?namespace=default&status=Running&pageNum=1&pageSize=10
```

## Rate Limiting

Currently, there are no rate limits enforced. However, it is recommended to implement rate limiting at the infrastructure level (e.g., via API gateway or reverse proxy) for production deployments.

## Monitoring and Metrics

The API service exposes Prometheus metrics at:

```
http://<api-server-host>:9090/metrics
```

Key metrics include:
- `http_requests_total` - Total HTTP requests by method, path, and status
- `http_request_duration_seconds` - Request latency histogram
- `http_requests_in_flight` - Current number of requests being processed

## Health Check

The API service provides health check endpoints:

```
GET /healthz
```

Returns `200 OK` if the service is healthy.

## Examples

### Get Cluster Overview

```bash
curl -X GET http://localhost:8080/api/clusters/overview
```

### List GPU Nodes

```bash
curl -X GET "http://localhost:8080/api/nodes?pageNum=1&pageSize=10"
```

### Get Node Details

```bash
curl -X GET http://localhost:8080/api/nodes/gpu-node-1
```

### List Workloads

```bash
curl -X GET "http://localhost:8080/api/workloads?namespace=default&status=Running"
```

### Get Workload Metrics

```bash
curl -X GET "http://localhost:8080/api/workloads/workload-uid-123/metrics?start=1609459200&end=1609545600&step=60"
```

## SDK and Client Libraries

Currently, there are no official SDK or client libraries. You can use standard HTTP clients in your preferred programming language:

- **Go**: `net/http` or `github.com/go-resty/resty`
- **Python**: `requests` or `httpx`
- **JavaScript**: `fetch` API or `axios`
- **Java**: `OkHttp` or `Apache HttpClient`

## Versioning

The current API version is `v1`. Future versions will be indicated through URL prefixing:

```
/api/v2/...
```

## Support

For questions, issues, or feature requests, please:
- Submit an issue on GitHub
- Contact the project maintainers
- Check the main project README for more information

## Additional Resources

- [Primus-Lens Core Module Documentation](../../modules/core/README.MD)
- [Primus-Lens API Module Documentation](../../modules/api/README.MD)
- [Project Repository](https://github.com/AMD-AGI/Primus-SaFE)

