/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
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
	// workload name (display only)
	DisplayName string `json:"displayName"`
	// workload description
	Description string `json:"description,omitempty"`
}

type CreateWorkloadResponse struct {
	WorkloadId string `json:"workloadId"`
}

type ListWorkloadRequest struct {
	// workspace id
	WorkspaceId string `form:"workspaceId" binding:"omitempty,max=64"`
	// Valid values include: Succeeded,Failed,Pending,Running,Stopped
	// If specifying multiple phase queries, separate them with commas
	Phase string `form:"phase" binding:"omitempty"`
	// cluster id
	ClusterId string `form:"clusterId" binding:"omitempty,max=64"`
	// user id
	UserId string `form:"userId" binding:"omitempty,max=64"`
	// workload submitter, Supports fuzzy matching
	UserName string `form:"userName" binding:"omitempty"`
	// Valid values include: Deployment/PyTorchJob/StatefulSet/Authoring
	// If specifying multiple kind queries, separate them with commas
	Kind string `form:"kind" binding:"omitempty"`
	// description, Supports fuzzy matching
	Description string `form:"description" binding:"omitempty"`
	// Starting offset for the results. dfault is 0
	Offset int `form:"offset" binding:"omitempty,min=0"`
	// Limit the number of returned results. default is 100
	Limit int `form:"limit" binding:"omitempty,min=1"`
	// Sort results by the specified field. default is create_time
	SortBy string `form:"sortBy" binding:"omitempty"`
	// default is desc
	Order string `form:"order" binding:"omitempty,oneof=desc asc"`
	// Query the start time of the workload, based on the task's creation time.
	// e.g. '2006-01-02T15:04:05.000Z'
	Since string `form:"since" binding:"omitempty"`
	// Query the end time of the workload, similar to since
	Until string `form:"until" binding:"omitempty"`
	// workloadId, Supports fuzzy matching
	WorkloadId string `form:"workloadId" binding:"omitempty,max=64"`
}

type ListWorkloadResponse struct {
	TotalCount int                    `json:"totalCount"`
	Items      []WorkloadResponseItem `json:"items"`
}

type WorkloadResponseItem struct {
	// workload id
	WorkloadId string `json:"workloadId"`
	// Requested workspace
	Workspace string `json:"workspace"`
	// Workload resource requirements
	Resource v1.WorkloadResource `json:"resource"`
	// workload name (display only)
	DisplayName string `json:"displayName"`
	// workload description
	Description string `json:"description"`
	// workload submitter's id
	UserId string `json:"userId"`
	// workload submitter's name
	UserName string `json:"userName"`
	// cluster to which the workload belongs
	Cluster string `json:"cluster"`
	// status of workload
	Phase string `json:"phase"`
	// Shows the reason if the workload is in pending status.
	Message string `json:"message"`
	// Workload scheduling priority. Defaults to 0; valid range: 0–2
	Priority int `json:"priority"`
	// workload creation time
	CreationTime string `json:"creationTime"`
	// workload start time
	StartTime string `json:"startTime"`
	// workload end time
	EndTime string `json:"endTime"`
	// workload deletion time
	DeletionTime string `json:"deletionTime"`
	// Seconds remaining before task timeout. Only applicable if a timeout is set.
	SecondsUntilTimeout int64 `json:"secondsUntilTimeout"`
	// show the queue position of the workload if it is pending.
	SchedulerOrder int `json:"schedulerOrder"`
	// total dispatch count
	DispatchCount int `json:"dispatchCount"`
	// Indicates whether the workload tolerates node taints
	IsTolerateAll    bool                `json:"isTolerateAll"`
	GroupVersionKind v1.GroupVersionKind `json:"groupVersionKind"`
	// Workload timeout in hours. Default is 0 (no timeout).
	Timeout *int `json:"timeout"`
	// workload's UID
	WorkloadUid string `json:"workloadUid"`
	// K8s object uid corresponding to the workload
	K8sObjectUid string `json:"k8sObjectUid"`
}

type GetWorkloadResponse struct {
	WorkloadResponseItem
	NodeList []string `json:"nodeList"`
	// Workload image address
	Image string `json:"image"`
	// workload entryPoint, required in base64 encoding
	EntryPoint string `json:"entryPoint"`
	// Supervision flag for the workload. When enabled, it performs operations like hang detection
	IsSupervised bool `json:"isSupervised"`
	// Failure retry limit. default: 0
	MaxRetry int `json:"maxRetry"`
	// The lifecycle of the workload after completion, in seconds. Default to 60.
	TTLSecondsAfterFinished *int `json:"ttlSecondsAfterFinished"`
	// detailed processing workflow of the workload
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
	// environment variables
	Env map[string]string `json:"env"`
	// K8s liveness check. used for deployment/statefulSet
	Liveness *v1.HealthCheck `json:"liveness,omitempty"`
	// K8s readiness check. used for deployment/statefulSet
	Readiness *v1.HealthCheck `json:"readiness,omitempty"`
	// Service configuration
	Service *v1.Service `json:"service,omitempty"`
}

type WorkloadPodWrapper struct {
	v1.WorkloadPod
	SSHAddr string `json:"sshAddr,omitempty"`
}

type PatchWorkloadRequest struct {
	// workload scheduling priority. valid range: 0–2
	Priority *int `json:"priority,omitempty"`
	// Requested replica count for the workload
	Replica *int `json:"replica,omitempty"`
	// cpu cores, e.g. 128
	CPU *string `json:"cpu,omitempty"`
	// gpu card, e.g. 8
	GPU *string `json:"gpu,omitempty"`
	// memory size, e.g. 128Gi
	Memory *string `json:"memory,omitempty"`
	// pod storage size, e.g. 50Gi
	EphemeralStorage *string `json:"ephemeralStorage,omitempty"`
	// shared memory, e.g. 20Gi
	SharedMemory *string `json:"sharedMemory,omitempty"`
	// the image used by workload
	Image *string `json:"image,omitempty"`
	// workload entryPoint, required in base64 encoding
	EntryPoint *string `json:"entryPoint,omitempty"`
	// environment variable for workload
	Env *map[string]string `json:"env,omitempty"`
	// workload description
	Description *string `json:"description,omitempty"`
	// workload timeout in hours. Default is 0 (no timeout).
	Timeout *int `json:"timeout,omitempty"`
}

type GetPodLogRequest struct {
	TailLines    int64  `form:"tailLines" binding:"omitempty,min=1" `
	Container    string `form:"container" binding:"omitempty"`
	SinceSeconds int64  `form:"sinceSeconds" binding:"omitempty"`
}

type GetWorkloadPodLogResponse struct {
	// workload id
	WorkloadId string `json:"workloadId"`
	// pod id
	PodId string `json:"podId"`
	// the namespace which the workload belongs to
	Namespace string `json:"namespace"`
	// An array of log lines, returned in the same order as they appear in the original logs
	Logs []string `json:"logs"`
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
