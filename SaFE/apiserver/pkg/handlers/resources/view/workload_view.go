/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package view

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

type CreateWorkloadRequest struct {
	v1.WorkloadSpec
	// When specifying a workload run on nodes, the replica count will be overwritten with the node count.
	SpecifiedNodes []string `json:"specifiedNodes,omitempty"`
	// ExcludedNodes is a list of node names that the workload should avoid running on.
	ExcludedNodes []string `json:"excludedNodes,omitempty"`
	// The Workload name(display only). Used to generate the workload ID,
	// which will do normalization processing, e.g. lowercase and random suffix
	DisplayName string `json:"displayName"`
	// The workload description
	Description string `json:"description,omitempty"`
	// Workspace ID to which the workload is delivered
	WorkspaceId string `json:"workspaceId,omitempty"`
	// User-defined labels. Keys cannot start with "primus-safe."
	Labels map[string]string `json:"labels,omitempty"`
	// User-defined annotations. Keys cannot start with "primus-safe."
	Annotations map[string]string `json:"annotations,omitempty"`
	// Preheat indicates whether to preheat the workload to prepare image in advance
	Preheat bool `json:"preheat,omitempty"`
}

type CreateWorkloadResponse struct {
	// Workload ID
	WorkloadId string `json:"workloadId"`
}

type ListWorkloadRequest struct {
	// Filter results by workspace ID
	WorkspaceId string `form:"workspaceId" binding:"omitempty,max=64"`
	// Filter by status: Succeeded/Failed/Pending/Running/Stopped (comma-separated)
	Phase string `form:"phase" binding:"omitempty"`
	// Filter results by cluster ID
	ClusterId string `form:"clusterId" binding:"omitempty,max=64"`
	// Filter results by user ID
	UserId string `form:"userId" binding:"omitempty,max=64"`
	// Filter results by username (fuzzy match)
	UserName string `form:"userName" binding:"omitempty"`
	// Filter by workload kind: Deployment/PyTorchJob/StatefulSet/Authoring/AutoscalingRunnerSet(comma-separated)
	Kind string `form:"kind" binding:"omitempty"`
	// Filter by description (fuzzy match)
	Description string `form:"description" binding:"omitempty"`
	// Starting offset for the results. default 0
	Offset int `form:"offset" binding:"omitempty,min=0"`
	// Limit the number of returned results. default 100
	Limit int `form:"limit" binding:"omitempty,min=1"`
	// Sort field. default creation_time
	SortBy string `form:"sortBy" binding:"omitempty"`
	// Sort order: desc/asc, default desc
	Order string `form:"order" binding:"omitempty,oneof=desc asc"`
	// Query the start time of the workload, based on the workload creation time.
	// RFC3339 format, e.g. '2006-01-02T15:04:05.000Z'
	Since string `form:"since" binding:"omitempty"`
	// Query the end time of the workload, similar to since
	Until string `form:"until" binding:"omitempty"`
	// Filter by workload ID (fuzzy match)
	WorkloadId string `form:"workloadId" binding:"omitempty,max=64"`
	// Filter by scale runner set.
	ScaleRunnerSet string `form:"scaleRunnerSet" binding:"omitempty,max=64"`
	// Filter by GitHub action runner id.
	ScaleRunnerId string `form:"scaleRunnerId" binding:"omitempty,max=64"`
}

type ListWorkloadResponse struct {
	// The total number of workloads, not limited by pagination
	TotalCount int                    `json:"totalCount"`
	Items      []WorkloadResponseItem `json:"items"`
}

type WorkloadResponseItem struct {
	// Workload ID
	WorkloadId string `json:"workloadId"`
	// The workspace which workload belongs to
	WorkspaceId string `json:"workspaceId"`
	// Workload resource requirements
	Resources []v1.WorkloadResource `json:"resources"`
	// Workload name (display only)
	DisplayName string `json:"displayName"`
	// Workload description
	Description string `json:"description"`
	// The user ID of workload submitter
	UserId string `json:"userId"`
	// The username of workload submitter
	UserName string `json:"userName"`
	// The cluster which the workload belongs to
	ClusterId string `json:"clusterId"`
	// The status of workload, e.g. Succeeded, Failed, Pending, Running, Stopped, Updating
	Phase string `json:"phase"`
	// Shows the pending reason
	Message string `json:"message"`
	// Workload scheduling Priority (0-2), default 0
	Priority int `json:"priority"`
	// Workload creation time (RFC3339Short, e.g. "2025-07-08T10:31:46")
	CreationTime string `json:"creationTime"`
	// Workload start time (RFC3339Short)
	StartTime string `json:"startTime"`
	// Workload end time (RFC3339Short)
	EndTime string `json:"endTime"`
	// Workload deletion time (RFC3339Short)
	DeletionTime string `json:"deletionTime"`
	// Duration represents the total execution time of the workload.
	// It is calculated from the start time to the end time (or current time if still running).
	// Format examples: "1h2m3s" (1 hour, 2 minutes, 3 seconds) or "1h15s" (1 hour, 15 seconds)
	Duration string `json:"duration"`
	// Seconds remaining before workload timeout. Only applicable if a timeout is set.
	// Seconds remaining until timeout, calculated from the start time. If it has not yet started, return -1
	SecondsUntilTimeout int64 `json:"secondsUntilTimeout"`
	// Show the queue position of the workload if it is pending.
	QueuePosition int `json:"queuePosition"`
	// Number of dispatch attempts
	DispatchCount int `json:"dispatchCount"`
	// Whether to tolerate all node taints
	IsTolerateAll bool `json:"isTolerateAll"`
	// Defines the group, version, and kind of the workload. Currently, the group is not used
	GroupVersionKind v1.GroupVersionKind `json:"groupVersionKind"`
	// Timeout seconds (0 means no timeout)
	Timeout *int `json:"timeout"`
	// Workload UID
	WorkloadUid string `json:"workloadUid"`
	// Average GPU usage in the last 3 hours. Returns -1 if no statistics available
	AvgGpuUsage float64 `json:"avgGpuUsage"`
	// If it is a CI/CD workload, it would be associated with a scale runner set.
	ScaleRunnerSet string `json:"scaleRunnerSet,omitempty"`
	// If it is a CI/CD workload, it would be associated with a github runner action id.
	ScaleRunnerId string `json:"scaleRunnerId,omitempty"`
}

type GetWorkloadResponse struct {
	WorkloadResponseItem
	// The node specified by the user when creating the workload
	SpecifiedNodes []string `json:"specifiedNodes,omitempty"`
	// ExcludedNodes is a list of node names that the workload should avoid running on.
	ExcludedNodes []string `json:"excludedNodes,omitempty"`
	// The address of the image used by the workload
	Image string `json:"image"`
	// Workload startup command, in base64 encoding
	EntryPoint string `json:"entryPoint"`
	// Supervision flag for the workload. When enabled, it performs operations like hang detection
	IsSupervised bool `json:"isSupervised"`
	// Failure retry limit. default 0
	MaxRetry int `json:"maxRetry"`
	// The lifecycle after completion, in seconds, default 60.
	TTLSecondsAfterFinished *int `json:"ttlSecondsAfterFinished"`
	// Detailed processing workflow of the workload
	Conditions []metav1.Condition `json:"conditions"`
	// Pod info related to the workload
	Pods []WorkloadPodWrapper `json:"pods"`
	// The node used for each workload execution. If the workload is retried multiple times, there will be multiple entries.
	Nodes [][]string `json:"nodes"`
	// The rank is only valid for the PyTorch job and corresponds one-to-one with the nodes listed above.
	Ranks [][]string `json:"ranks"`
	// Workload will run on nodes with the user-specified labels.
	// If multiple labels are specified, all of them must be satisfied.
	CustomerLabels map[string]string `json:"customerLabels"`
	// Environment variables key-value pairs
	Env map[string]string `json:"env"`
	// K8s liveness check. used for deployment/statefulSet
	Liveness *v1.HealthCheck `json:"liveness,omitempty"`
	// K8s readiness check. used for deployment/statefulSet
	Readiness *v1.HealthCheck `json:"readiness,omitempty"`
	// Service configuration
	Service *v1.Service `json:"service,omitempty"`
	// Dependencies defines a list of other Workloads that must complete successfully
	// before this Workload can start execution. If any dependency fails, this Workload
	// will not be scheduled and is considered failed.
	Dependencies []string `json:"dependencies,omitempty"`
	// Cron Job configuration
	CronJobs []v1.CronJob `json:"cronJobs,omitempty"`
	// The secrets used by the workload. Only the user themselves or an administrator can get this info.
	Secrets []v1.SecretEntity `json:"secrets,omitempty"`
}

type WorkloadPodWrapper struct {
	v1.WorkloadPod
	// SSH address to log in
	SSHAddr string `json:"sshAddr,omitempty"`
}

type PatchWorkloadRequest struct {
	// Workload scheduling Priority (0-2), default 0
	Priority *int `json:"priority,omitempty"`
	// Workload resource requirements
	Resources *[]v1.WorkloadResource `json:"resources"`
	// The image address used by workload
	Image *string `json:"image,omitempty"`
	// Workload startup command, required in base64 encoding
	EntryPoint *string `json:"entryPoint,omitempty"`
	// Environment variable for workload
	Env *map[string]string `json:"env,omitempty"`
	// Workload description
	Description *string `json:"description,omitempty"`
	// Timeout seconds (0 means no timeout)
	Timeout *int `json:"timeout,omitempty"`
	// Failure retry limit
	MaxRetry *int `json:"maxRetry,omitempty"`
	// Cron Job configuration
	CronJobs *[]v1.CronJob `json:"cronJobs,omitempty"`
	// Service configuration
	Service *v1.Service `json:"service,omitempty"`
}

type GetPodLogRequest struct {
	// Retrieve the last n lines of logs. default 1000
	TailLines int64 `form:"tailLines" binding:"omitempty,min=1"`
	// Return logs for the corresponding container
	Container string `form:"container" binding:"omitempty"`
	// Start time for retrieving logs, in seconds
	SinceSeconds int64 `form:"sinceSeconds" binding:"omitempty"`
}

type GetWorkloadPodLogResponse struct {
	// Workload ID
	WorkloadId string `json:"workloadId"`
	// The pod ID
	PodId string `json:"podId"`
	// The namespace which the workload belongs to
	Namespace string `json:"namespace"`
	// An array of log lines, returned in the same order as they appear in the original logs
	Logs []string `json:"logs"`
}

type BatchWorkloadsRequest struct {
	// List of workload IDs to be processed
	WorkloadIds []string `json:"workloadIds"`
}

type GetWorkloadServiceResponse struct {
	// The Service port information (protocol/port/targetPort) from the first port.
	Port corev1.ServicePort `json:"port"`
	// Externally accessible URL via Higress when enabled and system host is configured; empty otherwise.
	ExternalDomain string `json:"externalDomain"`
	// In-cluster DNS address of the Service with port, e.g. <name>.<namespace>.svc.cluster.local:<port>.
	InternalDomain string `json:"internalDomain"`
	// ClusterIP assigned to the Service (empty for headless or None).
	ClusterIp string `json:"clusterIp"`
	// Kubernetes Service type: ClusterIP, NodePort.
	Type corev1.ServiceType `json:"type"`
}

type WorkloadSlice []v1.Workload

// Len implements sort.Interface by returning the length of the slice.
func (ws WorkloadSlice) Len() int {
	return len(ws)
}

// Swap implements sort.Interface by swapping elements at the given indices.
func (ws WorkloadSlice) Swap(i, j int) {
	ws[i], ws[j] = ws[j], ws[i]
}

// Less implements sort.Interface for sorting.
func (ws WorkloadSlice) Less(i, j int) bool {
	if ws[i].CreationTimestamp.Time.Before(ws[j].CreationTimestamp.Time) {
		return true
	}
	if ws[i].CreationTimestamp.Time.Equal(ws[j].CreationTimestamp.Time) && ws[i].Name < ws[j].Name {
		return true
	}
	return false
}

// GetWorkloadPodContainersResponse represents the response containing the list of containers and supported shells for a workload pod.
type GetWorkloadPodContainersResponse struct {
	Containers []GetWorkloadPodContainersItem `json:"containers"` // List of containers in the workload pod.
	Shells     []string                       `json:"shells"`     // Supported shells, should allow user customization.
}

// GetWorkloadPodContainersItem represents a single container item in the response.
type GetWorkloadPodContainersItem struct {
	Name string `json:"name"` // Name of the container.
}
