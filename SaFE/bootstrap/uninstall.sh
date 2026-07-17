#!/bin/bash

#
# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

# All helm uninstalls tolerate a not-found release so this script is safely
# rerunnable (idempotent): re-running it will still run the operator-workload
# cleanup + PVC handling below even if the releases were already removed.
helm uninstall primus-safe -n primus-safe 2>/dev/null || true

helm uninstall primus-safe-cr -n primus-safe 2>/dev/null || true

# SaFE-native observability stack (metrics + logs: VictoriaMetrics + exporters +
# OpenSearch + FluentBit). Installed by install.sh Step 6b into its own
# namespace, gated on the observability prompt, so it may not exist.
#
# IMPORTANT ordering: the real workloads (OpenSearch StatefulSet, FluentBit
# DaemonSet, VMCluster/VMAgent pods) are created by the OPERATORS from CRs, not
# by Helm. `helm uninstall` only removes the chart objects (operators + CRs). If
# the operators are torn down at the same time as their CRs, they can't finalize
# / garbage-collect their children, leaving orphaned StatefulSets/DaemonSets/pods
# and CRs stuck in Terminating. So delete the CRs FIRST (while the operators are
# still running to clean up), wait, THEN uninstall the release.
OBS_NS=primus-safe-observability
# Every VictoriaMetrics CR kind the operator manages. We delete these (and later
# clear finalizers on the same set) so a lingering VM CR can't wedge the
# namespace in Terminating. vmcluster/vmagent own real pods; the rest
# (vmalert/vmalertmanager/vmauth/vmsingle) and the scrape CRs
# (vmnodescrape/vmpodscrape/vmservicescrape) are lighter but still carry the
# apps.victoriametrics.com finalizer.
VM_CR_KINDS="vmcluster vmagent vmalert vmalertmanager vmauth vmsingle vmnodescrape vmpodscrape vmservicescrape"
if helm status primus-safe-observability -n "$OBS_NS" >/dev/null 2>&1; then
  echo "Deleting observability CRs so operators can clean up their workloads..."
  kubectl -n "$OBS_NS" delete opensearchcluster --all --ignore-not-found --timeout=180s 2>/dev/null || true
  kubectl -n "$OBS_NS" delete fluentbit --all --ignore-not-found --timeout=120s 2>/dev/null || true
  for kind in $VM_CR_KINDS; do
    kubectl -n "$OBS_NS" delete "$kind" --all --ignore-not-found --timeout=120s 2>/dev/null || true
  done
  # Cluster-scoped Fluent config CRs (not namespaced).
  kubectl delete clusterfluentbitconfig,clusterinput,clusterfilter,clusteroutput --all --ignore-not-found 2>/dev/null || true
  sleep 10
fi
helm uninstall primus-safe-observability -n "$OBS_NS" 2>/dev/null || true

# The VictoriaMetrics operator installs a validating/mutating admission webhook.
# If the operator (and its webhook) is torn down before the VM CR finalizers
# clear, deleting the namespace hangs forever in "Terminating" and a later
# reinstall into it fails. Remove the webhooks first, then force-clear finalizers
# on any residual VM CRs below. All best-effort.
kubectl delete validatingwebhookconfiguration,mutatingwebhookconfiguration \
  -l app.kubernetes.io/name=victoria-metrics-operator --ignore-not-found 2>/dev/null || true
kubectl get validatingwebhookconfigurations,mutatingwebhookconfigurations -o name 2>/dev/null \
  | grep -i "victoria-metrics-operator" | xargs -r kubectl delete --ignore-not-found 2>/dev/null || true

# Safety net: if the operators were already gone (CR deletes hung on finalizers),
# clear finalizers and remove any orphaned workloads left behind. Covers both the
# logging CRs (opensearchcluster/fluentbit) and the full VictoriaMetrics CR set.
for cr in $(kubectl -n "$OBS_NS" get opensearchcluster,fluentbit -o name 2>/dev/null); do
  kubectl -n "$OBS_NS" patch "$cr" --type=merge -p '{"metadata":{"finalizers":[]}}' 2>/dev/null || true
done
for kind in $VM_CR_KINDS; do
  kubectl -n "$OBS_NS" get "$kind" -o name 2>/dev/null | while read -r cr; do
    kubectl -n "$OBS_NS" patch "$cr" --type=merge -p '{"metadata":{"finalizers":[]}}' 2>/dev/null || true
  done
done
kubectl -n "$OBS_NS" delete statefulset,daemonset --all --ignore-not-found 2>/dev/null || true

# Grafana operator: delete its CRs FIRST (while the operator is still alive to run the
# operator.grafana.com finalizer), then uninstall the operator, then force-clear the finalizer on
# any residual CRs — mirroring the VictoriaMetrics/OpenSearch teardown above. Without this, once
# the operator pod is gone the GrafanaDashboard/GrafanaDatasource finalizers can never run and the
# primus-safe namespace wedges in Terminating (issue #679). The CRs live in primus-safe (created
# by the primus-safe chart's grafana templates).
kubectl -n primus-safe delete grafanadashboard,grafanadatasource,grafana \
  --all --ignore-not-found --timeout=120s 2>/dev/null || true

helm uninstall grafana-operator -n primus-safe 2>/dev/null || true

# Safety net: if the operator was already gone (the deletes above hung on the finalizer), clear
# the finalizer on any residual Grafana CRs so namespace deletion can complete.
for kind in grafanadashboard grafanadatasource grafana grafanafolder grafanaalertrulegroup \
  grafanacontactpoint grafananotificationtemplate grafanaserviceaccount; do
  kubectl -n primus-safe get "$kind" -o name 2>/dev/null | while read -r cr; do
    kubectl -n primus-safe patch "$cr" --type=merge -p '{"metadata":{"finalizers":[]}}' 2>/dev/null || true
  done
done

helm uninstall primus-pgo -n primus-safe 2>/dev/null || true

helm uninstall node-agent -n primus-safe 2>/dev/null || true

# The steps above remove the Helm releases only. StatefulSet PVCs (OpenSearch
# indices, VictoriaMetrics, Postgres) and the created namespaces are left in
# place by default (safe: protects data on an accidental uninstall).
#
# IMPORTANT: reinstalling WITHOUT clearing these PVCs makes OpenSearch reuse the
# old data volume. A single-node OpenSearch (our default replicas:1) that comes
# back on a volume carrying stale cluster-coordination state (a voting config
# pointing at a node UUID the new pod no longer has) can never re-elect a
# cluster-manager -> pod stuck 0/1 -> Service has no endpoints -> the Logs tab
# gets "connection refused". A single node has no quorum peer to self-heal from.
#
# Whether to also purge PVCs. This is intentionally NOT the default: uninstall
# also runs during reinstall/upgrade flows, and purging deletes the management
# Postgres DB (users, workspaces, cluster registrations) alongside the
# disposable logs/metrics indices. Auto-destroying data on every uninstall would
# be a footgun (Helm/K8s never auto-delete PVCs for this reason).
#
# Resolution: an explicit PURGE_PVC env var always wins; otherwise, when running
# interactively, prompt (default No) so the option is discoverable each time.
#   PURGE_PVC=true  ./uninstall.sh   # non-interactive purge
#   PURGE_PVC=false ./uninstall.sh   # non-interactive retain (skip prompt)
purge="${PURGE_PVC:-}"
if [[ -z "$purge" && -t 0 ]]; then
  read -r -p "Also delete PVCs? DESTROYS all logs/metrics indices AND the primus-safe management DB [y/N]: " ans
  case "$ans" in [yY]*) purge=true ;; *) purge=false ;; esac
fi
if [[ "$purge" == "true" ]]; then
  echo "Deleting StatefulSet PVCs for a clean slate..."
  kubectl -n primus-safe-observability delete pvc --all --wait=false 2>/dev/null || true
  kubectl -n primus-safe delete pvc --all --wait=false 2>/dev/null || true
  echo "PVCs deleted."
  # Clear FluentBit's tail-state on every node. It lives on a node hostPath
  # (must match fluentbit.storage.hostPath), NOT a PVC, so the PVC purge above
  # does not touch it. Without this, a clean-slate that wipes OpenSearch leaves
  # FluentBit's durable position DB at EOF for every existing log file, so
  # readFromHead re-ships nothing to the fresh empty cluster (missing logs). A
  # short-lived DaemonSet (in the persistent `default` ns, since the observability
  # ns is being deleted) rm's the dir on each node so BOTH sides reset together.
  FLB_STATE_HOSTPATH="/var/lib/fluent-bit-state"
  # Use an image ALREADY cached on every node so the wipe never depends on a
  # pull. FluentBit is a DaemonSet and its readiness-gate initContainer runs
  # opensearchproject/opensearch on every node, so that image is present
  # cluster-wide and has a shell. Do NOT prefix proxy_image_registry: the Harbor
  # /proxy pull-through returns 401 when unprovisioned (the exact trap install.sh
  # Step 6b avoids for the observability images). imagePullPolicy: IfNotPresent
  # (set on the container below) then uses the cached copy without a pull.
  #
  # This MUST match the OpenSearch image the logging stack actually runs, i.e.
  # fluentbit/values.yaml readinessGate.image (repository:tag). If that image is
  # bumped, update this literal too so the cleanup keeps hitting a node-cached
  # copy (a mismatched ref would force a pull and can reintroduce the failure).
  cleanup_img="opensearchproject/opensearch:2.11.0"
  echo "Clearing FluentBit tail-state ($FLB_STATE_HOSTPATH) on all nodes..."
  cat <<EOF | kubectl apply -f - 2>/dev/null || true
apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: flb-state-cleanup
  namespace: default
  labels:
    app: flb-state-cleanup
spec:
  selector:
    matchLabels:
      app: flb-state-cleanup
  template:
    metadata:
      labels:
        app: flb-state-cleanup
    spec:
      tolerations:
        - operator: Exists
      containers:
        - name: cleanup
          image: ${cleanup_img}
          imagePullPolicy: IfNotPresent
          # One-shot wipe, then drop a sentinel and idle. The pod only becomes
          # Ready (via the readinessProbe below) AFTER the rm completes, so a
          # healthy rollout is a real "wiped on this node" signal -- not just
          # "pod scheduled". The trailing sleep just keeps the pod Running long
          # enough to report Ready before we tear it down.
          command: ["sh", "-c", "rm -rf ${FLB_STATE_HOSTPATH}/* ${FLB_STATE_HOSTPATH}/.[!.]* 2>/dev/null; touch /tmp/done; echo cleared; sleep 3600"]
          readinessProbe:
            exec:
              command: ["sh", "-c", "test -f /tmp/done"]
            initialDelaySeconds: 1
            periodSeconds: 2
          securityContext:
            runAsUser: 0
          volumeMounts:
            - name: state
              mountPath: ${FLB_STATE_HOSTPATH}
      volumes:
        - name: state
          hostPath:
            path: ${FLB_STATE_HOSTPATH}
            type: DirectoryOrCreate
EOF
  # Wait for provable completion on every node BEFORE teardown. A DaemonSet runs
  # exactly one pod per node, and each pod is Ready only after its rm ran, so a
  # successful rollout means the state is gone cluster-wide. Do NOT swallow the
  # result with `|| true`: only report success if the wait actually succeeded,
  # otherwise fail loudly with manual remediation (uninstall.sh has no set -e,
  # so a non-zero rollout status here is handled by the if, not fatal).
  if kubectl -n default rollout status ds/flb-state-cleanup --timeout=180s; then
    echo "FluentBit tail-state cleared on all nodes."
  else
    wipe_failed=1
    echo "WARNING: FluentBit state wipe did NOT complete on all nodes"
    echo "         (likely an image-pull failure or a NotReady node)."
    echo "         Clear it manually on each affected node before reinstalling:"
    echo "           rm -rf $FLB_STATE_HOSTPATH"
  fi
  kubectl -n default delete ds flb-state-cleanup --wait=true --timeout=60s 2>/dev/null || true
  # Strip primus-safe custom finalizers before deleting the namespaces. Some
  # SaFE resources (e.g. secrets carrying `primus-safe/secret.finalizer`) can
  # only be finalized by the apiserver/resource-manager we just uninstalled;
  # left in place they deadlock the namespace in Terminating. Stripping must
  # happen AFTER the controllers are gone (a live controller re-adds them).
  echo "Clearing leftover primus-safe finalizers so namespaces can terminate..."
  for ns in primus-safe primus-safe-observability; do
    for res in $(kubectl -n "$ns" get secrets -o name 2>/dev/null); do
      kubectl -n "$ns" patch "$res" --type=merge -p '{"metadata":{"finalizers":null}}' 2>/dev/null || true
    done
  done
  # Full clean slate: also remove the namespaces (install.sh recreates them).
  # --wait=false so the script never hangs.
  echo "Deleting namespaces for a full clean slate..."
  kubectl delete ns primus-safe-observability primus-safe --wait=false 2>/dev/null || true
  echo "Namespaces marked for deletion. A subsequent install will recreate them."
  echo "If a namespace still hangs in 'Terminating', find the resource holding a"
  echo "finalizer and clear it, e.g.:"
  echo "  kubectl get ns <ns> -o jsonpath='{.status.conditions}'   # names the blocking resource"
  echo "  kubectl -n <ns> patch <kind>/<name> --type=merge -p '{\"metadata\":{\"finalizers\":[]}}'"
else
  echo "Note: StatefulSet PVCs and namespaces retained (empty namespaces are"
  echo "harmless; reinstall reuses them). For a full clean slate before REINSTALL,"
  echo "re-run with: PURGE_PVC=true ./uninstall.sh"
fi

# Fail loudly if the clean-slate FluentBit wipe did not complete. Without this
# the script would return 0 and a follow-up ./install.sh would silently land on a
# fresh OpenSearch while the stale tail DB survives, re-creating the exact
# durability mismatch this purge exists to prevent.
if [[ "${wipe_failed:-0}" == "1" ]]; then
  echo
  echo "ERROR: FluentBit tail-state wipe did NOT complete (see warning above)."
  echo "Do NOT reinstall until you clear $FLB_STATE_HOSTPATH on every node, or the"
  echo "fresh OpenSearch will inherit stale offsets and miss pre-existing logs."
  exit 1
fi
