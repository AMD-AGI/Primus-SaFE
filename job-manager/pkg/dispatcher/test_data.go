/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package dispatcher

const (
	TestDeploymentTemplateConfig = `
apiVersion: v1
kind: ConfigMap
metadata:
  name: deployment-template
  namespace: primus-safe
  labels:
    group: apps
    version: v1
    kind: Deployment
  annotations:
    primus-safe.gpu.resource.name: "amd.com/gpu"
    # The main container name should match the configuration defined in the template below
    primus-safe.main.container: main
data:
 template: |
   apiVersion: apps/v1
   kind: Deployment
   spec:
     progressDeadlineSeconds: 10800
     template:
       spec:
         dnsPolicy: ClusterFirstWithHostNet
         initContainers:
           - name: preprocess
             image: docker.io/primussafe/preprocess:latest
             imagePullPolicy: IfNotPresent
             command: ["/bin/sh", "-c", "cp -r /preprocess/* /shared-data/"]
             securityContext:
               capabilities:
                 add: [ "IPC_LOCK" ]
             resources:
               limits:
                 cpu: 1000m
                 memory: 128Mi
             volumeMounts:
             - name: shared-data
               mountPath: /shared-data
         containers:
           - name: main
             imagePullPolicy: IfNotPresent
             env:
               - name: NCCL_SOCKET_IFNAME
                 value: "ens51f0"
               - name: GLOO_SOCKET_IFNAME
                 value: "ens51f0"
               - name: NCCL_IB_HCA
                 value: "bnxt_re0,bnxt_re1,bnxt_re2,bnxt_re3,bnxt_re4,bnxt_re5,bnxt_re6,bnxt_re7"
               - name: NCCL_DEBUG
                 value: "INFO"
               - name: NCCL_IB_DISABLE
                 value: "0"
               - name: NCCL_IB_TIMEOUT
                 value: "22"
               - name: NCCL_IB_QPS_PER_CONNECTION
                 value: "8"
               - name: NCCL_IB_RETRY_CNT
                 value: "12"
               - name: NCCL_NVLS_ENABLE
                 value: "0"
               - name: NCCL_SOCKET_FAMILY
                 value: "AF_INET"
               - name: POD_IP
                 valueFrom:
                   fieldRef:
                     fieldPath: status.podIP
             securityContext:
               capabilities:
                 add: [ "IPC_LOCK", "SYS_PTRACE", "SYS_RESOURCE"]
             volumeMounts:
               - name: sugaku-volume
                 mountPath: /dev/shm
               - name: varlog
                 mountPath: /var/log
               - name: shared-data
                 mountPath: /shared-data
         volumes:
           - name: varlog
             hostPath:
               path: /var/log
           - name: shared-data
             emptyDir: {}
         terminationGracePeriodSeconds: 10
         restartPolicy: Always
         schedulerName: default-scheduler
`
	TestPytorchJobTemplateConfig = `
apiVersion: v1
kind: ConfigMap
metadata:
  name: pytorch-job-template
  namespace: primus-safe
  labels:
    group: kubeflow.org
    version: v1
    kind: PyTorchJob
  annotations:
    primus-safe.gpu.resource.name: "amd.com/gpu"
    # The main container name should match the configuration defined in the template below
    primus-safe.main.container: pytorch
data:
 template: |
    apiVersion: kubeflow.org/v1
    kind: PyTorchJob
    spec:
      pytorchReplicaSpecs:
        Master:
          restartPolicy: Never
          template:
            spec:
              dnsPolicy: ClusterFirstWithHostNet
              initContainers:
                - name: preprocess
                  image: docker.io/primussafe/preprocess:latest
                  imagePullPolicy: IfNotPresent
                  command: ["/bin/sh", "-c", "cp -r /preprocess/* /shared-data/"]
                  securityContext:
                    capabilities:
                      add: [ "IPC_LOCK" ]
                  resources:
                    limits:
                      cpu: 1000m
                      memory: 128Mi
                  volumeMounts:
                  - name: shared-data
                    mountPath: /shared-data
              containers:
                - name: pytorch
                  imagePullPolicy: IfNotPresent
                  volumeMounts:
                    - mountPath: /dev/shm
                      name: sugaku-volume
                    - name: shared-data
                      mountPath: /shared-data
                    - name: varlog
                      mountPath: /var/log
                    - name: podinfo
                      mountPath: /etc/podinfo
                      readOnly: true
                  env:
                    - name: NCCL_SOCKET_IFNAME
                      value: "ens51f0"
                    - name: GLOO_SOCKET_IFNAME
                      value: "ens51f0"
                    - name: NCCL_IB_HCA
                      value: "bnxt_re0,bnxt_re1,bnxt_re2,bnxt_re3,bnxt_re4,bnxt_re5,bnxt_re6,bnxt_re7"
                    - name: NCCL_DEBUG
                      value: "INFO"
                    - name: NCCL_IB_DISABLE
                      value: "0"
                    - name: NCCL_IB_TIMEOUT
                      value: "22"
                    - name: NCCL_IB_QPS_PER_CONNECTION
                      value: "8"
                    - name: NCCL_IB_RETRY_CNT
                      value: "12"
                    - name: NCCL_NVLS_ENABLE
                      value: "0"
                    - name: NCCL_SOCKET_FAMILY
                      value: "AF_INET"
                    - name: MAIN_CONTAINER_NAME
                      value: "pytorch"
                    - name: JOB_KIND
                      value: "PytorchJob"
                    - name: POD_NAME
                      valueFrom:
                        fieldRef:
                          fieldPath: metadata.name
                    - name: POD_UID
                      valueFrom:
                        fieldRef:
                          fieldPath: metadata.uid
                    - name: POD_IP
                      valueFrom:
                        fieldRef:
                          fieldPath: status.podIP
                    - name: POD_NAMESPACE
                      valueFrom:
                        fieldRef:
                          fieldPath: metadata.namespace
                  securityContext:
                    capabilities:
                      add: [ "IPC_LOCK", "SYS_PTRACE", "SYS_RESOURCE"]
              schedulerName: default-scheduler
              volumes:
                - name: shared-data
                  emptyDir: {}
                - name: varlog
                  hostPath:
                    path: /var/log
                - name: podinfo
                  downwardAPI:
                    items:
                    - path: "labels"
                      fieldRef:
                        fieldPath: metadata.labels
              terminationGracePeriodSeconds: 5
        Worker:
          restartPolicy: Never
          template:
            spec:
              dnsPolicy: ClusterFirstWithHostNet
              initContainers:
                - name: preprocess
                  image: docker.io/primussafe/preprocess:latest
                  imagePullPolicy: IfNotPresent
                  command: ["/bin/sh", "-c", "cp -r /preprocess/* /shared-data/"]
                  securityContext:
                    capabilities:
                      add: [ "IPC_LOCK" ]
                  resources:
                    limits:
                      cpu: 1000m
                      memory: 128Mi
                  volumeMounts:
                  - name: shared-data
                    mountPath: /shared-data
              containers:
                - name: pytorch
                  imagePullPolicy: IfNotPresent
                  env:
                    - name: NCCL_SOCKET_IFNAME
                      value: "ens51f0"
                    - name: GLOO_SOCKET_IFNAME
                      value: "ens51f0"
                    - name: NCCL_IB_HCA
                      value: "bnxt_re0,bnxt_re1,bnxt_re2,bnxt_re3,bnxt_re4,bnxt_re5,bnxt_re6,bnxt_re7"
                    - name: NCCL_DEBUG
                      value: "INFO"
                    - name: NCCL_IB_DISABLE
                      value: "0"
                    - name: NCCL_IB_TIMEOUT
                      value: "22"
                    - name: NCCL_IB_QPS_PER_CONNECTION
                      value: "8"
                    - name: NCCL_IB_RETRY_CNT
                      value: "12"
                    - name: NCCL_NVLS_ENABLE
                      value: "0"
                    - name: NCCL_SOCKET_FAMILY
                      value: "AF_INET"
                    - name: MAIN_CONTAINER_NAME
                      value: "pytorch"
                    - name: JOB_KIND
                      value: "PytorchJob"
                    - name: POD_NAME
                      valueFrom:
                        fieldRef:
                          fieldPath: metadata.name
                    - name: POD_UID
                      valueFrom:
                        fieldRef:
                          fieldPath: metadata.uid
                    - name: POD_IP
                      valueFrom:
                        fieldRef:
                          fieldPath: status.podIP
                    - name: POD_NAMESPACE
                      valueFrom:
                        fieldRef:
                          fieldPath: metadata.namespace
                  securityContext:
                    capabilities:
                      add: [ "IPC_LOCK", "SYS_PTRACE", "SYS_RESOURCE"]
                  volumeMounts:
                    - name: sugaku-volume
                      mountPath: /dev/shm
                    - name: shared-data
                      mountPath: /shared-data
                    - name: varlog
                      mountPath: /var/log
                    - name: podinfo
                      mountPath: /etc/podinfo
                      readOnly: true
              schedulerName: default-scheduler
              volumes:
                - name: shared-data
                  emptyDir: {}
                - name: varlog
                  hostPath:
                    path: /var/log
                - name: podinfo
                  downwardAPI:
                    items:
                    - path: "labels"
                      fieldRef:
                        fieldPath: metadata.labels
              terminationGracePeriodSeconds: 5
`
)
