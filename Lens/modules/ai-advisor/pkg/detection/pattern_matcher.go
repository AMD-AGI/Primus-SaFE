package detection

import (
	"regexp"
	"sync"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
)

// PatternMatcher handles regex pattern matching for a framework
type PatternMatcher struct {
	framework            *FrameworkLogPatterns
	identifyRegexps      []*CompiledPattern
	performanceRegexps   []*CompiledPattern
	trainingEventRegexps map[string][]*CompiledPattern
	checkpointRegexps    map[string][]*CompiledPattern
	mu                   sync.RWMutex
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
	
	return nil
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

