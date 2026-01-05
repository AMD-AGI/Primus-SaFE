package detection

import (
	"context"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/framework"
	configHelper "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/helper/config"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
)

var (
	// Global instances
	detectionManager *framework.FrameworkDetectionManager
	wandbDetector    *WandBFrameworkDetector
	configManager    *FrameworkConfigManager
	patternMatchers  map[string]*PatternMatcher
	taskCreator      *TaskCreator
	layerResolver    *FrameworkLayerResolver
)

// InitializeDetectionManager initializes framework detection manager and all components
func InitializeDetectionManager(
	metadataFacade database.AiWorkloadMetadataFacadeInterface,
	systemConfigMgr *configHelper.Manager,
	instanceID string,
) (*framework.FrameworkDetectionManager, error) {

	// 1. Create detection manager with default config
	detectionConfig := framework.DefaultDetectionConfig()
	detectionManager = framework.NewFrameworkDetectionManager(
		metadataFacade,
		detectionConfig,
	)
	log.Info("Framework detection manager initialized")

	// 2. Initialize config manager
	configManager = NewFrameworkConfigManager(systemConfigMgr)

	// 3. Load all framework configurations
	ctx := context.Background()
	if err := configManager.LoadAllFrameworks(ctx); err != nil {
		log.Warnf("Failed to load some framework configs: %v", err)
	}

	// 4. Initialize pattern matchers for each framework
	patternMatchers = make(map[string]*PatternMatcher)
	for _, frameworkName := range configManager.ListFrameworks() {
		patterns := configManager.GetFramework(frameworkName)
		if patterns == nil {
			continue
		}

		matcher, err := NewPatternMatcher(patterns)
		if err != nil {
			log.Warnf("Failed to create matcher for %s: %v", frameworkName, err)
			continue
		}

		patternMatchers[frameworkName] = matcher
		log.Infof("Initialized pattern matcher for framework: %s", frameworkName)
	}

	// 5. Initialize layer resolver
	layerResolver = NewFrameworkLayerResolver(configManager)
	log.Info("Framework layer resolver initialized")

	// 6. Initialize WandB detector
	wandbDetector = NewWandBFrameworkDetector(detectionManager)
	log.Info("WandB framework detector initialized")

	// 7. Initialize and register TaskCreator
	// Note: In v2 architecture, DetectionCoordinator directly creates follow-up tasks
	// This registration is retained for backward compatibility with v1 event-driven approach
	// New detections through DetectionCoordinator bypass this event mechanism
	taskCreator = RegisterTaskCreatorWithDetectionManager(detectionManager, instanceID)
	log.Info("TaskCreator registered (v1 compatibility) - also provides ScanForUndetectedWorkloads")

	log.Infof("Framework detection system initialized with %d frameworks", len(patternMatchers))
	return detectionManager, nil
}

// GetDetectionManager returns the global detection manager
func GetDetectionManager() *framework.FrameworkDetectionManager {
	return detectionManager
}

// GetWandBDetector returns the global WandB detector
func GetWandBDetector() *WandBFrameworkDetector {
	return wandbDetector
}

// GetConfigManager returns the global config manager
func GetConfigManager() *FrameworkConfigManager {
	return configManager
}

// GetLayerResolver returns the global layer resolver
func GetLayerResolver() *FrameworkLayerResolver {
	return layerResolver
}

// GetPatternMatcher returns the pattern matcher for a framework
func GetPatternMatcher(framework string) *PatternMatcher {
	if patternMatchers == nil {
		return nil
	}
	return patternMatchers[framework]
}

// ListAvailableFrameworks returns all available framework names
func ListAvailableFrameworks() []string {
	if configManager == nil {
		return []string{}
	}
	return configManager.ListFrameworks()
}

// GetTaskCreator returns the global task creator
func GetTaskCreator() *TaskCreator {
	return taskCreator
}

// ============================================================================
// Inference Detection Service (Phase 3)
// ============================================================================

// InferenceDetectionRequest represents a request to detect inference frameworks
type InferenceDetectionRequest struct {
	WorkloadUID     string            `json:"workload_uid"`
	PodName         string            `json:"pod_name"`
	Namespace       string            `json:"namespace"`
	ProcessNames    []string          `json:"process_names"`
	ProcessCmdlines []string          `json:"process_cmdlines"`
	ImageName       string            `json:"image_name"`
	ContainerPorts  []int             `json:"container_ports"`
	EnvVars         map[string]string `json:"env_vars"`
}

// InferenceDetectionResponse represents the result of inference detection
type InferenceDetectionResponse struct {
	Detected       bool     `json:"detected"`
	FrameworkName  string   `json:"framework_name,omitempty"`
	FrameworkType  string   `json:"framework_type,omitempty"`
	Confidence     float64  `json:"confidence,omitempty"`
	MatchedSources []string `json:"matched_sources,omitempty"`
	Evidence       []string `json:"evidence,omitempty"`
}

// DetectInferenceFramework detects inference framework from the provided context
func DetectInferenceFramework(ctx context.Context, req *InferenceDetectionRequest) (*InferenceDetectionResponse, error) {
	if patternMatchers == nil || len(patternMatchers) == 0 {
		return &InferenceDetectionResponse{Detected: false}, nil
	}

	// Build match context
	matchCtx := &InferenceMatchContext{
		ProcessNames:    req.ProcessNames,
		ProcessCmdlines: req.ProcessCmdlines,
		ImageName:       req.ImageName,
		ContainerPorts:  req.ContainerPorts,
		EnvVars:         req.EnvVars,
	}

	// Try each inference framework matcher
	var bestMatch *InferenceMatchResult
	var bestConfidence float64

	for frameworkName, matcher := range patternMatchers {
		// Skip training frameworks
		if !matcher.IsInferenceFramework() {
			continue
		}

		result := matcher.MatchInference(matchCtx)
		if result.Matched && result.Confidence > bestConfidence {
			bestMatch = result
			bestConfidence = result.Confidence
			log.Debugf("Inference framework %s matched with confidence %.2f", frameworkName, result.Confidence)
		}
	}

	if bestMatch == nil {
		return &InferenceDetectionResponse{Detected: false}, nil
	}

	// Report detection to manager if available
	if detectionManager != nil && req.WorkloadUID != "" {
		reportInferenceDetection(ctx, req.WorkloadUID, bestMatch)
	}

	return &InferenceDetectionResponse{
		Detected:       true,
		FrameworkName:  bestMatch.FrameworkName,
		FrameworkType:  bestMatch.FrameworkType,
		Confidence:     bestMatch.Confidence,
		MatchedSources: bestMatch.MatchedSources,
		Evidence:       bestMatch.Evidence,
	}, nil
}

// reportInferenceDetection reports the inference detection to the detection manager
func reportInferenceDetection(ctx context.Context, workloadUID string, match *InferenceMatchResult) {
	if detectionManager == nil || match == nil {
		return
	}

	// Build evidence map
	evidence := make(map[string]interface{})
	evidence["matched_sources"] = match.MatchedSources
	evidence["evidence"] = match.Evidence
	evidence["detection_method"] = "inference_pattern_matcher"

	// Report detection using the existing API
	err := detectionManager.ReportDetection(
		ctx,
		workloadUID,
		"inference_detector",   // source
		match.FrameworkName,    // framework
		FrameworkTypeInference, // taskType
		match.Confidence,       // confidence
		evidence,               // evidence
	)

	if err != nil {
		log.Warnf("Failed to report inference detection for %s: %v", workloadUID, err)
	} else {
		log.Infof("Reported inference detection for workload %s: framework=%s, confidence=%.2f",
			workloadUID, match.FrameworkName, match.Confidence)
	}
}

// ListInferenceFrameworks returns all available inference framework names
func ListInferenceFrameworks() []string {
	if configManager == nil {
		return []string{}
	}
	return configManager.ListInferenceFrameworks()
}

// ListTrainingFrameworks returns all available training framework names
func ListTrainingFrameworks() []string {
	if configManager == nil {
		return []string{}
	}
	return configManager.ListTrainingFrameworks()
}

// GetInferencePatternMatchers returns all inference framework pattern matchers
func GetInferencePatternMatchers() map[string]*PatternMatcher {
	if patternMatchers == nil {
		return nil
	}

	inferenceMatchers := make(map[string]*PatternMatcher)
	for name, matcher := range patternMatchers {
		if matcher.IsInferenceFramework() {
			inferenceMatchers[name] = matcher
		}
	}
	return inferenceMatchers
}
