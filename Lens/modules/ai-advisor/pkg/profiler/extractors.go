package profiler

import (
	"strings"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
)

// ============================================================================
// Primus Extractor
// ============================================================================

// PrimusExtractor extracts config from Primus framework
// Output path structure: {workspace}/{work_group}/{user_name}/{exp_name}/tensorboard/
// Filename: primus-megatron-exp[{exp_name}]-rank[{rank}].{timestamp}.pt.trace.json{.gz}
type PrimusExtractor struct{}

// NewPrimusExtractor creates a new Primus extractor
func NewPrimusExtractor() *PrimusExtractor {
	return &PrimusExtractor{}
}

// FrameworkType returns the framework type
func (e *PrimusExtractor) FrameworkType() string {
	return "primus"
}

// Priority returns the priority
func (e *PrimusExtractor) Priority() int {
	return 10 // High priority
}

// CanHandle checks if this extractor can handle the config
func (e *PrimusExtractor) CanHandle(rawConfig map[string]interface{}) bool {
	// Check for Primus-specific config structure
	if modules, ok := rawConfig["modules"].(map[string]interface{}); ok {
		if _, hasPreTrainer := modules["pre_trainer"]; hasPreTrainer {
			return true
		}
	}
	return false
}

// ExtractPaths extracts key paths from Primus config
func (e *PrimusExtractor) ExtractPaths(rawConfig map[string]interface{}, env map[string]string) *ExtractedPaths {
	paths := &ExtractedPaths{
		CustomPaths: make(map[string]string),
	}

	// Extract base paths
	workspace := e.getString(rawConfig, "workspace", "./output")
	workGroup := e.getString(rawConfig, "work_group", "${PRIMUS_TEAM:amd}")
	userName := e.getString(rawConfig, "user_name", "${PRIMUS_USER:root}")
	expName := e.getString(rawConfig, "exp_name", "${PRIMUS_EXP_NAME:experiment}")

	// Resolve environment variables
	workspace = ResolveEnvVar(workspace, env)
	workGroup = ResolveEnvVar(workGroup, env)
	userName = ResolveEnvVar(userName, env)
	expName = ResolveEnvVar(expName, env)

	// Build base output path: {workspace}/{work_group}/{user_name}/{exp_name}
	basePath := JoinPaths(workspace, workGroup, userName, expName)

	// Profiler output is in tensorboard subdirectory
	paths.ProfilerDir = JoinPaths(basePath, "tensorboard")
	paths.TensorBoardDir = JoinPaths(basePath, "tensorboard")
	paths.CheckpointDir = JoinPaths(basePath, "checkpoints")
	paths.LogDir = JoinPaths(basePath, "logs")
	paths.WorkspaceDir = workspace

	// Store custom paths for Primus-specific use
	paths.CustomPaths["exp_name"] = expName
	paths.CustomPaths["work_group"] = workGroup
	paths.CustomPaths["user_name"] = userName
	paths.CustomPaths["base_path"] = basePath

	// Extract profiler-specific settings
	if modules, ok := rawConfig["modules"].(map[string]interface{}); ok {
		if preTrainer, ok := modules["pre_trainer"].(map[string]interface{}); ok {
			if overrides, ok := preTrainer["overrides"].(map[string]interface{}); ok {
				// Check if profiler is enabled
				if profile, ok := overrides["profile"].(bool); ok && profile {
					paths.CustomPaths["profile_enabled"] = "true"
				}
				if usePytorchProfiler, ok := overrides["use_pytorch_profiler"].(bool); ok && usePytorchProfiler {
					paths.CustomPaths["use_pytorch_profiler"] = "true"
				}
				// Extract profile ranks
				if profileRanks, ok := overrides["profile_ranks"].([]interface{}); ok {
					ranks := make([]string, 0, len(profileRanks))
					for _, r := range profileRanks {
						if rank, ok := r.(float64); ok {
							ranks = append(ranks, strings.TrimSuffix(strings.TrimSuffix(
								strings.ReplaceAll(strings.TrimPrefix(
									strings.TrimPrefix(
										strings.TrimSpace(
											strings.ReplaceAll(
												strings.ReplaceAll(
													strings.ReplaceAll(
														string(rune(int(rank)+'0')),
														"[", ""),
													"]", ""),
												",", "")),
										""),
									""),
									"\n", ""),
								"\n"), ""))
							// Simplified: just convert float64 to string
						}
					}
					// Store as comma-separated
				}
				// Check gzip setting
				if useGzip, ok := overrides["torch_profiler_use_gzip"].(bool); ok && useGzip {
					paths.CustomPaths["use_gzip"] = "true"
				}
			}
		}
	}

	log.Debugf("Primus paths extracted: profiler_dir=%s, tensorboard_dir=%s", paths.ProfilerDir, paths.TensorBoardDir)
	return paths
}

// GetProfilerLocations returns profiler file locations for Primus
func (e *PrimusExtractor) GetProfilerLocations(config *FrameworkConfig) []ProfilerLocation {
	locations := make([]ProfilerLocation, 0)

	if config == nil || config.ExtractedPaths == nil {
		return locations
	}

	profilerDir := config.ExtractedPaths.ProfilerDir
	if profilerDir == "" {
		return locations
	}

	// Build filename patterns based on config
	expName := ""
	useGzip := false
	var profileRanks []int

	if config.ExtractedPaths.CustomPaths != nil {
		expName = config.ExtractedPaths.CustomPaths["exp_name"]
		useGzip = config.ExtractedPaths.CustomPaths["use_gzip"] == "true"
	}

	patterns := make([]string, 0)
	if expName != "" {
		patterns = BuildProfilerFilenamePattern(expName, profileRanks, useGzip)
	}

	// Add generic patterns as fallback
	if useGzip {
		patterns = append(patterns, "*.pt.trace.json.gz")
	}
	patterns = append(patterns, "*.pt.trace.json")

	locations = append(locations, ProfilerLocation{
		Directory:    profilerDir,
		Patterns:     patterns,
		Recursive:    false, // Primus outputs directly to tensorboard dir
		MaxDepth:     1,
		Priority:     1,
		Source:       "primus_config",
		ProfileRanks: profileRanks,
	})

	return locations
}

// GetTensorBoardDir returns TensorBoard directory
func (e *PrimusExtractor) GetTensorBoardDir(config *FrameworkConfig) string {
	if config != nil && config.ExtractedPaths != nil {
		return config.ExtractedPaths.TensorBoardDir
	}
	return ""
}

// GetCheckpointDir returns checkpoint directory
func (e *PrimusExtractor) GetCheckpointDir(config *FrameworkConfig) string {
	if config != nil && config.ExtractedPaths != nil {
		return config.ExtractedPaths.CheckpointDir
	}
	return ""
}

// getString safely gets a string value from map
func (e *PrimusExtractor) getString(m map[string]interface{}, key, defaultVal string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return defaultVal
}

// ============================================================================
// Megatron Extractor
// ============================================================================

// MegatronExtractor extracts config from Megatron-LM framework
type MegatronExtractor struct{}

// NewMegatronExtractor creates a new Megatron extractor
func NewMegatronExtractor() *MegatronExtractor {
	return &MegatronExtractor{}
}

// FrameworkType returns the framework type
func (e *MegatronExtractor) FrameworkType() string {
	return "megatron"
}

// Priority returns the priority
func (e *MegatronExtractor) Priority() int {
	return 20
}

// CanHandle checks if this extractor can handle the config
func (e *MegatronExtractor) CanHandle(rawConfig map[string]interface{}) bool {
	// Check for Megatron-specific config keys
	if _, ok := rawConfig["tensorboard-dir"]; ok {
		return true
	}
	if _, ok := rawConfig["save"]; ok {
		if _, hasLoad := rawConfig["load"]; hasLoad {
			return true
		}
	}
	return false
}

// ExtractPaths extracts key paths from Megatron config
func (e *MegatronExtractor) ExtractPaths(rawConfig map[string]interface{}, env map[string]string) *ExtractedPaths {
	paths := &ExtractedPaths{
		CustomPaths: make(map[string]string),
	}

	// Extract tensorboard directory
	if tbDir, ok := rawConfig["tensorboard-dir"].(string); ok {
		paths.TensorBoardDir = ResolveEnvVar(tbDir, env)
		paths.ProfilerDir = paths.TensorBoardDir // Profiler usually outputs to same dir
	}

	// Extract save directory (checkpoints)
	if saveDir, ok := rawConfig["save"].(string); ok {
		paths.CheckpointDir = ResolveEnvVar(saveDir, env)
	}

	// Extract profile directory if specified
	if profileDir, ok := rawConfig["profile-dir"].(string); ok {
		paths.ProfilerDir = ResolveEnvVar(profileDir, env)
	}

	log.Debugf("Megatron paths extracted: profiler_dir=%s, tensorboard_dir=%s", paths.ProfilerDir, paths.TensorBoardDir)
	return paths
}

// GetProfilerLocations returns profiler file locations for Megatron
func (e *MegatronExtractor) GetProfilerLocations(config *FrameworkConfig) []ProfilerLocation {
	locations := make([]ProfilerLocation, 0)

	if config == nil || config.ExtractedPaths == nil {
		return locations
	}

	profilerDir := config.ExtractedPaths.ProfilerDir
	if profilerDir == "" {
		return locations
	}

	locations = append(locations, ProfilerLocation{
		Directory: profilerDir,
		Patterns: []string{
			"*.pt.trace.json",
			"*.pt.trace.json.gz",
			"kineto*.json",
		},
		Recursive: true,
		MaxDepth:  2,
		Priority:  1,
		Source:    "megatron_config",
	})

	return locations
}

// GetTensorBoardDir returns TensorBoard directory
func (e *MegatronExtractor) GetTensorBoardDir(config *FrameworkConfig) string {
	if config != nil && config.ExtractedPaths != nil {
		return config.ExtractedPaths.TensorBoardDir
	}
	return ""
}

// GetCheckpointDir returns checkpoint directory
func (e *MegatronExtractor) GetCheckpointDir(config *FrameworkConfig) string {
	if config != nil && config.ExtractedPaths != nil {
		return config.ExtractedPaths.CheckpointDir
	}
	return ""
}

// ============================================================================
// DeepSpeed Extractor
// ============================================================================

// DeepSpeedExtractor extracts config from DeepSpeed framework
type DeepSpeedExtractor struct{}

// NewDeepSpeedExtractor creates a new DeepSpeed extractor
func NewDeepSpeedExtractor() *DeepSpeedExtractor {
	return &DeepSpeedExtractor{}
}

// FrameworkType returns the framework type
func (e *DeepSpeedExtractor) FrameworkType() string {
	return "deepspeed"
}

// Priority returns the priority
func (e *DeepSpeedExtractor) Priority() int {
	return 30
}

// CanHandle checks if this extractor can handle the config
func (e *DeepSpeedExtractor) CanHandle(rawConfig map[string]interface{}) bool {
	// Check for DeepSpeed-specific config keys
	if _, ok := rawConfig["zero_optimization"]; ok {
		return true
	}
	if _, ok := rawConfig["fp16"]; ok {
		if _, hasBf16 := rawConfig["bf16"]; hasBf16 || true {
			// DeepSpeed typically has fp16/bf16 at top level
			return true
		}
	}
	if _, ok := rawConfig["flops_profiler"]; ok {
		return true
	}
	return false
}

// ExtractPaths extracts key paths from DeepSpeed config
func (e *DeepSpeedExtractor) ExtractPaths(rawConfig map[string]interface{}, env map[string]string) *ExtractedPaths {
	paths := &ExtractedPaths{
		CustomPaths: make(map[string]string),
	}

	// Extract flops profiler output path
	if profiler, ok := rawConfig["flops_profiler"].(map[string]interface{}); ok {
		if outputPath, ok := profiler["output_file"].(string); ok {
			paths.ProfilerDir = ResolveEnvVar(outputPath, env)
		}
	}

	// Extract tensorboard writer path
	if tbConfig, ok := rawConfig["tensorboard"].(map[string]interface{}); ok {
		if outputPath, ok := tbConfig["output_path"].(string); ok {
			paths.TensorBoardDir = ResolveEnvVar(outputPath, env)
		}
	}

	// Extract checkpoint path
	if ckptConfig, ok := rawConfig["checkpoint"].(map[string]interface{}); ok {
		if savePath, ok := ckptConfig["save_path"].(string); ok {
			paths.CheckpointDir = ResolveEnvVar(savePath, env)
		}
	}

	log.Debugf("DeepSpeed paths extracted: profiler_dir=%s, tensorboard_dir=%s", paths.ProfilerDir, paths.TensorBoardDir)
	return paths
}

// GetProfilerLocations returns profiler file locations for DeepSpeed
func (e *DeepSpeedExtractor) GetProfilerLocations(config *FrameworkConfig) []ProfilerLocation {
	locations := make([]ProfilerLocation, 0)

	if config == nil || config.ExtractedPaths == nil {
		return locations
	}

	profilerDir := config.ExtractedPaths.ProfilerDir
	if profilerDir == "" {
		// DeepSpeed default location
		profilerDir = "/tmp/deepspeed_profiler"
	}

	locations = append(locations, ProfilerLocation{
		Directory: profilerDir,
		Patterns: []string{
			"*.pt.trace.json",
			"*.pt.trace.json.gz",
			"flops_profiler*.txt",
		},
		Recursive: true,
		MaxDepth:  2,
		Priority:  1,
		Source:    "deepspeed_config",
	})

	return locations
}

// GetTensorBoardDir returns TensorBoard directory
func (e *DeepSpeedExtractor) GetTensorBoardDir(config *FrameworkConfig) string {
	if config != nil && config.ExtractedPaths != nil {
		return config.ExtractedPaths.TensorBoardDir
	}
	return ""
}

// GetCheckpointDir returns checkpoint directory
func (e *DeepSpeedExtractor) GetCheckpointDir(config *FrameworkConfig) string {
	if config != nil && config.ExtractedPaths != nil {
		return config.ExtractedPaths.CheckpointDir
	}
	return ""
}

// ============================================================================
// Generic PyTorch Extractor (Fallback)
// ============================================================================

// GenericPyTorchExtractor is a fallback extractor for generic PyTorch workloads
type GenericPyTorchExtractor struct{}

// NewGenericPyTorchExtractor creates a new generic PyTorch extractor
func NewGenericPyTorchExtractor() *GenericPyTorchExtractor {
	return &GenericPyTorchExtractor{}
}

// FrameworkType returns the framework type
func (e *GenericPyTorchExtractor) FrameworkType() string {
	return "pytorch"
}

// Priority returns the priority (lowest, fallback)
func (e *GenericPyTorchExtractor) Priority() int {
	return 100
}

// CanHandle always returns true as a fallback
func (e *GenericPyTorchExtractor) CanHandle(rawConfig map[string]interface{}) bool {
	return true // Always handle as fallback
}

// ExtractPaths extracts key paths from generic PyTorch config
func (e *GenericPyTorchExtractor) ExtractPaths(rawConfig map[string]interface{}, env map[string]string) *ExtractedPaths {
	paths := &ExtractedPaths{
		CustomPaths: make(map[string]string),
	}

	// Try common config keys
	if outputDir, ok := rawConfig["output_dir"].(string); ok {
		paths.WorkspaceDir = ResolveEnvVar(outputDir, env)
		paths.TensorBoardDir = JoinPaths(paths.WorkspaceDir, "tensorboard")
		paths.ProfilerDir = JoinPaths(paths.WorkspaceDir, "profiler")
		paths.CheckpointDir = JoinPaths(paths.WorkspaceDir, "checkpoints")
	}

	if logDir, ok := rawConfig["log_dir"].(string); ok {
		paths.LogDir = ResolveEnvVar(logDir, env)
		if paths.TensorBoardDir == "" {
			paths.TensorBoardDir = paths.LogDir
		}
	}

	if profilerDir, ok := rawConfig["profiler_dir"].(string); ok {
		paths.ProfilerDir = ResolveEnvVar(profilerDir, env)
	}

	log.Debugf("Generic PyTorch paths extracted: profiler_dir=%s, tensorboard_dir=%s", paths.ProfilerDir, paths.TensorBoardDir)
	return paths
}

// GetProfilerLocations returns default profiler file locations
func (e *GenericPyTorchExtractor) GetProfilerLocations(config *FrameworkConfig) []ProfilerLocation {
	locations := make([]ProfilerLocation, 0)

	// Try extracted paths first
	if config != nil && config.ExtractedPaths != nil && config.ExtractedPaths.ProfilerDir != "" {
		locations = append(locations, ProfilerLocation{
			Directory: config.ExtractedPaths.ProfilerDir,
			Patterns: []string{
				"*.pt.trace.json",
				"*.pt.trace.json.gz",
				"kineto*.json",
			},
			Recursive: true,
			MaxDepth:  3,
			Priority:  1,
			Source:    "config",
		})
	}

	// Add common default locations
	defaultDirs := []string{
		"/tmp/profiler",
		"/tmp/torch_profiler",
		"/workspace/profiler",
		"/output/profiler",
	}

	for i, dir := range defaultDirs {
		locations = append(locations, ProfilerLocation{
			Directory: dir,
			Patterns: []string{
				"*.pt.trace.json",
				"*.pt.trace.json.gz",
			},
			Recursive: false,
			MaxDepth:  1,
			Priority:  50 + i,
			Source:    "default",
		})
	}

	return locations
}

// GetTensorBoardDir returns TensorBoard directory
func (e *GenericPyTorchExtractor) GetTensorBoardDir(config *FrameworkConfig) string {
	if config != nil && config.ExtractedPaths != nil {
		return config.ExtractedPaths.TensorBoardDir
	}
	return ""
}

// GetCheckpointDir returns checkpoint directory
func (e *GenericPyTorchExtractor) GetCheckpointDir(config *FrameworkConfig) string {
	if config != nil && config.ExtractedPaths != nil {
		return config.ExtractedPaths.CheckpointDir
	}
	return ""
}
