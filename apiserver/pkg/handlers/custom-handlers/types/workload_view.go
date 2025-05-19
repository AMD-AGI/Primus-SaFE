/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package types

import (
	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

type CreateWorkloadRequest struct {
	v1.WorkloadSpec
	// workload name (display only)
	DisplayName string `json:"displayName"`
	// workload description
	Description string `json:"description,omitempty"`
	// workload submitter
	UserName string `json:"userName,omitempty"`
}

type CreateWorkloadResponse struct {
	WorkloadId string `json:"workloadId"`
}

type GetWorkloadRequest struct {
	// workspace id
	WorkspaceId string `form:"workspaceId" binding:"omitempty,max=64"`
	// Succeeded/Failed/Pending/Running/Stopped/Updating/NotReady
	Phase string `form:"phase" binding:"omitempty"`
	// cluster id
	ClusterId string `form:"clusterId" binding:"omitempty,max=64"`
	// Deployment/PyTorchJob/StatefulSet
	Kind string `form:"kind" binding:"omitempty"`
	// workload submitter
	UserName string `form:"userName" binding:"omitempty"`
}

type GetWorkloadResponseItem struct {
	// workload id
	WorkloadId string `json:"workloadId"`
	CreateWorkloadRequest
	// cluster to which the workload belongs
	Cluster string `json:"cluster,omitempty"`
	// workload submitter
	UserName string `json:"userName,omitempty"`
	// status of workload
	Phase string `json:"phase,omitempty"`
	// Shows the reason if the workload is in pending status.
	Message string `json:"message,omitempty"`
	// detailed processing workflow of the workload
	Conditions string `json:"conditions,omitempty"`
	// workload creation time
	CreatedTime string `json:"createdTime,omitempty"`
	// workload start time
	StartTime string `json:"startTime,omitempty"`
	// workload end time
	EndTime string `json:"endTime,omitempty"`
	// show the queue position of the workload if it is pending.
	SchedulerOrder int `json:"schedulerOrder,omitempty"`
	// total dispatch count
	DispatchCount int `json:"dispatchCount,omitempty"`
	// the pods information related to the workload
	Pods string `json:"pods,omitempty"`
	// node assigned to the task for each run
	Nodes string `json:"nodes,omitempty"`
}

type GetWorkloadResponse struct {
	TotalCount int                       `json:"totalCount"`
	Items      []GetWorkloadResponseItem `json:"items,omitempty"`
}

type WorkloadResource struct {
	// Valid only for PyTorchJob jobs; values can be "master" or "worker". Optional for other job types.
	Role string `json:"name,omitempty"`
	// Requested replica count for the workload
	Replica int `json:"replica,omitempty"`
	// cpu cores, e.g. 128
	CPU string `json:"cpu,omitempty"`
	// gpu card, e.g. 8
	GPU *string `json:"gpu,omitempty"`
	// memory size, e.g. 128Gi
	Memory string `json:"memory,omitempty"`
	// pod storage size, e.g. 50Gi
	EphemeralStorage string `json:"ephemeralStorage,omitempty"`
	// share memory, e.g. 20Gi
	ShareMemory string `json:"shareMemory,omitempty"`
}

type PatchWorkloadRequest struct {
	// workload scheduling priority. valid range: 0–2
	Priority *int `json:"priority,omitempty"`
	// workload resource requirements
	Resources *[]WorkloadResource `json:"resources,omitempty"`
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
	Namespace string `json:"namespace,omitempty"`
	// An array of log lines, returned in the same order as they appear in the original logs
	Logs []string `json:"logs,omitempty"`
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
