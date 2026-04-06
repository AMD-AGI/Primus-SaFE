// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/mcp/unified"
)

// ======================== Endpoint Registration ========================

func init() {
	unified.Register(&unified.EndpointDef[DiagProfileRequest, DiagProfileResponse]{
		Name:        "workload_diag_profile",
		Description: "Get workload diagnostic profile: identity, intent result, GPU utilization summary, and pod overview in a single call",
		Group:       "diagnostic",
		HTTPMethod:  "GET",
		HTTPPath:    "/workload-diag/:uid/profile",
		MCPToolName: "diag_workload_profile",
		Handler:     handleDiagProfile,
	})

	unified.Register(&unified.EndpointDef[DiagPodsRequest, DiagPodsResponse]{
		Name:        "workload_diag_pods",
		Description: "List all pods for a workload with node placement, GPU allocation, running periods, and restart counts",
		Group:       "diagnostic",
		HTTPMethod:  "GET",
		HTTPPath:    "/workload-diag/:uid/pods",
		MCPToolName: "diag_pod_list",
		Handler:     handleDiagPods,
	})

	unified.Register(&unified.EndpointDef[DiagPodEventsRequest, DiagPodEventsResponse]{
		Name:        "workload_diag_pod_events",
		Description: "Get pod phase changes, restart history, and running period gaps for a workload",
		Group:       "diagnostic",
		HTTPMethod:  "GET",
		HTTPPath:    "/workload-diag/:uid/pod-events",
		MCPToolName: "diag_pod_events",
		Handler:     handleDiagPodEvents,
	})

	unified.Register(&unified.EndpointDef[DiagK8sEventsRequest, DiagK8sEventsResponse]{
		Name:        "workload_diag_k8s_events",
		Description: "Get K8s native events (FailedScheduling, OOMKilled, etc.) from OpenSearch for a workload",
		Group:       "diagnostic",
		HTTPMethod:  "GET",
		HTTPPath:    "/workload-diag/:uid/k8s-events",
		MCPToolName: "diag_k8s_events",
		Handler:     handleDiagK8sEvents,
	})

	unified.Register(&unified.EndpointDef[DiagGPUUtilRequest, DiagGPUUtilResponse]{
		Name:        "workload_diag_gpu_utilization",
		Description: "Get GPU utilization statistics (avg/p50/p90/p95/max/min) and optional time-series for a workload",
		Group:       "diagnostic",
		HTTPMethod:  "GET",
		HTTPPath:    "/workload-diag/:uid/gpu-utilization",
		MCPToolName: "diag_gpu_utilization",
		Handler:     handleDiagGPUUtil,
	})

	unified.Register(&unified.EndpointDef[DiagComputeMetricsRequest, DiagComputeMetricsResponse]{
		Name:        "workload_diag_compute_metrics",
		Description: "Get CPU, memory, disk IO time-series for a workload",
		Group:       "diagnostic",
		HTTPMethod:  "GET",
		HTTPPath:    "/workload-diag/:uid/compute-metrics",
		MCPToolName: "diag_compute_metrics",
		Handler:     handleDiagComputeMetrics,
	})

	unified.Register(&unified.EndpointDef[DiagNetworkMetricsRequest, DiagNetworkMetricsResponse]{
		Name:        "workload_diag_network_metrics",
		Description: "Get RDMA bandwidth, error counters, XGMI/PCIe metrics for a workload",
		Group:       "diagnostic",
		HTTPMethod:  "GET",
		HTTPPath:    "/workload-diag/:uid/network-metrics",
		MCPToolName: "diag_network_metrics",
		Handler:     handleDiagNetworkMetrics,
	})

	unified.Register(&unified.EndpointDef[DiagTrainingProgressRequest, DiagTrainingProgressResponse]{
		Name:        "workload_diag_training_progress",
		Description: "Get training iteration progress with loss, throughput, tflops, and iteration cadence analysis",
		Group:       "diagnostic",
		HTTPMethod:  "GET",
		HTTPPath:    "/workload-diag/:uid/training-progress",
		MCPToolName: "diag_training_progress",
		Handler:     handleDiagTrainingProgress,
	})

	unified.Register(&unified.EndpointDef[DiagCodeSnapshotRequest, DiagCodeSnapshotResponse]{
		Name:        "workload_diag_code_snapshot",
		Description: "Get code snapshot: entry script, config files, pip freeze, and working directory tree",
		Group:       "diagnostic",
		HTTPMethod:  "GET",
		HTTPPath:    "/workload-diag/:uid/code-snapshot",
		MCPToolName: "diag_code_snapshot",
		Handler:     handleDiagCodeSnapshot,
	})

	unified.Register(&unified.EndpointDef[DiagImageAnalysisRequest, DiagImageAnalysisResponse]{
		Name:        "workload_diag_image_analysis",
		Description: "Get container image analysis: layer history, installed packages, framework hints, and base image",
		Group:       "diagnostic",
		HTTPMethod:  "GET",
		HTTPPath:    "/workload-diag/:uid/image-analysis",
		MCPToolName: "diag_image_analysis",
		Handler:     handleDiagImageAnalysis,
	})

	unified.Register(&unified.EndpointDef[DiagEvidenceRequest, DiagEvidenceResponse]{
		Name:        "workload_diag_evidence",
		Description: "Get detection evidence: cmdline, env vars, process tree, and framework detection results",
		Group:       "diagnostic",
		HTTPMethod:  "GET",
		HTTPPath:    "/workload-diag/:uid/evidence",
		MCPToolName: "diag_evidence",
		Handler:     handleDiagEvidence,
	})

	unified.Register(&unified.EndpointDef[DiagProfilerFilesRequest, DiagProfilerFilesResponse]{
		Name:        "workload_diag_profiler_files",
		Description: "List available profiler trace files with download URLs for a workload",
		Group:       "diagnostic",
		HTTPMethod:  "GET",
		HTTPPath:    "/workload-diag/:uid/profiler-files",
		MCPToolName: "diag_profiler_files",
		Handler:     handleDiagProfilerFiles,
	})

	unified.Register(&unified.EndpointDef[DiagK8sSpecRequest, DiagK8sSpecResponse]{
		Name:        "workload_diag_k8s_spec",
		Description: "Get full K8s workload and pod spec/metadata for a workload",
		Group:       "diagnostic",
		HTTPMethod:  "GET",
		HTTPPath:    "/workload-diag/:uid/k8s-spec",
		MCPToolName: "diag_k8s_spec",
		Handler:     handleDiagK8sSpec,
	})
}

// ======================== Helpers ========================

type DiagBaseRequest struct {
	UID     string `json:"uid" param:"uid" mcp:"uid,description=Workload UID,required"`
	Cluster string `json:"cluster" query:"cluster" mcp:"cluster,description=Target cluster name (optional)"`
}

func diagProxy(ctx context.Context, cluster, uid, subpath string, params url.Values, dest interface{}) error {
	rc, err := getRobustClient(cluster)
	if err != nil {
		return err
	}
	raw, err := rc.GetRaw(ctx, "/workload-diag/"+uid+"/"+subpath, params)
	if err != nil {
		return fmt.Errorf("robust diag %s: %w", subpath, err)
	}
	if err := json.Unmarshal(raw, dest); err != nil {
		return fmt.Errorf("robust diag %s decode: %w", subpath, err)
	}
	return nil
}

// ======================== 1. Workload Profile ========================

type DiagProfileRequest struct {
	DiagBaseRequest
}

type DiagProfileResponse struct {
	Workload   *DiagWorkloadInfo   `json:"workload"`
	Intent     *DiagIntentInfo     `json:"intent,omitempty"`
	Resource   *DiagResourceInfo   `json:"resource,omitempty"`
	PodSummary *DiagPodSummaryInfo `json:"pod_summary,omitempty"`
	Children   []DiagChildInfo     `json:"children,omitempty"`
}

type DiagWorkloadInfo struct {
	UID          string      `json:"uid"`
	Name         string      `json:"name"`
	Namespace    string      `json:"namespace"`
	Kind         string      `json:"kind"`
	GroupVersion string      `json:"group_version"`
	Status       string      `json:"status"`
	GpuRequest   int32       `json:"gpu_request"`
	CreatedAt    string      `json:"created_at"`
	EndAt        string      `json:"end_at,omitempty"`
	Labels       interface{} `json:"labels,omitempty"`
	Annotations  interface{} `json:"annotations,omitempty"`
}

type DiagIntentInfo struct {
	Category         string  `json:"category,omitempty"`
	Framework        string  `json:"framework,omitempty"`
	WrapperFramework string  `json:"wrapper_framework,omitempty"`
	BaseFramework    string  `json:"base_framework,omitempty"`
	ModelFamily      string  `json:"model_family,omitempty"`
	ModelScale       string  `json:"model_scale,omitempty"`
	ModelVariant     string  `json:"model_variant,omitempty"`
	WorkloadType     string  `json:"workload_type,omitempty"`
	Confidence       float64 `json:"confidence,omitempty"`
	IntentSource     string  `json:"intent_source,omitempty"`
	ExpectedBehavior string  `json:"expected_behavior,omitempty"`
	IntentState      string  `json:"intent_state,omitempty"`
}

type DiagResourceInfo struct {
	GpuTimeSeconds    float64 `json:"gpu_time_seconds"`
	GpuModel          string  `json:"gpu_model,omitempty"`
	PodCount          int32   `json:"pod_count"`
	GpuUtilizationAvg float64 `json:"gpu_utilization_avg"`
	GpuUtilizationP50 float64 `json:"gpu_utilization_p50"`
	GpuUtilizationP90 float64 `json:"gpu_utilization_p90"`
}

type DiagPodSummaryInfo struct {
	Total        int `json:"total"`
	Running      int `json:"running"`
	RestartTotal int `json:"restart_total"`
}

type DiagChildInfo struct {
	UID  string `json:"uid"`
	Name string `json:"name"`
	Kind string `json:"kind"`
}

func handleDiagProfile(ctx context.Context, req *DiagProfileRequest) (*DiagProfileResponse, error) {
	var resp DiagProfileResponse
	if err := diagProxy(ctx, req.Cluster, req.UID, "profile", nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// ======================== 2. Pod List ========================

type DiagPodsRequest struct {
	DiagBaseRequest
}

type DiagPodsResponse struct {
	Pods []DiagPodInfo `json:"pods"`
}

type DiagPodInfo struct {
	UID            string          `json:"uid"`
	Name           string          `json:"name"`
	Namespace      string          `json:"namespace"`
	NodeName       string          `json:"node_name"`
	IP             string          `json:"ip"`
	GpuAllocated   int32           `json:"gpu_allocated"`
	GpuModel       string          `json:"gpu_model,omitempty"`
	Phase          string          `json:"phase"`
	Running        bool            `json:"running"`
	ContainerImage string          `json:"container_image"`
	RestartCount   int32           `json:"restart_count"`
	RunningPeriods []DiagRunPeriod `json:"running_periods,omitempty"`
}

type DiagRunPeriod struct {
	StartAt         string `json:"start_at"`
	EndAt           string `json:"end_at,omitempty"`
	DurationSeconds int64  `json:"duration_seconds,omitempty"`
}

func handleDiagPods(ctx context.Context, req *DiagPodsRequest) (*DiagPodsResponse, error) {
	var resp DiagPodsResponse
	if err := diagProxy(ctx, req.Cluster, req.UID, "pods", nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// ======================== 3. Pod Events & Restarts ========================

type DiagPodEventsRequest struct {
	DiagBaseRequest
	Since string `json:"since" query:"since" mcp:"since,description=Start time (RFC3339) defaults to 7 days ago"`
}

type DiagPodEventsResponse struct {
	Events         []DiagPodEvent  `json:"events"`
	RunningPeriods []DiagRunDetail `json:"running_periods"`
}

type DiagPodEvent struct {
	PodUID       string `json:"pod_uid"`
	PodName      string `json:"pod_name"`
	PodPhase     string `json:"pod_phase"`
	EventType    string `json:"event_type"`
	RestartCount int32  `json:"restart_count"`
	CreatedAt    string `json:"created_at"`
}

type DiagRunDetail struct {
	PodUID          string `json:"pod_uid"`
	PodName         string `json:"pod_name"`
	StartAt         string `json:"start_at"`
	EndAt           string `json:"end_at,omitempty"`
	DurationSeconds int64  `json:"duration_seconds"`
}

func handleDiagPodEvents(ctx context.Context, req *DiagPodEventsRequest) (*DiagPodEventsResponse, error) {
	var p url.Values
	if req.Since != "" {
		p = url.Values{"since": {req.Since}}
	}
	var resp DiagPodEventsResponse
	if err := diagProxy(ctx, req.Cluster, req.UID, "pod-events", p, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// ======================== 4. K8s Events (OpenSearch) ========================

type DiagK8sEventsRequest struct {
	DiagBaseRequest
	Type  string `json:"type" query:"type" mcp:"type,description=Filter: Normal or Warning"`
	Since string `json:"since" query:"since" mcp:"since,description=Start time (RFC3339)"`
}

type DiagK8sEventsResponse struct {
	Events []DiagK8sEvent `json:"events"`
}

type DiagK8sEvent struct {
	Type           string                 `json:"type"`
	Reason         string                 `json:"reason"`
	Message        string                 `json:"message"`
	InvolvedObject map[string]interface{} `json:"involved_object"`
	Count          int32                  `json:"count"`
	FirstTimestamp string                 `json:"first_timestamp"`
	LastTimestamp   string                 `json:"last_timestamp"`
	Source         string                 `json:"source"`
}

func handleDiagK8sEvents(ctx context.Context, req *DiagK8sEventsRequest) (*DiagK8sEventsResponse, error) {
	p := url.Values{}
	if req.Type != "" {
		p.Set("type", req.Type)
	}
	if req.Since != "" {
		p.Set("since", req.Since)
	}
	var params url.Values
	if len(p) > 0 {
		params = p
	}
	var resp DiagK8sEventsResponse
	if err := diagProxy(ctx, req.Cluster, req.UID, "k8s-events", params, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// ======================== 5. GPU Utilization ========================

type DiagGPUUtilRequest struct {
	DiagBaseRequest
	Start string `json:"start" query:"start" mcp:"start,description=Time range start (RFC3339)"`
	End   string `json:"end" query:"end" mcp:"end,description=Time range end (RFC3339)"`
	Step  string `json:"step" query:"step" mcp:"step,description=Step interval in seconds (default 60)"`
}

type DiagGPUUtilResponse struct {
	Summary    *DiagGPUSummary        `json:"summary"`
	TimeSeries map[string]interface{} `json:"time_series,omitempty"`
}

type DiagGPUSummary struct {
	Avg               float64 `json:"avg"`
	P50               float64 `json:"p50"`
	P90               float64 `json:"p90"`
	P95               float64 `json:"p95"`
	Max               float64 `json:"max"`
	Min               float64 `json:"min"`
	SampleCount       int32   `json:"sample_count"`
	AllocatedGpuCount float64 `json:"allocated_gpu_count"`
}

func handleDiagGPUUtil(ctx context.Context, req *DiagGPUUtilRequest) (*DiagGPUUtilResponse, error) {
	p := url.Values{}
	if req.Start != "" {
		p.Set("start", req.Start)
	}
	if req.End != "" {
		p.Set("end", req.End)
	}
	if req.Step != "" {
		p.Set("step", req.Step)
	}
	var params url.Values
	if len(p) > 0 {
		params = p
	}
	var resp DiagGPUUtilResponse
	if err := diagProxy(ctx, req.Cluster, req.UID, "gpu-utilization", params, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// ======================== 6. Compute Metrics ========================

type DiagComputeMetricsRequest struct {
	DiagBaseRequest
	Start string `json:"start" query:"start" mcp:"start,description=Time range start (RFC3339),required"`
	End   string `json:"end" query:"end" mcp:"end,description=Time range end (RFC3339),required"`
	Step  string `json:"step" query:"step" mcp:"step,description=Step interval in seconds (default 60)"`
}

type DiagComputeMetricsResponse struct {
	TimeSeries map[string]interface{} `json:"time_series"`
}

func handleDiagComputeMetrics(ctx context.Context, req *DiagComputeMetricsRequest) (*DiagComputeMetricsResponse, error) {
	p := url.Values{}
	if req.Start != "" {
		p.Set("start", req.Start)
	}
	if req.End != "" {
		p.Set("end", req.End)
	}
	if req.Step != "" {
		p.Set("step", req.Step)
	}
	var params url.Values
	if len(p) > 0 {
		params = p
	}
	var resp DiagComputeMetricsResponse
	if err := diagProxy(ctx, req.Cluster, req.UID, "compute-metrics", params, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// ======================== 7. Network Metrics ========================

type DiagNetworkMetricsRequest struct {
	DiagBaseRequest
	Start string `json:"start" query:"start" mcp:"start,description=Time range start (RFC3339),required"`
	End   string `json:"end" query:"end" mcp:"end,description=Time range end (RFC3339),required"`
	Step  string `json:"step" query:"step" mcp:"step,description=Step interval in seconds (default 60)"`
}

type DiagNetworkMetricsResponse struct {
	RDMA         map[string]interface{} `json:"rdma,omitempty"`
	RDMAErrors   map[string]interface{} `json:"rdma_errors,omitempty"`
	Interconnect map[string]interface{} `json:"interconnect,omitempty"`
}

func handleDiagNetworkMetrics(ctx context.Context, req *DiagNetworkMetricsRequest) (*DiagNetworkMetricsResponse, error) {
	p := url.Values{}
	if req.Start != "" {
		p.Set("start", req.Start)
	}
	if req.End != "" {
		p.Set("end", req.End)
	}
	if req.Step != "" {
		p.Set("step", req.Step)
	}
	var params url.Values
	if len(p) > 0 {
		params = p
	}
	var resp DiagNetworkMetricsResponse
	if err := diagProxy(ctx, req.Cluster, req.UID, "network-metrics", params, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// ======================== 8. Training Progress ========================

type DiagTrainingProgressRequest struct {
	DiagBaseRequest
	Start      string `json:"start" query:"start" mcp:"start,description=Time range start (RFC3339)"`
	End        string `json:"end" query:"end" mcp:"end,description=Time range end (RFC3339)"`
	DataSource string `json:"data_source" query:"data_source" mcp:"data_source,description=Filter: log or wandb or tensorflow"`
	PageNum    int    `json:"page_num" query:"page_num" mcp:"page_num,description=Page number (default 1)"`
	PageSize   int    `json:"page_size" query:"page_size" mcp:"page_size,description=Items per page (default 100)"`
}

type DiagTrainingProgressResponse struct {
	Total            int                    `json:"total"`
	DataPoints       []DiagTrainingDataPoint `json:"data_points"`
	IterationCadence *DiagIterationCadence  `json:"iteration_cadence,omitempty"`
}

type DiagTrainingDataPoint struct {
	Iteration  int32                  `json:"iteration"`
	Timestamp  string                 `json:"timestamp"`
	DataSource string                 `json:"data_source"`
	Metrics    map[string]interface{} `json:"metrics"`
}

type DiagIterationCadence struct {
	AvgSecondsPerIteration float64 `json:"avg_seconds_per_iteration"`
	MinGapSeconds          float64 `json:"min_gap_seconds"`
	MaxGapSeconds          float64 `json:"max_gap_seconds"`
	StallCount             int     `json:"stall_count"`
	StallThresholdSeconds  float64 `json:"stall_threshold_seconds"`
}

func handleDiagTrainingProgress(ctx context.Context, req *DiagTrainingProgressRequest) (*DiagTrainingProgressResponse, error) {
	p := url.Values{}
	if req.Start != "" {
		p.Set("start", req.Start)
	}
	if req.End != "" {
		p.Set("end", req.End)
	}
	if req.DataSource != "" {
		p.Set("data_source", req.DataSource)
	}
	if req.PageNum > 0 {
		p.Set("page_num", strconv.Itoa(req.PageNum))
	}
	if req.PageSize > 0 {
		p.Set("page_size", strconv.Itoa(req.PageSize))
	}
	var params url.Values
	if len(p) > 0 {
		params = p
	}
	var resp DiagTrainingProgressResponse
	if err := diagProxy(ctx, req.Cluster, req.UID, "training-progress", params, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// ======================== 9. Code Snapshot ========================

type DiagCodeSnapshotRequest struct {
	DiagBaseRequest
	IncludeContent string `json:"include_content" query:"include_content" mcp:"include_content,description=Include file content (default true)"`
}

type DiagCodeSnapshotResponse struct {
	CapturedAt     string      `json:"captured_at,omitempty"`
	Fingerprint    string      `json:"fingerprint"`
	FileCount      int         `json:"file_count"`
	TotalSize      int         `json:"total_size"`
	DownloadURL    string      `json:"download_url,omitempty" mcp:"download_url,description=Relative URL to download source files as tar.gz"`
	EntryScript    interface{} `json:"entry_script,omitempty"`
	ConfigFiles    interface{} `json:"config_files,omitempty"`
	LocalModules   interface{} `json:"local_modules,omitempty"`
	PipFreeze      string      `json:"pip_freeze,omitempty"`
	WorkingDirTree string      `json:"working_dir_tree,omitempty"`
}

func handleDiagCodeSnapshot(ctx context.Context, req *DiagCodeSnapshotRequest) (*DiagCodeSnapshotResponse, error) {
	var p url.Values
	if req.IncludeContent != "" {
		p = url.Values{"include_content": {req.IncludeContent}}
	}
	var resp DiagCodeSnapshotResponse
	if err := diagProxy(ctx, req.Cluster, req.UID, "code-snapshot", p, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// ======================== 10. Image Analysis ========================

type DiagImageAnalysisRequest struct {
	DiagBaseRequest
}

type DiagImageAnalysisResponse struct {
	ImageRef          string      `json:"image_ref"`
	Digest            string      `json:"digest,omitempty"`
	BaseImage         string      `json:"base_image,omitempty"`
	TotalSize         int64       `json:"total_size"`
	ImageCreatedAt    string      `json:"image_created_at,omitempty"`
	LayerHistory      interface{} `json:"layer_history,omitempty"`
	InstalledPackages interface{} `json:"installed_packages,omitempty"`
	FrameworkHints    interface{} `json:"framework_hints,omitempty"`
	ImageEnv          interface{} `json:"image_env,omitempty"`
	ImageEntrypoint   string      `json:"image_entrypoint,omitempty"`
	ImageLabels       interface{} `json:"image_labels,omitempty"`
}

func handleDiagImageAnalysis(ctx context.Context, req *DiagImageAnalysisRequest) (*DiagImageAnalysisResponse, error) {
	var resp DiagImageAnalysisResponse
	if err := diagProxy(ctx, req.Cluster, req.UID, "image-analysis", nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// ======================== 11. Detection Evidence ========================

type DiagEvidenceRequest struct {
	DiagBaseRequest
	Source string `json:"source" query:"source" mcp:"source,description=Filter by source: process/env/image/log/label/active_detection"`
}

type DiagEvidenceResponse struct {
	EvidenceCount   int                `json:"evidence_count"`
	EvidenceSources []string           `json:"evidence_sources"`
	Evidence        []DiagEvidenceItem `json:"evidence"`
}

type DiagEvidenceItem struct {
	Source           string      `json:"source"`
	SourceType       string      `json:"source_type"`
	Framework        string      `json:"framework"`
	WorkloadType     string      `json:"workload_type,omitempty"`
	Confidence       float64     `json:"confidence"`
	DetectedAt       string      `json:"detected_at"`
	Evidence         interface{} `json:"evidence,omitempty"`
	WrapperFramework string      `json:"wrapper_framework,omitempty"`
	BaseFramework    string      `json:"base_framework,omitempty"`
}

func handleDiagEvidence(ctx context.Context, req *DiagEvidenceRequest) (*DiagEvidenceResponse, error) {
	var p url.Values
	if req.Source != "" {
		p = url.Values{"source": {req.Source}}
	}
	var resp DiagEvidenceResponse
	if err := diagProxy(ctx, req.Cluster, req.UID, "evidence", p, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// ======================== 12. Profiler Files ========================

type DiagProfilerFilesRequest struct {
	DiagBaseRequest
}

type DiagProfilerFilesResponse struct {
	Files []DiagProfilerFile `json:"files"`
}

type DiagProfilerFile struct {
	FileName    string `json:"file_name"`
	FilePath    string `json:"file_path"`
	FileType    string `json:"file_type"`
	FileSize    int64  `json:"file_size"`
	DownloadURL string `json:"download_url,omitempty"`
	PodName     string `json:"pod_name"`
	SourcePid   int32  `json:"source_pid,omitempty"`
	DetectedAt  string `json:"detected_at"`
}

func handleDiagProfilerFiles(ctx context.Context, req *DiagProfilerFilesRequest) (*DiagProfilerFilesResponse, error) {
	var resp DiagProfilerFilesResponse
	if err := diagProxy(ctx, req.Cluster, req.UID, "profiler-files", nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// ======================== 13. K8s Spec ========================

type DiagK8sSpecRequest struct {
	DiagBaseRequest
}

type DiagK8sSpecResponse struct {
	WorkloadSpec *DiagWorkloadSpec `json:"workload_spec,omitempty"`
	PodSpecs     []DiagPodSpec    `json:"pod_specs,omitempty"`
}

type DiagWorkloadSpec struct {
	GroupVersion string      `json:"group_version"`
	Kind         string      `json:"kind"`
	Detail       interface{} `json:"detail,omitempty"`
}

type DiagPodSpec struct {
	PodUID  string      `json:"pod_uid"`
	PodName string      `json:"pod_name"`
	Spec    interface{} `json:"spec,omitempty"`
	Status  interface{} `json:"status,omitempty"`
}

func handleDiagK8sSpec(ctx context.Context, req *DiagK8sSpecRequest) (*DiagK8sSpecResponse, error) {
	var resp DiagK8sSpecResponse
	if err := diagProxy(ctx, req.Cluster, req.UID, "k8s-spec", nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}
