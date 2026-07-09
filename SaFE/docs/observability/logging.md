# Logging (OpenSearch + FluentBit) — SaFE-native direct model

SaFE collects container/node logs with FluentBit, stores them in OpenSearch,
and the apiserver queries OpenSearch **directly** (no robust-analyzer proxy) to
serve the workload **Logs** tab. It is the logs counterpart of the SaFE-native
metrics stack and ships in the same [`primus-safe-observability`](../../charts/primus-safe-observability/)
chart, gated by `observability.logs.enable`.

## Architecture

```
FluentBit (DaemonSet)  --node-YYYY.MM.DD-->  OpenSearch (primus-safe-observability ns)
                                                     ^
SaFE Web "Logs" tab --> apiserver log.go --> observability.LogsRegistry (per-cluster) --> OpenSearch
```

- **Namespace:** OpenSearch + FluentBit + their operators install into
  `primus-safe-observability` (same release as metrics).
- **Query path:** the apiserver builds a per-cluster OpenSearch client from a
  discovered endpoint (Cluster CR annotation `primus-safe.amd.com/logs-endpoint`
  or the configured default) and queries OpenSearch directly. This mirrors the
  metrics `MetricsDiscovery` exactly, so multi-cluster works the same way.
- **Index prefix:** FluentBit writes `node-<date>` indices; the apiserver reads
  `observability.logs.index_prefix` (default `node-`, falls back to
  `opensearch.prefix`).

## Components (added to primus-safe-observability)

| Component | Subchart | Purpose |
|-----------|----------|---------|
| OpenSearch operator | `opensearchOperator` | OpenSearchCluster CRD + controller |
| Fluent operator | `fluentOperator` | FluentBit + pipeline CRDs + controller |
| OpenSearch cluster | `logs` (`primus-safe-logs`) | Log storage |
| FluentBit | `fluentbit` | Collection pipeline -> OpenSearch |

Operator CRDs (9 OpenSearch + 22 Fluent) ship in the chart `crds/` dir and are
installed before the CRs; the operator subcharts keep `installCRDs: false`.

## Enabling

Logs are enabled together with metrics by the SaFE-native observability toggle.

- **Bootstrap:** answer `y` to "install SaFE observability metrics stack" in
  [`bootstrap/install.sh`](../../bootstrap/install.sh). This sets both
  `observability.metrics.enable` and `observability.logs.enable` to true and
  installs the `primus-safe-observability` release (Step 6b), which now includes
  OpenSearch + FluentBit.
- **Credentials (chart-managed):** the chart creates a stable admin secret
  `primus-safe-logs-admin` (keys `username`/`password`, from
  `opensearch.security.adminUsername`/`adminPassword`) and a matching
  `primus-safe-logs-securityconfig`. The `OpenSearchCluster` references them as
  `adminCredentialsSecret` + `securityConfigSecret`, so the operator does not
  regenerate credentials / reapply the demo securityconfig every reconcile (the
  churn that caused a bootstrap rolling-restart). FluentBit authenticates with
  `primus-safe-logs-admin`, and Step 6b mirrors it into
  `primus-safe/primus-safe-opensearch-config` (the secret the apiserver mounts),
  then restarts the apiserver + resource-manager. TLS uses `InsecureSkipVerify`
  for the operator's self-signed HTTP cert.
- **Bootstrap-race hardening:** FluentBit is gated behind an initContainer that
  polls OpenSearch `/_cluster/health` with admin credentials and only starts once
  it returns 200 (security initialized + status >= yellow) -- not merely a TCP
  connect, which passes before security is up and causes 503 "Security not
  initialized" write bursts. It also writes to the ClusterIP service
  (`primus-safe-logs`, ready nodes only) and buffers to disk (filesystem storage
  + tail position DB + retry). No recovery `additionalConfig` is set: OpenSearch
  defaults are used and `gateway.expected_data_nodes`/`recover_after_data_nodes`
  are forbidden (they deadlock recovery on a small cluster; the chart `fail`s if
  set).

## Multi-cluster / remote data plane

The default `observability.logs.endpoint` targets the in-cluster OpenSearch
Service. For a remote data cluster, set a reachable endpoint on the Cluster CR:

```bash
kubectl annotate cluster <cluster> \
  primus-safe.amd.com/logs-endpoint=https://<opensearch-host>:9200 --overwrite
```

`LogsDiscovery` honors the annotation over the default and rebuilds the cached
client when the endpoint changes (no apiserver restart needed for endpoint
changes; credential changes still require a restart since creds are read at
client-build time).

## Verify

```bash
# OpenSearch + FluentBit + operators Running
kubectl -n primus-safe-observability get pods

# node-YYYY.MM.DD indices exist once FluentBit ships logs
PW=$(kubectl -n primus-safe-observability get secret primus-safe-logs-admin -o jsonpath='{.data.password}' | base64 -d)
kubectl -n primus-safe-observability exec sts/primus-safe-logs-nodes -- \
  curl -sk -u "admin:$PW" https://primus-safe-logs.primus-safe-observability.svc:9200/_cat/indices | grep node-

# apiserver credential secret is populated
kubectl -n primus-safe get secret primus-safe-opensearch-config -o jsonpath='{.data.username}' | base64 -d
```

Then open a workload's **Logs** tab in the SaFE UI — it should return log lines.

## Topology (HA), durability, and reinstalling

OpenSearch defaults to a **3-node** cluster (`logs.opensearch.nodePools[0].replicas: 3`,
roles master/data/ingest). Three master-eligible nodes give a voting quorum of 2,
so the cluster tolerates losing one node and self-heals. Do **not** set 2 (quorum
of 2 tolerates zero failures + split-brain risk); only drop to 1 for a throwaway
single-box dev cluster (a lone node has no quorum peer, so a volume with stale
coordination state wedges permanently at `0/1`).

### Durability model (two stores that must reset together)

Logs live in **two** independent stores, and they must be kept coherent:

- **OpenSearch data** — StatefulSet PVCs `data-primus-safe-logs-nodes-*`. Retained
  across `helm upgrade`/reinstall by Kubernetes design (helm never deletes a
  StatefulSet's volumeClaimTemplate PVCs). Only `PURGE_PVC=true ./uninstall.sh`
  deletes them.
- **FluentBit position DB** — a node **hostPath** (`/var/lib/fluent-bit-state`,
  `fluentbit.storage.hostPath`), *not* a PVC. It survives everything short of an
  explicit wipe, recording the byte offset already shipped per log file.

The failure mode is a **mismatch**: if you wipe OpenSearch but leave FluentBit's
DB, FluentBit's offsets point at EOF for every existing file, so `readFromHead`
re-ships nothing and the fresh, empty cluster shows gaps. So:

- **Routine redeploy = keep both.** Run `./upgrade.sh` (or re-run `./install.sh`).
  Both are `helm upgrade --install` in place, so the OpenSearchCluster CR /
  StatefulSet / PVCs are kept — OpenSearch data **persists**, matching FluentBit's
  durable DB. No gaps, and namespaces are never touched (so no stuck-`Terminating`
  risk). **Do not** uninstall+purge just to redeploy.
- **Clean slate = wipe both.** Run `PURGE_PVC=true ./uninstall.sh` then
  `./install.sh`. The purge now deletes the OpenSearch PVCs **and** clears the
  FluentBit hostPath on every node (via a short-lived privileged DaemonSet in the
  `default` namespace), so both stores reset in lockstep and `readFromHead`
  re-populates the fresh cluster from the top of every current log file. That
  cleanup pod runs a **node-cached image** (`opensearchproject/opensearch`, the
  same image FluentBit's readiness-gate initContainer already pulls onto every
  node) with `imagePullPolicy: IfNotPresent`, so it never pulls -- in particular
  it does **not** go through the Harbor `/proxy` registry (which 401s when
  unprovisioned). The image is a hardcoded literal in `uninstall.sh` that must
  track `fluentbit.readinessGate.image` -- bump both together if that image
  version changes. If the wipe cannot complete on some node, `uninstall.sh`
  **exits non-zero** and
  tells you to clear `/var/lib/fluent-bit-state` manually before reinstalling --
  so a broken (stale-offset) reinstall can't silently follow.

```bash
# Routine redeploy (persist logs — preferred):
./upgrade.sh                          # helm upgrade in place; OpenSearch PVCs + FluentBit DB kept

# Full clean slate (wipe everything, coherently):
PURGE_PVC=true ./uninstall.sh         # deletes observability + management PVCs AND FluentBit node state, then
./install.sh                          # reinstall bootstraps OpenSearch on clean volumes
```

`uninstall.sh` also prompts for the purge interactively (default No) so it is not
missed. PVC purge is opt-in because it also deletes the management Postgres DB. If
a node was unreachable during the purge, clear its state manually with
`rm -rf /var/lib/fluent-bit-state` before the next install.

### Post-clean-slate warmup window
Only after a **clean-slate** reinstall (fresh OpenSearch + cleared FluentBit tail
DB), `readFromHead` replays all pre-existing container logs into the empty cluster
— a burst of order ~1M docs. Expect a **~15-20 min warmup window** where the
cluster is catching up and short-lived-job logs may land late or, historically,
be dropped under write backpressure. The OpenSearch node-pool heap is sized (6g ->
~614MB indexing-pressure ceiling) to absorb this burst without HTTP 429s. A routine
`./upgrade.sh` does **not** trigger this (both stores are retained, so nothing is
replayed) — the warmup only applies to the clean-slate path.

## Switch back to primus-robust

The stack is a self-contained release, so:

```bash
helm uninstall primus-safe-observability -n primus-safe-observability
# then set observability.logs.enable=false (and metrics.enable=false) in primus-safe values
```

## Troubleshooting

| Symptom | Likely cause | Fix |
|---------|--------------|-----|
| Helm `no matches for kind OpenSearchCluster/FluentBit` | CRDs not installed first | CRDs ship in the chart `crds/` dir (installed on `helm install`). Confirm `kubectl get crd | grep -E 'opensearch|fluent'`. |
| `ImagePullBackOff` on opensearch/fluent-bit/busybox | `global.imageRegistry` set to `<root>/primussafe` or images not mirrored | `global.imageRegistry` must be the registry root; third-party images keep their upstream org path. |
| Logs tab: OpenSearch 401 | apiserver creds don't match OpenSearch admin | Re-run the Step 6b credential mirror; confirm `primus-safe-opensearch-config` matches the chart admin secret `primus-safe-logs-admin` (from `opensearch.security.adminPassword`), then restart the apiserver. |
| Logs tab: empty / `no such index` | index prefix mismatch | `observability.logs.index_prefix` (or `opensearch.prefix`) must be `node-` to match FluentBit `logstashPrefix: node`. |
| Logs tab: "not initialized" / no data | no endpoint discovered | Ensure `observability.logs.enable` is true and the Cluster is Ready; check the `logs-endpoint` annotation for remote clusters. |
| Logs tab: `connection refused` to `primus-safe-logs...:9200`; OpenSearch pod(s) stuck `0/1`; logs show `cluster-manager not discovered or elected yet ... not a quorum` | **Stale OpenSearch coordination state after a reinstall reusing retained PVCs.** Worst on a 1-node cluster (no quorum peer to recover); with the 3-node default it usually reforms, but all-stale/mismatched volumes can still wedge. | Purge and reinstall clean: `PURGE_PVC=true ./uninstall.sh && ./install.sh`. To recover in place, delete the logs PVCs `data-primus-safe-logs-nodes-{0,1,2}` (`kubectl -n primus-safe-observability delete pvc data-primus-safe-logs-nodes-0 data-primus-safe-logs-nodes-1 data-primus-safe-logs-nodes-2 --wait=false`) then delete the `primus-safe-logs-nodes-*` pods so they rebuild on fresh volumes. |
| fluent-operator `CrashLoopBackOff` `assignment to entry in nil map` | `--watch-namespaces` flag | Keep `fluentOperator.operator.extraArgs: []`. |
| Logs tab: HTTP 503 `cluster_block_exception` / `state not recovered / initialized`; OpenSearch pods look up but never serve | A recovery **gate** (`gateway.expected_data_nodes` or `gateway.recover_after_data_nodes`) is set to the node count. On a small cluster, node churn means fewer than N nodes are up at once, so the cluster-manager refuses to recover state and Security never initializes. | Remove the gate (leave it unset). It must not be in `opensearch.additionalConfig` (the chart now `fail`s on it); if it was hand-edited onto the live CR, `kubectl -n primus-safe-observability edit opensearchcluster primus-safe-logs` and delete the `gateway.*_data_nodes` line, then let the operator roll the nodes. |
| After `helm uninstall`, OpenSearch/FluentBit pods keep running and `data-*` PVCs stick in `Terminating` | The workloads are **operator-created** (StatefulSet/DaemonSet), not chart objects, so Helm can't delete them; removing the operators at the same time as their CRs orphans the children and leaves CRs stuck on finalizers. | Use `./uninstall.sh` (it deletes the CRs first, waits, then removes the operators). To clean up manually: clear finalizers on the `opensearchcluster`/`fluentbit` CRs, `kubectl delete statefulset,daemonset --all -n primus-safe-observability`, then `kubectl delete ns primus-safe-observability`. |
| `kubectl delete ns primus-safe` hangs in `Terminating`; ns condition says `Some content ... has finalizers remaining: primus-safe/secret.finalizer` | A SaFE resource (e.g. secret `node-managing-ssh-private-key`) carries a **custom `primus-safe/*` finalizer** that only the apiserver/resource-manager can remove — but those were already uninstalled, so nothing clears it. | `PURGE_PVC=true ./uninstall.sh` now strips these finalizers before deleting namespaces. To unblock manually: `kubectl -n primus-safe patch secret node-managing-ssh-private-key --type=merge -p '{"metadata":{"finalizers":[]}}'`. |
| Intermittent log loss right after a clean-slate reinstall: some jobs (esp. short-lived `sleep`/one-shot pods) show nothing, longer/repeat runs work, chatty pods always land | OpenSearch **write backpressure**: the `readFromHead` backfill flood exceeds the indexing-pressure ceiling (~10% of heap) and OpenSearch returns HTTP `429 rejected_execution_exception`. FluentBit retries (so streams eventually land), but a short job's single small chunk can be dropped when its retry coincides with the pressure window. Check FluentBit output for `"status":429` and `_nodes/stats/indexing_pressure` -> `primary_rejections` climbing. | Raise the OpenSearch node-pool heap so the ceiling clears the burst: `logs.opensearch.nodePools[0].jvm: "-Xms6g -Xmx6g"` with container memory `12Gi` (heap ~= 50% of limit) gives a ~614MB ceiling (vs ~204MB at 2g). Sized for large nodes; scale jvm + memory down together for smaller ones. Only bites during the clean-slate warmup window (above), not on `./upgrade.sh`. |
| OpenSearch pods stuck `Init:ImagePullBackOff`; kubelet events show `unexpected media type text/html` / HTTP 429 pulling the busybox init image; the log index's primary shard goes offline and Fluent Bit logs `node_not_connected_exception` | The operator's initHelper image was `busybox:latest` (⇒ `imagePullPolicy: Always`), pulled **anonymously** from Docker Hub on **every** pod start. Cumulative redeploys (all nodes share one NAT IP) exhausted Docker Hub's anonymous per-IP limit; it then serves a 429 HTML page that containerd rejects. | The chart now pins `logs.opensearch.initHelperImage` to `docker.io/library/busybox:1.36.1` (a version tag ⇒ `IfNotPresent`, so it is cached per node instead of re-pulled). For a mirrored/air-gapped registry or a Harbor Docker Hub proxy-cache, override it, e.g. `--set logs.opensearch.initHelperImage=harbor.<cluster>.primus-safe.amd.com/sync/library/busybox:1.36.1`. |
| `PURGE_PVC=true ./uninstall.sh` prints `0 of N updated pods are available` then `WARNING: FluentBit state wipe did NOT complete` and exits non-zero | The `flb-state-cleanup` pods can't pull their image, so the `rm` never runs. Historically this happened when the image was prefixed with the Harbor `/proxy` pull-through registry, which returns 401 when the project isn't provisioned. | The cleanup now uses a node-cached image (`opensearchproject/opensearch`, `imagePullPolicy: IfNotPresent`, no `/proxy` prefix), so it shouldn't recur. If it does (e.g. image evicted from a node, or a NotReady node), clear state manually on each node: `rm -rf /var/lib/fluent-bit-state` — then `./install.sh`. |
| After a clean-slate reinstall, an **idle** pod's logs never appear in the Logs tab even though ingestion is live and other namespaces populate | FluentBit uses `logstashFormat`, so each re-read line is indexed to `node-<the line's own date>`. When the FluentBit state was cleared, `readFromHead` re-ships an idle pod's old lines, but they land in an **old-dated index** (e.g. `node-2026.07.08`) that the UI's default recent-time window doesn't query. This is expected — actively-logging pods write current-dated docs and appear normally. | Widen the UI time range to cover the pod's last-active day, or query the old index directly: `curl -s -k -u "$user:$pass" "https://<os>:9200/node-2026.07.08/_search?q=kubernetes.pod_name:<pod>"`. To confirm the clean-slate wipe worked at all, check the oldest doc predates FluentBit start: `curl ... "https://<os>:9200/node-*/_search?size=1&sort=@timestamp:asc"`. |

## Notes / out of scope

- Metrics validation (VMServiceScrape conversion, exporter image tags, the
  Grafana dashboard `{{gpu_id}}` template) is tracked separately with the metrics
  stack.
- No robust-api, no Postgres, and no AddonTemplate are involved in the logs path.
