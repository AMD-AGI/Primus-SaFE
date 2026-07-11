#!/usr/bin/env bash
# Fetch (or mint) a user's platform API key ("ak-...") end to end:
#   - pulls the crypto key + DB password from cluster secrets
#   - port-forwards Postgres locally
#   - runs the platform-key tool (mirrors GetOrCreatePlatformKey)
#
# Usage:
#   ./run.sh                          # interactive: prompts for user id/name
#   ./run.sh <user-id> [user-name]    # non-interactive inputs
#   CREATE=1 ./run.sh <user-id> <name>  # mint if missing (WRITES to api_keys)
#
# Env overrides (defaults target the primus-safe release):
#   NS, CRYPTO_SECRET, DB_SECRET, PG_SVC, DB_NAME, DB_USER, LOCAL_PORT
set -euo pipefail

NS="${NS:-primus-safe}"
CRYPTO_SECRET="${CRYPTO_SECRET:-primus-safe-crypto}"
DB_SECRET="${DB_SECRET:-primus-safe-pguser-primus-safe}"
# CrunchyData's *-primary service is selectorless, so port-forward targets the
# master POD instead (resolved via the operator's role label). Override PG_POD
# to pin a specific pod.
PG_POD="${PG_POD:-}"
PG_MASTER_SELECTOR="${PG_MASTER_SELECTOR:-postgres-operator.crunchydata.com/role=master}"
DB_NAME="${DB_NAME:-primus-safe-db}"
DB_USER="${DB_USER:-primus-safe}"
LOCAL_PORT="${LOCAL_PORT:-15432}"
SSL_MODE="${SSL_MODE:-require}"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Inputs: args first, else prompt.
USER_ID="${1:-}"
USER_NAME="${2:-}"
if [ -z "$USER_ID" ]; then
  read -r -p "User ID: " USER_ID
fi
if [ -z "$USER_ID" ]; then
  echo "error: user id is required" >&2
  exit 1
fi
if [ -z "$USER_NAME" ]; then
  read -r -p "User name (optional, stored on create): " USER_NAME
fi

# Confirm minting when requested (writes to the production DB).
CREATE_FLAG=""
if [ "${CREATE:-0}" = "1" ]; then
  read -r -p "CREATE=1 will WRITE a platform key row for '$USER_ID'. Proceed? [y/N] " ans
  case "$ans" in
    y | Y | yes | YES) CREATE_FLAG="--create" ;;
    *) echo "aborted (running read-only)"; CREATE_FLAG="" ;;
  esac
fi

WORKDIR="$(mktemp -d)"
CRYPTO_FILE="$WORKDIR/crypto.key"
PASS_FILE="$WORKDIR/db.pass"
PF_LOG="$WORKDIR/pf.log"
PF_PID=""
cleanup() {
  [ -n "$PF_PID" ] && kill "$PF_PID" >/dev/null 2>&1 || true
  rm -rf "$WORKDIR"
}
trap cleanup EXIT

echo "==> fetching crypto key from secret/$CRYPTO_SECRET" >&2
kubectl get secret "$CRYPTO_SECRET" -n "$NS" -o jsonpath='{.data.key}' | base64 -d > "$CRYPTO_FILE"
echo "==> fetching db password from secret/$DB_SECRET" >&2
kubectl get secret "$DB_SECRET" -n "$NS" -o jsonpath='{.data.password}' | base64 -d > "$PASS_FILE"

# Default: this script starts its own port-forward on 127.0.0.1:$LOCAL_PORT.
# Set NO_PORT_FORWARD=1 (and DB_HOST/DB_PORT) to reuse an existing connection
# (e.g. a port-forward you started yourself, or in-cluster direct access).
DB_HOST="${DB_HOST:-127.0.0.1}"
DB_PORT="${DB_PORT:-$LOCAL_PORT}"

if [ "${NO_PORT_FORWARD:-0}" != "1" ]; then
  if [ -z "$PG_POD" ]; then
    PG_POD="$(kubectl get pods -n "$NS" -l "$PG_MASTER_SELECTOR" \
      -o jsonpath='{.items[0].metadata.name}' 2>/dev/null || true)"
  fi
  if [ -z "$PG_POD" ]; then
    echo "error: could not resolve master pg pod (selector: $PG_MASTER_SELECTOR)." >&2
    echo "hint: set PG_POD=<pod> explicitly, or use NO_PORT_FORWARD=1 with DB_HOST/DB_PORT." >&2
    exit 1
  fi
  echo "==> port-forwarding pod/$PG_POD $LOCAL_PORT:5432 (log: $PF_LOG)" >&2
  kubectl port-forward -n "$NS" "pod/$PG_POD" "$LOCAL_PORT:5432" >"$PF_LOG" 2>&1 &
  PF_PID=$!
  DB_HOST="127.0.0.1"
  DB_PORT="$LOCAL_PORT"

  # Readiness: wait for kubectl to log "Forwarding from ...:<port>" (up to ~30s).
  ready=0
  for _ in $(seq 1 60); do
    if ! kill -0 "$PF_PID" >/dev/null 2>&1; then
      echo "error: port-forward exited early. log:" >&2
      cat "$PF_LOG" >&2
      exit 1
    fi
    if grep -q "Forwarding from .*:$LOCAL_PORT" "$PF_LOG" 2>/dev/null; then
      ready=1
      break
    fi
    sleep 0.5
  done
  if [ "$ready" != "1" ]; then
    echo "error: port-forward not ready after 30s. log:" >&2
    cat "$PF_LOG" >&2
    echo "hint: is LOCAL_PORT=$LOCAL_PORT already in use? try LOCAL_PORT=25432 ./run.sh ..." >&2
    exit 1
  fi
fi

echo "==> resolving platform key for user $USER_ID (db $DB_HOST:$DB_PORT)" >&2
export GOWORK=off
go -C "$SCRIPT_DIR" run . \
  --user-id "$USER_ID" \
  --user-name "$USER_NAME" \
  $CREATE_FLAG \
  --db-host "$DB_HOST" --db-port "$DB_PORT" \
  --db-name "$DB_NAME" --db-user "$DB_USER" \
  --db-password-file "$PASS_FILE" \
  --crypto-key-file "$CRYPTO_FILE" \
  --ssl-mode "$SSL_MODE"
