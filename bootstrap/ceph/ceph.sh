#!/usr/bin/env bash

helm repo add rook-release https://charts.rook.io/release
helm install --create-namespace --namespace rook-ceph rook-ceph rook-release/rook-ceph -f values/ceph-values.yaml

kubectl wait crd cephclusters.ceph.rook.io --for=condition=Established
kubectl apply -f cephcluster.yaml
kubectl wait cephcluster -n rook-ceph rook-ceph --for=condition=Ready

while true; do
    phase=$(kubectl get cephcluster -n rook-ceph rook-ceph -o jsonpath='{.status.phase}')
    if [ $phase == "Ready" ]
    then
      break
    else
      echo 
      sleep 10
    fi
done


CLUSTERID=$(kubectl get cephcluster -n rook-ceph rook-ceph  -o jsonpath='{.status.ceph.fsid}')

helm repo add ceph-csi https://ceph.github.io/csi-charts
helm install --namespace "ceph-csi-rbd" "ceph-csi-rbd" ceph-csi/ceph-csi-rbd --create-namespace
kubectl get cm -n ceph-csi-rbd ceph-csi-config -o yaml > ceph-csi-config.yaml
COUNT=$(cat ceph-csi-config.yaml | grep $CLUSTERID | wc -c)
if [ $COUNT -eq 0 ]
then
  MONITORS=$(echo $(kubectl get cm -n rook-ceph rook-ceph-mon-endpoints -o jsonpath='{.data.data}')| sed 's/[a-z0-9_-]\+=//g')
  MONITORS=$(echo $MONITORS | sed 's/\([^,]*\)/"\1"/g')
  SECRET=$(kubectl get secret -n rook-ceph rook-ceph-mon -o jsonpath='{.data.ceph-secret}')
  sed -e "s/CLUSTERID/${CLUSTERID}/g" ceph-csi.template > temp
  sed -e "s/MONITORS/${MONITORS}/g" temp > ceph-csi.json
  index=$(sed -n "/config.json:/=" ceph-csi-config.yaml)
  sed -e "${index}r ceph-csi.json" ceph-csi-config.yaml > temp && mv temp ceph-csi-config.yaml
  sed -e "s/config.json: .*$/config.json: |/g"  ceph-csi-config.yaml > temp && mv temp ceph-csi-config.yaml
  kubectl replace -f ceph-csi-config.yaml
  sed -e "s/clusterID:.*$/clusterID: {{${CLUSTERID}}}/g" values/storageclass.yaml > temp && mv temp storageclass.yaml
  sed -e "s/userKey:.*$/userKey: {{${SECRET}}}/g" values/storageclass.yaml > temp && mv temp storageclass.yaml
  kubectl delete sc storage-rbd
  kubectl apply -f values/storageclass.yaml
fi
rm ceph-csi-config.yaml

kubectl apply -f ceph-s3.yaml
sleep 10
kubectl get cephobjectstoreusers.ceph.rook.io -n rook-ceph admin-user -o jsonpath='{.status.info.secretName}'


