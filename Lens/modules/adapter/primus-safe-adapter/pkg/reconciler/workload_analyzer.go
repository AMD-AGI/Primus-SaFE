package reconciler

import (
	"regexp"
	"sort"
	"strings"

	primusSafeV1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

// WorkloadAnalyzer Analyze Workload component features
type WorkloadAnalyzer struct {
	frameworkRules map[string]*FrameworkRule
}

// FrameworkRule Framework recognition rule
type FrameworkRule struct {
	Name           string
	Priority       int
	ImagePatterns  []string // Image name patterns
	EnvVarKeys     []string // Key environment variables
	BaseConfidence float64  // Base confidence
}

// ComponentDetectionResult Component detection result
type ComponentDetectionResult struct {
	Framework      string
	Type           string
	Confidence     float64
	Reason         string
	MatchedCommand []string
	MatchedArgs    []string
	MatchedEnvVars map[string]string
}

// NewWorkloadAnalyzer Create analyzer
func NewWorkloadAnalyzer() *WorkloadAnalyzer {
	return &WorkloadAnalyzer{
		frameworkRules: loadFrameworkRules(),
	}
}

// Analyze Analyze Workload
func (a *WorkloadAnalyzer) Analyze(workload *primusSafeV1.Workload) *ComponentDetectionResult {

	// Try matching rules by priority
	for _, rule := range a.getSortedRules() {
		result := a.matchRule(workload, rule)
		if result != nil {
			return result
		}
	}

	return &ComponentDetectionResult{
		Framework:  "unknown",
		Type:       "unknown",
		Confidence: 0.0,
		Reason:     "no_pattern_matched",
	}
}

// matchRule Match a single rule
func (a *WorkloadAnalyzer) matchRule(
	workload *primusSafeV1.Workload,
	rule *FrameworkRule,
) *ComponentDetectionResult {

	confidence := 0.0
	matches := []string{}
	matchedCommand := []string{}
	matchedArgs := []string{}
	matchedEnvVars := make(map[string]string)

	// Extract image, command, args, env from workload
	image := workload.Spec.Image
	command := []string{}
	args := []string{}
	env := make(map[string]string)

	// Extract command and args from EntryPoint
	if workload.Spec.EntryPoint != "" {
		command = []string{"sh", "-c"}
		args = []string{workload.Spec.EntryPoint}
	}

	// Extract environment variables
	if workload.Spec.Env != nil {
		env = workload.Spec.Env
	}

	// 1. Check image name
	for _, pattern := range rule.ImagePatterns {
		if a.matchImagePattern(image, pattern) {
			confidence += 0.40 // Image match contributes 40%
			matches = append(matches, "image:"+pattern)
			break
		}
	}

	// 2. Check Command
	// Record full command for evidence
	if len(command) > 0 {
		matchedCommand = command
	}

	// 3. Check Args
	// Record full args for evidence
	if len(args) > 0 {
		matchedArgs = args
	}

	// 4. Check environment variables
	for _, key := range rule.EnvVarKeys {
		if val, ok := env[key]; ok {
			confidence += 0.30 // Environment variable match contributes 30%
			matches = append(matches, "env:"+key)
			matchedEnvVars[key] = val
			break
		}
	}

	// If confidence is too low, don't return result
	if confidence < 0.4 {
		return nil
	}

	// Apply rule's base confidence
	finalConfidence := confidence * rule.BaseConfidence

	return &ComponentDetectionResult{
		Framework:      rule.Name,
		Type:           "training", // Default to training
		Confidence:     finalConfidence,
		Reason:         strings.Join(matches, ","),
		MatchedCommand: matchedCommand,
		MatchedArgs:    matchedArgs,
		MatchedEnvVars: matchedEnvVars,
	}
}

// matchImagePattern Match image pattern
func (a *WorkloadAnalyzer) matchImagePattern(image, pattern string) bool {
	// Support regex and simple containment
	if strings.Contains(pattern, "*") {
		// Convert to regex
		regexPattern := strings.ReplaceAll(pattern, "*", ".*")
		matched, _ := regexp.MatchString(regexPattern, image)
		return matched
	}

	return strings.Contains(strings.ToLower(image), strings.ToLower(pattern))
}

// loadFrameworkRules Load framework rules (hardcoded)
func loadFrameworkRules() map[string]*FrameworkRule {
	rules := make(map[string]*FrameworkRule)

	// Primus
	rules["primus"] = &FrameworkRule{
		Name:     "primus",
		Priority: 100,
		ImagePatterns: []string{
			"primus",
			"primus-training",
			"primus-rocm",
		},
		EnvVarKeys: []string{
			"PRIMUS_CONFIG",
			"PRIMUS_VERSION",
			"FRAMEWORK",
		},
		BaseConfidence: 0.9,
	}

	// DeepSpeed
	rules["deepspeed"] = &FrameworkRule{
		Name:     "deepspeed",
		Priority: 90,
		ImagePatterns: []string{
			"deepspeed",
			"ds-training",
		},
		EnvVarKeys: []string{
			"DEEPSPEED_CONFIG",
			"DS_CONFIG",
		},
		BaseConfidence: 0.85,
	}

	// Megatron
	rules["megatron"] = &FrameworkRule{
		Name:     "megatron",
		Priority: 85,
		ImagePatterns: []string{
			"megatron",
			"megatron-lm",
		},
		EnvVarKeys: []string{
			"MEGATRON_CONFIG",
		},
		BaseConfidence: 0.85,
	}

	return rules
}

func (a *WorkloadAnalyzer) getSortedRules() []*FrameworkRule {
	rules := make([]*FrameworkRule, 0, len(a.frameworkRules))
	for _, rule := range a.frameworkRules {
		rules = append(rules, rule)
	}

	// Sort by priority
	sort.Slice(rules, func(i, j int) bool {
		return rules[i].Priority > rules[j].Priority
	})

	return rules
}

