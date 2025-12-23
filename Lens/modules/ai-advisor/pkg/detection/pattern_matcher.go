package detection

import (
	"regexp"
	"sync"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
)

// Inference pattern weight constants
const (
	InferenceWeightProcess = 0.35
	InferenceWeightImage   = 0.25
	InferenceWeightEnv     = 0.20
	InferenceWeightPort    = 0.10
	InferenceWeightCmdline = 0.10

	// Minimum matches required for inference detection
	InferenceMinMatches = 2
)

// PatternMatcher handles regex pattern matching for a framework
type PatternMatcher struct {
	framework            *FrameworkLogPatterns
	identifyRegexps      []*CompiledPattern
	performanceRegexps   []*CompiledPattern
	trainingEventRegexps map[string][]*CompiledPattern
	checkpointRegexps    map[string][]*CompiledPattern

	// Inference pattern regexps (NEW)
	inferenceProcessRegexps []*CompiledPattern
	inferenceEnvRegexps     []*CompiledPattern
	inferenceImageRegexps   []*CompiledPattern
	inferenceCmdlineRegexps []*CompiledPattern
	inferencePorts          []int

	mu sync.RWMutex
}

// CompiledPattern wraps a compiled regex with metadata
type CompiledPattern struct {
	Name       string
	Pattern    *regexp.Regexp
	Confidence float64
	Tags       []string
}

// MatchResult represents a pattern match result
type MatchResult struct {
	Matched    bool
	Pattern    string
	Groups     map[string]string
	Confidence float64
}

// NewPatternMatcher creates a new pattern matcher for a framework
func NewPatternMatcher(framework *FrameworkLogPatterns) (*PatternMatcher, error) {
	matcher := &PatternMatcher{
		framework:            framework,
		trainingEventRegexps: make(map[string][]*CompiledPattern),
		checkpointRegexps:    make(map[string][]*CompiledPattern),
	}
	
	if err := matcher.compile(); err != nil {
		return nil, err
	}
	
	return matcher, nil
}

// compile compiles all regex patterns
func (m *PatternMatcher) compile() error {
	// Compile identify patterns
	for _, pattern := range m.framework.IdentifyPatterns {
		if !pattern.Enabled {
			continue
		}

		regex, err := regexp.Compile(pattern.Pattern)
		if err != nil {
			log.Warnf("Failed to compile identify pattern %s: %v", pattern.Name, err)
			continue
		}

		m.identifyRegexps = append(m.identifyRegexps, &CompiledPattern{
			Name:       pattern.Name,
			Pattern:    regex,
			Confidence: pattern.Confidence,
			Tags:       pattern.Tags,
		})
	}

	// Compile performance patterns
	for _, pattern := range m.framework.PerformancePatterns {
		if !pattern.Enabled {
			continue
		}

		regex, err := regexp.Compile(pattern.Pattern)
		if err != nil {
			log.Warnf("Failed to compile performance pattern %s: %v", pattern.Name, err)
			continue
		}

		m.performanceRegexps = append(m.performanceRegexps, &CompiledPattern{
			Name:       pattern.Name,
			Pattern:    regex,
			Confidence: pattern.Confidence,
			Tags:       pattern.Tags,
		})
	}

	// Compile training event patterns
	m.compileEventPatterns("start_training", m.framework.TrainingEvents.StartTraining)
	m.compileEventPatterns("end_training", m.framework.TrainingEvents.EndTraining)
	m.compileEventPatterns("pause_training", m.framework.TrainingEvents.PauseTraining)
	m.compileEventPatterns("resume_training", m.framework.TrainingEvents.ResumeTraining)

	// Compile checkpoint patterns
	m.compileCheckpointPatterns("start_saving", m.framework.CheckpointEvents.StartSaving)
	m.compileCheckpointPatterns("end_saving", m.framework.CheckpointEvents.EndSaving)
	m.compileCheckpointPatterns("loading", m.framework.CheckpointEvents.Loading)

	// Compile inference patterns (NEW)
	if m.framework.InferencePatterns != nil {
		m.compileInferencePatterns()
	}

	return nil
}

// compileInferencePatterns compiles inference-specific patterns
func (m *PatternMatcher) compileInferencePatterns() {
	inf := m.framework.InferencePatterns

	// Compile process patterns
	for _, pattern := range inf.ProcessPatterns {
		if !pattern.Enabled {
			continue
		}
		regex, err := regexp.Compile(pattern.Pattern)
		if err != nil {
			log.Warnf("Failed to compile inference process pattern %s: %v", pattern.Name, err)
			continue
		}
		m.inferenceProcessRegexps = append(m.inferenceProcessRegexps, &CompiledPattern{
			Name:       pattern.Name,
			Pattern:    regex,
			Confidence: pattern.Confidence,
			Tags:       pattern.Tags,
		})
	}

	// Compile environment patterns
	for _, pattern := range inf.EnvPatterns {
		if !pattern.Enabled {
			continue
		}
		regex, err := regexp.Compile(pattern.Pattern)
		if err != nil {
			log.Warnf("Failed to compile inference env pattern %s: %v", pattern.Name, err)
			continue
		}
		m.inferenceEnvRegexps = append(m.inferenceEnvRegexps, &CompiledPattern{
			Name:       pattern.Name,
			Pattern:    regex,
			Confidence: pattern.Confidence,
			Tags:       pattern.Tags,
		})
	}

	// Compile image patterns
	for _, pattern := range inf.ImagePatterns {
		if !pattern.Enabled {
			continue
		}
		regex, err := regexp.Compile(pattern.Pattern)
		if err != nil {
			log.Warnf("Failed to compile inference image pattern %s: %v", pattern.Name, err)
			continue
		}
		m.inferenceImageRegexps = append(m.inferenceImageRegexps, &CompiledPattern{
			Name:       pattern.Name,
			Pattern:    regex,
			Confidence: pattern.Confidence,
			Tags:       pattern.Tags,
		})
	}

	// Compile cmdline patterns
	for _, pattern := range inf.CmdlinePatterns {
		if !pattern.Enabled {
			continue
		}
		regex, err := regexp.Compile(pattern.Pattern)
		if err != nil {
			log.Warnf("Failed to compile inference cmdline pattern %s: %v", pattern.Name, err)
			continue
		}
		m.inferenceCmdlineRegexps = append(m.inferenceCmdlineRegexps, &CompiledPattern{
			Name:       pattern.Name,
			Pattern:    regex,
			Confidence: pattern.Confidence,
			Tags:       pattern.Tags,
		})
	}

	// Copy ports
	m.inferencePorts = inf.Ports
}

// compileEventPatterns compiles training event patterns
func (m *PatternMatcher) compileEventPatterns(eventType string, patterns []PatternConfig) {
	for _, pattern := range patterns {
		if !pattern.Enabled {
			continue
		}
		
		regex, err := regexp.Compile(pattern.Pattern)
		if err != nil {
			log.Warnf("Failed to compile %s pattern %s: %v", eventType, pattern.Name, err)
			continue
		}
		
		m.trainingEventRegexps[eventType] = append(m.trainingEventRegexps[eventType], &CompiledPattern{
			Name:       pattern.Name,
			Pattern:    regex,
			Confidence: pattern.Confidence,
			Tags:       pattern.Tags,
		})
	}
}

// compileCheckpointPatterns compiles checkpoint event patterns
func (m *PatternMatcher) compileCheckpointPatterns(eventType string, patterns []PatternConfig) {
	for _, pattern := range patterns {
		if !pattern.Enabled {
			continue
		}
		
		regex, err := regexp.Compile(pattern.Pattern)
		if err != nil {
			log.Warnf("Failed to compile checkpoint %s pattern %s: %v", eventType, pattern.Name, err)
			continue
		}
		
		m.checkpointRegexps[eventType] = append(m.checkpointRegexps[eventType], &CompiledPattern{
			Name:       pattern.Name,
			Pattern:    regex,
			Confidence: pattern.Confidence,
			Tags:       pattern.Tags,
		})
	}
}

// MatchIdentify checks if log line matches framework identification patterns
func (m *PatternMatcher) MatchIdentify(logLine string) *MatchResult {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	for _, compiled := range m.identifyRegexps {
		if compiled.Pattern.MatchString(logLine) {
			return &MatchResult{
				Matched:    true,
				Pattern:    compiled.Name,
				Confidence: compiled.Confidence,
			}
		}
	}
	
	return &MatchResult{Matched: false}
}

// MatchPerformance matches performance log patterns
func (m *PatternMatcher) MatchPerformance(logLine string) *MatchResult {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	for _, compiled := range m.performanceRegexps {
		if match := compiled.Pattern.FindStringSubmatch(logLine); match != nil {
			groups := extractGroups(compiled.Pattern, match)
			return &MatchResult{
				Matched:    true,
				Pattern:    compiled.Name,
				Groups:     groups,
				Confidence: compiled.Confidence,
			}
		}
	}
	
	return &MatchResult{Matched: false}
}

// MatchTrainingEvent matches training lifecycle events
func (m *PatternMatcher) MatchTrainingEvent(logLine string, eventType string) *MatchResult {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	patterns, ok := m.trainingEventRegexps[eventType]
	if !ok {
		return &MatchResult{Matched: false}
	}
	
	for _, compiled := range patterns {
		if match := compiled.Pattern.FindStringSubmatch(logLine); match != nil {
			groups := extractGroups(compiled.Pattern, match)
			return &MatchResult{
				Matched:    true,
				Pattern:    compiled.Name,
				Groups:     groups,
				Confidence: compiled.Confidence,
			}
		}
	}
	
	return &MatchResult{Matched: false}
}

// MatchCheckpointEvent matches checkpoint events
func (m *PatternMatcher) MatchCheckpointEvent(logLine string, eventType string) *MatchResult {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	patterns, ok := m.checkpointRegexps[eventType]
	if !ok {
		return &MatchResult{Matched: false}
	}
	
	for _, compiled := range patterns {
		if match := compiled.Pattern.FindStringSubmatch(logLine); match != nil {
			groups := extractGroups(compiled.Pattern, match)
			return &MatchResult{
				Matched:    true,
				Pattern:    compiled.Name,
				Groups:     groups,
				Confidence: compiled.Confidence,
			}
		}
	}
	
	return &MatchResult{Matched: false}
}

// CalculateMatchScore calculates overall match score for framework identification
func (m *PatternMatcher) CalculateMatchScore(logLines []string) float64 {
	if len(logLines) == 0 {
		return 0.0
	}
	
	totalMatches := 0
	for _, line := range logLines {
		if result := m.MatchIdentify(line); result.Matched {
			totalMatches++
		}
	}
	
	return float64(totalMatches) / float64(len(logLines))
}

// GetFrameworkName returns the framework name
func (m *PatternMatcher) GetFrameworkName() string {
	return m.framework.Name
}

// extractGroups extracts named groups from regex match
func extractGroups(pattern *regexp.Regexp, match []string) map[string]string {
	groups := make(map[string]string)
	names := pattern.SubexpNames()

	for i, name := range names {
		if i > 0 && i < len(match) && name != "" {
			groups[name] = match[i]
		}
	}

	return groups
}

// ============================================================================
// Inference Pattern Matching Methods (Phase 2)
// ============================================================================

// InferenceMatchContext contains context for inference pattern matching
type InferenceMatchContext struct {
	// Process information
	ProcessNames []string // e.g., ["python", "vllm.entrypoints.openai.api_server"]
	ProcessCmdlines []string // Full command lines

	// Container information
	ImageName      string   // e.g., "vllm/vllm-openai:latest"
	ContainerPorts []int    // e.g., [8000, 8001]

	// Environment variables
	EnvVars map[string]string // e.g., {"VLLM_HOST": "0.0.0.0"}
}

// InferenceMatchResult represents the result of inference pattern matching
type InferenceMatchResult struct {
	Matched        bool
	FrameworkName  string
	FrameworkType  string
	Confidence     float64
	MatchedSources []string  // Which sources matched: "process", "image", "env", "port", "cmdline"
	Evidence       []string  // Human-readable evidence
}

// MatchInference performs inference framework detection
func (m *PatternMatcher) MatchInference(ctx *InferenceMatchContext) *InferenceMatchResult {
	if ctx == nil || !m.framework.IsInference() {
		return &InferenceMatchResult{Matched: false}
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	var totalConfidence float64
	var matchCount int
	var matchedSources []string
	var evidence []string

	// 1. Match process patterns (weight: 0.35)
	if result := m.matchProcessPatterns(ctx.ProcessNames, ctx.ProcessCmdlines); result.Matched {
		totalConfidence += result.Confidence * InferenceWeightProcess
		matchCount++
		matchedSources = append(matchedSources, "process")
		evidence = append(evidence, result.Evidence...)
	}

	// 2. Match image patterns (weight: 0.25)
	if result := m.matchImagePattern(ctx.ImageName); result.Matched {
		totalConfidence += result.Confidence * InferenceWeightImage
		matchCount++
		matchedSources = append(matchedSources, "image")
		evidence = append(evidence, result.Evidence...)
	}

	// 3. Match environment patterns (weight: 0.20)
	if result := m.matchEnvPatterns(ctx.EnvVars); result.Matched {
		totalConfidence += result.Confidence * InferenceWeightEnv
		matchCount++
		matchedSources = append(matchedSources, "env")
		evidence = append(evidence, result.Evidence...)
	}

	// 4. Match ports (weight: 0.10)
	if result := m.matchPorts(ctx.ContainerPorts); result.Matched {
		totalConfidence += InferenceWeightPort // Port matching is binary, no confidence
		matchCount++
		matchedSources = append(matchedSources, "port")
		evidence = append(evidence, result.Evidence...)
	}

	// 5. Match cmdline patterns (weight: 0.10)
	if result := m.matchCmdlinePatterns(ctx.ProcessCmdlines); result.Matched {
		totalConfidence += result.Confidence * InferenceWeightCmdline
		matchCount++
		matchedSources = append(matchedSources, "cmdline")
		evidence = append(evidence, result.Evidence...)
	}

	// Require at least InferenceMinMatches for a positive match
	if matchCount < InferenceMinMatches {
		return &InferenceMatchResult{Matched: false}
	}

	return &InferenceMatchResult{
		Matched:        true,
		FrameworkName:  m.framework.Name,
		FrameworkType:  FrameworkTypeInference,
		Confidence:     totalConfidence,
		MatchedSources: matchedSources,
		Evidence:       evidence,
	}
}

// patternMatchResult is an internal result for individual pattern matching
type patternMatchResult struct {
	Matched    bool
	Confidence float64
	Evidence   []string
}

// matchProcessPatterns matches process name patterns
func (m *PatternMatcher) matchProcessPatterns(processNames []string, cmdlines []string) *patternMatchResult {
	if len(m.inferenceProcessRegexps) == 0 {
		return &patternMatchResult{Matched: false}
	}

	var maxConfidence float64
	var evidence []string

	// Check process names
	for _, procName := range processNames {
		for _, compiled := range m.inferenceProcessRegexps {
			if compiled.Pattern.MatchString(procName) {
				if compiled.Confidence > maxConfidence {
					maxConfidence = compiled.Confidence
				}
				evidence = append(evidence, "process:"+procName+" matched "+compiled.Name)
			}
		}
	}

	// Also check cmdlines for process patterns
	for _, cmdline := range cmdlines {
		for _, compiled := range m.inferenceProcessRegexps {
			if compiled.Pattern.MatchString(cmdline) {
				if compiled.Confidence > maxConfidence {
					maxConfidence = compiled.Confidence
				}
				// Truncate long cmdlines for evidence
				truncated := cmdline
				if len(truncated) > 100 {
					truncated = truncated[:100] + "..."
				}
				evidence = append(evidence, "cmdline matched process pattern "+compiled.Name)
			}
		}
	}

	if maxConfidence > 0 {
		return &patternMatchResult{
			Matched:    true,
			Confidence: maxConfidence,
			Evidence:   evidence,
		}
	}

	return &patternMatchResult{Matched: false}
}

// matchImagePattern matches container image name patterns
func (m *PatternMatcher) matchImagePattern(imageName string) *patternMatchResult {
	if imageName == "" || len(m.inferenceImageRegexps) == 0 {
		return &patternMatchResult{Matched: false}
	}

	for _, compiled := range m.inferenceImageRegexps {
		if compiled.Pattern.MatchString(imageName) {
			return &patternMatchResult{
				Matched:    true,
				Confidence: compiled.Confidence,
				Evidence:   []string{"image:" + imageName + " matched " + compiled.Name},
			}
		}
	}

	return &patternMatchResult{Matched: false}
}

// matchEnvPatterns matches environment variable patterns
func (m *PatternMatcher) matchEnvPatterns(envVars map[string]string) *patternMatchResult {
	if len(envVars) == 0 || len(m.inferenceEnvRegexps) == 0 {
		return &patternMatchResult{Matched: false}
	}

	var maxConfidence float64
	var evidence []string

	for envKey, envValue := range envVars {
		// Match against env key
		for _, compiled := range m.inferenceEnvRegexps {
			if compiled.Pattern.MatchString(envKey) {
				if compiled.Confidence > maxConfidence {
					maxConfidence = compiled.Confidence
				}
				evidence = append(evidence, "env:"+envKey+" matched "+compiled.Name)
			}
			// Also match against "KEY=VALUE" format
			envPair := envKey + "=" + envValue
			if compiled.Pattern.MatchString(envPair) {
				if compiled.Confidence > maxConfidence {
					maxConfidence = compiled.Confidence
				}
				evidence = append(evidence, "env:"+envKey+"=... matched "+compiled.Name)
			}
		}
	}

	if maxConfidence > 0 {
		return &patternMatchResult{
			Matched:    true,
			Confidence: maxConfidence,
			Evidence:   evidence,
		}
	}

	return &patternMatchResult{Matched: false}
}

// matchPorts matches container ports against known inference service ports
func (m *PatternMatcher) matchPorts(containerPorts []int) *patternMatchResult {
	if len(containerPorts) == 0 || len(m.inferencePorts) == 0 {
		return &patternMatchResult{Matched: false}
	}

	var evidence []string

	for _, containerPort := range containerPorts {
		for _, expectedPort := range m.inferencePorts {
			if containerPort == expectedPort {
				evidence = append(evidence, "port:"+string(rune(containerPort))+" matched expected port")
				// Return on first match
				return &patternMatchResult{
					Matched:    true,
					Confidence: 1.0, // Port matching is binary
					Evidence:   []string{"port:" + intToStr(containerPort) + " matched"},
				}
			}
		}
	}

	return &patternMatchResult{Matched: false}
}

// matchCmdlinePatterns matches command line patterns
func (m *PatternMatcher) matchCmdlinePatterns(cmdlines []string) *patternMatchResult {
	if len(cmdlines) == 0 || len(m.inferenceCmdlineRegexps) == 0 {
		return &patternMatchResult{Matched: false}
	}

	var maxConfidence float64
	var evidence []string

	for _, cmdline := range cmdlines {
		for _, compiled := range m.inferenceCmdlineRegexps {
			if compiled.Pattern.MatchString(cmdline) {
				if compiled.Confidence > maxConfidence {
					maxConfidence = compiled.Confidence
				}
				evidence = append(evidence, "cmdline matched "+compiled.Name)
			}
		}
	}

	if maxConfidence > 0 {
		return &patternMatchResult{
			Matched:    true,
			Confidence: maxConfidence,
			Evidence:   evidence,
		}
	}

	return &patternMatchResult{Matched: false}
}

// intToStr converts int to string (simple helper to avoid strconv import)
func intToStr(n int) string {
	if n == 0 {
		return "0"
	}
	var result []byte
	negative := n < 0
	if negative {
		n = -n
	}
	for n > 0 {
		result = append([]byte{byte('0' + n%10)}, result...)
		n /= 10
	}
	if negative {
		result = append([]byte{'-'}, result...)
	}
	return string(result)
}

// IsInferenceFramework returns true if this matcher is for an inference framework
func (m *PatternMatcher) IsInferenceFramework() bool {
	return m.framework.IsInference()
}

// IsTrainingFramework returns true if this matcher is for a training framework
func (m *PatternMatcher) IsTrainingFramework() bool {
	return m.framework.IsTraining()
}

// GetFrameworkType returns the framework type
func (m *PatternMatcher) GetFrameworkType() string {
	return m.framework.GetType()
}

