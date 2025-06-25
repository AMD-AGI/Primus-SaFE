/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
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
              name: sugaku-volume
    Worker:
      replicas: 63
      restartPolicy: Never
      template:
        spec:
          containers:
            - command:
                - sh
                - -c
                - ./test.sh
              env:
                - name: NCCL_SOCKET_IFNAME
                  value: eth0
              image: /docker.hub/test-image:0.0.1
              name: pytorch
              resources:
                limits:
                  cpu: "48"
                  memory: 960Gi
                  amd.com/gpu: "8"
                  rdma/hca: "1"
              volumeMounts:
                - mountPath: /pfs
                  name: pfs
                - name: sugaku-volume
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
              name: sugaku-volume
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
              image: /docker.hub/test-image:0.0.1
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
  completions: 1
  parallelism: 2
  suspend: false
  template:
    metadata:
      creationTimestamp: null
      labels:
        primus-safe.workload.id: test-abcd
        job-name: test-abcd
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
        image: /docker.hub/test-image:0.0.1
        imagePullPolicy: IfNotPresent
        name: job
        resources:
          limits:
            cpu: "1"
            memory: 100Mi
        terminationMessagePath: /dev/termination-log
        terminationMessagePolicy: File
      dnsPolicy: ClusterFirst
      restartPolicy: Never
      schedulerName: default-scheduler
      securityContext: {}
      terminationGracePeriodSeconds: 30
status:
  active: 2
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
            - c
            - chmod +x /shared-data/launcher.sh; /bin/sh /shared-data/launcher.sh 'abcd'
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
      volumes:
        - emptyDir:
            medium: Memory
            sizeLimit: 20Gi
          name: sugaku-volume
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
        image: /docker.hub/test-image:0.0.1
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
      dnsPolicy: ClusterFirstWithHostNet
      imagePullSecrets:
      - name: primus-safe-image
      initContainers:
      - command:
        - /bin/sh
        - -c
        - test.sh
        image: /docker.hub/test-image:0.0.1
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
        name: sugaku-volume
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
)

var (
	TestPytorchResourceTemplate = &v1.ResourceTemplate{
		ObjectMeta: metav1.ObjectMeta{
			Name: "pytorch-job",
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
				Replica:       1,
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

	TestJobTemplate = &v1.ResourceTemplate{
		ObjectMeta: metav1.ObjectMeta{
			Name: "job",
		},
		Spec: v1.ResourceTemplateSpec{
			GroupVersionKind: v1.GroupVersionKind{
				Group:   "batch",
				Version: "v1",
				Kind:    "Job",
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

	TestDeploymentTemplate = &v1.ResourceTemplate{
		ObjectMeta: metav1.ObjectMeta{
			Name: "deployment",
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

	TestPodTemplate = &v1.ResourceTemplate{
		ObjectMeta: metav1.ObjectMeta{
			Name: "pod",
		},
		Spec: v1.ResourceTemplateSpec{
			GroupVersionKind: v1.GroupVersionKind{
				Group:   "",
				Version: "v1",
				Kind:    "Pod",
			},
		},
	}

	TestStatefulSetTemplate = &v1.ResourceTemplate{
		ObjectMeta: metav1.ObjectMeta{
			Name: "statefulset",
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
				StorageType:  v1.FS,
				MountPath:    "/ceph",
				StorageClass: "storage-cephfs",
				Capacity:     "100Gi",
			}, {
				StorageType: v1.HOSTPATH,
				MountPath:   "/data",
				HostPath:    "/apps",
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

	TestWorkloadData = &v1.Workload{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-workload",
			Labels: map[string]string{
				v1.ClusterIdLabel: "test-cluster",
			},
			CreationTimestamp: metav1.NewTime(time.Now()),
		},
		Spec: v1.WorkloadSpec{
			Workspace:  "test-workspace",
			MaxRetry:   2,
			Priority:   2,
			Image:      "test-image",
			EntryPoint: "sh -c test.sh",
			GroupVersionKind: v1.GroupVersionKind{
				Group:   "kubeflow.org",
				Version: "v1",
				Kind:    "PyTorchJob",
			},
			Resource: v1.WorkloadResource{
				Replica:          1,
				CPU:              "32",
				GPU:              "4",
				GPUName:          "amd.com/gpu",
				Memory:           "256Gi",
				ShareMemory:      "32Gi",
				EphemeralStorage: "20Gi",
				JobPort:          12345,
			},
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
