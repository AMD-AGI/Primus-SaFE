#!/usr/bin/env bash
set -euo pipefail

REPO_RAW="https://raw.githubusercontent.com/AMD-AGI/Primus-SaFE/main/Scripts/setup-certs"

echo "[1/4] Downloading CA certificates from GitHub..."
curl -fsSL "$REPO_RAW/amd-root-ca.crt"    -o /usr/local/share/ca-certificates/amd-root-ca.crt
curl -fsSL "$REPO_RAW/amd-issuing-ca.crt" -o /usr/local/share/ca-certificates/amd-issuing-ca.crt

echo "[2/4] Updating system trust store..."
update-ca-certificates

echo "[3/4] Configuring Node.js to use system CA store..."
if grep -q 'NODE_EXTRA_CA_CERTS' /etc/environment 2>/dev/null; then
    echo "  NODE_EXTRA_CA_CERTS already set in /etc/environment, skipping."
else
    echo 'NODE_EXTRA_CA_CERTS=/etc/ssl/certs/ca-certificates.crt' >> /etc/environment
    echo "  Added NODE_EXTRA_CA_CERTS to /etc/environment."
fi

echo "[4/4] Done!"
echo ""
echo "  curl, python, Node.js (Cursor) will now trust AMD internal services."
echo "  If using Cursor Remote SSH, reconnect to pick up the new environment."
