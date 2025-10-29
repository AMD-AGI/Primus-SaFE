apiVersion: v1
kind: ConfigMap
data:
  config.yaml: |
    httpPort: 8989
    controllerConfig:
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
  name: primus-lens-api-config
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: primus-lens-api
  labels:
    app: primus-lens-api
spec:
  replicas: 1
  selector:
    matchLabels:
      app: primus-lens-api
  template:
    metadata:
      labels:
        app: primus-lens-api
    spec:
      containers:
        - name: app
          image: primussafe/primus-lens-api:202509191924
          env:
            - name: CONFIG_PATH
              value: /etc/primus-lens-api/config.yaml
          ports:
            - containerPort: 8989
          volumeMounts:
            - name: config-volume
              mountPath: /etc/primus-lens-api/config.yaml
              subPath: config.yaml
      imagePullSecrets:
        - name: primus-lens-image
      volumes:
        - name: config-volume
          configMap:
            name: primus-lens-api-config
            items:
              - key: config.yaml
                path: config.yaml
      serviceAccountName: primus-lens
---
apiVersion: v1
kind: Service
metadata:
  name: primus-lens-api
  labels:
    app: primus-lens-api
spec:
  selector:
    app: primus-lens-api
  ports:
    - name: http
      protocol: TCP
      port: 8989
      targetPort: 8989
  type: ClusterIP
