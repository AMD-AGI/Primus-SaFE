#!/usr/bin/env bash
kubectl create -f https://raw.githubusercontent.com/pingcap/tidb-operator/v1.6.2/manifests/crd.yaml

helm repo add pingcap https://charts.pingcap.org/
helm repo add juicefs https://juicedata.github.io/charts/
helm repo update

helm upgrade --install --namespace tidb-admin tidb-operator pingcap/tidb-operator --version v1.6.2 --create-namespace
kubectl apply -f tidb-cluster.yaml

helm upgrade --install juicefs-csi-driver juicefs/juicefs-csi-driver -n juicefs-csi --create-namespace -f  juicefs-csi-values.yaml


MONITORS=$(echo $(kubectl get cm -n rook-ceph rook-ceph-mon-endpoints -o jsonpath='{.data.data}')| sed 's/[a-z0-9_-]\+=//g')
MONITORS=$(echo $MONITORS | sed 's/\([^,]*\)/"\1"/g')
SECRET=$(kubectl get secret -n rook-ceph rook-ceph-mon -o jsonpath='{.data.ceph-secret}' | base64 -d)

echo $MONITORS
MONITORS=${MONITORS//\"/}
echo $MONITORS
echo "[global] 
mon_host = ${MONITORS}

[client.admin]
keyring = /etc/ceph/keyring" > ceph.conf

echo "[client.admin]
key = $SECRET" > keyring

kubectl -n juicefs-csi create secret generic ceph-secret \
  --from-file=ceph.conf \
  --from-file=keyring

cat <<EOYAML | kubectl apply -f -
apiVersion: v1
stringData:
  access-key: ceph
  bucket: ceph://juicefs
  configs: '{"ceph-secret": "/etc/ceph"}'
  metaurl: tikv://tidb-pd.juicefs-csi:2379/juicefs
  name: juicefs
  secret-key: client.admin
  storage: ceph
kind: Secret
metadata:
  name: juicefs-secret
  namespace: juicefs-csi
type: Opaque
EOYAML

kubectl apply -f juicefs-storageclass.yaml

kubectl -n rook-ceph exec -it deploy/rook-ceph-tools -- \
  ceph osd pool application enable juicefs juicefs --yes-i-really-mean-it
