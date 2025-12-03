package model

import "time"

type TrainingPerformance struct {
	// ========== Basic Iteration Information ==========
	CurrentIteration *int `json:"current_iteration,omitempty"`
	TargetIteration  *int `json:"target_iteration,omitempty"`
	Epoch            *int `json:"epoch,omitempty"`         // Current epoch
	TotalEpochs      *int `json:"total_epochs,omitempty"`  // Total number of epochs
	StepInEpoch      *int `json:"step_in_epoch,omitempty"` // Steps within current epoch

	// ========== Data Statistics ==========
	ConsumedSamples *int64 `json:"consumed_samples,omitempty"`
	ConsumedTokens  *int64 `json:"consumed_tokens,omitempty"`
	GlobalBatchSize *int   `json:"global_batch_size,omitempty"`
	MicroBatchSize  *int   `json:"micro_batch_size,omitempty"` // Micro batch size
	ActualSeqlen    *int   `json:"actual_seqlen,omitempty"`

	// ========== Time Performance ==========
	ElapsedTimePerIterationMS *float64 `json:"elapsed_time_per_iteration_ms,omitempty"`
	DataLoadingTimeMS         *float64 `json:"data_loading_time_ms,omitempty"`        // Data loading time
	ForwardTimeMS             *float64 `json:"forward_time_ms,omitempty"`             // Forward pass time
	BackwardTimeMS            *float64 `json:"backward_time_ms,omitempty"`            // Backward pass time
	OptimizerStepTimeMS       *float64 `json:"optimizer_step_time_ms,omitempty"`      // Optimizer update time
	TotalTrainingTimeSeconds  *float64 `json:"total_training_time_seconds,omitempty"` // Total training time (seconds)
	EstimatedTimeRemaining    *float64 `json:"estimated_time_remaining,omitempty"`    // Estimated remaining time (seconds)

	// ========== Throughput Metrics ==========
	SamplesPerSecond *float64 `json:"samples_per_second,omitempty"`
	TokensPerSecond  *float64 `json:"tokens_per_second,omitempty"` // Total tokens/second
	TokensPerGPU     *float64 `json:"tokens_per_gpu,omitempty"`
	TFLOPS           *float64 `json:"tflops,omitempty"` // throughput per GPU or TFLOPS
	Mfu              *float64 `json:"mfu,omitempty"`    // Model FLOPs Utilization

	// ========== Learning Rate and Optimizer ==========
	LearningRate *float64 `json:"learning_rate,omitempty"`
	BetaOne      *float64 `json:"beta_one,omitempty"`     // Adam beta1
	BetaTwo      *float64 `json:"beta_two,omitempty"`     // Adam beta2
	WeightDecay  *float64 `json:"weight_decay,omitempty"` // Weight decay
	Epsilon      *float64 `json:"epsilon,omitempty"`      // Adam epsilon

	// ========== Loss Function ==========
	LmLoss        *float64 `json:"lm_loss,omitempty"`        // Language model loss
	TotalLoss     *float64 `json:"total_loss,omitempty"`     // Total loss
	AuxiliaryLoss *float64 `json:"auxiliary_loss,omitempty"` // Auxiliary loss (if any)
	LossScale     *float64 `json:"loss_scale,omitempty"`

	// ========== Gradient Related ==========
	GradNorm      *float64 `json:"grad_norm,omitempty"`
	TotalGradNorm *float64 `json:"total_grad_norm,omitempty"`
	GradClipValue *float64 `json:"grad_clip_value,omitempty"` // Gradient clipping threshold
	NumZeros      *float64 `json:"num_zeros,omitempty"`
	NumNaNs       *float64 `json:"num_nans,omitempty"` // Number of NaN gradients
	NumInfs       *float64 `json:"num_infs,omitempty"` // Number of Inf gradients

	// ========== Iteration Statistics ==========
	SkippedIterationsNumber *int `json:"skipped_iterations_number,omitempty"`
	NanIterationsNumber     *int `json:"nan_iterations_number,omitempty"`
	SuccessfulIterations    *int `json:"successful_iterations,omitempty"` // Number of successful iterations

	// ========== Evaluation Metrics ==========
	Perplexity   *float64 `json:"perplexity,omitempty"`     // Perplexity
	Accuracy     *float64 `json:"accuracy,omitempty"`       // Accuracy
	TopKAccuracy *float64 `json:"top_k_accuracy,omitempty"` // Top-K accuracy
	F1Score      *float64 `json:"f1_score,omitempty"`       // F1 score
	BLEU         *float64 `json:"bleu,omitempty"`           // BLEU score (translation tasks)
	ROUGE        *float64 `json:"rouge,omitempty"`          // ROUGE score (summarization tasks)

	// ========== Memory Metrics ==========
	MemUsages        *float64 `json:"mem_usages,omitempty"`          // GPU memory usage (GB)
	MemFree          *float64 `json:"mem_free,omitempty"`            // GPU available memory (GB)
	MemTotal         *float64 `json:"mem_total,omitempty"`           // GPU total memory (GB)
	MemUsageRatio    *float64 `json:"mem_usage_ratio,omitempty"`     // GPU memory usage ratio (%)
	MemReserved      *float64 `json:"mem_reserved,omitempty"`        // Reserved memory (GB)
	MemAllocated     *float64 `json:"mem_allocated,omitempty"`       // Allocated memory (GB)
	MemCached        *float64 `json:"mem_cached,omitempty"`          // Cached memory (GB)
	CPUMemUsage      *float64 `json:"cpu_mem_usage,omitempty"`       // CPU memory usage (GB)
	CPUMemUsageRatio *float64 `json:"cpu_mem_usage_ratio,omitempty"` // CPU memory usage ratio (%)

	// ========== GPU Utilization ==========
	GPUUtilization    *float64 `json:"gpu_utilization,omitempty"`     // GPU compute utilization (%)
	GPUMemUtilization *float64 `json:"gpu_mem_utilization,omitempty"` // GPU memory utilization (%)
	GPUTemperature    *float64 `json:"gpu_temperature,omitempty"`     // GPU temperature (℃)
	GPUPowerUsage     *float64 `json:"gpu_power_usage,omitempty"`     // GPU power usage (W)
	GPUSMUtilization  *float64 `json:"gpu_sm_utilization,omitempty"`  // SM utilization (%)

	// ========== Distributed Training ==========
	WorldSize               *int     `json:"world_size,omitempty"`                // Total number of processes
	Rank                    *int     `json:"rank,omitempty"`                      // Current process rank
	LocalRank               *int     `json:"local_rank,omitempty"`                // Local rank
	DataParallelSize        *int     `json:"data_parallel_size,omitempty"`        // Data parallel size
	PipelineParallelSize    *int     `json:"pipeline_parallel_size,omitempty"`    // Pipeline parallel size
	TensorParallelSize      *int     `json:"tensor_parallel_size,omitempty"`      // Tensor parallel size
	AllReduceTimeMS         *float64 `json:"all_reduce_time_ms,omitempty"`        // AllReduce communication time
	CommunicationOverheadMS *float64 `json:"communication_overhead_ms,omitempty"` // Total communication overhead

	// ========== Checkpoint Related ==========
	CheckpointSaveTimeMS    *float64 `json:"checkpoint_save_time_ms,omitempty"`   // Checkpoint save time
	CheckpointLoadTimeMS    *float64 `json:"checkpoint_load_time_ms,omitempty"`   // Checkpoint load time
	LastCheckpointIteration *int     `json:"last_checkpoint_iteration,omitempty"` // Last checkpoint iteration

	// ========== Data Loading and Preprocessing ==========
	DataLoaderQueueSize *int     `json:"data_loader_queue_size,omitempty"` // Data loader queue size
	DataPrefetchTime    *float64 `json:"data_prefetch_time,omitempty"`     // Data prefetch time
	NumWorkers          *int     `json:"num_workers,omitempty"`            // Number of DataLoader workers

	// ========== Mixed Precision Training ==========
	UseMixedPrecision *bool    `json:"use_mixed_precision,omitempty"` // Whether to use mixed precision
	FP16Ratio         *float64 `json:"fp16_ratio,omitempty"`          // FP16 computation ratio
	BF16Ratio         *float64 `json:"bf16_ratio,omitempty"`          // BF16 computation ratio

	// ========== Other Performance Metrics ==========
	PCIeBandwidthUsage       *float64 `json:"pcie_bandwidth_usage,omitempty"`       // PCIe bandwidth usage (GB/s)
	NVLinkBandwidthUsage     *float64 `json:"nvlink_bandwidth_usage,omitempty"`     // NVLink bandwidth usage (GB/s)
	InfiniBandBandwidthUsage *float64 `json:"infiniband_bandwidth_usage,omitempty"` // InfiniBand bandwidth usage (GB/s)
	DiskIORead               *float64 `json:"disk_io_read,omitempty"`               // Disk read speed (MB/s)
	DiskIOWrite              *float64 `json:"disk_io_write,omitempty"`              // Disk write speed (MB/s)

	// ========== Model Related ==========
	NumParameters          *int64   `json:"num_parameters,omitempty"`           // Number of model parameters
	NumTrainableParameters *int64   `json:"num_trainable_parameters,omitempty"` // Number of trainable parameters
	ModelSizeGB            *float64 `json:"model_size_gb,omitempty"`            // Model size (GB)
	ActivationMemoryGB     *float64 `json:"activation_memory_gb,omitempty"`     // Activation memory (GB)
}

type Checkpoint struct {
	FastCKPT  bool      `json:"fast_ckpt"`
	Iteration int       `json:"iteration"`
	Path      string    `json:"path"`
	StartTime time.Time `json:"start_time"`
	EndTime   time.Time `json:"end_time"`
}

type TrainingLogEvent struct {
	Ip          string                 `json:"ip"`
	PodUid      string                 `json:"pod_uid"`
	PodName     string                 `json:"pod_name"`
	WorkloadUid string                 `json:"workload_uid"`
	Step        int                    `json:"step"`
	Data        map[string]interface{} `json:"data"`
}
