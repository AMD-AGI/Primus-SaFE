/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package v1

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type WorkloadPhase string

const (
	WorkloadKind = "Workload"

	MaxPriority = 2
	MinPriority = 0

	WorkloadSucceeded WorkloadPhase = "Succeeded"
	WorkloadFailed    WorkloadPhase = "Failed"
	WorkloadPending   WorkloadPhase = "Pending"
	WorkloadRunning   WorkloadPhase = "Running"
	// only for deployment/statefulSet
	WorkloadUpdating WorkloadPhase = "Updating"
	// only for deployment/statefulSet
	WorkloadNotReady WorkloadPhase = "NotReady"
	WorkloadStopped  WorkloadPhase = "Stopped"
	WorkloadStopping WorkloadPhase = "Stopping"
)

type WorkloadConditionType string

const (
	AdminScheduled  WorkloadConditionType = "AdminScheduled"
	AdminDispatched WorkloadConditionType = "AdminDispatched"
	K8sPending      WorkloadConditionType = "K8sPending"
	K8sSucceeded    WorkloadConditionType = "K8sSucceeded"
	K8sFailed       WorkloadConditionType = "K8sFailed"
	K8sRunning      WorkloadConditionType = "K8sRunning"
	K8sUpdating     WorkloadConditionType = "K8sUpdating"
	K8sDeleted      WorkloadConditionType = "K8sDeleted"
	AdminFailover   WorkloadConditionType = "AdminFailover"
	AdminFailed     WorkloadConditionType = "AdminFailed"
	AdminStopped    WorkloadConditionType = "AdminStopped"
	AdminStopping   WorkloadConditionType = "AdminStopping"
	AdminStart      WorkloadConditionType = "AdminStart"
	AdminRestart    WorkloadConditionType = "AdminRestart"
)

type WorkloadResource struct {
	// Valid only for PyTorchJob jobs; values can be "master" or "worker". Optional for other job types.
	Role string `json:"role,omitempty"`
	// Number of requested nodes
	Replica int `json:"replica"`
	// Requested CPU core count (e.g., 128)
	CPU string `json:"cpu"`
	// Requested GPU card count (e.g., 8)
	GPU string `json:"gpu,omitempty"`
	// This field is set internally to match the resource supported by the workspace. e.g. amd.com/gpu
	GPUName string `json:"-,omitempty"`
	// Requested Memory size (e.g., 128Gi)
	Memory string `json:"memory"`
	// Requested Share Memory size (e.g., 128Gi). default: Memory/2
	ShareMemory string `json:"shareMemory,omitempty"`
	// ephemeral-storage for pod，default: 50Gi
	EphemeralStorage string `json:"ephemeralStorage,omitempty"`
	// the port for job
	JobPort int `json:"jobPort,omitempty"`
	// the port for ssh
	SSHPort int `json:"SSHPort,omitempty"`
}

type HealthCheck struct {
	// the path for health check
	Path string `json:"path"`
	// Service port for health detect
	Port int `json:"port"`
	// initial delay seconds. default: 600s
	InitialDelaySeconds int `json:"initialDelaySeconds,omitempty"`
	// period check interval. default: 3s
	PeriodSeconds int `json:"periodSeconds,omitempty"`
	// Failure retry limit. default: 3
	FailureThreshold int `json:"failureThreshold,omitempty"`
}

type Service struct {
	// TCP/UDP
	Protocol corev1.Protocol `json:"protocol"`
	// Service port for external access
	Port int `json:"port"`
	// k8s node port
	NodePort int `json:"nodePort,omitempty"`
	// Pod service listening port
	TargetPort int `json:"targetPort"`
	// ClusterIP/NodePort
	ServiceType corev1.ServiceType `json:"serviceType"`
	// Extended environment variable
	Extends map[string]string `json:"extends,omitempty"`
}

type WorkloadSpec struct {
	// workload resource requirements. Only PyTorchJob uses multiple resource entries
	Resources []WorkloadResource `json:"resources"`
	// requested workspace
	Workspace string `json:"workspace,omitempty"`
	// task image address
	Image string `json:"image,omitempty"`
	// task entryPoint, required in base64 encoding
	EntryPoint string `json:"entryPoint,omitempty"`
	// environment variable for task
	Env map[string]string `json:"env,omitempty"`
	// whether ssh is enabled
	IsSSHEnabled bool `json:"isSSHEnabled,omitempty"`
	// Supervision flag for the task. When enabled, it performs operations like hang detection
	IsSupervised bool `json:"isSupervised,omitempty"`
	// task define
	GroupVersionKind `json:"gvk,omitempty"`
	// Failure retry limit. default: 0
	MaxRetry int `json:"maxRetry,omitempty"`
	// Task scheduling priority. Defaults to 0; valid range: 0–2
	Priority int `json:"priority,omitempty"`
	// The lifecycle of the task after completion, in seconds. Default to 60.
	TTLSecondsAfterFinished int `json:"ttlSecondsAfterFinished,omitempty"`
	// Task timeout in hours. Default is 0 (no timeout).
	Timeout *int `json:"timeout,omitempty"`
	// The task will run on nodes with the user-specified labels.
	// If multiple labels are specified, all of them must be satisfied.
	CustomerLabels map[string]string `json:"customerLabels,omitempty"`
	// k8s liveness check
	Liveness *HealthCheck `json:"liveness,omitempty"`
	// k8s readiness check
	Readiness *HealthCheck `json:"readiness,omitempty"`
	// service configuration. used for deployment/statefulSet
	Service *Service `json:"service,omitempty"`
}

type WorkloadStatus struct {
	// Task start time
	StartTime *metav1.Time `json:"startTime,omitempty"`
	// Task end time
	EndTime *metav1.Time `json:"endTime,omitempty"`
	// Some status descriptions of the task
	Conditions []metav1.Condition `json:"conditions,omitempty"`
	// Pending，Running，Succeeded，Failed, Stopped, Error
	Phase WorkloadPhase `json:"phase,omitempty"`
	// Some status descriptions of the task. only for pending
	Message string `json:"message,omitempty"`
	// The current position of the task in the queue
	SchedulerOrder int `json:"schedulerOrder,omitempty"`
	// Pod info related to the task
	Pods []WorkloadPod `json:"pods,omitempty"`
	// The node used for each task execution. If the task is retried multiple times, there will be multiple entries.
	Nodes [][]string `json:"nodes,omitempty"`
}

type WorkloadPod struct {
	// podId
	PodId string `json:"podId"`
	// role，master/worker
	Role string `json:"role,omitempty"`
	// the Kubernetes node that the Pod is scheduled on
	K8sNodeName string `json:"k8sNodeName,omitempty"`
	// the admin node that the Pod is scheduled on
	AdminNodeName string `json:"adminNodeName,omitempty"`
	// pod status：Pending, Running, Succeeded, Failed, Unknown
	Phase corev1.PodPhase `json:"phase,omitempty"`
	// The node's IP address where the Pod is running
	HostIp string `json:"hostIP,omitempty"`
	// pod start time
	StartTime string `json:"startTime,omitempty"`
	// pod end time
	EndTime string `json:"endTime,omitempty"`
	// error message
	Message *PodFailedMessage `json:"message,omitempty"`
}

type ContainerFailedMessage struct {
	Name     string `json:"name"`
	Reason   string `json:"reason,omitempty"`
	Message  string `json:"message,omitempty"`
	ExitCode int32  `json:"exitCode"`
	Signal   int32  `json:"signal"`
	Analysis string `json:"analysis,omitempty"`
}

type PodFailedMessage struct {
	Message    string                   `json:"message,omitempty"`
	Containers []ContainerFailedMessage `json:"containers,omitempty"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:webhook:path=/mutate-amd-primus-safe-v1-workload,mutating=true,failurePolicy=fail,sideEffects=None,groups=amd.com,resources=workloads,verbs=create;update,versions=v1,name=mworkload.kb.io,admissionReviewVersions={v1,v1beta1}
// +kubebuilder:webhook:path=/validate-amd-primus-safe-v1-workload,mutating=false,failurePolicy=fail,sideEffects=None,groups=amd.com,resources=workloads,verbs=create;update,versions=v1,name=vworkload.kb.io,admissionReviewVersions={v1,v1beta1}
// +kubebuilder:rbac:groups=amd.com,resources=workloads,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=amd.com,resources=workloads/status,verbs=get;update;patch
type Workload struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   WorkloadSpec   `json:"spec,omitempty"`
	Status WorkloadStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type WorkloadList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Workload `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Workload{}, &WorkloadList{})
}

func (w *Workload) IsPending() bool {
	if w.Status.Phase == "" || w.Status.Phase == WorkloadPending {
		return true
	}
	return false
}

func (w *Workload) IsRunning() bool {
	if w.Status.Phase == WorkloadRunning {
		return true
	}
	return false
}

func (w *Workload) IsStopped() bool {
	if w.Status.Phase == WorkloadStopped {
		return true
	}
	return false
}

func (w *Workload) IsStopping() bool {
	if w.Status.Phase == WorkloadStopping {
		return true
	}
	return false
}

func (w *Workload) IsEnd() bool {
	if w.Status.Phase == WorkloadSucceeded ||
		w.Status.Phase == WorkloadFailed {
		return true
	}
	if !w.GetDeletionTimestamp().IsZero() {
		return true
	}
	return false
}

func (w *Workload) CostTime() int64 {
	var costTime time.Duration
	if w.IsEnd() {
		if w.Status.EndTime == nil {
			return 0
		}
		costTime = w.Status.EndTime.Time.Sub(w.CreationTimestamp.Time)
	} else {
		costTime = time.Now().UTC().Sub(w.CreationTimestamp.Time)
	}
	return int64(costTime.Seconds())
}

func (w *Workload) ResourceGVK() schema.GroupVersionKind {
	return schema.GroupVersionKind{
		Group:   w.Spec.Group,
		Version: w.Spec.Version,
		Kind:    w.Spec.Kind,
	}
}

func (w *Workload) ResourceApiVersion() string {
	return w.Spec.Group + "/" + w.Spec.Version
}

func (w *Workload) IsTimeout() bool {
	if w.GetTimeout() <= 0 || w.Status.StartTime == nil {
		return false
	}
	duration := int(time.Now().UTC().Sub(w.Status.StartTime.Time).Hours())
	return duration >= *w.Spec.Timeout
}

func (w *Workload) GetTimeout() int {
	if w.Spec.Timeout == nil {
		return 0
	}
	return *w.Spec.Timeout
}

func (w *Workload) GetLastCond() *metav1.Condition {
	l := len(w.Status.Conditions)
	if l == 0 {
		return nil
	}
	return &w.Status.Conditions[l-1]
}

func IsPodRunning(p *WorkloadPod) bool {
	return corev1.PodSucceeded != p.Phase &&
		corev1.PodFailed != p.Phase &&
		p.K8sNodeName != ""
}
