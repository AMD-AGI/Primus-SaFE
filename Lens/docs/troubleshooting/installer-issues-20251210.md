# Primus Lens Installer Troubleshooting - 2025-12-10

## Issues Encountered and Fixes

### Issue 1: Go not in PATH
**Error:**
```
/bin/sh: 1: go: not found
```

**Solution:**
Add Go to PATH:
```bash
export PATH=$PATH:/usr/local/go/bin
```

---

### Issue 2: Helm `--create-namespace` not working
**Error:**
```
Error: no Namespace with the name "primus-lens" found
```

Despite using `--create-namespace` flag, Helm fails to create the namespace.

**Solution:**
Modified `pkg/stage/helm.go` to pre-create namespace with proper Helm labels before running `helm upgrade --install`:

```go
func (s *HelmStage) ensureNamespaceWithHelmLabels(ctx context.Context, namespace string, opts types.RunOptions) error {
    // Check if namespace exists, if not create with Helm labels
    manifest := fmt.Sprintf(`apiVersion: v1
kind: Namespace
metadata:
  name: %s
  labels:
    app.kubernetes.io/managed-by: Helm
  annotations:
    meta.helm.sh/release-name: %s
    meta.helm.sh/release-namespace: %s
`, namespace, s.releaseName, namespace)
    // ... apply manifest
}
```

---

### Issue 3: Namespace stuck in Terminating state
**Error:**
```
namespace "primus-lens" is Terminating
Some content in the namespace has finalizers remaining
```

**Cause:**
CRD resources (FluentBit, VMCluster, PostgresCluster) have finalizers that prevent namespace deletion when operators are already uninstalled.

**Solution:**
Manually remove finalizers from stuck resources:
```bash
kubectl patch fluentbit fluent-bit -n primus-lens -p '{"metadata":{"finalizers":null}}' --type=merge
kubectl patch vmcluster primus-lens-vmcluster -n primus-lens -p '{"metadata":{"finalizers":null}}' --type=merge
kubectl patch postgrescluster primus-lens -n primus-lens -p '{"metadata":{"finalizers":null}}' --type=merge
```

---

### Issue 4: Wrong PostgreSQL Secret Name
**Error:**
```
Error: secret "primus-lens-pguser-postgres" not found
```

**Cause:**
Init job was looking for wrong secret. PGO creates secret named `primus-lens-pguser-primus-lens`.

**Solution:**
Modified `charts/primus-lens-init/templates/postgres-init-job.yaml`:
```yaml
secretKeyRef:
  name: primus-lens-pguser-primus-lens  # Changed from primus-lens-pguser-postgres
  key: password
```

---

### Issue 5: PostgreSQL Connection Failed
**Error:**
```
psql: error: connection to server failed: FATAL: password authentication failed for user "postgres"
```

**Cause:**
Multiple issues:
1. Wrong host: used `primus-lens-ha.primus-lens.svc.cluster.local` instead of `primus-lens-primary.primus-lens.svc`
2. Wrong user: used `postgres` instead of `primus-lens`
3. Wrong database: used `postgres` instead of `primus_lens`
4. Missing SSL mode: PGO requires `sslmode=require`

**Solution:**
Modified init job to read all connection info from PGO-generated secret:
```yaml
env:
- name: PGHOST
  valueFrom:
    secretKeyRef:
      name: primus-lens-pguser-primus-lens
      key: host
- name: PGPORT
  valueFrom:
    secretKeyRef:
      name: primus-lens-pguser-primus-lens
      key: port
- name: PGUSER
  valueFrom:
    secretKeyRef:
      name: primus-lens-pguser-primus-lens
      key: user
- name: PGPASSWORD
  valueFrom:
    secretKeyRef:
      name: primus-lens-pguser-primus-lens
      key: password
- name: PGDATABASE
  valueFrom:
    secretKeyRef:
      name: primus-lens-pguser-primus-lens
      key: dbname
- name: PGSSLMODE
  value: "require"
```

---

### Issue 6: Wrong Operator Label Selectors
**Problem:**
Wait stage used incorrect label selectors for operators.

**Actual Labels:**
| Operator | Labels |
|----------|--------|
| VM Operator | `app.kubernetes.io/instance=plo,app.kubernetes.io/name=vm-operator` |
| PGO | `postgres-operator.crunchydata.com/control-plane=pgo` |
| OpenSearch | `control-plane=controller-manager` |
| Grafana | `app.kubernetes.io/name=grafana-operator,app.kubernetes.io/part-of=grafana-operator` |
| Fluent Operator | `app.kubernetes.io/component=operator,app.kubernetes.io/name=fluent-operator` |
| Kube State Metrics | `app.kubernetes.io/name=kube-state-metrics,app.kubernetes.io/part-of=kube-state-metrics` |

**Solution:**
Updated `pkg/workflow/dataplane.go` `buildOperatorConditions()` with correct labels.

---

### Issue 7: Wrong Application Label Selectors
**Problem:**
Wait stage for applications used wrong labels:
- Used: `app.kubernetes.io/component=web`
- Actual: `app=primus-lens-apps-web`

**Solution:**
Updated `pkg/workflow/dataplane.go` `buildAppConditions()`:
```go
func (w *DataplaneWorkflow) buildAppConditions() []stage.WaitCondition {
    return []stage.WaitCondition{
        {
            Kind:          "Deployment",
            LabelSelector: "app=primus-lens-apps-web",
            Condition:     "Available",
            Timeout:       5 * time.Minute,
        },
        {
            Kind:          "Deployment",
            LabelSelector: "app=primus-lens-apps-telemetry-processor",
            Condition:     "Available",
            Timeout:       5 * time.Minute,
        },
    }
}
```

---

### Issue 8: Wait Stage Timeout on Failed Pods
**Problem:**
When pods are in CrashLoopBackOff/Error state, wait stage keeps waiting until timeout instead of failing fast.

**Solution:**
Added `checkForFailedPods()` function to `pkg/stage/wait.go` that checks for pod failure states before waiting:
```go
func (s *WaitStage) checkForFailedPods(ctx context.Context, opts types.RunOptions, cond WaitCondition, namespace string) error {
    failedStates := []string{"CrashLoopBackOff", "ImagePullBackOff", "ErrImagePull", "CreateContainerConfigError", "InvalidImageName"}
    // Check pod statuses and return error if any pod is in failed state
}
```

---

## Files Modified

1. `pkg/stage/helm.go` - Added namespace pre-creation with Helm labels
2. `pkg/stage/wait.go` - Added failed pod detection
3. `pkg/workflow/dataplane.go` - Fixed operator and app label selectors
4. `charts/primus-lens-init/templates/postgres-init-job.yaml` - Fixed PostgreSQL connection configuration

---

## Post-Installation Issues (Runtime Issues)

These issues occur after installation completes but affect application functionality.

### Issue 9: Web Pod CrashLoopBackOff - Missing Upstream Service
**Error:**
```
nginx: [emerg] host not found in upstream "lens-conductor-service.primus-lens.svc.cluster.local" in /app/nginx.conf:29
```

**Cause:**
Web pod nginx configuration references `lens-conductor-service` which is a control plane component not deployed in dataplane-only installation.

**Status:** Expected behavior for dataplane-only deployment. Web frontend requires control plane to be fully functional.

---

### Issue 10: Multiple Apps RBAC Permission Denied for Secrets
**Error:**
```
Failed to bootstrap telemetry processor: error secrets "primus-lens-storage-config" is forbidden: 
User "system:serviceaccount:primus-lens:primus-lens-app" cannot get resource "secrets" in API group "" in the namespace "primus-lens"
```

**Affected Apps:**
- `primus-lens-apps-telemetry-processor`
- `primus-lens-apps-ai-advisor`
- `primus-lens-apps-jobs`
- `primus-lens-apps-gpu-resource-exporter`

**Apps NOT Affected (don't need secrets):**
- `primus-lens-apps-node-exporter` (runs fine)
- `primus-lens-apps-system-tuner` (runs fine)

**Cause:**
ClusterRole `primus-lens-app` was missing `secrets` permission. The apps need to read `primus-lens-storage-config` secret for storage configuration.

**Solution:**
Modified `charts/primus-lens-operators/templates/rbac.yaml` to add secrets permission:
```yaml
# Read secrets (required for storage-config, database credentials, etc.)
- apiGroups: [""]
  resources: ["secrets"]
  verbs: ["get", "list", "watch"]
```

**Apply fix:**
```bash
# Upgrade operators chart to apply RBAC fix
helm upgrade plo charts/primus-lens-operators -n primus-lens

# Restart affected pods
kubectl delete pods -n primus-lens -l app.kubernetes.io/instance=primus-lens-apps
```

---

### Issue 11: Missing storage-config Secret
**Error:**
```
error secrets "primus-lens-storage-config" not found
```

**Cause:**
The `primus-lens-storage-config` secret was created by the old `primus-lens-dataplane` chart but was missing in the new separated `primus-lens-init` chart.

**Solution:**
Added `storage-config-job.yaml` to `charts/primus-lens-init/templates/` that creates the storage-config secret by reading PostgreSQL credentials from PGO-managed secret.

Also updated `primus-lens-installer` ClusterRole to allow secrets read/write permissions.

---

### Issue 12: PostgreSQL Host Format Mismatch  
**Error:**
```
hostname resolving error: lookup primus-lens-primary.primus-lens.svc.primus-lens.svc.cluster.local
```

**Cause:**
PGO generates host as `primus-lens-primary.primus-lens.svc`, but the application code adds `{namespace}.svc.cluster.local`, resulting in double suffix.

**Solution:**
Modified storage-config-job.yaml to extract only the service name from PGO secret:
```bash
# PGO generates: primus-lens-primary.primus-lens.svc
# App expects: primus-lens-primary
PG_HOST=$(echo "$PG_HOST_FULL" | cut -d'.' -f1)
```

---

### Issue 13: Application Code Nil Pointer Dereference (OpenSearch disabled)
**Error:**
```
Successfully initialized DB for cluster 'default', DB pointer: 0xc0010c4780
panic: runtime error: invalid memory address or nil pointer dereference
  /app/Lens/modules/core/pkg/clientsets/storage.go:179
```

**Cause:**
When OpenSearch is disabled, `cfg.Opensearch` is nil. The code at line 179 tries to access `cfg.Opensearch.Scheme` without checking for nil, causing panic.

**Solution:**
Modified `modules/core/pkg/clientsets/storage.go` to add nil check before initializing OpenSearch client:
```go
// Init Opensearch client (optional - may be disabled)
if cfg.Opensearch != nil && cfg.Opensearch.Service != "" {
    // Initialize OpenSearch client
    opensearchClient, err := opensearch.NewClient(...)
    ...
} else {
    log.Infof("OpenSearch is not configured for cluster '%s', skipping OpenSearch client initialization", clusterName)
}
```

---

### Issue 14: Web Console in Wrong Chart (dataplane vs controlplane)
**Problem:**
Web console was incorrectly placed in `primus-lens-apps-dataplane` chart, but it requires control-plane services (`lens-conductor-service`).

**Solution:**
- Removed `app-web.yaml` from `charts/primus-lens-apps-dataplane/templates/`
- Updated `values.yaml` to remove web configuration
- Updated `NOTES.txt` to indicate web is in control-plane
- Updated `dataplane.go` wait conditions to remove web

Web console should be deployed as part of control-plane installation.

---

### Issue 11: OpenSearch Node Pod Pending
**Error:**
```
primus-lens-logs-nodes-0    0/1     Pending    0    <time>
```

**Cause:**
OpenSearch is disabled in values.yaml (`opensearch.enabled: false`), but the bootstrap pod still created the OpenSearch cluster CR. The nodes pod is pending likely due to:
1. StorageClass not available
2. Insufficient node resources
3. Node selector/affinity not satisfied

**Note:** Since OpenSearch is disabled, this can be ignored or the OpenSearch CR can be manually deleted.

---

### Issue 12: FluentBit Pods OOMKilled/Evicted
**Error:**
```
fluent-bit-xxx    0/1     OOMKilled
fluent-bit-xxx    0/1     Evicted
```

**Cause:**
FluentBit pods on certain nodes are being killed due to memory limits or node resource pressure.

**Solution:**
Increase FluentBit memory limits in values.yaml or node-specific configuration, or check node resource availability.

---

## Installation Success Summary

After fixing issues 1-8, the installer completed successfully:

```
✅ [1/7] Stage install-operators completed
✅ [2/7] Stage wait-operators completed
✅ [3/7] Stage install-infrastructure completed
✅ [4/7] Stage wait-infrastructure completed
✅ [5/7] Stage run-init-jobs completed (PostgreSQL init successful!)
✅ [6/7] Stage install-applications completed
✅ [7/7] Stage wait-applications completed

✅ dataplane installation completed successfully!
```

All Helm releases deployed:
```
NAME                        STATUS    CHART
plo                         deployed  primus-lens-operators-1.0.0
primus-lens-infrastructure  deployed  primus-lens-infrastructure-1.0.0
primus-lens-init            deployed  primus-lens-init-1.0.0
primus-lens-apps            deployed  primus-lens-apps-dataplane-1.0.0
```

---

## Cleanup Commands

If installation fails and namespace is stuck, run:
```bash
# List stuck resources
kubectl api-resources --verbs=list --namespaced -o name | xargs -I {} sh -c 'kubectl get {} -n primus-lens -o name 2>/dev/null'

# Remove finalizers from common stuck resources
kubectl patch fluentbit fluent-bit -n primus-lens -p '{"metadata":{"finalizers":null}}' --type=merge
kubectl patch vmcluster primus-lens-vmcluster -n primus-lens -p '{"metadata":{"finalizers":null}}' --type=merge
kubectl patch postgrescluster primus-lens -n primus-lens -p '{"metadata":{"finalizers":null}}' --type=merge
```

---

## Issue 14: OpenSearch PVC Uses Wrong StorageClass

### Symptom
OpenSearch PVC `data-primus-lens-logs-nodes-0` uses `storage-rbd` instead of configured `local-path`.

### Root Cause
Two problems:

1. **Installer not passing values file to Helm**: `install.go` loaded config but didn't pass the file path to workflow, so Helm used default values.

2. **Wrong field name in OpenSearch chart**: Template used `storageClassName` but OpenSearch CRD requires `storageClass`.

### Solution

**1. Modified `internal/cmd/install.go`** - Add `ValuesFileSetter` interface:
```go
type ValuesFileSetter interface {
    SetValuesFile(file string)
}

// In runInstall function, after creating workflow:
if cfgFile != "" {
    if setter, ok := wf.(ValuesFileSetter); ok {
        setter.SetValuesFile(cfgFile)
    }
}
```

**2. Modified workflow files** - `SetValuesFile()` re-initializes stages:
```go
func (w *DataplaneWorkflow) SetValuesFile(file string) {
    w.valuesFile = file
    w.stages = make([]Stage, 0)
    w.setupStages()
}
```

**3. Modified `charts/primus-lens-infrastructure/templates/opensearch-cluster.yaml`**:
```yaml
# Before (incorrect)
persistence:
  pvc:
    storageClassName: {{ include "primus-lens-infra.storageClass" $ }}

# After (correct)
persistence:
  pvc:
    storageClass: {{ include "primus-lens-infra.storageClass" $ }}
```

### Verification
```bash
kubectl get pvc -n primus-lens
# All PVCs should now show STORAGECLASS as "local-path"
```

---

## Quick Re-test Commands

```bash
# Uninstall everything
./build/primus-lens-installer uninstall dataplane --config values.yaml --force --delete-data --verbose

# Wait for namespace cleanup (if stuck)
kubectl api-resources --verbs=list --namespaced -o name | xargs -I {} sh -c 'kubectl get {} -n primus-lens -o name 2>/dev/null' | \
  xargs -I {} kubectl patch {} -n primus-lens -p '{"metadata":{"finalizers":null}}' --type=merge

# Re-install
./build/primus-lens-installer install dataplane --config values.yaml --verbose
```

