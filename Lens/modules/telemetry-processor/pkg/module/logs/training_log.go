package logs

import (
	"context"
	"fmt"
	"reflect"
	"strconv"
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
	tpapi "github.com/AMD-AGI/Primus-SaFE/Lens/telemetry-processor/pkg/api"
	"github.com/AMD-AGI/Primus-SaFE/Lens/telemetry-processor/pkg/module/pods"
)

var (
	// Global singletons (initialized at startup)
	detectionManager *framework.FrameworkDetectionManager
	configManager    *FrameworkConfigManager
	patternMatchers  map[string]*PatternMatcher
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

	// Get workload information from cache
	workloadRefs := pods.GetWorkloadsByPodUid(podUid)
	if len(workloadRefs) == 0 {
		return nil
	}

	// Framework detection must be initialized
	if detectionManager == nil || len(patternMatchers) == 0 {
		logrus.Errorf("Framework detection not initialized - cannot process logs for pod %s", podUid)
		return fmt.Errorf("framework detection not initialized")
	}

	// workloadRefs is [][]string, each element is []string{workloadName, workloadUID}
	// Since detection manager automatically handles workload hierarchy,
	// we only need to detect framework once (using first workload)
	firstWorkloadUID := workloadRefs[0][1]

	// Get or detect framework (only once, shared by all workloads in hierarchy)
	detection, err := detectionManager.GetDetection(ctx, firstWorkloadUID)
	if err != nil {
		logrus.Warnf("Failed to get detection for %s: %v", firstWorkloadUID, err)
	}

	// Determine framework to use
	var frameworkName string
	needsDetection := false

	if detection != nil && detection.Confidence >= 0.5 {
		// Use existing detection (shared across workload hierarchy)
		frameworkName = detection.Framework
		logrus.Debugf("Using existing framework detection: %s (confidence: %.2f)",
			frameworkName, detection.Confidence)
	} else {
		// Need to detect framework from log (only once)
		frameworkName, err = detectFrameworkFromLog(ctx, firstWorkloadUID, msg)
		if err != nil {
			logrus.Debugf("Framework not detected from log, using default")
			frameworkName = "primus" // Default
		}
		needsDetection = true
	}

	// Report detection if needed (only once, automatically propagates to root)
	if needsDetection && frameworkName != "" {
		confidence := calculateDetectionConfidence(frameworkName, msg)

		err = detectionManager.ReportDetection(
			ctx,
			firstWorkloadUID,
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
			logrus.Infof("Reported log detection: workload=%s, framework=%s, confidence=%.2f (shared with hierarchy)",
				firstWorkloadUID, frameworkName, confidence)
		}
	}

	// Process log with framework-specific parser (only once)
	// The processing functions will internally handle all workloads associated with this pod
	if err := processLogWithFramework(ctx, podUid, firstWorkloadUID, msg, logTime, frameworkName); err != nil {
		logrus.Errorf("Failed to process log with framework: %v", err)
	}

	log.Tracef("workload log consume success")
	return nil
}

// groupsToPerformance converts regex captured groups to TrainingPerformance model using reflection
// This dynamically maps group names to struct fields, eliminating hardcoded field mappings
func groupsToPerformance(groups map[string]string) *model.TrainingPerformance {
	perf := &model.TrainingPerformance{}

	// Use reflection to dynamically set fields based on group names
	perfValue := reflect.ValueOf(perf).Elem()
	perfType := perfValue.Type()

	// Iterate through all fields in TrainingPerformance struct
	for i := 0; i < perfValue.NumField(); i++ {
		field := perfValue.Field(i)
		fieldType := perfType.Field(i)
		fieldName := fieldType.Name

		// Skip unexported fields
		if !field.CanSet() {
			continue
		}

		// Look for matching group (case-sensitive match with field name)
		groupValue, exists := groups[fieldName]
		if !exists {
			// Try alternative names for backward compatibility
			groupValue = tryAlternativeNames(groups, fieldName)
			if groupValue == "" {
				continue
			}
		}

		// Set field value based on its type
		if err := setFieldValue(field, groupValue); err != nil {
			logrus.Warnf("Failed to set field %s with value %s: %v", fieldName, groupValue, err)
		}
	}

	return perf
}

// tryAlternativeNames tries alternative group names for common field name variations
func tryAlternativeNames(groups map[string]string, fieldName string) string {
	// Map field names to alternative group names
	alternatives := map[string][]string{
		"MemUsages": {"MemUsage"}, // MemUsages can also come from MemUsage group
	}

	if altNames, ok := alternatives[fieldName]; ok {
		for _, altName := range altNames {
			if val, exists := groups[altName]; exists && val != "" {
				return val
			}
		}
	}

	return ""
}

// setFieldValue sets a reflect.Value based on string input and field type
func setFieldValue(field reflect.Value, value string) error {
	if value == "" {
		return nil // Skip empty values
	}

	switch field.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		intVal, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return fmt.Errorf("failed to parse int: %w", err)
		}
		field.SetInt(intVal)

	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		uintVal, err := strconv.ParseUint(value, 10, 64)
		if err != nil {
			return fmt.Errorf("failed to parse uint: %w", err)
		}
		field.SetUint(uintVal)

	case reflect.Float32, reflect.Float64:
		floatVal, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return fmt.Errorf("failed to parse float: %w", err)
		}
		field.SetFloat(floatVal)

	case reflect.String:
		field.SetString(value)

	case reflect.Bool:
		boolVal, err := strconv.ParseBool(value)
		if err != nil {
			return fmt.Errorf("failed to parse bool: %w", err)
		}
		field.SetBool(boolVal)

	default:
		return fmt.Errorf("unsupported field type: %s", field.Kind())
	}

	return nil
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

func saveStartTrain(ctx context.Context, msg, podId string, docTime time.Time) (bool, error) {
	if !strings.Contains(strings.TrimSpace(msg), "training ...") {
		return false, nil
	}

	// Get nearest workload (still from DB as it's complex logic)
	nearestWorkloadUid := ""
	nearestWorkload, err := database.GetFacade().GetWorkload().GetNearestWorkloadByPodUid(ctx, podId)
	if err != nil {
		return false, err
	}
	if nearestWorkload != nil {
		nearestWorkloadUid = nearestWorkload.UID
	}

	// Get workload references from cache instead of database
	workloadRefs := pods.GetWorkloadsByPodUid(podId)
	if workloadRefs == nil || len(workloadRefs) == 0 {
		return true, nil
	}

	for _, workloadRef := range workloadRefs {
		if len(workloadRef) < 2 {
			continue
		}
		workloadUID := workloadRef[1]
		err = saveStartTrainForSingleWorkload(ctx, podId, workloadUID, nearestWorkloadUid, docTime)
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
		logrus.Warnf("No pattern matcher for framework: %s - skipping log processing", frameworkName)
		return fmt.Errorf("no pattern matcher for framework: %s", frameworkName)
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

	// No pattern matched - this is normal for most logs
	logrus.Tracef("No pattern matched for log from pod %s, workload %s", podUid, workloadUID)
	return nil
}

// handlePerformanceLog handles performance log extraction
func handlePerformanceLog(
	ctx context.Context,
	workloadUID, podUid string,
	groups map[string]string,
	logTime time.Time,
) error {
	// Convert groups (extracted from regex) to TrainingPerformance model
	performance := groupsToPerformance(groups)
	if performance == nil {
		return fmt.Errorf("failed to convert groups to performance data")
	}

	// Get nearest workload for serial calculation
	nearestWorkloadUid := ""
	nearestWorkload, err := database.GetFacade().GetWorkload().GetNearestWorkloadByPodUid(ctx, podUid)
	if err != nil {
		return err
	}
	if nearestWorkload != nil {
		nearestWorkloadUid = nearestWorkload.UID
	}

	// Get workload references from cache
	workloadRefs := pods.GetWorkloadsByPodUid(podUid)
	if workloadRefs == nil || len(workloadRefs) == 0 {
		log.Tracef("no workload references found in cache for pod %s", podUid)
		return nil
	}

	// Save performance data for each workload
	for _, workloadRef := range workloadRefs {
		if len(workloadRef) < 2 {
			continue
		}
		wUID := workloadRef[1]
		err = saveTrainingPerformanceForSingleWorkload(ctx, podUid, wUID, nearestWorkloadUid, performance, logTime)
		if err != nil {
			log.Errorf("saveTrainingPerformanceForSingleWorkload err %+v", err)
		} else {
			log.Tracef("save training performance for workload %s", wUID)
		}
	}

	return nil
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

// truncateLog truncates log line to specified length
func truncateLog(log string, maxLen int) string {
	if len(log) <= maxLen {
		return log
	}
	return log[:maxLen] + "..."
}
