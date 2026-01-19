/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package utils

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
)

const (
	TestPytorchData = `
apiVersion: kubeflow.org/v1
kind: PyTorchJob
metadata:
  name: "test-job"
spec:
  pytorchReplicaSpecs:
    Master:
      replicas: 1
      restartPolicy: Never
      template:
        spec:
          containers:
            - command:
                - sh
                - -c
                - test.sh
              env:
                - name: NCCL_SOCKET_IFNAME
                  value: eth0
              image: test-image:0.0.1
              name: pytorch
              resources:
                limits:
                  cpu: "48"
                  memory: 960Gi
                  amd.com/gpu: "8"
              volumeMounts:
                - mountPath: /pfs
                  name: pfs
                - name: shared-memory
                  mountPath: /dev/shm
          dnsPolicy: ClusterFirstWithHostNet
          hostNetwork: true
          priorityClassName: "test-med-priority"
          schedulerName: default-scheduler
          volumes:
            - hostPath:
                path: /pfs
              name: pfs
            - emptyDir:
                medium: Memory
                sizeLimit: 10Gi
              name: shared-memory
    Worker:
      replicas: 63
      restartPolicy: Never
      template:
        spec:
          containers:
            - command:
                - sh
                - -c
                - test.sh
              env:
                - name: NCCL_SOCKET_IFNAME
                  value: eth0
              image: docker.io/test-image:0.0.1
              name: pytorch
              resources:
                limits:
                  cpu: "48"
                  memory: 960Gi
                  amd.com/gpu: "8"
                  rdma/hca: "1k"
              volumeMounts:
                - mountPath: /pfs
                  name: pfs
                - name: shared-memory
                  mountPath: /dev/shm
          dnsPolicy: ClusterFirstWithHostNet
          hostNetwork: true
          schedulerName: default-scheduler
          priorityClassName: "test-med-priority"
          volumes:
            - hostPath:
                path: /pfs
              name: pfs
            - emptyDir:
                medium: Memory
                sizeLimit: 10Gi
              name: shared-memory
status:
  conditions:
    - lastTransitionTime: "2025-05-21T11:27:56Z"
      lastUpdateTime: "2025-05-21T11:27:56Z"
      message: job is created.
      reason: PyTorchJobCreated
      status: "True"
      type: Created
  replicaStatuses:
    Master:
      active: 1
      selector: key1=value1
    Worker:
      active: 63
      selector: key1=value1
  startTime: "2025-05-21T11:28:04Z"
  `
	TestPytorchData2 = `
apiVersion: kubeflow.org/v1
kind: PyTorchJob
metadata:
  name: "test-job"
spec:
  pytorchReplicaSpecs:
    Master:
      replicas: 1
      restartPolicy: Never
      template:
        spec:
          containers:
            - command:
                - sh
                - -c
                - test.sh
              env:
                - name: NCCL_SOCKET_IFNAME
                  value: eth0
              image: docker.io/test-image:0.0.1
              name: pytorch
              resources:
                limits:
                  cpu: "48"
                  memory: 960Gi
                  amd.com/gpu: "8"
              volumeMounts:
                - mountPath: /pfs
                  name: pfs
          dnsPolicy: ClusterFirstWithHostNet
          hostNetwork: true
          schedulerName: default-scheduler
          volumes:
            - hostPath:
                path: /pfs
              name: pfs
  `

	TestJobData = `
apiVersion: batch/v1
kind: Job
metadata:
  annotations:
    primus-safe.workload.dispatch.count: "1"
  creationTimestamp: "2025-05-04T05:30:36Z"
  generation: 1
  name: test-abcd
  namespace: test-namespace
  resourceVersion: "1"
spec:
  backoffLimit: 0
  completionMode: NonIndexed
  completions: 2
  parallelism: 2
  suspend: false
  template:
    metadata:
      labels:
        primus-safe.workload.id: test-abcd
    spec:
      affinity:
        nodeAffinity:
          requiredDuringSchedulingIgnoredDuringExecution:
            nodeSelectorTerms:
            - matchExpressions:
              - key: primus-safe.workspace.id
                operator: In
                values:
                - test-workspace
      containers:
      - command:
        - sleep 10s; exit 3
        image: docker.hub/test-image:0.0.1
        imagePullPolicy: IfNotPresent
        name: job
        resources:
          limits:
            cpu: "1"
            memory: 100Mi
        securityContext:
          capabilities:
            add: [ "IPC_LOCK", "SYS_PTRACE", "SYS_RESOURCE"]
        terminationMessagePath: /dev/termination-log
        terminationMessagePolicy: File
      dnsPolicy: ClusterFirst
      restartPolicy: Never
      schedulerName: default-scheduler
      securityContext: {}
      terminationGracePeriodSeconds: 30
status:
  active: 2
  ready: 2
  startTime: "2025-07-27T06:17:42Z"
  terminating: 0
  uncountedTerminatedPods: {}
`
	TestDeploymentData = `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: test-deployment
  namespace: primus-safe
spec:
  selector:
    matchLabels:
      control-plane: test-deployment
  replicas: 2
  template:
    metadata:
      labels:
        control-plane: test-deployment
    spec:
      containers:
        - command:
            - sh
            - -c
            - /bin/sh run.sh 'abcd'
          env:
            - name: NCCL_SOCKET_IFNAME
              value: eth0
            - name: GLOO_SOCKET_IFNAME
              value: eth0
          image: test-image:latest
          name: test
          resources:
            limits:
              cpu: "64"
              ephemeral-storage: 100Gi
              memory: 200Gi
              amd.com/gpu: "8"
            requests:
              cpu: "64"
              ephemeral-storage: 50Gi
              memory: 100Gi
              amd.com/gpu: "8"
            volumeMounts:
              - name: shared-memory
                mountPath: /dev/shm
      volumes:
        - emptyDir:
            medium: Memory
            sizeLimit: 20Gi
          name: shared-memory
        - name: shared-data
          emptyDir: {}
      terminationGracePeriodSeconds: 10
status:
  availableReplicas: 2
  conditions:
  - lastTransitionTime: "2025-05-21T06:29:08Z"
    lastUpdateTime: "2025-05-21T06:29:08Z"
    message: Deployment has minimum availability.
    reason: MinimumReplicasAvailable
    status: "True"
    type: Available
  readyReplicas: 2
  replicas: 2
  updatedReplicas: 1
      `
	TestStatefulSetData = `
apiVersion: apps/v1
kind: StatefulSet
metadata:
  creationTimestamp: "2025-05-21T08:47:29Z"
  generation: 1
  labels:
    primus-safe.workload.dispatch.count: "1"
    primus-safe.workload.id: safe-test-abcd
  name: safe-test-abcd
  namespace: test-cluster-dev
spec:
  podManagementPolicy: OrderedReady
  replicas: 2
  revisionHistoryLimit: 10
  selector:
    matchLabels:
      primus-safe.workload.id: safe-test-abcd
  serviceName: safe-test-abcd
  template:
    metadata:
      labels:
        primus-safe.workload.dispatch.count: "1"
        primus-safe.workload.id: safe-test-abcd
    spec:
      affinity:
        nodeAffinity:
          requiredDuringSchedulingIgnoredDuringExecution:
            nodeSelectorTerms:
            - matchExpressions:
              - key: primus-safe.workspace.id
                operator: In
                values:
                - test-cluster-dev
      containers:
      - command:
        - /bin/sh
        - -c
        - chmod +x /shared-data/launcher.sh; /bin/sh /shared-data/launcher.sh 'abcd'
        env:
        - name: NCCL_SOCKET_IFNAME
          value: eth0
        - name: POOL_SIZE
          value: "60"
        - name: WORKER_SIZE
          value: "8"
        - name: SSH_PORT
          value: "12345"
        image: docker.io/test-image:0.0.1
        imagePullPolicy: IfNotPresent
        name: main
        ports:
        - containerPort: 12346
          protocol: TCP
        - containerPort: 12345
          name: ssh-port
          protocol: TCP
        resources:
          limits:
            cpu: "16"
            ephemeral-storage: 20Gi
            memory: 32Gi
          requests:
            cpu: "16"
            ephemeral-storage: 20Gi
            memory: 32Gi
        securityContext:
          capabilities:
            add:
            - IPC_LOCK
            - SYS_RESOURCE
        terminationMessagePath: /dev/termination-log
        terminationMessagePolicy: File
        volumeMounts:
        - mountPath: /shared-data
          name: shared-data
        - name: shared-memory
          mountPath: /dev/shm
      dnsPolicy: ClusterFirstWithHostNet
      initContainers:
      - command:
        - /bin/sh
        - -c
        - test.sh
        image: docker.io/test-image:0.0.1
        imagePullPolicy: IfNotPresent
        name: prepare
        resources:
          limits:
            cpu: "1"
            memory: 128Mi
        securityContext:
          capabilities:
            add:
            - IPC_LOCK
        terminationMessagePath: /dev/termination-log
        terminationMessagePolicy: File
        volumeMounts:
        - mountPath: /shared-data
          name: shared-data
      terminationGracePeriodSeconds: 10
      volumes:
      - emptyDir:
          medium: Memory
          sizeLimit: 16Gi
        name: shared-memory
      - emptyDir: {}
        name: shared-data
  updateStrategy:
    rollingUpdate:
      partition: 0
    type: RollingUpdate
status:
  availableReplicas: 2
  collisionCount: 0
  currentReplicas: 2
  observedGeneration: 1
  readyReplicas: 2
  replicas: 2
  updatedReplicas: 2
      `

	TestCICDEphemeralRunnerData = `
apiVersion: actions.github.com/v1alpha1
kind: EphemeralRunner
metadata:
  annotations:
    actions.github.com/patch-id: "1622"
    actions.github.com/runner-group-name: Default
    actions.github.com/runner-scale-set-name: primus-safe-cicd-tnznd
    actions.github.com/runner-spec-hash: 599f54dcc4
  creationTimestamp: "2025-12-08T02:18:34Z"
  finalizers:
  - ephemeralrunner.actions.github.com/finalizer
  - ephemeralrunner.actions.github.com/runner-registration-finalizer
  generateName: primus-safe-cicd-tnznd-xvt59-runner-
  generation: 1
  labels:
    actions.github.com/organization: AMD-AGI
    actions.github.com/repository: Primus-SaFE
    actions.github.com/scale-set-name: primus-safe-cicd-tnznd
    actions.github.com/scale-set-namespace: tw-project2-control-plane
    app.kubernetes.io/component: runner
    app.kubernetes.io/part-of: gha-runner-scale-set
    app.kubernetes.io/version: 0.13.0
    primus-safe.workload.dispatch.count: "1"
    primus-safe.workload.id: primus-safe-cicd-tnznd
  name: primus-safe-cicd-tnznd-xvt59-runner-469t5
  namespace: tw-project2-control-plane
  ownerReferences:
  - apiVersion: actions.github.com/v1alpha1
    blockOwnerDeletion: true
    controller: true
    kind: EphemeralRunnerSet
    name: primus-safe-cicd-tnznd-xvt59
    uid: 4a555601-6a79-4e17-b867-2d1cf1dc7049
  resourceVersion: "352896351"
  uid: 06d551f9-1c13-4405-9629-ce714942298a
spec:
  githubConfigSecret: primus-safe-cicd
  githubConfigUrl: https://github.com/AMD-AGI/Primus-SaFE
  runnerScaleSetId: 13
  spec:
    affinity:
      nodeAffinity:
        requiredDuringSchedulingIgnoredDuringExecution:
          nodeSelectorTerms:
          - matchExpressions:
            - key: primus-safe.workspace.id
              operator: In
              values:
              - tw-project2-control-plane
    containers:
    - env:
      - name: RUNNER_ALLOW_RUNASROOT
        value: "1"
      - name: ACTIONS_RUNNER_PRINT_LOG_TO_STDOUT
        value: "1"
      image: docker.io/test-cicd-runner:latest
      name: runner
      resources:
        limits:
          cpu: "1"
          ephemeral-storage: 10Gi
          memory: 4Gi
        requests:
          cpu: "1"
          ephemeral-storage: 10Gi
          memory: 4Gi
      securityContext:
        privileged: true
      volumeMounts:
      - mountPath: /etc/podinfo
        name: podinfo
      - mountPath: /home
        name: hostpath-1
    imagePullSecrets:
    - name: primus-safe-image
    priorityClassName: tw-project2-med-priority
    restartPolicy: Never
    serviceAccountName: gha-rs-manager-no-permission
    volumes:
    - downwardAPI:
        items:
        - fieldRef:
            fieldPath: metadata.labels
          path: labels
        - fieldRef:
            fieldPath: metadata.annotations
          path: annotations
      name: podinfo
    - hostPath:
        path: /home
      name: hostpath-1
status:
  phase: Running
  ready: true
  runnerId: 4345
  runnerName: primus-safe-cicd-tnznd-xvt59-runner-469t5`

	TestAutoscalingRunnerSetData = `
apiVersion: actions.github.com/v1alpha1
kind: AutoscalingRunnerSet
metadata:
  annotations:
    actions.github.com/runner-group-name: Default
    actions.github.com/runner-scale-set-name: primus-safe-cicd-tnznd
    primus-safe.user.name: Wei, Lei
    runner-scale-set-id: "13"
  creationTimestamp: "2025-12-04T08:38:31Z"
  finalizers:
  - autoscalingrunnerset.actions.github.com/finalizer
  generation: 14
  labels:
    actions.github.com/organization: AMD-AGI
    actions.github.com/repository: Primus-SaFE
    app.kubernetes.io/component: autoscaling-runner-set
    app.kubernetes.io/part-of: gha-rs
    app.kubernetes.io/version: 0.13.0
    primus-safe.workload.dispatch.count: "1"
    primus-safe.workload.id: primus-safe-cicd-tnznd
  name: primus-safe-cicd-tnznd
  namespace: tw-project2-control-plane
  resourceVersion: "361771183"
  uid: 22071ca9-1ece-4fe9-8599-39802e752d12
spec:
  githubConfigSecret: primus-safe-cicd
  githubConfigUrl: https://github.com/AMD-AGI/Primus-SaFE
  maxRunners: 20
  minRunners: 0
  template:
    metadata:
      annotations:
        primus-safe.user.name: Wei, Lei
      labels:
        primus-safe.workload.dispatch.count: "1"
        primus-safe.workload.id: primus-safe-cicd-tnznd
    spec:
      affinity:
        nodeAffinity:
          requiredDuringSchedulingIgnoredDuringExecution:
            nodeSelectorTerms:
            - matchExpressions:
              - key: primus-safe.workspace.id
                operator: In
                values:
                - tw-project2-control-plane
      containers:
      - env:
        - name: RUNNER_ALLOW_RUNASROOT
          value: "1"
        - name: ACTIONS_RUNNER_PRINT_LOG_TO_STDOUT
          value: "1"
        - name: APISERVER_NODE_PORT
          value: "32495"
        - name: POD_NAME
          valueFrom:
            fieldRef:
              fieldPath: metadata.name
        - name: USER_ID
          value: 7fda556669b09dcec5d779438e7432c5
        - name: PRIORITY
          value: "1"
        - name: RESOURCES
          value: '[{"replica":1,"cpu":"4","memory":"16Gi","sharedMemory":"8Gi","ephemeralStorage":"50Gi"}]'
        - name: IMAGE
          value: primussafe/buildah-runner:v2.329.0-3
        - name: GITHUB_CONFIG_URL
          value: https://github.com/AMD-AGI/Primus-SaFE
        - name: UNIFIED_JOB_ENABLE
          value: "false"
        - name: SCALE_RUNNER_SET_ID
          value: primus-safe-cicd-tnznd
        - name: WORKSPACE_ID
          value: tw-project2-control-plane
        - name: ENTRYPOINT
          value: ZXhlYyAvaG9tZS9ydW5uZXIvYWN0aW9ucy1ydW5uZXIvcnVuLnNo
        - name: WORKLOAD_ID
          value: primus-safe-cicd-tnznd
        - name: WORKLOAD_KIND
          value: AutoscalingRunnerSet
        - name: DISPATCH_COUNT
          value: "1"
        - name: GITHUB_SECRET_ID
          value: primus-safe-cicd
        - name: ADMIN_CONTROL_PLANE
          value: 10.32.80.102
        image: harbor.tw325.primus-safe.amd.com/proxy/primussafe/cicd-runner-proxy:202512111845
        name: runner
        resources:
          limits:
            cpu: "1"
            ephemeral-storage: 10Gi
            memory: 4Gi
          requests:
            cpu: "1"
            ephemeral-storage: 10Gi
            memory: 4Gi
        securityContext:
          privileged: true
        volumeMounts:
        - mountPath: /etc/podinfo
          name: podinfo
        - mountPath: /home
          name: hostpath-1
          readOnly: false
        - mountPath: /etc/secrets/primus-safe-cicd
          name: primus-safe-cicd
          readOnly: true
      hostNetwork: false
      imagePullSecrets:
      - name: rocm-private
      - name: primus-safe-image
      - name: tas-private
      priorityClassName: tw-project2-med-priority
      restartPolicy: Never
      serviceAccountName: gha-rs-manager-no-permission
      volumes:
      - downwardAPI:
          items:
          - fieldRef:
              fieldPath: metadata.labels
            path: labels
          - fieldRef:
              fieldPath: metadata.annotations
            path: annotations
        name: podinfo
      - hostPath:
          path: /home
        name: hostpath-1
      - name: primus-safe-cicd
        secret:
          secretName: primus-safe-cicd
status:
  currentRunners: 0
  pendingEphemeralRunners: 0
  runningEphemeralRunners: 0
`
)

var (
	TestPytorchResourceTemplate = &v1.ResourceTemplate{
		ObjectMeta: metav1.ObjectMeta{
			Name: "pytorch-job",
			Labels: map[string]string{
				v1.WorkloadVersionLabel: "v1",
			},
			Annotations: map[string]string{
				v1.WorkloadKindLabel: "PyTorchJob",
			},
		},
		Spec: v1.ResourceTemplateSpec{
			GroupVersionKind: v1.GroupVersionKind{
				Group:   "kubeflow.org",
				Version: "v1",
				Kind:    "PyTorchJob",
			},
			ResourceSpecs: []v1.ResourceSpec{{
				PrePaths:      []string{"spec", "pytorchReplicaSpecs", "Master"},
				TemplatePaths: []string{"template"},
				ReplicasPaths: []string{"replicas"},
			}, {
				PrePaths:      []string{"spec", "pytorchReplicaSpecs", "Worker"},
				TemplatePaths: []string{"template"},
				ReplicasPaths: []string{"replicas"},
			}},
			ResourceStatus: v1.ResourceStatus{
				PrePaths:     []string{"status", "conditions"},
				MessagePaths: []string{"message"},
				ReasonPaths:  []string{"reason"},
				Phases: []v1.PhaseExpression{{
					MatchExpressions: map[string]string{
						"type":   "Succeeded",
						"status": "True",
					},
					Phase: "K8sSucceeded",
				}, {
					MatchExpressions: map[string]string{
						"type":   "Failed",
						"status": "True",
					},
					Phase: "K8sFailed",
				}, {
					MatchExpressions: map[string]string{
						"type":   "FailedValidation",
						"status": "True",
					},
					Phase: "K8sFailed",
				}, {
					MatchExpressions: map[string]string{
						"type":   "Running",
						"status": "True",
					},
					Phase: "K8sRunning",
				}},
			},
			ActiveReplica: v1.ActiveReplica{
				PrePaths:    []string{"status", "replicaStatuses"},
				ReplicaPath: "active",
			},
		},
	}

	TestJobResourceTemplate = &v1.ResourceTemplate{
		ObjectMeta: metav1.ObjectMeta{
			Name: "job",
			Labels: map[string]string{
				v1.WorkloadVersionLabel: "v1",
			},
			Annotations: map[string]string{
				v1.WorkloadKindLabel: common.JobKind,
			},
		},
		Spec: v1.ResourceTemplateSpec{
			GroupVersionKind: v1.GroupVersionKind{
				Group:   "batch",
				Version: "v1",
				Kind:    common.JobKind,
			},
			ResourceSpecs: []v1.ResourceSpec{{
				PrePaths:         []string{"spec"},
				TemplatePaths:    []string{"template"},
				ReplicasPaths:    []string{"parallelism"},
				CompletionsPaths: []string{"completions"},
			}},
			ResourceStatus: v1.ResourceStatus{
				PrePaths:     []string{"status", "conditions"},
				MessagePaths: []string{"message"},
				ReasonPaths:  []string{"reason"},
				Phases: []v1.PhaseExpression{{
					MatchExpressions: map[string]string{
						"type":   "Complete",
						"status": "True",
					},
					Phase: "K8sSucceeded",
				}, {
					MatchExpressions: map[string]string{
						"type":   "Failed",
						"status": "True",
					},
					Phase: "K8sFailed",
				}},
			},
			ActiveReplica: v1.ActiveReplica{
				PrePaths:    []string{"status"},
				ReplicaPath: "active",
			},
		},
	}

	TestDeploymentResourceTemplate = &v1.ResourceTemplate{
		ObjectMeta: metav1.ObjectMeta{
			Name: "deployment",
			Labels: map[string]string{
				v1.WorkloadVersionLabel: "v1",
			},
			Annotations: map[string]string{
				v1.WorkloadKindLabel: "Deployment",
			},
		},
		Spec: v1.ResourceTemplateSpec{
			GroupVersionKind: v1.GroupVersionKind{
				Group:   "apps",
				Version: "v1",
				Kind:    "Deployment",
			},
			ResourceSpecs: []v1.ResourceSpec{{
				PrePaths:      []string{"spec"},
				TemplatePaths: []string{"template"},
				ReplicasPaths: []string{"replicas"},
			}},
			ResourceStatus: v1.ResourceStatus{
				PrePaths:     []string{"status", "conditions"},
				MessagePaths: []string{"message"},
				ReasonPaths:  []string{"reason"},
				Phases: []v1.PhaseExpression{{
					MatchExpressions: map[string]string{
						"type":   "Available",
						"status": "False",
					},
					Phase: string(v1.K8sFailed),
				}, {
					MatchExpressions: map[string]string{
						"type":   "Progressing",
						"status": "True",
						"reason": "ReplicaSetUpdated",
					},
					Phase: string(v1.K8sUpdating),
				}, {
					MatchExpressions: map[string]string{
						"type":   "Available",
						"status": "True",
					},
					Phase: string(v1.K8sRunning),
				}},
			},
			ActiveReplica: v1.ActiveReplica{
				PrePaths:    []string{"status"},
				ReplicaPath: "availableReplicas",
			},
		},
	}

	TestStatefulSetResourceTemplate = &v1.ResourceTemplate{
		ObjectMeta: metav1.ObjectMeta{
			Name: "statefulset",
			Labels: map[string]string{
				v1.WorkloadVersionLabel: "v1",
			},
			Annotations: map[string]string{
				v1.WorkloadKindLabel: "StatefulSet",
			},
		},
		Spec: v1.ResourceTemplateSpec{
			GroupVersionKind: v1.GroupVersionKind{
				Group:   "apps",
				Version: "v1",
				Kind:    "StatefulSet",
			},
			ResourceSpecs: []v1.ResourceSpec{{
				PrePaths:      []string{"spec"},
				TemplatePaths: []string{"template"},
				ReplicasPaths: []string{"replicas"},
			}},
			ActiveReplica: v1.ActiveReplica{
				PrePaths:    []string{"status"},
				ReplicaPath: "availableReplicas",
			},
		},
	}

	TestWorkspaceData = &v1.Workspace{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "amd.com/v1",
			Kind:       "Workspace",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-workspace-abc12",
			Labels: map[string]string{
				v1.DisplayNameLabel: "test-workspace",
				v1.ClusterIdLabel:   "test-cluster",
			},
		},
		Spec: v1.WorkspaceSpec{
			Cluster:    "test-cluster",
			Replica:    3,
			NodeFlavor: "nf1",
			Volumes: []v1.WorkspaceVolume{{
				Id:           1,
				Type:         v1.PFS,
				MountPath:    "/ceph",
				StorageClass: "storage-cephfs",
				Capacity:     "100Gi",
			}, {
				Id:        2,
				Type:      v1.HOSTPATH,
				MountPath: "/data",
				HostPath:  "/apps",
			}},
			ImageSecrets: []corev1.ObjectReference{{
				Name: "test-image",
			}},
		},
		Status: v1.WorkspaceStatus{
			Phase: v1.WorkspaceRunning,
			TotalResources: corev1.ResourceList{
				corev1.ResourceCPU:              *resource.NewQuantity(50, resource.DecimalSI),
				corev1.ResourceMemory:           *resource.NewQuantity(1024*1024*1024*512, resource.BinarySI),
				common.NvidiaGpu:                *resource.NewQuantity(8, resource.DecimalSI),
				corev1.ResourceEphemeralStorage: *resource.NewQuantity(1024*1024*1024*128, resource.BinarySI),
			},
			AvailableResources: corev1.ResourceList{
				corev1.ResourceCPU:              *resource.NewQuantity(50, resource.DecimalSI),
				corev1.ResourceMemory:           *resource.NewQuantity(1024*1024*1024*512, resource.BinarySI),
				common.NvidiaGpu:                *resource.NewQuantity(8, resource.DecimalSI),
				corev1.ResourceEphemeralStorage: *resource.NewQuantity(1024*1024*1024*128, resource.BinarySI),
			},
		},
	}

	TestCICDScaleSetResourceTemplate = &v1.ResourceTemplate{
		ObjectMeta: metav1.ObjectMeta{
			Name: "job",
			Labels: map[string]string{
				v1.WorkloadVersionLabel: "v1",
			},
			Annotations: map[string]string{
				v1.WorkloadKindLabel: common.CICDScaleRunnerSetKind,
			},
		},
		Spec: v1.ResourceTemplateSpec{
			GroupVersionKind: v1.GroupVersionKind{
				Group:   "actions.github.com",
				Version: "v1alpha1",
				Kind:    "AutoscalingRunnerSet",
			},
			ResourceSpecs: []v1.ResourceSpec{{
				PrePaths:      []string{"spec"},
				TemplatePaths: []string{"template"},
			}},
		},
	}

	TestCICDRunnerResourceTemplate = &v1.ResourceTemplate{
		ObjectMeta: metav1.ObjectMeta{
			Name: "job",
			Labels: map[string]string{
				v1.WorkloadVersionLabel: "v1",
			},
			Annotations: map[string]string{
				v1.WorkloadKindLabel: common.CICDEphemeralRunnerKind,
			},
		},
		Spec: v1.ResourceTemplateSpec{
			GroupVersionKind: v1.GroupVersionKind{
				Group:   "actions.github.com",
				Version: "v1alpha1",
				Kind:    "EphemeralRunner",
			},
			ResourceSpecs: []v1.ResourceSpec{{
				PrePaths: []string{"spec"},
			}},
			ResourceStatus: v1.ResourceStatus{
				PrePaths:     []string{"status"},
				MessagePaths: []string{"message"},
				ReasonPaths:  []string{"reason"},
				Phases: []v1.PhaseExpression{{
					MatchExpressions: map[string]string{
						"phase": "Running",
					},
					Phase: string(v1.K8sRunning),
				}, {
					MatchExpressions: map[string]string{
						"phase": "Failed",
					},
					Phase: string(v1.K8sFailed),
				}, {
					MatchExpressions: map[string]string{
						"phase": "Succeeded",
					},
					Phase: string(v1.K8sSucceeded),
				}, {
					MatchExpressions: map[string]string{
						"phase": "Pending",
					},
					Phase: string(v1.K8sPending),
				}},
			},
		},
	}

	TestWorkloadData = &v1.Workload{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-workload-abcde",
			Labels: map[string]string{
				v1.ClusterIdLabel:   "test-cluster",
				v1.DisplayNameLabel: "test-workload",
			},
			Annotations: map[string]string{
				v1.UserNameAnnotation: "test-user",
				"key":                 "val",
			},
			CreationTimestamp: metav1.NewTime(time.Now()),
		},
		Spec: v1.WorkloadSpec{
			Workspace: "test-workspace",
			MaxRetry:  2,
			Priority:  2,
			JobPort:   12345,
			SSHPort:   23456,
			GroupVersionKind: v1.GroupVersionKind{
				Version: "v1",
				Kind:    "PyTorchJob",
			},
			Resources: []v1.WorkloadResource{{
				Replica:          1,
				CPU:              "32",
				GPU:              "4",
				GPUName:          "amd.com/gpu",
				Memory:           "256Gi",
				SharedMemory:     "32Gi",
				EphemeralStorage: "20Gi",
				RdmaResource:     "1k",
			}},
			Images:      []string{"test-image"},
			EntryPoints: []string{"sh -c test.sh"},
			Env: map[string]string{
				"key": "value",
			},
			CustomerLabels: map[string]string{
				"key1": "val1",
				"key2": "val2",
			},
		},
	}

	TestNodeFlavorData = &v1.NodeFlavor{
		ObjectMeta: metav1.ObjectMeta{
			Name: "nodeflavor",
		},
		Spec: v1.NodeFlavorSpec{
			Cpu: v1.CpuChip{
				Quantity: *resource.NewQuantity(64, resource.DecimalSI),
			},
			Memory: *resource.NewQuantity(1024*1024*1024, resource.BinarySI),
			Gpu: &v1.GpuChip{
				ResourceName: common.AmdGpu,
				Quantity:     *resource.NewQuantity(8, resource.DecimalSI),
			},
		},
	}
)
