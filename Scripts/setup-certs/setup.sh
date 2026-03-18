#!/usr/bin/env bash
set -euo pipefail

REPO_RAW="https://raw.githubusercontent.com/AMD-AGI/Primus-SaFE/main/Scripts/setup-certs"

echo "[1/4] Downloading CA certificates from GitHub..."
curl -fsSL "$REPO_RAW/amd-root-ca.crt"    -o /usr/local/share/ca-certificates/amd-root-ca.crt
curl -fsSL "$REPO_RAW/amd-issuing-ca.crt" -o /usr/local/share/ca-certificates/amd-issuing-ca.crt

echo "[2/4] Updating system trust store..."
update-ca-certificates

echo "[3/5] Appending CA certs to Python certifi bundle..."
BUNDLE=""
for py in python3 python; do
    BUNDLE=$(command -v "$py" &>/dev/null && "$py" -c "import certifi; print(certifi.where())" 2>/dev/null) && break
    BUNDLE=""
done
if [ -n "$BUNDLE" ]; then
    if grep -q 'AMD Corporate Root CA' "$BUNDLE" 2>/dev/null; then
        echo "  certifi bundle already contains AMD certs ($BUNDLE), skipping."
    else
        cat /usr/local/share/ca-certificates/amd-root-ca.crt \
            /usr/local/share/ca-certificates/amd-issuing-ca.crt >> "$BUNDLE"
        echo "  Appended AMD certs to $BUNDLE"
    fi
else
    echo "  Python/certifi not found, skipping."
fi

echo "[4/5] Configuring Node.js to use system CA store..."
if grep -q 'NODE_EXTRA_CA_CERTS' /etc/environment 2>/dev/null; then
    echo "  NODE_EXTRA_CA_CERTS already set in /etc/environment, skipping."
else
    echo 'NODE_EXTRA_CA_CERTS=/etc/ssl/certs/ca-certificates.crt' >> /etc/environment
    echo "  Added NODE_EXTRA_CA_CERTS to /etc/environment."
fi

echo "[5/5] Done!"
echo ""
echo "  curl, python (requests/httpx), Node.js (Cursor) will now trust AMD internal services."
echo "  If using Cursor Remote SSH, reconnect to pick up the new environment."
