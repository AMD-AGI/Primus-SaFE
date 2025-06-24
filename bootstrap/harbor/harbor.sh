#!/usr/bin/env bash
HARBORPWD=$1
HARBOR=$(2:-harbor.safe.primus.ai)
AUTHORIZE=$(3:-${HOME}/.ssh/id_rsa)

if [ -z "$HARBORPWD" ]
then
  echo "Please enter the Harbor registry admin password."
  read HARBORPWD
fi

sed -i "s/harborAdminPassword: .*$/harborAdminPassword: ${HARBORPWD}/g" harbor/values.yaml
sed -i "s/externalURL: .*$/externalURL: https:\/\/${HARBOR}/g" harbor/values.yaml
sed -i "s/      core: .*$/      core: https:\/\/${HARBOR}/g" harbor/values.yaml
helm repo add harbor https://helm.goharbor.io
helm upgrade --install harbor  harbor/harbor -n harbor --create-namespace -f harbor/values.yaml

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
      - xcs
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
kubectl get secret -n harbor harbor-tls -o json | jq -r  '.data["ca.crt"]' | base64 -d > harbor-ca.crt

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

ansible-playbook  -i hosts.yaml --private-key ${AUTHORIZE} harbor_ca_task.yaml --become-user=root -b -vvv

sed -e "s/HARBORHOST/$(kubectl get ep -n higress-system higress-gateway -o jsonpath='{.subsets[0].addresses[0].ip}') ${HARBOR}/g" harbor/hosts.template > hosts
kubectl get cm nodelocaldns -n kube-system -o yaml >  nodelocaldns.yaml
sed -e "/kubectl.kubernetes.io.*$/d" nodelocaldns.yaml  > temp && mv temp nodelocaldns.yaml
sed -e "/annotations.*$/d"  nodelocaldns.yaml  > temp && mv temp nodelocaldns.yaml
sed -e "/{\"apiVersion.*$/d" nodelocaldns.yaml  > temp && mv temp nodelocaldns.yaml
index=$(sed -n "/\.:53 {/=" nodelocaldns.yaml)
sed -e "${index}r hosts" nodelocaldns.yaml > temp && mv temp nodelocaldns.yaml
kubectl apply -f nodelocaldns.yaml

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
    addresses=$(kubectl get ep -n harbor harbor -o jsonpath='{.subsets[*].addresses[*].ip}' | wc -c)
    if [ $addresses -ne 0 ]
    then
      break
    else
      echo Waiting for the Harbor service to become ready...
      sleep 10
    fi
done

AUTH=$(echo -n "admin:${HARBORPWD}" | base64)
curl -k -d '{"project_name":"primussafe","metadata":{"public":"true"},"storage_limit":-1,"registry_id":null}' -H "Content-Type: application/json" -H "Authorization: Basic ${AUTH}" -X POST https://${HARBOR}/api/v2.0/projects
curl -k -d '{"project_name":"public","metadata":{"public":"true"},"storage_limit":-1,"registry_id":null}' -H "Content-Type: application/json" -H "Authorization: Basic ${AUTH}" -X POST https://${HARBOR}/api/v2.0/projects


