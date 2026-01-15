/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package v1

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type (
	WorkloadPhase string
	CronAction    string
)

const (
	WorkloadKind = "Workload"

	WorkloadSucceeded WorkloadPhase = "Succeeded"
	WorkloadFailed    WorkloadPhase = "Failed"
	WorkloadPending   WorkloadPhase = "Pending"
	WorkloadRunning   WorkloadPhase = "Running"
	// only for deployment/statefulSet
	WorkloadUpdating WorkloadPhase = "Updating"
	// only for deployment/statefulSet/AutoscalingRunnerSet
	WorkloadNotReady WorkloadPhase = "NotReady"
	WorkloadStopped  WorkloadPhase = "Stopped"

	CronStart CronAction = "start"
	CronScale CronAction = "scale"
)

type WorkloadConditionType string

const (
	AdminScheduling WorkloadConditionType = "AdminScheduling"
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
)

type WorkloadResource struct {
	// Number of requested nodes
	Replica int `json:"replica"`
	// Requested CPU core count (e.g., 128)
	CPU string `json:"cpu"`
	// Requested GPU card count (e.g., 8)
	GPU string `json:"gpu,omitempty"`
	// This field is set internally. e.g. amd.com/gpu
	GPUName string `json:"gpuName,omitempty"`
	// Requested Memory size (e.g., 128Gi)
	Memory string `json:"memory"`
	// Requested Shared Memory size (e.g., 128Gi). Used for sharing data between processes. default: Memory/2
	SharedMemory string `json:"sharedMemory,omitempty"`
	// Ephemeral-storage for pod. default 50Gi
	EphemeralStorage string `json:"ephemeralStorage,omitempty"`
	// RDMA resource is effective only with hostNetwork enabled (default: 1).
	// This field is set internally
	RdmaResource string `json:"rdmaResource,omitempty"`
	// The address of the image used by the workload
	Image string `json:"image,omitempty"`
	// Startup command, required in base64 encoding
	EntryPoint string `json:"entryPoint,omitempty"`
}

type HealthCheck struct {
	// Liveness probe HTTP path
	Path string `json:"path"`
	// Liveness probe port
	Port int `json:"port"`
	// Initial delay seconds. default 600s
	InitialDelaySeconds int `json:"initialDelaySeconds,omitempty"`
	// Period check interval. default 3s
	PeriodSeconds int `json:"periodSeconds,omitempty"`
	// Failure retry limit. default 3
	FailureThreshold int `json:"failureThreshold,omitempty"`
}

type Service struct {
	// Service protocol, e.g. TCP/UDP, default TCP
	Protocol corev1.Protocol `json:"protocol"`
	// Service port for external access, Defaults to targetPort.
	Port int `json:"port,omitempty"`
	// Port of Host Node (for NodePort type)
	NodePort int `json:"nodePort,omitempty"`
	// Target container port
	TargetPort int `json:"targetPort"`
	// Service type, e.g. ClusterIP/NodePort
	ServiceType corev1.ServiceType `json:"serviceType"`
	// Extended environment variable
	Extends map[string]string `json:"extends,omitempty"`
}

type CronJob struct {
	// Scheduled execution time, e.g. "2025-09-30T16:04:00.000Z" or "0 3 * * *"
	// Note: Only minute-level input is supported; seconds are not supported.
	Schedule string `json:"schedule"`
	// The action to take when the schedule is triggered. e.g. start or scale
	Action CronAction `json:"action"`
}

type SecretEntity struct {
	// Secret id, required
	Id string `json:"id"`
	// Secret type, optional. e.g. ssh/image/general
	Type SecretType `json:"type,omitempty"`
}

type WorkloadSpec struct {
	// Deprecated: resource is old field, will be replaced by Resources
	Resource WorkloadResource `json:"resource,omitempty"`
	// Resource requirements, It may involve multiple resources, e.g., a PyTorchJob with master and worker roles.
	Resources []WorkloadResource `json:"resources,omitempty"`
	// Requested workspace id
	Workspace string `json:"workspace"`
	// Deprecated: resource is old field, will be replaced by Images
	Image string `json:"image,omitempty"`
	// The address of the image used by the workload
	// It must match the length of resources.
	Images []string `json:"images,omitempty"`
	// Deprecated: resource is old field, will be replaced by Images
	EntryPoint string `json:"entryPoint,omitempty"`
	// Workload startup command, required in base64 encoding
	// It must match the length of resources.
	EntryPoints []string `json:"entryPoints,omitempty"`
	// The port for pytorch-job, This field is set internally
	JobPort int `json:"jobPort,omitempty"`
	// The port for ssh, This field is set internally
	SSHPort int `json:"sshPort,omitempty"`
	// Environment variable for workload
	Env map[string]string `json:"env,omitempty"`
	// Supervision flag for the workload. When enabled, it performs operations like hang detection
	IsSupervised bool `json:"isSupervised,omitempty"`
	// Group: An extension field that is not currently in use
	// Version: version of workload, default value is v1
	// Kind: kind of workload, Valid values includes: PyTorchJob/Deployment/StatefulSet/Authoring/AutoscalingRunnerSet, default PyTorchJob
	// AutoscalingRunnerSet is a CI/CD configuration, and if enabled, it requires NFS storage support.
	GroupVersionKind `json:"groupVersionKind"`
	// Failure retry limit. default: 0
	MaxRetry int `json:"maxRetry,omitempty"`
	// Workload scheduling priority. Defaults is 0, valid range: 0–2
	Priority int `json:"priority"`
	// The lifecycle of the workload after completion, in seconds. default 60.
	TTLSecondsAfterFinished *int `json:"ttlSecondsAfterFinished,omitempty"`
	// Workload timeout in seconds. default 0 (no timeout).
	// The timeout is calculated from the moment the workload is dispatched.
	// If the workload remains in the queue (not yet dispatched), the timeout is not considered.
	Timeout *int `json:"timeout,omitempty"`
	// The workload will run on nodes with the user-specified labels.
	// If multiple labels are specified, all of them must be satisfied.
	CustomerLabels map[string]string `json:"customerLabels,omitempty"`
	// K8s liveness check. used for deployment/statefulSet
	Liveness *HealthCheck `json:"liveness,omitempty"`
	// K8s readiness check. used for deployment/statefulSet
	Readiness *HealthCheck `json:"readiness,omitempty"`
	// Service configuration
	Service *Service `json:"service,omitempty"`
	// Indicates whether the workload tolerates node taints
	IsTolerateAll bool `json:"isTolerateAll,omitempty"`
	// The workload will automatically mount the volumes defined in the workspace,
	// and you can also define specific hostPath for mounting.
	Hostpath []string `json:"hostpath,omitempty"`
	// Dependencies defines a list of other Workloads that must complete successfully
	// before this Workload can start execution. If any dependency fails, this Workload
	// will not be scheduled and is considered failed.
	Dependencies []string `json:"dependencies,omitempty"`
	// Cron Job configuration
	CronJobs []CronJob `json:"cronJobs,omitempty"`
	// The secrets used by the workload. Including some token secrets (only for CI/CD) and specified image secrets.
	// Image secrets automatically use all image secrets bound to the workspace.
	Secrets []SecretEntity `json:"secrets,omitempty"`
}

type WorkloadStatus struct {
	// Workload start time
	StartTime *metav1.Time `json:"startTime,omitempty"`
	// Workload end time
	EndTime *metav1.Time `json:"endTime,omitempty"`
	// Detailed processing workflow of the workload
	Conditions []metav1.Condition `json:"conditions,omitempty"`
	// The status of workload, e.g. Pending, Running, Succeeded, Failed, Stopped, Updating
	Phase WorkloadPhase `json:"phase,omitempty"`
	// Some status descriptions of the workload. only for pending
	Message string `json:"message,omitempty"`
	// The current position of the workload in the queue, only for pending
	QueuePosition int `json:"queuePosition,omitempty"`
	// Pod info related to the workload
	Pods []WorkloadPod `json:"pods,omitempty"`
	// The node used for each workload execution. If the workload is retried multiple times, there will be multiple entries.
	Nodes [][]string `json:"nodes,omitempty"`
	// The node's rank is only valid for the PyTorch job and corresponds one-to-one with the nodes listed above.
	Ranks [][]string `json:"ranks,omitempty"`
	// The corresponding ID applied to the cicd AutoscalingRunnerSet object.
	RunnerScaleSetId string `json:"runnerScaleSetId,omitempty"`
	// The phase of each dependency workload.
	DependenciesPhase map[string]WorkloadPhase `json:"dependenciesPhase,omitempty"`
	// The phase of each torchFT object.
	TorchFTPhase map[string]WorkloadPhase `json:"torchFTPhase,omitempty"`
}

type WorkloadPod struct {
	// The podId
	PodId string `json:"podId"`
	// The id of workload resources that the pod is bound to
	ResourceId int `json:"resourceId,omitempty"`
	// The Kubernetes node that the Pod is scheduled on
	K8sNodeName string `json:"k8sNodeName,omitempty"`
	// The admin node that the Pod is scheduled on
	AdminNodeName string `json:"adminNodeName,omitempty"`
	// Pod status: Pending, Running, Succeeded, Failed, Unknown
	Phase corev1.PodPhase `json:"phase,omitempty"`
	// The node's IP address where the Pod is running
	HostIp string `json:"hostIP,omitempty"`
	// The pod's IP address where the Pod is running
	PodIp string `json:"podIP,omitempty"`
	// The rank of pod, only for pytorch-job
	Rank string `json:"rank,omitempty"`
	// Pod start time
	StartTime string `json:"startTime,omitempty"`
	// Pod end time
	EndTime string `json:"endTime,omitempty"`
	// Pod failed reason. may be empty
	FailedMessage string `json:"failedMessage,omitempty"`
	// The Container info of pod
	Containers []Container `json:"containers,omitempty"`
	// The group id of pod, only for torchft. 0 lighthouse, > 0 worker
	GroupId int `json:"groupId,omitempty"`
}

type Container struct {
	// Container name
	Name string `json:"name"`
	// (brief) reason from the last termination of the container
	Reason string `json:"reason,omitempty"`
	// Message regarding the last termination of the container
	Message string `json:"message,omitempty"`
	// Exit status from the last termination of the container
	ExitCode int32 `json:"exitCode"`
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

// IsPending returns true if the operations job is pending execution.
func (w *Workload) IsPending() bool {
	if w.Status.Phase == "" || w.Status.Phase == WorkloadPending {
		return true
	}
	return false
}

// IsRunning returns true if the workload is in Running phase.
func (w *Workload) IsRunning() bool {
	if w.Status.Phase == WorkloadRunning {
		return true
	}
	return false
}

// IsEnd returns true if the workload is terminated
func (w *Workload) IsEnd() bool {
	if w.Status.Phase == WorkloadSucceeded ||
		w.Status.Phase == WorkloadFailed ||
		w.Status.Phase == WorkloadStopped {
		return true
	}
	if !w.GetDeletionTimestamp().IsZero() {
		return true
	}
	return false
}

// ElapsedTime returns the elapsed time in seconds from workload creation to completion or current time.
func (w *Workload) ElapsedTime() int64 {
	var elapsedTime time.Duration
	if w.IsEnd() {
		if w.Status.EndTime == nil {
			return 0
		}
		elapsedTime = w.Status.EndTime.Time.Sub(w.CreationTimestamp.Time)
	} else {
		elapsedTime = time.Now().UTC().Sub(w.CreationTimestamp.Time)
	}
	return int64(elapsedTime.Seconds())
}

// EndTime returns the workload end time, or zero time if not set.
func (w *Workload) EndTime() time.Time {
	if w.Status.EndTime == nil || w.Status.EndTime.IsZero() {
		return time.Time{}
	}
	return w.Status.EndTime.Time
}

// IsTimeout returns true if the operations job has timed out.
func (w *Workload) IsTimeout() bool {
	if w.GetTimeout() <= 0 || w.Status.StartTime == nil {
		return false
	}
	duration := int(time.Now().UTC().Sub(w.Status.StartTime.Time).Seconds())
	return duration >= w.GetTimeout()
}

// GetTimeout returns the timeout value in seconds for the workload.
func (w *Workload) GetTimeout() int {
	if w.Spec.Timeout == nil {
		return 0
	}
	return *w.Spec.Timeout
}

// GetTTLSecond returns the TTL (time to live) in seconds for the workload after completion.
func (w *Workload) GetTTLSecond() int {
	if w.Spec.TTLSecondsAfterFinished == nil {
		return 0
	}
	return *w.Spec.TTLSecondsAfterFinished
}

// GetLastCondition returns the most recent condition in the workload status.
func (w *Workload) GetLastCondition() *metav1.Condition {
	l := len(w.Status.Conditions)
	if l == 0 {
		return nil
	}
	return &w.Status.Conditions[l-1]
}

// IsPodRunning returns true if the pod is in Running phase.
func IsPodRunning(p *WorkloadPod) bool {
	return corev1.PodSucceeded != p.Phase &&
		corev1.PodFailed != p.Phase &&
		p.K8sNodeName != ""
}

// IsPodTerminated returns true if the pod is in terminated phase.
func IsPodTerminated(p *WorkloadPod) bool {
	return corev1.PodSucceeded == p.Phase ||
		corev1.PodFailed == p.Phase
}

// ToSchemaGVK converts the resource template GVK to schema.GroupVersionKind.
func (w *Workload) ToSchemaGVK() schema.GroupVersionKind {
	return w.Spec.GroupVersionKind.ToSchemaGVK()
}

// SpecKind returns the kind string from the resource spec.
func (w *Workload) SpecKind() string {
	return w.Spec.GroupVersionKind.Kind
}

// SpecVersion returns the version string from the workload spec.
func (w *Workload) SpecVersion() string {
	return w.Spec.GroupVersionKind.Version
}

// SetDependenciesPhase sets the phase of a dependency workload.
func (w *Workload) SetDependenciesPhase(workloadId string, phase WorkloadPhase) {
	if w.Status.DependenciesPhase == nil {
		w.Status.DependenciesPhase = make(map[string]WorkloadPhase)
	}
	w.Status.DependenciesPhase[workloadId] = phase
}

// GetDependenciesPhase gets the phase of a dependency workload.
func (w *Workload) GetDependenciesPhase(workloadId string) (WorkloadPhase, bool) {
	if w.Status.DependenciesPhase == nil {
		return WorkloadPending, false
	}
	phase, ok := w.Status.DependenciesPhase[workloadId]
	return phase, ok
}

// IsDependenciesFinish checks if all dependencies are finished.
func (w *Workload) IsDependenciesFinish() bool {
	if w.IsEnd() {
		return false
	}
	for _, dep := range w.Spec.Dependencies {
		phase, ok := w.GetDependenciesPhase(dep)
		if !ok {
			return false
		}
		if phase != WorkloadSucceeded {
			return false
		}
	}

	return true
}

// HasScheduled checks if the workload has been scheduled at least once.
func (w *Workload) HasScheduled() bool {
	if IsWorkloadScheduled(w) || GetWorkloadDispatchCnt(w) > 0 {
		return true
	}
	return false
}

// HasSpecifiedNodes checks if the workload has specified node constraints.
// It returns true if CustomerLabels contains a non-empty K8sHostName label
func (w *Workload) HasSpecifiedNodes() bool {
	if len(w.Spec.CustomerLabels) > 0 {
		if val, ok := w.Spec.CustomerLabels[K8sHostName]; ok && val != "" {
			return true
		}
	}
	return false
}

// GetEnv retrieves the value of an environment variable by name from the workload's spec.
// It returns the value if found, otherwise returns an empty string.
func (w *Workload) GetEnv(name string) string {
	for key, val := range w.Spec.Env {
		if key == name {
			return val
		}
	}
	return ""
}
