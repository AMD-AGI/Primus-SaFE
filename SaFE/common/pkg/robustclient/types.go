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
