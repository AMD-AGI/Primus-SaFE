package metadata

import "time"

// WorkloadMetadata represents complete metadata for a training workload
type WorkloadMetadata struct {
	WorkloadUID  string `json:"workload_uid"`
	PodName      string `json:"pod_name"`
	PodNamespace string `json:"pod_namespace"`
	NodeName     string `json:"node_name"`

	// Framework information
	Frameworks       []string `json:"frameworks"`        // All detected frameworks
	BaseFramework    string   `json:"base_framework"`    // PyTorch, TensorFlow, JAX
	WrapperFramework string   `json:"wrapper_framework"` // Megatron, Primus, DeepSpeed

	// Training information
	PyTorchInfo     *PyTorchMetadata     `json:"pytorch_info,omitempty"`
	MegatronInfo    *MegatronMetadata    `json:"megatron_info,omitempty"`
	PrimusInfo      *PrimusMetadata      `json:"primus_info,omitempty"`
	JAXInfo         *JAXMetadata         `json:"jax_info,omitempty"`
	TensorBoardInfo *TensorBoardMetadata `json:"tensorboard_info,omitempty"`

	// Collection metadata
	CollectedAt      time.Time `json:"collected_at"`
	CollectionSource string    `json:"collection_source"` // "node-exporter"
	Confidence       float64   `json:"confidence"`
}

// PyTorchMetadata represents PyTorch-specific metadata
type PyTorchMetadata struct {
	Version         string      `json:"version"`
	CudaAvailable   bool        `json:"cuda_available"`
	CudaVersion     string      `json:"cuda_version,omitempty"`
	Models          []ModelInfo `json:"models,omitempty"`
	TotalParams     int64       `json:"total_params,omitempty"`
	TrainableParams int64       `json:"trainable_params,omitempty"`
	Device          string      `json:"device,omitempty"`
	DistributedMode string      `json:"distributed_mode,omitempty"` // DDP, FSDP, etc.
	MixedPrecision  bool        `json:"mixed_precision,omitempty"`
}

// ModelInfo represents model information
type ModelInfo struct {
	Name            string `json:"name"`
	Type            string `json:"type"`
	Parameters      int64  `json:"parameters"`
	TrainableParams int64  `json:"trainable_params"`
	Device          string `json:"device"`
}

// MegatronMetadata represents Megatron-LM specific metadata
type MegatronMetadata struct {
	Version           string  `json:"version,omitempty"`
	TensorParallel    int     `json:"tensor_parallel,omitempty"`
	PipelineParallel  int     `json:"pipeline_parallel,omitempty"`
	DataParallel      int     `json:"data_parallel,omitempty"`
	SequenceParallel  bool    `json:"sequence_parallel,omitempty"`
	MicroBatchSize    int     `json:"micro_batch_size,omitempty"`
	GlobalBatchSize   int     `json:"global_batch_size,omitempty"`
	SequenceLength    int     `json:"sequence_length,omitempty"`
	HiddenSize        int     `json:"hidden_size,omitempty"`
	NumLayers         int     `json:"num_layers,omitempty"`
	NumAttentionHeads int     `json:"num_attention_heads,omitempty"`
	VocabSize         int     `json:"vocab_size,omitempty"`
	LearningRate      float64 `json:"learning_rate,omitempty"`
	Optimizer         string  `json:"optimizer,omitempty"`
}

// PrimusMetadata represents Primus wrapper framework metadata
type PrimusMetadata struct {
	Version          string                 `json:"version,omitempty"`
	Mode             string                 `json:"mode,omitempty"` // training, inference
	BackendFramework string                 `json:"backend_framework,omitempty"`
	Configuration    map[string]interface{} `json:"configuration,omitempty"`
	Features         []string               `json:"features,omitempty"`
}

// JAXMetadata represents JAX-specific metadata
type JAXMetadata struct {
	Version      string `json:"version,omitempty"`
	Backend      string `json:"backend,omitempty"` // GPU, TPU, CPU
	NumDevices   int    `json:"num_devices,omitempty"`
	ParallelMode string `json:"parallel_mode,omitempty"`
	JIT          bool   `json:"jit,omitempty"`
}

// TensorBoardMetadata represents TensorBoard configuration
type TensorBoardMetadata struct {
	Enabled    bool     `json:"enabled"`
	LogDir     string   `json:"log_dir,omitempty"`
	Port       int      `json:"port,omitempty"`
	Writers    []string `json:"writers,omitempty"` // List of SummaryWriter instances
	UpdateFreq string   `json:"update_freq,omitempty"`
}

// CollectionRequest represents a metadata collection request
type CollectionRequest struct {
	WorkloadUID  string `json:"workload_uid" binding:"required"`
	PodName      string `json:"pod_name" binding:"required"`
	PodNamespace string `json:"pod_namespace" binding:"required"`
	PodUID       string `json:"pod_uid" binding:"required"`
	NodeName     string `json:"node_name" binding:"required"`

	// Optional parameters
	Force   bool     `json:"force,omitempty"`   // Force re-collection even if cached
	Scripts []string `json:"scripts,omitempty"` // Specific scripts to run
	Timeout int      `json:"timeout,omitempty"` // Timeout in seconds
}

// CollectionResult represents the result of metadata collection
type CollectionResult struct {
	Success        bool              `json:"success"`
	Metadata       *WorkloadMetadata `json:"metadata,omitempty"`
	Error          string            `json:"error,omitempty"`
	Duration       float64           `json:"duration"` // seconds
	ProcessCount   int               `json:"process_count"`
	PythonCount    int               `json:"python_count"`
	InspectedCount int               `json:"inspected_count"`
}

// MetadataQuery represents a query for workload metadata
type MetadataQuery struct {
	WorkloadUID string     `json:"workload_uid,omitempty"`
	Framework   string     `json:"framework,omitempty"`
	Type        string     `json:"type,omitempty"`
	StartTime   *time.Time `json:"start_time,omitempty"`
	EndTime     *time.Time `json:"end_time,omitempty"`
	Limit       int        `json:"limit,omitempty"`
}
