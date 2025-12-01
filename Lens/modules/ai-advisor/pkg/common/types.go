package common

import "time"

// Detection represents framework detection result
type Detection struct {
	WorkloadUID string                 `json:"workload_uid"`
	Framework   string                 `json:"framework"`
	Type        string                 `json:"type"`
	Confidence  float64                `json:"confidence"`
	Status      string                 `json:"status"`
	Sources     []DetectionSource      `json:"sources"`
	Conflicts   []string               `json:"conflicts,omitempty"`
	ReuseInfo   *ReuseInfo             `json:"reuse_info,omitempty"`
	UpdatedAt   time.Time              `json:"updated_at"`
}

// DetectionSource represents a detection data source
type DetectionSource struct {
	Source     string                 `json:"source"`
	Framework  string                 `json:"framework"`
	Confidence float64                `json:"confidence"`
	Evidence   map[string]interface{} `json:"evidence"`
	Timestamp  time.Time              `json:"timestamp"`
}

// ReuseInfo contains reuse metadata
type ReuseInfo struct {
	ReusedFrom      string    `json:"reused_from"`
	SimilarityScore float64   `json:"similarity_score"`
	ReusedAt        time.Time `json:"reused_at"`
}

// DetectionRequest represents a detection report request
type DetectionRequest struct {
	WorkloadUID string                 `json:"workload_uid" binding:"required"`
	Source      string                 `json:"source" binding:"required"`
	Framework   string                 `json:"framework" binding:"required"`
	Type        string                 `json:"type"`
	Confidence  float64                `json:"confidence" binding:"min=0,max=1"`
	Evidence    map[string]interface{} `json:"evidence"`
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
	Imports     []string      `json:"imports,omitempty"`
	Environment []string      `json:"environment,omitempty"`
	WandB       WandBInfo     `json:"wandb"`
	PyTorch     PyTorchInfo   `json:"pytorch"`
}

// WandBInfo contains WandB run information
type WandBInfo struct {
	ID         string                 `json:"id"`
	Name       string                 `json:"name"`
	Project    string                 `json:"project"`
	Entity     string                 `json:"entity"`
	ConfigKeys []string               `json:"config_keys,omitempty"`
	Config     map[string]interface{} `json:"config,omitempty"`
}

// PyTorchInfo contains PyTorch environment information
type PyTorchInfo struct {
	Version       string   `json:"version"`
	CudaAvailable bool     `json:"cuda_available"`
	CudaVersion   string   `json:"cuda_version,omitempty"`
	ModulePaths   []string `json:"module_paths,omitempty"`
}

// WandBHints contains detection hints
type WandBHints struct {
	PossibleFrameworks []string `json:"possible_frameworks,omitempty"`
	WrapperFrameworks  []string `json:"wrapper_frameworks,omitempty"`
	BaseFrameworks     []string `json:"base_frameworks,omitempty"`
	ConfidenceAdjust   float64  `json:"confidence_adjust,omitempty"`
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

