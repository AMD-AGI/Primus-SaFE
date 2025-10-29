apiVersion: opensearch.opster.io/v1
kind: OpenSearchCluster
metadata:
  name: primus-lens-logs
  namespace: primus-lens
spec:
  general:
    serviceName: primus-lens-logs
    version: 2.6.0
    monitoring:
      enable: true
  nodePools:
    - component: nodes
      replicas: 3
      diskSize: ${OPENSEARCH_DISK_SIZE}
      nodeSelector:
      resources:
         requests:
            memory: ${OPENSEARCH_MEMORY}
            cpu: ${OPENSEARCH_CPU}
         limits:
            memory: ${OPENSEARCH_MEMORY}
            cpu: ${OPENSEARCH_CPU}
      roles:
        - "cluster_manager"
        - "data"
      persistence:
        pvc:
          storageClass: "${STORAGE_CLASS}"
          accessModes:
            - "${ACCESS_MODE}"
