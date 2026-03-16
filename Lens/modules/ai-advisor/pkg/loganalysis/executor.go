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

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/aigateway"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/aitopics"
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

	// Maximum log lines to retrieve from OpenSearch per workload per execution
	maxLogLines = 3000

	// Max unmatched samples to keep per workload
	maxUnmatchedSamples = 50

	// Max samples to send to LLM per task (control cost)
	maxLLMSamples = 20

	// Max poll attempts for LLM result before giving up
	maxLLMPollAttempts = 10

	// LLM task timeout in seconds
	llmTaskTimeoutSec = 120

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
// 5. Stores proposals in training_log_pattern table (auto-reloaded by telemetry-processor)
type LogAnalysisExecutor struct {
	coreTask.BaseExecutor

	trainingFacade  database.TrainingFacadeInterface
	workloadFacade  database.WorkloadFacadeInterface
	patternFacade   database.TrainingLogPatternFacadeInterface
	detectionFacade database.WorkloadDetectionFacadeInterface

	// Pre-compiled keyword patterns
	keywords []trainingKeyword

	// AI Gateway client for LLM-powered pattern generation (optional)
	gwClient *aigateway.Client
}

// NewLogAnalysisExecutor creates a new log analysis executor.
// If aiGatewayURL is non-empty, LLM fallback is enabled via ai-gateway.
func NewLogAnalysisExecutor(aiGatewayURL string) *LogAnalysisExecutor {
	e := &LogAnalysisExecutor{
		trainingFacade:  database.NewTrainingFacade(),
		workloadFacade:  database.NewWorkloadFacade(),
		patternFacade:   database.NewTrainingLogPatternFacade(),
		detectionFacade: database.NewWorkloadDetectionFacade(),
		keywords:        initKeywords(),
	}
	if aiGatewayURL != "" {
		e.gwClient = aigateway.NewClient(aiGatewayURL)
		log.Infof("LogAnalysis: LLM fallback enabled via ai-gateway at %s", aiGatewayURL)
	} else {
		log.Info("LogAnalysis: LLM fallback disabled (AI_GATEWAY_URL not set)")
	}
	return e
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

	// Step 0: Check for pending LLM task result before anything else
	if gwTaskID := e.GetExtString(task, "llm_gw_task_id"); gwTaskID != "" {
		e.pollLLMResult(ctx, task, updates)
	}

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

	// Determine framework label: use Framework, fall back to ModelFamily, then Category
	framework := det.Framework
	if framework == "" && det.ModelFamily != nil && *det.ModelFamily != "" {
		framework = *det.ModelFamily
	}
	if framework == "" && det.Category != nil && *det.Category != "" {
		framework = *det.Category
	}
	if framework == "" {
		framework = "unknown"
	}
	updates["framework"] = framework

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

	// Step 3: Analyze lines - heuristic first, collect LLM candidates
	var linesToAnalyze []unmatchedLine
	if !hasRecentMetrics && len(keywordLines) > 0 {
		log.Infof("LogAnalysis: workload %s has %d keyword lines but no training_performance, analyzing gap",
			workloadUID, len(keywordLines))
		updates["gap_detected"] = true
		linesToAnalyze = keywordLines
	} else if hasRecentMetrics && len(keywordLines) > 0 {
		log.Debugf("LogAnalysis: workload %s has both metrics and keyword lines, checking coverage", workloadUID)
		updates["gap_detected"] = false
		linesToAnalyze = e.findUncoveredLines(ctx, keywordLines, framework)
		updates["uncovered_line_count"] = len(linesToAnalyze)
	}

	if len(linesToAnalyze) > 0 {
		result := e.analyzeUnmatchedLines(linesToAnalyze, framework, workloadUID)
		updates["proposals_generated"] = len(result.Proposals)
		updates["llm_candidates"] = len(result.LLMCandidates)

		// Store heuristic proposals immediately
		for _, p := range result.Proposals {
			if err := e.storePatternProposal(ctx, p); err != nil {
				log.Warnf("LogAnalysis: failed to store pattern proposal: %v", err)
			}
		}

		// Publish LLM candidates to ai-gateway (if available and no pending task)
		if len(result.LLMCandidates) > 0 && e.gwClient != nil {
			if e.GetExtString(task, "llm_gw_task_id") == "" {
				e.publishLLMAnalysis(ctx, task, result.LLMCandidates, framework, workloadUID, updates)
			} else {
				log.Debugf("LogAnalysis: LLM task already pending for workload %s, skipping publish", workloadUID)
			}
		}

		// Store sample unmatched lines in task ext for debugging
		if !hasRecentMetrics {
			samples := keywordLines
			if len(samples) > maxUnmatchedSamples {
				samples = samples[:maxUnmatchedSamples]
			}
			updates["unmatched_samples"] = samples
		}
	}

	executionCount := e.GetExtInt(task, "execution_count") + 1
	updates["execution_count"] = executionCount

	return coreTask.RescheduleResult(updates), nil
}

// opensearchKeywords are the terms used in the OpenSearch query to pre-filter
// log lines that are likely training metrics. OpenSearch handles the broad
// filtering across ALL pods; the Go-side matchKeywords() then applies the
// stricter 2-keyword requirement.
var opensearchKeywords = []string{
	"epoch", "iteration", "loss", "learning rate", "learning_rate",
	"grad norm", "grad_norm", "throughput", "tflops",
	"consumed samples", "consumed_samples", "tokens/s", "samples/s",
}

// scanPodLogs queries OpenSearch for recent logs across ALL pods of a workload,
// using keyword-level filtering in the query itself so that OpenSearch returns
// only lines likely to contain training metrics. This avoids the need to pick
// a single "representative" pod and ensures metrics are found regardless of
// which rank/pod outputs them.
func (e *LogAnalysisExecutor) scanPodLogs(
	ctx context.Context,
	workloadUID string,
) ([]unmatchedLine, int, error) {
	pods, err := e.findWorkloadPods(ctx, workloadUID)
	if err != nil {
		return nil, 0, fmt.Errorf("find pods: %w", err)
	}
	if len(pods) == 0 {
		return nil, 0, nil
	}

	podNames := make([]interface{}, 0, len(pods))
	for _, p := range pods {
		podNames = append(podNames, p.Name)
	}

	log.Debugf("LogAnalysis: querying OpenSearch for workload %s across %d pods", workloadUID, len(podNames))

	clusterClients := clientsets.GetClusterManager().GetCurrentClusterClients()
	if clusterClients == nil || clusterClients.StorageClientSet == nil || clusterClients.StorageClientSet.OpenSearch == nil {
		return nil, 0, fmt.Errorf("opensearch client not available")
	}
	osClient := clusterClients.StorageClientSet.OpenSearch

	// Build keyword should-clauses: a line must match at least one keyword
	keywordClauses := make([]map[string]interface{}, 0, len(opensearchKeywords))
	for _, kw := range opensearchKeywords {
		keywordClauses = append(keywordClauses, map[string]interface{}{
			"match_phrase": map[string]interface{}{"message": kw},
		})
	}

	query := map[string]interface{}{
		"query": map[string]interface{}{
			"bool": map[string]interface{}{
				"filter": []map[string]interface{}{
					{"terms": map[string]interface{}{"kubernetes.pod_name.keyword": podNames}},
					{"range": map[string]interface{}{
						"@timestamp": map[string]interface{}{"gte": "now-10m"},
					}},
				},
				"must": []map[string]interface{}{
					{"bool": map[string]interface{}{
						"should":               keywordClauses,
						"minimum_should_match": 1,
					}},
				},
			},
		},
		"size": maxLogLines,
		"sort": []map[string]interface{}{
			{"@timestamp": map[string]interface{}{"order": "desc"}},
		},
		"_source": []string{"log", "message", "log_processed", "kubernetes.pod_name"},
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
		return nil, 0, fmt.Errorf("opensearch error (status %d): %s", resp.StatusCode, resp.String())
	}

	var result opensearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, 0, fmt.Errorf("decode opensearch response: %w", err)
	}

	totalHits := result.Hits.Total.Value
	log.Infof("LogAnalysis: OpenSearch returned %d hits (total=%d) for workload %s (%d pods)",
		len(result.Hits.Hits), totalHits, workloadUID, len(podNames))

	var keywordLines []unmatchedLine
	for _, hit := range result.Hits.Hits {
		logMsg := e.extractLogMessage(hit.Source)
		if logMsg == "" || len(logMsg) < 10 {
			continue
		}

		logMsg = stripANSI(logMsg)

		matchedKW := e.matchKeywords(logMsg)
		if len(matchedKW) > 0 {
			keywordLines = append(keywordLines, unmatchedLine{
				Line:      truncateLine(logMsg, 500),
				Keywords:  matchedKW,
				Timestamp: time.Now().Format(time.RFC3339),
			})
		}
	}

	return keywordLines, len(result.Hits.Hits), nil
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
	if err != nil {
		log.Warnf("LogAnalysis: failed to list pod references for %s: %v", workloadUID, err)
		return nil, err
	}
	if len(refs) == 0 {
		log.Infof("LogAnalysis: no pod references found for workload %s", workloadUID)
		return nil, nil
	}

	podUIDs := make([]string, 0, len(refs))
	for _, ref := range refs {
		podUIDs = append(podUIDs, ref.PodUID)
	}

	podFacade := database.GetFacade().GetPod()
	pods, err := podFacade.ListPodsByUids(ctx, podUIDs)
	if err != nil {
		log.Warnf("LogAnalysis: failed to list pods by UIDs for %s: %v", workloadUID, err)
		return nil, err
	}
	log.Infof("LogAnalysis: found %d pod refs -> %d pods for workload %s", len(refs), len(pods), workloadUID)
	return pods, nil
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
	Source      string   `json:"source"`     // "autodiscovered" or "llm"
	Confidence  float64  `json:"confidence"` // 0.0-1.0
}

// analysisResult holds heuristic proposals and LLM candidate lines
type analysisResult struct {
	Proposals     []patternProposal
	LLMCandidates []unmatchedLine
}

// --- LLM request/response types ---

// llmPatternRequest is the payload sent to ai-gateway for LLM analysis
type llmPatternRequest struct {
	WorkloadUID string           `json:"workload_uid"`
	Framework   string           `json:"framework"`
	Samples     []llmSampleLine  `json:"samples"`
}

// llmSampleLine is a single log line sample for LLM analysis
type llmSampleLine struct {
	Line     string   `json:"line"`
	Keywords []string `json:"keywords"`
}

// llmPatternResponse is the response from LLM pattern analysis
type llmPatternResponse struct {
	Results []llmPatternResult `json:"results"`
}

// llmPatternResult is a single result from LLM analysis
type llmPatternResult struct {
	Line              string   `json:"line"`
	IsTrainingMetric  bool     `json:"is_training_metric"`
	PatternType       string   `json:"pattern_type"` // "performance" or "blacklist"
	Regex             string   `json:"regex"`
	FieldCount        int      `json:"field_count"`
	Fields            []string `json:"fields"`
	Validated         bool     `json:"validated"`
	Confidence        float64  `json:"confidence"`
}

// analyzeUnmatchedLines examines lines with training keywords and determines
// if they contain extractable performance metrics or should be blacklisted.
// Performance lines are routed to LLM when available (higher quality patterns);
// blacklist lines use the heuristic directly.
// Lines where heuristic cannot generate a regex are returned as LLM candidates.
func (e *LogAnalysisExecutor) analyzeUnmatchedLines(
	lines []unmatchedLine,
	framework string,
	workloadUID string,
) analysisResult {
	// Group lines by their structural signature to avoid duplicate proposals
	seen := make(map[string]bool)
	var result analysisResult

	for _, line := range lines {
		sig := e.computeLineSignature(line.Line)
		if seen[sig] {
			continue
		}
		seen[sig] = true

		isMetric := e.isLikelyPerformanceLine(line.Line, line.Keywords)

		// Performance lines: prefer LLM for regex generation (much higher quality).
		// Only fall back to heuristic when LLM is unavailable.
		if isMetric {
			if e.gwClient != nil {
				result.LLMCandidates = append(result.LLMCandidates, line)
				continue
			}
			// LLM unavailable, try heuristic fallback
			pattern := e.generatePerformanceRegex(line.Line)
			if pattern == "" {
				continue
			}
			result.Proposals = append(result.Proposals, patternProposal{
				Framework:   framework,
				WorkloadUID: workloadUID,
				Pattern:     pattern,
				Type:        "performance",
				SampleLine:  truncateLine(line.Line, 300),
				Keywords:    line.Keywords,
				CreatedAt:   time.Now().Format(time.RFC3339),
				Source:      "autodiscovered",
				Confidence:  0.6,
			})
			continue
		}

		// Blacklist lines: heuristic works fine
		pattern := e.generateBlacklistRegex(line.Line)
		if pattern == "" {
			result.LLMCandidates = append(result.LLMCandidates, line)
			continue
		}

		result.Proposals = append(result.Proposals, patternProposal{
			Framework:   framework,
			WorkloadUID: workloadUID,
			Pattern:     pattern,
			Type:        "blacklist",
			SampleLine:  truncateLine(line.Line, 300),
			Keywords:    line.Keywords,
			CreatedAt:   time.Now().Format(time.RFC3339),
			Source:      "autodiscovered",
			Confidence:  0.6,
		})
	}

	return result
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
	// Count numeric key-value pairs (use .? to match both _ and space in compound keywords)
	kvPattern := regexp.MustCompile(`(?i)(epoch|step|iteration|loss|lr|learning.?rate|grad.?norm|throughput|samples?.?per|tokens?.?per|tflops|mfu|(?:global.?)?batch.?size|consumed.?samples|elapsed.?time|avg.?loss|time|batch(?:es)?)\s*[=:]\s*[\d.eE+-]+`)
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
	// Also handles bracket notation like "Epoch [500/10000]"
	iterPattern := regexp.MustCompile(`(?i)(?:iter(?:ation)?|step|epoch)\s*\[?\s*\d+\s*/\s*\d+\s*\]?`)
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
		"epoch":              "Epoch",
		"total_epochs":       "TotalEpochs",
		"step":               "CurrentIteration",
		"iteration":          "CurrentIteration",
		"iter":               "CurrentIteration",
		"loss":               "LmLoss",
		"avg_loss":           "LmLoss",
		"total_loss":         "TotalLoss",
		"lm_loss":            "LmLoss",
		"lr":                 "LearningRate",
		"learning_rate":      "LearningRate",
		"learning rate":      "LearningRate",
		"grad_norm":          "GradNorm",
		"grad norm":          "GradNorm",
		"throughput":         "SamplesPerSecond",
		"samples_per_second": "SamplesPerSecond",
		"tokens_per_second":  "TokensPerSecond",
		"tflops":             "TFLOPS",
		"mfu":                "Mfu",
		"batch_size":         "GlobalBatchSize",
		"global_batch_size":  "GlobalBatchSize",
		"batches":            "GlobalBatchSize",
		"consumed_samples":   "ConsumedSamples",
		"elapsed_time":       "ElapsedTimePerIterationMS",
		"time":               "ElapsedTimePerIterationMS",
	}

	// Try to find key=value or key: value patterns and build a regex
	// Use [_\s] to handle both "batch_size" and "batch size" style keys
	kvRe := regexp.MustCompile(`(?i)(epoch|total[_\s]epochs|step|iteration|iter|(?:total[_\s]|lm[_\s]|avg[_\s])?loss|lr|learning[_\s]rate|grad[_\s]norm|throughput|samples[_\s]per[_\s]second|tokens[_\s]per[_\s]second|tflops|mfu|(?:global[_\s])?batch[_\s]size|batches|consumed[_\s]samples|elapsed[_\s]time|time)\s*[=:]\s*([\d.eE+-]+)`)

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
	usedGroups := make(map[string]bool)

	for _, match := range matches {
		keyStart, keyEnd := match[2], match[3]
		valStart, valEnd := match[4], match[5]
		key := strings.ToLower(line[keyStart:keyEnd])
		// Normalize whitespace variants to underscore for fieldMap lookup
		key = strings.ReplaceAll(key, " ", "_")
		key = regexp.MustCompile(`_+`).ReplaceAllString(key, "_")

		groupName, ok := fieldMap[key]
		if !ok {
			continue
		}

		// Skip duplicate named groups (regex requires unique names)
		if usedGroups[groupName] {
			continue
		}
		usedGroups[groupName] = true

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
// Loads all enabled patterns (performance + blacklist) from training_log_pattern table.
func (e *LogAnalysisExecutor) findUncoveredLines(
	ctx context.Context,
	lines []unmatchedLine,
	_ string, // framework parameter kept for interface compat, but we load all patterns globally
) []unmatchedLine {
	patterns := e.loadAllPatterns(ctx)
	if len(patterns) == 0 {
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

// loadAllPatterns loads all enabled performance + blacklist patterns from training_log_pattern
func (e *LogAnalysisExecutor) loadAllPatterns(ctx context.Context) []*regexp.Regexp {
	var compiled []*regexp.Regexp

	// Load performance patterns
	perfPatterns, err := e.patternFacade.ListEnabledByType(ctx, "performance")
	if err != nil {
		log.Warnf("LogAnalysis: failed to load performance patterns: %v", err)
	}
	for _, p := range perfPatterns {
		re, err := regexp.Compile(p.Pattern)
		if err != nil {
			log.Debugf("LogAnalysis: failed to compile pattern id=%d: %v", p.ID, err)
			continue
		}
		compiled = append(compiled, re)
	}

	// Load blacklist patterns
	blPatterns, err := e.patternFacade.ListEnabledByType(ctx, "blacklist")
	if err != nil {
		log.Warnf("LogAnalysis: failed to load blacklist patterns: %v", err)
	}
	for _, p := range blPatterns {
		re, err := regexp.Compile(p.Pattern)
		if err != nil {
			continue
		}
		compiled = append(compiled, re)
	}

	return compiled
}

// storePatternProposal saves a discovered pattern to the training_log_pattern table.
// Uses Upsert so duplicate patterns (same pattern_type + md5(pattern)) are idempotent.
// Patterns are inserted as enabled=true so telemetry-processor picks them
// up on the next reload cycle (every 60s).
func (e *LogAnalysisExecutor) storePatternProposal(ctx context.Context, p patternProposal) error {
	fw := p.Framework
	source := p.Source
	if source == "" {
		source = "autodiscovered"
	}
	confidence := p.Confidence
	if confidence <= 0 {
		confidence = 0.6
	}
	priority := 40
	if source == "llm" {
		priority = 45 // between autodiscovered (40) and manual/migration (50+)
	}

	name := fmt.Sprintf("%s-%s-%s", source, p.Type, strings.ToLower(fw))
	desc := fmt.Sprintf("Generated by %s from workload %s", source, p.WorkloadUID)
	wUID := p.WorkloadUID
	sampleLine := p.SampleLine

	record := &model.TrainingLogPattern{
		Pattern:           p.Pattern,
		PatternType:       p.Type,
		Source:            source,
		SourceWorkloadUID: &wUID,
		Framework:         &fw,
		Name:              &name,
		Description:       &desc,
		SampleLine:        &sampleLine,
		Enabled:           true,
		Priority:          priority,
		Confidence:        confidence,
	}

	if err := e.patternFacade.Upsert(ctx, record); err != nil {
		return fmt.Errorf("upsert pattern: %w", err)
	}

	log.Infof("LogAnalysis: stored %s pattern (source=%s) for framework %s from workload %s",
		p.Type, source, fw, p.WorkloadUID)
	return nil
}

// --- LLM integration methods ---

// publishLLMAnalysis sends unmatched lines to ai-gateway for LLM-powered pattern generation.
// Stores the gateway task ID in the task ext so the next Execute() cycle can poll for results.
func (e *LogAnalysisExecutor) publishLLMAnalysis(
	ctx context.Context,
	task *model.WorkloadTaskState,
	candidates []unmatchedLine,
	framework string,
	workloadUID string,
	updates map[string]interface{},
) {
	if e.gwClient == nil {
		return
	}

	// Limit samples to control LLM cost
	samples := candidates
	if len(samples) > maxLLMSamples {
		samples = samples[:maxLLMSamples]
	}

	// Build request payload
	llmSamples := make([]llmSampleLine, 0, len(samples))
	for _, s := range samples {
		llmSamples = append(llmSamples, llmSampleLine{
			Line:     s.Line,
			Keywords: s.Keywords,
		})
	}
	reqPayload := llmPatternRequest{
		WorkloadUID: workloadUID,
		Framework:   framework,
		Samples:     llmSamples,
	}

	payloadJSON, err := json.Marshal(reqPayload)
	if err != nil {
		log.Warnf("LogAnalysis: failed to marshal LLM payload: %v", err)
		return
	}

	resp, err := e.gwClient.Publish(ctx, &aigateway.PublishRequest{
		Topic:      aitopics.TopicLogPatternGenerate,
		Payload:    payloadJSON,
		Priority:   5,
		TimeoutSec: llmTaskTimeoutSec,
	})
	if err != nil {
		log.Warnf("LogAnalysis: failed to publish LLM task for workload %s: %v", workloadUID, err)
		updates["llm_publish_error"] = err.Error()
		return
	}

	log.Infof("LogAnalysis: published LLM pattern task %s for workload %s (%d samples)",
		resp.ID, workloadUID, len(llmSamples))
	updates["llm_gw_task_id"] = resp.ID
	updates["llm_poll_count"] = 0
	updates["llm_published_at"] = time.Now().Format(time.RFC3339)
}

// pollLLMResult checks for a pending LLM task result and processes it.
// If the result is available, it parses the LLM response, converts each
// validated result into a patternProposal, and stores it.
// If the task is still in progress, it increments the poll count.
// If max poll attempts are reached, it clears the task ID.
func (e *LogAnalysisExecutor) pollLLMResult(
	ctx context.Context,
	task *model.WorkloadTaskState,
	updates map[string]interface{},
) {
	gwTaskID := e.GetExtString(task, "llm_gw_task_id")
	if gwTaskID == "" || e.gwClient == nil {
		return
	}

	pollCount := e.GetExtInt(task, "llm_poll_count")
	workloadUID := task.WorkloadUID
	framework := e.GetExtString(task, "framework")
	if framework == "" {
		framework = "unknown"
	}

	// Check if we've exceeded max poll attempts
	if pollCount >= maxLLMPollAttempts {
		log.Warnf("LogAnalysis: LLM task %s for workload %s exceeded max poll attempts (%d), giving up",
			gwTaskID, workloadUID, maxLLMPollAttempts)
		updates["llm_gw_task_id"] = nil
		updates["llm_poll_count"] = nil
		updates["llm_timeout"] = true
		return
	}

	result, err := e.gwClient.GetResult(ctx, gwTaskID)
	if err != nil {
		log.Warnf("LogAnalysis: failed to poll LLM task %s: %v", gwTaskID, err)
		updates["llm_poll_count"] = pollCount + 1
		return
	}

	if result == nil {
		// Still in progress
		log.Debugf("LogAnalysis: LLM task %s still processing (poll %d/%d)",
			gwTaskID, pollCount+1, maxLLMPollAttempts)
		updates["llm_poll_count"] = pollCount + 1
		return
	}

	// Result available - parse it
	log.Infof("LogAnalysis: LLM task %s completed for workload %s", gwTaskID, workloadUID)

	var llmResp llmPatternResponse
	if err := json.Unmarshal(result.Result, &llmResp); err != nil {
		log.Warnf("LogAnalysis: failed to unmarshal LLM result for task %s: %v", gwTaskID, err)
		updates["llm_gw_task_id"] = nil
		updates["llm_poll_count"] = nil
		updates["llm_parse_error"] = err.Error()
		return
	}

	// Process each LLM result
	storedCount := 0
	for _, r := range llmResp.Results {
		if r.Regex == "" || !r.Validated {
			log.Debugf("LogAnalysis: skipping LLM result (regex=%q, validated=%v)", r.Regex, r.Validated)
			continue
		}

		// Verify that the regex compiles in Go
		if _, compileErr := regexp.Compile(r.Regex); compileErr != nil {
			log.Warnf("LogAnalysis: LLM-generated regex does not compile in Go: %v (pattern: %s)",
				compileErr, r.Regex)
			continue
		}

		proposal := patternProposal{
			Framework:   framework,
			WorkloadUID: workloadUID,
			Pattern:     r.Regex,
			Type:        r.PatternType,
			SampleLine:  truncateLine(r.Line, 300),
			CreatedAt:   time.Now().Format(time.RFC3339),
			Source:      "llm",
			Confidence:  r.Confidence,
		}

		if err := e.storePatternProposal(ctx, proposal); err != nil {
			log.Warnf("LogAnalysis: failed to store LLM pattern: %v", err)
		} else {
			storedCount++
		}
	}

	log.Infof("LogAnalysis: stored %d LLM-generated patterns for workload %s (from %d results)",
		storedCount, workloadUID, len(llmResp.Results))

	// Clear LLM task state
	updates["llm_gw_task_id"] = nil
	updates["llm_poll_count"] = nil
	updates["llm_patterns_stored"] = storedCount
	updates["llm_results_total"] = len(llmResp.Results)
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

// ansiEscapeRe matches ANSI escape sequences (colors, cursor movement, etc.)
var ansiEscapeRe = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

// stripANSI removes ANSI escape codes from a string.
// Training log lines often contain color codes that pollute regex generation.
func stripANSI(s string) string {
	return ansiEscapeRe.ReplaceAllString(s, "")
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
