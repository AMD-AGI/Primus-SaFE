// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

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
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/ahocorasick"
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
	aiAdvisorClient *advisorClient.Client  // AI Advisor client (used by WandB handler, etc.)
	globalExtractor *UniversalExtractor    // Framework-agnostic AC automaton extractor
	globalRegistry  *GlobalPatternRegistry // Global pattern registry (loads from training_log_pattern table)

	// ANSI escape code regex for cleaning logs
	// Matches: \x1b[...X (standard ANSI), [...m (simplified color), [...X (control sequences ending in letter)
	// The third pattern requires a letter suffix to avoid stripping legitimate bracket-number like [9074/...]
	ansiEscapeRegex = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]|\[[0-9;]*m|\[[\d;]+[a-zA-Z]`)
)

// InitializeWandBHandlerAndLogProcessing initializes WandB handler with AI Advisor client
// and local log processing components
func InitializeWandBHandlerAndLogProcessing(aiAdvisorURL string, _ *config.Manager) error {
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

	// 2. Initialize GlobalPatternRegistry (loads from training_log_pattern table)
	ctx := context.Background()
	globalRegistry = NewGlobalPatternRegistry()
	if err := globalRegistry.Load(ctx); err != nil {
		logrus.Warnf("Failed to load global pattern registry: %v (will retry in background)", err)
	}
	go globalRegistry.StartAutoReload(context.Background())
	logrus.Infof("Global pattern registry initialized with %d patterns", globalRegistry.PatternCount())

	// 3. Initialize universal extractor (AC automaton for intent analysis)
	globalExtractor = NewUniversalExtractor()
	// Patterns will be loaded periodically from the DB by a background goroutine
	go startExtractorPatternRefresh(context.Background())
	logrus.Info("Universal extractor initialized (patterns loaded in background)")

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

// GetUniversalExtractor returns the global universal extractor instance
func GetUniversalExtractor() *UniversalExtractor {
	return globalExtractor
}

// startExtractorPatternRefresh periodically loads promoted intent rules from DB
func startExtractorPatternRefresh(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	// Initial load after short delay
	time.Sleep(10 * time.Second)
	refreshExtractorPatterns(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			refreshExtractorPatterns(ctx)
		}
	}
}

// refreshExtractorPatterns loads promoted rules from the intent_rule table
func refreshExtractorPatterns(ctx context.Context) {
	if globalExtractor == nil {
		return
	}

	ruleFacade := database.NewIntentRuleFacade()
	rules, err := ruleFacade.GetPromotedRules(ctx)
	if err != nil {
		logrus.Warnf("Failed to load promoted rules for extractor: %v", err)
		return
	}

	if len(rules) == 0 {
		return
	}

	var patterns []*ahocorasick.Pattern
	for _, rule := range rules {
		// Only use literal patterns (not regex) for AC automaton
		// Regex rules are handled by the evaluator, not the AC automaton
		if strings.ContainsAny(rule.Pattern, `.*+?[](){}^$|\`) {
			continue
		}
		patterns = append(patterns, &ahocorasick.Pattern{
			Keyword:    rule.Pattern,
			ID:         rule.ID,
			Field:      rule.DetectsField,
			Value:      rule.DetectsValue,
			Confidence: rule.Confidence,
		})
	}

	if len(patterns) > 0 {
		globalExtractor.LoadPatterns(patterns)
		logrus.Infof("Extractor patterns refreshed: %d literal patterns from %d rules", len(patterns), len(rules))
	}
}

// stripAnsiCodes removes ANSI escape codes from log messages
// This handles color codes like [[32m, [0m, etc. that appear in terminal output
func stripAnsiCodes(msg string) string {
	return ansiEscapeRegex.ReplaceAllString(msg, "")
}

func WorkloadLog(ctx context.Context, podUid string, msg string, logTime time.Time) error {
	log.Tracef("before consume workload log , pod uid %s", podUid)

	// Clean ANSI escape codes from log message (color codes, etc.)
	cleanMsg := stripAnsiCodes(msg)

	// Get workload information from cache
	workloadRefs := pods.GetWorkloadsByPodUid(podUid)
	if len(workloadRefs) == 0 {
		return nil
	}

	firstWorkloadUID := workloadRefs[0][1]

	// Global pattern matching: all enabled patterns from training_log_pattern table
	// are tried against every log line, regardless of framework.
	if globalRegistry != nil {
		if err := processLogWithGlobalRegistry(ctx, podUid, firstWorkloadUID, cleanMsg, logTime); err != nil {
			// No pattern matched - this is normal for most log lines
			logrus.Tracef("No global pattern matched for pod %s: %v", podUid, err)
		}
	}

	log.Tracef("workload log consume success")
	return nil
}

// processLogWithGlobalRegistry processes a log line using the global pattern registry.
// Returns nil if a pattern matched (even if processing had errors), or an error to signal fallback.
func processLogWithGlobalRegistry(
	ctx context.Context,
	podUid, workloadUID, msg string,
	logTime time.Time,
) error {
	// Skip blacklisted lines early
	if globalRegistry.IsBlacklisted(msg) {
		return nil
	}

	// Try performance patterns (most common match)
	if result := globalRegistry.MatchPerformance(msg); result.Matched {
		fw := result.Framework
		if fw == "" {
			fw = "global"
		}
		IncLogPatternMatchCount(fw, "performance", result.Pattern)
		err := handlePerformanceLog(ctx, workloadUID, podUid, result.Groups, logTime, fw)
		if err != nil {
			IncLogPatternMatchErrors(fw, "performance", "processing_failed")
		}
		return nil // matched, do not fall back
	}

	// Try training events
	if subtype, result := globalRegistry.MatchTrainingEvent(msg); result.Matched {
		fw := result.Framework
		if fw == "" {
			fw = "global"
		}
		IncLogPatternMatchCount(fw, "training_event", result.Pattern)
		eventType := mapEventSubtype(subtype)
		if eventType != "" {
			_ = handleTrainingEvent(ctx, workloadUID, podUid, eventType, logTime)
		}
		return nil
	}

	// Try checkpoint events
	if subtype, result := globalRegistry.MatchCheckpointEvent(msg); result.Matched {
		fw := result.Framework
		if fw == "" {
			fw = "global"
		}
		IncLogPatternMatchCount(fw, "checkpoint_event", result.Pattern)
		IncCheckpointEventCount(subtype, fw)
		err := handleCheckpointEvent(ctx, workloadUID, podUid, subtype, result.Groups, logTime)
		if err != nil {
			IncCheckpointEventErrors(subtype, fw, "processing_failed")
		}
		return nil
	}

	// No match in global registry - return error to allow legacy fallback
	return fmt.Errorf("no global pattern matched")
}

// mapEventSubtype maps event_subtype string to the internal event type
func mapEventSubtype(subtype string) string {
	switch subtype {
	case "start_training":
		return "StartTrain"
	case "end_training":
		return "EndTrain"
	default:
		return ""
	}
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
		// Record reason for skipping: duplicate record already exists
		logrus.Debugf("Training performance already exists for workload=%s, serial=%d, iteration=%d - skipping insert",
			workloadId, serial, currentIteration)
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

	// Record data about to be inserted
	logrus.Debugf("Inserting training performance: workload=%s, serial=%d, iteration=%d, pod=%s, time=%s",
		workloadId, serial, currentIteration, podId, docTime.Format(time.RFC3339))

	err = database.GetFacade().GetTraining().CreateTrainingPerformance(ctx, existDbPerformance)
	if err != nil {
		logrus.Errorf("Failed to insert training performance: workload=%s, serial=%d, iteration=%d, error=%v",
			workloadId, serial, currentIteration, err)
		return err
	}

	logrus.Infof("✓ Successfully inserted training performance: workload=%s, serial=%d, iteration=%d",
		workloadId, serial, currentIteration)
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
		logrus.Debugf("Processing performance data for workload=%s, pod=%s, iteration=%v",
			wUID, podUid, performance.CurrentIteration)

		err = saveTrainingPerformanceForSingleWorkload(ctx, podUid, wUID, nearestWorkloadUid, performance, logTime)
		if err != nil {
			log.Errorf("saveTrainingPerformanceForSingleWorkload failed: workload=%s, error=%+v", wUID, err)
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

// GetGlobalRegistryDebugInfo returns debug info about the global pattern registry
func GetGlobalRegistryDebugInfo() map[string]interface{} {
	if globalRegistry == nil {
		return map[string]interface{}{
			"status": "not_initialized",
		}
	}
	return globalRegistry.GetDebugInfo()
}
