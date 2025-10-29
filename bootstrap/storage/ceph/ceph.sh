#!/usr/bin/env bash

# Add the Rook Helm repository for Ceph
helm repo add rook-release https://charts.rook.io/release
# Install the Rook Ceph operator in the 'rook-ceph' namespace
helm install --create-namespace --namespace rook-ceph rook-ceph rook-release/rook-ceph

# Wait for the CephCluster CRD to be established
kubectl wait crd cephclusters.ceph.rook.io --for=condition=Established
# Apply the Ceph cluster configuration
kubectl apply -f cephcluster.yaml
# Wait for the Ceph cluster to be ready
kubectl wait cephcluster -n rook-ceph rook-ceph --for=condition=Ready

# Poll the Ceph cluster status until it is 'Ready'
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

# Get the Ceph cluster ID (FSID)
CLUSTERID=$(kubectl get cephcluster -n rook-ceph rook-ceph  -o jsonpath='{.status.ceph.fsid}')

# Add the Ceph CSI Helm repository
helm repo add ceph-csi https://ceph.github.io/csi-charts
# Install the Ceph CSI RBD driver in its own namespace
helm install --namespace "ceph-csi-rbd" "ceph-csi-rbd" ceph-csi/ceph-csi-rbd --create-namespace
# Export the Ceph CSI configmap to a local file
kubectl get cm -n ceph-csi-rbd ceph-csi-config -o yaml > ceph-csi-config.yaml
# Check if the cluster ID is already present in the config
COUNT=$(cat ceph-csi-config.yaml | grep $CLUSTERID | wc -c)
if [ $COUNT -eq 0 ]
then
  # Get the monitor endpoints from the Rook Ceph configmap
  MONITORS=$(echo $(kubectl get cm -n rook-ceph rook-ceph-mon-endpoints -o jsonpath='{.data.data}')| sed 's/[a-z0-9_-]\+=//g')
  MONITORS=$(echo $MONITORS | sed 's/\([^,]*\)/"\1"/g')
  # Get the Ceph secret for the cluster
  SECRET=$(kubectl get secret -n rook-ceph rook-ceph-mon -o jsonpath='{.data.ceph-secret}')
  # Prepare the CSI config JSON with the cluster ID and monitors
  sed -e "s/CLUSTERID/${CLUSTERID}/g" csi.template > temp
  sed -e "s/MONITORS/${MONITORS}/g" temp > ceph-csi.json
  # Insert the new config into the configmap YAML
  index=$(sed -n "/config.json:/=" ceph-csi-config.yaml)
  sed -e "${index}r ceph-csi.json" ceph-csi-config.yaml > temp && mv temp ceph-csi-config.yaml
  # Fix the config.json formatting in the YAML
  sed -e "s/config.json: .*$/config.json: |/g"  ceph-csi-config.yaml > temp && mv temp ceph-csi-config.yaml
  # Replace the configmap in the cluster
  kubectl replace -f ceph-csi-config.yaml
  # Update the storage class YAML with the cluster ID and secret
  sed -e "s/clusterID:.*$/clusterID: ${CLUSTERID}/g" storageclass.yaml > temp && mv temp storageclass.yaml
  sed -e "s/userKey:.*$/userKey: ${SECRET}/g" storageclass.yaml > temp && mv temp storageclass.yaml
  # Delete and re-apply the storage class
  kubectl delete sc storage-rbd
  kubectl apply -f storageclass.yaml
fi
# Clean up the temporary config file
rm ceph-csi-config.yaml

# Deploy the Ceph S3 object store user
kubectl apply -f ceph-rgw.yaml
sleep 10
# Output the secret name for the admin user
kubectl get cephobjectstoreusers.ceph.rook.io -n rook-ceph admin-user -o jsonpath='{.status.info.secretName}'


