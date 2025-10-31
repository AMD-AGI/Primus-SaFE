/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package types

import (
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

type BaseOpsJobRequest struct {
	// Used to generate the ops job ID, which will do normalization processing, e.g. lowercase and random suffix
	Name string `json:"name"`
	// Opsjob type, e.g. addon, dumplog, preflight
	Type v1.OpsJobType `json:"type"`
	// The resource objects to be processed. e.g. {{"name": "node", "value": "tus1-p8-g6"}}
	Inputs []v1.Parameter `json:"inputs"`
	// The job Timeout (in seconds), Less than or equal to 0 means no timeout
	TimeoutSecond int `json:"timeoutSecond,omitempty"`
	// The lifecycle of ops-job after it finishes
	TTLSecondsAfterFinished int `json:"ttlSecondsAfterFinished,omitempty"`
	// Nodes to be excluded, not participating in the ops job
	ExcludedNodes []string `json:"excludedNodes,omitempty"`
	// Indicates whether the job tolerates node taints. default false
	IsTolerateAll bool `json:"isTolerateAll"`
}

type CreatePreflightRequest struct {
	BaseOpsJobRequest
	// Opsjob resource requirements
	Resource *v1.WorkloadResource `json:"resource,omitempty"`
	// Opsjob image address
	Image *string `json:"image,omitempty"`
	// Opsjob entryPoint(startup command), required in base64 encoding
	EntryPoint *string `json:"entryPoint,omitempty"`
	// Environment variables
	Env map[string]string `json:"env,omitempty"`
	// The hostpath for opsjob mounting.
	Hostpath []string `json:"hostpath,omitempty"`
}

type CreateAddonRequest struct {
	BaseOpsJobRequest
	// The number of nodes to process simultaneously during the addon upgrade. default 1
	// If the number exceeds the job limit, it will be capped to the maximum available node count.
	BatchCount int `json:"batchCount,omitempty"`
	// Job Success Ratio: A percentage value used during the addon upgrade.
	// The job is marked as successful if the number of successfully upgraded nodes exceeds total nodes * ratio.
	// default: 1
	AvailableRatio *float64 `json:"availableRatio,omitempty"`
	// When enabled, the operation will wait until the node is idle(no workloads), only to addon
	SecurityUpgrade bool `json:"securityUpgrade,omitempty"`
}

type CreateDumplogRequest struct {
	BaseOpsJobRequest
}

type CreateOpsJobResponse struct {
	// Ops job ID
	JobId string `json:"jobId"`
}

// ListOpsJobInput The query input by the user for listing ops jobs
type ListOpsJobInput struct {
	// Starting offset for the results. default 0
	Offset int `form:"offset" binding:"omitempty,min=0"`
	// Limit the number of returned results. default 100
	Limit int `form:"limit" binding:"omitempty,min=1"`
	// Sort results by the specified field. default create_time
	SortBy string `form:"sortBy" binding:"omitempty"`
	// The sorting order. Valid values are "desc" (default) or "asc"
	Order string `form:"order" binding:"omitempty,oneof=desc asc"`
	// Query the start time of the job, based on the job creation time.
	// e.g. '2006-01-02T15:04:05.000Z'. default until - 720h
	Since string `form:"since" binding:"omitempty"`
	// Query the end time of the job, similar to since. default now
	Until string `form:"until" binding:"omitempty"`
	// The cluster which the job belongs to
	ClusterId string `form:"clusterId" binding:"omitempty,max=64"`
	// Filter by submitter username (fuzzy match)
	UserName string `form:"userName" binding:"omitempty,max=64"`
	// The job phase, e.g. Succeeded, Failed, Running, Pending
	Phase v1.OpsJobPhase `form:"phase" binding:"omitempty"`
	// The job type, e.g. addon, dumplog, preflight, reboot
	Type v1.OpsJobType `form:"type" binding:"omitempty,max=64"`
}

// ListOpsJobRequest internal use
type ListOpsJobRequest struct {
	ListOpsJobInput
	// Start timestamp of the query
	SinceTime time.Time
	// End timestamp of the query
	UntilTime time.Time
	// The user ID of list-job submitter
	UserId string
}

type ListOpsJobResponse struct {
	// The total number of ops jobs, not limited by pagination
	TotalCount int                  `json:"totalCount"`
	Items      []OpsJobResponseItem `json:"items"`
}

type OpsJobResponseItem struct {
	// The job ID
	JobId string `json:"jobId"`
	// The job name
	JobName string `json:"jobName"`
	// The cluster which the job belongs to
	ClusterId string `json:"clusterId"`
	// The workspace which the job belongs to
	WorkspaceId string `json:"workspaceId"`
	// The user ID of job submitter
	UserId string `json:"userId"`
	// The username of job submitter
	UserName string `json:"userName"`
	// The job type, e.g. addon, dumplog, preflight, reboot
	Type v1.OpsJobType `json:"type"`
	// The job status: Succeeded/Failed/Running/Pending
	Phase v1.OpsJobPhase `json:"phase"`
	// The job creation time
	CreationTime string `json:"creationTime"`
	// The job start time
	StartTime string `json:"startTime"`
	// The job end time
	EndTime string `json:"endTime"`
	// The job deletion time
	DeletionTime string `json:"deletionTime"`
	// The job Timeout (in seconds), Less than or equal to 0 means no timeout
	TimeoutSecond int `json:"timeoutSecond"`
}

type GetOpsJobResponse struct {
	OpsJobResponseItem
	// Description of the job execution process
	Conditions []metav1.Condition `json:"conditions"`
	// The job inputs
	Inputs []v1.Parameter `json:"inputs"`
	// The job outputs
	Outputs []v1.Parameter `json:"outputs"`
	// The environment variables
	Env map[string]string `json:"env"`
	// Opsjob resource requirements, only for preflight
	Resource *v1.WorkloadResource `json:"resource"`
	// Opsjob image address, only for preflight
	Image string `json:"image"`
	// Opsjob entryPoint, required in base64 encoding, only for preflight
	EntryPoint string `json:"entryPoint"`
	// Indicates whether the job tolerates node taints. default false
	IsTolerateAll bool `json:"isTolerateAll"`
	// The hostpath for opsjob mounting.
	Hostpath []string `json:"hostpath"`
}
