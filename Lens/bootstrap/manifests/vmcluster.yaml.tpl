apiVersion: operator.victoriametrics.com/v1beta1
kind: VMCluster
metadata:
  name: primus-lens-metrics
spec:
  retentionPeriod: "24"
  replicationFactor: 1
  vmstorage:
    replicaCount: ${VMSTORAGE_REPLICAS}
    rollingUpdateStrategy: RollingUpdate
    storageDataPath: "/victoria-metrics-data"
    resources:
      limits:
        cpu: ${VMSTORAGE_CPU}
        memory: ${VMSTORAGE_MEMORY}
      requests:
        cpu: ${VMSTORAGE_CPU}
        memory: ${VMSTORAGE_MEMORY}
    storage:
      volumeClaimTemplate:
        metadata:
          annotations:
            operator.victoriametrics.com/pvc-allow-volume-expansion: "true"
        spec:
          accessModes: [ "${ACCESS_MODE}" ]
          resources:
            requests:
              storage: ${VMSTORAGE_SIZE}
          storageClassName:  "${STORAGE_CLASS}"
  vmselect:
    replicaCount: ${VMSELECT_REPLICAS}
    resources:
      limits:
        cpu: ${VMSELECT_CPU}
        memory: ${VMSELECT_MEMORY}
      requests:
        cpu: ${VMSELECT_CPU}
        memory: ${VMSELECT_MEMORY}
  vminsert:
    replicaCount: ${VMINSERT_REPLICAS}
    extraArgs:
      maxInsertRequestSize: "1000000000"
    resources:
      limits:
        cpu: ${VMINSERT_CPU}
        memory: ${VMINSERT_MEMORY}
      requests:
        cpu: ${VMINSERT_CPU}
        memory: ${VMINSERT_MEMORY}