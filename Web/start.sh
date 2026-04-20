#!/bin/bash
set -e

# Ensure the analytics log file and report directory exist before goaccess
# starts writing to them.
touch /var/log/nginx/analytics.log
mkdir -p /usr/share/nginx/html/__internal_analytics

: "${PUBLIC_HOST:=localhost}"
: "${PUBLIC_SCHEME:=https}"

if [ "$PUBLIC_SCHEME" = "https" ]; then
  WS_SCHEME=wss
  WS_EXTERNAL_PORT=443
else
  WS_SCHEME=ws
  WS_EXTERNAL_PORT=80
fi

WS_URL="${WS_SCHEME}://${PUBLIC_HOST}:${WS_EXTERNAL_PORT}/__internal_analytics/ws"

# Live HTML report fed by a long-running goaccess instance.
goaccess /var/log/nginx/analytics.log \
  --log-format='%h [%d:%t %^] "%r" %s %b "%u" %e' \
  --date-format='%d/%b/%Y' \
  --time-format='%T' \
  --real-time-html \
  --addr=0.0.0.0 \
  --port=7890 \
  --ws-url="${WS_URL}" \
  -o /usr/share/nginx/html/__internal_analytics/report-live.html &

# Periodic static snapshot (cheap fallback if the WebSocket pipe drops).
(
  while true; do
    goaccess /var/log/nginx/analytics.log \
      --log-format='%h [%d:%t %^] "%r" %s %b "%u" %e' \
      --date-format='%d/%b/%Y' \
      --time-format='%T' \
      -o /usr/share/nginx/html/__internal_analytics/snapshot.html \
      > /dev/null 2>&1
    sleep 30
  done
) &

exec nginx -g 'daemon off;'
