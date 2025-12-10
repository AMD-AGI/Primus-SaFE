package logs

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/framework"
	"github.com/sirupsen/logrus"
)

// WandBDetectionRequest request data reported by wandb-exporter
type WandBDetectionRequest struct {
	Source      string        `json:"source"`                 // "wandb"
	Type        string        `json:"type"`                   // "framework_detection_raw"
	Version     string        `json:"version"`                // "1.0"
	WorkloadUID string        `json:"workload_uid,omitempty"` // Optional (for compatibility)
	PodUID      string        `json:"pod_uid,omitempty"`
	PodName     string        `json:"pod_name"` // Required: client gets from environment variable
	Namespace   string        `json:"namespace"`
	Evidence    WandBEvidence `json:"evidence"` // Raw evidence
	Hints       WandBHints    `json:"hints"`    // Lightweight hints
	Timestamp   float64       `json:"timestamp"`
}

// WandBEvidence raw evidence data
type WandBEvidence struct {
	WandB             WandBInfo                         `json:"wandb"`
	Environment       map[string]string                 `json:"environment"`
	PyTorch           *PyTorchInfo                      `json:"pytorch,omitempty"`
	WrapperFrameworks map[string]map[string]interface{} `json:"wrapper_frameworks,omitempty"` // Wrapper framework detection results
	BaseFrameworks    map[string]map[string]interface{} `json:"base_frameworks,omitempty"`    // Base framework detection results
	System            map[string]interface{}            `json:"system"`
}

// WandBInfo WandB project information
type WandBInfo struct {
	Project string                 `json:"project"`
	Name    string                 `json:"name"`
	ID      string                 `json:"id"`
	Config  map[string]interface{} `json:"config"`
	Tags    []string               `json:"tags"`
}

// PyTorchInfo PyTorch environment information
type PyTorchInfo struct {
	Available       bool            `json:"available"`
	Version         string          `json:"version"`
	CudaAvailable   bool            `json:"cuda_available"`
	DetectedModules map[string]bool `json:"detected_modules"`
}

// WandBHints pre-judgment hints (supporting dual-layer framework detection)
type WandBHints struct {
	WrapperFrameworks  []string                          `json:"wrapper_frameworks"`         // Wrapper frameworks (e.g. primus, lightning)
	BaseFrameworks     []string                          `json:"base_frameworks"`            // Base frameworks (e.g. megatron, deepspeed, jax)
	PossibleFrameworks []string                          `json:"possible_frameworks"`        // All frameworks (maintain backward compatibility)
	Confidence         string                            `json:"confidence"`                 // low/medium/high
	PrimaryIndicators  []string                          `json:"primary_indicators"`         // Detection indicator sources
	FrameworkLayers    map[string]map[string]interface{} `json:"framework_layers,omitempty"` // Framework hierarchy mapping
}

// DetectionResult detection result (supporting dual-layer frameworks)
type DetectionResult struct {
	Framework        string // Primary framework (wrapper or base)
	FrameworkLayer   string // Framework layer: wrapper or base
	WrapperFramework string // Wrapper framework (if any)
	BaseFramework    string // Base framework (if any)
	Confidence       float64
	Method           string
	MatchedEnvVars   []string
	MatchedModules   []string
}

// WandBFrameworkDetector WandB framework detector
type WandBFrameworkDetector struct {
	detectionManager *framework.FrameworkDetectionManager
}

// NewWandBFrameworkDetector creates detector
func NewWandBFrameworkDetector(
	detectMgr *framework.FrameworkDetectionManager,
) *WandBFrameworkDetector {
	return &WandBFrameworkDetector{
		detectionManager: detectMgr,
	}
}

// ProcessWandBDetection processes WandB detection request
func (d *WandBFrameworkDetector) ProcessWandBDetection(
	ctx context.Context,
	req *WandBDetectionRequest,
) error {
	// Record metrics: request count and duration
	startTime := time.Now()
	IncWandBRequestCount("detection")
	defer func() {
		ObserveWandBRequestDuration("detection", time.Since(startTime).Seconds())
	}()

	// 1. Parse WorkloadUID from PodName
	workloadUID, err := resolveWorkloadUID(req.WorkloadUID, req.PodName)
	if err != nil {
		IncWandBRequestErrorCount("detection", "validation")
		return err
	}

	logrus.Infof("Processing WandB detection for pod %s -> workload %s", req.PodName, workloadUID)

	// 2. Record hints (for monitoring and tuning, supporting dual-layer frameworks)
	if len(req.Hints.WrapperFrameworks) > 0 || len(req.Hints.BaseFrameworks) > 0 {
		logrus.Infof("WandB hints (dual-layer frameworks): wrapper=%v, base=%v, confidence=%s",
			req.Hints.WrapperFrameworks,
			req.Hints.BaseFrameworks,
			req.Hints.Confidence)
		logrus.Debugf("WandB hints indicators: %v", req.Hints.PrimaryIndicators)
	} else if len(req.Hints.PossibleFrameworks) > 0 {
		// Backward compatibility: old format hints
		logrus.Debugf("WandB hints (legacy): frameworks=%v, confidence=%s, indicators=%v",
			req.Hints.PossibleFrameworks,
			req.Hints.Confidence,
			req.Hints.PrimaryIndicators)
	}

	// 3. Execute framework detection rules
	result := d.detectFramework(req)
	if result == nil || result.Framework == "" {
		logrus.Debug("No framework detected from WandB data")
		IncFrameworkDetectionErrors("wandb", "no_match")
		return nil
	}

	// Output different logs based on framework layer
	if result.FrameworkLayer == "wrapper" && result.BaseFramework != "" {
		logrus.Infof("✓ Detected framework from WandB: %s/%s (wrapper/base, confidence: %.2f, method: %s)",
			result.Framework, result.BaseFramework, result.Confidence, result.Method)
	} else if result.FrameworkLayer != "" {
		logrus.Infof("✓ Detected framework from WandB: %s (layer: %s, confidence: %.2f, method: %s)",
			result.Framework, result.FrameworkLayer, result.Confidence, result.Method)
	} else {
		logrus.Infof("✓ Detected framework from WandB: %s (confidence: %.2f, method: %s)",
			result.Framework, result.Confidence, result.Method)
	}

	// Record detection metrics
	IncFrameworkDetectionCount(result.Framework, result.Method, "wandb")
	ObserveFrameworkDetectionConfidence(result.Framework, result.Method, result.Confidence)

	// 4. Construct evidence (including dual-layer framework information)
	evidence := map[string]interface{}{
		"method":            result.Method,
		"framework_layer":   result.FrameworkLayer,
		"wrapper_framework": result.WrapperFramework,
		"base_framework":    result.BaseFramework,
		"wandb_project":     req.Evidence.WandB.Project,
		"wandb_name":        req.Evidence.WandB.Name,
		"environment_vars":  result.MatchedEnvVars,
		"pytorch_modules":   result.MatchedModules,
		"hints":             req.Hints,
		"pod_name":          req.PodName,
		"detected_at":       time.Now().Format(time.RFC3339),
	}

	// 5. Report to FrameworkDetectionManager
	err = d.detectionManager.ReportDetection(
		ctx,
		workloadUID,
		"wandb",
		result.Framework,
		"training",
		result.Confidence,
		evidence,
	)

	if err != nil {
		logrus.Errorf("Failed to report WandB detection: %v", err)
		IncFrameworkDetectionErrors("wandb", "report_failed")
		IncWandBRequestErrorCount("detection", "report_failed")
		return err
	}

	logrus.Infof("✓ Successfully reported WandB detection for workload %s", workloadUID)

	return nil
}

// detectFramework detects framework based on WandB data (supporting dual-layer frameworks)
func (d *WandBFrameworkDetector) detectFramework(
	req *WandBDetectionRequest,
) *DetectionResult {

	// Use Import detection results first (strongest indicator)
	if result := d.detectFromImportEvidence(req.Evidence); result != nil {
		return result
	}

	// Apply detection rules by priority

	// 1. Environment variable detection (highest priority, confidence: 0.80)
	if result := d.detectFromEnvVars(req.Evidence.Environment); result != nil {
		return result
	}

	// 2. WandB Config detection (confidence: 0.70)
	if result := d.detectFromWandBConfig(req.Evidence.WandB.Config); result != nil {
		return result
	}

	// 3. PyTorch module detection (confidence: 0.60)
	if req.Evidence.PyTorch != nil && req.Evidence.PyTorch.Available {
		if result := d.detectFromPyTorchModules(req.Evidence.PyTorch); result != nil {
			return result
		}
	}

	// 4. WandB Project name detection (confidence: 0.50)
	if result := d.detectFromWandBProject(req.Evidence.WandB.Project); result != nil {
		return result
	}

	return nil
}

// detectFromImportEvidence extracts framework information from Import detection evidence (strongest indicator)
func (d *WandBFrameworkDetector) detectFromImportEvidence(evidence WandBEvidence) *DetectionResult {
	var wrapperFramework string
	var baseFramework string

	// Check wrapper_frameworks
	if len(evidence.WrapperFrameworks) > 0 {
		// Prioritize Primus (if exists)
		if primusInfo, ok := evidence.WrapperFrameworks["primus"]; ok {
			if detected, _ := primusInfo["detected"].(bool); detected {
				wrapperFramework = "primus"
				// Try to get Primus's base_framework
				if baseFrameworkVal, ok := primusInfo["base_framework"]; ok && baseFrameworkVal != nil {
					if baseStr, ok := baseFrameworkVal.(string); ok && baseStr != "" {
						baseFramework = strings.ToLower(baseStr)
					}
				}
			}
		}

		// Other wrapper frameworks
		if wrapperFramework == "" {
			for frameworkName, frameworkInfo := range evidence.WrapperFrameworks {
				if detected, ok := frameworkInfo["detected"].(bool); ok && detected {
					wrapperFramework = frameworkName
					break
				}
			}
		}
	}

	// Check base_frameworks
	if len(evidence.BaseFrameworks) > 0 && baseFramework == "" {
		// Priority: megatron > deepspeed > jax > transformers
		priority := []string{"megatron", "deepspeed", "jax", "transformers"}
		for _, frameworkName := range priority {
			if frameworkInfo, ok := evidence.BaseFrameworks[frameworkName]; ok {
				if detected, ok := frameworkInfo["detected"].(bool); ok && detected {
					baseFramework = frameworkName
					break
				}
			}
		}

		// If not found in priority list, check other frameworks
		if baseFramework == "" {
			for frameworkName, frameworkInfo := range evidence.BaseFrameworks {
				if detected, ok := frameworkInfo["detected"].(bool); ok && detected {
					baseFramework = frameworkName
					break
				}
			}
		}
	}

	// Construct detection result
	if wrapperFramework != "" || baseFramework != "" {
		result := &DetectionResult{
			Confidence: 0.90, // Import detection is the strongest indicator
			Method:     "import_detection",
		}

		// Report wrapper framework first
		if wrapperFramework != "" {
			result.Framework = wrapperFramework
			result.FrameworkLayer = "wrapper"
			result.WrapperFramework = wrapperFramework
			result.BaseFramework = baseFramework
		} else {
			result.Framework = baseFramework
			result.FrameworkLayer = "base"
			result.BaseFramework = baseFramework
		}

		return result
	}

	return nil
}

// detectFromEnvVars detects from environment variables (supporting dual-layer frameworks)
func (d *WandBFrameworkDetector) detectFromEnvVars(env map[string]string) *DetectionResult {

	// Wrapper Frameworks

	// Primus (wrapper)
	primusVars := []string{"PRIMUS_CONFIG", "PRIMUS_VERSION", "PRIMUS_BACKEND"}
	if matched := hasAnyKey(env, primusVars); len(matched) > 0 {
		result := &DetectionResult{
			Framework:        "primus",
			FrameworkLayer:   "wrapper",
			WrapperFramework: "primus",
			Confidence:       0.80,
			Method:           "env_vars",
			MatchedEnvVars:   matched,
		}
		// Check PRIMUS_BACKEND to determine base framework
		if backend := env["PRIMUS_BACKEND"]; backend != "" {
			result.BaseFramework = strings.ToLower(backend)
		}
		return result
	}

	// Base Frameworks

	// DeepSpeed (base)
	deepspeedVars := []string{"DEEPSPEED_CONFIG", "DS_CONFIG", "DEEPSPEED_VERSION"}
	if matched := hasAnyKey(env, deepspeedVars); len(matched) > 0 {
		return &DetectionResult{
			Framework:      "deepspeed",
			FrameworkLayer: "base",
			BaseFramework:  "deepspeed",
			Confidence:     0.80,
			Method:         "env_vars",
			MatchedEnvVars: matched,
		}
	}

	// Megatron (base)
	megatronVars := []string{"MEGATRON_CONFIG", "MEGATRON_LM_PATH"}
	if matched := hasAnyKey(env, megatronVars); len(matched) > 0 {
		return &DetectionResult{
			Framework:      "megatron",
			FrameworkLayer: "base",
			BaseFramework:  "megatron",
			Confidence:     0.80,
			Method:         "env_vars",
			MatchedEnvVars: matched,
		}
	}

	// JAX (base)
	jaxVars := []string{"JAX_BACKEND", "JAX_PLATFORMS"}
	if matched := hasAnyKey(env, jaxVars); len(matched) > 0 {
		return &DetectionResult{
			Framework:      "jax",
			FrameworkLayer: "base",
			BaseFramework:  "jax",
			Confidence:     0.80,
			Method:         "env_vars",
			MatchedEnvVars: matched,
		}
	}

	// Generic FRAMEWORK environment variable (determine layer based on framework name)
	if fw := env["FRAMEWORK"]; fw != "" {
		fwLower := strings.ToLower(fw)
		result := &DetectionResult{
			Framework:      fwLower,
			Confidence:     0.75,
			Method:         "env_framework",
			MatchedEnvVars: []string{"FRAMEWORK"},
		}

		// Determine if wrapper or base
		if isWrapperFramework(fwLower) {
			result.FrameworkLayer = "wrapper"
			result.WrapperFramework = fwLower
		} else {
			result.FrameworkLayer = "base"
			result.BaseFramework = fwLower
		}

		return result
	}

	return nil
}

// detectFromWandBConfig detects from WandB Config (supporting dual-layer frameworks)
func (d *WandBFrameworkDetector) detectFromWandBConfig(config map[string]interface{}) *DetectionResult {

	// Check config.framework field
	if fw, ok := config["framework"]; ok {
		framework := strings.ToLower(fmt.Sprintf("%v", fw))
		result := &DetectionResult{
			Framework:  framework,
			Confidence: 0.70,
			Method:     "wandb_config_framework",
		}

		// Determine framework layer
		if isWrapperFramework(framework) {
			result.FrameworkLayer = "wrapper"
			result.WrapperFramework = framework
		} else {
			result.FrameworkLayer = "base"
			result.BaseFramework = framework
		}

		return result
	}

	// Check config.base_framework field (Primus specific)
	if baseFw, ok := config["base_framework"]; ok {
		baseFramework := strings.ToLower(fmt.Sprintf("%v", baseFw))
		return &DetectionResult{
			Framework:      baseFramework,
			FrameworkLayer: "base",
			BaseFramework:  baseFramework,
			Confidence:     0.70,
			Method:         "wandb_config_base_framework",
		}
	}

	// Check config.trainer field (may contain framework information)
	if trainer, ok := config["trainer"]; ok {
		trainerStr := strings.ToLower(fmt.Sprintf("%v", trainer))
		if strings.Contains(trainerStr, "deepspeed") {
			return &DetectionResult{
				Framework:      "deepspeed",
				FrameworkLayer: "base",
				BaseFramework:  "deepspeed",
				Confidence:     0.65,
				Method:         "wandb_config_trainer",
			}
		}
		if strings.Contains(trainerStr, "megatron") {
			return &DetectionResult{
				Framework:      "megatron",
				FrameworkLayer: "base",
				BaseFramework:  "megatron",
				Confidence:     0.65,
				Method:         "wandb_config_trainer",
			}
		}
	}

	// Check specific framework config keys
	configKeys := map[string]struct {
		framework string
		layer     string
	}{
		"primus_config":    {"primus", "wrapper"},
		"deepspeed_config": {"deepspeed", "base"},
		"megatron_config":  {"megatron", "base"},
	}

	for key, info := range configKeys {
		if _, exists := config[key]; exists {
			result := &DetectionResult{
				Framework:      info.framework,
				FrameworkLayer: info.layer,
				Confidence:     0.65,
				Method:         "wandb_config_key",
			}

			if info.layer == "wrapper" {
				result.WrapperFramework = info.framework
			} else {
				result.BaseFramework = info.framework
			}

			return result
		}
	}

	return nil
}

// detectFromPyTorchModules detects from PyTorch modules (supporting dual-layer frameworks)
func (d *WandBFrameworkDetector) detectFromPyTorchModules(pytorch *PyTorchInfo) *DetectionResult {

	modules := pytorch.DetectedModules
	if modules == nil {
		return nil
	}

	// Wrapper frameworks
	if modules["lightning"] {
		return &DetectionResult{
			Framework:        "lightning",
			FrameworkLayer:   "wrapper",
			WrapperFramework: "lightning",
			Confidence:       0.60,
			Method:           "pytorch_modules",
			MatchedModules:   []string{"lightning"},
		}
	}

	// Base frameworks (check by priority)
	if modules["deepspeed"] {
		return &DetectionResult{
			Framework:      "deepspeed",
			FrameworkLayer: "base",
			BaseFramework:  "deepspeed",
			Confidence:     0.60,
			Method:         "pytorch_modules",
			MatchedModules: []string{"deepspeed"},
		}
	}

	if modules["megatron"] {
		return &DetectionResult{
			Framework:      "megatron",
			FrameworkLayer: "base",
			BaseFramework:  "megatron",
			Confidence:     0.60,
			Method:         "pytorch_modules",
			MatchedModules: []string{"megatron"},
		}
	}

	if modules["transformers"] {
		return &DetectionResult{
			Framework:      "transformers",
			FrameworkLayer: "base",
			BaseFramework:  "transformers",
			Confidence:     0.55,
			Method:         "pytorch_modules",
			MatchedModules: []string{"transformers"},
		}
	}

	return nil
}

// detectFromWandBProject detects from WandB project name (supporting dual-layer frameworks)
func (d *WandBFrameworkDetector) detectFromWandBProject(project string) *DetectionResult {
	if project == "" {
		return nil
	}

	projectLower := strings.ToLower(project)

	// Wrapper frameworks
	wrapperFrameworks := map[string][]string{
		"primus":    {"primus", "primus-training", "primus-exp"},
		"lightning": {"lightning", "pl-training", "pytorch-lightning"},
	}

	for frameworkName, patterns := range wrapperFrameworks {
		for _, pattern := range patterns {
			if strings.Contains(projectLower, pattern) {
				return &DetectionResult{
					Framework:        frameworkName,
					FrameworkLayer:   "wrapper",
					WrapperFramework: frameworkName,
					Confidence:       0.50,
					Method:           "wandb_project_name",
				}
			}
		}
	}

	// Base frameworks
	baseFrameworks := map[string][]string{
		"deepspeed":    {"deepspeed", "ds-training", "deepspeed-exp"},
		"megatron":     {"megatron", "megatron-lm", "megatron-training"},
		"jax":          {"jax", "jax-training"},
		"transformers": {"transformers", "hf-transformers"},
	}

	for frameworkName, patterns := range baseFrameworks {
		for _, pattern := range patterns {
			if strings.Contains(projectLower, pattern) {
				return &DetectionResult{
					Framework:      frameworkName,
					FrameworkLayer: "base",
					BaseFramework:  frameworkName,
					Confidence:     0.50,
					Method:         "wandb_project_name",
				}
			}
		}
	}

	return nil
}

// hasAnyKey checks if map contains any of the keys
func hasAnyKey(m map[string]string, keys []string) []string {
	matched := []string{}
	for _, key := range keys {
		if _, ok := m[key]; ok {
			matched = append(matched, key)
		}
	}
	return matched
}

// isWrapperFramework determines if the framework is a wrapper framework
func isWrapperFramework(framework string) bool {
	wrapperFrameworks := map[string]bool{
		"primus":               true,
		"lightning":            true,
		"pytorch_lightning":    true,
		"transformers_trainer": true,
	}
	return wrapperFrameworks[framework]
}
