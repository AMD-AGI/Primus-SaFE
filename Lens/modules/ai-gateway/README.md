# AI Gateway

AI Gateway is an optional module that provides:
- Agent registration REST API
- Task status and management API
- Background jobs for health checking, timeout handling, and cleanup

## Overview

The AI Gateway acts as a bridge between the Lens platform and external AI Agents (like those in Primus-Conductor). It provides:

1. **Agent Registration**: Allows AI Agents to register themselves and declare which topics they handle
2. **Task Management**: Provides APIs to query and manage async AI tasks
3. **Health Monitoring**: Periodically checks agent health and updates their status
4. **Background Jobs**: Handles task timeouts and cleanup of old completed tasks

## Deployment

AI Gateway is **optional**. If not deployed:
- AI SDK gracefully degrades
- No AI-assisted features (aggregation, suggestions) will be available
- Basic rule-based features continue to work

### Deployment Modes

1. **Full AI Features**: Deploy ai-gateway + AI Agent
2. **No AI Features**: Don't deploy ai-gateway
3. **Static Agent**: Deploy ai-gateway with `registry.mode=config` for fixed agent endpoints

## Configuration

Configuration can be provided via:
1. YAML config file (set `AI_GATEWAY_CONFIG` environment variable)
2. Environment variables

### Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `AI_GATEWAY_PORT` | HTTP server port | `8003` |
| `AI_GATEWAY_HOST` | HTTP server host | `0.0.0.0` |
| `AI_GATEWAY_CONFIG` | Path to YAML config file | - |
| `AI_GATEWAY_REGISTRY_MODE` | Registry mode: memory, db, config | `db` |
| `DB_HOST` | Database host | - |
| `DB_USER` | Database username | - |
| `DB_PASSWORD` | Database password | - |
| `DB_NAME` | Database name | - |

### Example YAML Config

```yaml
server:
  port: 8003
  host: 0.0.0.0
  read_timeout: 30
  write_timeout: 60

registry:
  mode: db  # memory, db, or config
  health_check_interval: 30s
  unhealthy_threshold: 3
  # Static agents (only used when mode=config)
  agents:
    - name: alert-advisor
      endpoint: http://alert-advisor:8080
      topics:
        - alert.advisor.*
      timeout: 60s

background:
  health_check:
    enabled: true
    interval: 30s
  timeout:
    enabled: true
    interval: 1m
  cleanup:
    enabled: true
    interval: 1h
    retention_period: 168h  # 7 days
```

## API Endpoints

### Agent Management

| Method | Path | Description |
|--------|------|-------------|
| POST | `/api/v1/ai/agents/register` | Register an agent |
| DELETE | `/api/v1/ai/agents/:name` | Unregister an agent |
| GET | `/api/v1/ai/agents` | List all agents |
| GET | `/api/v1/ai/agents/:name` | Get agent details |
| GET | `/api/v1/ai/agents/:name/health` | Get agent health status |

### Task Management

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/ai/tasks/:id` | Get task details |
| GET | `/api/v1/ai/tasks/:id/status` | Get task status |
| POST | `/api/v1/ai/tasks/:id/cancel` | Cancel a task |
| GET | `/api/v1/ai/tasks` | List tasks (with filters) |

### Statistics

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/ai/stats` | Get agent and task statistics |

### Health

| Method | Path | Description |
|--------|------|-------------|
| GET | `/health` | Health check |
| GET | `/ready` | Readiness check |

## Building

```bash
cd Lens/modules/ai-gateway
go build -o ai-gateway ./cmd/ai-gateway
```

## Running

```bash
# With environment variables
export DB_HOST=localhost
export DB_USER=lens
export DB_PASSWORD=secret
export DB_NAME=lens
./ai-gateway

# With config file
export AI_GATEWAY_CONFIG=/etc/ai-gateway/config.yaml
./ai-gateway
```

