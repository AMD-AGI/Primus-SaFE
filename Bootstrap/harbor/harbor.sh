#!/usr/bin/env bash
HARBORPWD=$1
HARBOR="${2:-primus-safe.amd.com}"
STORAGECLASS="${3:-rbd}"
AUTHORIZE="${4:-${HOME}/.ssh/id_ed25519}"

echo "Using Harbor host: $HARBOR"
echo "Using SSH key: $AUTHORIZE"

if [ -z "$HARBORPWD" ]
then
  echo "Please enter the Harbor registry admin password."
  read HARBORPWD
fi

sed -i "s/harborAdminPassword: .*$/harborAdminPassword: ${HARBORPWD}/g" values.yaml
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


