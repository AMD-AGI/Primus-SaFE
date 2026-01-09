// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package common

import "time"

// Detection represents framework detection result with multi-layer support
type Detection struct {
	WorkloadUID string                 `json:"workload_uid"`
	Frameworks  []string               `json:"frameworks"` // Detected frameworks: [wrapper, orchestration, runtime] for multi-layer, [framework] for single-layer
	Type        string                 `json:"type"`
	Confidence  float64                `json:"confidence"`
	Status      string                 `json:"status"`
	Sources     []DetectionSource      `json:"sources"`
	Conflicts   []DetectionConflict    `json:"conflicts,omitempty"`
	ReuseInfo   *ReuseInfo             `json:"reuse_info,omitempty"`
	UpdatedAt   time.Time              `json:"updated_at"`

	// Multi-layer framework support (Detection V2)
	// Layer hierarchy: wrapper (L1) > orchestration (L2) > runtime (L3)
	FrameworkLayer         string `json:"framework_layer,omitempty"`          // Primary framework layer: "wrapper", "orchestration", "runtime", "inference"
	WrapperFramework       string `json:"wrapper_framework,omitempty"`        // L1: Wrapper framework (e.g., primus, lightning)
	OrchestrationFramework string `json:"orchestration_framework,omitempty"` // L2: Orchestration framework (e.g., megatron, deepspeed)
	RuntimeFramework       string `json:"runtime_framework,omitempty"`        // L3: Runtime framework (e.g., pytorch, tensorflow)
	BaseFramework          string `json:"base_framework,omitempty"`           // Deprecated: use RuntimeFramework, kept for backward compatibility
}

// DetectionSource represents a detection data source with multi-layer support
type DetectionSource struct {
	Source     string                 `json:"source"`
	Frameworks []string               `json:"frameworks"` // Detected frameworks: [wrapper, orchestration, runtime] or [framework]
	Confidence float64                `json:"confidence"`
	Evidence   map[string]interface{} `json:"evidence"`
	Timestamp  time.Time              `json:"timestamp"`

	// Multi-layer framework support (Detection V2)
	FrameworkLayer         string `json:"framework_layer,omitempty"`
	WrapperFramework       string `json:"wrapper_framework,omitempty"`
	OrchestrationFramework string `json:"orchestration_framework,omitempty"`
	RuntimeFramework       string `json:"runtime_framework,omitempty"`
	BaseFramework          string `json:"base_framework,omitempty"` // Deprecated: use RuntimeFramework
}

// DetectionConflict represents a conflict between two detection sources
type DetectionConflict struct {
	Source1    string    `json:"source1"`
	Source2    string    `json:"source2"`
	Framework1 string    `json:"framework1"`
	Framework2 string    `json:"framework2"`
	Resolution string    `json:"resolution"`
	ResolvedAt time.Time `json:"resolved_at"`
}

// ReuseInfo contains reuse metadata
type ReuseInfo struct {
	ReusedFrom      string    `json:"reused_from"`
	SimilarityScore float64   `json:"similarity_score"`
	ReusedAt        time.Time `json:"reused_at"`
}

// DetectionRequest represents a detection report request with multi-layer support
type DetectionRequest struct {
	WorkloadUID string                 `json:"workload_uid" binding:"required"`
	Source      string                 `json:"source" binding:"required"`
	Frameworks  []string               `json:"frameworks" binding:"required"` // Detected frameworks: [wrapper, orchestration, runtime] or [framework]
	Type        string                 `json:"type"`
	Confidence  float64                `json:"confidence" binding:"min=0,max=1"`
	Evidence    map[string]interface{} `json:"evidence"`

	// Multi-layer framework support (optional, for backward compatibility)
	FrameworkLayer         string `json:"framework_layer,omitempty"`          // "wrapper", "orchestration", "runtime", or "inference"
	WrapperFramework       string `json:"wrapper_framework,omitempty"`        // L1: Wrapper framework
	OrchestrationFramework string `json:"orchestration_framework,omitempty"` // L2: Orchestration framework
	RuntimeFramework       string `json:"runtime_framework,omitempty"`        // L3: Runtime framework
	BaseFramework          string `json:"base_framework,omitempty"`           // Deprecated: use RuntimeFramework
}

// PerformanceAnalysis represents performance analysis result
type PerformanceAnalysis struct {
	WorkloadUID        string                 `json:"workload_uid"`
	AnalysisTime       time.Time              `json:"analysis_time"`
	OverallScore       float64                `json:"overall_score"`
	GPUUtilization     *GPUMetrics            `json:"gpu_utilization,omitempty"`
	TrainingEfficiency *TrainingMetrics       `json:"training_efficiency,omitempty"`
	Bottlenecks        []Bottleneck           `json:"bottlenecks,omitempty"`
	Trends             map[string]interface{} `json:"trends,omitempty"`
}

// GPUMetrics represents GPU performance metrics
type GPUMetrics struct {
	AvgUtilization    float64 `json:"avg_utilization"`
	PeakUtilization   float64 `json:"peak_utilization"`
	MemoryUtilization float64 `json:"memory_utilization"`
	TFLOPS            float64 `json:"tflops,omitempty"`
}

// TrainingMetrics represents training performance metrics
type TrainingMetrics struct {
	IterationsPerSecond float64 `json:"iterations_per_second"`
	SamplesPerSecond    float64 `json:"samples_per_second"`
	LossConvergence     string  `json:"loss_convergence"` // "good", "slow", "unstable"
	EstimatedCompletion *time.Time `json:"estimated_completion,omitempty"`
}

// Bottleneck represents a performance bottleneck
type Bottleneck struct {
	Type        string  `json:"type"`        // "gpu", "cpu", "memory", "io", "network"
	Severity    string  `json:"severity"`    // "low", "medium", "high", "critical"
	Description string  `json:"description"`
	Impact      float64 `json:"impact"`      // 0-1 scale
}

// Anomaly represents a detected anomaly
type Anomaly struct {
	WorkloadUID string                 `json:"workload_uid"`
	Type        string                 `json:"type"` // "loss", "gradient", "oom", "deadlock"
	Severity    string                 `json:"severity"`
	DetectedAt  time.Time              `json:"detected_at"`
	Description string                 `json:"description"`
	Confidence  float64                `json:"confidence"`
	Evidence    map[string]interface{} `json:"evidence"`
}

// Recommendation represents an optimization recommendation
type Recommendation struct {
	WorkloadUID string                 `json:"workload_uid"`
	Category    string                 `json:"category"` // "hyperparameter", "resource", "config", "checkpoint"
	Priority    string                 `json:"priority"` // "low", "medium", "high"
	Title       string                 `json:"title"`
	Description string                 `json:"description"`
	Impact      string                 `json:"impact"` // Expected improvement
	Action      map[string]interface{} `json:"action"`
	CreatedAt   time.Time              `json:"created_at"`
}

// Diagnostic represents a diagnostic report
type Diagnostic struct {
	WorkloadUID     string                 `json:"workload_uid"`
	DiagnosticTime  time.Time              `json:"diagnostic_time"`
	Status          string                 `json:"status"` // "healthy", "warning", "critical"
	RootCauses      []RootCause            `json:"root_causes,omitempty"`
	Recommendations []Recommendation       `json:"recommendations,omitempty"`
	Summary         string                 `json:"summary"`
	Details         map[string]interface{} `json:"details,omitempty"`
}

// RootCause represents a root cause analysis result
type RootCause struct {
	Issue       string  `json:"issue"`
	Confidence  float64 `json:"confidence"`
	Description string  `json:"description"`
	Evidence    []string `json:"evidence"`
}

// ModelInsight represents model architecture insights
type ModelInsight struct {
	WorkloadUID      string                 `json:"workload_uid"`
	ModelName        string                 `json:"model_name,omitempty"`
	Architecture     string                 `json:"architecture,omitempty"`
	TotalParameters  int64                  `json:"total_parameters,omitempty"`
	TrainableParams  int64                  `json:"trainable_params,omitempty"`
	MemoryEstimate   *MemoryEstimate        `json:"memory_estimate,omitempty"`
	ComputeEstimate  *ComputeEstimate       `json:"compute_estimate,omitempty"`
	AdditionalInfo   map[string]interface{} `json:"additional_info,omitempty"`
}

// MemoryEstimate represents memory usage estimation
type MemoryEstimate struct {
	ModelSize       int64   `json:"model_size_bytes"`
	ActivationSize  int64   `json:"activation_size_bytes"`
	OptimizerSize   int64   `json:"optimizer_size_bytes"`
	TotalEstimate   int64   `json:"total_estimate_bytes"`
	RecommendedGPUs int     `json:"recommended_gpus"`
}

// ComputeEstimate represents compute requirements
type ComputeEstimate struct {
	FLOPsPerIteration int64   `json:"flops_per_iteration"`
	EstimatedTFLOPS   float64 `json:"estimated_tflops"`
	EstimatedTime     string  `json:"estimated_time"` // e.g., "2h30m"
}

// Statistics represents aggregated statistics
type Statistics struct {
	TotalWorkloads      int                    `json:"total_workloads"`
	ByFramework         map[string]int         `json:"by_framework,omitempty"`
	ByStatus            map[string]int         `json:"by_status,omitempty"`
	BySource            map[string]int         `json:"by_source,omitempty"`
	AverageConfidence   float64                `json:"average_confidence,omitempty"`
	ConflictRate        float64                `json:"conflict_rate,omitempty"`
	ReuseRate           float64                `json:"reuse_rate,omitempty"`
	AdditionalMetrics   map[string]interface{} `json:"additional_metrics,omitempty"`
}

// WandBDetectionRequest represents WandB detection data from wandb-exporter
type WandBDetectionRequest struct {
	Source      string        `json:"source"`
	Type        string        `json:"type"`
	Version     string        `json:"version"`
	WorkloadUID string        `json:"workload_uid,omitempty"`
	PodUID      string        `json:"pod_uid,omitempty"`
	PodName     string        `json:"pod_name"`
	Namespace   string        `json:"namespace"`
	Evidence    WandBEvidence `json:"evidence"`
	Hints       WandBHints    `json:"hints"`
	Timestamp   float64       `json:"timestamp"`
}

// WandBEvidence contains evidence data from WandB
type WandBEvidence struct {
	WandB             WandBInfo                         `json:"wandb"`
	Imports           []string                          `json:"imports,omitempty"`
	Environment       map[string]string                 `json:"environment,omitempty"`
	PyTorch           *PyTorchInfo                      `json:"pytorch,omitempty"`
	WrapperFrameworks map[string]map[string]interface{} `json:"wrapper_frameworks,omitempty"`
	BaseFrameworks    map[string]map[string]interface{} `json:"base_frameworks,omitempty"`
	System            map[string]interface{}            `json:"system,omitempty"`
	Hardware          *HardwareInfo                     `json:"hardware,omitempty"`
	Software          *SoftwareInfo                     `json:"software,omitempty"`
	Build             *BuildInfo                        `json:"build,omitempty"`
}

// HardwareInfo contains hardware information
type HardwareInfo struct {
	GPUArch     string  `json:"gpu_arch,omitempty"`
	GPUCount    int     `json:"gpu_count,omitempty"`
	GPUMemoryGB float64 `json:"gpu_memory_gb,omitempty"`
	GPUName     string  `json:"gpu_name,omitempty"`
	ROCmVersion string  `json:"rocm_version,omitempty"`
	CUDAVersion string  `json:"cuda_version,omitempty"`
}

// SoftwareInfo contains software package versions
type SoftwareInfo struct {
	ROCmVersion string            `json:"rocm_version,omitempty"`
	Packages    map[string]string `json:"packages,omitempty"`
}

// BuildInfo contains CI/CD build information
type BuildInfo struct {
	BuildURL      string `json:"build_url,omitempty"`
	DockerfileURL string `json:"dockerfile_url,omitempty"`
	ImageTag      string `json:"image_tag,omitempty"`
	BuildDate     string `json:"build_date,omitempty"`
	GitCommit     string `json:"git_commit,omitempty"`
	GitBranch     string `json:"git_branch,omitempty"`
	GitRepo       string `json:"git_repo,omitempty"`
	CIPipelineID  string `json:"ci_pipeline_id,omitempty"`
}

// WandBInfo contains WandB run information
type WandBInfo struct {
	ID         string                 `json:"id"`
	Name       string                 `json:"name"`
	Project    string                 `json:"project"`
	Entity     string                 `json:"entity,omitempty"`
	ConfigKeys []string               `json:"config_keys,omitempty"`
	Config     map[string]interface{} `json:"config,omitempty"`
	Tags       []string               `json:"tags,omitempty"`
}

// PyTorchInfo contains PyTorch environment information
type PyTorchInfo struct {
	Available       bool            `json:"available"`
	Version         string          `json:"version"`
	CudaAvailable   bool            `json:"cuda_available"`
	CudaVersion     string          `json:"cuda_version,omitempty"`
	ModulePaths     []string        `json:"module_paths,omitempty"`
	DetectedModules map[string]bool `json:"detected_modules,omitempty"`
}

// WandBHints contains detection hints (supports dual-layer framework detection)
type WandBHints struct {
	WrapperFrameworks  []string                          `json:"wrapper_frameworks,omitempty"`
	BaseFrameworks     []string                          `json:"base_frameworks,omitempty"`
	PossibleFrameworks []string                          `json:"possible_frameworks,omitempty"`
	Confidence         string                            `json:"confidence,omitempty"`
	ConfidenceAdjust   float64                           `json:"confidence_adjust,omitempty"`
	PrimaryIndicators  []string                          `json:"primary_indicators,omitempty"`
	FrameworkLayers    map[string]map[string]interface{} `json:"framework_layers,omitempty"`
}

// BatchDetectionRequest represents a batch detection query request
type BatchDetectionRequest struct {
	WorkloadUIDs []string `json:"workload_uids" binding:"required"`
}

// BatchDetectionResponse represents a batch detection query response
type BatchDetectionResponse struct {
	Results []BatchDetectionResult `json:"results"`
	Total   int                    `json:"total"`
}

// BatchDetectionResult represents a single result in batch query
type BatchDetectionResult struct {
	WorkloadUID string     `json:"workload_uid"`
	Detection   *Detection `json:"detection,omitempty"`
	Error       string     `json:"error,omitempty"`
}

