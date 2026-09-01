# Ceph Storage Bootstrap

Before applying `storageclass.yaml`, replace the `storage-rbd` Secret placeholders with a dedicated Ceph user and key.

Do not deploy the manifest with `<ceph-user-id>` or `<ceph-user-key>` unchanged. The RBD CSI driver requires valid credentials for provisioning, expanding, and staging volumes.

## Node plugin tolerations

`ceph.sh` installs the RBD driver with `ceph-csi-rbd-values.yaml`, whose only
job is to give the node plugin `tolerations: [{operator: Exists}]`. The upstream
chart ships none, so on a cluster that taints nodes the DaemonSet silently
covers only the untainted ones and any node that loses its plugin pod can never
run an RBD-backed workload again. The values file explains the failure in full.

An existing install predates this and will not pick the file up on its own:

```sh
helm upgrade --namespace ceph-csi-rbd ceph-csi-rbd ceph-csi/ceph-csi-rbd \
  -f ceph-csi-rbd-values.yaml --reuse-values
```

Then check that the DaemonSet's `DESIRED` equals the node count — that number
falling short is the symptom, and nothing else alerts on it:

```sh
kubectl get nodes --no-headers | wc -l
kubectl -n ceph-csi-rbd get ds ceph-csi-rbd-nodeplugin
```

The rollout that follows is serial (`maxUnavailable: 1`) and stalls while any
node is unreachable, which is harmless: pods already running keep the old spec
and work fine, and pods created from then on carry the tolerations.
