#!/usr/bin/env bash
set -euo pipefail

# Re-exec as root if needed, so the `curl ... | bash` one-liner still works
# on hosts where the user is not root but has sudo.
if [ "$(id -u)" -ne 0 ]; then
    if command -v sudo >/dev/null 2>&1; then
        echo "Not running as root, re-executing with sudo..."
        exec sudo -E bash "$0" "$@"
    fi
    echo "ERROR: this script must run as root, and sudo is not available." >&2
    exit 1
fi

REPO_RAW="https://raw.githubusercontent.com/AMD-AGI/Primus-SaFE/main/Scripts/setup-certs"

# Detect the distro family and pick the matching trust-store layout. Debian and
# RHEL families use different anchor dirs, update tools, and Node CA bundles.
ID="" ID_LIKE="" PRETTY_NAME=""
if [ -r /etc/os-release ]; then
    . /etc/os-release
fi
case " ${ID_LIKE:-} ${ID:-} " in
    *rhel*|*fedora*|*centos*|*rocky*|*almalinux*)
        ANCHOR_DIR=/etc/pki/ca-trust/source/anchors
        UPDATE_CMD=(update-ca-trust extract)
        NODE_BUNDLE=/etc/pki/tls/certs/ca-bundle.crt
        ;;
    *debian*|*ubuntu*)
        ANCHOR_DIR=/usr/local/share/ca-certificates
        UPDATE_CMD=(update-ca-certificates)
        NODE_BUNDLE=/etc/ssl/certs/ca-certificates.crt
        ;;
    *)
        echo "ERROR: unsupported distro: ${PRETTY_NAME:-${ID:-unknown}}" >&2
        echo "  Supported: Debian/Ubuntu and RHEL/CentOS/Rocky/Alma/Fedora families." >&2
        exit 1
        ;;
esac

ROOT_CA="$ANCHOR_DIR/amd-root-ca.crt"
ISSUING_CA="$ANCHOR_DIR/amd-issuing-ca.crt"

echo "[1/4] Downloading CA certificates from GitHub..."
mkdir -p "$ANCHOR_DIR"
curl -fsSL "$REPO_RAW/amd-root-ca.crt"    -o "$ROOT_CA"
curl -fsSL "$REPO_RAW/amd-issuing-ca.crt" -o "$ISSUING_CA"

echo "[2/4] Updating system trust store..."
"${UPDATE_CMD[@]}"

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
        cat "$ROOT_CA" "$ISSUING_CA" >> "$BUNDLE"
        echo "  Appended AMD certs to $BUNDLE"
    fi
else
    echo "  Python/certifi not found, skipping."
fi

echo "[4/5] Configuring Node.js to use system CA store..."
if grep -q 'NODE_EXTRA_CA_CERTS' /etc/environment 2>/dev/null; then
    echo "  NODE_EXTRA_CA_CERTS already set in /etc/environment, skipping."
else
    echo "NODE_EXTRA_CA_CERTS=$NODE_BUNDLE" >> /etc/environment
    echo "  Added NODE_EXTRA_CA_CERTS to /etc/environment."
fi

echo "[5/5] Done!"
echo ""
echo "  curl, python (requests/httpx), Node.js (Cursor) will now trust AMD internal services."
echo "  If using Cursor Remote SSH, reconnect to pick up the new environment."
