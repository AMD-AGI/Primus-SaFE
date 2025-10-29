apiVersion: operator.victoriametrics.com/v1beta1
kind: VMPodScrape
metadata:
  name: juicefs-mounts-monitor
  labels:
    name: juicefs-mounts-monitor
spec:
  namespaceSelector:
    matchNames:
      - juicefs-csi
  selector:
    matchLabels:
      app.kubernetes.io/name: juicefs-mount
  podMetricsEndpoints:
    - port: metrics
      path: /metrics
      scheme: http
      interval: 30s
---
apiVersion: operator.victoriametrics.com/v1beta1
kind: VMServiceScrape
metadata:
  name: juicefs-s3-gateway
spec:
  namespaceSelector:
    matchNames:
      - juicefs-csi
  selector:
    matchLabels:
      app.kubernetes.io/name: juicefs-s3-gateway
  endpoints:
    - port: metrics
---
apiVersion: operator.victoriametrics.com/v1beta1
kind: VMPodScrape
metadata:
  name: juicefs-controller-monitor
  labels:
    name: juicefs-controller-monitor
spec:
  namespaceSelector:
    matchNames:
      - juicefs-csi
  selector:
    matchLabels:
      app.kubernetes.io/name: juicefs-csi-driver
  podMetricsEndpoints:
    - port: metrics
      path: /metrics
      scheme: http
      interval: 30s
---
apiVersion: operator.victoriametrics.com/v1beta1
kind: VMServiceScrape
metadata:
  name: amd-gpu-operator
spec:
  attach_metadata: {}
  endpoints:
    - attach_metadata: {}
      interval: 30s
      path: /metrics
      port: exporter-port
      scrapeTimeout: 10s
  namespaceSelector:
    matchNames:
      - kube-amd-gpu
  selector:
    matchLabels:
      app.kubernetes.io/service: default-metrics-exporter