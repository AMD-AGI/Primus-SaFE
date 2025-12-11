package hyperparameters

import (
	"encoding/json"
	"time"
)

// HyperparametersMetadata represents complete hyperparameter metadata from multiple sources
type HyperparametersMetadata struct {
	WorkloadUID string                     `json:"workload_uid"`
	Version     int                        `json:"version"` // Version for tracking updates
	Sources     []HyperparameterSource     `json:"sources"` // All data sources
	Merged      map[string]interface{}     `json:"merged"`  // Merged hyperparameters with source priority
	Categorized CategorizedHyperparameters `json:"categorized,omitempty"`
	Summary     HyperparameterSummary      `json:"summary"`
	CreatedAt   time.Time                  `json:"created_at"`
	UpdatedAt   time.Time                  `json:"updated_at"`
}

// HyperparameterSource represents hyperparameters from a single source
type HyperparameterSource struct {
	SourceType  string                 `json:"source_type"`           // tensorboard, config_file, checkpoint, env_vars, cmdline, inference
	SourcePath  string                 `json:"source_path,omitempty"` // File path or location
	CollectedAt time.Time              `json:"collected_at"`
	Confidence  float64                `json:"confidence"`      // 0.0-1.0, confidence level of this source
	Priority    int                    `json:"priority"`        // Higher priority overwrites lower priority in merge
	Raw         map[string]interface{} `json:"raw"`             // Raw hyperparameters from this source
	Count       int                    `json:"count"`           // Number of hyperparameters
	Error       string                 `json:"error,omitempty"` // Error message if collection failed
}

// CategorizedHyperparameters groups hyperparameters by category
type CategorizedHyperparameters struct {
	Training   map[string]interface{} `json:"training,omitempty"`
	Model      map[string]interface{} `json:"model,omitempty"`
	Parallel   map[string]interface{} `json:"parallel,omitempty"`
	Optimizer  map[string]interface{} `json:"optimizer,omitempty"`
	Precision  map[string]interface{} `json:"precision,omitempty"`
	Data       map[string]interface{} `json:"data,omitempty"`
	Checkpoint map[string]interface{} `json:"checkpoint,omitempty"`
	Other      map[string]interface{} `json:"other,omitempty"`
}

// HyperparameterSummary provides quick access to key hyperparameters
type HyperparameterSummary struct {
	// Training
	LearningRate    interface{} `json:"learning_rate,omitempty"`
	GlobalBatchSize interface{} `json:"global_batch_size,omitempty"`
	MicroBatchSize  interface{} `json:"micro_batch_size,omitempty"`
	TrainIters      interface{} `json:"train_iters,omitempty"`
	Optimizer       interface{} `json:"optimizer,omitempty"`
	WeightDecay     interface{} `json:"weight_decay,omitempty"`

	// Model Architecture
	NumLayers         interface{} `json:"num_layers,omitempty"`
	HiddenSize        interface{} `json:"hidden_size,omitempty"`
	NumAttentionHeads interface{} `json:"num_attention_heads,omitempty"`
	SeqLength         interface{} `json:"seq_length,omitempty"`
	VocabSize         interface{} `json:"vocab_size,omitempty"`

	// Parallelism
	TensorParallel   interface{} `json:"tensor_parallel,omitempty"`
	PipelineParallel interface{} `json:"pipeline_parallel,omitempty"`
	DataParallel     interface{} `json:"data_parallel,omitempty"`
	WorldSize        interface{} `json:"world_size,omitempty"`

	// Precision
	FP16 interface{} `json:"fp16,omitempty"`
	BF16 interface{} `json:"bf16,omitempty"`
	FP8  interface{} `json:"fp8,omitempty"`

	// Framework
	Framework        string `json:"framework,omitempty"` // pytorch, megatron, jax, etc.
	FrameworkVersion string `json:"framework_version,omitempty"`
}

// SourceType constants
const (
	SourceTypeTensorBoard = "tensorboard"
	SourceTypeConfigFile  = "config_file"
	SourceTypeCheckpoint  = "checkpoint"
	SourceTypeEnvVars     = "env_vars"
	SourceTypeCmdLine     = "cmdline"
	SourceTypeInference   = "inference"
	SourceTypeManual      = "manual"
)

// Source priorities (higher value = higher priority in merge)
const (
	PriorityEnvVars     = 100 // Highest - runtime overrides
	PriorityCmdLine     = 90
	PriorityConfigFile  = 80
	PriorityCheckpoint  = 70
	PriorityTensorBoard = 60
	PriorityInference   = 50 // Lowest - inferred values
	PriorityManual      = 40
)

// ToExtType converts HyperparametersMetadata to database ExtType format
func (h *HyperparametersMetadata) ToExtType() map[string]interface{} {
	data, _ := json.Marshal(h)
	var result map[string]interface{}
	json.Unmarshal(data, &result)
	return result
}

// FromExtType creates HyperparametersMetadata from database ExtType
func FromExtType(ext map[string]interface{}) (*HyperparametersMetadata, error) {
	data, err := json.Marshal(ext)
	if err != nil {
		return nil, err
	}

	var result HyperparametersMetadata
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

// AddSource adds a new hyperparameter source
func (h *HyperparametersMetadata) AddSource(source HyperparameterSource) {
	// Update source collection time if not set
	if source.CollectedAt.IsZero() {
		source.CollectedAt = time.Now()
	}

	// Set count
	source.Count = len(source.Raw)

	// Append to sources
	h.Sources = append(h.Sources, source)

	// Update timestamp
	h.UpdatedAt = time.Now()
	if h.CreatedAt.IsZero() {
		h.CreatedAt = h.UpdatedAt
	}

	// Increment version
	h.Version++
}

// MergeSources merges all sources based on priority
func (h *HyperparametersMetadata) MergeSources() {
	merged := make(map[string]interface{})

	// Sort sources by priority (lower priority first, so higher priority overwrites)
	// Create a map to track which source provided each parameter
	sourceMap := make(map[string]string) // param -> source_type

	for _, source := range h.Sources {
		if source.Error != "" {
			continue // Skip sources with errors
		}

		for key, value := range source.Raw {
			// Check if we should overwrite based on priority
			if existingSource, exists := sourceMap[key]; exists {
				// Find existing source priority
				existingPriority := 0
				for _, s := range h.Sources {
					if s.SourceType == existingSource {
						existingPriority = s.Priority
						break
					}
				}

				// Only overwrite if new source has higher priority
				if source.Priority > existingPriority {
					merged[key] = value
					sourceMap[key] = source.SourceType
				}
			} else {
				// New parameter
				merged[key] = value
				sourceMap[key] = source.SourceType
			}
		}
	}

	h.Merged = merged
}

// GetSource returns a specific source by type
func (h *HyperparametersMetadata) GetSource(sourceType string) *HyperparameterSource {
	for i := range h.Sources {
		if h.Sources[i].SourceType == sourceType {
			return &h.Sources[i]
		}
	}
	return nil
}

// HasSource checks if a source type exists
func (h *HyperparametersMetadata) HasSource(sourceType string) bool {
	return h.GetSource(sourceType) != nil
}

// GetParameter gets a parameter from merged data
func (h *HyperparametersMetadata) GetParameter(key string) (interface{}, bool) {
	val, ok := h.Merged[key]
	return val, ok
}

// GetParameterString gets a parameter as string
func (h *HyperparametersMetadata) GetParameterString(key string) string {
	if val, ok := h.Merged[key]; ok {
		if str, ok := val.(string); ok {
			return str
		}
	}
	return ""
}

// GetParameterInt gets a parameter as int
func (h *HyperparametersMetadata) GetParameterInt(key string) int {
	if val, ok := h.Merged[key]; ok {
		switch v := val.(type) {
		case int:
			return v
		case int64:
			return int(v)
		case float64:
			return int(v)
		}
	}
	return 0
}

// GetParameterFloat gets a parameter as float64
func (h *HyperparametersMetadata) GetParameterFloat(key string) float64 {
	if val, ok := h.Merged[key]; ok {
		switch v := val.(type) {
		case float64:
			return v
		case int:
			return float64(v)
		case int64:
			return float64(v)
		}
	}
	return 0.0
}

// GetParameterBool gets a parameter as bool
func (h *HyperparametersMetadata) GetParameterBool(key string) bool {
	if val, ok := h.Merged[key]; ok {
		if b, ok := val.(bool); ok {
			return b
		}
	}
	return false
}

// UpdateSummary updates the summary with key hyperparameters
func (h *HyperparametersMetadata) UpdateSummary() {
	h.Summary = HyperparameterSummary{
		// Training
		LearningRate:    h.getAny("lr", "learning_rate"),
		GlobalBatchSize: h.getAny("global_batch_size", "batch_size"),
		MicroBatchSize:  h.getAny("micro_batch_size", "per_device_batch_size"),
		TrainIters:      h.getAny("train_iters", "max_steps", "num_train_epochs"),
		Optimizer:       h.getAny("optimizer", "optim"),
		WeightDecay:     h.getAny("weight_decay"),

		// Model
		NumLayers:         h.getAny("num_layers", "n_layers", "num_hidden_layers"),
		HiddenSize:        h.getAny("hidden_size", "d_model", "n_embd"),
		NumAttentionHeads: h.getAny("num_attention_heads", "n_head", "num_heads"),
		SeqLength:         h.getAny("seq_length", "max_position_embeddings", "n_positions"),
		VocabSize:         h.getAny("vocab_size", "padded_vocab_size"),

		// Parallel
		TensorParallel:   h.getAny("tensor_model_parallel_size", "tensor_parallel", "tp"),
		PipelineParallel: h.getAny("pipeline_model_parallel_size", "pipeline_parallel", "pp"),
		DataParallel:     h.getAny("data_parallel_size", "data_parallel", "dp"),
		WorldSize:        h.getAny("world_size"),

		// Precision
		FP16: h.getAny("fp16"),
		BF16: h.getAny("bf16"),
		FP8:  h.getAny("fp8"),
	}
}

// getAny tries to get a parameter from multiple possible keys
func (h *HyperparametersMetadata) getAny(keys ...string) interface{} {
	for _, key := range keys {
		if val, ok := h.Merged[key]; ok && val != nil {
			return val
		}
	}
	return nil
}

// Diff compares two hyperparameter sets and returns differences
func (h *HyperparametersMetadata) Diff(other *HyperparametersMetadata) map[string]interface{} {
	diff := make(map[string]interface{})

	// Check for changed or new parameters
	for key, value := range h.Merged {
		if otherValue, ok := other.Merged[key]; !ok {
			diff[key] = map[string]interface{}{
				"status": "added",
				"new":    value,
			}
		} else if !deepEqual(value, otherValue) {
			diff[key] = map[string]interface{}{
				"status": "changed",
				"old":    otherValue,
				"new":    value,
			}
		}
	}

	// Check for removed parameters
	for key, value := range other.Merged {
		if _, ok := h.Merged[key]; !ok {
			diff[key] = map[string]interface{}{
				"status": "removed",
				"old":    value,
			}
		}
	}

	return diff
}

// deepEqual compares two values deeply
func deepEqual(a, b interface{}) bool {
	aJSON, _ := json.Marshal(a)
	bJSON, _ := json.Marshal(b)
	return string(aJSON) == string(bJSON)
}
