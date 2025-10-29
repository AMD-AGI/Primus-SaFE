apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  annotations:
    higress.io/destination: grafana-embended.${NAMESPACE}.svc.cluster.local:80
    higress.io/ignore-path-case: "true"
  labels:
    higress.io/domain_${CLUSTER_NAME}.lens-primus.ai: "true"
    higress.io/resource-definer: higress
  name: primus-lens-grafana
spec:
  ingressClassName: higress
  rules:
  - host: ${CLUSTER_NAME}.lens-primus.ai
    http:
      paths:
      - backend:
          service:
            name: grafana
            port:
              number: 80
        path: /grafana
        pathType: Prefix
  tls:
  - hosts:
    - ${CLUSTER_NAME}.lens-primus.ai
    secretName: default