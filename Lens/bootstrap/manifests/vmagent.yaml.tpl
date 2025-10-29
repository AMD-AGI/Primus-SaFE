apiVersion: operator.victoriametrics.com/v1beta1
kind: VMAgent
metadata:
  name: primus-lens-vm
spec:
  logLevel: ERROR
  extraArgs:
    promscrape.maxScrapeSize: "524288000"
  scrapeTimeout: 20s
  scrapeInterval: 30s
  selectAllByDefault: true
  replicaCount: 1
  vmAgentExternalLabelName: vmagent_primus_lengs
  remoteWrite:
    - url: "http://vminsert-primus-lens-metrics.${NAMESPACE}.svc.cluster.local:8480/insert/0/prometheus/api/v1/write"
    - url: "http://primus-lens-telemetry-processor.${NAMESPACE}.svc.cluster.local:8989/v1/prometheus"
  serviceScrapeRelabelTemplate:
    - sourceLabels:
        - __meta_kubernetes_endpoint_node_name
      targetLabel: primus_lens_node_name
    - replacement: "${CLUSTER_NAME}"
      target_label: primus_lens_cluster
  podScrapeRelabelTemplate:
    - sourceLabels:
        - __meta_kubernetes_pod_node_name
      targetLabel: primus_lens_node_name
    - replacement: "${CLUSTER_NAME}"
      targetLabel: primus_lens_cluster
  inlineScrapeConfig: |
          - job_name: pods
            kubernetes_sd_configs:
              - role: pod
            relabel_configs:
              - action: drop
                source_labels: [__meta_kubernetes_namespace]
                regex: kube-system
              - source_labels: [__meta_kubernetes_pod_annotation_prometheus_io_scrape]
                action: keep
                regex: true
              - action: drop
                source_labels: [__meta_kubernetes_pod_container_init]
                regex: true
              - source_labels: [__meta_kubernetes_pod_annotation_prometheus_io_path]
                action: replace
                target_label: __metrics_path__
                regex: (.+)
              - source_labels: [__address__, __meta_kubernetes_pod_annotation_prometheus_io_port]
                action: replace
                regex: ([^:]+)(?::\d+)?;(\d+)
                replacement: $1:$2
                target_label: __address__
              - source_labels: [__meta_kubernetes_namespace]
                action: replace
                target_label: namespace
              - source_labels: [__meta_kubernetes_pod_name]
                action: replace
                target_label: primus_lens_source_pod_name
              - source_labels: [__meta_kubernetes_pod_label_app]
                target_label: app
              - target_label: primus_lens_cluster
                replacement: "${CLUSTER_NAME}"
              - target_label: primus_lens_node_name
                source_labels: [__meta_kubernetes_pod_node_name]
          - job_name: endpoints
            kubernetes_sd_configs:
              - role: endpoints
            relabel_configs:
              - action: drop
                source_labels: [__meta_kubernetes_namespace]
                regex: kube-system
              - source_labels: [__meta_kubernetes_service_annotation_prometheus_io_scrape]
                action: keep
                regex: true
              - action: keep_if_equal
                source_labels: [__meta_kubernetes_service_annotation_prometheus_io_port, __meta_kubernetes_pod_container_port_number]
              - action: drop
                source_labels: [__meta_kubernetes_pod_container_init]
                regex: true
              - source_labels: [__meta_kubernetes_service_annotation_prometheus_io_path]
                action: replace
                target_label: __metrics_path__
                regex: (.+)
              - source_labels: [__address__, __meta_kubernetes_service_annotation_prometheus_io_port]
                action: replace
                regex: ([^:]+)(?::\d+)?;(\d+)
                replacement: $1:$2
                target_label: __address__
              - source_labels: [__meta_kubernetes_namespace]
                action: replace
                target_label: namespace
              - source_labels: [__meta_kubernetes_pod_name]
                action: replace
                target_label: primus_lens_source_pod_name
              - source_labels: [__meta_kubernetes_service_name]
                target_label: service
              - source_labels: [__meta_kubernetes_service_name]
                target_label: job
              - source_labels: [__meta_kubernetes_pod_label_app]
                target_label: app
              - target_label: primus_lens_cluster
                replacement: "${CLUSTER_NAME}"
              - source_labels: [__meta_kubernetes_endpoint_node_name]
                target_label: primus_lens_node_name
  resources:
    limits:
      cpu: ${VMAGENT_CPU}
      memory: ${VMAGENT_MEMORY}
    requests:
      cpu: ${VMAGENT_CPU}
      memory: ${VMAGENT_MEMORY}
