#!/bin/bash

set -euo pipefail

MANIFEST_DIR="manifests"
NAMESPACE="primus-lens"

echo "============================"
echo "🔧 Step 1: Input Parameters"
echo "============================"

function validate_k8s_name() {
    local name="$1"
    if [[ ! "$name" =~ ^[a-z0-9]([-a-z0-9]*[a-z0-9])?$ ]]; then
        return 1
    fi
    if [[ ${#name} -gt 16 ]]; then
        return 1
    fi
    return 0
}


while true; do
    read -rp "Enter your cluster name (must be lowercase, alphanumeric or '-', start and end with alphanumeric, max 16 chars): " CLUSTER_NAME
    if validate_k8s_name "$CLUSTER_NAME"; then
        break
    else
        echo "Invalid cluster name. Please follow Kubernetes naming rules."
    fi
done

echo "Your cluster name is: $CLUSTER_NAME"

read -rp "Enter your StorageClass name: " STORAGE_CLASS
read -rp "Support ReadWriteMany? (y/n): " RWX_SUPPORT
read -rp "Enter deployment profile (minimal, normal, large):Choose based on your cluster size — this will determine the resource capacity allocated for all components: " PROFILE
read -rp "Enter domain access type (ingress/ssh tunnel) [default ssh tunnel]: " ACCESS_TYPE

if [[ -z "$ACCESS_TYPE" ]]; then
  ACCESS_TYPE="ssh tunnel"
fi

if [[ "$RWX_SUPPORT" == "y" || "$RWX_SUPPORT" == "Y" ]]; then
  ACCESS_MODE="ReadWriteMany"
else
  ACCESS_MODE="ReadWriteOnce"
fi

echo "✅ Cluster name: $CLUSTER_NAME"
echo "✅ StorageClass: $STORAGE_CLASS"
echo "✅ Access mode: $ACCESS_MODE"
echo "✅ Deployment profile: $PROFILE"


export CLUSTER_NAME STORAGE_CLASS ACCESS_MODE NAMESPACE MANIFEST_DIR

echo "============================"
echo "🔑 Step 1.1: Docker Registry Credentials"
echo "============================"

read -rp "Do you want to specify docker.io credentials? (y/n): " USE_DOCKER_CRED

if [[ "$USE_DOCKER_CRED" == "y" || "$USE_DOCKER_CRED" == "Y" ]]; then
    read -rp "Enter your docker.io username: " DOCKER_USER
    read -rsp "Enter your docker.io password: " DOCKER_PASS
    echo ""
    echo "🔐 Creating imagePullSecret with provided credentials..."

    # Create or update namespace
    kubectl get ns "$NAMESPACE" >/dev/null 2>&1 || kubectl create ns "$NAMESPACE"

    # Create the secret with docker credentials
    kubectl create secret docker-registry primus-lens-image \
      --docker-server=docker.io \
      --docker-username="$DOCKER_USER" \
      --docker-password="$DOCKER_PASS" \
      --namespace "$NAMESPACE" \
      --dry-run=client -o yaml | kubectl apply -f -
else
    echo "⚙️ No credentials provided. Creating empty imagePullSecret..."
    kubectl get ns "$NAMESPACE" >/dev/null 2>&1 || kubectl create ns "$NAMESPACE"
    cat <<EOF | kubectl apply -f -
apiVersion: v1
kind: Secret
metadata:
  name: primus-lens-image
  namespace: $NAMESPACE
type: kubernetes.io/dockerconfigjson
data:
  .dockerconfigjson: ""
EOF
fi

echo "✅ imagePullSecret 'primus-lens-image' has been created successfully."


CONFIG_FILE=configs/profiles.tbl

if [ -z "$PROFILE" ]; then
  echo "Usage: $0 {minimal|normal|large}"
  exit 1
fi

LINE=$(awk -v p="$PROFILE" 'NR>1 && $1==p {print}' "$CONFIG_FILE" | tr -s '[:space:]' ' ')

if [ -z "$LINE" ]; then
  echo "Profile '$PROFILE' not found!"
  exit 1
fi

IFS=' ' read -r \
  profile OPENSEARCH_DISK_SIZE OPENSEARCH_MEMORY OPENSEARCH_CPU \
  PG_BACKUP_SIZE PG_DATA_SIZE PG_REPLICAS \
  VMAGENT_CPU VMAGENT_MEMORY \
  VMSTORAGE_REPLICAS VMSTORAGE_CPU VMSTORAGE_MEMORY VMSTORAGE_SIZE \
  VMSELECT_REPLICAS VMSELECT_CPU VMSELECT_MEMORY \
  VMINSERT_REPLICAS VMINSERT_CPU VMINSERT_MEMORY <<< "$LINE"

export OPENSEARCH_DISK_SIZE OPENSEARCH_MEMORY OPENSEARCH_CPU \
  PG_BACKUP_SIZE PG_DATA_SIZE PG_REPLICAS \
  VMAGENT_CPU VMAGENT_MEMORY \
  VMSTORAGE_REPLICAS VMSTORAGE_CPU VMSTORAGE_MEMORY VMSTORAGE_SIZE \
  VMSELECT_REPLICAS VMSELECT_CPU VMSELECT_MEMORY \
  VMINSERT_REPLICAS VMINSERT_CPU VMINSERT_MEMORY


echo
echo "============================"
echo "📦 Step 2: Create Namespace and Service Account"
echo "============================"
kubectl create namespace "$NAMESPACE" --dry-run=client -o yaml | kubectl apply -f -
echo "✅ Namespace '$NAMESPACE' created or already exists."

envsubst < "$MANIFEST_DIR/app-sa.yaml" |kubectl apply -n "$NAMESPACE" -f -
envsubst < "$MANIFEST_DIR/cert.yaml.tpl" | kubectl apply -n "$NAMESPACE" -f -
echo "✅ app-sa.yaml applied."


echo "🔧 app-system-tuner..."
kubectl apply -n "$NAMESPACE" -f "$MANIFEST_DIR/app-system-tuner.yaml.tpl"

echo
echo "==============================="
echo "📈 Step 3: Install VictoriaMetrics Operator"
echo "==============================="
helm repo add vm https://victoriametrics.github.io/helm-charts/
helm repo update
helm upgrade --install -n "$NAMESPACE" primus-lens-vm vm/victoria-metrics-operator \
  -f "$MANIFEST_DIR/vm-operator-values.yaml.tpl"
echo "✅ VictoriaMetrics Operator installed."

echo "⏳ Step 3.1: Waiting for VictoriaMetrics Operator pod..."
for i in {1..30}; do
  if kubectl get pods -n "$NAMESPACE" | grep victoria-metrics-operator | grep -q "Running"; then
    echo "✅ VictoriaMetrics Operator is running."
    break
  fi
  echo "⏳ [$i/30] Waiting for victoria-metrics-operator..."
  sleep 5
done

echo
echo "=============================="
echo "🔥 Step 7: Install FluentBit Operator"
echo "=============================="
helm repo add fluent https://fluent.github.io/helm-charts
helm upgrade --install fluent-operator fluent/fluent-operator -n "$NAMESPACE" \
  -f "$MANIFEST_DIR/fluentbit-values.yaml.tpl"
echo "✅ FluentBit Operator installed."

echo "⏳ Step 7.1: Waiting for FluentBit Operator pod..."
for i in {1..30}; do
  if kubectl get pods -n "$NAMESPACE" | grep fluent-operator | grep -q "Running"; then
    echo "✅ FluentBit Operator is running."
    break
  fi
  echo "⏳ [$i/30] Waiting for fluent-operator..."
  sleep 5
done

echo
echo "=============================="
echo "🔍 Step 5: Install OpenSearch Operator"
echo "=============================="
helm repo add opensearch-operator https://opensearch-project.github.io/opensearch-k8s-operator/
helm upgrade --install -n "$NAMESPACE" opensearch-operator opensearch-operator/opensearch-operator \
  -f "$MANIFEST_DIR/opensearch-operator-value.yaml.tpl"
echo "✅ OpenSearch Operator installed."

echo "⏳ Step 5.1: Waiting for OpenSearch Operator pod..."
for i in {1..30}; do
  if kubectl get pods -n "$NAMESPACE" | grep opensearch-operator-controller | grep -q "Running"; then
    echo "✅ OpenSearch Operator is running."
    break
  fi
  echo "⏳ [$i/30] Waiting for opensearch-operator-controller..."
  sleep 5
done

echo
echo "============================"
echo "📄 Step 4: Apply vmcluster.yaml"
echo "============================"
envsubst < "$MANIFEST_DIR/vmcluster.yaml.tpl" | kubectl apply -n "$NAMESPACE" -f -
echo "✅ vmcluster.yaml applied."


echo
echo "=============================="
echo "📄 Step 6: Apply opensearch.yaml"
echo "=============================="
envsubst < "$MANIFEST_DIR/opensearch.yaml.tpl" | kubectl apply -n "$NAMESPACE" -f -
echo "✅ opensearch.yaml applied."

echo
echo "=============================="
echo "📄 Step 8: Apply fluent-bit config (includes scrape config)"
echo "=============================="
envsubst <"$MANIFEST_DIR/fluent-bit-config.yaml.tpl"| kubectl apply -n "$NAMESPACE" -f -
echo "✅ fluent-bit-config.yaml.tpl applied."

echo
echo "=============================="
echo "🐘 Step 9: Install PostgreSQL Operator"
echo "=============================="
rm -rf postgres-operator-examples
git clone https://github.com/CrunchyData/postgres-operator-examples.git
helm upgrade --install -n "$NAMESPACE" pg-operator postgres-operator-examples/helm/install \
  -f "$MANIFEST_DIR/pg-operator-values.yaml.tpl"
rm -rf postgres-operator-examples
echo "✅ PostgreSQL Operator installed."

echo "⏳ Step 9.1: Waiting for PostgreSQL Operator pod..."
for i in {1..30}; do
  if kubectl get pods -n "$NAMESPACE" | grep pgo- | grep -q "Running"; then
    echo "✅ PostgreSQL Operator is running."
    break
  fi
  echo "⏳ [$i/30] Waiting for pgo-..."
  sleep 5
done


echo
echo "=============================="
echo "📄 Step 10: Apply pg-cr.yaml"
echo "=============================="
envsubst < "$MANIFEST_DIR/pg-cr.yaml.tpl" | kubectl apply -n "$NAMESPACE" -f -
echo "✅ pg-cr.yaml applied."

echo
echo "=============================="
echo "⏳ Step 11: Wait for PostgreSQL service IP..."
echo "=============================="

SERVICE_NAME="primus-lens-ha"
IP=""
for i in {1..30}; do
  IP=$(kubectl get endpoints -n "$NAMESPACE" "$SERVICE_NAME" -o jsonpath="{.subsets[0].addresses[0].ip}" 2>/dev/null || echo "")
  if [[ -n "$IP" ]]; then
    echo "✅ PostgreSQL service IP found: $IP"
    break
  fi
  echo "⏳ [$i/30] Waiting for PostgreSQL endpoint IP..."
  sleep 5
done

if [[ -z "$IP" ]]; then
  echo "❌ Error: Failed to retrieve PostgreSQL IP after waiting."
  exit 1
fi

echo "🔍 Finding Pod by IP..."
POD_NAME=$(kubectl get pods -n "$NAMESPACE" -o wide | grep "$IP" | awk '{print $1}')
echo "✅ Pod Name: $POD_NAME"

echo
echo "📥 Step 12: Initialize PostgreSQL Database"
kubectl exec -i "$POD_NAME" -n "$NAMESPACE" -- psql -U postgres -d postgres < manifests/setup_primus_lens.sql
echo "✅ Database initialized."

echo
echo "🔐 Step 13: Extract PostgreSQL password from secret"
PG_PASSWORD=$(kubectl get secret -n "$NAMESPACE" primus-lens-pguser-primus-lens -o jsonpath="{.data.password}" | base64 -d)
export PG_PASSWORD
echo "✅ PG_PASSWORD loaded."

echo
echo "=============================="
echo "🚀 Step 14: Apply Primus Lens App Components"
echo "=============================="

echo "🔧 app-telemetry-collector..."
envsubst < "$MANIFEST_DIR/app-telemetry-collector.tpl" | kubectl apply -n "$NAMESPACE" -f -
echo "🔧 app-jobs..."
envsubst < "$MANIFEST_DIR/app-jobs.yaml.tpl" | kubectl apply -n "$NAMESPACE" -f -
echo "🔧 app-api..."
envsubst < "$MANIFEST_DIR/app-api.yaml.tpl" | kubectl apply -n "$NAMESPACE" -f -
echo "🔧 app-gpu-resource-exporter..."
envsubst < "$MANIFEST_DIR/app-gpu-resource-exporter.yaml.tpl" | kubectl apply -n "$NAMESPACE" -f -
echo "🔧 app-node-exporter..."
envsubst < "$MANIFEST_DIR/app-node-exporter.yaml.tpl" | kubectl apply -n "$NAMESPACE" -f -
echo "✅ All application components applied."

echo
echo "=============================="
echo "🔁 Step 15: Apply vmagent.yaml"
echo "=============================="
envsubst < "$MANIFEST_DIR/vmagent.yaml.tpl" | kubectl apply -n "$NAMESPACE" -f -
echo "✅ vmagent.yaml applied."

echo
echo "=============================="
echo "🔁 Step 16: Install console"
echo "=============================="
envsubst < "$MANIFEST_DIR/app-web.yaml.tpl" | kubectl apply -n "$NAMESPACE" -f -
echo "✅ Console installed."



echo
echo "=============================="
echo "📊 Step 17: Install Grafana"
echo "=============================="
export GRAFANA_ROOT_URL=$([[ "$ACCESS_TYPE" == "ssh tunnel" ]] && echo "http://127.0.0.1:30182/grafana" || echo "")
export GRAFANA_DOMAIN=$([[ "$ACCESS_TYPE" == "ingress" ]] && echo "$CLUSTER_NAME.lens-primus.ai" || echo "")
bash install-grafana.sh
echo "✅ Grafana installed."

echo
echo "=============================="
echo "🎉 Step 18: Apply Basic Metrics Scrape Config"
echo "=============================="
envsubst < "$MANIFEST_DIR/vmscrape-basic-metrics.yaml.tpl" | kubectl apply -n "$NAMESPACE" -f -



echo
echo "=============================="
echo "📊 Step 19: Install Kube State Metrics"
echo "=============================="
git clone https://github.com/kubernetes/kube-state-metrics.git
rm -rf kube-state-metrics/examples/standard/kustomization.yaml
kubectl apply -f kube-state-metrics/examples/standard
rm -rf kube-state-metrics

if [[ "$ACCESS_TYPE" == "ssh tunnel" ]]; then
    echo
    echo "=============================="
    echo "📊 Step 19: Configure Nginx"
    echo "=============================="
    echo "Access type is SSH tunnel, configuring nginx..."
    envsubst '${NAMESPACE}' < "$MANIFEST_DIR/primus-lens-nginx.yaml.tpl" | kubectl apply -n "$NAMESPACE" -f -
fi

echo
echo "🎉 All installation steps completed successfully!"

