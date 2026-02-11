// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package constant

// Task status constants
const (
	TaskStatusPending   = "pending"
	TaskStatusRunning   = "running"
	TaskStatusPaused    = "paused"
	TaskStatusCompleted = "completed"
	TaskStatusFailed    = "failed"
	TaskStatusCancelled = "cancelled"
)

// Task type constants
const (
	TaskTypeDetection          = "detection"
	TaskTypeActiveDetection    = "active_detection"
	TaskTypeMetadataCollection = "metadata_collection"
	TaskTypeTensorBoardStream  = "tensorboard_stream"
	TaskTypeMetricCollection   = "metric_collection"
	TaskTypeLogCollection      = "log_collection"
	TaskTypeCheckpointMonitor  = "checkpoint_monitor"
	TaskTypeProfilerCollection = "profiler_collection"

	// Detection coordinator and sub-tasks
	TaskTypeDetectionCoordinator = "detection_coordinator"
	TaskTypeProcessProbe         = "detection_process_probe"
	TaskTypeLogDetection         = "detection_log_scan"
	TaskTypeImageProbe           = "detection_image_probe"
	TaskTypeLabelProbe           = "detection_label_probe"

	// Py-spy profiling task (executed by node-exporter on target node, dispatched by jobs module)
	TaskTypePySpySample = "pyspy_sample"

	// GitHub Workflow related task types
	TaskTypeGithubWorkflowCollection = "github_workflow_collection" // Metrics collection from workflow runs
	TaskTypeGithubWorkflowAnalysis   = "github_workflow_analysis"   // Performance analysis and fluctuation detection
	TaskTypeGithubSchemaAnalyze      = "github_schema_analyze"      // AI-based schema analysis
	TaskTypeGithubCodeIndexing       = "github_code_indexing"       // Code indexing for AI-Me
	TaskTypeGithubGraphFetch         = "github_graph_fetch"         // Fetch workflow graph from GitHub API
	TaskTypeGithubRunSync            = "github_run_sync"            // Sync workflow run status from GitHub API

	// Event-driven sync task types (replacing high-frequency polling)
	TaskTypeGithubInitialSync    = "github_initial_sync"    // One-shot sync on runner creation
	TaskTypeGithubCompletionSync = "github_completion_sync" // One-shot sync on runner completion
	TaskTypeGithubPeriodicSync   = "github_periodic_sync"   // Periodic sync every 5 minutes until workflow completes
	TaskTypeGithubManualSync     = "github_manual_sync"     // Manual sync triggered by user

	// Workload Analysis Pipeline (replaces DetectionCoordinator for new workloads)
	TaskTypeAnalysisPipeline = "analysis_pipeline"
	TaskTypeLogAnalysis      = "log_analysis" // Offline log analysis (async, triggered after pipeline completes)
)

// Detection coverage source constants
const (
	DetectionSourceProcess = "process"
	DetectionSourceLog     = "log"
	DetectionSourceImage   = "image"
	DetectionSourceLabel   = "label"
	DetectionSourceWandb   = "wandb"
	DetectionSourceImport  = "import"
)

// Detection coverage status constants
const (
	DetectionStatusPending       = "pending"
	DetectionStatusCollecting    = "collecting"
	DetectionStatusCollected     = "collected"
	DetectionStatusFailed        = "failed"
	DetectionStatusNotApplicable = "not_applicable"
)

// Coordinator state constants
const (
	CoordinatorStateInit      = "init"
	CoordinatorStateWaiting   = "waiting"
	CoordinatorStateProbing   = "probing"
	CoordinatorStateAnalyzing = "analyzing"
	CoordinatorStateConfirmed = "confirmed"
	CoordinatorStateCompleted = "completed"
)

// Analysis Pipeline state constants
const (
	PipelineStateInit              = "init"              // Initialize coverage records
	PipelineStateCollecting        = "collecting"        // Collecting evidence from all sources
	PipelineStateEvaluating        = "evaluating"        // Running EvidenceEvaluator
	PipelineStateRequestingLLM     = "requesting_llm"    // Awaiting Conductor LLM analysis
	PipelineStateMergingResult     = "merging_result"     // Merging deterministic + LLM results
	PipelineStateConfirmed         = "confirmed"          // Intent confirmed, trigger side-effects
	PipelineStateMonitoring        = "monitoring"         // Continuous re-evaluation (long-running workloads)
	PipelineStateCompleted         = "completed"          // Terminal state
)

// Analysis mode constants
const (
	AnalysisModeLocal = "local" // Deterministic-only (no LLM)
	AnalysisModeFull  = "full"  // Deterministic + LLM (via Conductor)
)

// Intent state constants (for workload_detection.intent_state)
const (
	IntentStatePending     = "pending"
	IntentStateCollecting  = "collecting"
	IntentStateAnalyzing   = "analyzing"
	IntentStateConfirmed   = "confirmed"
	IntentStateFailed      = "failed"
)

// AnalysisTaskTypes defines all task types related to analysis
var AnalysisTaskTypes = []string{
	TaskTypeGithubWorkflowAnalysis,
	TaskTypeGithubSchemaAnalyze,
	TaskTypeGithubCodeIndexing,
}

// AnalysisTaskTypeDisplayNames maps task types to human-readable display names
var AnalysisTaskTypeDisplayNames = map[string]string{
	TaskTypeGithubWorkflowAnalysis: "Failure Analysis",
	TaskTypeGithubSchemaAnalyze:    "Schema Analysis",
	TaskTypeGithubCodeIndexing:     "Code Analysis",
}

// Build trigger: 2026-01-27 - workflow sync API parameter fix

