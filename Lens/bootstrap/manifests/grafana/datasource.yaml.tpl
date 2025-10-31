apiVersion: grafana.integreatly.org/v1beta1
kind: GrafanaDatasource
metadata:
  name: postgresql
spec:
  instanceSelector:
    matchLabels:
      system: primus-lens
  datasource:
    name: postgresql
    type: postgres
    jsonData:
      database: primus-lens
      connMaxLifetime: 14400
      maxIdleConns: 2
      maxOpenConns: 0
      postgresVersion: 1400
      sslmode: require
      timescaledb: false
    access: proxy
    secureJsonData:
      password: ${PG_PASSWORD}
    url: primus-lens-ha.${NAMESPACE}.svc.cluster.local:5432
    user: primus-lens
---
apiVersion: grafana.integreatly.org/v1beta1
kind: GrafanaDatasource
metadata:
  name: prometheus
spec:
  instanceSelector:
    matchLabels:
      system: primus-lens
  datasource:
    name: prometheus
    type: prometheus
    access: proxy
    url: http://vmselect-primus-lens-metrics.${NAMESPACE}.svc.cluster.local:8481/select/0/prometheus
    isDefault: true
    jsonData:
      "tlsSkipVerify": true
      "timeInterval": "5s"
