#!/bin/bash
# Writes the nginx location that sends the API key route of an identity provider to it, to the file named by
# the first argument (nginx.conf includes it). It is driven by API_KEYS_UPSTREAM (chart: auth.api_keys_upstream,
# only when auth.mode is external). Unset or empty, the file is empty and /api/v1/apikeys goes to the apiserver
# with the rest of /api/v1/.
#
# Only /api/v1/apikeys and what is below it is routed. /api/apikeys, the console's compatibility alias, is not:
# it keeps reaching the apiserver, so the keys the apiserver issued before the switch can still be listed and
# revoked there.
set -eu

out="${1:?usage: api-keys-route.sh <output file>}"
upstream="${API_KEYS_UPSTREAM:-}"

if [ -z "$upstream" ]; then
  : > "$out"
  exit 0
fi

# host[:port] and nothing else: the value is written into nginx configuration. Use the full in-cluster name
# (service.namespace.svc.cluster.local): the resolver nginx asks does not apply the pod's search domains.
# Matched as a whole string (a line-by-line grep would accept a value with a second line).
pattern='^[A-Za-z0-9]([A-Za-z0-9.-]*[A-Za-z0-9])?(:[0-9]{1,5})?$'
if ! [[ "$upstream" =~ $pattern ]]; then
  echo "ERROR: API_KEYS_UPSTREAM must be host[:port], got: $upstream" >&2
  exit 1
fi

# A regular expression location: it is tried before the plain /api/v1/ prefix that goes to the apiserver.
# The upstream is a variable so that nginx resolves it through the http-level resolver on every request
# instead of pinning an address at start-up, and a proxy_pass with a variable and no URI passes the request
# URI on as it came, query string included.
cat > "$out" <<CONF
location ~ ^/api/v1/apikeys(/.*)?\$ {
    set \$api_keys_upstream ${upstream};
    proxy_pass http://\$api_keys_upstream;
    proxy_http_version 1.1;
    proxy_set_header Host \$host;
    proxy_set_header X-Real-IP \$remote_addr;
    proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto \$scheme;

    proxy_connect_timeout 30s;
    proxy_read_timeout 60s;
    proxy_send_timeout 60s;
}
CONF
