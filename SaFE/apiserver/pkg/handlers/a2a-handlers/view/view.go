/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package view

import "time"

// ServiceView is the API response for an A2A service.
type ServiceView struct {
	Id              int64      `json:"id"`
	WorkloadId      string     `json:"workloadId,omitempty"`
	ServiceName     string     `json:"serviceName"`
	DisplayName     string     `json:"displayName"`
	Description     string     `json:"description,omitempty"`
	Endpoint        string     `json:"endpoint"`
	A2APathPrefix   string     `json:"a2aPathPrefix"`
	A2AAgentCard    string     `json:"a2aAgentCard,omitempty"`
	A2ASkills       string     `json:"a2aSkills,omitempty"`
	A2AHealth       string     `json:"a2aHealth"`
	A2ALastSeen     *time.Time `json:"a2aLastSeen,omitempty"`
	K8sNamespace    string     `json:"k8sNamespace,omitempty"`
	K8sService      string     `json:"k8sService,omitempty"`
	DiscoverySource string     `json:"discoverySource"`
	Status          string     `json:"status"`
	CreatedBy       string     `json:"createdBy,omitempty"`
	CreatedAt       *time.Time `json:"createdAt,omitempty"`
	UpdatedAt       *time.Time `json:"updatedAt,omitempty"`
}

// CreateServiceRequest is the request body for creating an A2A service.
type CreateServiceRequest struct {
	ServiceName   string `json:"serviceName" binding:"required"`
	DisplayName   string `json:"displayName"`
	Description   string `json:"description"`
	Endpoint      string `json:"endpoint" binding:"required"`
	A2APathPrefix string `json:"a2aPathPrefix"`
	WorkloadId    string `json:"workloadId"`
}

// UpdateServiceRequest is the request body for updating an A2A service.
type UpdateServiceRequest struct {
	DisplayName   *string `json:"displayName"`
	Description   *string `json:"description"`
	Endpoint      *string `json:"endpoint"`
	A2APathPrefix *string `json:"a2aPathPrefix"`
	Status        *string `json:"status"`
}

// CallLogView is the API response for an A2A call log entry.
type CallLogView struct {
	Id                int64      `json:"id"`
	TraceId           string     `json:"traceId"`
	CallerServiceName string     `json:"callerServiceName"`
	CallerUserId      string     `json:"callerUserId"`
	TargetServiceName string     `json:"targetServiceName"`
	SkillId           string     `json:"skillId,omitempty"`
	Status            string     `json:"status"`
	LatencyMs         float64    `json:"latencyMs"`
	RequestSizeBytes  int64      `json:"requestSizeBytes"`
	ResponseSizeBytes int64      `json:"responseSizeBytes"`
	ErrorMessage      string     `json:"errorMessage,omitempty"`
	CreatedAt         *time.Time `json:"createdAt,omitempty"`
}

// TopologyEdge represents a caller→target relationship for the topology graph.
type TopologyEdge struct {
	Caller string `json:"caller"`
	Target string `json:"target"`
	Count  int    `json:"count"`
}

// TopologyResponse is the API response for the A2A topology.
type TopologyResponse struct {
	Nodes []ServiceView  `json:"nodes"`
	Edges []TopologyEdge `json:"edges"`
}
