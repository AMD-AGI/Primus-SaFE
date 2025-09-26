apiVersion: grafana.integreatly.org/v1beta1
kind: Grafana
metadata:
  name: grafana
  labels:
    system: primus-safe
spec:
  config:
    paths:
      data: /var/lib/grafana/
      logs: /var/log/grafana
      plugins: /var/lib/grafana/plugins
      provisioning: /etc/grafana/provisioning
    analytics:
      check_for_updates: "true"
    log:
      mode: console
    grafana_net:
      url: https://grafana.net
    server:
      domain: "127.0.0.1"
      root_url: "http://127.0.0.1:30183/grafana"
      serve_from_sub_path: "true"
    security:
      allow_embedding: "true"
    auth.anonymous:
      enabled: "true"
  persistentVolumeClaim:
    spec:
      accessModes:
        - "ReadWriteOnce"
      resources:
        requests:
          storage: 1Gi
      storageClassName: "${STORAGE_CLASS}"