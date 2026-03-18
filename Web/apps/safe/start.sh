#!/bin/bash
set -e

# Ensure log file and report directory exist
touch /var/log/nginx/analytics.log
mkdir -p /usr/share/nginx/html/__internal_analytics

# Start goaccess (background daemon)

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

goaccess /var/log/nginx/analytics.log \
  --log-format='%h [%d:%t %^] "%r" %s %b "%u" %e' \
  --date-format='%d/%b/%Y' \
  --time-format='%T' \
  --real-time-html \
  --addr=0.0.0.0 \
  --port=7890 \
  --ws-url="${WS_URL}" \
  -o /usr/share/nginx/html/__internal_analytics/report-live.html &

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

# Start nginx (foreground, keeps container running)
exec nginx -g 'daemon off;'
