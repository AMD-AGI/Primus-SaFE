#!/bin/bash

NAMESPACE="${NAMESPACE:-primus-lens}"


echo "Uninstalling Grafana from namespace: $NAMESPACE"
helm uninstall -n "$NAMESPACE" grafana


echo "Uninstalling Victoria Metrics cluster"
kubectl delete -n "$NAMESPACE" vmcluster --ignore-not-found primus-lens-metrics

echo "Uninstalling Victoria Metrics agent"
kubectl delete -n "$NAMESPACE" vmagent --ignore-not-found primus-lens-vm

echo "Uninstalling Opensearch"
kubectl delete -n "$NAMESPACE" opensearchcluster --ignore-not-found primus-lens-logs

echo "Uninstalling Postgres"
kubectl delete -n "$NAMESPACE" postgrescluster --ignore-not-found primus-lens

echo "Uninstalling fluentbit"
kubectl delete -n "$NAMESPACE" fluentbits --ignore-not-found fluent-bit

echo "Uninstalling grafana"
kubectl get grafanadashboards -n "$NAMESPACE"|awk '{print $1}'|xargs kubectl delete grafanadashboard -n "$NAMESPACE" --ignore-not-found
kubectl get grafanafolders -n "$NAMESPACE"|awk '{print $1}'|xargs kubectl delete grafanafolder -n "$NAMESPACE" --ignore-not-found
kubectl get grafanadatasources -n "$NAMESPACE"|awk '{print $1}'|xargs kubectl delete grafanadatasource -n "$NAMESPACE" --ignore-not-found
kubectl delete -n "$NAMESPACE" grafana --ignore-not-found grafana


echo "Waiting for resources to be deleted..."
sleep 20

echo "Uninstalling Operators"

echo "Uninstalling Postgres Operator"
helm uninstall -n "$NAMESPACE" pg-operator --ignore-not-found

echo "Uninstalling Opensearch Operator"
helm uninstall -n "$NAMESPACE" opensearch-operator --ignore-not-found

echo "Uninstalling Victoria Metrics Operator"
helm uninstall -n "$NAMESPACE" primus-lens-vm --ignore-not-found

echo "Uninstalling Fluent Operator"
helm uninstall -n "$NAMESPACE" fluent-operator --ignore-not-found

echo "Uninstalling Grafana"
helm uninstall -n "$NAMESPACE" grafana-operator --ignore-not-found


echo "Deleting ClusterRoleBinding"
kubectl delete clusterrolebinding primus-lens-binding --ignore-not-found

sleep 10

echo "Clear Pods"
kubectl get pods -n primus-lens|awk '{print $1}'|xargs kubectl delete pod -n primus-lens --force

echo "Deleting Namespace"
kubectl delete namespace "$NAMESPACE" --ignore-not-found

echo "Uninstallation complete."