/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
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
    primus-safe.workload.version: v1
    primus-safe.workload.kind: Deployment
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
  name: amd-pytorch-job-template
  namespace: primus-safe
  labels:
    primus-safe.workload.version: v1
    primus-safe.workload.kind: PyTorchJob
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
                  image: test-preprocess:latest
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
                    - name: NCCL_IB_TIMEOUT
                      value: "22"
                    - name: NCCL_IB_RETRY_CNT
                      value: "12"
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
              schedulerName: kube-scheduler-plugins
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
                  image: test-preprocess:latest
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
                    - name: NCCL_IB_TIMEOUT
                      value: "22"
                    - name: NCCL_IB_RETRY_CNT
                      value: "12"
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
                    - name: shared-data
                      mountPath: /shared-data
                    - name: varlog
                      mountPath: /var/log
                    - name: podinfo
                      mountPath: /etc/podinfo
                      readOnly: true
              schedulerName: kube-scheduler-plugins
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

	TestJobTemplateConfig = `
apiVersion: v1
kind: ConfigMap
metadata:
  name: amd-job-template
  namespace: "primus-safe"
  labels:
    primus-safe.workload.version: v1
    primus-safe.workload.kind: Job
  annotations:
    # The main container name should match the configuration defined in the template below
    primus-safe.main.container: job
data:
  template: |
    apiVersion: batch/v1
    kind: Job
    spec:
      completionMode: NonIndexed
      backoffLimit: 0
      completions: 1
      parallelism: 1
      suspend: false
      template:
        spec:
          restartPolicy: Never
          dnsPolicy: ClusterFirstWithHostNet
          containers:
            - name: job
              imagePullPolicy: IfNotPresent
              volumeMounts:
                - name: podinfo
                  mountPath: /etc/podinfo
                  readOnly: true
              securityContext:
                capabilities:
                  add: [ "IPC_LOCK", "SYS_PTRACE", "SYS_RESOURCE"]
          schedulerName: kube-scheduler-plugins
          volumes:
            - name: podinfo
              downwardAPI:
                items:
                - path: "labels"
                  fieldRef:
                    fieldPath: metadata.labels
          terminationGracePeriodSeconds: 5
`

	TestCICDScaleSetTemplateConfig = `
apiVersion: v1
kind: ConfigMap
metadata:
  name: github-scale-set-template
  namespace: "primus-safe"
  labels:
    primus-safe.workload.version: v1
    primus-safe.workload.kind: AutoscalingRunnerSet
  annotations:
    # The main container name should match the configuration defined in the template below
    primus-safe.main.container: runner
data:
 template: |
  apiVersion: actions.github.com/v1alpha1
  kind: AutoscalingRunnerSet
  metadata:
    annotations:
      actions.github.com/runner-group-name: Default
      runner-scale-set-id: "1"
    labels:
      app.kubernetes.io/component: autoscaling-runner-set
      app.kubernetes.io/part-of: gha-rs
      app.kubernetes.io/version: 0.13.0
  spec:
    template:
      spec:
        containers:
        - command:
            - /bin/bash
            - -c
            - run.sh
          env:
            - name: RUNNER_ALLOW_RUNASROOT
              value: "1"
            - name: APISERVER_NODE_PORT
              value: "32495"
          image: docker.io/primussafe/cicd-runner-proxy:latest
          name: runner
          resources:
            limits:
              cpu: "2"
              memory: 4Gi
            requests:
              cpu: "2"
              memory: 4Gi
          securityContext:
            privileged: true
        - command:
            - /bin/bash
            - -c
            - run.sh
          env:
            - name: APISERVER_NODE_PORT
              value: "32495"
          image: docker.io/primussafe/cicd-unified-job-proxy:latest
          name: unified_job
          resources:
            limits:
              cpu: "2"
              memory: 4Gi
            requests:
              cpu: "2"
              memory: 4Gi
          securityContext:
            privileged: true
        restartPolicy: Never
        serviceAccountName: primus-safe
        tolerations:
          - effect: NoSchedule
            operator: Exists
          - effect: PreferNoSchedule
            operator: Exists
          - effect: NoExecute
            operator: Exists
`

	TestRayJobTemplateConfig = `
apiVersion: v1
kind: ConfigMap
metadata:
  name: amd-ray-job-template
  namespace: primus-safe
  labels:
    primus-safe.workload.version: v1
    primus-safe.workload.kind: RayJob
  annotations:
    # The main container name should match the configuration defined in the template below
    primus-safe.main.container: main
data:
  template: |
    apiVersion: ray.io/v1
    kind: RayJob
    spec:
      shutdownAfterJobFinishes: true
      ttlSecondsAfterFinished: 10
      backoffLimit: 3
      submissionMode: K8sJobMode
      submitterPodTemplate:
        spec:
          dnsPolicy: ClusterFirstWithHostNet
          initContainers:
            - name: preprocess
              image: docker.io/primussafe/latest
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
            - name: ray-job-submitter
              volumeMounts:
                - name: shared-data
                  mountPath: /shared-data
          schedulerName: kube-scheduler-plugins
          volumes:
            - name: shared-data
              emptyDir: {}
          restartPolicy: Never
      rayClusterSpec:
        headGroupSpec:
          template:
            spec:
              dnsPolicy: ClusterFirstWithHostNet
              initContainers:
              - name: preprocess
                image: docker.io/primussafe/latest
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
                env:
                - name: NCCL_SOCKET_IFNAME
                  value: "ens51f0"
                - name: GLOO_SOCKET_IFNAME
                  value: "ens51f0"
                - name: NCCL_IB_HCA
                  value: "bnxt_re0,bnxt_re1,bnxt_re2,bnxt_re3,bnxt_re4,bnxt_re5,bnxt_re6,bnxt_re7"
                - name: NCCL_IB_TIMEOUT
                  value: "23"
                - name: NCCL_IB_RETRY_CNT
                  value: "11"
                - name: NCCL_IB_QPS_PER_CONNECTION
                  value: "1"
                - name: NCCL_CROSS_NIC
                  value: "0"
                - name: NCCL_CHECKS_DISABLE
                  value: "1"
                - name: HSA_ENABLE_SDMA
                  value: "1"
                - name: POD_NAME
                  valueFrom:
                    fieldRef:
                      apiVersion: v1
                      fieldPath: metadata.name
                - name: POD_UID
                  valueFrom:
                    fieldRef:
                      apiVersion: v1
                      fieldPath: metadata.uid
                - name: POD_IP
                  valueFrom:
                    fieldRef:
                      apiVersion: v1
                      fieldPath: status.podIP
                - name: POD_NAMESPACE
                  valueFrom:
                    fieldRef:
                      apiVersion: v1
                      fieldPath: metadata.namespace
                ports:
                - containerPort: 6379
                  name: gcs-server
                  protocol: TCP
                - containerPort: 8265
                  name: dashboard
                  protocol: TCP
                imagePullPolicy: IfNotPresent
                volumeMounts:
                  - name: shared-data
                    mountPath: /shared-data
                  - name: podinfo
                    mountPath: /etc/podinfo
                    readOnly: true
              schedulerName: kube-scheduler-plugins
              volumes:
                - name: shared-data
                  emptyDir: {}
                - name: podinfo
                  downwardAPI:
                    items:
                    - path: "labels"
                      fieldRef:
                        fieldPath: metadata.labels
              terminationGracePeriodSeconds: 15
        workerGroupSpecs:
        - groupName: "1"
          template:
            spec:
              dnsPolicy: ClusterFirstWithHostNet
              initContainers:
              - name: preprocess
                image: docker.io/primussafe/latest
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
                env:
                - name: NCCL_SOCKET_IFNAME
                  value: "ens51f0"
                - name: GLOO_SOCKET_IFNAME
                  value: "ens51f0"
                - name: NCCL_IB_HCA
                  value: "bnxt_re0,bnxt_re1,bnxt_re2,bnxt_re3,bnxt_re4,bnxt_re5,bnxt_re6,bnxt_re7"
                - name: NCCL_IB_TIMEOUT
                  value: "23"
                - name: NCCL_IB_RETRY_CNT
                  value: "11"
                - name: NCCL_IB_QPS_PER_CONNECTION
                  value: "1"
                - name: NCCL_CROSS_NIC
                  value: "0"
                - name: NCCL_CHECKS_DISABLE
                  value: "1"
                - name: HSA_ENABLE_SDMA
                  value: "1"
                - name: POD_NAME
                  valueFrom:
                    fieldRef:
                      apiVersion: v1
                      fieldPath: metadata.name
                - name: POD_UID
                  valueFrom:
                    fieldRef:
                      apiVersion: v1
                      fieldPath: metadata.uid
                - name: POD_IP
                  valueFrom:
                    fieldRef:
                      apiVersion: v1
                      fieldPath: status.podIP
                - name: POD_NAMESPACE
                  valueFrom:
                    fieldRef:
                      apiVersion: v1
                      fieldPath: metadata.namespace
                imagePullPolicy: IfNotPresent
                volumeMounts:
                  - name: shared-data
                    mountPath: /shared-data
                  - name: podinfo
                    mountPath: /etc/podinfo
                    readOnly: true
              schedulerName: kube-scheduler-plugins
              volumes:
                - name: shared-data
                  emptyDir: {}
                - name: podinfo
                  downwardAPI:
                    items:
                    - path: "labels"
                      fieldRef:
                        fieldPath: metadata.labels
              terminationGracePeriodSeconds: 10
        - groupName: "2"
          template:
            spec:
              dnsPolicy: ClusterFirstWithHostNet
              initContainers:
              - name: preprocess
                image: docker.io/primussafe/latest
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
                env:
                - name: NCCL_SOCKET_IFNAME
                  value: "ens51f0"
                - name: GLOO_SOCKET_IFNAME
                  value: "ens51f0"
                - name: NCCL_IB_HCA
                  value: "bnxt_re0,bnxt_re1,bnxt_re2,bnxt_re3,bnxt_re4,bnxt_re5,bnxt_re6,bnxt_re7"
                - name: NCCL_IB_TIMEOUT
                  value: "23"
                - name: NCCL_IB_RETRY_CNT
                  value: "11"
                - name: NCCL_IB_QPS_PER_CONNECTION
                  value: "1"
                - name: NCCL_CROSS_NIC
                  value: "0"
                - name: NCCL_CHECKS_DISABLE
                  value: "1"
                - name: HSA_ENABLE_SDMA
                  value: "1"
                - name: POD_NAME
                  valueFrom:
                    fieldRef:
                      apiVersion: v1
                      fieldPath: metadata.name
                - name: POD_UID
                  valueFrom:
                    fieldRef:
                      apiVersion: v1
                      fieldPath: metadata.uid
                - name: POD_IP
                  valueFrom:
                    fieldRef:
                      apiVersion: v1
                      fieldPath: status.podIP
                - name: POD_NAMESPACE
                  valueFrom:
                    fieldRef:
                      apiVersion: v1
                      fieldPath: metadata.namespace
                imagePullPolicy: IfNotPresent
                volumeMounts:
                  - name: shared-data
                    mountPath: /shared-data
                  - name: podinfo
                    mountPath: /etc/podinfo
                    readOnly: true
              schedulerName: kube-scheduler-plugins
              volumes:
                - name: shared-data
                  emptyDir: {}
                - name: podinfo
                  downwardAPI:
                    items:
                    - path: "labels"
                      fieldRef:
                        fieldPath: metadata.labels
              terminationGracePeriodSeconds: 10
`

	TestMonarchClientTemplateConfig = `
piVersion: v1
kind: ConfigMap
metadata:
  name: amd-monarch-client-template
  namespace: primus-safe
  labels:
    primus-safe.workload.version: v1
    primus-safe.workload.kind: MonarchClient
  annotations:
    # The main container name should match the configuration defined in the template below
    primus-safe.main.container: main
data:
  template: |
    apiVersion: v1
    kind: Pod
    spec:
      dnsPolicy: ClusterFirstWithHostNet
      schedulerName: default-scheduler
      serviceAccount: monarch-client
      serviceAccountName: monarch-client
      initContainers:
        - name: preprocess
          image: harbor.oci-slc.primus-safe.amd.com/proxy/primussafe/preprocess:latest
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
          image: harbor.oci-slc.primus-safe.amd.com/proxy/primussafe/monarch:latest
          imagePullPolicy: Always
          terminationMessagePath: /dev/termination-log
          terminationMessagePolicy: File
          volumeMounts:
            - name: shared-data
              mountPath: /shared-data
          env:
            # TorchFT timeout settings (increase from default 60s to reduce commit failures)
            - name: TORCHFT_TIMEOUT_SEC
              value: "300"
            - name: TORCHFT_QUORUM_TIMEOUT_SEC
              value: "300"
            - name: TORCHFT_CONNECT_TIMEOUT_SEC
              value: "120"
            - name: NCCL_SOCKET_IFNAME
              value: "ens9np0"
            - name: GLOO_SOCKET_IFNAME
              value: "ens9np0"
            - name: NCCL_IB_HCA
              value: "ionic_0,ionic_2,ionic_3,ionic_4,ionic_5,ionic_7,ionic_8,ionic_9"
            - name: NCCL_IB_TIMEOUT
              value: "23"
            - name: NCCL_IB_RETRY_CNT
              value: "11"
            - name: NCCL_IB_QPS_PER_CONNECTION
              value: "1"
            - name: NCCL_CROSS_NIC
              value: "0"
            - name: NCCL_CHECKS_DISABLE
              value: "1"
            - name: HSA_ENABLE_SDMA
              value: "1"
            - name: NCCL_IB_SL
              value: "0"
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
            - name: NODE_NAME
              valueFrom:
                fieldRef:
                  fieldPath: spec.nodeName
      volumes:
        - name: shared-data
          emptyDir: {}
      terminationGracePeriodSeconds: 5
`

	TestMonarchMeshTemplateConfig = `
apiVersion: v1
kind: ConfigMap
metadata:
  name: amd-monarch-mesh-template
  namespace: "primus-safe"
  labels:
    primus-safe.workload.version: v1
    primus-safe.workload.kind: MonarchMesh
  annotations:
    # The main container name should match the configuration defined in the template below
    primus-safe.main.container: main
data:
  template: |
    apiVersion: monarch.pytorch.org/v1alpha1
    kind: MonarchMesh
    spec:
      podTemplate:
        dnsPolicy: ClusterFirstWithHostNet
        schedulerName: kube-scheduler-plugins
        initContainers:
          - name: preprocess
            image: harbor.oci-slc.primus-safe.amd.com/proxy/primussafe/preprocess:latest
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
            image: harbor.oci-slc.primus-safe.amd.com/proxy/primussafe/monarch:latest
            imagePullPolicy: Always
            env:
              # TorchFT timeout settings (increase from default 60s to reduce commit failures)
              - name: TORCHFT_TIMEOUT_SEC
                value: "300"
              - name: TORCHFT_QUORUM_TIMEOUT_SEC
                value: "300"
              - name: TORCHFT_CONNECT_TIMEOUT_SEC
                value: "120"
              - name: NCCL_SOCKET_IFNAME
                value: "ens9np0"
              - name: GLOO_SOCKET_IFNAME
                value: "ens9np0"
              - name: NCCL_IB_HCA
                value: "ionic_0,ionic_2,ionic_3,ionic_4,ionic_5,ionic_7,ionic_8,ionic_9"
              - name: NCCL_IB_TIMEOUT
                value: "23"
              - name: NCCL_IB_RETRY_CNT
                value: "11"
              - name: NCCL_IB_QPS_PER_CONNECTION
                value: "1"
              - name: NCCL_CROSS_NIC
                value: "0"
              - name: NCCL_CHECKS_DISABLE
                value: "1"
              - name: HSA_ENABLE_SDMA
                value: "1"
              - name: NCCL_IB_SL
                value: "0"
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
              - name: NODE_NAME
                valueFrom:
                  fieldRef:
                    fieldPath: spec.nodeName
            volumeMounts:
              - name: shared-data
                mountPath: /shared-data
        volumes:
          - name: shared-data
            emptyDir: {}
        terminationGracePeriodSeconds: 5
`

	TestSandboxConfig = `
apiVersion: v1
kind: ConfigMap
metadata:
  name: amd-sandbox-template
  namespace: "primus-safe"
  labels:
    primus-safe.workload.version: v1
    primus-safe.workload.kind: Sandbox
  annotations:
    # The main container name should match the configuration defined in the template below
    primus-safe.main.container: codeinterpreter
data:
  template: |
    apiVersion: agents.x-k8s.io/v1alpha1
    kind: Sandbox
    spec:
      podTemplate:
        spec:
          containers:
          - args:
            - export PATH=/shared/bin:$PATH && mkdir -p /app && exec /shared/bin/envd
              --port=8080 --workspace=/app
            command:
            - /bin/sh
            - -c
            env:
            - name: PATH
              value: /shared/bin:/home/sandbox/.local/bin:/usr/local/bin:/usr/bin:/bin:/sbin
            - name: ENVD_AUTH_PUBLIC_KEY
              value: |
                -----BEGIN PUBLIC KEY-----
                MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEAysuW9+3nAQqjmekz49RN
                CubMoGZrVghgg7Xu9yhoWgu2ytqrTk1PcGoxqaGOkd+hUoNF6qxSKHP5wPfIvlNL
                PoLzQ5rVXxz5/5uQ1eaqvQyy3GBuCAu+DFZhP2McVsYYiLrznBZ9yIkJQjgckIU6
                Tbzd3dpN4fzzPsnYynlOph5bf9N7P/3yoqjdFItSiyDvvRXj+vjZt00xSk4O/9ZX
                cAXhmjUNcxdPG6nVw4BVDQ3iMnsTJff5Kw1ArfGdriYYrXB5BfNcorvrPlZ0X9z4
                WPJvpXdJHzb+S7nzsDBJRLMmd7yXpNh6dhBdzEXdVpqxULq8V01Y11JThWsRZJQ6
                hwIDAQAB
                -----END PUBLIC KEY-----
            - name: OPENAI_BASE_URL
              value: https://oci-slc.primus-safe.amd.com/api/v1/llm-proxy/v1
            - name: WORKLOAD_MANAGER_URL
              value: http://workloadmanager.agent-sandbox-system.svc.cluster.local:8080
            imagePullPolicy: IfNotPresent
            name: codeinterpreter
            readinessProbe:
              failureThreshold: 30
              httpGet:
                path: /health
                port: 8080
              initialDelaySeconds: 1
              periodSeconds: 2
              successThreshold: 1
              timeoutSeconds: 2
            volumeMounts:
            - mountPath: /shared/bin
              name: envd-bin
          initContainers:
          - command:
            - sh
            - -c
            - cp /envd /shared/bin/envd && (cp /tmux /shared/bin/tmux 2>/dev/null || true)
              && (cp /iptables /shared/bin/iptables && cp /iptables /shared/bin/ip6tables
              && ln -sf iptables /shared/bin/iptables-legacy && ln -sf ip6tables /shared/bin/ip6tables-legacy
              && cp /musl-ld.so /shared/bin/ld-musl-x86_64.so.1 && ln -sf ld-musl-x86_64.so.1
              /shared/bin/libc.musl-x86_64.so.1 2>/dev/null || true)
            image: harbor.oci-slc.primus-safe.amd.com/agent-sandbox/agent-sandbox-envd-injector:202604141639
            imagePullPolicy: IfNotPresent
            name: envd-injector
            resources: {}
            volumeMounts:
            - mountPath: /shared/bin
              name: envd-bin
          restartPolicy: Never
          volumes:
          - emptyDir: {}
            name: envd-bin
`

	TestSandboxTemplateData = `
apiVersion: extensions.agents.x-k8s.io/v1alpha1
kind: SandboxTemplate
metadata:
  annotations:
    runtime.agent-sandbox.io/spec-hash: dfa4d9b9d908f0b19641895bce9592e8
  creationTimestamp: "2026-04-07T08:05:21Z"
  generation: 58
  name: primus-claw-executor-gpu-a9563c537593c7
  namespace: default
  ownerReferences:
  - apiVersion: runtime.agent-sandbox.io/v1alpha1
    blockOwnerDeletion: true
    controller: true
    kind: CodeInterpreter
    name: primus-claw-executor-gpu-a9563c537593c7
    uid: 841b9e9e-4fa9-4769-95ed-7e33221f3859
  resourceVersion: "195463510"
  uid: 548c7a16-95a2-4ae7-9776-73eb42d3f8b4
spec:
  podTemplate:
    metadata:
      annotations:
        description: Primus-Claw executor (FastAPI + Claude Agent SDK)
      labels:
        component: executor
        team: primus-claw
    spec:
      containers:
      - args:
        - export PATH=/shared/bin:$PATH && mkdir -p /app && exec /shared/bin/envd
          --port=8080 --workspace=/app
        command:
        - /bin/sh
        - -c
        env:
        - name: ENGINE_TYPE
          value: claude
        - name: EXECUTOR_TYPE
          value: ts
        - name: ANTHROPIC_SKIP_TLS_VERIFY
          value: "true"
        - name: NODE_TLS_REJECT_UNAUTHORIZED
          value: "0"
        - name: PYTHONHTTPSVERIFY
          value: "0"
        - name: API_TIMEOUT_MS
          value: "3000000"
        - name: EXECUTOR_CLI_INITIALIZE_TIMEOUT_MS
          value: "300000"
        - name: CLAUDE_CODE_STREAM_CLOSE_TIMEOUT
          value: "300000"
        - name: CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC
          value: "1"
        - name: CLAUDE_THINKING_MODE
          value: enabled
        - name: CLAUDE_INCLUDE_PARTIAL_MESSAGES
          value: "true"
        - name: WORKSPACE_PATH
          value: /workspace
        - name: S3_BUCKET
          value: claw
        - name: S3_REGION
          value: us
        - name: S3_FORCE_PATH_STYLE
          value: "true"
        - name: PATH
          value: /shared/bin:/home/sandbox/.local/bin:/usr/local/bin:/usr/bin:/bin:/sbin
        - name: ENVD_AUTH_PUBLIC_KEY
          value: |
            -----BEGIN PUBLIC KEY-----
            MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEAysuW9+3nAQqjmekz49RN
            CubMoGZrVghgg7Xu9yhoWgu2ytqrTk1PcGoxqaGOkd+hUoNF6qxSKHP5wPfIvlNL
            PoLzQ5rVXxz5/5uQ1eaqvQyy3GBuCAu+DFZhP2McVsYYiLrznBZ9yIkJQjgckIU6
            Tbzd3dpN4fzzPsnYynlOph5bf9N7P/3yoqjdFItSiyDvvRXj+vjZt00xSk4O/9ZX
            cAXhmjUNcxdPG6nVw4BVDQ3iMnsTJff5Kw1ArfGdriYYrXB5BfNcorvrPlZ0X9z4
            WPJvpXdJHzb+S7nzsDBJRLMmd7yXpNh6dhBdzEXdVpqxULq8V01Y11JThWsRZJQ6
            hwIDAQAB
            -----END PUBLIC KEY-----
        - name: OPENAI_BASE_URL
          value: https://oci-slc.primus-safe.amd.com/api/v1/llm-proxy/v1
        - name: WORKLOAD_MANAGER_URL
          value: http://workloadmanager.agent-sandbox-system.svc.cluster.local:8080
        image: harbor.oci-slc.primus-safe.amd.com/sync/lmsysorg/sglang:v0.5.9-rocm700-mi35x
        imagePullPolicy: IfNotPresent
        name: codeinterpreter
        readinessProbe:
          failureThreshold: 30
          httpGet:
            path: /health
            port: 8080
          initialDelaySeconds: 1
          periodSeconds: 2
          successThreshold: 1
          timeoutSeconds: 2
        resources:
          limits:
            amd.com/gpu: "8"
            cpu: "96"
            ephemeral-storage: 1Ti
            memory: 1Ti
          requests:
            amd.com/gpu: "8"
            cpu: "96"
            ephemeral-storage: 1Ti
            memory: 1Ti
        volumeMounts:
        - mountPath: /shared/bin
          name: envd-bin
        - mountPath: /dev/shm
          name: dshm
        - mountPath: /shared_nfs
          name: shared-nfs
          readOnly: true
        - mountPath: /hyperloom
          name: hyperloom
          readOnly: true
      initContainers:
      - command:
        - sh
        - -c
        - cp /envd /shared/bin/envd && cp /tmux /shared/bin/tmux 2>/dev/null || true
        image: harbor.oci-slc.primus-safe.amd.com/agent-sandbox/agent-sandbox-envd-injector:202604070913
        imagePullPolicy: IfNotPresent
        name: envd-injector
        resources: {}
        volumeMounts:
        - mountPath: /shared/bin
          name: envd-bin
      nodeSelector:
        primus-safe.workspace.id: control-plane-sandbox
      restartPolicy: Never
      volumes:
      - emptyDir: {}
        name: envd-bin
      - emptyDir:
          medium: Memory
          sizeLimit: 16Gi
        name: dshm
      - hostPath:
          path: /shared_nfs
          type: Directory
        name: shared-nfs
      - hostPath:
          path: /hyperloom
          type: Directory
        name: hyperloom
`
)
