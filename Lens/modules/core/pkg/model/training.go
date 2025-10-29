package model

import "time"

type TrainingPerformance struct {
	CurrentIteration          int     `json:"current_iteration"`
	TargetIteration           int     `json:"target_iteration"`
	ConsumedSamples           int64   `json:"consumed_samples"`
	ConsumedTokens            int64   `json:"consumed_tokens"`
	ElapsedTimePerIterationMS float64 `json:"elapsed_time_per_iteration_ms"`
	LearningRate              float64 `json:"learning_rate"`
	GlobalBatchSize           int     `json:"global_batch_size"`
	LmLoss                    float64 `json:"lm_loss"`
	LossScale                 float64 `json:"loss_scale"`
	TotalGradNorm             float64 `json:"total_grad_norm"`
	NumZeros                  float64 `json:"num_zeros"`
	ActualSeqlen              int     `json:"actual_seqlen"`
	SkippedIterationsNumber   int     `json:"skipped_iterations_number"`
	NanIterationsNumber       int     `json:"nan_iterations_number"`
	SamplesPerSecond          float64 `json:"samples_per_second"`
	TFLOPS                    float64 `json:"tflops"` // throughput per gpu 或 tflops
	Mfu                       float64 `json:"mfu"`
	TokensPerGPU              float64 `json:"tokens_per_gpu"`
	GradNorm                  float64 `json:"grad_norm"`
	MemUsages                 float64 `json:"mem_usages"`
	MemFree                   float64 `json:"mem_free"`
	MemTotal                  float64 `json:"mem_total"`
	MemUsageRatio             float64 `json:"mem_usage_ratio"`
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
