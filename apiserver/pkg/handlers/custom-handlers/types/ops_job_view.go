/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package types

import (
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

type BaseOpsJobRequest struct {
	// ops job name
	Name string `json:"name"`
	// valid values include: addon/dumplog/preflight
	Type v1.OpsJobType `json:"type"`
	// the resource objects to be processed. e.g. {{"name": "node", "value": "node.id"}}
	Inputs []v1.Parameter `json:"inputs"`
	// job Timeout (in seconds), Less than or equal to 0 means no timeout
	TimeoutSecond int `json:"timeoutSecond,omitempty"`
	// the lifecycle of ops-job after it finishes
	TTLSecondsAfterFinished int `json:"ttlSecondsAfterFinished,omitempty"`
	// Excluded nodes
	ExcludedNodes []string `json:"excludedNodes,omitempty"`
}

type CreatePreflightRequest struct {
	BaseOpsJobRequest
	// Opsjob resource requirements
	Resource *v1.WorkloadResource `json:"resource,omitempty"`
	// opsjob image address
	Image *string `json:"image,omitempty"`
	// opsjob entryPoint, required in base64 encoding
	EntryPoint *string `json:"entryPoint,omitempty"`
	// environment variables
	Env map[string]string `json:"env,omitempty"`
	// Indicates whether the job tolerates node taints. default false
	IsTolerateAll bool `json:"isTolerateAll"`
}

type CreateAddonRequest struct {
	BaseOpsJobRequest
	// the number of nodes to process simultaneously during the addon upgrade. default 1
	// If the number exceeds the job's limit, it will be capped to the maximum available node count.
	BatchCount int `json:"batchCount,omitempty"`
	// Job Success Ratio: A percentage value used during the addon upgrade.
	// The job is marked as successful if the number of successfully upgraded nodes exceeds total nodes * ratio.
	// default: 1
	AvailableRatio *float64 `json:"availableRatio,omitempty"`
	// When enabled, the operation will wait until the node is idle, only to addon
	SecurityUpgrade bool `json:"securityUpgrade,omitempty"`
}

type CreateDumplogRequest struct {
	BaseOpsJobRequest
}

type CreateOpsJobResponse struct {
	JobId string `json:"jobId"`
}

type ListOpsJobRequest struct {
	// Starting offset for the results. dfault is 0
	Offset int `form:"offset" binding:"omitempty,min=0"`
	// Limit the number of returned results. default is 100
	Limit int `form:"limit" binding:"omitempty,min=1"`
	// Sort results by the specified field. default is create_time
	SortBy string `form:"sortBy" binding:"omitempty"`
	// default is desc
	Order string `form:"order" binding:"omitempty,oneof=desc asc"`
	// Query the start time of the job, based on the job's creation time.
	// e.g. '2006-01-02T15:04:05.000Z'. default is until - 720h
	Since string `form:"since" binding:"omitempty"`
	// Query the end time of the job, similar to since. default is now
	Until string `form:"until" binding:"omitempty"`
	// the cluster which the job belongs to
	Cluster string `form:"cluster" binding:"required,max=64"`
	// job submitter
	UserName string `form:"userName" binding:"omitempty,max=64"`
	// job phase
	Phase v1.OpsJobPhase `form:"phase" binding:"omitempty"`
	// job type
	Type v1.OpsJobType `form:"type" binding:"required"`

	// for internal use
	SinceTime time.Time
	UntilTime time.Time
	UserId    string
}

type ListOpsJobResponse struct {
	TotalCount int                  `json:"totalCount"`
	Items      []OpsJobResponseItem `json:"items"`
}

type OpsJobResponseItem struct {
	// job id
	JobId string `json:"jobId"`
	// the cluster which the job belongs to
	Cluster string `json:"cluster"`
	// the workspace which the job belongs to
	Workspace string `json:"workspace"`
	// job submitter
	UserId string `json:"userId"`
	// job submitter
	UserName string `json:"userName"`
	// job type
	Type v1.OpsJobType `json:"type"`
	// job phase: Succeeded/Failed/Running
	Phase v1.OpsJobPhase `json:"phase"`
	// job execution flow
	Conditions []metav1.Condition `json:"conditions"`
	// job creation time
	CreationTime string `json:"creationTime"`
	// job start time
	StartTime string `json:"startTime"`
	// job end time
	EndTime string `json:"endTime"`
	// job deletion time
	DeletionTime string `json:"deletionTime"`
	// job inputs
	Inputs []v1.Parameter `json:"inputs"`
	// job outputs
	Outputs []v1.Parameter `json:"outputs"`
	// environment variables
	Env map[string]string `json:"env"`
	// Opsjob resource requirements, only for preflight
	Resource v1.WorkloadResource `json:"resource,omitempty"`
	// opsjob image address, only for preflight
	Image string `json:"image,omitempty"`
	// opsjob entryPoint, required in base64 encoding, only for preflight
	EntryPoint string `json:"entryPoint,omitempty"`
}
