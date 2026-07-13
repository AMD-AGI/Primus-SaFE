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
kubectl delete namespace "$OBS_NAMESPACE" --ignore-not-found
