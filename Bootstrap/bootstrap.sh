#!/usr/bin/env bash

KUBE_CONFIG=/etc/kubernetes/admin.conf

KUBE_VERSION=1.32.5
KUBE_DIR=${HOME}/.kube
CONFIG_FILE=hosts.ini
NODE_LOCAL_DNS_IP=169.254.25.10
KUBE_PODS_SUBNET=172.16.0.0/12 #10.232.0.0/14
KUBE_SERVICE_ADDRESSES=192.168.0.0/16 #10.236.0.0/14
KUBE_NETWORK_PLUGIN=cilium
NGINX_IMAGE=public.ecr.aws/docker/library/nginx

# -----------------------------------------------------------------------------
# 1. Clone Kubespray (if missing)
# -----------------------------------------------------------------------------
if [[ ! -d kubespray ]]; then
  sudo git clone https://github.com/kubernetes-sigs/kubespray.git || {
    echo "[ERROR] Git clone failed – please check connectivity or permissions." >&2
    exit 1
  }
fi
 
# -----------------------------------------------------------------------------
# 2. Install build dependencies (silently)
# -----------------------------------------------------------------------------
# REVIEW: use non‑interactive apt and trap failures. Combine into a single layer if
# you convert to Docker later.
  sudo apt-get update -qq && \
  sudo DEBIAN_FRONTEND=noninteractive apt-get install -yq --no-install-recommends \
       curl python3 python3-pip python3-venv sshpass vim rsync openssh-client && \
  sudo apt-get clean

# Increase inotify limit for large ansible runs
sudo sysctl -w fs.inotify.max_user_instances=8192

# -----------------------------------------------------------------------------
# 3. Python virtualenv for Kubespray
# -----------------------------------------------------------------------------
VENVDIR=kubespray-venv

KUBESPRAYDIR=kubespray
python3 -m venv $VENVDIR
source $VENVDIR/bin/activate
cd $KUBESPRAYDIR
pip install -U -r requirements.txt
 
case $1 in
    reset)
        shift
        ansible-playbook -i "../$CONFIG_FILE" reset.yml --become --become-user=root \
          -e kube_version="$KUBE_VERSION" \
          -e kube_pods_subnet="$KUBE_PODS_SUBNET" \
          -e kube_service_addresses="$KUBE_SERVICE_ADDRESSES" \
          -e nodelocaldns_ip="$NODE_LOCAL_DNS_IP" \
          -e auto_renew_certificates=true \
          -e nginx_image_repo="$NGINX_IMAGE" \
          -e kube_network_plugin="$KUBE_NETWORK_PLUGIN" \
          -v
        exit 0
        ;;
esac

ansible-playbook -i "../$CONFIG_FILE" ../playbooks/set-vm-max-map-count.yml --become --become-user=root -v

# -----------------------------------------------------------------------------
# 4. Patch Kubespray defaults (APIServer & ETCD tuning)
# -----------------------------------------------------------------------------
# REVIEW: Use ansible‑extra‑vars or inventory overrides rather than sed—less fragile.
sudo sed -i 's/kube_kubeadm_apiserver_extra_args: {}/kube_kubeadm_apiserver_extra_args: \n  max-mutating-requests-inflight: 10000 \n  max-requests-inflight: 20000/g' \
  roles/kubernetes/control-plane/defaults/main/main.yml
{
  echo 'etcd_quota_backend_bytes: "8589934592"'
  echo 'etcd_memory_limit: "8GB"'
} | sudo tee -a roles/kubernetes/control-plane/defaults/main/etcd.yml >/dev/null

{
  echo securityContext:
  echo "  privileged: true"
} | sudo tee -a roles/network_plugin/cilium/templates/values.yaml.j2 >/dev/null

# -----------------------------------------------------------------------------
# 5. Run Kubespray playbook
# -----------------------------------------------------------------------------
ansible-playbook -i "../$CONFIG_FILE" cluster.yml --become --become-user=root \
  -e kube_version="$KUBE_VERSION" \
  -e kube_pods_subnet="$KUBE_PODS_SUBNET" \
  -e kube_service_addresses="$KUBE_SERVICE_ADDRESSES" \
  -e nodelocaldns_ip="$NODE_LOCAL_DNS_IP" \
  -e auto_renew_certificates=true \
  -e nginx_image_repo="$NGINX_IMAGE" \
  -e kube_network_plugin="$KUBE_NETWORK_PLUGIN" \
  -v


# -----------------------------------------------------------------------------
# 6. Verify kubeconfig and set up local client directory
# -----------------------------------------------------------------------------
if [[ ! -f $KUBE_CONFIG ]]; then
  echo "[ERROR] Kubernetes cluster appears not to have been created (missing $KUBE_CONFIG)." >&2
  exit 1
fi
 
mkdir -p "$KUBE_DIR"
sudo cp "$KUBE_CONFIG" "$KUBE_DIR/config"
sudo chown "$(id -u):$(id -g)" "$KUBE_DIR/config"

kubectl delete deploy -n kube-system dns-autoscaler

# -----------------------------------------------------------------------------
# 7. Helm installation (skip if already installed)
# -----------------------------------------------------------------------------
if ! command -v helm >/dev/null; then
  curl -fsSL https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3 -o /tmp/get_helm.sh && \
  chmod +x /tmp/get_helm.sh && \
  /tmp/get_helm.sh
fi
 
# -----------------------------------------------------------------------------
# 8. Helm repos and chart deployments
# -----------------------------------------------------------------------------
# REVIEW: Consider pinning chart versions for reproducibility.
helm repo add rocm https://rocm.github.io/gpu-operator
helm repo add jetstack https://charts.jetstack.io --force-update

helm upgrade --install cert-manager jetstack/cert-manager \
  --namespace cert-manager --create-namespace \
  --version v1.17.2 --set installCRDs=true

helm upgrade --install amd-gpu-operator rocm/gpu-operator-charts \
  --namespace kube-amd-gpu --create-namespace
 
helm repo add prometheus-community https://prometheus-community.github.io/helm-charts

helm upgrade --install prometheus prometheus-community/kube-prometheus-stack \
  --namespace prometheus --create-namespace --set installCRDs=true
 
helm upgrade --install network-operator oci://registry-1.docker.io/primussafe/network-operator --version 25.4.0 \
  --namespace network-operator --create-namespace

helm upgrade --install kube-scheduler-plugins oci://registry-1.docker.io/primussafe/scheduler-plugins --version 0.31.8 \
  --namespace kube-system --create-namespace

 
