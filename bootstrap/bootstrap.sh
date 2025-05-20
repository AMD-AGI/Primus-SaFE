#!/usr/bin/env bash

HARBORPWD=$1
DOMAIN=$2
CLUSTER=cluster-test
STORAGE=rook-ceph
KUBE_CONFIG=/etc/kubernetes/admin.conf
AUTHORIZE=${HOME}/.ssh/id_rsa

KUBE_VERSION=v1.30.2
KUBE=${HOME}/.kube
KUBE_CONTROL_HOSTS=3
HOST_PREFIX=xcloud-
CONFIG_FILE=../hosts.yaml
NODE_LOCAL_DNS_IP=169.254.25.10
KUBE_PODS_SUBNET=10.0.0.0/16 #10.232.0.0/14
KUBE_SERVICE_ADDRESSES=10.254.0.0/16 #10.236.0.0/14
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
  cd kubespray
  sudo git checkout -b release-2.27 remotes/origin/release-2.27 
  cd ..
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

sudo sed -i 's/kube_kubeadm_apiserver_extra_args: {}/kube_kubeadm_apiserver_extra_args: \n  max-mutating-requests-inflight: 1000 \n  max-requests-inflight: 2000/g' roles/kubernetes/control-plane/defaults/main/main.yml


ansible-playbook -i ${CONFIG_FILE} cluster.yml --become-user=root  \
  -e kube_version=${KUBE_VERSION} -e kube_pods_subnet=${KUBE_PODS_SUBNET} -e kube_service_addresses=${KUBE_SERVICE_ADDRESSES}\
  -e nodelocaldns_ip=${NODE_LOCAL_DNS_IP} -e kube_network_node_prefix=24 -e auto_renew_certificates=true \
  -e nginx_image_repo=$NGINX_IMAGE -e kube_network_plugin=cilium -b -vvv

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

helm repo add jetstack https://charts.jetstack.io --force-update

helm install \
    cert-manager jetstack/cert-manager \
    --namespace cert-manager \
    --create-namespace \
    --version v1.17.2 \
    --set crds.enabled=true

helm repo add higress.io https://higress.io/helm-charts
helm install higress higress.io/higress -n higress-system --create-namespace --set higress-core.gateway.hostNetwork=true


helm install primus-safe oci://registry-1.docker.io/primussafe/primus-safe --version 0.1.0 -n primus-safe --create-namespace
