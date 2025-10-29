apiVersion: postgres-operator.crunchydata.com/v1beta1
kind: PostgresCluster
metadata:
  name: primus-lens
  namespace: primus-lens
spec:
  config:
    parameters:
      max_connections: "500"
  backups:
    pgbackrest:
      repos:
        - name: repo1
          volume:
            volumeClaimSpec:
              storageClassName: "${STORAGE_CLASS}"
              accessModes:
                - "${ACCESS_MODE}"
              resources:
                requests:
                  storage: ${PG_BACKUP_SIZE}
  instances:
    - dataVolumeClaimSpec:
        storageClassName: "${STORAGE_CLASS}"
        accessModes:
          - "${ACCESS_MODE}"
        resources:
          requests:
            storage: ${PG_DATA_SIZE}
      name: lens
      replicas: ${PG_REPLICAS}
      resources:

  monitoring:
    pgmonitor:
      exporter:
        image: "registry.developers.crunchydata.com/crunchydata/crunchy-postgres-exporter:ubi8-0.16.0-1"
  port: 5432
  postgresVersion: 17