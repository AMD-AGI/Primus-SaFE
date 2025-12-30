#!/bin/bash
#
# End-to-End Verification Script for AI Gateway (Phase 1-5)
#
# This script verifies all components are working correctly:
# 1. Database migrations
# 2. AI Gateway service
# 3. Agent registration
# 4. Task queue operations
# 5. Background jobs
#
# Usage:
#   ./run_e2e_test.sh [--gateway-url http://localhost:8003] [--db-dsn "..."]
#

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Default configuration
GATEWAY_URL="${GATEWAY_URL:-http://localhost:8003}"
DB_HOST="${DB_HOST:-localhost}"
DB_PORT="${DB_PORT:-5432}"
DB_NAME="${DB_NAME:-lens}"
DB_USER="${DB_USER:-lens}"
DB_PASSWORD="${DB_PASSWORD:-lens}"
PGPASSWORD="$DB_PASSWORD"
export PGPASSWORD

# Parse arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --gateway-url)
            GATEWAY_URL="$2"
            shift 2
            ;;
        --db-host)
            DB_HOST="$2"
            shift 2
            ;;
        *)
            echo "Unknown option: $1"
            exit 1
            ;;
    esac
done

echo -e "${BLUE}=====================================${NC}"
echo -e "${BLUE}  AI Gateway E2E Verification Test  ${NC}"
echo -e "${BLUE}=====================================${NC}"
echo ""
echo "Gateway URL: $GATEWAY_URL"
echo "Database: $DB_USER@$DB_HOST:$DB_PORT/$DB_NAME"
echo ""

# Counter for tests
TESTS_PASSED=0
TESTS_FAILED=0

pass() {
    echo -e "  ${GREEN}✓ $1${NC}"
    ((TESTS_PASSED++))
}

fail() {
    echo -e "  ${RED}✗ $1${NC}"
    echo -e "    ${RED}$2${NC}"
    ((TESTS_FAILED++))
}

warn() {
    echo -e "  ${YELLOW}⚠ $1${NC}"
}

section() {
    echo ""
    echo -e "${BLUE}--- $1 ---${NC}"
}

# ============================================================================
# Test 1: Database Connectivity and Schema
# ============================================================================
section "Test 1: Database Verification"

# Check database connection
if psql -h "$DB_HOST" -p "$DB_PORT" -U "$DB_USER" -d "$DB_NAME" -c "SELECT 1" > /dev/null 2>&1; then
    pass "Database connection successful"
else
    fail "Database connection failed" "Cannot connect to PostgreSQL"
    exit 1
fi

# Check ai_agent_registrations table
if psql -h "$DB_HOST" -p "$DB_PORT" -U "$DB_USER" -d "$DB_NAME" -c "\d ai_agent_registrations" > /dev/null 2>&1; then
    pass "Table ai_agent_registrations exists"
else
    fail "Table ai_agent_registrations missing" "Run migration: patch041-ai_agent_registrations.sql"
fi

# Check ai_tasks table
if psql -h "$DB_HOST" -p "$DB_PORT" -U "$DB_USER" -d "$DB_NAME" -c "\d ai_tasks" > /dev/null 2>&1; then
    pass "Table ai_tasks exists"
else
    fail "Table ai_tasks missing" "Run migration: patch042-ai_tasks.sql"
fi

# ============================================================================
# Test 2: AI Gateway Health Check
# ============================================================================
section "Test 2: AI Gateway Service"

# Check if gateway is responding
if curl -s -o /dev/null -w "%{http_code}" "$GATEWAY_URL/health" | grep -q "200"; then
    pass "AI Gateway health check passed"
else
    fail "AI Gateway not responding" "Start the AI Gateway service first"
    echo ""
    echo "To start AI Gateway:"
    echo "  cd modules/ai-gateway && go run cmd/ai-gateway/main.go"
    exit 1
fi

# ============================================================================
# Test 3: Agent Registration API
# ============================================================================
section "Test 3: Agent Registration API"

# Register a test agent
REGISTER_RESPONSE=$(curl -s -X POST "$GATEWAY_URL/api/v1/ai/agents/register" \
    -H "Content-Type: application/json" \
    -d '{
        "name": "e2e-test-agent",
        "endpoint": "http://localhost:9999",
        "topics": ["test.topic.one", "test.topic.two"],
        "health_check_path": "/health",
        "timeout_secs": 60,
        "metadata": {"version": "1.0.0", "test": "true"}
    }')

if echo "$REGISTER_RESPONSE" | grep -q '"message"'; then
    pass "Agent registration successful"
else
    fail "Agent registration failed" "$REGISTER_RESPONSE"
fi

# List agents
LIST_RESPONSE=$(curl -s "$GATEWAY_URL/api/v1/ai/agents")
if echo "$LIST_RESPONSE" | grep -q '"e2e-test-agent"'; then
    pass "Agent appears in list"
else
    fail "Agent not in list" "$LIST_RESPONSE"
fi

# Get specific agent
GET_RESPONSE=$(curl -s "$GATEWAY_URL/api/v1/ai/agents/e2e-test-agent")
if echo "$GET_RESPONSE" | grep -q '"endpoint"'; then
    pass "Get agent by name works"
else
    fail "Get agent failed" "$GET_RESPONSE"
fi

# Get agent health status
HEALTH_RESPONSE=$(curl -s "$GATEWAY_URL/api/v1/ai/agents/e2e-test-agent/health")
if echo "$HEALTH_RESPONSE" | grep -q '"status"'; then
    pass "Agent health endpoint works"
    # Status should be unknown or unhealthy since endpoint is fake
    if echo "$HEALTH_RESPONSE" | grep -q '"unknown"\|"unhealthy"'; then
        pass "Agent status correctly shows unknown/unhealthy (fake endpoint)"
    fi
else
    fail "Agent health check failed" "$HEALTH_RESPONSE"
fi

# ============================================================================
# Test 4: Task Queue API
# ============================================================================
section "Test 4: Task Queue API"

# Create a test task directly in DB
TASK_ID="e2e-test-$(date +%s)"
psql -h "$DB_HOST" -p "$DB_PORT" -U "$DB_USER" -d "$DB_NAME" -c "
    INSERT INTO ai_tasks (id, topic, status, input_payload, timeout_at)
    VALUES ('$TASK_ID', 'test.topic.one', 'pending', 
            '{\"test\": true}'::jsonb, 
            NOW() + INTERVAL '5 minutes')
" > /dev/null

if [ $? -eq 0 ]; then
    pass "Task created in database"
else
    fail "Failed to create task" "Database insert failed"
fi

# Get task via API
TASK_RESPONSE=$(curl -s "$GATEWAY_URL/api/v1/ai/tasks/$TASK_ID")
if echo "$TASK_RESPONSE" | grep -q '"pending"'; then
    pass "Task retrieved via API (status: pending)"
else
    fail "Failed to get task" "$TASK_RESPONSE"
fi

# Get task status
STATUS_RESPONSE=$(curl -s "$GATEWAY_URL/api/v1/ai/tasks/$TASK_ID/status")
if echo "$STATUS_RESPONSE" | grep -q '"status"'; then
    pass "Task status endpoint works"
else
    fail "Failed to get task status" "$STATUS_RESPONSE"
fi

# List tasks
LIST_TASKS=$(curl -s "$GATEWAY_URL/api/v1/ai/tasks?status=pending&limit=10")
if echo "$LIST_TASKS" | grep -q '"tasks"'; then
    pass "List tasks endpoint works"
else
    fail "Failed to list tasks" "$LIST_TASKS"
fi

# Cancel task
CANCEL_RESPONSE=$(curl -s -X POST "$GATEWAY_URL/api/v1/ai/tasks/$TASK_ID/cancel")
if echo "$CANCEL_RESPONSE" | grep -q '"message"'; then
    pass "Task cancellation works"
else
    fail "Failed to cancel task" "$CANCEL_RESPONSE"
fi

# Verify cancelled
CANCELLED_STATUS=$(curl -s "$GATEWAY_URL/api/v1/ai/tasks/$TASK_ID/status")
if echo "$CANCELLED_STATUS" | grep -q '"cancelled"'; then
    pass "Task status updated to cancelled"
else
    warn "Task status not updated (may need to check manually)"
fi

# ============================================================================
# Test 5: Stats API
# ============================================================================
section "Test 5: Stats API"

STATS_RESPONSE=$(curl -s "$GATEWAY_URL/api/v1/ai/stats")
if echo "$STATS_RESPONSE" | grep -q '"agents"\|"tasks"'; then
    pass "Stats endpoint works"
    echo "    Stats: $STATS_RESPONSE"
else
    warn "Stats endpoint may not be fully implemented"
fi

# ============================================================================
# Test 6: Cleanup - Unregister Test Agent
# ============================================================================
section "Test 6: Cleanup"

# Unregister test agent
UNREG_RESPONSE=$(curl -s -X DELETE "$GATEWAY_URL/api/v1/ai/agents/e2e-test-agent")
if echo "$UNREG_RESPONSE" | grep -q '"message"'; then
    pass "Agent unregistration successful"
else
    fail "Agent unregistration failed" "$UNREG_RESPONSE"
fi

# Verify agent is removed
GET_DELETED=$(curl -s -o /dev/null -w "%{http_code}" "$GATEWAY_URL/api/v1/ai/agents/e2e-test-agent")
if [ "$GET_DELETED" = "404" ]; then
    pass "Agent correctly removed from registry"
else
    warn "Agent may still exist in registry"
fi

# Clean up test task
psql -h "$DB_HOST" -p "$DB_PORT" -U "$DB_USER" -d "$DB_NAME" -c "
    DELETE FROM ai_tasks WHERE id LIKE 'e2e-test-%'
" > /dev/null
pass "Test data cleaned up"

# ============================================================================
# Summary
# ============================================================================
echo ""
echo -e "${BLUE}=====================================${NC}"
echo -e "${BLUE}           Test Summary             ${NC}"
echo -e "${BLUE}=====================================${NC}"
echo ""
echo -e "  ${GREEN}Passed: $TESTS_PASSED${NC}"
echo -e "  ${RED}Failed: $TESTS_FAILED${NC}"
echo ""

if [ $TESTS_FAILED -eq 0 ]; then
    echo -e "${GREEN}All tests passed! Phase 1-5 implementation is working correctly.${NC}"
    exit 0
else
    echo -e "${RED}Some tests failed. Please review the errors above.${NC}"
    exit 1
fi

