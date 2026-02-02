# Tools Repository

Centralized tool management service for the Primus AI Platform.

## Overview

Tools Repository provides:
- **Tool Registration**: Register tools with metadata and schema
- **Multi-Provider Support**: MCP, HTTP, and A2A (Agent-to-Agent) providers
- **Semantic Search**: Find tools by natural language description
- **Access Control**: Platform/Team/User scoping
- **Analytics**: Usage tracking and statistics

## Architecture

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                           Tools Repository                                   │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐             │
│  │ API Handler     │  │ Registry        │  │ Provider Factory│             │
│  │                 │  │                 │  │                 │             │
│  │ - List          │  │ - CRUD          │  │ - HTTP          │             │
│  │ - Get           │  │ - Search        │  │ - MCP           │             │
│  │ - Register      │  │ - Stats         │  │ - A2A           │             │
│  │ - Execute       │  │ - Access        │  │                 │             │
│  └────────┬────────┘  └────────┬────────┘  └────────┬────────┘             │
│           │                    │                    │                       │
│           └────────────────────┼────────────────────┘                       │
│                                │                                            │
│                                ▼                                            │
│                    ┌─────────────────────┐                                  │
│                    │ PostgreSQL          │                                  │
│                    │ + pgvector          │                                  │
│                    └─────────────────────┘                                  │
└─────────────────────────────────────────────────────────────────────────────┘
```

## API Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/api/v1/tools` | GET | List all tools |
| `/api/v1/tools/:name` | GET | Get tool by name |
| `/api/v1/tools` | POST | Register new tool |
| `/api/v1/tools/:name` | DELETE | Delete tool |
| `/api/v1/tools/search` | POST | Search tools |
| `/api/v1/tools/execute` | POST | Execute tool |
| `/api/v1/tools/:name/stats` | GET | Get tool statistics |
| `/api/v1/tools/:name/definition` | GET | Get MCP tool definition |

## Provider Types

### HTTP Provider

For tools accessible via HTTP APIs:

```json
{
  "name": "weather_api",
  "provider_type": "http",
  "provider_config": {
    "url": "https://api.weather.com/v1/current",
    "method": "GET",
    "headers": {
      "Accept": "application/json"
    },
    "auth_type": "api_key",
    "auth_config": {
      "header": "X-API-Key",
      "key": "${WEATHER_API_KEY}"
    }
  }
}
```

### MCP Provider

For tools provided by MCP servers:

```json
{
  "name": "git_status",
  "provider_type": "mcp",
  "provider_config": {
    "server_url": "http://mcp-server:8080",
    "transport": "sse"
  }
}
```

### A2A Provider

For tools provided by AI agents:

```json
{
  "name": "code_review",
  "provider_type": "a2a",
  "provider_config": {
    "agent_url": "https://agent.example.com",
    "capabilities": ["code_review", "security_scan"]
  }
}
```

## Database Schema

See `migrations/001_create_tools_tables.sql` for the complete schema.

## Configuration

Environment variables:

| Variable | Description | Default |
|----------|-------------|---------|
| `DATABASE_URL` | PostgreSQL connection string | `postgres://localhost:5432/tools_repository` |
| `PORT` | Server port | `8081` |

## Development

```bash
# Build
go build -o tools-repository ./cmd/tools-repository

# Run
DATABASE_URL="postgres://..." ./tools-repository

# Run migrations
psql $DATABASE_URL -f migrations/001_create_tools_tables.sql
```

## Usage Examples

### Register a Tool

```bash
curl -X POST http://localhost:8081/api/v1/tools \
  -H "Content-Type: application/json" \
  -d '{
    "name": "k8s_pod_status",
    "description": "Check Kubernetes pod status",
    "provider_type": "http",
    "provider_config": {
      "url": "http://k8s-api/pods",
      "method": "GET"
    },
    "input_schema": {
      "type": "object",
      "properties": {
        "namespace": {"type": "string"},
        "pod_name": {"type": "string"}
      },
      "required": ["namespace"]
    },
    "category": "kubernetes",
    "tags": ["k8s", "pods", "status"]
  }'
```

### Search for Tools

```bash
curl -X POST http://localhost:8081/api/v1/tools/search \
  -H "Content-Type: application/json" \
  -d '{"query": "kubernetes pod status", "limit": 5}'
```

### Execute a Tool

```bash
curl -X POST http://localhost:8081/api/v1/tools/execute \
  -H "Content-Type: application/json" \
  -d '{
    "tool_name": "k8s_pod_status",
    "arguments": {"namespace": "default"}
  }'
```

## Related

- [Skills Repository](../skills-repository/README.md)
- [AI Platform Implementation Plan](../../docs/ai-platform-implementation-plan.md)
