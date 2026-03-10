apiVersion: v1
kind: ConfigMap
data:
  config.yaml: |
    loadK8SClient: true
    httpPort: 8995
metadata:
  name: primus-lens-component-health-exporter-config
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: primus-lens-component-health-exporter
  labels:
    app: primus-lens-component-health-exporter
spec:
  replicas: 1
  selector:
    matchLabels:
      app: primus-lens-component-health-exporter
  template:
    metadata:
      annotations:
        prometheus.io/scrape: "true"
        prometheus.io/port: "8996"
        prometheus.io/path: "/metrics"
      labels:
        app: primus-lens-component-health-exporter
        primus-lens-app-name: component-health-exporter
    spec:
      containers:
        - name: app
          image: primussafe/primus-lens-component-health-exporter:latest
          env:
            - name: CONFIG_PATH
              value: /etc/primus-lens-component-health-exporter/config.yaml
            - name: CLUSTER_NAME
              value: "${CLUSTER_NAME}"
          ports:
            - containerPort: 8995
          volumeMounts:
            - name: config-volume
              mountPath: /etc/primus-lens-component-health-exporter/config.yaml
              subPath: config.yaml
      imagePullSecrets:
        - name: primus-lens-image
      volumes:
        - name: config-volume
          configMap:
            name: primus-lens-component-health-exporter-config
            items:
              - key: config.yaml
                path: config.yaml
      serviceAccountName: primus-lens
---
apiVersion: v1
kind: Service
metadata:
  name: primus-lens-component-health-exporter
  labels:
    app: primus-lens-component-health-exporter
spec:
  selector:
    app: primus-lens-component-health-exporter
  ports:
    - name: http
      protocol: TCP
      port: 8995
      targetPort: 8995
  type: ClusterIP
