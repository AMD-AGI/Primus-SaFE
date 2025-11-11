#!/bin/bash
# GPU Usage Analysis Agent Startup Script

set -e

echo "=========================================="
echo "Starting GPU Usage Analysis Agent"
echo "=========================================="

# Check environment variables
if [ -z "$LLM_API_KEY" ]; then
    echo "Warning: LLM_API_KEY is not set"
    echo "Please set the environment variable or configure it in the .env file"
fi


echo "Configuration:"
echo "  - Lens API URL: $LENS_API_URL"
echo "  - LLM Provider: $LLM_PROVIDER"
echo "  - LLM Model: $LLM_MODEL"
echo ""

# Check Python version
python_version=$(python --version 2>&1 | awk '{print $2}')
echo "Python version: $python_version"

# Install dependencies (if needed)
if [ ! -d "venv" ]; then
    echo "Creating virtual environment..."
    python -m venv venv
fi

echo "Activating virtual environment..."
source venv/bin/activate

echo "Installing dependencies..."
pip install -q -r requirements.txt

echo ""
echo "=========================================="
echo "Starting service..."
echo "=========================================="
echo ""

# Start the service (run from root directory, do not cd to api)
python -m api.main

