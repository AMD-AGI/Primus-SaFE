package logs

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/sirupsen/logrus"

	advisorClient "github.com/AMD-AGI/Primus-SaFE/Lens/ai-advisor/pkg/client"
	advisorCommon "github.com/AMD-AGI/Primus-SaFE/Lens/ai-advisor/pkg/common"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/constant"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	dbModel "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/helper/config"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/utils/mapUtil"
	"github.com/AMD-AGI/Primus-SaFE/Lens/telemetry-processor/pkg/module/pods"
)

var (
	// Global singletons (initialized at startup)
	aiAdvisorClient *advisorClient.Client      // AI Advisor client for framework detection
	configManager   *FrameworkConfigManager    // For local log pattern matching
	patternMatchers map[string]*PatternMatcher // For local log parsing

	// ANSI escape code regex for cleaning logs
	// Matches: \x1b[...m (standard ANSI), [...m (simplified format), [...][ (any bracket sequences)
	ansiEscapeRegex = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]|\[[0-9;]*m|\[[\d;]+\w`)
)

// InitializeWandBHandlerAndLogProcessing initializes WandB handler with AI Advisor client
// and local log processing components
func InitializeWandBHandlerAndLogProcessing(aiAdvisorURL string, systemConfigMgr *config.Manager) error {
	// 1. Create AI Advisor client
	if aiAdvisorURL == "" {
		aiAdvisorURL = os.Getenv("AI_ADVISOR_URL")
		if aiAdvisorURL == "" {
			aiAdvisorURL = "http://ai-advisor:8080" // Default
		}
	}

	aiAdvisorClient = advisorClient.NewClientWithDefaults(aiAdvisorURL).
		SetTimeout(30*time.Second).
		SetRetry(3, 1*time.Second)

	logrus.Infof("AI Advisor client initialized: %s", aiAdvisorURL)

	// 2. Initialize config manager (for local log pattern matching)
	configManager = NewFrameworkConfigManager(systemConfigMgr)

	// Load all framework configurations
	ctx := context.Background()
	if err := configManager.LoadAllFrameworks(ctx); err != nil {
		logrus.Warnf("Failed to load some framework configs: %v", err)
	}

	// 3. Initialize pattern matchers (for local log parsing)
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

	logrus.Infof("Framework pattern matchers initialized with %d frameworks", len(patternMatchers))

	// 4. Initialize WandB log/metrics processor (local processing)
	metricsStorage := NewInMemoryMetricsStorage(10000) // Max 10000 metrics per workload
	wandbLogProcessor := NewWandBLogProcessor(metricsStorage)

	// 5. Initialize WandB Handler (with AI Advisor client)
	InitWandBHandlerWithClient(aiAdvisorClient, wandbLogProcessor)
	logrus.Info("WandB handler initialized successfully")

	return nil
}

// GetAIAdvisorClient returns the AI Advisor client
func GetAIAdvisorClient() *advisorClient.Client {
	return aiAdvisorClient
}

// stripAnsiCodes removes ANSI escape codes from log messages
// This handles color codes like [[32m, [0m, etc. that appear in terminal output
func stripAnsiCodes(msg string) string {
	return ansiEscapeRegex.ReplaceAllString(msg, "")
}

func WorkloadLog(ctx context.Context, podUid string, msg string, logTime time.Time) error {
	log.Tracef("before consume workload log , pod uid %s", podUid)

	// Clean ANSI escape codes from log message (color codes, etc.)
	// This ensures pattern matching works correctly for logs with terminal formatting
	cleanMsg := stripAnsiCodes(msg)

	// Get workload information from cache
	workloadRefs := pods.GetWorkloadsByPodUid(podUid)
	if len(workloadRefs) == 0 {
		return nil
	}

	// Check if pattern matchers are initialized (for local log parsing)
	if len(patternMatchers) == 0 {
		logrus.Tracef("Pattern matchers not initialized - skipping framework-specific log processing for pod %s", podUid)
		// Continue processing without framework detection
	}

	// workloadRefs is [][]string, each element is []string{workloadName, workloadUID}
	// Since detection manager automatically handles workload hierarchy,
	// we only need to detect framework once (using first workload)
	firstWorkloadUID := workloadRefs[0][1]

	// Get or detect framework
	var frameworkName string
	needsDetection := false

	// Try to query existing detection from ai-advisor
	if aiAdvisorClient != nil {
		detection, err := aiAdvisorClient.GetDetection(firstWorkloadUID)
		if err != nil {
			logrus.Debugf("Failed to query detection from AI Advisor: %v", err)
		} else if detection != nil && detection.Confidence >= 0.5 {
			// Use existing detection (shared across workload hierarchy)
			// Priority: WrapperFramework > BaseFramework > Frameworks[0]
			// This prevents issues if Frameworks array order changes
			if detection.WrapperFramework != "" {
				frameworkName = detection.WrapperFramework
				IncFrameworkUsageCount(frameworkName, "wrapper")
			} else if detection.BaseFramework != "" {
				frameworkName = detection.BaseFramework
				IncFrameworkUsageCount(frameworkName, "base")
			} else if len(detection.Frameworks) > 0 {
				frameworkName = detection.Frameworks[0]
				IncFrameworkUsageCount(frameworkName, "primary")
			}

		}
	}

	// If no framework detected yet, try to detect from log
	if frameworkName == "" && len(patternMatchers) > 0 {
		detectedFramework, err := detectFrameworkFromLog(ctx, firstWorkloadUID, cleanMsg)
		if err != nil {
			// Framework not detected from this log - this is OK
			logrus.Tracef("Framework not detected from log: %v - skipping framework-specific processing", err)
			// No framework detected, keep frameworkName empty
		} else {
			// Successfully detected framework from log
			frameworkName = detectedFramework
			needsDetection = true
		}
	}

	// Report detection to AI Advisor if we detected something new
	if needsDetection && frameworkName != "" && aiAdvisorClient != nil {
		confidence := calculateDetectionConfidence(frameworkName, cleanMsg)

		// Report to AI Advisor
		_, err := aiAdvisorClient.ReportDetection(&advisorCommon.DetectionRequest{
			WorkloadUID: firstWorkloadUID,
			Source:      "log",
			Frameworks:  []string{frameworkName},
			Type:        "training",
			Confidence:  confidence,
			Evidence: map[string]interface{}{
				"method":     "log_pattern_match",
				"sample_log": truncateLog(cleanMsg, 200),
			},
		})
		if err != nil {
			logrus.Errorf("Failed to report log detection to AI Advisor: %v", err)
			IncFrameworkDetectionErrors("log", "report_failed")
		} else {
			logrus.Infof("✓ Reported log detection to AI Advisor: workload=%s, framework=%s, confidence=%.2f",
				firstWorkloadUID, frameworkName, confidence)
			// Record detection metrics
			IncFrameworkDetectionCount(frameworkName, "log_pattern", "log")
			ObserveFrameworkDetectionConfidence(frameworkName, "log_pattern", confidence)
		}
	}

	// Process log with framework-specific parser (only once)
	// Skip if framework is unknown - we don't want to force processing with a default framework
	if frameworkName != "" {
		if err := processLogWithFramework(ctx, podUid, firstWorkloadUID, cleanMsg, logTime, frameworkName); err != nil {
			logrus.Debugf("Failed to process log with framework %s: %v", frameworkName, err)
		}
	} else {
		logrus.Tracef("Skipping framework-specific log processing - framework not yet determined")
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
// Supports both direct values and pointer types
func setFieldValue(field reflect.Value, value string) error {
	if value == "" {
		return nil // Skip empty values
	}

	// Handle pointer types
	if field.Kind() == reflect.Ptr {
		// Create a new pointer if nil
		if field.IsNil() {
			field.Set(reflect.New(field.Type().Elem()))
		}
		// Recursively set the pointed-to value
		return setFieldValue(field.Elem(), value)
	}

	// Handle direct value types
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
	
	// Get current iteration value (handle pointer)
	currentIteration := 0
	if perf.CurrentIteration != nil {
		currentIteration = *perf.CurrentIteration
	}
	
	existDbPerformance, err := database.GetFacade().GetTraining().GetTrainingPerformanceByWorkloadIdSerialAndIteration(ctx, workloadId, serial, currentIteration)
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
		Iteration:   int32(currentIteration),
		CreatedAt:   docTime,
		Serial:      int32(serial),
		WorkloadUID: workloadId,
		DataSource:  constant.DataSourceLog, // Data parsed from application logs
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
	if len(workloadRefs) == 0 {
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
		// Record pattern match success (pattern name not available in this context)
		IncLogPatternMatchCount(bestMatch, "identify", "")
		return bestMatch, nil
	}

	// Record detection failure
	IncFrameworkDetectionErrors("log", "no_match")
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
		IncLogPatternMatchCount(frameworkName, "performance", result.Pattern)
		err := handlePerformanceLog(ctx, workloadUID, podUid, result.Groups, logTime, frameworkName)
		if err != nil {
			IncLogPatternMatchErrors(frameworkName, "performance", "processing_failed")
		}
		return err
	}

	// Try training events
	if result := matcher.MatchTrainingEvent(msg, "start_training"); result.Matched {
		IncLogPatternMatchCount(frameworkName, "training_event", result.Pattern)
		return handleTrainingEvent(ctx, workloadUID, podUid, "StartTrain", logTime)
	}
	if result := matcher.MatchTrainingEvent(msg, "end_training"); result.Matched {
		IncLogPatternMatchCount(frameworkName, "training_event", result.Pattern)
		return handleTrainingEvent(ctx, workloadUID, podUid, "EndTrain", logTime)
	}

	// Try checkpoint events
	if result := matcher.MatchCheckpointEvent(msg, "start_saving"); result.Matched {
		IncLogPatternMatchCount(frameworkName, "checkpoint_event", result.Pattern)
		IncCheckpointEventCount("start_saving", frameworkName)
		err := handleCheckpointEvent(ctx, workloadUID, podUid, "start_saving", result.Groups, logTime)
		if err != nil {
			IncCheckpointEventErrors("start_saving", frameworkName, "processing_failed")
		}
		return err
	}
	if result := matcher.MatchCheckpointEvent(msg, "end_saving"); result.Matched {
		IncLogPatternMatchCount(frameworkName, "checkpoint_event", result.Pattern)
		IncCheckpointEventCount("end_saving", frameworkName)
		err := handleCheckpointEvent(ctx, workloadUID, podUid, "end_saving", result.Groups, logTime)
		if err != nil {
			IncCheckpointEventErrors("end_saving", frameworkName, "processing_failed")
		}
		return err
	}
	if result := matcher.MatchCheckpointEvent(msg, "loading"); result.Matched {
		IncLogPatternMatchCount(frameworkName, "checkpoint_event", result.Pattern)
		IncCheckpointEventCount("loading", frameworkName)
		err := handleCheckpointEvent(ctx, workloadUID, podUid, "loading", result.Groups, logTime)
		if err != nil {
			IncCheckpointEventErrors("loading", frameworkName, "processing_failed")
		}
		return err
	}

	// No pattern matched - this is normal for most logs
	logrus.Tracef("No pattern matched for log from pod %s, workload %s", podUid, workloadUID)
	return nil
}

// ConvertGroupsToPerformance converts regex groups to TrainingPerformance (pure function for testing)
// This is a public wrapper around groupsToPerformance for use in tests and debug APIs
func ConvertGroupsToPerformance(groups map[string]string) (*model.TrainingPerformance, error) {
	if len(groups) == 0 {
		return nil, fmt.Errorf("no groups provided")
	}

	performance := groupsToPerformance(groups)
	if performance == nil {
		return nil, fmt.Errorf("failed to convert groups to performance data")
	}

	return performance, nil
}

// handlePerformanceLog handles performance log extraction
func handlePerformanceLog(
	ctx context.Context,
	workloadUID, podUid string,
	groups map[string]string,
	logTime time.Time,
	frameworkName string,
) error {
	// Convert groups (extracted from regex) to TrainingPerformance model
	performance, err := ConvertGroupsToPerformance(groups)
	if err != nil {
		return err
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
	if len(workloadRefs) == 0 {
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
			IncTrainingPerformanceSaveErrors(wUID, "log", "db_error")
		} else {
			log.Tracef("save training performance for workload %s", wUID)
			IncTrainingPerformanceSaveCount(wUID, "log")
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

// PatternMatcherInfo represents debug information for a pattern matcher
type PatternMatcherInfo struct {
	Framework           string              `json:"framework"`
	IdentifyPatterns    []PatternDebugInfo  `json:"identify_patterns"`
	PerformancePatterns []PatternDebugInfo  `json:"performance_patterns"`
	TrainingEvents      map[string][]string `json:"training_events"`
	CheckpointEvents    map[string][]string `json:"checkpoint_events"`
	TotalPatterns       int                 `json:"total_patterns"`
}

// PatternDebugInfo represents debug info for a single pattern
type PatternDebugInfo struct {
	Name       string   `json:"name"`
	Pattern    string   `json:"pattern"`
	Confidence float64  `json:"confidence"`
	Tags       []string `json:"tags,omitempty"`
}

// GetPatternMatchersInfo returns debug information about all pattern matchers
func GetPatternMatchersInfo() map[string]*PatternMatcherInfo {
	result := make(map[string]*PatternMatcherInfo)

	for frameworkName, matcher := range patternMatchers {
		info := &PatternMatcherInfo{
			Framework:        frameworkName,
			TrainingEvents:   make(map[string][]string),
			CheckpointEvents: make(map[string][]string),
		}

		// Get identify patterns
		matcher.mu.RLock()
		for _, cp := range matcher.identifyRegexps {
			info.IdentifyPatterns = append(info.IdentifyPatterns, PatternDebugInfo{
				Name:       cp.Name,
				Pattern:    cp.Pattern.String(),
				Confidence: cp.Confidence,
				Tags:       cp.Tags,
			})
			info.TotalPatterns++
		}

		// Get performance patterns
		for _, cp := range matcher.performanceRegexps {
			info.PerformancePatterns = append(info.PerformancePatterns, PatternDebugInfo{
				Name:       cp.Name,
				Pattern:    cp.Pattern.String(),
				Confidence: cp.Confidence,
				Tags:       cp.Tags,
			})
			info.TotalPatterns++
		}

		// Get training event patterns
		for eventType, patterns := range matcher.trainingEventRegexps {
			patternNames := make([]string, 0, len(patterns))
			for _, cp := range patterns {
				patternNames = append(patternNames, cp.Name)
				info.TotalPatterns++
			}
			info.TrainingEvents[eventType] = patternNames
		}

		// Get checkpoint event patterns
		for eventType, patterns := range matcher.checkpointRegexps {
			patternNames := make([]string, 0, len(patterns))
			for _, cp := range patterns {
				patternNames = append(patternNames, cp.Name)
				info.TotalPatterns++
			}
			info.CheckpointEvents[eventType] = patternNames
		}
		matcher.mu.RUnlock()

		result[frameworkName] = info
	}

	return result
}

// GetFrameworkList returns the list of available frameworks
func GetFrameworkList() []string {
	frameworks := make([]string, 0, len(patternMatchers))
	for frameworkName := range patternMatchers {
		frameworks = append(frameworks, frameworkName)
	}
	return frameworks
}
