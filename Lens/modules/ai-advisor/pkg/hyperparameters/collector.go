package hyperparameters

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/ai-advisor/pkg/tensorboard"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
)

// Collector collects hyperparameters from multiple sources
type Collector struct {
	tbExtractor *tensorboard.HyperparameterExtractor
}

// NewCollector creates a new hyperparameter collector
func NewCollector() *Collector {
	return &Collector{
		tbExtractor: tensorboard.NewHyperparameterExtractor(),
	}
}

// CollectFromTensorBoard collects hyperparameters from TensorBoard events
func (c *Collector) CollectFromTensorBoard(
	ctx context.Context,
	events []*tensorboard.ParsedEvent,
	logDir string,
) HyperparameterSource {
	source := HyperparameterSource{
		SourceType:  SourceTypeTensorBoard,
		SourcePath:  logDir,
		CollectedAt: time.Now(),
		Priority:    PriorityTensorBoard,
		Confidence:  0.9, // High confidence for direct TensorBoard data
	}

	// Extract hyperparameters
	hparams := c.tbExtractor.ExtractHyperparameters(events)
	if len(hparams) == 0 {
		source.Error = "no hyperparameters found in TensorBoard events"
		source.Confidence = 0.0
	} else {
		source.Raw = hparams
		log.Infof("Collected %d hyperparameters from TensorBoard", len(hparams))
	}

	return source
}

// CollectFromConfigFile collects hyperparameters from a configuration file
func (c *Collector) CollectFromConfigFile(
	ctx context.Context,
	filePath string,
) HyperparameterSource {
	source := HyperparameterSource{
		SourceType:  SourceTypeConfigFile,
		SourcePath:  filePath,
		CollectedAt: time.Now(),
		Priority:    PriorityConfigFile,
		Confidence:  0.95, // Very high confidence for config files
	}

	// Read file
	data, err := os.ReadFile(filePath)
	if err != nil {
		source.Error = fmt.Sprintf("failed to read config file: %v", err)
		source.Confidence = 0.0
		return source
	}

	// Try to parse as JSON
	var config map[string]interface{}
	if err := json.Unmarshal(data, &config); err != nil {
		source.Error = fmt.Sprintf("failed to parse config file: %v", err)
		source.Confidence = 0.0
		return source
	}

	source.Raw = config
	log.Infof("Collected %d hyperparameters from config file: %s", len(config), filePath)

	return source
}

// CollectFromEnvVars collects hyperparameters from environment variables
func (c *Collector) CollectFromEnvVars(
	ctx context.Context,
	envVars map[string]string,
) HyperparameterSource {
	source := HyperparameterSource{
		SourceType:  SourceTypeEnvVars,
		CollectedAt: time.Now(),
		Priority:    PriorityEnvVars,
		Confidence:  1.0, // Highest confidence - explicit runtime overrides
		Raw:         make(map[string]interface{}),
	}

	// Convert known environment variables to hyperparameters
	knownEnvVars := []string{
		"LEARNING_RATE", "LR",
		"BATCH_SIZE", "GLOBAL_BATCH_SIZE", "MICRO_BATCH_SIZE",
		"NUM_LAYERS", "HIDDEN_SIZE",
		"WORLD_SIZE", "RANK", "LOCAL_RANK",
		"TENSOR_PARALLEL", "PIPELINE_PARALLEL",
		"MASTER_ADDR", "MASTER_PORT",
	}

	for _, key := range knownEnvVars {
		if value, ok := envVars[key]; ok {
			source.Raw[key] = value
		}
	}

	// Also check for custom prefixed env vars
	for key, value := range envVars {
		if len(key) > 7 && key[:7] == "HPARAM_" {
			paramKey := key[7:] // Remove HPARAM_ prefix
			source.Raw[paramKey] = value
		}
	}

	if len(source.Raw) == 0 {
		source.Confidence = 0.0
		log.Debug("No hyperparameters found in environment variables")
	} else {
		log.Infof("Collected %d hyperparameters from environment variables", len(source.Raw))
	}

	return source
}

// CollectFromCmdLine collects hyperparameters from command line arguments
func (c *Collector) CollectFromCmdLine(
	ctx context.Context,
	cmdline string,
) HyperparameterSource {
	source := HyperparameterSource{
		SourceType:  SourceTypeCmdLine,
		SourcePath:  cmdline,
		CollectedAt: time.Now(),
		Priority:    PriorityCmdLine,
		Confidence:  0.85,
		Raw:         make(map[string]interface{}),
	}

	// Parse command line arguments
	// This is a simplified parser - you may need a more robust one
	params := parseCommandLine(cmdline)
	if len(params) == 0 {
		source.Confidence = 0.5
	} else {
		source.Raw = params
		log.Infof("Collected %d hyperparameters from command line", len(params))
	}

	return source
}

// CollectFromCheckpoint collects hyperparameters from a checkpoint file
func (c *Collector) CollectFromCheckpoint(
	ctx context.Context,
	checkpointPath string,
	checkpointData map[string]interface{},
) HyperparameterSource {
	source := HyperparameterSource{
		SourceType:  SourceTypeCheckpoint,
		SourcePath:  checkpointPath,
		CollectedAt: time.Now(),
		Priority:    PriorityCheckpoint,
		Confidence:  0.8,
		Raw:         make(map[string]interface{}),
	}

	// Look for common checkpoint keys that contain hyperparameters
	hparamKeys := []string{"args", "config", "hparams", "hyperparameters", "model_config"}

	for _, key := range hparamKeys {
		if data, ok := checkpointData[key]; ok {
			if hparams, ok := data.(map[string]interface{}); ok {
				source.Raw = hparams
				log.Infof("Collected %d hyperparameters from checkpoint key '%s'", len(hparams), key)
				return source
			}
		}
	}

	source.Error = "no hyperparameters found in checkpoint"
	source.Confidence = 0.0

	return source
}

// CollectAll collects hyperparameters from all available sources
func (c *Collector) CollectAll(
	ctx context.Context,
	workloadUID string,
	opts CollectionOptions,
) (*HyperparametersMetadata, error) {
	metadata := &HyperparametersMetadata{
		WorkloadUID: workloadUID,
		Version:     1,
		Sources:     make([]HyperparameterSource, 0),
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	// Collect from TensorBoard if available
	if opts.TensorBoardEvents != nil && len(opts.TensorBoardEvents) > 0 {
		source := c.CollectFromTensorBoard(ctx, opts.TensorBoardEvents, opts.TensorBoardLogDir)
		metadata.AddSource(source)
	}

	// Collect from config file if specified
	if opts.ConfigFilePath != "" {
		source := c.CollectFromConfigFile(ctx, opts.ConfigFilePath)
		metadata.AddSource(source)
	}

	// Collect from environment variables if provided
	if opts.EnvVars != nil && len(opts.EnvVars) > 0 {
		source := c.CollectFromEnvVars(ctx, opts.EnvVars)
		metadata.AddSource(source)
	}

	// Collect from command line if provided
	if opts.CmdLine != "" {
		source := c.CollectFromCmdLine(ctx, opts.CmdLine)
		metadata.AddSource(source)
	}

	// Collect from checkpoint if provided
	if opts.CheckpointPath != "" && opts.CheckpointData != nil {
		source := c.CollectFromCheckpoint(ctx, opts.CheckpointPath, opts.CheckpointData)
		metadata.AddSource(source)
	}

	// Merge all sources
	metadata.MergeSources()

	// Categorize hyperparameters
	categorized := c.tbExtractor.CategorizeHyperparameters(metadata.Merged)
	metadata.Categorized = CategorizedHyperparameters{
		Training:   categorized["training"],
		Model:      categorized["model"],
		Parallel:   categorized["parallel"],
		Optimizer:  categorized["optimizer"],
		Precision:  categorized["precision"],
		Data:       categorized["data"],
		Checkpoint: categorized["checkpoint"],
		Other:      categorized["other"],
	}

	// Update summary
	metadata.UpdateSummary()

	log.Infof("Collected hyperparameters from %d sources, merged %d parameters",
		len(metadata.Sources), len(metadata.Merged))

	return metadata, nil
}

// CollectionOptions specifies options for collecting hyperparameters
type CollectionOptions struct {
	// TensorBoard
	TensorBoardEvents []*tensorboard.ParsedEvent
	TensorBoardLogDir string

	// Config file
	ConfigFilePath string

	// Environment variables
	EnvVars map[string]string

	// Command line
	CmdLine string

	// Checkpoint
	CheckpointPath string
	CheckpointData map[string]interface{}
}

// parseCommandLine is a simple command line parser
// For more robust parsing, consider using a proper argument parser
func parseCommandLine(cmdline string) map[string]interface{} {
	params := make(map[string]interface{})

	// Simple parsing: look for --key value or --key=value patterns
	// This is a basic implementation - enhance as needed

	// TODO: Implement proper command line parsing
	// For now, just return empty map

	return params
}
