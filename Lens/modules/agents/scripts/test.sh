#!/bin/bash
# Test Runner Script

set -e

echo "=========================================="
echo "Running Tests"
echo "=========================================="

# Activate virtual environment (if exists)
if [ -d "venv" ]; then
    source venv/bin/activate
fi

# Install test dependencies
pip install -q pytest pytest-asyncio pytest-cov

# Run tests
echo ""
echo "Running unit tests..."
pytest tests/ -v

echo ""
echo "Running tests with coverage report..."
pytest --cov=gpu_usage_agent --cov-report=html --cov-report=term tests/

echo ""
echo "=========================================="
echo "Tests completed"
echo "Coverage report: htmlcov/index.html"
echo "=========================================="

