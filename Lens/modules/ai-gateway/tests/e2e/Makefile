# E2E Test Makefile for AI Gateway (Phase 1-5)
#
# Usage:
#   make test-all      - Run all tests
#   make test-basic    - Run basic API tests (no mock agent)
#   make test-async    - Run async flow tests (requires mock agent)
#   make start-mock    - Start mock agent in foreground
#   make setup-db      - Setup test database
#

# Configuration
GATEWAY_URL ?= http://localhost:8003
DB_HOST ?= localhost
DB_PORT ?= 5432
DB_NAME ?= lens
DB_USER ?= lens
DB_PASSWORD ?= lens

export GATEWAY_URL DB_HOST DB_PORT DB_NAME DB_USER DB_PASSWORD

.PHONY: all test-all test-basic test-async test-unit start-mock setup-db help

help:
	@echo "AI Gateway E2E Test Commands:"
	@echo ""
	@echo "  make test-all      Run all tests (basic + async flow)"
	@echo "  make test-basic    Run basic API tests (no mock agent needed)"
	@echo "  make test-async    Run async flow test (starts mock agent)"
	@echo "  make test-unit     Run SDK unit tests (with DB)"
	@echo "  make start-mock    Start mock agent in foreground"
	@echo "  make setup-db      Create test database and apply migrations"
	@echo "  make clean         Clean up test data"
	@echo ""
	@echo "Environment Variables:"
	@echo "  GATEWAY_URL    AI Gateway URL (default: $(GATEWAY_URL))"
	@echo "  DB_HOST        PostgreSQL host (default: $(DB_HOST))"
	@echo "  DB_PORT        PostgreSQL port (default: $(DB_PORT))"
	@echo "  DB_NAME        Database name (default: $(DB_NAME))"
	@echo "  DB_USER        Database user (default: $(DB_USER))"
	@echo "  DB_PASSWORD    Database password (default: $(DB_PASSWORD))"

all: test-all

# Run basic API tests (registration, task APIs)
test-basic:
	@echo "Running basic API tests..."
	@chmod +x run_e2e_test.sh
	./run_e2e_test.sh

# Run async flow test with mock agent
test-async:
	@echo "Running async flow tests..."
	@chmod +x test_async_flow.sh
	./test_async_flow.sh

# Run SDK unit tests
test-unit:
	@echo "Running SDK unit tests..."
	go test -v ./... -short

# Run SDK integration tests (requires DB)
test-integration:
	@echo "Running SDK integration tests..."
	TEST_DB_DSN="host=$(DB_HOST) port=$(DB_PORT) dbname=$(DB_NAME) user=$(DB_USER) password=$(DB_PASSWORD) sslmode=disable" \
	go test -v ./... -run Integration

# Run all tests
test-all: test-basic test-async
	@echo ""
	@echo "All tests completed!"

# Start mock agent in foreground
start-mock:
	@echo "Starting mock agent..."
	python3 mock_agent.py \
		--name mock-agent \
		--port 8002 \
		--gateway "$(GATEWAY_URL)" \
		--db-dsn "host=$(DB_HOST) port=$(DB_PORT) dbname=$(DB_NAME) user=$(DB_USER) password=$(DB_PASSWORD)" \
		--topics "alert.advisor.aggregate-workloads,alert.advisor.generate-suggestions,scan.identify-component"

# Setup test database
setup-db:
	@echo "Setting up test database..."
	PGPASSWORD=$(DB_PASSWORD) psql -h $(DB_HOST) -p $(DB_PORT) -U $(DB_USER) -d postgres -c \
		"SELECT 'exists' FROM pg_database WHERE datname = '$(DB_NAME)'" | grep -q exists || \
	PGPASSWORD=$(DB_PASSWORD) psql -h $(DB_HOST) -p $(DB_PORT) -U $(DB_USER) -d postgres -c \
		"CREATE DATABASE $(DB_NAME)"
	@echo "Applying migrations..."
	PGPASSWORD=$(DB_PASSWORD) psql -h $(DB_HOST) -p $(DB_PORT) -U $(DB_USER) -d $(DB_NAME) \
		-f ../../../core/pkg/database/migrations/patch041-ai_agent_registrations.sql
	PGPASSWORD=$(DB_PASSWORD) psql -h $(DB_HOST) -p $(DB_PORT) -U $(DB_USER) -d $(DB_NAME) \
		-f ../../../core/pkg/database/migrations/patch042-ai_tasks.sql
	@echo "Database setup complete!"

# Clean up test data
clean:
	@echo "Cleaning up test data..."
	PGPASSWORD=$(DB_PASSWORD) psql -h $(DB_HOST) -p $(DB_PORT) -U $(DB_USER) -d $(DB_NAME) -c \
		"DELETE FROM ai_tasks WHERE id LIKE 'e2e-%' OR id LIKE 'async-test-%';"
	PGPASSWORD=$(DB_PASSWORD) psql -h $(DB_HOST) -p $(DB_PORT) -U $(DB_USER) -d $(DB_NAME) -c \
		"DELETE FROM ai_agent_registrations WHERE name LIKE 'e2e-%' OR name = 'mock-agent';"
	@echo "Cleanup complete!"

# Verify prerequisites
check-prereqs:
	@echo "Checking prerequisites..."
	@which psql > /dev/null || (echo "Error: psql not found" && exit 1)
	@which curl > /dev/null || (echo "Error: curl not found" && exit 1)
	@which python3 > /dev/null || (echo "Error: python3 not found" && exit 1)
	@python3 -c "import psycopg2, requests" 2>/dev/null || \
		(echo "Warning: Python packages missing. Install with: pip3 install psycopg2-binary requests")
	@echo "Prerequisites check passed!"

# Docker-based test environment
docker-start:
	@echo "Starting PostgreSQL in Docker..."
	docker run -d --name lens-test-postgres \
		-e POSTGRES_USER=$(DB_USER) \
		-e POSTGRES_PASSWORD=$(DB_PASSWORD) \
		-e POSTGRES_DB=$(DB_NAME) \
		-p $(DB_PORT):5432 \
		postgres:15
	@echo "Waiting for PostgreSQL to start..."
	@sleep 5
	@make setup-db

docker-stop:
	@echo "Stopping PostgreSQL container..."
	docker stop lens-test-postgres || true
	docker rm lens-test-postgres || true

