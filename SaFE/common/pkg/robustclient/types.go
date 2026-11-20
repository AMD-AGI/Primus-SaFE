/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package robustclient

import "encoding/json"

type ResponseMeta struct {
	Code    int    `json:"code"`
	Message string `json:"message,omitempty"`
}

type ResponseEnvelope struct {
	Meta ResponseMeta    `json:"meta"`
	Data json.RawMessage `json:"data"`
}

type WorkloadSyncPayload struct {
	UID         string            `json:"uid"`
	Name        string            `json:"name"`
	Workspace   string            `json:"workspace"`
	Labels      map[string]string `json:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
	Phase       string            `json:"phase"`
	GPURequest  int               `json:"gpu_request"`
	CreatedAt   *string           `json:"created_at,omitempty"`
	EndAt       *string           `json:"end_at,omitempty"`
	// ResourceVersion is the Workload's metadata.resourceVersion. Robust ignores
	// a payload whose version is not newer than the one it holds.
	ResourceVersion string `json:"resource_version,omitempty"`
}

type WorkloadSyncBatchPayload struct {
	Workloads []WorkloadSyncPayload `json:"workloads"`
}

type WorkloadSyncResponse struct {
	UID     string `json:"uid"`
	Status  string `json:"status"`
	Matched bool   `json:"matched"`
}

type WorkloadSyncBatchResponse struct {
	Synced int `json:"synced"`
	Failed int `json:"failed"`
}

// WorkloadSyncDigestEntry is what Robust holds for one workload synced from SaFE.
type WorkloadSyncDigestEntry struct {
	UID         string `json:"uid"`
	SyncVersion string `json:"sync_version"`
	Phase       string `json:"phase"`
	Deleted     bool   `json:"deleted"`
}

type WorkloadSyncDigest struct {
	Workloads []WorkloadSyncDigestEntry `json:"workloads"`
}

// Values of NodeSyncPayload.Phase.
const (
	// The machine is ready, the node is in its cluster and accepts new pods.
	NodeSyncPhaseReady = "Ready"
	// The machine is ready and in its cluster but cordoned.
	NodeSyncPhaseUnschedulable = "Unschedulable"
	// Anything else: the machine or its cluster membership is not ready.
	NodeSyncPhaseNotReady = "NotReady"
)

// Values of NodeSyncPayload.Lifecycle.
const (
	NodeSyncLifecycleManaged  = "managed"
	NodeSyncLifecycleExternal = "external"
)

// NodeSyncPayload is one SaFE Node as Robust stores it. Absent fields keep
// what Robust holds.
type NodeSyncPayload struct {
	// The SaFE Node name, the Kubernetes node name for virtual nodes.
	Name      string `json:"name"`
	Hostname  string `json:"hostname,omitempty"`
	Workspace string `json:"workspace"`
	// Labels and annotations of the SaFE Node.
	Labels      map[string]string `json:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
	// One of the NodeSyncPhase values.
	Phase    string `json:"phase"`
	GPUCount int    `json:"gpu_count"`
	GPUModel string `json:"gpu_model,omitempty"`
	// CPU cores.
	CPU   int   `json:"cpu"`
	MemMB int64 `json:"mem_mb"`
	// The address Robust scrapes the host exporters on. Nil keeps what Robust
	// holds; an empty string clears it.
	Address *string `json:"address,omitempty"`
	// One of the NodeSyncLifecycle values; External is set only for external nodes.
	Lifecycle string            `json:"lifecycle"`
	External  *NodeSyncExternal `json:"external,omitempty"`
	// ResourceVersion is the Node's metadata.resourceVersion. Robust ignores a
	// payload whose version is not newer than the one it holds.
	ResourceVersion string `json:"resource_version,omitempty"`
}

// NodeSyncExternal is the provider allocation behind an external node.
type NodeSyncExternal struct {
	Provider     string `json:"provider"`
	AllocationID string `json:"allocation_id"`
	Generation   int64  `json:"generation"`
	HostKey      string `json:"host_key,omitempty"`
}

type NodeSyncBatchPayload struct {
	Nodes []NodeSyncPayload `json:"nodes"`
}

// NodeSyncDigestEntry is what Robust holds for one node synced from SaFE.
type NodeSyncDigestEntry struct {
	Name        string `json:"name"`
	SyncVersion string `json:"sync_version"`
	Deleted     bool   `json:"deleted"`
}

type NodeSyncDigest struct {
	Nodes []NodeSyncDigestEntry `json:"nodes"`
}

type GPUWindowRequest struct {
	WorkloadUIDs []string `json:"workload_uids"`
	Window       string   `json:"window"`
	Step         string   `json:"step"`
}

// GPUWindowOverall summarizes the per-step average utilization (percent)
// across a workload's nodes over the window.
type GPUWindowOverall struct {
	Max             float64 `json:"max"`
	Avg             float64 `json:"avg"`
	Samples         int     `json:"samples"`
	ExpectedSamples int     `json:"expected_samples"`
}

// GPUWindowNode is one node's peak utilization (percent) over the window.
type GPUWindowNode struct {
	Node    string  `json:"node"`
	Max     float64 `json:"max"`
	Samples int     `json:"samples"`
}

type WorkloadGPUWindow struct {
	UID     string           `json:"uid"`
	Overall GPUWindowOverall `json:"overall"`
	Nodes   []GPUWindowNode  `json:"nodes"`
}

type GPUWindowResponse struct {
	Workloads []WorkloadGPUWindow `json:"workloads"`
}

type LogSearchRequest struct {
	Index     string          `json:"index"`
	Query     json.RawMessage `json:"query"`
	Size      int             `json:"size"`
	Sort      json.RawMessage `json:"sort,omitempty"`
	Source    json.RawMessage `json:"_source,omitempty"`
	ScrollTTL string          `json:"scroll,omitempty"`
}

type LogSearchResponse struct {
	ScrollID string          `json:"scroll_id,omitempty"`
	Hits     json.RawMessage `json:"hits"`
	TimedOut bool            `json:"timed_out"`
	Took     int             `json:"took"`
}

type LogScrollRequest struct {
	ScrollID  string `json:"scroll_id"`
	ScrollTTL string `json:"scroll"`
}

type LogScrollResponse struct {
	ScrollID string          `json:"scroll_id,omitempty"`
	Hits     json.RawMessage `json:"hits"`
	TimedOut bool            `json:"timed_out"`
	Took     int             `json:"took"`
}
