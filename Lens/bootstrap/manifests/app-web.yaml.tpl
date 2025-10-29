apiVersion: apps/v1
kind: Deployment
metadata:
  name: primus-lens-web
  labels:
    app: primus-lens-web
spec:
  replicas: 1
  selector:
    matchLabels:
      app: primus-lens-web
  template:
    metadata:
      labels:
        app: primus-lens-web
    spec:
      imagePullSecrets:
        - name: primus-lens-image
      containers:
        - name: app
          image: primussafe/primus-lens-web:1.2.3
          ports:
            - containerPort: 80
---
apiVersion: v1
kind: Service
metadata:
  name: primus-lens-web
  labels:
    app: primus-lens-web
spec:
  selector:
    app: primus-lens-web
  ports:
    - name: http
      protocol: TCP
      port: 80
      targetPort: 80
  type: ClusterIP
---
 apiVersion: networking.k8s.io/v1
 kind: Ingress
 metadata:
   annotations:
     higress.io/destination: primus-lens-web.${NAMESPACE}.svc.cluster.local:80
     higress.io/ignore-path-case: "false"
   labels:
     higress.io/domain_${CLUSTER_NAME}.lens-primus.ai: "true"
     higress.io/resource-definer: higress
   name: primus-lens-web
 spec:
   ingressClassName: higress
   rules:
   - host: ${CLUSTER_NAME}.lens-primus.ai
     http:
       paths:
       - backend:
           service:
             name: primus-lens-web
             port:
               number: 80
         path: /
         pathType: Prefix
   tls:
   - hosts:
     - ${CLUSTER_NAME}.lens-primus.ai
     secretName: default