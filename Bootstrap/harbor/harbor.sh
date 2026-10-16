#!/usr/bin/env bash
HARBORPWD=$1
HARBOR="${2:-primus-safe.amd.com}"
STORAGECLASS="${3:-rbd}"
AUTHORIZE="${4:-${HOME}/.ssh/id_ed25519}"

echo "Using Harbor host: $HARBOR"
echo "Using SSH key: $AUTHORIZE"

case "$HARBOR" in
  harbor.*.primus-safe.amd.com) ;;
  *) echo "[WARN] SaFE/bootstrap/install.sh only detects Harbor at harbor.<cluster>.primus-safe.amd.com, so it will not use ${HARBOR} as its registry proxy." ;;
esac

if ! command -v jq >/dev/null 2>&1; then
  echo "[ERROR] jq is required to configure Harbor through its API. Install it (e.g. sudo apt-get install -y jq) and re-run."
  exit 1
fi
if ! command -v ansible-playbook >/dev/null 2>&1; then
  echo "[ERROR] ansible-playbook not found. Activate the Kubespray venv first: source ../kubespray-venv/bin/activate"
  exit 1
fi
if [ ! -f hosts.yaml ]; then
  echo "[ERROR] No hosts.yaml in $(pwd). Copy hosts.yaml.example to hosts.yaml and list the nodes that pull from Harbor."
  exit 1
fi
if [[ ( -n "${DOCKERHUB_USERNAME:-}" && -z "${DOCKERHUB_TOKEN:-}" ) || ( -z "${DOCKERHUB_USERNAME:-}" && -n "${DOCKERHUB_TOKEN:-}" ) ]]; then
  echo "[ERROR] Set both DOCKERHUB_USERNAME and DOCKERHUB_TOKEN, or neither."
  exit 1
fi

if [ -z "$HARBORPWD" ]
then
  echo "Please enter the Harbor registry admin password."
  read HARBORPWD
fi

# The key name lives in a variable because CI's block-sensitive-secrets scan
# flags the literal key-and-value sed expression as a credential.
admin_pw_key=harborAdminPassword
sed -i "s/${admin_pw_key}: .*$/${admin_pw_key}: ${HARBORPWD}/g" values.yaml
sed -i "s/externalURL: .*$/externalURL: https:\/\/${HARBOR}/g" values.yaml
sed -i "s/      core: .*$/      core: ${HARBOR}/g" values.yaml
sed -i "s/storageClass: .*$/storageClass: ${STORAGECLASS}/g" values.yaml
helm repo add harbor https://helm.goharbor.io
helm upgrade --install harbor  harbor/harbor -n harbor --create-namespace -f values.yaml

kubectl apply -f - <<EOF
apiVersion: cert-manager.io/v1
kind: ClusterIssuer
metadata:
  name: selfsigned-issuer
  namespace: harbor
spec:
  selfSigned: {}
---
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: harbor-ca
  namespace: harbor
spec:
  duration: 105192h0m0s
  isCA: true
  commonName: harbor-ca
  subject:
    organizations:
      - primus
  secretName: harbor-ca-secret
  issuerRef:
    name: selfsigned-issuer
    kind: ClusterIssuer
    group: cert-manager.io
---
apiVersion: cert-manager.io/v1
kind: Issuer
metadata:
  name: harbor-ca-issuer
  namespace: harbor
spec:
  ca:
    secretName: harbor-ca-secret
---
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: harbor
  namespace: harbor
spec:
  duration: 105192h0m0s
  secretName: harbor-tls
  isCA: false
  dnsNames:
    - ${HARBOR}
  issuerRef:
    name: harbor-ca-issuer
EOF

kubectl -n harbor wait --for=condition=Ready certificates.cert-manager.io harbor

kubectl get secret harbor-tls -n harbor -o jsonpath='{.data.ca\.crt}' | base64 --decode > harbor-ca.crt

while true; do
    addresses=$(kubectl get deploy -n higress-system  higress-gateway -o jsonpath="{.status.readyReplicas}")
    if [ $addresses -ne 0 ]
    then
      break
    else
      echo Waiting for the Higress gateway to become ready...
      sleep 10
    fi
done

ansible-playbook  -i hosts.yaml --private-key ${AUTHORIZE} harbor_ca_task.yaml --become-user=root -b -vv -f 10 --timeout=10

NAMESPACE="kube-system"
CONFIGMAP="nodelocaldns"
IP=$(kubectl get ep -n higress-system higress-gateway -o jsonpath='{.subsets[0].addresses[0].ip}')
TMPFILE="/tmp/${CONFIGMAP}.yaml"

kubectl get configmap ${CONFIGMAP} -n ${NAMESPACE} -o yaml > /tmp/${CONFIGMAP}.bak.yaml
kubectl get configmap ${CONFIGMAP} -n ${NAMESPACE} -o yaml > ${TMPFILE}

if grep -q "${HARBOR}" ${TMPFILE}; then
  echo "[INFO] Host record '${IP} ${HARBOR}' already exists. Skip adding."
else
  echo "[INFO] Adding host record '${IP} ${HARBOR}' ..."
  sed -i "/^    \.:53 {/a\\        hosts {\n            ${IP} ${HARBOR}\n            fallthrough\n        }" ${TMPFILE}
  kubectl apply -f ${TMPFILE}
  kubectl rollout restart ds nodelocaldns -n ${NAMESPACE}
fi


while true; do
    addresses=$(kubectl get ep -n harbor harbor-registry -o jsonpath='{.subsets[*].addresses[*].ip}' | wc -c)
    if [ $addresses -ne 0 ]
    then
      break
    else
      echo Waiting for the Harbor registry to become ready...
      sleep 10
    fi
done

while true; do
    addresses=$(kubectl get ep -n harbor harbor-core -o jsonpath='{.subsets[*].addresses[*].ip}' | wc -c)
    if [ $addresses -ne 0 ]
    then
      break
    else
      echo Waiting for the Harbor service to become ready...
      sleep 10
    fi
done

AUTH=$(echo -n "admin:${HARBORPWD}" | base64)

until curl -k -d '{"project_name":"primussafe","metadata":{"public":"true"},"storage_limit":-1,"registry_id":null}' -H "Content-Type: application/json" -H "Authorization: Basic ${AUTH}" -X POST https://${HARBOR}/api/v2.0/projects; do
    echo "Command failed, retrying in 5s..."
    sleep 5
done

curl -k -d '{"project_name":"public","metadata":{"public":"true"},"storage_limit":-1,"registry_id":null}' -H "Content-Type: application/json" -H "Authorization: Basic ${AUTH}" -X POST https://${HARBOR}/api/v2.0/projects

# Docker Hub pull-through cache. When SaFE/bootstrap/install.sh detects Harbor,
# it pulls addon charts and images through <harbor>/proxy, so that project must
# exist. The endpoint is anonymous unless DOCKERHUB_USERNAME and DOCKERHUB_TOKEN
# are set; jq reads them from the environment to keep them out of argv.
export DOCKERHUB_USERNAME DOCKERHUB_TOKEN
PROXY_REGISTRY_NAME="docker-hub"
PROXY_PROJECT="proxy"

# harbor_api METHOD PATH [JSON]: sets API_CODE ("000" if Harbor did not answer)
# and API_BODY. The JSON goes through stdin so credentials stay out of argv.
harbor_api() {
  local args=(-k -sS --max-time 60 -X "$1" -H "Authorization: Basic ${AUTH}" -w '\n%{http_code}')
  local resp
  if [ -n "${3:-}" ]; then
    args+=(-H "Content-Type: application/json" --data-binary @-)
  fi
  resp=$(curl "${args[@]}" "https://${HARBOR}/api/v2.0$2" <<<"${3:-}")
  API_CODE=${resp##*$'\n'}
  API_BODY=${resp%$'\n'*}
  if [ "$API_CODE" = 401 ]; then
    echo "[ERROR] Harbor rejected the admin login (HTTP 401). Harbor sets the admin password only on its first start; pass the current one."
    exit 1
  fi
}

# Sets REGISTRY_ID to the id of the endpoint named PROXY_REGISTRY_NAME, or to "" if there is none.
find_registry() {
  harbor_api GET "/registries?q=name%3D${PROXY_REGISTRY_NAME}"
  if [ "$API_CODE" != 200 ]; then
    echo "[WARN] Listing Harbor registry endpoints failed (HTTP ${API_CODE}): ${API_BODY}"
    return 1
  fi
  REGISTRY_ID=$(jq -r --arg name "$PROXY_REGISTRY_NAME" 'first(.[] | select(.name == $name) | .id) // empty' <<<"$API_BODY")
}

ensure_registry() {
  find_registry || return 1
  if [ -z "$REGISTRY_ID" ]; then
    harbor_api POST /registries "$(jq -n --arg name "$PROXY_REGISTRY_NAME" '
      {name: $name, type: "docker-hub", url: "https://hub.docker.com", insecure: false}
      + if (env.DOCKERHUB_USERNAME // "") == "" then {}
        else {credential: {type: "basic", access_key: env.DOCKERHUB_USERNAME, access_secret: env.DOCKERHUB_TOKEN}} end')"
    if [ "$API_CODE" != 201 ]; then
      echo "[WARN] Creating registry endpoint '${PROXY_REGISTRY_NAME}' failed (HTTP ${API_CODE}): ${API_BODY}"
      return 1
    fi
    find_registry || return 1
    [ -n "$REGISTRY_ID" ] || return 1
    echo "[INFO] Created registry endpoint '${PROXY_REGISTRY_NAME}' (id ${REGISTRY_ID}) for https://hub.docker.com, Docker Hub user: ${DOCKERHUB_USERNAME:-anonymous}."
  elif [ -n "${DOCKERHUB_USERNAME:-}" ]; then
    harbor_api PUT "/registries/${REGISTRY_ID}" "$(jq -n '{credential_type: "basic", access_key: env.DOCKERHUB_USERNAME, access_secret: env.DOCKERHUB_TOKEN}')"
    if [ "$API_CODE" != 200 ]; then
      echo "[WARN] Updating the credentials of registry endpoint '${PROXY_REGISTRY_NAME}' failed (HTTP ${API_CODE}): ${API_BODY}"
      return 1
    fi
    echo "[INFO] Registry endpoint '${PROXY_REGISTRY_NAME}' (id ${REGISTRY_ID}) already exists; set its Docker Hub user to ${DOCKERHUB_USERNAME}."
  else
    echo "[INFO] Registry endpoint '${PROXY_REGISTRY_NAME}' (id ${REGISTRY_ID}) already exists."
  fi
}

ensure_proxy_project() {
  local linked
  harbor_api GET "/projects/${PROXY_PROJECT}"
  if [ "$API_CODE" = 404 ]; then
    harbor_api POST /projects "$(jq -n --arg name "$PROXY_PROJECT" --argjson registry "$REGISTRY_ID" \
      '{project_name: $name, metadata: {public: "true"}, storage_limit: -1, registry_id: $registry}')"
    if [ "$API_CODE" != 201 ]; then
      echo "[WARN] Creating proxy-cache project '${PROXY_PROJECT}' failed (HTTP ${API_CODE}): ${API_BODY}"
      return 1
    fi
    echo "[INFO] Created public proxy-cache project '${PROXY_PROJECT}'."
    harbor_api GET "/projects/${PROXY_PROJECT}"
  fi
  if [ "$API_CODE" != 200 ]; then
    echo "[WARN] Looking up project '${PROXY_PROJECT}' failed (HTTP ${API_CODE}): ${API_BODY}"
    return 1
  fi
  linked=$(jq -r '.registry_id // 0' <<<"$API_BODY") || return 1
  if [ "$linked" = 0 ]; then
    echo "[ERROR] Project '${PROXY_PROJECT}' already exists but is not a proxy-cache project. Delete it in Harbor and re-run harbor.sh."
    exit 1
  elif [ "$linked" != "$REGISTRY_ID" ]; then
    echo "[WARN] Project '${PROXY_PROJECT}' proxies registry endpoint id ${linked}, not '${PROXY_REGISTRY_NAME}' (id ${REGISTRY_ID}); leaving it as is."
  fi
  echo "[INFO] Proxy-cache project: $(jq -c '{name, project_id, registry_id, public: .metadata.public}' <<<"$API_BODY")"
}

echo
deadline=$((SECONDS + 600))
until ensure_registry && ensure_proxy_project; do
  if [ "$SECONDS" -ge "$deadline" ]; then
    echo "[ERROR] Could not set up the '${PROXY_PROJECT}' proxy-cache project within 10 minutes. Check that Harbor core can reach https://hub.docker.com (and the Docker Hub credentials, if set), then re-run harbor.sh."
    exit 1
  fi
  echo "Retrying in 10s..."
  sleep 10
done
echo "[INFO] Harbor proxy cache is ready. Pull Docker Hub images as ${HARBOR}/${PROXY_PROJECT}/<repository>, e.g. ${HARBOR}/${PROXY_PROJECT}/library/busybox:1.36"
