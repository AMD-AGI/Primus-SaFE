# Ceph Storage Bootstrap

`ceph.sh` deploys Rook-Ceph (`cephcluster.yaml`), the RBD CSI driver, the `rbd`
pool and default StorageClass (`storageclass.yaml`) and the S3 object store
(`ceph-rgw.yaml`).

The StorageClass authenticates with the `storage-rbd` Secret in the `default`
namespace. No manifest carries Ceph credentials, so re-applying one never
resets that Secret. `ceph.sh` writes it:

1. It applies `cephclient.yaml`, a Rook `CephClient` that creates the dedicated
   Ceph user `client.storage-rbd` with the `rbd` profile on pool `rbd`, and
   waits for it to be `Ready`.
2. It decodes the key that Rook stores in the `rook-ceph` namespace, in the
   Secret named by the CephClient's `.status.info.secretName`
   (`rook-ceph-client-storage-rbd`).
3. It writes `storage-rbd` with `userID: storage-rbd` and that key as `userKey`.

Re-running the script rewrites the Secret from the same user.

To use an existing Ceph user instead, export both variables before running
`ceph.sh`. It then skips the `CephClient`:

```sh
export CEPH_USER_ID=<id>    # without the "client." prefix
export CEPH_USER_KEY=<key>  # the plain key: ceph auth get-key client.<id>
```

That user needs `mon 'profile rbd'`, `osd 'profile rbd pool=rbd'` and
`mgr 'profile rbd pool=rbd'`.

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
