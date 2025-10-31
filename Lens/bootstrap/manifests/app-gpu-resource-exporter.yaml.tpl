apiVersion: v1
kind: ConfigMap
data:
  config.yaml: |
    httpPort: 8989
    controller:
      namespace: primus-lens
      leaderElectionId: primus-lens-gpu-resource
      metricsPort: 19193
      healthzPort: 19194
      pprofPort: 19195
    metricsRead:
      endpoints: http://vmselect-primus-lens-metrics.primus-lens.svc.cluster.local:8481/select/0/prometheus
    metricsWrite:
      endpoints: http://vminsert-primus-lens-metrics.primus-lens.svc.cluster.local:8480/insert/0/prometheus
    db:
      host: primus-lens-primary.${NAMESPACE}.svc.cluster.local
      port: 5432
      user_name: primus-lens
      db_name: primus-lens
      password: ${PG_PASSWORD}
      enable_ssl: true
metadata:
  name: primus-lens-gpu-resource-exporter-config
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: primus-lens-gpu-resource-exporter
  labels:
    app: primus-lens-gpu-resource-exporter
spec:
  replicas: 1
  selector:
    matchLabels:
      app: primus-lens-gpu-resource-exporter
  template:
    metadata:
      annotations:
        prometheus.io/scrape: "true"
        prometheus.io/port: "8990"
        prometheus.io/path: "/metrics"
      labels:
        app: primus-lens-gpu-resource-exporter
    spec:
      containers:
        - name: app
          image: primussafe/primus-lens-gpu-resource-exporter:202509191924
          env:
            - name: CONFIG_PATH
              value: /etc/primus-lens-gpu-resource-exporter/config.yaml
          ports:
            - containerPort: 8989
          volumeMounts:
            - name: config-volume
              mountPath: /etc/primus-lens-gpu-resource-exporter/config.yaml
              subPath: config.yaml
      imagePullSecrets:
        - name: primus-lens-image
      volumes:
        - name: config-volume
          configMap:
            name: primus-lens-gpu-resource-exporter-config
            items:
              - key: config.yaml
                path: config.yaml
      serviceAccountName: primus-lens
---
apiVersion: v1
kind: Service
metadata:
  name: primus-lens-gpu-resource-exporter
  namespace: primus-lens
  labels:
    app: primus-lens-gpu-resource-exporter
spec:
  selector:
    app: primus-lens-gpu-resource-exporter
  ports:
    - name: grpc
      protocol: TCP
      port: 8991
      targetPort: 8991
  type: ClusterIP

