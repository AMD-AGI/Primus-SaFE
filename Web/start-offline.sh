#!/bin/sh
# Minimal, dependency-free entrypoint for the offline release image
# (Web/Dockerfile.release). It does the one thing nginx needs to start — patch
# the DNS resolver into nginx.conf — using busybox awk/sed that ship in
# nginx:alpine, so the image needs no apk/bash/goaccess (which the build host's
# TLS-intercepting proxy blocks). The /__internal_analytics goaccess dashboard
# is intentionally omitted here; the console itself is unaffected.
set -e

RESOLVER_IP=$(awk '/^nameserver/{print $2; exit}' /etc/resolv.conf)
if [ -z "$RESOLVER_IP" ]; then
  echo "WARN: no nameserver in /etc/resolv.conf, falling back to 10.96.0.10"
  RESOLVER_IP="10.96.0.10"
fi
cp /etc/nginx/nginx.conf /tmp/nginx.conf
sed -i "s/__RESOLVER__/${RESOLVER_IP}/g" /tmp/nginx.conf

exec nginx -c /tmp/nginx.conf -g 'daemon off;'
