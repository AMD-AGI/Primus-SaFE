apiVersion: v1
kind: ConfigMap
data:
  config.yaml: |
    httpPort: 8989
    jobs:
      grpc_port: 8991
    controller:
      namespace: ${NAMESPACE}
      leaderElectionId: primus-lens-core
      metricsPort: 19193
      healthzPort: 19194
      pprofPort: 19195
    metricsRead:
      endpoints: http://vmselect-primus-lens-metrics.${NAMESPACE}.svc.cluster.local:8481/select/0/prometheus
    metricsWrite:
      endpoints: http://vminsert-primus-lens-metrics.${NAMESPACE}.svc.cluster.local:8480/insert/0/prometheus
    db:
      host: primus-lens-primary.${NAMESPACE}.svc.cluster.local
      port: 5432
      user_name: primus-lens
      db_name: primus-lens
      password: ${PG_PASSWORD}
      enable_ssl: true
metadata:
  name: primus-lens-jobs-config
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: primus-lens-jobs
  labels:
    app: primus-lens-jobs
spec:
  replicas: 1
  selector:
    matchLabels:
      app: primus-lens-jobs
  template:
    metadata:
      annotations:
        prometheus.io/scrape: "true"
        prometheus.io/port: "8990"
        prometheus.io/path: "/metrics"
      labels:
        app: primus-lens-jobs
        primus-lens-app-name: jobs
    spec:
      containers:
        - name: app
          image: primussafe/primus-lens-jobs:202509191924
          env:
            - name: CONFIG_PATH
              value: /etc/primus-lens-jobs/config.yaml
          ports:
            - containerPort: 8989
          volumeMounts:
            - name: config-volume
              mountPath: /etc/primus-lens-jobs/config.yaml
              subPath: config.yaml
      imagePullSecrets:
        - name: primus-lens-image
      volumes:
        - name: config-volume
          configMap:
            name: primus-lens-jobs-config
            items:
              - key: config.yaml
                path: config.yaml
      serviceAccountName: primus-lens
---
apiVersion: v1
kind: Service
metadata:
  name: primus-lens-jobs
  labels:
    app: primus-lens-jobs
spec:
  selector:
    app: primus-lens-jobs
  ports:
    - name: grpc
      protocol: TCP
      port: 8991
      targetPort: 8991
  type: ClusterIP

