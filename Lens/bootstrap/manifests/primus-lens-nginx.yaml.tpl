apiVersion: v1
kind: ConfigMap
metadata:
  name: primus-lens-nginx-config
data:
  nginx.conf: |
    worker_processes 1;
    events { worker_connections 1024; }
    http {
      include       mime.types;
      default_type  application/octet-stream;
      server {
        listen 80;
        server_name _;
        location / {
          proxy_pass http://primus-lens-web.${NAMESPACE}.svc.cluster.local:80;
          proxy_set_header Host $host;
          proxy_set_header X-Real-IP $remote_addr;
          proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
          proxy_set_header X-Forwarded-Proto $scheme;
        }
        location /grafana {
          proxy_pass http://grafana-service.${NAMESPACE}.svc.cluster.local:3000;
          proxy_set_header Host $host;
          proxy_set_header X-Real-IP $remote_addr;
          proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
          proxy_set_header X-Forwarded-Proto $scheme;
          proxy_redirect default;
        }
        access_log /var/log/nginx/access.log;
        error_log /var/log/nginx/error.log;
        }
    }
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: primus-lens-nginx
spec:
  replicas: 1
  selector:
    matchLabels:
      app: primus-lens-nginx
  template:
    metadata:
      labels:
        app: primus-lens-nginx
    spec:
      tolerations:
        - effect: NoSchedule
          operator: Exists
        - effect: PreferNoSchedule
          operator: Exists
        - effect: NoExecute
          operator: Exists
      containers:
        - name: nginx
          image: nginx:latest
          ports:
            - containerPort: 80
              hostPort: 80
          volumeMounts:
            - name: nginx-config
              mountPath: /etc/nginx/nginx.conf
              subPath: nginx.conf
      imagePullSecrets:
        - name: primus-lens-image
      volumes:
        - name: nginx-config
          configMap:
            name: primus-lens-nginx-config
            items:
              - key: nginx.conf
                path: nginx.conf
---
apiVersion: v1
kind: Service
metadata:
  name: primus-lens-nginx
spec:
  ports:
    - nodePort: 30182
      port: 80
      protocol: TCP
      targetPort: 80
  selector:
    app: primus-lens-nginx
  sessionAffinity: None
  type: NodePort
