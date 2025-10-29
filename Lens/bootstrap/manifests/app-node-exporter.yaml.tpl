apiVersion: v1
kind: ConfigMap
data:
  config.yaml: |
    httpPort: 8989
    nodeExporter:
      containerd_socket_path: /hostrun/containerd/containerd.sock
      grpc_server: primus-lens-jobs.${NAMESPACE}.svc.cluster.local:8991
metadata:
  name: primus-lens-node-exporter-config
---
apiVersion: apps/v1
kind: DaemonSet
metadata:
  labels:
    app: primus-lens-node-exporter
  name: primus-lens-node-exporter
spec:
  revisionHistoryLimit: 10
  selector:
    matchLabels:
      app: primus-lens-node-exporter
  template:
    metadata:
      labels:
        app: primus-lens-node-exporter
      annotations:
        prometheus.io/scrape: "true"
        prometheus.io/port: "8989"
        prometheus.io/path: "/v1/metrics"
    spec:
      containers:
        - env:
            - name: NODE_NAME
              valueFrom:
                fieldRef:
                  apiVersion: v1
                  fieldPath: spec.nodeName
            - name: NODE_IP
              valueFrom:
                fieldRef:
                  apiVersion: v1
                  fieldPath: status.hostIP
            - name: CONFIG_PATH
              value: /etc/primus-lens-node-exporter/config.yaml
          image: primussafe/primus-lens-node-exporter:202509191947
          imagePullPolicy: Always
          name: primus-lens-node-exporter
          securityContext:
            privileged: true
          ports:
            - containerPort: 8989
          volumeMounts:
            - name: config-volume
              mountPath: /etc/primus-lens-node-exporter/config.yaml
              subPath: config.yaml
            - name: host-dev
              mountPath: /hostdev
            - name: host-run
              mountPath: /hostrun
      hostPID: true
      priorityClassName: system-cluster-critical
      serviceAccountName: primus-lens
      terminationGracePeriodSeconds: 10
      tolerations:
        - effect: NoSchedule
          operator: Exists
      imagePullSecrets:
        - name: primus-lens-image
      volumes:
        - name: config-volume
          configMap:
            name: primus-lens-node-exporter-config
            items:
              - key: config.yaml
                path: config.yaml
        - name: host-dev
          hostPath:
            path: /dev
            type: Directory
        - name: host-run
          hostPath:
            path: /run
            type: Directory
  updateStrategy:
    rollingUpdate:
      maxSurge: 0
      maxUnavailable: 10
    type: RollingUpdate
