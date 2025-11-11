package log_alert_engine

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
)

// PodLogData represents the log data needed for evaluation (to avoid circular dependency)
type PodLogData struct {
	Time      time.Time
	Message   string
	PodName   string
	PodId     string
	Namespace string
	Host      string
	Labels    map[string]string
}

// LogAlertRuleEngine is the core engine for evaluating log alert rules
type LogAlertRuleEngine struct {
	ctx    context.Context
	cancel context.CancelFunc

	// Rules cache
	rules      []*model.LogAlertRules
	rulesMutex sync.RWMutex

	// Compiled regex patterns cache
	patterns      map[int64]*regexp.Regexp
	patternsMutex sync.RWMutex

	// Rule states for threshold matching
	states      map[int64]*RuleState
	statesMutex sync.RWMutex

	// Cluster name
	clusterName string

	// Statistics
	evalCount  int64
	matchCount int64
	statsMutex sync.RWMutex
}

var (
	globalEngine     *LogAlertRuleEngine
	globalEngineOnce sync.Once
)

// InitGlobalEngine initializes the global rule engine
func InitGlobalEngine(ctx context.Context, clusterName string) error {
	var err error
	globalEngineOnce.Do(func() {
		engine := &LogAlertRuleEngine{
			clusterName: clusterName,
			patterns:    make(map[int64]*regexp.Regexp),
			states:      make(map[int64]*RuleState),
		}
		engine.ctx, engine.cancel = context.WithCancel(ctx)

		// Load rules
		if loadErr := engine.ReloadRules(); loadErr != nil {
			err = loadErr
			return
		}

		// Start periodic reload
		go engine.startPeriodicReload()

		// Start periodic cleanup
		go engine.startPeriodicCleanup()

		globalEngine = engine
		log.Info("Log alert rule engine initialized successfully")
	})

	return err
}

// GetGlobalEngine returns the global engine instance
func GetGlobalEngine() *LogAlertRuleEngine {
	return globalEngine
}

// ReloadRules reloads all rules from the database
func (e *LogAlertRuleEngine) ReloadRules() error {
	facade := database.GetFacade().GetLogAlertRule()

	enabled := true
	filter := &database.LogAlertRuleFilter{
		ClusterName: e.clusterName,
		Enabled:     &enabled,
		Limit:       1000, // Load up to 1000 rules
	}

	rules, _, err := facade.ListLogAlertRules(e.ctx, filter)
	if err != nil {
		log.Errorf("Failed to reload log alert rules: %v", err)
		return err
	}

	e.rulesMutex.Lock()
	e.rules = rules
	e.rulesMutex.Unlock()

	// Compile patterns
	e.compilePatterns()

	log.Infof("Reloaded %d log alert rules", len(rules))
	return nil
}

// compilePatterns compiles regex patterns for all rules
func (e *LogAlertRuleEngine) compilePatterns() {
	e.patternsMutex.Lock()
	defer e.patternsMutex.Unlock()

	e.patterns = make(map[int64]*regexp.Regexp)

	e.rulesMutex.RLock()
	defer e.rulesMutex.RUnlock()

	for _, rule := range e.rules {
		var matchConfig MatchConfig
		configBytes, _ := json.Marshal(rule.MatchConfig)
		if err := json.Unmarshal(configBytes, &matchConfig); err != nil {
			log.Errorf("Failed to unmarshal match config for rule %d: %v", rule.ID, err)
			continue
		}

		if matchConfig.Pattern != "" {
			pattern := matchConfig.Pattern
			if matchConfig.IgnoreCase {
				pattern = "(?i)" + pattern
			}

			re, err := regexp.Compile(pattern)
			if err != nil {
				log.Errorf("Failed to compile pattern for rule %d: %v", rule.ID, err)
				continue
			}

			e.patterns[rule.ID] = re
		}
	}
}

// startPeriodicReload starts periodic rule reloading
func (e *LogAlertRuleEngine) startPeriodicReload() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := e.ReloadRules(); err != nil {
				log.Errorf("Failed to reload rules: %v", err)
			}
		case <-e.ctx.Done():
			return
		}
	}
}

// startPeriodicCleanup starts periodic state cleanup
func (e *LogAlertRuleEngine) startPeriodicCleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			e.cleanupExpiredStates()
		case <-e.ctx.Done():
			return
		}
	}
}

// cleanupExpiredStates removes expired window counters
func (e *LogAlertRuleEngine) cleanupExpiredStates() {
	e.statesMutex.Lock()
	defer e.statesMutex.Unlock()

	now := time.Now()
	for ruleID, state := range e.states {
		for key, counter := range state.WindowCounters {
			// Remove events older than 1 hour
			if now.Sub(counter.LastUpdate) > time.Hour {
				delete(state.WindowCounters, key)
			}
		}

		// Remove empty states
		if len(state.WindowCounters) == 0 && now.Sub(state.LastEvaluation) > time.Hour {
			delete(e.states, ruleID)
		}
	}
}

// EvaluateLog evaluates a log against all rules
func (e *LogAlertRuleEngine) EvaluateLog(logData *PodLogData) []*EvaluationResult {
	if logData == nil {
		return nil
	}

	// Build evaluation context
	ctx := e.buildEvaluationContext(logData)

	var results []*EvaluationResult

	e.rulesMutex.RLock()
	rules := make([]*model.LogAlertRules, len(e.rules))
	copy(rules, e.rules)
	e.rulesMutex.RUnlock()

	for _, rule := range rules {
		startTime := time.Now()

		// Check label selectors first
		if !e.matchLabelSelectors(rule, ctx) {
			continue
		}

		// Evaluate the rule
		matched, reason := e.evaluateRule(rule, ctx)

		evalTimeMs := float64(time.Since(startTime).Microseconds()) / 1000.0

		if matched {
			result := &EvaluationResult{
				Matched:     true,
				RuleID:      rule.ID,
				RuleName:    rule.Name,
				Severity:    rule.Severity,
				Context:     ctx,
				MatchReason: reason,
				EvalTimeMs:  evalTimeMs,
			}

			// Parse alert template
			var template AlertTemplate
			templateBytes, _ := json.Marshal(rule.AlertTemplate)
			json.Unmarshal(templateBytes, &template)
			result.AlertTemplate = template

			// Parse route config
			var routeConfig RouteConfig
			routeBytes, _ := json.Marshal(rule.RouteConfig)
			json.Unmarshal(routeBytes, &routeConfig)
			result.RouteConfig = routeConfig

			results = append(results, result)

			// Update statistics
			e.statsMutex.Lock()
			e.matchCount++
			e.statsMutex.Unlock()
		}

		// Update evaluation count
		e.statsMutex.Lock()
		e.evalCount++
		e.statsMutex.Unlock()

		// Update rule state
		e.updateRuleState(rule.ID, matched, evalTimeMs)
	}

	return results
}

// buildEvaluationContext builds context from log data
func (e *LogAlertRuleEngine) buildEvaluationContext(logData *PodLogData) *EvaluationContext {
	ctx := &EvaluationContext{
		Log:         logData,
		LogTime:     logData.Time,
		Message:     logData.Message,
		PodName:     logData.PodName,
		PodID:       logData.PodId,
		NodeName:    logData.Host,
		Namespace:   logData.Namespace,
		Labels:      logData.Labels,
		ClusterName: e.clusterName,
	}

	// Extract workload ID from labels
	if jobName, ok := logData.Labels["training.kubeflow.org/job-name"]; ok {
		ctx.WorkloadID = jobName
	}

	return ctx
}

// matchLabelSelectors checks if log matches label selectors
func (e *LogAlertRuleEngine) matchLabelSelectors(rule *model.LogAlertRules, ctx *EvaluationContext) bool {
	var selectors []LabelSelector
	selectorsBytes, _ := json.Marshal(rule.LabelSelectors)
	if err := json.Unmarshal(selectorsBytes, &selectors); err != nil {
		log.Errorf("Failed to unmarshal label selectors for rule %d: %v", rule.ID, err)
		return false
	}

	// If no selectors, match all
	if len(selectors) == 0 {
		return true
	}

	// All selectors must match
	for _, selector := range selectors {
		if !e.matchSelector(selector, ctx) {
			return false
		}
	}

	return true
}

// matchSelector checks if a single selector matches
func (e *LogAlertRuleEngine) matchSelector(selector LabelSelector, ctx *EvaluationContext) bool {
	var value string
	var exists bool

	switch selector.Type {
	case "workload":
		value, exists = ctx.Labels[selector.Key], ctx.Labels[selector.Key] != ""
	case "namespace":
		if selector.Key == "namespace" {
			value, exists = ctx.Namespace, ctx.Namespace != ""
		} else {
			value, exists = ctx.Labels[selector.Key], ctx.Labels[selector.Key] != ""
		}
	case "pod":
		if selector.Key == "pod_name" {
			value, exists = ctx.PodName, ctx.PodName != ""
		} else {
			value, exists = ctx.Labels[selector.Key], ctx.Labels[selector.Key] != ""
		}
	case "node":
		if selector.Key == "node_name" {
			value, exists = ctx.NodeName, ctx.NodeName != ""
		}
	case "cluster":
		if selector.Key == "cluster_name" {
			value, exists = ctx.ClusterName, ctx.ClusterName != ""
		}
	case "custom":
		value, exists = ctx.Labels[selector.Key], ctx.Labels[selector.Key] != ""
	}

	switch selector.Operator {
	case "exists":
		return exists
	case "notexists":
		return !exists
	case "eq":
		return exists && len(selector.Values) > 0 && value == selector.Values[0]
	case "ne":
		return exists && len(selector.Values) > 0 && value != selector.Values[0]
	case "in":
		if !exists {
			return false
		}
		for _, v := range selector.Values {
			if value == v {
				return true
			}
		}
		return false
	case "notin":
		if !exists {
			return true
		}
		for _, v := range selector.Values {
			if value == v {
				return false
			}
		}
		return true
	case "regex":
		if !exists || len(selector.Values) == 0 {
			return false
		}
		re, err := regexp.Compile(selector.Values[0])
		if err != nil {
			return false
		}
		return re.MatchString(value)
	}

	return false
}

// evaluateRule evaluates a rule against the context
func (e *LogAlertRuleEngine) evaluateRule(rule *model.LogAlertRules, ctx *EvaluationContext) (bool, string) {
	var matchConfig MatchConfig
	configBytes, _ := json.Marshal(rule.MatchConfig)
	if err := json.Unmarshal(configBytes, &matchConfig); err != nil {
		return false, ""
	}

	switch rule.MatchType {
	case "pattern":
		return e.evaluatePattern(rule.ID, matchConfig, ctx)
	case "threshold":
		return e.evaluateThreshold(rule.ID, matchConfig, ctx)
	case "composite":
		return e.evaluateComposite(rule.ID, matchConfig, ctx)
	default:
		log.Warnf("Unknown match type: %s for rule %d", rule.MatchType, rule.ID)
		return false, ""
	}
}

// evaluatePattern evaluates pattern matching
func (e *LogAlertRuleEngine) evaluatePattern(ruleID int64, config MatchConfig, ctx *EvaluationContext) (bool, string) {
	e.patternsMutex.RLock()
	re, exists := e.patterns[ruleID]
	e.patternsMutex.RUnlock()

	if !exists || re == nil {
		return false, ""
	}

	matched := re.MatchString(ctx.Message)
	if matched {
		return true, "Pattern matched in log message"
	}

	return false, ""
}

// evaluateThreshold evaluates threshold matching
func (e *LogAlertRuleEngine) evaluateThreshold(ruleID int64, config MatchConfig, ctx *EvaluationContext) (bool, string) {
	if config.Threshold == nil {
		return false, ""
	}

	// First check if pattern matches
	if config.Pattern != "" {
		e.patternsMutex.RLock()
		re, exists := e.patterns[ruleID]
		e.patternsMutex.RUnlock()

		if !exists || re == nil || !re.MatchString(ctx.Message) {
			return false, ""
		}
	}

	// Build aggregation key
	aggKey := e.buildAggregationKey(config.Threshold.AggregateBy, ctx)

	// Get or create rule state
	e.statesMutex.Lock()
	state, exists := e.states[ruleID]
	if !exists {
		state = &RuleState{
			RuleID:         ruleID,
			WindowCounters: make(map[string]*WindowCounter),
		}
		e.states[ruleID] = state
	}

	counter, exists := state.WindowCounters[aggKey]
	if !exists {
		counter = &WindowCounter{
			Events: make([]time.Time, 0),
		}
		state.WindowCounters[aggKey] = counter
	}
	e.statesMutex.Unlock()

	// Add current event
	counter.Events = append(counter.Events, ctx.LogTime)
	counter.LastUpdate = time.Now()

	// Remove events outside time window
	windowStart := ctx.LogTime.Add(-time.Duration(config.Threshold.TimeWindow) * time.Second)
	validEvents := make([]time.Time, 0)
	for _, eventTime := range counter.Events {
		if eventTime.After(windowStart) {
			validEvents = append(validEvents, eventTime)
		}
	}
	counter.Events = validEvents
	counter.Count = len(validEvents)

	// Check threshold
	if counter.Count >= config.Threshold.CountThreshold {
		return true, "Threshold exceeded: " +
			"" + string(rune(counter.Count)) + " occurrences in " +
			string(rune(config.Threshold.TimeWindow)) + " seconds"
	}

	return false, ""
}

// evaluateComposite evaluates composite rules
func (e *LogAlertRuleEngine) evaluateComposite(ruleID int64, config MatchConfig, ctx *EvaluationContext) (bool, string) {
	// TODO: Implement composite rule evaluation
	return false, ""
}

// buildAggregationKey builds a key for aggregating events
func (e *LogAlertRuleEngine) buildAggregationKey(dimensions []string, ctx *EvaluationContext) string {
	if len(dimensions) == 0 {
		return "global"
	}

	parts := make([]string, 0, len(dimensions))
	for _, dim := range dimensions {
		switch dim {
		case "workload_id":
			parts = append(parts, ctx.WorkloadID)
		case "pod_name":
			parts = append(parts, ctx.PodName)
		case "node_name":
			parts = append(parts, ctx.NodeName)
		case "namespace":
			parts = append(parts, ctx.Namespace)
		}
	}

	return strings.Join(parts, ":")
}

// updateRuleState updates the evaluation state of a rule
func (e *LogAlertRuleEngine) updateRuleState(ruleID int64, matched bool, evalTimeMs float64) {
	e.statesMutex.Lock()
	defer e.statesMutex.Unlock()

	state, exists := e.states[ruleID]
	if !exists {
		state = &RuleState{
			RuleID:         ruleID,
			WindowCounters: make(map[string]*WindowCounter),
		}
		e.states[ruleID] = state
	}

	state.LastEvaluation = time.Now()
	if matched {
		state.LastFiring = time.Now()
		state.FiringCount++
	}
}

// GetStatistics returns engine statistics
func (e *LogAlertRuleEngine) GetStatistics() map[string]interface{} {
	e.statsMutex.RLock()
	defer e.statsMutex.RUnlock()

	e.rulesMutex.RLock()
	ruleCount := len(e.rules)
	e.rulesMutex.RUnlock()

	return map[string]interface{}{
		"rule_count":  ruleCount,
		"eval_count":  e.evalCount,
		"match_count": e.matchCount,
	}
}

// Shutdown gracefully shuts down the engine
func (e *LogAlertRuleEngine) Shutdown() {
	if e.cancel != nil {
		e.cancel()
	}
	log.Info("Log alert rule engine shutdown")
}
