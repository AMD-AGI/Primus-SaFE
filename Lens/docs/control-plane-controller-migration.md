# Control Plane Controller Migration Guide

## Overview

This document describes how to migrate from `primus-lens-control-plane-jobs` to the new `control-plane-controller` component.

### What Changed

| Before | After |
|--------|-------|
| `primus-lens-jobs` (single module) | `jobs` (data plane only) + `control-plane-controller` (management plane) |
| Mixed responsibilities | Clear separation of concerns |

### Jobs Migration

| Job | Old Location | New Location |
|-----|--------------|--------------|
| DataplaneInstallerJob | primus-lens-jobs | control-plane-controller |
| MultiClusterConfigSyncJob | primus-lens-jobs | control-plane-controller |
| TraceLensCleanupJob | primus-lens-jobs | control-plane-controller |
| GpuUsageWeeklyReportJob | primus-lens-jobs | control-plane-controller |
| GpuUsageWeeklyReportBackfillJob | primus-lens-jobs | control-plane-controller |

---

## Prerequisites

- kubectl access to the management cluster
- Access to Control Plane PostgreSQL database
- Image: `docker.io/primussafe/control-plane-controller:202601291743` (or newer)

---

## Step 1: Apply Database Migration

Add `k8s_manual_mode` and `storage_manual_mode` columns to `cluster_config` table.

### 1.1 Get Control Plane DB Credentials

```bash
# Get the secret name from your primus-lens-api config
kubectl -n primus-lens get configmap primus-lens-api-config -o yaml | grep secretName

# Extract credentials
kubectl -n primus-lens get secret <SECRET_NAME> -o jsonpath='{.data.uri}' | base64 -d
```

### 1.2 Apply Migration SQL

```bash
# Replace <DB_URI> with the actual connection string
kubectl -n primus-lens run psql-migration --rm -i --restart=Never \
  --image=postgres:15-alpine -- psql "<DB_URI>" -c "
ALTER TABLE cluster_config ADD COLUMN IF NOT EXISTS k8s_manual_mode BOOLEAN DEFAULT FALSE;
ALTER TABLE cluster_config ADD COLUMN IF NOT EXISTS storage_manual_mode BOOLEAN DEFAULT FALSE;
"
```

Expected output:
```
ALTER TABLE
ALTER TABLE
```

### 1.3 Verify Migration

```bash
kubectl -n primus-lens run psql-verify --rm -i --restart=Never \
  --image=postgres:15-alpine -- psql "<DB_URI>" -c "
SELECT column_name, data_type FROM information_schema.columns 
WHERE table_name = 'cluster_config' AND column_name LIKE '%manual_mode%';
"
```

Expected output:
```
     column_name     | data_type 
---------------------+-----------
 k8s_manual_mode     | boolean
 storage_manual_mode | boolean
(2 rows)
```

---

## Step 2: Deploy Control Plane Controller

### 2.1 Create ConfigMap

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: control-plane-controller-config
  namespace: primus-lens
data:
  config.yaml: |
    httpPort: 8080
    multiCluster: true
    loadK8SClient: true
    loadStorageClient: true
    isControlPlane: true
    controlPlane:
      secretName: "<CP_DB_SECRET_NAME>"      # e.g., primus-lens-control-plane-pguser-primus-lens-control-plane
      secretNamespace: "primus-lens"
    metricsRead:
      endpoints: http://vmselect-primus-lens-metrics.primus-lens.svc.cluster.local:8481/select/0/prometheus
    metricsWrite:
      endpoints: http://vminsert-primus-lens-metrics.primus-lens.svc.cluster.local:8480/insert/0/prometheus
    jobs:
      weeklyReport:
        enabled: true
        cron: "0 9 * * 1"
        clusters: []
        outputFormats:
          - html
          - pdf
        brand:
          primaryColor: "#ED1C24"
          companyName: "AMD AGI"
      weeklyReportBackfill:
        enabled: true
        cron: "0 10 * * 1"
        maxWeeksToBackfill: 12
```

### 2.2 Create Deployment

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: control-plane-controller
  namespace: primus-lens
  labels:
    app: control-plane-controller
    component: control-plane
spec:
  replicas: 1
  selector:
    matchLabels:
      app: control-plane-controller
  template:
    metadata:
      labels:
        app: control-plane-controller
        component: control-plane
    spec:
      serviceAccountName: primus-lens-app    # Use existing SA with cluster access
      containers:
      - name: control-plane-controller
        image: docker.io/primussafe/control-plane-controller:202601291743
        imagePullPolicy: Always
        ports:
        - containerPort: 8080
          name: http
          protocol: TCP
        env:
        - name: CONFIG_PATH
          value: /config/config.yaml
        - name: GIN_MODE
          value: release
        volumeMounts:
        - name: config
          mountPath: /config
          readOnly: true
        - name: storage-config
          mountPath: /etc/primus-lens/storage
          readOnly: true
        resources:
          requests:
            cpu: 100m
            memory: 256Mi
          limits:
            cpu: 500m
            memory: 512Mi
        livenessProbe:
          httpGet:
            path: /health
            port: 8080
          initialDelaySeconds: 10
          periodSeconds: 30
        readinessProbe:
          httpGet:
            path: /ready
            port: 8080
          initialDelaySeconds: 5
          periodSeconds: 10
      volumes:
      - name: config
        configMap:
          name: control-plane-controller-config
      - name: storage-config
        secret:
          secretName: primus-lens-storage-config
          optional: true
```

### 2.3 Create Service (Optional)

```yaml
apiVersion: v1
kind: Service
metadata:
  name: control-plane-controller
  namespace: primus-lens
  labels:
    app: control-plane-controller
spec:
  type: ClusterIP
  ports:
  - port: 8080
    targetPort: 8080
    protocol: TCP
    name: http
  selector:
    app: control-plane-controller
```

### 2.4 Apply Resources

```bash
kubectl apply -f control-plane-controller.yaml
```

### 2.5 Verify Deployment

```bash
# Check pod status
kubectl -n primus-lens get pods -l app=control-plane-controller

# Check logs
kubectl -n primus-lens logs -l app=control-plane-controller --tail=50
```

Expected logs:
```
level=info msg="Registered: DataplaneInstallerJob (pure CP job)"
level=info msg="Registered: MultiClusterConfigSyncJob (pure CP job)"
level=info msg="Registered: TraceLensCleanupJob (multi-cluster job)"
level=info msg="Registered: GpuUsageWeeklyReportJob (multi-cluster job)"
level=info msg="Registered: GpuUsageWeeklyReportBackfillJob (multi-cluster job)"
level=info msg="Control plane controller: 5 jobs registered"
level=info msg="Starting health server on :8080"
```

---

## Step 3: Remove Old Control Plane Jobs

Once `control-plane-controller` is running and healthy:

```bash
kubectl -n primus-lens delete deployment primus-lens-control-plane-jobs
```

---

## Step 4: Verify Migration

### 4.1 Check Running Components

```bash
kubectl -n primus-lens get deployments | grep -E "jobs|controller"
```

Expected output:
```
control-plane-controller              1/1     1            1           Xm
primus-lens-apps-dataplane-jobs       1/1     1            1           Xd
```

### 4.2 Verify Jobs Execution

Wait for ~30 seconds and check logs:

```bash
kubectl -n primus-lens logs -l app=control-plane-controller --tail=20
```

Look for:
- `MultiClusterConfigSyncJob: completed - synced X clusters`
- `Created proxy service primus-lens-*`
- `Updated Grafana datasource: *`

---

## Troubleshooting

### Error: column "k8s_manual_mode" does not exist

**Cause**: Database migration not applied.

**Solution**: Apply Step 1 (Database Migration).

### Error: cached plan must not change result type

**Cause**: PostgreSQL prepared statement cache issue after schema change.

**Solution**: Restart the controller pod:
```bash
kubectl -n primus-lens rollout restart deployment control-plane-controller
```

### Error: serviceaccount not found

**Cause**: ServiceAccount specified in deployment doesn't exist.

**Solution**: Check available ServiceAccounts and use an existing one:
```bash
kubectl -n primus-lens get serviceaccounts
```

Common options: `primus-lens-app`, `primus-lens`, `default`

---

## Architecture Summary

```
┌─────────────────────────────────────────────────────────────┐
│                    Management Cluster                        │
├─────────────────────────────────────────────────────────────┤
│                                                             │
│  ┌─────────────────────────┐  ┌─────────────────────────┐  │
│  │ control-plane-controller│  │   primus-lens-api       │  │
│  │                         │  │                         │  │
│  │ - DataplaneInstaller    │  │ - REST API              │  │
│  │ - MultiClusterConfigSync│  │ - MCP Server            │  │
│  │ - TraceLensCleanup      │  │ - TraceLens Proxy       │  │
│  │ - GpuUsageWeeklyReport  │  │                         │  │
│  └───────────┬─────────────┘  └───────────┬─────────────┘  │
│              │                            │                 │
│              └──────────┬─────────────────┘                 │
│                         │                                   │
│                         ▼                                   │
│              ┌─────────────────────┐                        │
│              │  Control Plane DB   │                        │
│              │  - cluster_config   │                        │
│              │  - tracelens_sessions│                       │
│              │  - gpu_usage_reports │                       │
│              └─────────────────────┘                        │
└─────────────────────────────────────────────────────────────┘
                         │
         ┌───────────────┼───────────────┐
         ▼               ▼               ▼
┌─────────────┐   ┌─────────────┐   ┌─────────────┐
│  Cluster A  │   │  Cluster B  │   │  Cluster C  │
│             │   │             │   │             │
│ dataplane-  │   │ dataplane-  │   │ dataplane-  │
│   jobs      │   │   jobs      │   │   jobs      │
│             │   │             │   │             │
│ - GPU stats │   │ - GPU stats │   │ - GPU stats │
│ - Workload  │   │ - Workload  │   │ - Workload  │
│ - Storage   │   │ - Storage   │   │ - Storage   │
└─────────────┘   └─────────────┘   └─────────────┘
```
