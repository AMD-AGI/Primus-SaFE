# AI Gateway End-to-End Verification (Phase 1-5)

This directory contains end-to-end tests for verifying the AI Gateway implementation through Phase 5.

## Overview

Based on the design document, Phase 1-5 includes:

| Phase | Component | Description |
|-------|-----------|-------------|
| Phase 1 | `core/aitopics` | Topic constants and payload schemas |
| Phase 2 | `core/airegistry` | Agent registry SDK |
| Phase 3 | `core/aiclient` | AI Client SDK |
| Phase 4 | `core/aitaskqueue` | Task queue SDK |
| Phase 5 | `ai-gateway` | REST APIs and background jobs |

## Prerequisites

1. **PostgreSQL** database with migrations applied
2. **AI Gateway** service running
3. **Python 3** with `psycopg2-binary` and `requests` packages
4. **curl** and **psql** CLI tools

## Quick Start with Docker

```bash
# Option 1: Use Makefile
cd modules/ai-gateway/tests/e2e

# Start PostgreSQL container and setup DB
make docker-start

# In another terminal, start AI Gateway
cd modules/ai-gateway
go run cmd/ai-gateway/main.go

# Run all tests
make test-all
```

## Manual Step-by-Step Verification

### Step 1: Database Setup

```bash
# Start PostgreSQL (if not running)
docker run -d --name lens-postgres \
  -e POSTGRES_USER=lens \
  -e POSTGRES_PASSWORD=lens \
  -e POSTGRES_DB=lens \
  -p 5432:5432 \
  postgres:15

# Wait for PostgreSQL to start
sleep 5

# Apply migrations
export PGPASSWORD=lens
psql -h localhost -U lens -d lens -f \
  modules/core/pkg/database/migrations/patch041-ai_agent_registrations.sql
psql -h localhost -U lens -d lens -f \
  modules/core/pkg/database/migrations/patch042-ai_tasks.sql
```

### Step 2: Start AI Gateway

```bash
cd modules/ai-gateway
go run cmd/ai-gateway/main.go

# Verify it's running
curl http://localhost:8003/health
# Expected: {"status":"ok"}
```

### Step 3: Run Basic API Tests

```bash
cd modules/ai-gateway/tests/e2e
./run_e2e_test.sh
```

This tests:
- ✅ Database connection and tables
- ✅ AI Gateway health endpoint
- ✅ Agent registration API
- ✅ Agent listing/retrieval APIs
- ✅ Task status APIs
- ✅ Agent unregistration

### Step 4: Run Full Async Flow Test

```bash
cd modules/ai-gateway/tests/e2e
./test_async_flow.sh
```

This tests the complete async flow:
- ✅ Mock Agent registration
- ✅ Mock Agent health checks
- ✅ Task publishing (simulating SDK)
- ✅ Task polling by Mock Agent
- ✅ Task processing and completion
- ✅ Result retrieval via API

### Step 5: Run SDK Unit Tests

```bash
cd modules/ai-gateway/tests/e2e
go test -v ./... -short
```

This verifies:
- ✅ Topic envelope serialization
- ✅ Payload schema structures
- ✅ Registry in-memory operations
- ✅ Topic pattern matching

### Step 6: Run SDK Integration Tests (with DB)

```bash
cd modules/ai-gateway/tests/e2e
export TEST_DB_DSN="host=localhost port=5432 dbname=lens user=lens password=lens sslmode=disable"
go test -v ./... -run Integration
```

This verifies:
- ✅ Task publishing to PostgreSQL
- ✅ Task claiming and processing
- ✅ Task completion and result retrieval
- ✅ Task listing and counting

## Test Components

### 1. `run_e2e_test.sh`
Basic API verification script that tests all REST endpoints without requiring a mock agent.

### 2. `test_async_flow.sh`
Full end-to-end async flow test that starts a mock agent and verifies the complete task lifecycle.

### 3. `mock_agent.py`
Python-based mock AI agent that:
- Registers with AI Gateway
- Provides `/health` endpoint
- Polls for tasks from PostgreSQL
- Processes tasks and returns mock results

### 4. `sdk_integration_test.go`
Go test file that verifies SDK components:
- Topic schemas and envelopes
- Registry operations
- Task queue operations
- Topic routing

## Verification Checklist

After running all tests, verify these components are working:

### Phase 1: Topic Schema (`core/aitopics`)
- [ ] Topic constants defined (`TopicAlertAdvisorAggregateWorkloads`, etc.)
- [ ] Request/Response envelopes serialize correctly
- [ ] Payload schemas (`AggregateWorkloadsInput/Output`, etc.) work

### Phase 2: AI Registry (`core/airegistry`)
- [ ] Memory store: Register, Get, List, Unregister
- [ ] DB store: Persist to `ai_agent_registrations` table
- [ ] Health status updates work
- [ ] Topic pattern matching works (exact and wildcard)

### Phase 3: AI Client (`core/aiclient`)
- [ ] Client interface compiles
- [ ] Retry and circuit breaker logic present

### Phase 4: Task Queue (`core/aitaskqueue`)
- [ ] Tasks stored in `ai_tasks` table
- [ ] Publish creates pending task
- [ ] Claim updates status to processing
- [ ] Complete stores result
- [ ] List and count work with filters

### Phase 5: AI Gateway (`ai-gateway`)
- [ ] Service starts and responds to health check
- [ ] `POST /api/v1/ai/agents/register` works
- [ ] `GET /api/v1/ai/agents` lists agents
- [ ] `GET /api/v1/ai/agents/:name` gets specific agent
- [ ] `DELETE /api/v1/ai/agents/:name` unregisters agent
- [ ] `GET /api/v1/ai/tasks/:id` gets task
- [ ] `GET /api/v1/ai/tasks/:id/status` gets task status
- [ ] `POST /api/v1/ai/tasks/:id/cancel` cancels task
- [ ] Background health checker runs
- [ ] Background timeout handler runs
- [ ] Background cleanup job runs

## Troubleshooting

### AI Gateway won't start
- Check database connection settings in config
- Verify PostgreSQL is running: `psql -h localhost -U lens -d lens -c "SELECT 1"`

### Mock agent fails to connect
- Check database DSN format
- Install Python packages: `pip3 install -r requirements.txt`

### Tests timeout
- Increase poll interval in mock agent
- Check for network connectivity issues

### Task stuck in processing
- Background timeout handler should reset it after ~5 minutes
- Check agent_id in ai_tasks table to see which agent claimed it

## Files

```
tests/e2e/
├── README.md                 # This file
├── Makefile                  # Make targets for testing
├── requirements.txt          # Python dependencies
├── run_e2e_test.sh          # Basic API tests
├── test_async_flow.sh       # Full async flow test
├── mock_agent.py            # Mock AI Agent
└── sdk_integration_test.go  # Go SDK tests
```

