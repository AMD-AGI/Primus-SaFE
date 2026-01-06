#!/bin/sh
set -e

echo "=========================================="
echo "Perfetto Viewer Container Starting..."
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

# Build authentication header if token is provided
AUTH_HEADER=""
if [ -n "$INTERNAL_TOKEN" ]; then
    AUTH_HEADER="-H 'X-Internal-Token: ${INTERNAL_TOKEN}'"
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
            --max-time 300 \
            "${DOWNLOAD_URL}")
    else
        HTTP_CODE=$(curl -s -w "%{http_code}" -o "$TRACE_FILE" \
            --connect-timeout 30 \
            --max-time 300 \
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

# Check if it's valid trace file (JSON or protobuf)
# Perfetto traces can be JSON (starts with { or [) or protobuf binary
FIRST_BYTES=$(head -c 4 "$TRACE_FILE" | od -An -tx1 | tr -d ' ')
if echo "$FIRST_BYTES" | grep -q "^0a"; then
    echo "Detected protobuf format trace file"
elif head -c 100 "$TRACE_FILE" | grep -q '[{"\[]'; then
    echo "Detected JSON format trace file"
else
    echo "WARNING: Unknown trace file format"
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
    
    # Inject url-fix.js script to run BEFORE Perfetto loads
    # This removes the url parameter from hash to prevent "Invalid URL" error
    # Must be first script in <head> to run before any other scripts
    if [ -n "$UI_BASE_PATH" ]; then
        # Insert after <base> tag
        sed -i "s|<base href=\"${UI_BASE_PATH}\">|<base href=\"${UI_BASE_PATH}\"><script src=\"/url-fix.js\"></script>|" "$INDEX_FILE"
    else
        # Insert right after <head>
        sed -i 's|<head>|<head><script src="/url-fix.js"></script>|' "$INDEX_FILE"
    fi
    
    echo "Perfetto UI configured successfully"
fi

echo "=========================================="
echo "Starting nginx server..."
echo "=========================================="

# Start nginx in foreground
exec nginx -g 'daemon off;'

