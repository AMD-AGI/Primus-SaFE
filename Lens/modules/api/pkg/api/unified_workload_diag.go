// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package api

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	dbmodel "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/errors"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/helper/prom"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/mcp/unified"
)

// ======================== Endpoint Registration ========================

func init() {
	// 1. Workload Profile
	unified.Register(&unified.EndpointDef[DiagProfileRequest, DiagProfileResponse]{
		Name:        "workload_diag_profile",
		Description: "Get workload diagnostic profile: identity, intent result, GPU utilization summary, and pod overview in a single call",
		Group:       "diagnostic",
		HTTPMethod:  "GET",
		HTTPPath:    "/workload-diag/:uid/profile",
		MCPToolName: "diag_workload_profile",
		Handler:     handleDiagProfile,
	})

	// 2. Pod List
	unified.Register(&unified.EndpointDef[DiagPodsRequest, DiagPodsResponse]{
		Name:        "workload_diag_pods",
		Description: "List all pods for a workload with node placement, GPU allocation, running periods, and restart counts",
		Group:       "diagnostic",
		HTTPMethod:  "GET",
		HTTPPath:    "/workload-diag/:uid/pods",
		MCPToolName: "diag_pod_list",
		Handler:     handleDiagPods,
	})

	// 3. Pod Events & Restarts
	unified.Register(&unified.EndpointDef[DiagPodEventsRequest, DiagPodEventsResponse]{
		Name:        "workload_diag_pod_events",
		Description: "Get pod phase changes, restart history, and running period gaps for a workload",
		Group:       "diagnostic",
		HTTPMethod:  "GET",
		HTTPPath:    "/workload-diag/:uid/pod-events",
		MCPToolName: "diag_pod_events",
		Handler:     handleDiagPodEvents,
	})

	// 4. K8s Events (OpenSearch)
	unified.Register(&unified.EndpointDef[DiagK8sEventsRequest, DiagK8sEventsResponse]{
		Name:        "workload_diag_k8s_events",
		Description: "Get K8s native events (FailedScheduling, OOMKilled, etc.) from OpenSearch for a workload",
		Group:       "diagnostic",
		HTTPMethod:  "GET",
		HTTPPath:    "/workload-diag/:uid/k8s-events",
		MCPToolName: "diag_k8s_events",
		Handler:     handleDiagK8sEvents,
	})

	// 5. GPU Utilization
	unified.Register(&unified.EndpointDef[DiagGPUUtilRequest, DiagGPUUtilResponse]{
		Name:        "workload_diag_gpu_utilization",
		Description: "Get GPU utilization statistics (avg/p50/p90/p95/max/min) and optional time-series for a workload",
		Group:       "diagnostic",
		HTTPMethod:  "GET",
		HTTPPath:    "/workload-diag/:uid/gpu-utilization",
		MCPToolName: "diag_gpu_utilization",
		Handler:     handleDiagGPUUtil,
	})

	// 6. Compute Metrics
	unified.Register(&unified.EndpointDef[DiagComputeMetricsRequest, DiagComputeMetricsResponse]{
		Name:        "workload_diag_compute_metrics",
		Description: "Get CPU, memory, disk IO time-series for a workload",
		Group:       "diagnostic",
		HTTPMethod:  "GET",
		HTTPPath:    "/workload-diag/:uid/compute-metrics",
		MCPToolName: "diag_compute_metrics",
		Handler:     handleDiagComputeMetrics,
	})

	// 7. Network Metrics
	unified.Register(&unified.EndpointDef[DiagNetworkMetricsRequest, DiagNetworkMetricsResponse]{
		Name:        "workload_diag_network_metrics",
		Description: "Get RDMA bandwidth, error counters, XGMI/PCIe metrics for a workload",
		Group:       "diagnostic",
		HTTPMethod:  "GET",
		HTTPPath:    "/workload-diag/:uid/network-metrics",
		MCPToolName: "diag_network_metrics",
		Handler:     handleDiagNetworkMetrics,
	})

	// 8. Training Progress
	unified.Register(&unified.EndpointDef[DiagTrainingProgressRequest, DiagTrainingProgressResponse]{
		Name:        "workload_diag_training_progress",
		Description: "Get training iteration progress with loss, throughput, tflops, and iteration cadence analysis",
		Group:       "diagnostic",
		HTTPMethod:  "GET",
		HTTPPath:    "/workload-diag/:uid/training-progress",
		MCPToolName: "diag_training_progress",
		Handler:     handleDiagTrainingProgress,
	})

	// 9. Code Snapshot
	unified.Register(&unified.EndpointDef[DiagCodeSnapshotRequest, DiagCodeSnapshotResponse]{
		Name:        "workload_diag_code_snapshot",
		Description: "Get code snapshot: entry script, config files, pip freeze, and working directory tree",
		Group:       "diagnostic",
		HTTPMethod:  "GET",
		HTTPPath:    "/workload-diag/:uid/code-snapshot",
		MCPToolName: "diag_code_snapshot",
		Handler:     handleDiagCodeSnapshot,
	})

	// 10. Image Analysis
	unified.Register(&unified.EndpointDef[DiagImageAnalysisRequest, DiagImageAnalysisResponse]{
		Name:        "workload_diag_image_analysis",
		Description: "Get container image analysis: layer history, installed packages, framework hints, and base image",
		Group:       "diagnostic",
		HTTPMethod:  "GET",
		HTTPPath:    "/workload-diag/:uid/image-analysis",
		MCPToolName: "diag_image_analysis",
		Handler:     handleDiagImageAnalysis,
	})

	// 11. Detection Evidence
	unified.Register(&unified.EndpointDef[DiagEvidenceRequest, DiagEvidenceResponse]{
		Name:        "workload_diag_evidence",
		Description: "Get detection evidence: cmdline, env vars, process tree, and framework detection results",
		Group:       "diagnostic",
		HTTPMethod:  "GET",
		HTTPPath:    "/workload-diag/:uid/evidence",
		MCPToolName: "diag_evidence",
		Handler:     handleDiagEvidence,
	})

	// 12. Profiler Files
	unified.Register(&unified.EndpointDef[DiagProfilerFilesRequest, DiagProfilerFilesResponse]{
		Name:        "workload_diag_profiler_files",
		Description: "List available profiler trace files with download URLs for a workload",
		Group:       "diagnostic",
		HTTPMethod:  "GET",
		HTTPPath:    "/workload-diag/:uid/profiler-files",
		MCPToolName: "diag_profiler_files",
		Handler:     handleDiagProfilerFiles,
	})

	// 13. K8s Spec
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

// ======================== Common ========================

type diagBaseRequest struct {
	UID     string `json:"uid" param:"uid" mcp:"uid,description=Workload UID,required"`
	Cluster string `json:"cluster" query:"cluster" mcp:"cluster,description=Target cluster name (optional)"`
}

func diagResolveFacade(ctx context.Context, uid string, cluster string) (database.FacadeInterface, string, error) {
	if uid == "" {
		return nil, "", errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("uid is required")
	}
	clusterName, err := resolveWorkloadCluster(ctx, uid, cluster)
	if err != nil {
		return nil, "", err
	}
	return database.GetFacadeForCluster(clusterName), clusterName, nil
}

func diagResolveStorage(ctx context.Context, uid string, cluster string) (*clientsets.StorageClientSet, string, error) {
	if uid == "" {
		return nil, "", errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("uid is required")
	}
	clients, err := getClusterClientsForWorkload(ctx, uid, cluster)
	if err != nil {
		return nil, "", err
	}
	return clients.StorageClientSet, clients.ClusterName, nil
}

func parseDiagTimeRange(startStr, endStr string) (time.Time, time.Time, error) {
	now := time.Now()
	start := now.Add(-24 * time.Hour)
	end := now
	var err error
	if startStr != "" {
		start, err = time.Parse(time.RFC3339, startStr)
		if err != nil {
			return time.Time{}, time.Time{}, errors.WrapError(err, "invalid start time format, use RFC3339", errors.RequestParameterInvalid)
		}
	}
	if endStr != "" {
		end, err = time.Parse(time.RFC3339, endStr)
		if err != nil {
			return time.Time{}, time.Time{}, errors.WrapError(err, "invalid end time format, use RFC3339", errors.RequestParameterInvalid)
		}
	}
	return start, end, nil
}

func parseDiagStep(stepStr string) int {
	if stepStr == "" {
		return 60
	}
	v, err := strconv.Atoi(stepStr)
	if err != nil || v <= 0 {
		return 60
	}
	return v
}

// ======================== 1. Workload Profile ========================

type DiagProfileRequest struct {
	diagBaseRequest
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
	GpuTimeSeconds   float64 `json:"gpu_time_seconds"`
	GpuModel         string  `json:"gpu_model,omitempty"`
	PodCount         int32   `json:"pod_count"`
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
	facade, _, err := diagResolveFacade(ctx, req.UID, req.Cluster)
	if err != nil {
		return nil, err
	}

	resp := &DiagProfileResponse{}

	// Workload identity
	wl, err := facade.GetWorkload().GetGpuWorkloadByUid(ctx, req.UID)
	if err != nil {
		return nil, errors.NewError().WithCode(errors.InternalError).WithMessage("failed to get workload: " + err.Error())
	}
	if wl == nil {
		return nil, errors.NewError().WithCode(errors.RequestDataNotExisted).WithMessage("workload not found: " + req.UID)
	}
	resp.Workload = &DiagWorkloadInfo{
		UID:          wl.UID,
		Name:         wl.Name,
		Namespace:    wl.Namespace,
		Kind:         wl.Kind,
		GroupVersion: wl.GroupVersion,
		Status:       wl.Status,
		GpuRequest:   wl.GpuRequest,
		CreatedAt:    wl.CreatedAt.Format(time.RFC3339),
		Labels:       wl.Labels,
		Annotations:  wl.Annotations,
	}
	if !wl.EndAt.IsZero() {
		resp.Workload.EndAt = wl.EndAt.Format(time.RFC3339)
	}

	// Intent
	det, err := facade.GetWorkloadDetection().GetDetection(ctx, req.UID)
	if err == nil && det != nil {
		intent := &DiagIntentInfo{
			Framework:        det.Framework,
			WrapperFramework: det.WrapperFramework,
			BaseFramework:    det.BaseFramework,
			WorkloadType:     det.WorkloadType,
			Confidence:       det.Confidence,
		}
		if det.Category != nil {
			intent.Category = *det.Category
		}
		if det.ModelFamily != nil {
			intent.ModelFamily = *det.ModelFamily
		}
		if det.ModelScale != nil {
			intent.ModelScale = *det.ModelScale
		}
		if det.ModelVariant != nil {
			intent.ModelVariant = *det.ModelVariant
		}
		if det.IntentSource != nil {
			intent.IntentSource = *det.IntentSource
		}
		if det.ExpectedBehavior != nil {
			intent.ExpectedBehavior = *det.ExpectedBehavior
		}
		if det.IntentState != nil {
			intent.IntentState = *det.IntentState
		}
		resp.Intent = intent
	}

	// Resource
	res, err := facade.GetWorkloadResource().GetByWorkloadUID(ctx, req.UID)
	if err == nil && res != nil {
		resp.Resource = &DiagResourceInfo{
			GpuTimeSeconds: res.GpuTimeSeconds,
			GpuModel:       res.GpuModel,
			PodCount:       res.PodCount,
		}
	}
	stat, err := facade.GetWorkloadStatistic().GetByUID(ctx, req.UID)
	if err == nil && stat != nil {
		if resp.Resource == nil {
			resp.Resource = &DiagResourceInfo{}
		}
		resp.Resource.GpuUtilizationAvg = stat.AvgGpuUtilization
		resp.Resource.GpuUtilizationP50 = stat.P50GpuUtilization
		resp.Resource.GpuUtilizationP90 = stat.P90GpuUtilization
	}

	// Pod summary
	podRefs, err := facade.GetWorkload().ListWorkloadPodReferenceByWorkloadUid(ctx, req.UID)
	if err == nil && len(podRefs) > 0 {
		podUIDs := make([]string, 0, len(podRefs))
		for _, ref := range podRefs {
			podUIDs = append(podUIDs, ref.PodUID)
		}
		pods, _ := facade.GetPod().ListPodsByUids(ctx, podUIDs)
		summary := &DiagPodSummaryInfo{Total: len(pods)}
		for _, p := range pods {
			if p.Running {
				summary.Running++
			}
		}
		for _, podUID := range podUIDs {
			events, _ := facade.GetPod().ListPodEventsByUID(ctx, podUID)
			var maxRestart int32
			for _, e := range events {
				if e.RestartCount > maxRestart {
					maxRestart = e.RestartCount
				}
			}
			summary.RestartTotal += int(maxRestart)
		}
		resp.PodSummary = summary
	}

	// Children
	children, err := facade.GetWorkload().ListChildrenWorkloadByParentUid(ctx, req.UID)
	if err == nil && len(children) > 0 {
		for _, c := range children {
			resp.Children = append(resp.Children, DiagChildInfo{
				UID:  c.UID,
				Name: c.Name,
				Kind: c.Kind,
			})
		}
	}

	return resp, nil
}

// ======================== 2. Pod List ========================

type DiagPodsRequest struct {
	diagBaseRequest
}

type DiagPodsResponse struct {
	Pods []DiagPodInfo `json:"pods"`
}

type DiagPodInfo struct {
	UID            string           `json:"uid"`
	Name           string           `json:"name"`
	Namespace      string           `json:"namespace"`
	NodeName       string           `json:"node_name"`
	IP             string           `json:"ip"`
	GpuAllocated   int32            `json:"gpu_allocated"`
	GpuModel       string           `json:"gpu_model,omitempty"`
	Phase          string           `json:"phase"`
	Running        bool             `json:"running"`
	ContainerImage string           `json:"container_image"`
	RestartCount   int32            `json:"restart_count"`
	RunningPeriods []DiagRunPeriod  `json:"running_periods,omitempty"`
}

type DiagRunPeriod struct {
	StartAt         string `json:"start_at"`
	EndAt           string `json:"end_at,omitempty"`
	DurationSeconds int64  `json:"duration_seconds,omitempty"`
}

func handleDiagPods(ctx context.Context, req *DiagPodsRequest) (*DiagPodsResponse, error) {
	facade, _, err := diagResolveFacade(ctx, req.UID, req.Cluster)
	if err != nil {
		return nil, err
	}

	podRefs, err := facade.GetWorkload().ListWorkloadPodReferenceByWorkloadUid(ctx, req.UID)
	if err != nil {
		return nil, errors.NewError().WithCode(errors.InternalError).WithMessage("failed to list pod references: " + err.Error())
	}
	if len(podRefs) == 0 {
		return &DiagPodsResponse{Pods: []DiagPodInfo{}}, nil
	}

	podUIDs := make([]string, 0, len(podRefs))
	for _, ref := range podRefs {
		podUIDs = append(podUIDs, ref.PodUID)
	}

	pods, err := facade.GetPod().ListPodsByUids(ctx, podUIDs)
	if err != nil {
		return nil, errors.NewError().WithCode(errors.InternalError).WithMessage("failed to list pods: " + err.Error())
	}

	resourceMap := make(map[string]*dbmodel.PodResource)
	resources, _ := facade.GetPod().ListPodResourcesByUids(ctx, podUIDs)
	for _, r := range resources {
		resourceMap[r.UID] = r
	}

	result := make([]DiagPodInfo, 0, len(pods))
	for _, p := range pods {
		info := DiagPodInfo{
			UID:            p.UID,
			Name:           p.Name,
			Namespace:      p.Namespace,
			NodeName:       p.NodeName,
			IP:             p.IP,
			GpuAllocated:   p.GpuAllocated,
			Phase:          p.Phase,
			Running:        p.Running,
			ContainerImage: p.ContainerImage,
		}
		if r, ok := resourceMap[p.UID]; ok {
			info.GpuModel = r.GpuModel
		}

		// Restart count from events
		events, _ := facade.GetPod().ListPodEventsByUID(ctx, p.UID)
		if len(events) > 0 {
			info.RestartCount = events[len(events)-1].RestartCount
		}

		// Running periods
		period, _ := facade.GetPodRunningPeriods().GetCurrentRunningPeriod(ctx, p.UID)
		if period != nil {
			rp := DiagRunPeriod{StartAt: period.StartAt.Format(time.RFC3339)}
			if !period.EndAt.IsZero() {
				rp.EndAt = period.EndAt.Format(time.RFC3339)
				rp.DurationSeconds = int64(period.EndAt.Sub(period.StartAt).Seconds())
			} else {
				rp.DurationSeconds = int64(time.Since(period.StartAt).Seconds())
			}
			info.RunningPeriods = []DiagRunPeriod{rp}
		}

		result = append(result, info)
	}

	return &DiagPodsResponse{Pods: result}, nil
}

// ======================== 3. Pod Events & Restarts ========================

type DiagPodEventsRequest struct {
	diagBaseRequest
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
	facade, _, err := diagResolveFacade(ctx, req.UID, req.Cluster)
	if err != nil {
		return nil, err
	}

	sinceTime := time.Now().Add(-7 * 24 * time.Hour)
	if req.Since != "" {
		sinceTime, err = time.Parse(time.RFC3339, req.Since)
		if err != nil {
			return nil, errors.WrapError(err, "invalid since time format", errors.RequestParameterInvalid)
		}
	}

	podRefs, err := facade.GetWorkload().ListWorkloadPodReferenceByWorkloadUid(ctx, req.UID)
	if err != nil {
		return nil, errors.NewError().WithCode(errors.InternalError).WithMessage("failed to list pod references: " + err.Error())
	}

	podNameMap := make(map[string]string)
	if len(podRefs) > 0 {
		podUIDs := make([]string, 0, len(podRefs))
		for _, ref := range podRefs {
			podUIDs = append(podUIDs, ref.PodUID)
		}
		pods, _ := facade.GetPod().ListPodsByUids(ctx, podUIDs)
		for _, p := range pods {
			podNameMap[p.UID] = p.Name
		}
	}

	var allEvents []DiagPodEvent
	var allPeriods []DiagRunDetail

	for _, ref := range podRefs {
		podName := podNameMap[ref.PodUID]

		events, _ := facade.GetPod().ListPodEventsByUID(ctx, ref.PodUID)
		for _, e := range events {
			if e.CreatedAt.Before(sinceTime) {
				continue
			}
			allEvents = append(allEvents, DiagPodEvent{
				PodUID:       ref.PodUID,
				PodName:      podName,
				PodPhase:     e.PodPhase,
				EventType:    e.EventType,
				RestartCount: e.RestartCount,
				CreatedAt:    e.CreatedAt.Format(time.RFC3339),
			})
		}

		periods, _ := facade.GetPodRunningPeriods().ListRunningPeriodsInTimeRangeByPodUIDs(ctx, []string{ref.PodUID}, sinceTime, time.Now())
		for _, p := range periods {
			detail := DiagRunDetail{
				PodUID:  ref.PodUID,
				PodName: podName,
				StartAt: p.StartAt.Format(time.RFC3339),
			}
			if !p.EndAt.IsZero() {
				detail.EndAt = p.EndAt.Format(time.RFC3339)
				detail.DurationSeconds = int64(p.EndAt.Sub(p.StartAt).Seconds())
			} else {
				detail.DurationSeconds = int64(time.Since(p.StartAt).Seconds())
			}
			allPeriods = append(allPeriods, detail)
		}
	}

	return &DiagPodEventsResponse{
		Events:         allEvents,
		RunningPeriods: allPeriods,
	}, nil
}

// ======================== 4. K8s Events (OpenSearch) ========================

type DiagK8sEventsRequest struct {
	diagBaseRequest
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
	LastTimestamp  string                 `json:"last_timestamp"`
	Source         string                 `json:"source"`
}

func handleDiagK8sEvents(ctx context.Context, req *DiagK8sEventsRequest) (*DiagK8sEventsResponse, error) {
	storage, _, err := diagResolveStorage(ctx, req.UID, req.Cluster)
	if err != nil {
		return nil, err
	}
	if storage == nil || storage.OpenSearch == nil {
		return &DiagK8sEventsResponse{Events: []DiagK8sEvent{}}, nil
	}

	facade, _, err := diagResolveFacade(ctx, req.UID, req.Cluster)
	if err != nil {
		return nil, err
	}

	// Collect pod UIDs for this workload
	podRefs, _ := facade.GetWorkload().ListWorkloadPodReferenceByWorkloadUid(ctx, req.UID)
	podUIDs := make([]string, 0, len(podRefs)+1)
	podUIDs = append(podUIDs, req.UID)
	podNames := make([]string, 0)
	if len(podRefs) > 0 {
		uids := make([]string, 0, len(podRefs))
		for _, ref := range podRefs {
			uids = append(uids, ref.PodUID)
		}
		pods, _ := facade.GetPod().ListPodsByUids(ctx, uids)
		for _, p := range pods {
			podUIDs = append(podUIDs, p.UID)
			podNames = append(podNames, p.Name)
		}
	}

	sinceTime := time.Now().Add(-7 * 24 * time.Hour)
	if req.Since != "" {
		if t, err := time.Parse(time.RFC3339, req.Since); err == nil {
			sinceTime = t
		}
	}

	// Build OpenSearch query: match events involving these pods
	shouldClauses := make([]map[string]interface{}, 0)
	for _, uid := range podUIDs {
		shouldClauses = append(shouldClauses, map[string]interface{}{
			"match": map[string]interface{}{"involvedObject.uid": uid},
		})
	}
	for _, name := range podNames {
		shouldClauses = append(shouldClauses, map[string]interface{}{
			"match": map[string]interface{}{"involvedObject.name": name},
		})
	}

	must := []map[string]interface{}{
		{"bool": map[string]interface{}{"should": shouldClauses, "minimum_should_match": 1}},
		{"range": map[string]interface{}{"metadata.creationTimestamp": map[string]interface{}{"gte": sinceTime.Format(time.RFC3339)}}},
	}
	if req.Type != "" {
		must = append(must, map[string]interface{}{"match": map[string]interface{}{"type": req.Type}})
	}

	query := map[string]interface{}{
		"size": 500,
		"query": map[string]interface{}{
			"bool": map[string]interface{}{"must": must},
		},
		"sort": []map[string]interface{}{
			{"metadata.creationTimestamp": map[string]interface{}{"order": "desc"}},
		},
	}

	queryBytes, _ := json.Marshal(query)
	res, err := storage.OpenSearch.Search(
		storage.OpenSearch.Search.WithContext(ctx),
		storage.OpenSearch.Search.WithIndex("kubernetes-events-*"),
		storage.OpenSearch.Search.WithBody(strings.NewReader(string(queryBytes))),
	)
	if err != nil {
		log.Warnf("OpenSearch query failed for workload %s: %v", req.UID, err)
		return &DiagK8sEventsResponse{Events: []DiagK8sEvent{}}, nil
	}
	defer res.Body.Close()

	if res.IsError() {
		log.Warnf("OpenSearch returned error for workload %s: %s", req.UID, res.String())
		return &DiagK8sEventsResponse{Events: []DiagK8sEvent{}}, nil
	}

	var osResp struct {
		Hits struct {
			Hits []struct {
				Source map[string]interface{} `json:"_source"`
			} `json:"hits"`
		} `json:"hits"`
	}
	if err := json.NewDecoder(res.Body).Decode(&osResp); err != nil {
		log.Warnf("Failed to decode OpenSearch response: %v", err)
		return &DiagK8sEventsResponse{Events: []DiagK8sEvent{}}, nil
	}

	events := make([]DiagK8sEvent, 0, len(osResp.Hits.Hits))
	for _, hit := range osResp.Hits.Hits {
		ev := DiagK8sEvent{}
		if v, ok := hit.Source["type"].(string); ok {
			ev.Type = v
		}
		if v, ok := hit.Source["reason"].(string); ok {
			ev.Reason = v
		}
		if v, ok := hit.Source["message"].(string); ok {
			ev.Message = v
		}
		if v, ok := hit.Source["involvedObject"].(map[string]interface{}); ok {
			ev.InvolvedObject = v
		}
		if v, ok := hit.Source["count"].(float64); ok {
			ev.Count = int32(v)
		}
		if v, ok := hit.Source["firstTimestamp"].(string); ok {
			ev.FirstTimestamp = v
		}
		if v, ok := hit.Source["lastTimestamp"].(string); ok {
			ev.LastTimestamp = v
		}
		if v, ok := hit.Source["source"].(map[string]interface{}); ok {
			if comp, ok := v["component"].(string); ok {
				ev.Source = comp
			}
		}
		events = append(events, ev)
	}

	return &DiagK8sEventsResponse{Events: events}, nil
}

// ======================== 5. GPU Utilization ========================

type DiagGPUUtilRequest struct {
	diagBaseRequest
	Start string `json:"start" query:"start" mcp:"start,description=Time range start (RFC3339)"`
	End   string `json:"end" query:"end" mcp:"end,description=Time range end (RFC3339)"`
	Step  string `json:"step" query:"step" mcp:"step,description=Step interval in seconds (default 60)"`
}

type DiagGPUUtilResponse struct {
	Summary    *DiagGPUSummary          `json:"summary"`
	TimeSeries map[string]interface{}   `json:"time_series,omitempty"`
}

type DiagGPUSummary struct {
	Avg              float64 `json:"avg"`
	P50              float64 `json:"p50"`
	P90              float64 `json:"p90"`
	P95              float64 `json:"p95"`
	Max              float64 `json:"max"`
	Min              float64 `json:"min"`
	SampleCount      int32   `json:"sample_count"`
	AllocatedGpuCount float64 `json:"allocated_gpu_count"`
}

func handleDiagGPUUtil(ctx context.Context, req *DiagGPUUtilRequest) (*DiagGPUUtilResponse, error) {
	facade, _, err := diagResolveFacade(ctx, req.UID, req.Cluster)
	if err != nil {
		return nil, err
	}

	resp := &DiagGPUUtilResponse{}

	// Summary from DB
	stat, err := facade.GetWorkloadStatistic().GetByUID(ctx, req.UID)
	if err == nil && stat != nil {
		resp.Summary = &DiagGPUSummary{
			Avg:              stat.AvgGpuUtilization,
			P50:              stat.P50GpuUtilization,
			P90:              stat.P90GpuUtilization,
			P95:              stat.P95GpuUtilization,
			Max:              stat.MaxGpuUtilization,
			Min:              stat.MinGpuUtilization,
			SampleCount:      stat.SampleCount,
			AllocatedGpuCount: stat.AllocatedGpuCount,
		}
	}

	// Time series from Prometheus if time range specified
	if req.Start != "" || req.End != "" {
		start, end, err := parseDiagTimeRange(req.Start, req.End)
		if err != nil {
			return nil, err
		}
		step := parseDiagStep(req.Step)
		storage, _, err := diagResolveStorage(ctx, req.UID, req.Cluster)
		if err == nil && storage != nil {
			ts := make(map[string]interface{})

			utilQuery := fmt.Sprintf(`workload_gpu_utilization{workload_uid="%s"}`, req.UID)
			if series, err := prom.QueryRange(ctx, storage, utilQuery, start, end, step, nil); err == nil {
				ts["utilization"] = series
			}

			vramQuery := fmt.Sprintf(`workload_gpu_total_vram{workload_uid="%s"} - workload_gpu_free_vram{workload_uid="%s"}`, req.UID, req.UID)
			if series, err := prom.QueryRange(ctx, storage, vramQuery, start, end, step, nil); err == nil {
				ts["vram_used"] = series
			}

			powerQuery := fmt.Sprintf(`workload_gpu_socket_power_watts{workload_uid="%s"}`, req.UID)
			if series, err := prom.QueryRange(ctx, storage, powerQuery, start, end, step, nil); err == nil {
				ts["power_watts"] = series
			}

			if len(ts) > 0 {
				resp.TimeSeries = ts
			}
		}
	}

	return resp, nil
}

// ======================== 6. Compute Metrics ========================

type DiagComputeMetricsRequest struct {
	diagBaseRequest
	Start string `json:"start" query:"start" mcp:"start,description=Time range start (RFC3339),required"`
	End   string `json:"end" query:"end" mcp:"end,description=Time range end (RFC3339),required"`
	Step  string `json:"step" query:"step" mcp:"step,description=Step interval in seconds (default 60)"`
}

type DiagComputeMetricsResponse struct {
	TimeSeries map[string]interface{} `json:"time_series"`
}

func handleDiagComputeMetrics(ctx context.Context, req *DiagComputeMetricsRequest) (*DiagComputeMetricsResponse, error) {
	start, end, err := parseDiagTimeRange(req.Start, req.End)
	if err != nil {
		return nil, err
	}
	step := parseDiagStep(req.Step)

	storage, _, err := diagResolveStorage(ctx, req.UID, req.Cluster)
	if err != nil {
		return nil, err
	}

	ts := make(map[string]interface{})
	queries := map[string]string{
		"cpu_usage_cores":         fmt.Sprintf(`sum(rate(workload_container_cpu_usage_seconds_total{workload_uid="%s"}[5m])) by (pod)`, req.UID),
		"memory_usage_bytes":      fmt.Sprintf(`sum by (pod) (workload_container_memory_working_set_bytes{container!="",pod!="",workload_uid="%s"})`, req.UID),
		"ephemeral_storage_bytes": fmt.Sprintf(`sum by (pod) (workload_pod_ephemeral_storage_usage_bytes{workload_uid="%s"})`, req.UID),
		"fs_read_bytes_rate":      fmt.Sprintf(`sum by (pod) (rate(workload_container_fs_reads_bytes_total{container!="",pod!="",workload_uid="%s"}[5m]))`, req.UID),
		"fs_write_bytes_rate":     fmt.Sprintf(`sum by (pod) (rate(workload_container_fs_writes_bytes_total{container!="",pod!="",workload_uid="%s"}[5m]))`, req.UID),
	}

	for name, query := range queries {
		if series, err := prom.QueryRange(ctx, storage, query, start, end, step, nil); err == nil {
			ts[name] = series
		}
	}

	return &DiagComputeMetricsResponse{TimeSeries: ts}, nil
}

// ======================== 7. Network Metrics ========================

type DiagNetworkMetricsRequest struct {
	diagBaseRequest
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
	start, end, err := parseDiagTimeRange(req.Start, req.End)
	if err != nil {
		return nil, err
	}
	step := parseDiagStep(req.Step)

	storage, _, err := diagResolveStorage(ctx, req.UID, req.Cluster)
	if err != nil {
		return nil, err
	}

	resp := &DiagNetworkMetricsResponse{}

	rdma := make(map[string]interface{})
	rdmaQueries := map[string]string{
		"tx_bytes_rate":   fmt.Sprintf(`sum(rate(workload_rdma_stat_tx_roce_only_bytes{workload_uid="%s"}[1m])) by (primus_lens_node_name)`, req.UID),
		"rx_bytes_rate":   fmt.Sprintf(`sum(rate(workload_rdma_stat_rx_roce_only_bytes{workload_uid="%s"}[1m])) by (primus_lens_node_name)`, req.UID),
		"tx_packets_rate": fmt.Sprintf(`sum(rate(workload_rdma_stat_tx_roce_only_pkts{workload_uid="%s"}[1m])) by (primus_lens_node_name)`, req.UID),
		"rx_packets_rate": fmt.Sprintf(`sum(rate(workload_rdma_stat_rx_roce_only_pkts{workload_uid="%s"}[1m])) by (primus_lens_node_name)`, req.UID),
	}
	for name, query := range rdmaQueries {
		if series, err := prom.QueryRange(ctx, storage, query, start, end, step, nil); err == nil {
			rdma[name] = series
		}
	}
	if len(rdma) > 0 {
		resp.RDMA = rdma
	}

	rdmaErrors := make(map[string]interface{})
	errorQueries := map[string]string{
		"retransmits":        fmt.Sprintf(`sum(increase(workload_rdma_stat_to_retransmits{workload_uid="%s"}[1m])) by (primus_lens_node_name)`, req.UID),
		"max_retry_exceeded": fmt.Sprintf(`sum(increase(workload_rdma_stat_max_retry_exceeded{workload_uid="%s"}[1m])) by (primus_lens_node_name)`, req.UID),
		"seq_err_naks":       fmt.Sprintf(`sum(increase(workload_rdma_stat_seq_err_naks_rcvd{workload_uid="%s"}[1m])) by (primus_lens_node_name)`, req.UID),
		"rnr_naks":           fmt.Sprintf(`sum(increase(workload_rdma_stat_rnr_naks_rcvd{workload_uid="%s"}[1m])) by (primus_lens_node_name)`, req.UID),
		"rx_discards":        fmt.Sprintf(`sum(increase(workload_rdma_stat_rx_roce_discards{workload_uid="%s"}[1m])) by (primus_lens_node_name)`, req.UID),
		"tx_errors":          fmt.Sprintf(`sum(increase(workload_rdma_stat_tx_roce_errors{workload_uid="%s"}[1m])) by (primus_lens_node_name)`, req.UID),
		"unrecoverable_err":  fmt.Sprintf(`sum(increase(workload_rdma_stat_unrecoverable_err{workload_uid="%s"}[1m])) by (primus_lens_node_name)`, req.UID),
	}
	for name, query := range errorQueries {
		if series, err := prom.QueryRange(ctx, storage, query, start, end, step, nil); err == nil {
			rdmaErrors[name] = series
		}
	}
	if len(rdmaErrors) > 0 {
		resp.RDMAErrors = rdmaErrors
	}

	interconnect := make(map[string]interface{})
	interQueries := map[string]string{
		"xgmi_tx_rate":       fmt.Sprintf(`sum(rate(workload_gpu_xgmi_link_tx{workload_uid="%s"}[1m])) by (primus_lens_node_name)`, req.UID),
		"xgmi_rx_rate":       fmt.Sprintf(`sum(rate(gpu_xgmi_link_rx{workload_uid="%s"}[1m])) by (primus_lens_node_name)`, req.UID),
		"pcie_bandwidth_mbs": fmt.Sprintf(`sum(rate(workload_gpu_pcie_bandwidth_mbs{workload_uid="%s"}[1m])) by (primus_lens_node_name)`, req.UID),
	}
	for name, query := range interQueries {
		if series, err := prom.QueryRange(ctx, storage, query, start, end, step, nil); err == nil {
			interconnect[name] = series
		}
	}
	if len(interconnect) > 0 {
		resp.Interconnect = interconnect
	}

	return resp, nil
}

// ======================== 8. Training Progress ========================

type DiagTrainingProgressRequest struct {
	diagBaseRequest
	Start      string `json:"start" query:"start" mcp:"start,description=Time range start (RFC3339)"`
	End        string `json:"end" query:"end" mcp:"end,description=Time range end (RFC3339)"`
	DataSource string `json:"data_source" query:"data_source" mcp:"data_source,description=Filter: log or wandb or tensorflow"`
	PageNum    int    `json:"page_num" query:"page_num" mcp:"page_num,description=Page number (default 1)"`
	PageSize   int    `json:"page_size" query:"page_size" mcp:"page_size,description=Items per page (default 100)"`
}

type DiagTrainingProgressResponse struct {
	Total            int                     `json:"total"`
	DataPoints       []DiagTrainingDataPoint `json:"data_points"`
	IterationCadence *DiagIterationCadence   `json:"iteration_cadence,omitempty"`
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
	facade, _, err := diagResolveFacade(ctx, req.UID, req.Cluster)
	if err != nil {
		return nil, err
	}

	var perfs []*dbmodel.TrainingPerformance
	if req.Start != "" && req.End != "" && req.DataSource != "" {
		start, end, err := parseDiagTimeRange(req.Start, req.End)
		if err != nil {
			return nil, err
		}
		perfs, err = facade.GetTraining().ListTrainingPerformanceByWorkloadUIDDataSourceAndTimeRange(ctx, req.UID, req.DataSource, start, end)
		if err != nil {
			return nil, errors.NewError().WithCode(errors.InternalError).WithMessage("failed to query training data: " + err.Error())
		}
	} else if req.DataSource != "" {
		perfs, err = facade.GetTraining().ListTrainingPerformanceByWorkloadUIDAndDataSource(ctx, req.UID, req.DataSource)
		if err != nil {
			return nil, errors.NewError().WithCode(errors.InternalError).WithMessage("failed to query training data: " + err.Error())
		}
	} else {
		perfs, err = facade.GetTraining().ListTrainingPerformanceByWorkloadUID(ctx, req.UID)
		if err != nil {
			return nil, errors.NewError().WithCode(errors.InternalError).WithMessage("failed to query training data: " + err.Error())
		}
	}

	total := len(perfs)

	// Pagination
	pageNum := req.PageNum
	pageSize := req.PageSize
	if pageNum <= 0 {
		pageNum = 1
	}
	if pageSize <= 0 {
		pageSize = 100
	}
	if pageSize > 1000 {
		pageSize = 1000
	}
	startIdx := (pageNum - 1) * pageSize
	endIdx := startIdx + pageSize
	if startIdx > len(perfs) {
		startIdx = len(perfs)
	}
	if endIdx > len(perfs) {
		endIdx = len(perfs)
	}
	page := perfs[startIdx:endIdx]

	dataPoints := make([]DiagTrainingDataPoint, 0, len(page))
	for _, p := range page {
		dp := DiagTrainingDataPoint{
			Iteration:  p.Iteration,
			Timestamp:  p.CreatedAt.Format(time.RFC3339),
			DataSource: p.DataSource,
			Metrics:    p.Performance,
		}
		dataPoints = append(dataPoints, dp)
	}

	// Iteration cadence from all data
	var cadence *DiagIterationCadence
	if len(perfs) > 1 {
		cadence = computeIterationCadence(perfs)
	}

	return &DiagTrainingProgressResponse{
		Total:            total,
		DataPoints:       dataPoints,
		IterationCadence: cadence,
	}, nil
}

func computeIterationCadence(perfs []*dbmodel.TrainingPerformance) *DiagIterationCadence {
	if len(perfs) < 2 {
		return nil
	}

	var gaps []float64
	for i := 1; i < len(perfs); i++ {
		gap := perfs[i].CreatedAt.Sub(perfs[i-1].CreatedAt).Seconds()
		if gap > 0 {
			gaps = append(gaps, gap)
		}
	}
	if len(gaps) == 0 {
		return nil
	}

	var sum, minGap, maxGap float64
	minGap = gaps[0]
	maxGap = gaps[0]
	for _, g := range gaps {
		sum += g
		if g < minGap {
			minGap = g
		}
		if g > maxGap {
			maxGap = g
		}
	}
	avg := sum / float64(len(gaps))

	// Stall = gap > 3x average
	stallThreshold := avg * 3
	if stallThreshold < 60 {
		stallThreshold = 60
	}
	stallCount := 0
	for _, g := range gaps {
		if g > stallThreshold {
			stallCount++
		}
	}

	return &DiagIterationCadence{
		AvgSecondsPerIteration: avg,
		MinGapSeconds:          minGap,
		MaxGapSeconds:          maxGap,
		StallCount:             stallCount,
		StallThresholdSeconds:  stallThreshold,
	}
}

// ======================== 9. Code Snapshot ========================

type DiagCodeSnapshotRequest struct {
	diagBaseRequest
	IncludeContent string `json:"include_content" query:"include_content" mcp:"include_content,description=Include file content (default true)"`
}

type DiagCodeSnapshotResponse struct {
	CapturedAt     string      `json:"captured_at,omitempty"`
	Fingerprint    string      `json:"fingerprint"`
	FileCount      int         `json:"file_count"`
	TotalSize      int         `json:"total_size"`
	EntryScript    interface{} `json:"entry_script,omitempty"`
	ConfigFiles    interface{} `json:"config_files,omitempty"`
	LocalModules   interface{} `json:"local_modules,omitempty"`
	PipFreeze      string      `json:"pip_freeze,omitempty"`
	WorkingDirTree string      `json:"working_dir_tree,omitempty"`
}

func handleDiagCodeSnapshot(ctx context.Context, req *DiagCodeSnapshotRequest) (*DiagCodeSnapshotResponse, error) {
	facade, _, err := diagResolveFacade(ctx, req.UID, req.Cluster)
	if err != nil {
		return nil, err
	}

	snapshot, err := facade.GetWorkloadCodeSnapshot().GetByWorkloadUID(ctx, req.UID)
	if err != nil {
		return nil, errors.NewError().WithCode(errors.InternalError).WithMessage("failed to get code snapshot: " + err.Error())
	}
	if snapshot == nil {
		return nil, errors.NewError().WithCode(errors.RequestDataNotExisted).WithMessage("no code snapshot found for workload: " + req.UID)
	}

	resp := &DiagCodeSnapshotResponse{
		Fingerprint: snapshot.Fingerprint,
		FileCount:   int(snapshot.FileCount),
		TotalSize:   int(snapshot.TotalSize),
	}
	if snapshot.CapturedAt != nil {
		resp.CapturedAt = snapshot.CapturedAt.Format(time.RFC3339)
	}

	includeContent := req.IncludeContent != "false"
	if includeContent {
		resp.EntryScript = snapshot.EntryScript
		resp.ConfigFiles = snapshot.ConfigFiles
		resp.LocalModules = snapshot.LocalModules
		resp.PipFreeze = snapshot.PipFreeze
		resp.WorkingDirTree = snapshot.WorkingDirTree
	}

	return resp, nil
}

// ======================== 10. Image Analysis ========================

type DiagImageAnalysisRequest struct {
	diagBaseRequest
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
	facade, _, err := diagResolveFacade(ctx, req.UID, req.Cluster)
	if err != nil {
		return nil, err
	}

	// Resolve image ref from pod
	podRefs, err := facade.GetWorkload().ListWorkloadPodReferenceByWorkloadUid(ctx, req.UID)
	if err != nil || len(podRefs) == 0 {
		return nil, errors.NewError().WithCode(errors.RequestDataNotExisted).WithMessage("no pods found for workload: " + req.UID)
	}

	pods, err := facade.GetPod().ListPodsByUids(ctx, []string{podRefs[0].PodUID})
	if err != nil || len(pods) == 0 {
		return nil, errors.NewError().WithCode(errors.RequestDataNotExisted).WithMessage("pod not found")
	}

	imageRef := pods[0].ContainerImage
	if imageRef == "" {
		return nil, errors.NewError().WithCode(errors.RequestDataNotExisted).WithMessage("no container image found for pod")
	}

	cache, err := facade.GetImageRegistryCache().GetByImageRef(ctx, imageRef)
	if err != nil {
		return nil, errors.NewError().WithCode(errors.InternalError).WithMessage("failed to get image analysis: " + err.Error())
	}
	if cache == nil {
		return &DiagImageAnalysisResponse{ImageRef: imageRef}, nil
	}

	resp := &DiagImageAnalysisResponse{
		ImageRef:          cache.ImageRef,
		Digest:            cache.Digest,
		BaseImage:         cache.BaseImage,
		TotalSize:         cache.TotalSize,
		LayerHistory:      cache.LayerHistory,
		InstalledPackages: cache.InstalledPackages,
		FrameworkHints:    cache.FrameworkHints,
		ImageEnv:          cache.ImageEnv,
		ImageEntrypoint:   cache.ImageEntrypoint,
		ImageLabels:       cache.ImageLabels,
	}
	if cache.ImageCreatedAt != nil {
		resp.ImageCreatedAt = cache.ImageCreatedAt.Format(time.RFC3339)
	}

	return resp, nil
}

// ======================== 11. Detection Evidence ========================

type DiagEvidenceRequest struct {
	diagBaseRequest
	Source string `json:"source" query:"source" mcp:"source,description=Filter by source: process/env/image/log/label/active_detection"`
}

type DiagEvidenceResponse struct {
	EvidenceCount   int              `json:"evidence_count"`
	EvidenceSources []string         `json:"evidence_sources"`
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
	facade, _, err := diagResolveFacade(ctx, req.UID, req.Cluster)
	if err != nil {
		return nil, err
	}

	var records []*dbmodel.WorkloadDetectionEvidence
	if req.Source != "" {
		records, err = facade.GetWorkloadDetectionEvidence().ListEvidenceBySource(ctx, req.UID, req.Source)
	} else {
		records, err = facade.GetWorkloadDetectionEvidence().ListEvidenceByWorkload(ctx, req.UID)
	}
	if err != nil {
		return nil, errors.NewError().WithCode(errors.InternalError).WithMessage("failed to list evidence: " + err.Error())
	}

	sources, _ := facade.GetWorkloadDetectionEvidence().GetDistinctSourcesByWorkload(ctx, req.UID)

	items := make([]DiagEvidenceItem, 0, len(records))
	for _, r := range records {
		items = append(items, DiagEvidenceItem{
			Source:           r.Source,
			SourceType:       r.SourceType,
			Framework:        r.Framework,
			WorkloadType:     r.WorkloadType,
			Confidence:       r.Confidence,
			DetectedAt:       r.DetectedAt.Format(time.RFC3339),
			Evidence:         r.Evidence,
			WrapperFramework: r.WrapperFramework,
			BaseFramework:    r.BaseFramework,
		})
	}

	return &DiagEvidenceResponse{
		EvidenceCount:   len(items),
		EvidenceSources: sources,
		Evidence:        items,
	}, nil
}

// ======================== 12. Profiler Files ========================

type DiagProfilerFilesRequest struct {
	diagBaseRequest
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
	facade, _, err := diagResolveFacade(ctx, req.UID, req.Cluster)
	if err != nil {
		return nil, err
	}

	files, err := facade.GetProfilerFile().ListByWorkloadUID(ctx, req.UID)
	if err != nil {
		return nil, errors.NewError().WithCode(errors.InternalError).WithMessage("failed to list profiler files: " + err.Error())
	}

	result := make([]DiagProfilerFile, 0, len(files))
	for _, f := range files {
		result = append(result, DiagProfilerFile{
			FileName:    f.FileName,
			FilePath:    f.FilePath,
			FileType:    f.FileType,
			FileSize:    f.FileSize,
			DownloadURL: f.DownloadURL,
			PodName:     f.PodName,
			SourcePid:   f.SourcePid,
			DetectedAt:  f.DetectedAt.Format(time.RFC3339),
		})
	}

	return &DiagProfilerFilesResponse{Files: result}, nil
}

// ======================== 13. K8s Spec ========================

type DiagK8sSpecRequest struct {
	diagBaseRequest
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
	facade, _, err := diagResolveFacade(ctx, req.UID, req.Cluster)
	if err != nil {
		return nil, err
	}

	resp := &DiagK8sSpecResponse{}

	// Workload spec snapshot
	wlSnap, err := facade.GetWorkload().GetLatestGpuWorkloadSnapshotByUid(ctx, req.UID, 0)
	if err == nil && wlSnap != nil {
		resp.WorkloadSpec = &DiagWorkloadSpec{
			GroupVersion: wlSnap.GroupVersion,
			Kind:         wlSnap.Kind,
			Detail:       wlSnap.Detail,
		}
	}

	// Pod specs
	podRefs, err := facade.GetWorkload().ListWorkloadPodReferenceByWorkloadUid(ctx, req.UID)
	if err == nil && len(podRefs) > 0 {
		for _, ref := range podRefs {
			podSnap, err := facade.GetPod().GetLastPodSnapshot(ctx, ref.PodUID, 0)
			if err != nil || podSnap == nil {
				continue
			}
			resp.PodSpecs = append(resp.PodSpecs, DiagPodSpec{
				PodUID:  podSnap.PodUID,
				PodName: podSnap.PodName,
				Spec:    podSnap.Spec,
				Status:  podSnap.Status,
			})
		}
	}

	return resp, nil
}
