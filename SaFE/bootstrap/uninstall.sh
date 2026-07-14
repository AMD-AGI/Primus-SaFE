#!/bin/bash

#
# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

# Best-effort teardown: keep going even if a release/namespace is already gone.
set +e

NAMESPACE="primus-safe"
OBS_NAMESPACE="primus-safe-observability"

# ── Admin plane + data plane (primus-safe namespace) ────────────────────
helm uninstall primus-safe -n "$NAMESPACE"

helm uninstall primus-safe-cr -n "$NAMESPACE"

helm uninstall grafana-operator -n "$NAMESPACE"

helm uninstall primus-pgo -n "$NAMESPACE"

helm uninstall node-agent -n "$NAMESPACE"

# ── SaFE-native observability stack ─────────────────────────────────────
# Installed by install.sh Step 6b as its own release in a dedicated namespace
# (VictoriaMetrics operator + VMCluster/VMAgent, kube-state-metrics, the gpu/
# rdma/network exporters and the metrics-enricher). The previous uninstall.sh
# left all of this running. Remove the release, then delete the namespace so
# its PVCs (vmstorage) and any leftover VM CRs are cleaned up too.
helm uninstall primus-safe-observability -n "$OBS_NAMESPACE"

# The VictoriaMetrics operator installs a validating admission webhook and puts
# a finalizer (apps.victoriametrics.com/finalizer) on its CRs. If the operator
# (and its webhook) is torn down before those finalizers clear, deleting the
# namespace hangs forever in "Terminating" and a later reinstall into it fails.
# Remove the webhook first, then force-clear finalizers on any residual VM CRs,
# so namespace teardown (and reinstall) is reliable. All best-effort.
kubectl delete validatingwebhookconfiguration,mutatingwebhookconfiguration \
  -l app.kubernetes.io/name=victoria-metrics-operator --ignore-not-found 2>/dev/null
kubectl get validatingwebhookconfigurations,mutatingwebhookconfigurations -o name 2>/dev/null \
  | grep -i "victoria-metrics-operator" | xargs -r kubectl delete --ignore-not-found 2>/dev/null

for kind in vmcluster vmagent vmalert vmalertmanager vmauth vmsingle vmnodescrape vmpodscrape vmservicescrape; do
  kubectl get "$kind" -n "$OBS_NAMESPACE" -o name 2>/dev/null | while read -r cr; do
    kubectl patch -n "$OBS_NAMESPACE" "$cr" --type=merge -p '{"metadata":{"finalizers":[]}}' 2>/dev/null
  done
done

kubectl delete namespace "$OBS_NAMESPACE" --ignore-not-found
