apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: system-tuner
spec:
  selector:
    matchLabels:
      app: system-tuner
  template:
    metadata:
      labels:
        app: system-tuner
        primus-lens-app-name: system-tuner
    spec:
      securityContext:
        runAsUser: 0
        runAsGroup: 0
        fsGroup: 0
      imagePullSecrets:
        - name: primus-lens-image
      volumes:
        - name: host-etc
          hostPath:
            path: /etc
            type: Directory
        - name: host-proc
          hostPath:
            path: /proc
            type: Directory
      containers:
        - name: system-tuner
          image: primussafe/primus-lens-system-tuner:202509251859
          imagePullPolicy: IfNotPresent
          securityContext:
            privileged: true
          volumeMounts:
            - name: host-etc
              mountPath: /etc
              readOnly: false
            - name: host-proc
              mountPath: /host-proc
              readOnly: true
          resources:
            limits:
              cpu: 100m
              memory: 128Mi
            requests:
              cpu: 100m
              memory: 128Mi
      restartPolicy: Always
  updateStrategy:
    rollingUpdate:
      maxSurge: 0
      maxUnavailable: 10
    type: RollingUpdate