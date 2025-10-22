/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package types

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

type CreateWorkloadRequest struct {
	v1.WorkloadSpec
	// When specifying nodes, the replica count corresponds to the number of nodes
	NodeList []string `json:"nodeList,omitempty"`
	// The Workload name(display only). Used to generate the workload id,
	// which will do normalization processing, such as lowercase and random suffix
	DisplayName string `json:"displayName"`
	// The workload description
	Description string `json:"description,omitempty"`
	// Workspace ID to which the workload is delivered
	WorkspaceId string `json:"workspaceId,omitempty"`
}

type CreateWorkloadResponse struct {
	// The workload id
	WorkloadId string `json:"workloadId"`
}

type ListWorkloadRequest struct {
	// Filter results by workspace id
	WorkspaceId string `form:"workspaceId" binding:"omitempty,max=64"`
	// Filter results by phase
	// Valid values include: Succeeded,Failed,Pending,Running,Stopped
	// If specifying multiple phase queries, separate them with commas
	Phase string `form:"phase" binding:"omitempty"`
	// Filter results by cluster id
	ClusterId string `form:"clusterId" binding:"omitempty,max=64"`
	// Filter results by user id
	UserId string `form:"userId" binding:"omitempty,max=64"`
	// Filter results by username, supports fuzzy matching
	UserName string `form:"userName" binding:"omitempty"`
	// Filter results by workload kind
	// Valid values include: Deployment/PyTorchJob/StatefulSet/Authoring
	// If specifying multiple kind queries, separate them with commas
	Kind string `form:"kind" binding:"omitempty"`
	// Filter results by workload description, supports fuzzy matching
	Description string `form:"description" binding:"omitempty"`
	// Starting offset for the results. dfault is 0
	Offset int `form:"offset" binding:"omitempty,min=0"`
	// Limit the number of returned results. default is 100
	Limit int `form:"limit" binding:"omitempty,min=1"`
	// Sort results by the specified field. default is create_time
	SortBy string `form:"sortBy" binding:"omitempty"`
	// The sorting order. Valid values are "desc" (default) or "asc"
	Order string `form:"order" binding:"omitempty,oneof=desc asc"`
	// Query the start time of the workload, based on the workload creation time.
	// e.g. '2006-01-02T15:04:05.000Z'
	Since string `form:"since" binding:"omitempty"`
	// Query the end time of the workload, similar to since
	Until string `form:"until" binding:"omitempty"`
	// The workload id, Supports fuzzy matching
	WorkloadId string `form:"workloadId" binding:"omitempty,max=64"`
}

type ListWorkloadResponse struct {
	// The total number of node templates, not limited by pagination
	TotalCount int                    `json:"totalCount"`
	Items      []WorkloadResponseItem `json:"items"`
}

type WorkloadResponseItem struct {
	// The workload id
	WorkloadId string `json:"workloadId"`
	// The workspace which workload belongs to
	WorkspaceId string `json:"workspaceId"`
	// The workload resource requirements
	Resource v1.WorkloadResource `json:"resource"`
	// The workload name (display only)
	DisplayName string `json:"displayName"`
	// The workload description
	Description string `json:"description"`
	// The user id of workload submitter
	UserId string `json:"userId"`
	// The username of workload submitter
	UserName string `json:"userName"`
	// The cluster which the workload belongs to
	ClusterId string `json:"cluster"`
	// The status of workload, such as Succeeded, Failed, Pending, Running, Stopped, Updating
	Phase string `json:"phase"`
	// Shows the reason if the workload is in pending status.
	Message string `json:"message"`
	// Workload scheduling priority. Defaults is 0, valid range: 0–2
	Priority int `json:"priority"`
	// The workload creation time
	CreationTime string `json:"creationTime"`
	// The workload start time
	StartTime string `json:"startTime"`
	// The workload end time
	EndTime string `json:"endTime"`
	// The workload deletion time
	DeletionTime string `json:"deletionTime"`
	// Seconds remaining before workload timeout. Only applicable if a timeout is set.
	SecondsUntilTimeout int64 `json:"secondsUntilTimeout"`
	// Show the queue position of the workload if it is pending.
	SchedulerOrder int `json:"schedulerOrder"`
	// Total dispatch count of workload
	DispatchCount int `json:"dispatchCount"`
	// Indicates whether the workload tolerates node taints
	IsTolerateAll bool `json:"isTolerateAll"`
	// Defines the group, version, and kind of the workload. Currently, the group is not used
	GroupVersionKind v1.GroupVersionKind `json:"groupVersionKind"`
	// Workload timeout in hours. Default is 0 (no timeout).
	Timeout *int `json:"timeout"`
	// Workload uid
	WorkloadUid string `json:"workloadUid"`
	// K8s object uid corresponding to the workload
	K8sObjectUid string `json:"k8sObjectUid"`
}

type GetWorkloadResponse struct {
	WorkloadResponseItem
	// The node specified by the user when creating the workload
	NodeList []string `json:"nodeList"`
	// The address of the image used by the workload
	Image string `json:"image"`
	// Workload startup command, required in base64 encoding
	EntryPoint string `json:"entryPoint"`
	// Supervision flag for the workload. When enabled, it performs operations like hang detection
	IsSupervised bool `json:"isSupervised"`
	// Failure retry limit. default: 0
	MaxRetry int `json:"maxRetry"`
	// The lifecycle of the workload after completion, in seconds. Default to 60.
	TTLSecondsAfterFinished *int `json:"ttlSecondsAfterFinished"`
	// Detailed processing workflow of the workload
	Conditions []metav1.Condition `json:"conditions"`
	// Pod info related to the workload
	Pods []WorkloadPodWrapper `json:"pods"`
	// The node used for each workload execution. If the workload is retried multiple times, there will be multiple entries.
	Nodes [][]string `json:"nodes"`
	// The node's rank is only valid for the PyTorch job and corresponds one-to-one with the nodes listed above.
	Ranks [][]string `json:"ranks"`
	// The workload will run on nodes with the user-specified labels.
	// If multiple labels are specified, all of them must be satisfied.
	CustomerLabels map[string]string `json:"customerLabels"`
	// Environment variables
	Env map[string]string `json:"env"`
	// K8s liveness check. used for deployment/statefulSet
	Liveness *v1.HealthCheck `json:"liveness,omitempty"`
	// K8s readiness check. used for deployment/statefulSet
	Readiness *v1.HealthCheck `json:"readiness,omitempty"`
	// Service configuration
	Service *v1.Service `json:"service,omitempty"`
	// Scheduled workload configuration
	CronSchedules []v1.CronSchedule `json:"cronSchedules,omitempty"`
	// Dependencies defines a list of other Workloads that must complete successfully
	// before this Workload can start execution. If any dependency fails, this Workload
	// will not be scheduled and is considered failed.
	Dependencies []string `json:"dependencies,omitempty"`
}

type WorkloadPodWrapper struct {
	v1.WorkloadPod
	// SSH address to log in
	SSHAddr string `json:"sshAddr,omitempty"`
}

type PatchWorkloadRequest struct {
	// Workload scheduling priority, valid range: 0–2
	Priority *int `json:"priority,omitempty"`
	// Requested replica count for the workload
	Replica *int `json:"replica,omitempty"`
	// Cpu cores, e.g. 128
	CPU *string `json:"cpu,omitempty"`
	// Gpu card, e.g. 8
	GPU *string `json:"gpu,omitempty"`
	// Memory size, e.g. 128Gi
	Memory *string `json:"memory,omitempty"`
	// Pod storage size, e.g. 50Gi
	EphemeralStorage *string `json:"ephemeralStorage,omitempty"`
	// Shared memory, e.g. 20Gi
	SharedMemory *string `json:"sharedMemory,omitempty"`
	// The image address used by workload
	Image *string `json:"image,omitempty"`
	// Workload startup command, required in base64 encoding
	EntryPoint *string `json:"entryPoint,omitempty"`
	// Environment variable for workload
	Env *map[string]string `json:"env,omitempty"`
	// Workload description
	Description *string `json:"description,omitempty"`
	// Workload timeout in hours. Default is 0 (no timeout).
	Timeout *int `json:"timeout,omitempty"`
	// Failure retry limit
	MaxRetry *int `json:"maxRetry,omitempty"`
}

type GetPodLogRequest struct {
	// Retrieve the last n lines of logs
	TailLines int64 `form:"tailLines" binding:"omitempty,min=1"`
	// Return logs for the corresponding container
	Container string `form:"container" binding:"omitempty"`
	// Start time for retrieving logs, in seconds
	SinceSeconds int64 `form:"sinceSeconds" binding:"omitempty"`
}

type GetWorkloadPodLogResponse struct {
	// The workload id
	WorkloadId string `json:"workloadId"`
	// The pod id
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

type WorkloadSlice []v1.Workload

func (ws WorkloadSlice) Len() int {
	return len(ws)
}

func (ws WorkloadSlice) Swap(i, j int) {
	ws[i], ws[j] = ws[j], ws[i]
}

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
