# Logging stack (OpenSearch + FluentBit) operator runbook

This runbook covers the **logging-only** subset of the Primus-Robust observability
stack that now ships inside the Primus-SaFE repo. It collects container/node logs
with FluentBit, stores them in OpenSearch, and serves them to the SaFE **Logs**
tab through the `robust-api` HTTP proxy.

Metrics (VictoriaMetrics / Grafana / exporters) are intentionally **not** part of
this release. The full `deploy/charts` tree is mirrored under
[`SaFE/charts/primus-robust/`](../../charts/primus-robust/) for a later metrics
release, but only the logging umbrella is built and installed now.

## Architecture

```
FluentBit (DaemonSet)  --node-YYYY.MM.DD-->  OpenSearch (primus-robust-logs:9200)
                                                     ^
SaFE Web "Logs" tab --> apiserver --> robustclient --> robust-api:8085 /api/v1/logs/raw
                                                     |
                                        (startup ping only) --> minimal Postgres
```

- **Namespace:** everything installs into `primus-robust` on the data cluster.
- **Query path:** SaFE never dials OpenSearch directly; it proxies through
  `robust-api` (`/api/v1/logs/raw`). This keeps the management/data-plane split
  and makes multi-cluster reachability work via a per-cluster endpoint annotation.
- **Index prefix:** FluentBit writes `node-<date>` indices; SaFE's
  `opensearch.prefix` is `node-` (must stay aligned).

## Components

| Component | Chart | Purpose |
|-----------|-------|---------|
| OpenSearch operator | `operators/opensearch-operator` | Reconciles the `OpenSearchCluster` CR |
| Fluent operator | `operators/fluent-operator-core` | Reconciles FluentBit + pipeline CRs |
| OpenSearch cluster | `opensearch-cluster` (`primus-robust-logs`) | Log storage |
| FluentBit | `fluentbit` | Log collection -> OpenSearch |
| robust-api | `api` (Service `robust-api:8085`) | Log proxy serving `/api/v1/logs/raw` |
| Postgres (minimal) | umbrella `templates/postgres.yaml` | Satisfies the robust-api startup DB ping only |

The umbrella that bundles these is
[`SaFE/charts/primus-robust/primus-robust-logging/`](../../charts/primus-robust/primus-robust-logging/).

## Prerequisites

1. **Mirror the Helm chart** to your OCI registry:

   ```bash
   # from SaFE/charts/primus-robust
   ./publish-logging.sh                      # -> oci://registry-1.docker.io/primussafe
   ./publish-logging.sh harbor.example.com   # -> oci://harbor.example.com/primussafe
   ```

   (Run `helm registry login <registry>` first.)

2. **Mirror the container images** to the `<image_registry>/primussafe` that SaFE
   is configured to use (`global.proxy_image_registry`). The full list is in the
   AddonTemplate header
   ([`primus-robust-logging.0.1.0.yaml`](../../charts/primus-safe-cr/templates/addon_template/primus-robust-logging.0.1.0.yaml)):

   - First-party: `primussafe/api:latest`
   - Third-party (kept under their upstream org path): `opensearchproject/opensearch:2.11.0`,
     `opensearchproject/opensearch-operator:2.6.0`, `kubebuilder/kube-rbac-proxy:v0.15.0`,
     `busybox:latest`, `kubesphere/fluent-bit:v3.1.5`, `kubesphere/fluent-operator:v3.1.0`,
     `library/docker:20.10`, `postgres:16-alpine`

   > Image registry rule (A2/A3): `global.imageRegistry` is the registry **root**.
   > First-party repos carry `primussafe/`; third-party repos keep their upstream
   > org. Never set `global.imageRegistry` to `<root>/primussafe` — that mangles
   > third-party refs.

## Install

The AddonTemplate ships in the `primus-safe-cr` chart, so it is applied
automatically when SaFE bootstrap runs `helm upgrade --install primus-safe-cr`.
No manual per-addon script is needed.

- **Default (auto-install on every cluster):** the template carries the
  `primus-safe.addon.default: ""` label, so `ClusterReconciler.guaranteeDefaultAddon`
  synthesizes an `Addon` per registered Cluster and the `AddonController` helm-installs
  the chart into `primus-robust`.

- **Opt-in per cluster (if you remove the default label):** create an `Addon` CR
  manually:

  ```yaml
  apiVersion: amd.com/v1
  kind: Addon
  metadata:
    name: <cluster>-primus-robust-logging
  spec:
    cluster:
      kind: Cluster
      name: <cluster>
    addonSource:
      helmRepository:
        releaseName: primus-robust-logging
        namespace: primus-robust
        template:
          kind: AddonTemplate
          name: primus-robust-logging.0.1.0
  ```

On successful deploy (`AddonDeployed`) the release name `primus-robust-logging`
triggers `registerRobustEndpointIfApplicable`, which annotates the Cluster CR with
`primus-safe.amd.com/robust-api-endpoint = http://robust-api.primus-robust.svc:8085`.

## Multi-cluster / remote data plane

The in-cluster DNS annotation only resolves when SaFE and the data plane are in the
**same** cluster. For a remote data cluster, override the annotation with a
reachable address (the `robust-api` Service defaults to NodePort `32626`):

```bash
kubectl annotate cluster <cluster> \
  primus-safe.amd.com/robust-api-endpoint=http://<data-node-ip>:32626 --overwrite
```

`robustclient` discovery honors the annotation over the in-cluster default, and the
apiserver's cached OpenSearch client is rebuilt automatically when the endpoint
changes (no restart needed).

## Credentials

The OpenSearch operator generates the admin secret
`primus-robust-logs-admin-password` in `primus-robust`. Both FluentBit and
robust-api authenticate to OpenSearch using it — no manual credential entry is
required for the proxy query path. SaFE's `primus-safe-opensearch-config` secret is
only echoed in the `/envs` response and is not used to authenticate in proxy mode.

## Verify

```bash
# 1. Addon reached AddonDeployed
kubectl get addon <cluster>-primus-robust-logging -o jsonpath='{.status.phase}{"\n"}'

# 2. Operators + workloads Running
kubectl -n primus-robust get pods

# 3. OpenSearch indices exist (from inside the cluster)
#    Expect node-YYYY.MM.DD indices once FluentBit ships logs.
kubectl -n primus-robust exec deploy/robust-api -- \
  sh -c 'curl -sk -u "$OPENSEARCH_USER:$OPENSEARCH_PASSWORD" https://primus-robust-logs.primus-robust.svc:9200/_cat/indices'

# 4. robust-api log proxy returns data
kubectl -n primus-robust exec deploy/robust-api -- \
  curl -s -XPOST localhost:8085/api/v1/logs/raw \
  -H 'Content-Type: application/json' \
  -d '{"uri":"/node-*/_search","method":"POST","body":{"size":1}}'
```

Then open a workload's **Logs** tab in the SaFE UI — it should return log lines.

## Troubleshooting

| Symptom | Likely cause | Fix |
|---------|--------------|-----|
| Addon `phase: error`, Helm `no matches for kind OpenSearchCluster/FluentBit` | CRDs not installed first | The umbrella ships CRDs in `crds/`; confirm they were applied (`kubectl get crd | grep -E 'opensearch|fluent'`). CRDs install on `helm install`, not on upgrade. |
| `ImagePullBackOff` on opensearch/fluent-bit/busybox | `global.imageRegistry` set to `<root>/primussafe` (A2) or images not mirrored | Ensure `imageRegistry` is the registry root and third-party images are mirrored under it. |
| `ImagePullBackOff` on robust-api | `primussafe/api` tag not published | Confirm `docker.io/primussafe/api:latest` (or the mirrored copy) exists; pin `api.image.tag` if needed. |
| robust-api `CrashLoopBackOff`, `Failed to connect to database` | minimal Postgres not ready | Check `robust-logging-db` pod; robust-api pings the DB at startup. |
| Logs tab: OpenSearch 401 | robust-api not using the admin secret (B10) | Confirm `api.modules.robustApi.opensearchExistingSecret=primus-robust-logs-admin-password`. |
| Logs tab: empty / `no such index [fluentbit-...]` | index prefix mismatch | SaFE `opensearch.prefix` must be `node-` (matches FluentBit `logstashPrefix: node`). |
| Logs tab: connection refused after endpoint change | (fixed) stale client cache (B2) | The cached client now rebuilds on endpoint change; if seen, confirm you are on the patched apiserver. |
| fluent-operator `CrashLoopBackOff`, `assignment to entry in nil map` | `--watch-namespaces` flag (A5) | Keep `fluent-operator-core operator.extraArgs: []`. |

## What is intentionally deferred

- Metrics (VictoriaMetrics, exporters, Grafana per-workload dashboards).
- Migrations / the full `robust-db` (PGO). The bundled Postgres is startup-ping-only
  and stores no logging data; robust-api's non-log modules will log harmless
  "table does not exist" warnings.
