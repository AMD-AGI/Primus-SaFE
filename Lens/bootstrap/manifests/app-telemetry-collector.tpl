apiVersion: v1
kind: ConfigMap
data:
  config.yaml: |
    httpPort: 8989
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
  name: primus-lens-telemetry-processor-config
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: primus-lens-telemetry-processor
  labels:
    app: primus-lens-telemetry-processor
spec:
  replicas: 1
  selector:
    matchLabels:
      app: primus-lens-telemetry-processor
  template:
    metadata:
      annotations:
        prometheus.io/scrape: "true"
        prometheus.io/port: "8990"
        prometheus.io/path: "/metrics"
      labels:
        app: primus-lens-telemetry-processor
        primus-lens-app-name: telemetry-processor
    spec:
      containers:
        - name: app
          image: primussafe/primus-lens-telemetry-processor:202509191924
          env:
            - name: CONFIG_PATH
              value: /etc/primus-lens-telemetry-processor/config.yaml
          ports:
            - containerPort: 8989
          volumeMounts:
            - name: config-volume
              mountPath: /etc/primus-lens-telemetry-processor/config.yaml
              subPath: config.yaml
      imagePullSecrets:
        - name: primus-lens-image
      volumes:
        - name: config-volume
          configMap:
            name: primus-lens-telemetry-processor-config
            items:
              - key: config.yaml
                path: config.yaml
      serviceAccountName: primus-lens
---
apiVersion: v1
kind: Service
metadata:
  name: primus-lens-telemetry-processor
  labels:
    app: primus-lens-telemetry-processor
spec:
  selector:
    app: primus-lens-telemetry-processor
  ports:
    - name: http
      protocol: TCP
      port: 8989
      targetPort: 8989
  type: ClusterIP