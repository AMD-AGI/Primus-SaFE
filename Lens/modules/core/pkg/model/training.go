package model

import "time"

type TrainingPerformance struct {
	// ========== 基础迭代信息 ==========
	CurrentIteration *int `json:"current_iteration,omitempty"`
	TargetIteration  *int `json:"target_iteration,omitempty"`
	Epoch            *int `json:"epoch,omitempty"`         // 当前 epoch
	TotalEpochs      *int `json:"total_epochs,omitempty"`  // 总 epoch 数
	StepInEpoch      *int `json:"step_in_epoch,omitempty"` // 当前 epoch 内的步数

	// ========== 数据统计 ==========
	ConsumedSamples *int64 `json:"consumed_samples,omitempty"`
	ConsumedTokens  *int64 `json:"consumed_tokens,omitempty"`
	GlobalBatchSize *int   `json:"global_batch_size,omitempty"`
	MicroBatchSize  *int   `json:"micro_batch_size,omitempty"` // 微批次大小
	ActualSeqlen    *int   `json:"actual_seqlen,omitempty"`

	// ========== 时间性能 ==========
	ElapsedTimePerIterationMS *float64 `json:"elapsed_time_per_iteration_ms,omitempty"`
	DataLoadingTimeMS         *float64 `json:"data_loading_time_ms,omitempty"`        // 数据加载时间
	ForwardTimeMS             *float64 `json:"forward_time_ms,omitempty"`             // 前向传播时间
	BackwardTimeMS            *float64 `json:"backward_time_ms,omitempty"`            // 反向传播时间
	OptimizerStepTimeMS       *float64 `json:"optimizer_step_time_ms,omitempty"`      // 优化器更新时间
	TotalTrainingTimeSeconds  *float64 `json:"total_training_time_seconds,omitempty"` // 总训练时间（秒）
	EstimatedTimeRemaining    *float64 `json:"estimated_time_remaining,omitempty"`    // 预估剩余时间（秒）

	// ========== 吞吐量指标 ==========
	SamplesPerSecond *float64 `json:"samples_per_second,omitempty"`
	TokensPerSecond  *float64 `json:"tokens_per_second,omitempty"` // 总 tokens/秒
	TokensPerGPU     *float64 `json:"tokens_per_gpu,omitempty"`
	TFLOPS           *float64 `json:"tflops,omitempty"` // throughput per GPU or TFLOPS
	Mfu              *float64 `json:"mfu,omitempty"`    // Model FLOPs Utilization

	// ========== 学习率和优化器 ==========
	LearningRate *float64 `json:"learning_rate,omitempty"`
	BetaOne      *float64 `json:"beta_one,omitempty"`     // Adam beta1
	BetaTwo      *float64 `json:"beta_two,omitempty"`     // Adam beta2
	WeightDecay  *float64 `json:"weight_decay,omitempty"` // 权重衰减
	Epsilon      *float64 `json:"epsilon,omitempty"`      // Adam epsilon

	// ========== 损失函数 ==========
	LmLoss        *float64 `json:"lm_loss,omitempty"`        // 语言模型损失
	TotalLoss     *float64 `json:"total_loss,omitempty"`     // 总损失
	AuxiliaryLoss *float64 `json:"auxiliary_loss,omitempty"` // 辅助损失（如果有）
	LossScale     *float64 `json:"loss_scale,omitempty"`

	// ========== 梯度相关 ==========
	GradNorm      *float64 `json:"grad_norm,omitempty"`
	TotalGradNorm *float64 `json:"total_grad_norm,omitempty"`
	GradClipValue *float64 `json:"grad_clip_value,omitempty"` // 梯度裁剪阈值
	NumZeros      *float64 `json:"num_zeros,omitempty"`
	NumNaNs       *float64 `json:"num_nans,omitempty"` // NaN 梯度数量
	NumInfs       *float64 `json:"num_infs,omitempty"` // Inf 梯度数量

	// ========== 迭代统计 ==========
	SkippedIterationsNumber *int `json:"skipped_iterations_number,omitempty"`
	NanIterationsNumber     *int `json:"nan_iterations_number,omitempty"`
	SuccessfulIterations    *int `json:"successful_iterations,omitempty"` // 成功的迭代数

	// ========== 评估指标 ==========
	Perplexity   *float64 `json:"perplexity,omitempty"`     // 困惑度
	Accuracy     *float64 `json:"accuracy,omitempty"`       // 准确率
	TopKAccuracy *float64 `json:"top_k_accuracy,omitempty"` // Top-K 准确率
	F1Score      *float64 `json:"f1_score,omitempty"`       // F1 分数
	BLEU         *float64 `json:"bleu,omitempty"`           // BLEU 分数（翻译任务）
	ROUGE        *float64 `json:"rouge,omitempty"`          // ROUGE 分数（摘要任务）

	// ========== 内存指标 ==========
	MemUsages        *float64 `json:"mem_usages,omitempty"`          // GPU 内存使用量（GB）
	MemFree          *float64 `json:"mem_free,omitempty"`            // GPU 可用内存（GB）
	MemTotal         *float64 `json:"mem_total,omitempty"`           // GPU 总内存（GB）
	MemUsageRatio    *float64 `json:"mem_usage_ratio,omitempty"`     // GPU 内存使用率（%）
	MemReserved      *float64 `json:"mem_reserved,omitempty"`        // 预留内存（GB）
	MemAllocated     *float64 `json:"mem_allocated,omitempty"`       // 已分配内存（GB）
	MemCached        *float64 `json:"mem_cached,omitempty"`          // 缓存内存（GB）
	CPUMemUsage      *float64 `json:"cpu_mem_usage,omitempty"`       // CPU 内存使用量（GB）
	CPUMemUsageRatio *float64 `json:"cpu_mem_usage_ratio,omitempty"` // CPU 内存使用率（%）

	// ========== GPU 利用率 ==========
	GPUUtilization    *float64 `json:"gpu_utilization,omitempty"`     // GPU 计算利用率（%）
	GPUMemUtilization *float64 `json:"gpu_mem_utilization,omitempty"` // GPU 内存利用率（%）
	GPUTemperature    *float64 `json:"gpu_temperature,omitempty"`     // GPU 温度（℃）
	GPUPowerUsage     *float64 `json:"gpu_power_usage,omitempty"`     // GPU 功耗（W）
	GPUSMUtilization  *float64 `json:"gpu_sm_utilization,omitempty"`  // SM 利用率（%）

	// ========== 分布式训练 ==========
	WorldSize               *int     `json:"world_size,omitempty"`                // 总进程数
	Rank                    *int     `json:"rank,omitempty"`                      // 当前进程 rank
	LocalRank               *int     `json:"local_rank,omitempty"`                // 本地 rank
	DataParallelSize        *int     `json:"data_parallel_size,omitempty"`        // 数据并行大小
	PipelineParallelSize    *int     `json:"pipeline_parallel_size,omitempty"`    // 流水线并行大小
	TensorParallelSize      *int     `json:"tensor_parallel_size,omitempty"`      // 张量并行大小
	AllReduceTimeMS         *float64 `json:"all_reduce_time_ms,omitempty"`        // AllReduce 通信时间
	CommunicationOverheadMS *float64 `json:"communication_overhead_ms,omitempty"` // 总通信开销

	// ========== Checkpoint 相关 ==========
	CheckpointSaveTimeMS    *float64 `json:"checkpoint_save_time_ms,omitempty"`   // checkpoint 保存时间
	CheckpointLoadTimeMS    *float64 `json:"checkpoint_load_time_ms,omitempty"`   // checkpoint 加载时间
	LastCheckpointIteration *int     `json:"last_checkpoint_iteration,omitempty"` // 最后一次保存 checkpoint 的迭代

	// ========== 数据加载和预处理 ==========
	DataLoaderQueueSize *int     `json:"data_loader_queue_size,omitempty"` // 数据加载队列大小
	DataPrefetchTime    *float64 `json:"data_prefetch_time,omitempty"`     // 数据预取时间
	NumWorkers          *int     `json:"num_workers,omitempty"`            // DataLoader workers 数量

	// ========== 混合精度训练 ==========
	UseMixedPrecision *bool    `json:"use_mixed_precision,omitempty"` // 是否使用混合精度
	FP16Ratio         *float64 `json:"fp16_ratio,omitempty"`          // FP16 计算占比
	BF16Ratio         *float64 `json:"bf16_ratio,omitempty"`          // BF16 计算占比

	// ========== 其他性能指标 ==========
	PCIeBandwidthUsage       *float64 `json:"pcie_bandwidth_usage,omitempty"`       // PCIe 带宽利用率（GB/s）
	NVLinkBandwidthUsage     *float64 `json:"nvlink_bandwidth_usage,omitempty"`     // NVLink 带宽利用率（GB/s）
	InfiniBandBandwidthUsage *float64 `json:"infiniband_bandwidth_usage,omitempty"` // InfiniBand 带宽利用率（GB/s）
	DiskIORead               *float64 `json:"disk_io_read,omitempty"`               // 磁盘读取速度（MB/s）
	DiskIOWrite              *float64 `json:"disk_io_write,omitempty"`              // 磁盘写入速度（MB/s）

	// ========== 模型相关 ==========
	NumParameters          *int64   `json:"num_parameters,omitempty"`           // 模型参数量
	NumTrainableParameters *int64   `json:"num_trainable_parameters,omitempty"` // 可训练参数量
	ModelSizeGB            *float64 `json:"model_size_gb,omitempty"`            // 模型大小（GB）
	ActivationMemoryGB     *float64 `json:"activation_memory_gb,omitempty"`     // 激活值内存（GB）
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
