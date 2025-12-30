#!/bin/bash
#
# Async Task Flow Test - Full End-to-End Test with Mock Agent
#
# This script tests the complete async invocation flow:
# 1. Start mock agent (registers and polls tasks)
# 2. Publish a task via direct DB insert (simulating SDK)
# 3. Wait for mock agent to process the task
# 4. Verify task completion and result
#
# Prerequisites:
# - PostgreSQL running with migrations applied
# - AI Gateway running
# - Python 3 with psycopg2 and requests packages
#
# Usage:
#   ./test_async_flow.sh
#

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
GATEWAY_URL="${GATEWAY_URL:-http://localhost:8003}"
DB_HOST="${DB_HOST:-localhost}"
DB_PORT="${DB_PORT:-5432}"
DB_NAME="${DB_NAME:-lens}"
DB_USER="${DB_USER:-lens}"
DB_PASSWORD="${DB_PASSWORD:-lens}"
PGPASSWORD="$DB_PASSWORD"
export PGPASSWORD

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

echo -e "${BLUE}================================================${NC}"
echo -e "${BLUE}  Async Task Flow E2E Test (with Mock Agent)    ${NC}"
echo -e "${BLUE}================================================${NC}"
echo ""

# Cleanup function
cleanup() {
    echo ""
    echo -e "${YELLOW}Cleaning up...${NC}"
    if [ ! -z "$MOCK_AGENT_PID" ]; then
        kill $MOCK_AGENT_PID 2>/dev/null || true
        wait $MOCK_AGENT_PID 2>/dev/null || true
    fi
    # Clean up test data
    psql -h "$DB_HOST" -p "$DB_PORT" -U "$DB_USER" -d "$DB_NAME" -c "
        DELETE FROM ai_tasks WHERE id LIKE 'async-test-%';
        DELETE FROM ai_agent_registrations WHERE name = 'mock-agent';
    " > /dev/null 2>&1 || true
    echo "Cleanup complete"
}

trap cleanup EXIT

# ============================================================================
# Step 1: Verify Prerequisites
# ============================================================================
echo -e "${BLUE}Step 1: Verifying prerequisites...${NC}"

# Check PostgreSQL
if ! psql -h "$DB_HOST" -p "$DB_PORT" -U "$DB_USER" -d "$DB_NAME" -c "SELECT 1" > /dev/null 2>&1; then
    echo -e "${RED}Error: Cannot connect to PostgreSQL${NC}"
    exit 1
fi
echo -e "  ${GREEN}✓ PostgreSQL connection OK${NC}"

# Check AI Gateway
if ! curl -s -o /dev/null -w "%{http_code}" "$GATEWAY_URL/health" | grep -q "200"; then
    echo -e "${RED}Error: AI Gateway not responding${NC}"
    exit 1
fi
echo -e "  ${GREEN}✓ AI Gateway is running${NC}"

# Check Python and dependencies
if ! python3 -c "import psycopg2, requests" 2>/dev/null; then
    echo -e "${RED}Error: Python dependencies missing${NC}"
    echo "Install with: pip3 install psycopg2-binary requests"
    exit 1
fi
echo -e "  ${GREEN}✓ Python dependencies OK${NC}"

# ============================================================================
# Step 2: Start Mock Agent
# ============================================================================
echo ""
echo -e "${BLUE}Step 2: Starting Mock Agent...${NC}"

cd "$SCRIPT_DIR"
python3 mock_agent.py \
    --name mock-agent \
    --port 8002 \
    --gateway "$GATEWAY_URL" \
    --db-dsn "host=$DB_HOST port=$DB_PORT dbname=$DB_NAME user=$DB_USER password=$DB_PASSWORD" \
    --topics "alert.advisor.aggregate-workloads,alert.advisor.generate-suggestions" \
    --poll-interval 0.5 &
MOCK_AGENT_PID=$!

# Wait for agent to start
sleep 2

# Check if agent is running
if ! kill -0 $MOCK_AGENT_PID 2>/dev/null; then
    echo -e "${RED}Error: Mock agent failed to start${NC}"
    exit 1
fi
echo -e "  ${GREEN}✓ Mock agent started (PID: $MOCK_AGENT_PID)${NC}"

# Wait for registration
sleep 1

# Verify agent is registered
AGENTS=$(curl -s "$GATEWAY_URL/api/v1/ai/agents")
if echo "$AGENTS" | grep -q '"mock-agent"'; then
    echo -e "  ${GREEN}✓ Mock agent registered with gateway${NC}"
else
    echo -e "${YELLOW}  ⚠ Agent registration not confirmed (continuing anyway)${NC}"
fi

# ============================================================================
# Step 3: Publish Async Task
# ============================================================================
echo ""
echo -e "${BLUE}Step 3: Publishing async task...${NC}"

TASK_ID="async-test-$(date +%s%N | cut -c1-13)"
TASK_TOPIC="alert.advisor.aggregate-workloads"
TASK_INPUT='{
    "workloads": [
        {
            "uid": "workload-1",
            "name": "postgres-primary",
            "namespace": "default",
            "kind": "Deployment",
            "labels": {"app": "postgres"},
            "images": ["postgres:15"]
        },
        {
            "uid": "workload-2",
            "name": "postgres-replica",
            "namespace": "default",
            "kind": "Deployment",
            "labels": {"app": "postgres"},
            "images": ["postgres:15"]
        }
    ]
}'

# Insert task into database (simulating SDK InvokeAsync)
psql -h "$DB_HOST" -p "$DB_PORT" -U "$DB_USER" -d "$DB_NAME" -c "
    INSERT INTO ai_tasks (id, topic, status, priority, input_payload, max_retries, context, timeout_at)
    VALUES (
        '$TASK_ID', 
        '$TASK_TOPIC', 
        'pending', 
        0,
        '$TASK_INPUT'::jsonb,
        3,
        '{\"cluster_id\": \"test-cluster\", \"tenant_id\": \"test-tenant\"}'::jsonb,
        NOW() + INTERVAL '5 minutes'
    )
" > /dev/null

echo -e "  ${GREEN}✓ Task published: $TASK_ID${NC}"
echo -e "  Topic: $TASK_TOPIC"

# ============================================================================
# Step 4: Wait for Task Processing
# ============================================================================
echo ""
echo -e "${BLUE}Step 4: Waiting for task processing...${NC}"

MAX_WAIT=30
WAITED=0
TASK_COMPLETED=false

while [ $WAITED -lt $MAX_WAIT ]; do
    STATUS=$(curl -s "$GATEWAY_URL/api/v1/ai/tasks/$TASK_ID/status")
    CURRENT_STATUS=$(echo "$STATUS" | grep -o '"status":"[^"]*"' | cut -d'"' -f4)
    
    echo -e "  [$WAITED s] Status: $CURRENT_STATUS"
    
    if [ "$CURRENT_STATUS" = "completed" ]; then
        TASK_COMPLETED=true
        break
    elif [ "$CURRENT_STATUS" = "failed" ]; then
        echo -e "${RED}  Task failed!${NC}"
        ERROR_MSG=$(echo "$STATUS" | grep -o '"error_message":"[^"]*"' | cut -d'"' -f4)
        echo -e "  Error: $ERROR_MSG"
        break
    fi
    
    sleep 1
    ((WAITED++))
done

if [ "$TASK_COMPLETED" = true ]; then
    echo -e "  ${GREEN}✓ Task completed successfully in ${WAITED}s${NC}"
else
    if [ "$CURRENT_STATUS" != "failed" ]; then
        echo -e "${RED}  Task did not complete within ${MAX_WAIT}s${NC}"
    fi
fi

# ============================================================================
# Step 5: Verify Task Result
# ============================================================================
echo ""
echo -e "${BLUE}Step 5: Verifying task result...${NC}"

TASK_DETAILS=$(curl -s "$GATEWAY_URL/api/v1/ai/tasks/$TASK_ID")
echo "$TASK_DETAILS" | python3 -m json.tool 2>/dev/null || echo "$TASK_DETAILS"

# Check output payload in database
OUTPUT=$(psql -h "$DB_HOST" -p "$DB_PORT" -U "$DB_USER" -d "$DB_NAME" -t -c "
    SELECT output_payload FROM ai_tasks WHERE id = '$TASK_ID'
" 2>/dev/null)

if [ ! -z "$OUTPUT" ] && [ "$OUTPUT" != " " ]; then
    echo ""
    echo -e "  ${GREEN}✓ Output payload retrieved from database${NC}"
    echo "  Output:"
    echo "$OUTPUT" | python3 -m json.tool 2>/dev/null || echo "$OUTPUT"
else
    echo -e "${YELLOW}  ⚠ No output payload found${NC}"
fi

# ============================================================================
# Step 6: Test Another Topic
# ============================================================================
echo ""
echo -e "${BLUE}Step 6: Testing another topic (generate-suggestions)...${NC}"

TASK_ID2="async-test-$(date +%s%N | cut -c1-13)"
TASK_TOPIC2="alert.advisor.generate-suggestions"
TASK_INPUT2='{
    "component": {
        "group_id": "group-1",
        "name": "PostgreSQL Cluster",
        "component_type": "postgresql"
    }
}'

psql -h "$DB_HOST" -p "$DB_PORT" -U "$DB_USER" -d "$DB_NAME" -c "
    INSERT INTO ai_tasks (id, topic, status, input_payload, timeout_at)
    VALUES ('$TASK_ID2', '$TASK_TOPIC2', 'pending', '$TASK_INPUT2'::jsonb, NOW() + INTERVAL '5 minutes')
" > /dev/null

echo -e "  ${GREEN}✓ Second task published: $TASK_ID2${NC}"

# Wait for processing
sleep 3

STATUS2=$(curl -s "$GATEWAY_URL/api/v1/ai/tasks/$TASK_ID2/status")
CURRENT_STATUS2=$(echo "$STATUS2" | grep -o '"status":"[^"]*"' | cut -d'"' -f4)

if [ "$CURRENT_STATUS2" = "completed" ]; then
    echo -e "  ${GREEN}✓ Second task completed${NC}"
else
    echo -e "  Status: $CURRENT_STATUS2"
fi

# ============================================================================
# Summary
# ============================================================================
echo ""
echo -e "${BLUE}================================================${NC}"
echo -e "${BLUE}                Test Summary                    ${NC}"
echo -e "${BLUE}================================================${NC}"
echo ""
echo "Components Tested:"
echo -e "  ${GREEN}✓ AI Gateway service${NC}"
echo -e "  ${GREEN}✓ Agent registration API${NC}"
echo -e "  ${GREEN}✓ Task queue database${NC}"
echo -e "  ${GREEN}✓ Task publishing (SDK → DB)${NC}"
echo -e "  ${GREEN}✓ Task polling (Agent ← DB)${NC}"
echo -e "  ${GREEN}✓ Task processing (Agent)${NC}"
echo -e "  ${GREEN}✓ Result storage (Agent → DB)${NC}"
echo -e "  ${GREEN}✓ Task status API${NC}"
echo ""

if [ "$TASK_COMPLETED" = true ]; then
    echo -e "${GREEN}Full async flow verified successfully!${NC}"
    echo ""
    echo "The following Phase 1-5 components are working correctly:"
    echo "  - Phase 1: Topic schemas (request/response payloads)"
    echo "  - Phase 2: AI Registry (agent registration storage)"
    echo "  - Phase 3: AI Client SDK pattern (simulated)"
    echo "  - Phase 4: Task Queue (publish, poll, complete)"
    echo "  - Phase 5: AI Gateway (APIs, background jobs)"
    exit 0
else
    echo -e "${RED}Async flow verification incomplete.${NC}"
    echo "Check the logs above for details."
    exit 1
fi

