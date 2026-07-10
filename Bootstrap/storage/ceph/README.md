# Ceph Storage Bootstrap

Before applying `storageclass.yaml`, replace the `storage-rbd` Secret placeholders with a dedicated Ceph user and key.

Do not deploy the manifest with `<ceph-user-id>` or `<ceph-user-key>` unchanged. The RBD CSI driver requires valid credentials for provisioning, expanding, and staging volumes.
