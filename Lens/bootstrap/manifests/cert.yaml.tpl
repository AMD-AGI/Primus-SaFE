apiVersion: cert-manager.io/v1
kind: Issuer
metadata:
  name: selfsigned-issuer
  namespace: ${NAMESPACE}
spec:
  selfSigned: {}
---
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: default
  namespace: ${NAMESPACE}
spec:
  secretName: default
  dnsNames:
    - ${CLUSTER_NAME}.lens-primus.ai
  issuerRef:
    name: selfsigned-issuer
    kind: Issuer
  duration: 2160h # 90 days
  renewBefore: 12h