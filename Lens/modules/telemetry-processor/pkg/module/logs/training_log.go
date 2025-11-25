package logs

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/constant"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	dbModel "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/framework"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/helper/config"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/utils/mapUtil"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/utils/regexUtil"
	tpapi "github.com/AMD-AGI/Primus-SaFE/Lens/telemetry-processor/pkg/api"
)

var (
	// Global singletons (initialized at startup)
	detectionManager *framework.FrameworkDetectionManager
	configManager    *FrameworkConfigManager
	patternMatchers  map[string]*PatternMatcher
	
	perfRegexps = map[string]*regexp.Regexp{
		"primus-legancy": regexp.MustCompile(`\.*iteration\s+(?P<CurrentIteration>\d+)\s*/\s*(?P<TargetIteration>\d+)\s*\|\s*consumed samples:\s+(?P<ConsumedSamples>\d+)\s*\|\s*elapsed\stime\sper\siteration\s\(ms\):\s+(?P<ElapsedTimePerIterationMS>\d+(?:\.\d+)*)/\d+(?:\.\d+)*\s+\|\s+mem\s+usages:\s+(?P<MemUsages>\d+\.\d+)\s+\|\s+throughput\s+per\s+GPU\s+\(TFLOP/s/GPU\):\s+(?P<TFLOPS>\d+(?:\.\d+)*)/\d+(?:\.\d+)*\s+\|\s+tokens\s+per\s+GPU\s+\(tokens/s/GPU\):\s+(?P<TokensPerGPU>\d+(?:\.\d+)*)/\d+(?:\.\d+)*\s+\|\s+learning\s+rate:\s+(?P<LearningRate>[+-]?\d+(?:\.\d+)?(?:[Ee][+-]?\d+)?)\s+\|\s+global\s+batch\s+size:\s+(?P<GlobalBatchSize>\d+(?:\.\d+)*)\s+\|\s+lm\s+loss:\s+(?P<LmLoss>[+-]?\d+(?:\.\d+)?(?:[Ee][+-]?\d+)?)\s+\|\s+loss\s+scale:\s+(?P<LossScale>\d+(?:\.\d+)*)\s+\|\s+grad\s+norm:\s+(?P<GradNorm>\d+(?:\.\d+)*)\s+\|\s+num\s+zeros:\s(?P<NumZeros>\d+(?:\.\d+)*)\s+\|\s+number\s+of\s+skipped\s+iterations:\s+(?P<SkippedIterationsNumber>\d+)\s+\|\s+number\s+of\s+nan\s+iterations:\s+(?P<NanIterationsNumber>\d+)\.*`),
		"primus": regexp.MustCompile(`\.*iteration\s+(?P<CurrentIteration>\d+)\s*/\s*(?P<TargetIteration>\d+)\s*\|` +
			`\s*consumed samples:\s+(?P<ConsumedSamples>\d+)\s*\|` +
			`\s*elapsed\stime\sper\siteration\s\(ms\):\s+(?P<ElapsedTimePerIterationMS>\d+(?:\.\d+)*)/\d+(?:\.\d+)*\s+\|` +
			`\s+rocm\s+mem\s+usage/free/total/usage_ratio:\s+` +
			`(?P<MemUsage>\d+\.\d+)GB/` +
			`(?P<MemFree>\d+\.\d+)GB/` +
			`(?P<MemTotal>\d+\.\d+)GB/` +
			`(?P<MemUsageRatio>\d+\.\d+)%\s+\|` +
			`\s+throughput\s+per\s+GPU\s+\(TFLOP/s/GPU\):\s+(?P<TFLOPS>\d+(?:\.\d+)*)/\d+(?:\.\d+)*\s+\|` +
			`\s+tokens\s+per\s+GPU\s+\(tokens/s/GPU\):\s+(?P<TokensPerGPU>\d+(?:\.\d+)*)/\d+(?:\.\d+)*\s+\|` +
			`\s*learning\s+rate:\s+(?P<LearningRate>[+-]?\d+(?:\.\d+)?(?:[Ee][+-]?\d+)?)\s*\|` +
			`\s+global\s+batch\s+size:\s+(?P<GlobalBatchSize>\d+(?:\.\d+)*)\s+\|` +
			`\s+lm\s+loss:\s+(?P<LmLoss>[+-]?\d+(?:\.\d+)?(?:[Ee][+-]?\d+)?)\s+\|` +
			`\s+loss\s+scale:\s+(?P<LossScale>\d+(?:\.\d+)*)\s+\|` +
			`\s+grad\s+norm:\s+(?P<GradNorm>\d+(?:\.\d+)*)\s+\|` +
			`\s+num\s+zeros:\s(?P<NumZeros>\d+(?:\.\d+)*)\s+\|` +
			`\s+number\s+of\s+skipped\s+iterations:\s+(?P<SkippedIterationsNumber>\d+)\s+\|` +
			`\s+number\s+of\s+nan\s+iterations:\s+(?P<NanIterationsNumber>\d+)\.*`),
		"primus-hip-memory": regexp.MustCompile(`\.*iteration\s+(?P<CurrentIteration>\d+)\s*/\s*(?P<TargetIteration>\d+)\s*\|` +
			`\s*consumed samples:\s+(?P<ConsumedSamples>\d+)\s*\|` +
			`\s*elapsed\stime\sper\siteration\s\(ms\):\s+(?P<ElapsedTimePerIterationMS>\d+(?:\.\d+)*)/\d+(?:\.\d+)*\s+\|` +
			`\s+hip\s+mem\s+usage/free/total/usage_ratio:\s+` +
			`(?P<MemUsage>\d+\.\d+)GB/` +
			`(?P<MemFree>\d+\.\d+)GB/` +
			`(?P<MemTotal>\d+\.\d+)GB/` +
			`(?P<MemUsageRatio>\d+\.\d+)%\s+\|` +
			`\s+throughput\s+per\s+GPU\s+\(TFLOP/s/GPU\):\s+(?P<TFLOPS>\d+(?:\.\d+)*)/\d+(?:\.\d+)*\s+\|` +
			`\s+tokens\s+per\s+GPU\s+\(tokens/s/GPU\):\s+(?P<TokensPerGPU>\d+(?:\.\d+)*)/\d+(?:\.\d+)*\s+\|` +
			`\s*learning\s+rate:\s+(?P<LearningRate>[+-]?\d+(?:\.\d+)?(?:[Ee][+-]?\d+)?)\s*\|` +
			`\s+global\s+batch\s+size:\s+(?P<GlobalBatchSize>\d+(?:\.\d+)*)\s+\|` +
			`\s+lm\s+loss:\s+(?P<LmLoss>[+-]?\d+(?:\.\d+)?(?:[Ee][+-]?\d+)?)\s+\|` +
			`\s+loss\s+scale:\s+(?P<LossScale>\d+(?:\.\d+)*)\s+\|` +
			`\s+grad\s+norm:\s+(?P<GradNorm>\d+(?:\.\d+)*)\s+\|` +
			`\s+num\s+zeros:\s(?P<NumZeros>\d+(?:\.\d+)*)\s+\|` +
			`\s+number\s+of\s+skipped\s+iterations:\s+(?P<SkippedIterationsNumber>\d+)\s+\|` +
			`\s+number\s+of\s+nan\s+iterations:\s+(?P<NanIterationsNumber>\d+)\.*`),
	}
)

// InitializeFrameworkDetection initializes framework detection components
func InitializeFrameworkDetection(
	metadataFacade database.AiWorkloadMetadataFacadeInterface,
	systemConfigMgr *config.Manager,
) error {
	// Initialize detection manager
	detectionManager = framework.NewFrameworkDetectionManager(
		metadataFacade,
		framework.DefaultDetectionConfig(),
	)
	
	// Initialize config manager
	configManager = NewFrameworkConfigManager(systemConfigMgr)
	
	// Load all framework configurations
	ctx := context.Background()
	if err := configManager.LoadAllFrameworks(ctx); err != nil {
		logrus.Warnf("Failed to load some framework configs: %v", err)
	}
	
	// Initialize pattern matchers
	patternMatchers = make(map[string]*PatternMatcher)
	for _, frameworkName := range configManager.ListFrameworks() {
		patterns := configManager.GetFramework(frameworkName)
		if patterns == nil {
			continue
		}
		
		matcher, err := NewPatternMatcher(patterns)
		if err != nil {
			logrus.Warnf("Failed to create matcher for %s: %v", frameworkName, err)
			continue
		}
		
		patternMatchers[frameworkName] = matcher
		logrus.Infof("Initialized pattern matcher for framework: %s", frameworkName)
	}
	
	logrus.Infof("Framework detection initialized with %d frameworks", len(patternMatchers))
	
	// Initialize WandB components
	wandbDetector := NewWandBFrameworkDetector(detectionManager)
	metricsStorage := NewInMemoryMetricsStorage(10000) // Max 10000 metrics per workload
	wandbLogProcessor := NewWandBLogProcessor(metricsStorage)
	
	// Initialize WandB Handler
	InitWandBHandler(wandbDetector, wandbLogProcessor)
	logrus.Info("WandB handler initialized successfully")
	
	// Initialize Framework Detection API Handler
	tpapi.InitFrameworkDetectionHandler(detectionManager)
	logrus.Info("Framework detection API handler initialized successfully")
	
	return nil
}

func WorkloadLog(ctx context.Context, podUid string, msg string, logTime time.Time) error {
	log.Tracef("before consume workload log , pod uid %s", podUid)
	
	// Get workload information
	workloadRefs, err := database.GetFacade().GetWorkload().ListWorkloadPodReferencesByPodUids(ctx, []string{podUid})
	if err != nil || len(workloadRefs) == 0 {
		// If no workload reference, fall back to old logic
		err := singleWorkloadLog(ctx, podUid, msg, logTime)
		if err != nil {
			log.GlobalLogger().WithError(err).Errorln("singleWorkloadLog error")
		}
		return nil
	}
	workloadUID := workloadRefs[0].WorkloadUID
	
	// If framework detection is initialized, use it
	if detectionManager != nil && len(patternMatchers) > 0 {
		// Get current detection result
		detection, err := detectionManager.GetDetection(ctx, workloadUID)
		if err != nil {
			logrus.Warnf("Failed to get detection for %s: %v", workloadUID, err)
		}
		
		// Determine framework to use
		var frameworkName string
		needsDetection := false
		
		if detection != nil && detection.Confidence >= 0.5 {
			// Use existing detection
			frameworkName = detection.Framework
			logrus.Debugf("Using existing framework detection: %s (confidence: %.2f)",
				frameworkName, detection.Confidence)
		} else {
			// Need to detect framework from log
			frameworkName, err = detectFrameworkFromLog(ctx, workloadUID, msg)
			if err != nil {
				logrus.Debugf("Framework not detected from log, using default")
				frameworkName = "primus" // Default
			}
			needsDetection = true
		}
		
		// Report detection if needed
		if needsDetection && frameworkName != "" {
			confidence := calculateDetectionConfidence(frameworkName, msg)
			
			err = detectionManager.ReportDetection(
				ctx,
				workloadUID,
				"log",
				frameworkName,
				"training",
				confidence,
				map[string]interface{}{
					"method":     "log_pattern_match",
					"sample_log": truncateLog(msg, 200),
				},
			)
			if err != nil {
				logrus.Errorf("Failed to report log detection: %v", err)
			} else {
				logrus.Infof("Reported log detection: workload=%s, framework=%s, confidence=%.2f",
					workloadUID, frameworkName, confidence)
			}
		}
		
		// Process log with framework-specific parser
		if err := processLogWithFramework(ctx, podUid, workloadUID, msg, logTime, frameworkName); err != nil {
			logrus.Errorf("Failed to process log with framework: %v", err)
		}
	} else {
		// Fall back to old logic if detection not initialized
		err := singleWorkloadLog(ctx, podUid, msg, logTime)
		if err != nil {
			log.GlobalLogger().WithError(err).Errorln("singleWorkloadLog error")
		}
	}
	
	log.Tracef("workload log  consume success.before consume diagnosis")
	return nil
}

func singleWorkloadLog(ctx context.Context, podUid string, msg string, docTime time.Time) error {
	if hit, err := saveStartTrain(ctx, msg, podUid, docTime); hit {
		log.GlobalLogger().Debugf("start train, podUid: %s, ", podUid)
		IncLogAnalysisCount(constant.TrainingEventStartTrain)
		if err != nil {
			return err
		}
	} else if err != nil {
		log.Errorf("saveStartTrain err %+v", err)
	}
	if hit, err := saveTrainingPerformance(ctx, msg, podUid, docTime); hit {
		log.GlobalLogger().Debugf("Got training performance, podUid: %s, ", podUid)
		IncLogAnalysisCount(constant.TrainingPerformance)
		if err != nil {
			return err
		}
	} else if err != nil {
		log.Errorf("saveTrainingPerformance err %+v", err)
	}

	return nil
}

func saveTrainingPerformance(ctx context.Context, msg, podUid string, docTime time.Time) (bool, error) {
	performance, err := filterTrainingPerformance(msg)
	if err != nil {
		return false, err
	}
	if performance == nil {
		return false, nil
	}
	log.Tracef("save training performance, podUid: %s, peformance %+v ", podUid, performance)
	nearestWorkloadUid := ""
	nearestWorkload, err := database.GetFacade().GetWorkload().GetNearestWorkloadByPodUid(ctx, podUid)
	if err != nil {
		return true, err
	}
	if nearestWorkload != nil {
		nearestWorkloadUid = nearestWorkload.UID
	}
	log.Tracef("nearest workload uid: %s", nearestWorkloadUid)

	workloadReferences, err := database.GetFacade().GetWorkload().ListWorkloadPodReferencesByPodUids(ctx, []string{podUid})
	if err != nil {
		return true, err
	}
	log.Tracef("workload references %+v", workloadReferences)
	for _, reference := range workloadReferences {
		err = saveTrainingPerformanceForSingleWorkload(ctx, podUid, reference.WorkloadUID, nearestWorkloadUid, performance, docTime)
		if err != nil {
			log.Errorf("saveStartTrainForSingleWorkload err %+v", err)
		} else {
			log.Tracef("save training performance for workload %s", reference.WorkloadUID)
		}
	}
	return true, nil
}

func saveTrainingPerformanceForSingleWorkload(ctx context.Context, podId, workloadId, nearestWorkloadId string, perf *model.TrainingPerformance, docTime time.Time) error {
	serial, err := getCurrentRunSerial(ctx, workloadId, nearestWorkloadId)
	if err != nil {
		return err
	}
	existDbPerformance, err := database.GetFacade().GetTraining().GetTrainingPerformanceByWorkloadIdSerialAndIteration(ctx, workloadId, serial, perf.CurrentIteration)
	if err != nil {
		return err
	}
	if existDbPerformance != nil {
		return nil
	}
	ext, _ := mapUtil.EncodeMap(perf)
	existDbPerformance = &dbModel.TrainingPerformance{
		ID:          0,
		PodUUID:     podId,
		Performance: ext,
		Iteration:   int32(perf.CurrentIteration),
		CreatedAt:   docTime,
		Serial:      int32(serial),
		WorkloadUID: workloadId,
	}
	err = database.GetFacade().GetTraining().CreateTrainingPerformance(ctx, existDbPerformance)
	if err != nil {
		return err
	}
	return nil
}

func filterTrainingPerformance(msg string) (*model.TrainingPerformance, error) {
	for name, reg := range perfRegexps {
		if !reg.MatchString(msg) {
			continue
		}
		if strings.Contains(msg, "rocm mem usage/free/total/usage_ratio") {
			log.Debugf("msg %s match regular expression %s", msg, name)
		}
		result := &model.TrainingPerformance{}
		err := regexUtil.RegexToStruct(reg, msg, result)
		if err != nil {
			log.Errorf("Regex match error for %s. %+v", name, err)
		} else {
			return result, nil
		}
	}
	return nil, nil
}

func saveStartTrain(ctx context.Context, msg, podId string, docTime time.Time) (bool, error) {
	if !strings.Contains(strings.TrimSpace(msg), "training ...") {
		return false, nil
	}
	nearestWorkloadUid := ""
	nearestWorkload, err := database.GetFacade().GetWorkload().GetNearestWorkloadByPodUid(ctx, podId)
	if err != nil {
		return false, err
	}
	if nearestWorkload != nil {
		nearestWorkloadUid = nearestWorkload.UID
	}

	workloadReferences, err := database.GetFacade().GetWorkload().ListWorkloadPodReferencesByPodUids(ctx, []string{podId})
	if err != nil {
		return true, err
	}
	for _, reference := range workloadReferences {
		err = saveStartTrainForSingleWorkload(ctx, podId, reference.WorkloadUID, nearestWorkloadUid, docTime)
		if err != nil {
			log.Errorf("saveStartTrainForSingleWorkload err %+v", err)
		}
	}
	return true, nil
}

func getCurrentRunSerial(ctx context.Context, workloadId, nearestWorkloadId string) (int, error) {
	existEvent, err := database.GetFacade().GetWorkload().GetWorkloadEventByWorkloadUidAndNearestWorkloadIdAndType(ctx, workloadId, nearestWorkloadId, constant.TrainingEventStartTrain)
	if err != nil {
		return 0, err
	}
	if existEvent != nil {
		return 1, nil
	}
	serial := 1
	latestEvent, err := database.GetFacade().GetWorkload().GetLatestOtherWorkloadEvent(ctx, workloadId, nearestWorkloadId)
	if err != nil {
		return 0, err
	}
	if latestEvent != nil {
		serial = int(latestEvent.RunSerial + 1)
	}
	return serial, nil
}

func saveStartTrainForSingleWorkload(ctx context.Context, podId, workloadId, nearestWorkloadId string, docTime time.Time) error {
	serial, err := getCurrentRunSerial(ctx, workloadId, nearestWorkloadId)
	if err != nil {
		return err
	}
	newEvent := &dbModel.WorkloadEvent{
		WorkloadUID:        workloadId,
		Type:               constant.TrainingEventStartTrain,
		RunSerial:          int32(serial),
		CreatedAt:          docTime,
		PodUID:             podId,
		NearestWorkloadUID: nearestWorkloadId,
	}
	err = database.GetFacade().GetWorkload().CreateWorkloadEvent(ctx, newEvent)
	if err != nil {
		return err
	}
	return nil
}

// detectFrameworkFromLog detects framework from log content
func detectFrameworkFromLog(ctx context.Context, workloadUID, logLine string) (string, error) {
	// Try each framework's pattern matcher
	bestMatch := ""
	bestConfidence := 0.0
	
	for frameworkName, matcher := range patternMatchers {
		result := matcher.MatchIdentify(logLine)
		if result.Matched && result.Confidence > bestConfidence {
			bestMatch = frameworkName
			bestConfidence = result.Confidence
		}
	}
	
	if bestMatch != "" {
		logrus.Infof("Detected framework %s from log with confidence %.2f", bestMatch, bestConfidence)
		return bestMatch, nil
	}
	
	return "", fmt.Errorf("no framework detected")
}

// calculateDetectionConfidence calculates confidence for log-based detection
func calculateDetectionConfidence(frameworkName, logLine string) float64 {
	matcher, ok := patternMatchers[frameworkName]
	if !ok {
		return 0.5 // Default confidence
	}
	
	result := matcher.MatchIdentify(logLine)
	if result.Matched {
		return result.Confidence
	}
	
	return 0.5
}

// processLogWithFramework processes log using framework-specific logic
func processLogWithFramework(
	ctx context.Context,
	podUid, workloadUID, msg string,
	logTime time.Time,
	frameworkName string,
) error {
	matcher, ok := patternMatchers[frameworkName]
	if !ok {
		logrus.Debugf("No pattern matcher for framework: %s, using legacy logic", frameworkName)
		return singleWorkloadLog(ctx, podUid, msg, logTime)
	}
	
	// Try performance pattern
	if result := matcher.MatchPerformance(msg); result.Matched {
		return handlePerformanceLog(ctx, workloadUID, podUid, result.Groups, logTime)
	}
	
	// Try training events
	if result := matcher.MatchTrainingEvent(msg, "start_training"); result.Matched {
		return handleTrainingEvent(ctx, workloadUID, podUid, "StartTrain", logTime)
	}
	if result := matcher.MatchTrainingEvent(msg, "end_training"); result.Matched {
		return handleTrainingEvent(ctx, workloadUID, podUid, "EndTrain", logTime)
	}
	
	// Try checkpoint events
	if result := matcher.MatchCheckpointEvent(msg, "start_saving"); result.Matched {
		return handleCheckpointEvent(ctx, workloadUID, podUid, "start_saving", result.Groups, logTime)
	}
	if result := matcher.MatchCheckpointEvent(msg, "end_saving"); result.Matched {
		return handleCheckpointEvent(ctx, workloadUID, podUid, "end_saving", result.Groups, logTime)
	}
	if result := matcher.MatchCheckpointEvent(msg, "loading"); result.Matched {
		return handleCheckpointEvent(ctx, workloadUID, podUid, "loading", result.Groups, logTime)
	}
	
	// No pattern matched, try legacy logic
	return singleWorkloadLog(ctx, podUid, msg, logTime)
}

// handlePerformanceLog handles performance log extraction
func handlePerformanceLog(
	ctx context.Context,
	workloadUID, podUid string,
	groups map[string]string,
	logTime time.Time,
) error {
	// Convert groups to TrainingPerformance model
	// For now, delegate to existing logic
	// TODO: Use groups directly to construct performance data
	_, err := saveTrainingPerformance(ctx, formatGroupsToLogLine(groups), podUid, logTime)
	return err
}

// handleTrainingEvent handles training lifecycle events
func handleTrainingEvent(
	ctx context.Context,
	workloadUID, podUid, eventType string,
	logTime time.Time,
) error {
	// Delegate to existing logic
	if eventType == "StartTrain" {
		_, err := saveStartTrain(ctx, "training ...", podUid, logTime)
		return err
	}
	return nil
}

// formatGroupsToLogLine formats extracted groups back to log line format
// This is a helper for backward compatibility with legacy parsing
func formatGroupsToLogLine(groups map[string]string) string {
	// For now, return empty to trigger fallback
	return ""
}

// truncateLog truncates log line to specified length
func truncateLog(log string, maxLen int) string {
	if len(log) <= maxLen {
		return log
	}
	return log[:maxLen] + "..."
}
