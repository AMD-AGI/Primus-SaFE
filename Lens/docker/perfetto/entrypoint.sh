#!/bin/sh
set -e

echo "=========================================="
echo "Perfetto Viewer Container Starting..."
echo "Mode: trace_processor HTTP Server"
echo "=========================================="
echo "SESSION_ID: ${SESSION_ID}"
echo "PROFILER_FILE_ID: ${PROFILER_FILE_ID}"
echo "CLUSTER: ${CLUSTER}"
echo "API_BASE_URL: ${API_BASE_URL}"
echo "=========================================="

# Validate required environment variables
if [ -z "$PROFILER_FILE_ID" ]; then
    echo "ERROR: PROFILER_FILE_ID is required"
    exit 1
fi

if [ -z "$API_BASE_URL" ]; then
    echo "ERROR: API_BASE_URL is required"
    exit 1
fi

# Build cluster query parameter
CLUSTER_PARAM=""
if [ -n "$CLUSTER" ]; then
    CLUSTER_PARAM="?cluster=${CLUSTER}"
fi

# Download trace file from API
TRACE_FILE="/data/trace.perfetto"
DOWNLOAD_URL="${API_BASE_URL}/v1/profiler/files/${PROFILER_FILE_ID}/content${CLUSTER_PARAM}"

echo "Downloading trace file from: ${DOWNLOAD_URL}"

# Retry download up to 3 times
MAX_RETRIES=3
RETRY_COUNT=0
DOWNLOAD_SUCCESS=false

while [ $RETRY_COUNT -lt $MAX_RETRIES ]; do
    RETRY_COUNT=$((RETRY_COUNT + 1))
    echo "Download attempt ${RETRY_COUNT}/${MAX_RETRIES}..."
    
    if [ -n "$INTERNAL_TOKEN" ]; then
        HTTP_CODE=$(curl -s -w "%{http_code}" -o "$TRACE_FILE" \
            -H "X-Internal-Token: ${INTERNAL_TOKEN}" \
            --connect-timeout 30 \
            --max-time 600 \
            "${DOWNLOAD_URL}")
    else
        HTTP_CODE=$(curl -s -w "%{http_code}" -o "$TRACE_FILE" \
            --connect-timeout 30 \
            --max-time 600 \
            "${DOWNLOAD_URL}")
    fi
    
    if [ "$HTTP_CODE" = "200" ] && [ -f "$TRACE_FILE" ] && [ -s "$TRACE_FILE" ]; then
        DOWNLOAD_SUCCESS=true
        break
    else
        echo "Download failed with HTTP code: ${HTTP_CODE}"
        if [ $RETRY_COUNT -lt $MAX_RETRIES ]; then
            echo "Retrying in 5 seconds..."
            sleep 5
        fi
    fi
done

if [ "$DOWNLOAD_SUCCESS" != "true" ]; then
    echo "ERROR: Failed to download trace file after ${MAX_RETRIES} attempts"
    exit 1
fi

# Get file size
FILE_SIZE=$(stat -c%s "$TRACE_FILE" 2>/dev/null || stat -f%z "$TRACE_FILE" 2>/dev/null || echo "unknown")
echo "Trace file downloaded successfully. Size: ${FILE_SIZE} bytes"

echo "=========================================="
echo "Starting trace_processor HTTP server..."
echo "=========================================="

# Start trace_processor_shell in HTTP mode
# Port 9001 is the default Perfetto RPC port
# The server will load the trace file and serve queries via HTTP
trace_processor_shell \
    --httpd \
    --http-port=9001 \
    "$TRACE_FILE" &

TRACE_PROCESSOR_PID=$!
echo "trace_processor_shell started with PID: ${TRACE_PROCESSOR_PID}"

# Wait for trace_processor to be ready
echo "Waiting for trace_processor to load trace file..."
MAX_WAIT=120  # Maximum wait time in seconds (trace loading can take a while)
WAIT_COUNT=0
while [ $WAIT_COUNT -lt $MAX_WAIT ]; do
    if curl -s http://127.0.0.1:9001/status > /dev/null 2>&1; then
        echo "trace_processor HTTP server is ready!"
        break
    fi
    sleep 1
    WAIT_COUNT=$((WAIT_COUNT + 1))
    if [ $((WAIT_COUNT % 10)) -eq 0 ]; then
        echo "Still waiting... (${WAIT_COUNT}s)"
    fi
done

if [ $WAIT_COUNT -ge $MAX_WAIT ]; then
    echo "WARNING: trace_processor may not be fully ready, but continuing..."
fi

echo "=========================================="
echo "Configuring Perfetto UI..."
echo "=========================================="

# Configure Perfetto UI for proxy access
INDEX_FILE="/usr/share/nginx/html/perfetto/index.html"
if [ -f "$INDEX_FILE" ]; then
    echo "Configuring Perfetto UI..."
    
    # Set base path for Perfetto UI if UI_BASE_PATH is provided
    if [ -n "$UI_BASE_PATH" ]; then
        echo "Setting UI base path to: ${UI_BASE_PATH}"
        sed -i "s|<head>|<head><base href=\"${UI_BASE_PATH}\">|" "$INDEX_FILE"
    fi
    
    # Inject scripts: url-fix.js and httpd-connect.js
    # httpd-connect.js tells Perfetto to use the local HTTP server
    if [ -n "$UI_BASE_PATH" ]; then
        sed -i "s|<base href=\"${UI_BASE_PATH}\">|<base href=\"${UI_BASE_PATH}\"><script src=\"url-fix.js\"></script><script src=\"httpd-connect.js\"></script>|" "$INDEX_FILE"
    else
        sed -i 's|<head>|<head><script src="/url-fix.js"></script><script src="/httpd-connect.js"></script>|' "$INDEX_FILE"
    fi
    
    echo "Perfetto UI configured successfully"
fi

echo "=========================================="
echo "Starting nginx server..."
echo "=========================================="

# Start nginx in foreground
exec nginx -g 'daemon off;'
