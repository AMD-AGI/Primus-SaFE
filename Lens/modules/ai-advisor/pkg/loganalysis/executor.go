// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package loganalysis

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/constant"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	coreTask "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/task"
	"github.com/opensearch-project/opensearch-go/opensearchapi"
)

const (
	// Check interval between executions (enforced via ext field)
	defaultCheckInterval = 2 * time.Minute

	// Maximum log lines to retrieve from OpenSearch per pod per execution
	maxLogLines = 3000

	// Max unmatched samples to keep per workload
	maxUnmatchedSamples = 50

	// Config key prefix for auto-discovered patterns
	configKeyPrefix = "training.log.parser.autodiscovered."

	// OpenSearch index patterns (FluentBit writes with Logstash_Format)
	// Bootstrap uses "node-*", chart uses "primus-lens-*"
	opensearchIndexPattern = "node-*,primus-lens-*"
)

// trainingKeyword is a keyword-regex pair for detecting training metric lines
type trainingKeyword struct {
	name    string
	pattern *regexp.Regexp
}

// unmatchedLine represents a log line that contains training keywords
// but was not captured by telemetry-processor
type unmatchedLine struct {
	Line      string   `json:"line"`
	Keywords  []string `json:"keywords"`
	Timestamp string   `json:"timestamp"`
}

// LogAnalysisExecutor implements a periodic log scanner that detects
// training metric lines not captured by telemetry-processor's pattern matcher.
//
// For each training workload it:
// 1. Reads recent pod logs from OpenSearch (where FluentBit stores them)
// 2. Scans for lines containing training-metric keywords (epoch, step, loss, lr, etc.)
// 3. Checks if training_performance records exist for the workload
// 4. If keyword lines exist but metrics are not captured, classifies them as:
//   - Performance lines -> generates a new regex pattern proposal
//   - Non-performance lines -> generates a blacklist regex
//
// 5. Stores proposals in system_config for review or auto-application
type LogAnalysisExecutor struct {
	coreTask.BaseExecutor

	trainingFacade  database.TrainingFacadeInterface
	workloadFacade  database.WorkloadFacadeInterface
	sysConfigFacade database.SystemConfigFacadeInterface
	detectionFacade database.WorkloadDetectionFacadeInterface

	// Pre-compiled keyword patterns
	keywords []trainingKeyword
}

// NewLogAnalysisExecutor creates a new log analysis executor
func NewLogAnalysisExecutor() *LogAnalysisExecutor {
	return &LogAnalysisExecutor{
		trainingFacade:  database.NewTrainingFacade(),
		workloadFacade:  database.NewWorkloadFacade(),
		sysConfigFacade: database.NewSystemConfigFacade(),
		detectionFacade: database.NewWorkloadDetectionFacade(),
		keywords:        initKeywords(),
	}
}

// GetTaskType returns the task type this executor handles
func (e *LogAnalysisExecutor) GetTaskType() string {
	return constant.TaskTypeLogAnalysis
}

// Validate validates task parameters
func (e *LogAnalysisExecutor) Validate(task *model.WorkloadTaskState) error {
	if task.WorkloadUID == "" {
		return fmt.Errorf("workload_uid is required")
	}
	return nil
}

// Cancel cancels the task
func (e *LogAnalysisExecutor) Cancel(_ context.Context, _ *model.WorkloadTaskState) error {
	return nil
}

// Execute performs one cycle of log analysis
func (e *LogAnalysisExecutor) Execute(
	ctx context.Context,
	execCtx *coreTask.ExecutionContext,
) (*coreTask.ExecutionResult, error) {
	task := execCtx.Task
	workloadUID := task.WorkloadUID
	updates := make(map[string]interface{})

	// Throttle: only run every defaultCheckInterval
	lastCheck := e.GetExtString(task, "last_check_at")
	if lastCheck != "" {
		if t, err := time.Parse(time.RFC3339, lastCheck); err == nil {
			if time.Since(t) < defaultCheckInterval {
				return coreTask.RescheduleResult(nil), nil
			}
		}
	}
	updates["last_check_at"] = time.Now().Format(time.RFC3339)

	// Check if workload is still running; if terminated, complete the task
	if e.isWorkloadTerminated(ctx, workloadUID) {
		log.Infof("LogAnalysis: workload %s terminated, completing task", workloadUID)
		updates["completed_reason"] = "workload_terminated"
		return coreTask.SuccessResult(updates), nil
	}

	// Get workload detection to know the framework
	det, err := e.detectionFacade.GetDetection(ctx, workloadUID)
	if err != nil || det == nil {
		log.Debugf("LogAnalysis: no detection record for workload %s, rescheduling", workloadUID)
		updates["skip_reason"] = "no_detection_record"
		return coreTask.RescheduleResult(updates), nil
	}

	// Check if framework is known
	framework := det.Framework
	if framework == "" {
		log.Debugf("LogAnalysis: framework unknown for workload %s, rescheduling", workloadUID)
		updates["skip_reason"] = "framework_unknown"
		return coreTask.RescheduleResult(updates), nil
	}

	// Step 1: Check if training_performance records exist recently
	since := time.Now().Add(-10 * time.Minute)
	perfRecords, err := e.trainingFacade.ListWorkloadPerformanceByWorkloadIdAndTimeRange(
		ctx, workloadUID, since, time.Now(),
	)
	if err != nil {
		log.Warnf("LogAnalysis: failed to query training_performance for %s: %v", workloadUID, err)
		updates["error"] = err.Error()
		return coreTask.RescheduleResult(updates), nil
	}
	hasRecentMetrics := len(perfRecords) > 0
	updates["has_recent_metrics"] = hasRecentMetrics
	updates["recent_metric_count"] = len(perfRecords)

	// Step 2: Read recent pod logs from OpenSearch
	keywordLines, totalScanned, err := e.scanPodLogs(ctx, workloadUID)
	if err != nil {
		log.Warnf("LogAnalysis: failed to scan logs for %s: %v", workloadUID, err)
		updates["log_scan_error"] = err.Error()
		return coreTask.RescheduleResult(updates), nil
	}
	updates["lines_scanned"] = totalScanned
	updates["keyword_lines_found"] = len(keywordLines)

	if len(keywordLines) == 0 {
		// No training-keyword lines in recent logs - nothing to do
		log.Debugf("LogAnalysis: no keyword lines found for %s (%d lines scanned)", workloadUID, totalScanned)
		return coreTask.RescheduleResult(updates), nil
	}

	// Step 3: If we have keyword lines but no metrics captured, analyze the gap
	if !hasRecentMetrics && len(keywordLines) > 0 {
		log.Infof("LogAnalysis: workload %s has %d keyword lines but no training_performance, analyzing gap",
			workloadUID, len(keywordLines))

		proposals := e.analyzeUnmatchedLines(keywordLines, framework, workloadUID)
		updates["gap_detected"] = true
		updates["proposals_generated"] = len(proposals)

		// Store proposals in system_config
		for _, p := range proposals {
			if err := e.storePatternProposal(ctx, p); err != nil {
				log.Warnf("LogAnalysis: failed to store pattern proposal: %v", err)
			}
		}

		// Store sample unmatched lines in task ext for debugging
		samples := keywordLines
		if len(samples) > maxUnmatchedSamples {
			samples = samples[:maxUnmatchedSamples]
		}
		updates["unmatched_samples"] = samples
	} else if hasRecentMetrics && len(keywordLines) > 0 {
		// Metrics are being captured. Check if there are lines with keywords
		// that are NOT being captured (partial coverage).
		log.Debugf("LogAnalysis: workload %s has both metrics and keyword lines, checking coverage", workloadUID)
		updates["gap_detected"] = false

		// Try to detect uncovered patterns by comparing keyword lines with known patterns
		uncovered := e.findUncoveredLines(ctx, keywordLines, framework)
		if len(uncovered) > 0 {
			updates["uncovered_line_count"] = len(uncovered)
			proposals := e.analyzeUnmatchedLines(uncovered, framework, workloadUID)
			updates["proposals_generated"] = len(proposals)
			for _, p := range proposals {
				if err := e.storePatternProposal(ctx, p); err != nil {
					log.Warnf("LogAnalysis: failed to store pattern proposal: %v", err)
				}
			}
		}
	}

	executionCount := e.GetExtInt(task, "execution_count") + 1
	updates["execution_count"] = executionCount

	return coreTask.RescheduleResult(updates), nil
}

// scanPodLogs reads recent logs from OpenSearch for the workload's pods
// and returns lines containing training-metric keywords.
func (e *LogAnalysisExecutor) scanPodLogs(
	ctx context.Context,
	workloadUID string,
) ([]unmatchedLine, int, error) {
	// Find pods for this workload (to get pod names for OpenSearch query)
	pods, err := e.findWorkloadPods(ctx, workloadUID)
	if err != nil {
		return nil, 0, fmt.Errorf("find pods: %w", err)
	}
	if len(pods) == 0 {
		return nil, 0, nil
	}

	// Pick one representative pod (prefer master-0)
	pod := e.selectRepresentativePod(pods)
	log.Debugf("LogAnalysis: scanning OpenSearch for pod %s/%s (workload %s, %d pods total)",
		pod.Namespace, pod.Name, workloadUID, len(pods))

	// Get OpenSearch client from StorageClientSet
	clusterClients := clientsets.GetClusterManager().GetCurrentClusterClients()
	if clusterClients == nil || clusterClients.StorageClientSet == nil || clusterClients.StorageClientSet.OpenSearch == nil {
		return nil, 0, fmt.Errorf("opensearch client not available")
	}
	osClient := clusterClients.StorageClientSet.OpenSearch

	// Build OpenSearch query: filter by pod name, last 10 minutes, sorted by timestamp
	query := map[string]interface{}{
		"query": map[string]interface{}{
			"bool": map[string]interface{}{
				"must": []map[string]interface{}{
					{"match_phrase": map[string]interface{}{"kubernetes.pod_name": pod.Name}},
					{"range": map[string]interface{}{
						"@timestamp": map[string]interface{}{
							"gte": "now-10m",
						},
					}},
				},
			},
		},
		"size": maxLogLines,
		"sort": []map[string]interface{}{
			{"@timestamp": map[string]interface{}{"order": "asc"}},
		},
		"_source": []string{"log", "message", "log_processed"},
	}

	queryJSON, err := json.Marshal(query)
	if err != nil {
		return nil, 0, fmt.Errorf("marshal query: %w", err)
	}

	searchReq := opensearchapi.SearchRequest{
		Index: []string{opensearchIndexPattern},
		Body:  strings.NewReader(string(queryJSON)),
	}

	resp, err := searchReq.Do(ctx, osClient)
	if err != nil {
		return nil, 0, fmt.Errorf("opensearch search: %w", err)
	}
	defer resp.Body.Close()

	if resp.IsError() {
		return nil, 0, fmt.Errorf("opensearch error: %s", resp.String())
	}

	// Parse OpenSearch response
	var result opensearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, 0, fmt.Errorf("decode opensearch response: %w", err)
	}

	totalScanned := len(result.Hits.Hits)
	var keywordLines []unmatchedLine

	for _, hit := range result.Hits.Hits {
		// Extract the log message from the hit
		logMsg := e.extractLogMessage(hit.Source)
		if logMsg == "" || len(logMsg) < 10 {
			continue
		}

		// Check for training keyword matches
		matchedKW := e.matchKeywords(logMsg)
		if len(matchedKW) > 0 {
			keywordLines = append(keywordLines, unmatchedLine{
				Line:      truncateLine(logMsg, 500),
				Keywords:  matchedKW,
				Timestamp: time.Now().Format(time.RFC3339),
			})
		}
	}

	return keywordLines, totalScanned, nil
}

// opensearchResponse represents the search response from OpenSearch
type opensearchResponse struct {
	Hits struct {
		Total struct {
			Value int `json:"value"`
		} `json:"total"`
		Hits []opensearchHit `json:"hits"`
	} `json:"hits"`
}

// opensearchHit represents a single search hit
type opensearchHit struct {
	Source map[string]interface{} `json:"_source"`
}

// extractLogMessage extracts the log text from an OpenSearch hit's _source.
// FluentBit writes logs with different field names depending on configuration:
// - "log": raw log message (bootstrap config with mergeLog + mergeLogKey)
// - "message": some configurations store the text here
// - "log_processed": parsed JSON content (bootstrap config mergeLogKey)
func (e *LogAnalysisExecutor) extractLogMessage(source map[string]interface{}) string {
	// Try "log" field first (most common for FluentBit kubernetes filter)
	if msg, ok := source["log"].(string); ok && msg != "" {
		return strings.TrimSpace(msg)
	}
	// Try "message" field
	if msg, ok := source["message"].(string); ok && msg != "" {
		return strings.TrimSpace(msg)
	}
	// Try "log_processed" -> might be a nested object with a "log" key
	if processed, ok := source["log_processed"].(map[string]interface{}); ok {
		if msg, ok := processed["log"].(string); ok && msg != "" {
			return strings.TrimSpace(msg)
		}
		if msg, ok := processed["message"].(string); ok && msg != "" {
			return strings.TrimSpace(msg)
		}
	}
	return ""
}

// matchKeywords returns the list of keyword names that match in the line
func (e *LogAnalysisExecutor) matchKeywords(line string) []string {
	lower := strings.ToLower(line)
	var matched []string
	for _, kw := range e.keywords {
		if kw.pattern.MatchString(lower) {
			matched = append(matched, kw.name)
		}
	}

	// Require at least 2 keyword matches to reduce false positives
	// (a line with just "step" could be anything, but "step" + "loss" is likely metrics)
	if len(matched) < 2 {
		return nil
	}
	return matched
}

// findWorkloadPods finds pods for a workload via workload_pod_reference.
// Includes deleted pods since OpenSearch retains historical logs.
func (e *LogAnalysisExecutor) findWorkloadPods(
	ctx context.Context,
	workloadUID string,
) ([]*model.GpuPods, error) {
	refs, err := e.workloadFacade.ListWorkloadPodReferenceByWorkloadUid(ctx, workloadUID)
	if err != nil || len(refs) == 0 {
		return nil, err
	}

	podUIDs := make([]string, 0, len(refs))
	for _, ref := range refs {
		podUIDs = append(podUIDs, ref.PodUID)
	}

	podFacade := database.GetFacade().GetPod()
	pods, err := podFacade.ListPodsByUids(ctx, podUIDs)
	if err != nil {
		return nil, err
	}
	return pods, nil
}

// selectRepresentativePod picks the best pod for log scanning (prefer master-0)
func (e *LogAnalysisExecutor) selectRepresentativePod(pods []*model.GpuPods) *model.GpuPods {
	for _, p := range pods {
		if strings.HasSuffix(p.Name, "master-0") || strings.HasSuffix(p.Name, "-0") {
			return p
		}
	}
	return pods[0]
}

// patternProposal represents a proposed regex pattern for telemetry-processor
type patternProposal struct {
	Framework   string   `json:"framework"`
	WorkloadUID string   `json:"workload_uid"`
	Pattern     string   `json:"pattern"`
	Type        string   `json:"type"` // "performance" or "blacklist"
	SampleLine  string   `json:"sample_line"`
	Keywords    []string `json:"keywords"`
	CreatedAt   string   `json:"created_at"`
}

// analyzeUnmatchedLines examines lines with training keywords and determines
// if they contain extractable performance metrics or should be blacklisted.
func (e *LogAnalysisExecutor) analyzeUnmatchedLines(
	lines []unmatchedLine,
	framework string,
	workloadUID string,
) []patternProposal {
	// Group lines by their structural signature to avoid duplicate proposals
	seen := make(map[string]bool)
	var proposals []patternProposal

	for _, line := range lines {
		sig := e.computeLineSignature(line.Line)
		if seen[sig] {
			continue
		}
		seen[sig] = true

		isMetric := e.isLikelyPerformanceLine(line.Line, line.Keywords)
		proposalType := "blacklist"
		if isMetric {
			proposalType = "performance"
		}

		pattern := e.generateRegexForLine(line.Line, isMetric)
		if pattern == "" {
			continue
		}

		proposals = append(proposals, patternProposal{
			Framework:   framework,
			WorkloadUID: workloadUID,
			Pattern:     pattern,
			Type:        proposalType,
			SampleLine:  truncateLine(line.Line, 300),
			Keywords:    line.Keywords,
			CreatedAt:   time.Now().Format(time.RFC3339),
		})
	}

	return proposals
}

// isLikelyPerformanceLine determines if a log line is a genuine training
// performance metric line (as opposed to a log message that happens to
// mention "loss" or "step").
//
// Heuristics for a real performance line:
// - Contains numeric values after keyword (e.g. "loss: 0.123", "step 500")
// - Has multiple key=value or key: value pairs
// - Contains separator patterns like |, comma, or tab between fields
func (e *LogAnalysisExecutor) isLikelyPerformanceLine(line string, keywords []string) bool {
	// Count numeric key-value pairs
	kvPattern := regexp.MustCompile(`(?i)(epoch|step|iteration|loss|lr|learning.?rate|grad.?norm|throughput|samples?.?per|tokens?.?per|tflops|mfu|batch.?size)\s*[=:]\s*[\d.eE+-]+`)
	kvMatches := kvPattern.FindAllString(line, -1)

	// If we have 3+ key-value pairs with numeric values, it's almost certainly a metric line
	if len(kvMatches) >= 3 {
		return true
	}

	// Check for tabular/delimited format: multiple numbers separated by | or ,
	delimPattern := regexp.MustCompile(`\d+\.?\d*\s*[|,]\s*\d+\.?\d*`)
	if delimPattern.MatchString(line) && len(kvMatches) >= 2 {
		return true
	}

	// Check for structured iteration output like "iteration 500/10000 | loss: 0.5"
	iterPattern := regexp.MustCompile(`(?i)(?:iter(?:ation)?|step|epoch)\s*(?:\d+\s*/\s*\d+|\d+)`)
	if iterPattern.MatchString(line) && len(kvMatches) >= 1 {
		return true
	}

	return false
}

// generateRegexForLine creates a regex pattern from a sample log line.
// For performance lines: generates a pattern with named capture groups.
// For blacklist lines: generates a pattern that matches the line structure.
func (e *LogAnalysisExecutor) generateRegexForLine(line string, isMetric bool) string {
	if isMetric {
		return e.generatePerformanceRegex(line)
	}
	return e.generateBlacklistRegex(line)
}

// generatePerformanceRegex creates a regex with named capture groups for
// extracting training metrics from a log line.
func (e *LogAnalysisExecutor) generatePerformanceRegex(line string) string {
	// Map of keyword -> named group
	fieldMap := map[string]string{
		"epoch":             "Epoch",
		"total_epochs":      "TotalEpochs",
		"step":              "CurrentIteration",
		"iteration":         "CurrentIteration",
		"iter":              "CurrentIteration",
		"loss":              "LmLoss",
		"total_loss":        "TotalLoss",
		"lm_loss":           "LmLoss",
		"lr":                "LearningRate",
		"learning_rate":     "LearningRate",
		"learning rate":     "LearningRate",
		"grad_norm":         "GradNorm",
		"grad norm":         "GradNorm",
		"throughput":        "SamplesPerSecond",
		"samples_per_second": "SamplesPerSecond",
		"tokens_per_second":  "TokensPerSecond",
		"tflops":            "TFLOPS",
		"mfu":               "Mfu",
		"batch_size":        "GlobalBatchSize",
		"global_batch_size": "GlobalBatchSize",
	}

	// Try to find key=value or key: value patterns and build a regex
	kvRe := regexp.MustCompile(`(?i)(epoch|total_epochs|step|iteration|iter|(?:total_|lm_)?loss|lr|learning[_\s]rate|grad[_\s]norm|throughput|samples_per_second|tokens_per_second|tflops|mfu|(?:global_)?batch_size)\s*[=:]\s*([\d.eE+-]+)`)

	matches := kvRe.FindAllStringSubmatchIndex(line, -1)
	if len(matches) < 2 {
		// Not enough key-value pairs to build a meaningful pattern
		return ""
	}

	// We need to work from right to left to preserve indices
	type replacement struct {
		start, end int
		groupName  string
		origValue  string
	}
	var replacements []replacement

	for _, match := range matches {
		keyStart, keyEnd := match[2], match[3]
		valStart, valEnd := match[4], match[5]
		key := strings.ToLower(line[keyStart:keyEnd])
		key = strings.ReplaceAll(key, " ", "_")

		groupName, ok := fieldMap[key]
		if !ok {
			continue
		}

		replacements = append(replacements, replacement{
			start:     valStart,
			end:       valEnd,
			groupName: groupName,
			origValue: line[valStart:valEnd],
		})
	}

	if len(replacements) < 2 {
		return ""
	}

	// Build a looser regex: replace the specific structure with patterns
	// Use the original line but with flexible whitespace and named groups for values
	var parts []string
	parts = append(parts, ".*") // prefix
	for i, r := range replacements {
		// Extract the key portion before the value
		keySearchStart := 0
		if i > 0 {
			keySearchStart = replacements[i-1].end
		}
		between := line[keySearchStart:r.start]
		// Make the between-text flexible (replace exact whitespace with \s+)
		between = regexp.QuoteMeta(between)
		between = strings.ReplaceAll(between, `\ `, `\s+`)
		between = strings.ReplaceAll(between, `\	`, `\s+`) // tab

		parts = append(parts, between)
		parts = append(parts, fmt.Sprintf(`(?P<%s>[+-]?\d+(?:\.\d+)?(?:[Ee][+-]?\d+)?)`, r.groupName))
	}
	parts = append(parts, ".*")

	return strings.Join(parts, "")
}

// generateBlacklistRegex creates a regex that matches a non-metric line
// so it can be excluded from future analysis.
func (e *LogAnalysisExecutor) generateBlacklistRegex(line string) string {
	// Extract a distinctive prefix/keyword from the line
	// Focus on the first ~80 chars as a signature
	sig := line
	if len(sig) > 80 {
		sig = sig[:80]
	}

	// Replace all numbers with \d+ for generalization
	numRe := regexp.MustCompile(`\d+`)
	sig = numRe.ReplaceAllString(sig, `\d+`)

	// Quote special regex chars but keep our \d+ replacements
	// First escape, then restore \d+
	escaped := regexp.QuoteMeta(sig)
	escaped = strings.ReplaceAll(escaped, `\\d\+`, `\d+`)

	// Replace whitespace sequences with \s+
	wsRe := regexp.MustCompile(`(?:\\\ )+`)
	escaped = wsRe.ReplaceAllString(escaped, `\s+`)

	return "^" + escaped + ".*$"
}

// computeLineSignature creates a structural signature of a line for deduplication.
// It replaces all numbers with # and collapses whitespace.
func (e *LogAnalysisExecutor) computeLineSignature(line string) string {
	// Replace numbers with #
	numRe := regexp.MustCompile(`[\d.]+(?:[eE][+-]?\d+)?`)
	sig := numRe.ReplaceAllString(line, "#")
	// Collapse whitespace
	wsRe := regexp.MustCompile(`\s+`)
	sig = wsRe.ReplaceAllString(sig, " ")
	// Truncate
	if len(sig) > 100 {
		sig = sig[:100]
	}
	return sig
}

// findUncoveredLines filters keyword lines to find those not matching existing patterns.
// It loads the framework's performance patterns from system_config and tests each line.
func (e *LogAnalysisExecutor) findUncoveredLines(
	ctx context.Context,
	lines []unmatchedLine,
	framework string,
) []unmatchedLine {
	// Load existing performance patterns for this framework
	patterns := e.loadFrameworkPatterns(ctx, framework)
	if len(patterns) == 0 {
		// No patterns loaded = all lines are uncovered
		return lines
	}

	var uncovered []unmatchedLine
	for _, line := range lines {
		matched := false
		for _, p := range patterns {
			if p.MatchString(line.Line) {
				matched = true
				break
			}
		}
		if !matched {
			uncovered = append(uncovered, line)
		}
	}
	return uncovered
}

// loadFrameworkPatterns loads compiled performance regex patterns for a framework
func (e *LogAnalysisExecutor) loadFrameworkPatterns(ctx context.Context, framework string) []*regexp.Regexp {
	configKey := "training.log.parser.framework." + strings.ToLower(framework)
	config, err := e.sysConfigFacade.GetByKey(ctx, configKey)
	if err != nil || config == nil {
		return nil
	}

	// Parse the FrameworkLogPatterns structure from JSONB
	perfPatterns, ok := config.Value["performance_patterns"]
	if !ok {
		return nil
	}

	patternsSlice, ok := perfPatterns.([]interface{})
	if !ok {
		return nil
	}

	var compiled []*regexp.Regexp
	for _, p := range patternsSlice {
		pm, ok := p.(map[string]interface{})
		if !ok {
			continue
		}
		patStr, ok := pm["pattern"].(string)
		if !ok || patStr == "" {
			continue
		}
		enabled, _ := pm["enabled"].(bool)
		if !enabled {
			continue
		}
		re, err := regexp.Compile(patStr)
		if err != nil {
			log.Debugf("LogAnalysis: failed to compile pattern %q: %v", patStr, err)
			continue
		}
		compiled = append(compiled, re)
	}

	// Also load blacklist patterns (to exclude known non-metric lines)
	blacklistKey := configKeyPrefix + "blacklist." + strings.ToLower(framework)
	blacklistConfig, err := e.sysConfigFacade.GetByKey(ctx, blacklistKey)
	if err == nil && blacklistConfig != nil {
		if blPatterns, ok := blacklistConfig.Value["patterns"].([]interface{}); ok {
			for _, bp := range blPatterns {
				if bpm, ok := bp.(map[string]interface{}); ok {
					if pat, ok := bpm["pattern"].(string); ok {
						if re, err := regexp.Compile(pat); err == nil {
							compiled = append(compiled, re)
						}
					}
				}
			}
		}
	}

	return compiled
}

// storePatternProposal saves a discovered pattern proposal to system_config
func (e *LogAnalysisExecutor) storePatternProposal(ctx context.Context, p patternProposal) error {
	var configKey string
	if p.Type == "performance" {
		configKey = configKeyPrefix + "proposals." + strings.ToLower(p.Framework)
	} else {
		configKey = configKeyPrefix + "blacklist." + strings.ToLower(p.Framework)
	}

	// Load existing config or create new
	existing, err := e.sysConfigFacade.GetByKey(ctx, configKey)
	if err != nil {
		return fmt.Errorf("load existing config: %w", err)
	}

	var patterns []interface{}
	if existing != nil {
		if pp, ok := existing.Value["patterns"].([]interface{}); ok {
			patterns = pp
		}
	}

	// Check for duplicates (by pattern string)
	for _, ep := range patterns {
		if epm, ok := ep.(map[string]interface{}); ok {
			if existPat, ok := epm["pattern"].(string); ok && existPat == p.Pattern {
				// Already exists
				return nil
			}
		}
	}

	// Add the new proposal
	entry := map[string]interface{}{
		"pattern":      p.Pattern,
		"sample_line":  p.SampleLine,
		"keywords":     p.Keywords,
		"workload_uid": p.WorkloadUID,
		"created_at":   p.CreatedAt,
		"status":       "proposed", // proposed -> validated -> promoted
		"enabled":      false,      // not enabled until reviewed
	}
	patterns = append(patterns, entry)

	// Cap the number of proposals
	if len(patterns) > 100 {
		patterns = patterns[len(patterns)-100:]
	}

	value := model.ExtType{
		"framework": p.Framework,
		"type":      p.Type,
		"patterns":  patterns,
	}

	if existing != nil {
		return e.sysConfigFacade.Update(ctx, existing, map[string]interface{}{
			"value":      value,
			"updated_at": time.Now(),
		})
	}

	description := fmt.Sprintf("Auto-discovered %s patterns for framework %s", p.Type, p.Framework)
	if p.Type == "blacklist" {
		description = fmt.Sprintf("Blacklisted non-metric patterns for framework %s", p.Framework)
	}

	newConfig := &model.SystemConfig{
		Key:         configKey,
		Value:       value,
		Description: description,
		Category:    "training.log.parser",
	}
	return e.sysConfigFacade.Create(ctx, newConfig)
}

// isWorkloadTerminated checks if the workload has finished running
func (e *LogAnalysisExecutor) isWorkloadTerminated(ctx context.Context, workloadUID string) bool {
	workload, err := e.workloadFacade.GetGpuWorkloadByUid(ctx, workloadUID)
	if err != nil || workload == nil {
		return true
	}
	terminatedStatuses := map[string]bool{
		"Completed": true, "Failed": true, "Succeeded": true, "Stopped": true,
		"Done": true, "Deleted": true,
	}
	isDeleted := workload.DeletedAt.Valid
	return isDeleted || terminatedStatuses[string(workload.Status)]
}

// initKeywords creates the pre-compiled keyword patterns
func initKeywords() []trainingKeyword {
	return []trainingKeyword{
		{name: "epoch", pattern: regexp.MustCompile(`\bepoch\b`)},
		{name: "step", pattern: regexp.MustCompile(`\bstep\b`)},
		{name: "iteration", pattern: regexp.MustCompile(`\biter(?:ation)?\b`)},
		{name: "loss", pattern: regexp.MustCompile(`\bloss\b`)},
		{name: "lr", pattern: regexp.MustCompile(`\b(?:lr|learning[_\s]?rate)\b`)},
		{name: "grad_norm", pattern: regexp.MustCompile(`\bgrad[_\s]?norm\b`)},
		{name: "throughput", pattern: regexp.MustCompile(`\b(?:throughput|samples?[_/]s|tokens?[_/]s)\b`)},
		{name: "tflops", pattern: regexp.MustCompile(`\btflops?\b`)},
		{name: "mfu", pattern: regexp.MustCompile(`\bmfu\b`)},
		{name: "batch_size", pattern: regexp.MustCompile(`\bbatch[_\s]?size\b`)},
		{name: "consumed_samples", pattern: regexp.MustCompile(`\bconsumed[_\s]?samples\b`)},
		{name: "consumed_tokens", pattern: regexp.MustCompile(`\bconsumed[_\s]?tokens\b`)},
	}
}

// truncateLine truncates a line to maxLen characters
func truncateLine(line string, maxLen int) string {
	if len(line) <= maxLen {
		return line
	}
	return line[:maxLen] + "..."
}

// Ensure interface compliance
var _ coreTask.TaskExecutor = (*LogAnalysisExecutor)(nil)
