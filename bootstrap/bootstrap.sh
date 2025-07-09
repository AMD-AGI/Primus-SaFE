#!/usr/bin/env bash

#
# Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

KUBE_CONFIG=/etc/kubernetes/admin.conf
AUTHORIZE=${HOME}/.ssh/id_rsa

KUBE_VERSION=1.32.5
KUBE=${HOME}/.kube
KUBE_CONTROL_HOSTS=3
CONFIG_FILE=../hosts.yaml
NODE_LOCAL_DNS_IP=169.254.25.10
KUBE_PODS_SUBNET=172.16.0.0/12 #10.232.0.0/14
KUBE_SERVICE_ADDRESSES=192.168.0.0/16 #10.236.0.0/14
KUBEENETWORKPLUGIN=cilium
NGINX_IMAGE=public.ecr.aws/docker/library/nginx
ARCH=$(uname -m)


if [ ! -d "kubespray" ]
then
  sudo git clone https://github.com/kubernetes-sigs/kubespray.git
  if [ ! -d "kubespray" ]
  then
    echo "git cloning failed, please try again."
    exit
  fi
fi

apt-get update -q \
    && apt-get install -yq --no-install-recommends \
    curl \
    python3 \
    python3-pip \
    sshpass \
    vim \
    rsync \
    openssh-client \
    && apt-get clean \

sudo sysctl -w fs.inotify.max_user_instances=8192

VENVDIR=kubespray-venv                                                                                                                                                                                                                                                                      
KUBESPRAYDIR=kubespray
python3 -m venv $VENVDIR
source $VENVDIR/bin/activate
cd $KUBESPRAYDIR
pip install -U -r requirements.txt

sudo sed -i 's/kube_kubeadm_apiserver_extra_args: {}/kube_kubeadm_apiserver_extra_args: \n  max-mutating-requests-inflight: 10000 \n  max-requests-inflight: 20000/g' roles/kubernetes/control-plane/defaults/main/main.yml
echo 'etcd_quota_backend_bytes: "8589934592"' >> roles/kubernetes/control-plane/defaults/main/etcd.yml
echo 'etcd_memory_limit: "8GB"' >> roles/kubernetes/control-plane/defaults/main/etcd.yml

ansible-playbook -i ${CONFIG_FILE} cluster.yml --become-user=root  \
  -e kube_version=${KUBE_VERSION} -e kube_pods_subnet=${KUBE_PODS_SUBNET} -e kube_service_addresses=${KUBE_SERVICE_ADDRESSES}\
  -e nodelocaldns_ip=${NODE_LOCAL_DNS_IP} -e auto_renew_certificates=true \
  -e nginx_image_repo=${NGINX_IMAGE} -e kube_network_plugin=${KUBEENETWORKPLUGIN} -b -vvv

if [ ! -f $KUBECONFIG ]
then
  echo "Failed to create kubernetes cluster, please try again"
  exit
fi

cd ..
mkdir -p $KUBE
sudo cp ${KUBECONFIG} ${KUBE}/config
sudo chmod -R 777 ${KUBE}/config

if [ ! -f "/usr/local/bin/helm" ]
then
    curl -fsSL -o get_helm.sh https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3
    chmod 777 get_helm.sh
    ./get_helm.sh
fi

helm repo add rocm https://rocm.github.io/gpu-operator
helm upgrade --install amd-gpu-operator -n kube-amd-gpu rocm/gpu-operator-charts --create-namespace

helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
helm upgrade --install prometheus prometheus-community/kube-prometheus-stack \
      -n prometheus --create-namespace --set installCRDs=true


helm repo add jetstack https://charts.jetstack.io --force-update
helm install cert-manager jetstack/cert-manager --namespace cert-manager \
    --create-namespace --version v1.17.2 --set crds.enabled=true

helm repo add higress.io https://higress.io/helm-charts
helm upgrade --install higress higress.io/higress -n higress-system --create-namespace \
      --set higress-core.gateway.hostNetwork=true

helm repo add nvidia https://helm.ngc.nvidia.com/nvidia
helm upgrade --install network-operator nvidia/network-operator -n network-operator --create-namespace

helm upgrade --install primus-safe oci://registry-1.docker.io/primussafe/primus-safe -n primus-safe \
      --version 0.2.0  --create-namespace

