#!/bin/bash
# TraceLens Container Entrypoint
# Starts Streamlit with proper configuration

set -e

# Default values
PORT=${STREAMLIT_SERVER_PORT:-8501}
BASE_URL_PATH=${BASE_URL_PATH:-""}

# Build Streamlit command
CMD="streamlit run /app/analyze_trace.py"
CMD="$CMD --server.port=$PORT"
CMD="$CMD --server.headless=true"
CMD="$CMD --server.enableCORS=false"
CMD="$CMD --server.enableXsrfProtection=false"

# Add base URL path if specified (for reverse proxy)
if [ -n "$BASE_URL_PATH" ]; then
    CMD="$CMD --server.baseUrlPath=$BASE_URL_PATH"
fi

echo "Starting TraceLens..."
echo "  Session ID: ${SESSION_ID:-not set}"
echo "  Trace File: ${TRACE_FILE_PATH:-not set}"
echo "  Base URL Path: ${BASE_URL_PATH:-/}"
echo "  Port: $PORT"

exec $CMD

