package detectors

import (
	"strings"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model"
)

// RuntimeDetector detects runtime/capability layer (pytorch, tensorflow, jax, etc.)
type RuntimeDetector struct {
	knownRuntimes map[string]bool
}

// NewRuntimeDetector creates a new runtime detector
func NewRuntimeDetector() *RuntimeDetector {
	return &RuntimeDetector{
		knownRuntimes: map[string]bool{
			"pytorch":    true,
			"tensorflow": true,
			"jax":        true,
			"mxnet":      true,
			"paddle":     true,
		},
	}
}

// GetDimension returns the dimension this detector handles
func (d *RuntimeDetector) GetDimension() model.DetectionDimension {
	return model.DimensionRuntime
}

// Detect performs detection for runtime dimension
func (d *RuntimeDetector) Detect(sources []model.DetectionSource) ([]model.DimensionValue, error) {
	var values []model.DimensionValue

	for _, source := range sources {
		// Check evidence for runtime indicators
		if source.Evidence == nil {
			continue
		}

		// Method 1: Check pytorch field directly
		if pytorchInfo, ok := source.Evidence["pytorch"]; ok {
			if pytorchMap, ok := pytorchInfo.(map[string]interface{}); ok {
				if available, ok := pytorchMap["available"].(bool); ok && available {
					value := model.DimensionValue{
						Value:      "pytorch",
						Confidence: 0.90, // High confidence from direct detection
						Source:     source.Source,
						DetectedAt: source.DetectedAt,
						Evidence: map[string]interface{}{
							"method":  "pytorch_info_available",
							"version": pytorchMap["version"],
						},
					}
					values = append(values, value)
					log.Debugf("Detected PyTorch runtime from source %s (available=true)", source.Source)
					continue
				}
			}
		}

		// Method 2: Check base_framework field
		if baseFramework, ok := source.Evidence["base_framework"].(string); ok && baseFramework != "" {
			runtime := d.inferRuntimeFromFramework(baseFramework)
			if runtime != "" {
				value := model.DimensionValue{
					Value:      runtime,
					Confidence: 0.85, // High confidence from framework inference
					Source:     source.Source,
					DetectedAt: source.DetectedAt,
					Evidence: map[string]interface{}{
						"method":         "inferred_from_base_framework",
						"base_framework": baseFramework,
					},
				}
				values = append(values, value)
				log.Debugf("Inferred %s runtime from base framework %s", runtime, baseFramework)
				continue
			}
		}

		// Method 3: Check environment variables
		if env, ok := source.Evidence["environment"].(map[string]string); ok {
			if runtime := d.detectFromEnvironment(env); runtime != "" {
				value := model.DimensionValue{
					Value:      runtime,
					Confidence: 0.80,
					Source:     source.Source,
					DetectedAt: source.DetectedAt,
					Evidence: map[string]interface{}{
						"method":      "environment_variables",
						"environment": env,
					},
				}
				values = append(values, value)
				log.Debugf("Detected %s runtime from environment variables", runtime)
				continue
			}
		}

		// Method 4: Check detected modules
		if modules, ok := source.Evidence["pytorch_modules"].(map[string]bool); ok {
			if d.hasAnyPyTorchModule(modules) {
				value := model.DimensionValue{
					Value:      "pytorch",
					Confidence: 0.75,
					Source:     source.Source,
					DetectedAt: source.DetectedAt,
					Evidence: map[string]interface{}{
						"method":  "detected_modules",
						"modules": modules,
					},
				}
				values = append(values, value)
				log.Debugf("Detected PyTorch runtime from modules")
				continue
			}
		}

		// Method 5: Infer from frameworks list
		for _, fw := range source.Frameworks {
			runtime := d.inferRuntimeFromFramework(fw)
			if runtime != "" {
				value := model.DimensionValue{
					Value:      runtime,
					Confidence: 0.70,
					Source:     source.Source,
					DetectedAt: source.DetectedAt,
					Evidence: map[string]interface{}{
						"method":    "inferred_from_framework",
						"framework": fw,
					},
				}
				values = append(values, value)
				log.Debugf("Inferred %s runtime from framework %s", runtime, fw)
				break // Only take first match
			}
		}
	}

	return values, nil
}

// Validate checks if detected value is valid in this dimension
func (d *RuntimeDetector) Validate(value string) bool {
	return d.knownRuntimes[strings.ToLower(value)]
}

// inferRuntimeFromFramework infers runtime from framework name
func (d *RuntimeDetector) inferRuntimeFromFramework(framework string) string {
	fw := strings.ToLower(framework)

	// PyTorch-based frameworks
	pytorchFrameworks := []string{
		"megatron", "deepspeed", "fairscale", "pytorch_lightning",
		"lightning", "horovod", "transformers",
	}
	for _, pytorchFw := range pytorchFrameworks {
		if fw == pytorchFw || strings.Contains(fw, pytorchFw) {
			return "pytorch"
		}
	}

	// TensorFlow-based frameworks
	if strings.Contains(fw, "tensorflow") || strings.Contains(fw, "keras") {
		return "tensorflow"
	}

	// JAX-based frameworks
	if strings.Contains(fw, "jax") || strings.Contains(fw, "flax") {
		return "jax"
	}

	return ""
}

// detectFromEnvironment detects runtime from environment variables
func (d *RuntimeDetector) detectFromEnvironment(env map[string]string) string {
	// PyTorch indicators
	pytorchVars := []string{
		"PYTORCH_VERSION", "TORCH_HOME", "TORCH_CUDA_ARCH_LIST",
	}
	for _, key := range pytorchVars {
		if _, exists := env[key]; exists {
			return "pytorch"
		}
	}

	// TensorFlow indicators
	if _, exists := env["TF_CONFIG"]; exists {
		return "tensorflow"
	}

	// JAX indicators
	if _, exists := env["JAX_PLATFORMS"]; exists {
		return "jax"
	}

	return ""
}

// hasAnyPyTorchModule checks if any PyTorch-related modules are detected
func (d *RuntimeDetector) hasAnyPyTorchModule(modules map[string]bool) bool {
	pytorchModules := []string{
		"torch", "pytorch", "torch.nn", "torch.optim",
	}

	for _, module := range pytorchModules {
		if modules[module] {
			return true
		}
	}

	return false
}

// LanguageDetector detects programming language
type LanguageDetector struct {
	knownLanguages map[string]bool
}

// NewLanguageDetector creates a new language detector
func NewLanguageDetector() *LanguageDetector {
	return &LanguageDetector{
		knownLanguages: map[string]bool{
			"python": true,
			"java":   true,
			"go":     true,
			"cpp":    true,
		},
	}
}

// GetDimension returns the dimension this detector handles
func (d *LanguageDetector) GetDimension() model.DetectionDimension {
	return model.DimensionLanguage
}

// Detect performs detection for language dimension
func (d *LanguageDetector) Detect(sources []model.DetectionSource) ([]model.DimensionValue, error) {
	var values []model.DimensionValue

	for _, source := range sources {
		if source.Evidence == nil {
			continue
		}

		// Check system field for python version
		if system, ok := source.Evidence["system"].(map[string]interface{}); ok {
			if pythonVersion, ok := system["python_version"].(string); ok && pythonVersion != "" {
				value := model.DimensionValue{
					Value:      "python",
					Confidence: 0.95, // Very high confidence
					Source:     source.Source,
					DetectedAt: source.DetectedAt,
					Evidence: map[string]interface{}{
						"method":         "python_version",
						"python_version": pythonVersion,
					},
				}
				values = append(values, value)
				log.Debugf("Detected Python language (version: %s)", pythonVersion)
			}
		}

		// Check environment
		if env, ok := source.Evidence["environment"].(map[string]string); ok {
			if pythonHome, exists := env["PYTHON_HOME"]; exists {
				value := model.DimensionValue{
					Value:      "python",
					Confidence: 0.85,
					Source:     source.Source,
					DetectedAt: source.DetectedAt,
					Evidence: map[string]interface{}{
						"method":      "environment_python_home",
						"python_home": pythonHome,
					},
				}
				values = append(values, value)
			}
		}
	}

	return values, nil
}

// Validate checks if detected value is valid in this dimension
func (d *LanguageDetector) Validate(value string) bool {
	return d.knownLanguages[strings.ToLower(value)]
}

// BehaviorDetector detects workload behavior (training, inference, evaluation)
type BehaviorDetector struct {
	knownBehaviors map[string]bool
}

// NewBehaviorDetector creates a new behavior detector
func NewBehaviorDetector() *BehaviorDetector {
	return &BehaviorDetector{
		knownBehaviors: map[string]bool{
			"training":   true,
			"inference":  true,
			"evaluation": true,
			"serving":    true,
		},
	}
}

// GetDimension returns the dimension this detector handles
func (d *BehaviorDetector) GetDimension() model.DetectionDimension {
	return model.DimensionBehavior
}

// Detect performs detection for behavior dimension
func (d *BehaviorDetector) Detect(sources []model.DetectionSource) ([]model.DimensionValue, error) {
	var values []model.DimensionValue

	for _, source := range sources {
		// Check source.Type field (backward compatible)
		if source.Type != "" && d.knownBehaviors[source.Type] {
			value := model.DimensionValue{
				Value:      source.Type,
				Confidence: 0.80,
				Source:     source.Source,
				DetectedAt: source.DetectedAt,
				Evidence: map[string]interface{}{
					"method":      "source_type",
					"source_type": source.Type,
				},
			}
			values = append(values, value)
			log.Debugf("Detected %s behavior from source type", source.Type)
		}

		// Check wandb config for training indicators
		if source.Evidence != nil {
			if wandb, ok := source.Evidence["wandb"].(map[string]interface{}); ok {
				if config, ok := wandb["config"].(map[string]interface{}); ok {
					// Check for training-specific configs
					trainingIndicators := []string{
						"train_iters", "num_epochs", "learning_rate",
						"optimizer", "loss_function",
					}
					for _, indicator := range trainingIndicators {
						if _, exists := config[indicator]; exists {
							value := model.DimensionValue{
								Value:      "training",
								Confidence: 0.75,
								Source:     source.Source,
								DetectedAt: source.DetectedAt,
								Evidence: map[string]interface{}{
									"method":    "wandb_config_indicators",
									"indicator": indicator,
								},
							}
							values = append(values, value)
							log.Debugf("Detected training behavior from wandb config (%s)", indicator)
							break // Only add once
						}
					}
				}
			}
		}
	}

	return values, nil
}

// Validate checks if detected value is valid in this dimension
func (d *BehaviorDetector) Validate(value string) bool {
	return d.knownBehaviors[strings.ToLower(value)]
}
