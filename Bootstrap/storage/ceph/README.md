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

## Object store (RGW) networking

`ceph-rgw.yaml` keeps the S3 gateway (RGW) off the host network with
`gateway.hostNetwork: false`, although `cephcluster.yaml` turns host networking
on for the other Ceph daemons. The Higress gateway
(`Bootstrap/higress/values.yaml`) runs on the host network and binds ports 80
and 443 on each node that runs one of its pods. A host-networked RGW would take
port 80 on its node too, and because the RGW pod declares no ports the scheduler
cannot keep the two apart: on a shared node, whichever starts second fails to
bind.

In the cluster, S3 is reachable through the gateway's Service on port 80 (Rook
points it at RGW's internal port 8080):

```text
http://rook-ceph-rgw-object-store.rook-ceph.svc
```

RGW serves plain HTTP here, so keep the `http://` scheme when you give this
endpoint to a client. Parts of SaFE assume `https://` when the scheme is left
out.

Nothing exposes S3 outside the cluster by default. If external clients need it,
route to that Service through an Ingress on Higress, or add a separate
`NodePort` or `LoadBalancer` Service that selects the RGW pods and targets port
8080, as Rook's `deploy/examples/rgw-external.yaml` does. Don't change the type
of `rook-ceph-rgw-object-store` itself, since Rook rewrites that Service's spec
whenever it reconciles the object store, and don't move RGW back onto the host
network.
