package tensorboard

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
)

// HyperparameterExtractor extracts hyperparameters from TensorBoard events
type HyperparameterExtractor struct {
	// Known hyperparameter categories
	knownHparams map[string]string
}

// NewHyperparameterExtractor creates a new hyperparameter extractor
func NewHyperparameterExtractor() *HyperparameterExtractor {
	return &HyperparameterExtractor{
		knownHparams: initKnownHparams(),
	}
}

// ExtractHyperparameters extracts hyperparameters from parsed events
func (h *HyperparameterExtractor) ExtractHyperparameters(events []*ParsedEvent) map[string]interface{} {
	hparams := make(map[string]interface{})

	// Look for hyperparameters in step 0 (initialization)
	for _, event := range events {
		if event.Step != 0 {
			continue
		}

		// Extract from text events (text_summary)
		for tag, text := range event.Texts {
			if strings.HasSuffix(tag, "/text_summary") {
				paramName := strings.TrimSuffix(tag, "/text_summary")
				hparams[paramName] = h.parseValue(text)
			} else if strings.Contains(tag, "hparam") || strings.Contains(tag, "config") {
				// Also check for explicit hparam tags
				hparams[tag] = h.parseValue(text)
			}
		}

		// Some hyperparameters might be logged as scalars at step 0
		for tag, value := range event.Scalars {
			if h.isHyperparameterTag(tag) {
				hparams[tag] = value
			}
		}
	}

	log.Infof("Extracted %d hyperparameters from TensorBoard events", len(hparams))
	return hparams
}

// CategorizeHyperparameters groups hyperparameters by category
func (h *HyperparameterExtractor) CategorizeHyperparameters(hparams map[string]interface{}) map[string]map[string]interface{} {
	categorized := map[string]map[string]interface{}{
		"training":   make(map[string]interface{}),
		"model":      make(map[string]interface{}),
		"parallel":   make(map[string]interface{}),
		"optimizer":  make(map[string]interface{}),
		"precision":  make(map[string]interface{}),
		"data":       make(map[string]interface{}),
		"checkpoint": make(map[string]interface{}),
		"other":      make(map[string]interface{}),
	}

	for key, value := range hparams {
		category := h.categorizeParam(key)
		categorized[category][key] = value
	}

	return categorized
}

// GetKeyHyperparameters extracts the most important hyperparameters
func (h *HyperparameterExtractor) GetKeyHyperparameters(hparams map[string]interface{}) map[string]interface{} {
	keyParams := []string{
		"lr", "learning_rate",
		"global_batch_size", "micro_batch_size",
		"num_layers", "hidden_size",
		"num_attention_heads",
		"seq_length", "max_position_embeddings",
		"optimizer",
		"weight_decay",
		"tensor_model_parallel_size",
		"pipeline_model_parallel_size",
		"data_parallel_size",
		"train_iters",
		"fp16", "bf16", "fp8",
	}

	result := make(map[string]interface{})
	for _, key := range keyParams {
		if val, ok := hparams[key]; ok {
			result[key] = val
		}
	}

	return result
}

// parseValue attempts to parse a string value into appropriate type
func (h *HyperparameterExtractor) parseValue(text string) interface{} {
	text = strings.TrimSpace(text)

	// Handle None/null
	if text == "None" || text == "null" || text == "" {
		return nil
	}

	// Try boolean
	if strings.EqualFold(text, "true") {
		return true
	}
	if strings.EqualFold(text, "false") {
		return false
	}

	// Try integer
	if val, err := strconv.ParseInt(text, 10, 64); err == nil {
		return val
	}

	// Try float
	if val, err := strconv.ParseFloat(text, 64); err == nil {
		return val
	}

	// Try JSON
	var jsonData interface{}
	if err := json.Unmarshal([]byte(text), &jsonData); err == nil {
		return jsonData
	}

	// Return as string
	return text
}

// isHyperparameterTag checks if a tag represents a hyperparameter
func (h *HyperparameterExtractor) isHyperparameterTag(tag string) bool {
	hparamKeywords := []string{
		"learning_rate", "lr",
		"batch_size",
		"hidden", "layers",
		"optimizer",
		"weight_decay",
		"parallel",
		"precision",
	}

	lowerTag := strings.ToLower(tag)
	for _, keyword := range hparamKeywords {
		if strings.Contains(lowerTag, keyword) {
			return true
		}
	}

	return false
}

// categorizeParam determines the category of a hyperparameter
func (h *HyperparameterExtractor) categorizeParam(param string) string {
	param = strings.ToLower(param)

	// Training related
	if strings.Contains(param, "lr") || strings.Contains(param, "learning_rate") ||
		strings.Contains(param, "train_iter") || strings.Contains(param, "epoch") ||
		strings.Contains(param, "warmup") || strings.Contains(param, "decay") {
		return "training"
	}

	// Model architecture
	if strings.Contains(param, "num_layers") || strings.Contains(param, "hidden_size") ||
		strings.Contains(param, "num_attention_heads") || strings.Contains(param, "seq_length") ||
		strings.Contains(param, "vocab_size") || strings.Contains(param, "ffn") ||
		strings.Contains(param, "embedding") || strings.Contains(param, "kv_channels") {
		return "model"
	}

	// Parallel configuration
	if strings.Contains(param, "parallel") || strings.Contains(param, "world_size") ||
		strings.Contains(param, "rank") || strings.Contains(param, "distributed") {
		return "parallel"
	}

	// Optimizer
	if strings.Contains(param, "optimizer") || strings.Contains(param, "adam") ||
		strings.Contains(param, "sgd") || strings.Contains(param, "momentum") ||
		strings.Contains(param, "weight_decay") || strings.Contains(param, "beta") {
		return "optimizer"
	}

	// Precision
	if strings.Contains(param, "fp16") || strings.Contains(param, "fp32") ||
		strings.Contains(param, "bf16") || strings.Contains(param, "fp8") ||
		strings.Contains(param, "precision") || strings.Contains(param, "mixed") {
		return "precision"
	}

	// Data
	if strings.Contains(param, "data") || strings.Contains(param, "batch") ||
		strings.Contains(param, "dataset") || strings.Contains(param, "tokenizer") {
		return "data"
	}

	// Checkpoint
	if strings.Contains(param, "ckpt") || strings.Contains(param, "checkpoint") ||
		strings.Contains(param, "save") || strings.Contains(param, "load") {
		return "checkpoint"
	}

	return "other"
}

// initKnownHparams initializes the map of known hyperparameters
func initKnownHparams() map[string]string {
	return map[string]string{
		"lr":                           "training",
		"learning_rate":                "training",
		"global_batch_size":            "data",
		"micro_batch_size":             "data",
		"num_layers":                   "model",
		"hidden_size":                  "model",
		"num_attention_heads":          "model",
		"tensor_model_parallel_size":   "parallel",
		"pipeline_model_parallel_size": "parallel",
		"optimizer":                    "optimizer",
		"weight_decay":                 "optimizer",
		"fp16":                         "precision",
		"bf16":                         "precision",
		"fp8":                          "precision",
	}
}
