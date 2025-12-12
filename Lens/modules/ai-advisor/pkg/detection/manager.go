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

	// 5. Initialize WandB detector
	wandbDetector = NewWandBFrameworkDetector(detectionManager)
	log.Info("WandB framework detector initialized")

	// 6. Initialize and register TaskCreator
	// TaskCreator will automatically create metadata collection tasks after detection completes
	taskCreator = RegisterTaskCreatorWithDetectionManager(detectionManager, instanceID)
	log.Info("TaskCreator registered - metadata collection tasks will be created automatically after detection")

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
