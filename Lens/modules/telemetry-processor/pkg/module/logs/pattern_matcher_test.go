package logs

import (
	"testing"
	"time"
)

func TestNewPatternMatcher(t *testing.T) {
	framework := &FrameworkLogPatterns{
		Name: "primus",
		IdentifyPatterns: []PatternConfig{
			{
				Name:       "test-identify",
				Pattern:    "primus.*training",
				Enabled:    true,
				Confidence: 0.8,
			},
		},
		PerformancePatterns: []PatternConfig{
			{
				Name:       "test-perf",
				Pattern:    "iteration\\s+(?P<Iteration>\\d+)",
				Enabled:    true,
				Confidence: 0.9,
			},
		},
	}

	matcher, err := NewPatternMatcher(framework)
	if err != nil {
		t.Fatalf("Failed to create pattern matcher: %v", err)
	}

	if matcher.framework.Name != "primus" {
		t.Error("Framework name mismatch")
	}

	if len(matcher.identifyRegexps) != 1 {
		t.Errorf("Expected 1 identify regex, got %d", len(matcher.identifyRegexps))
	}

	if len(matcher.performanceRegexps) != 1 {
		t.Errorf("Expected 1 performance regex, got %d", len(matcher.performanceRegexps))
	}
}

func TestPatternMatcher_MatchIdentify(t *testing.T) {
	framework := &FrameworkLogPatterns{
		Name: "primus",
		IdentifyPatterns: []PatternConfig{
			{
				Name:       "primus-identifier",
				Pattern:    "primus|Primus|PRIMUS",
				Enabled:    true,
				Confidence: 0.7,
			},
		},
	}

	matcher, err := NewPatternMatcher(framework)
	if err != nil {
		t.Fatalf("Failed to create pattern matcher: %v", err)
	}

	tests := []struct {
		name             string
		logLine          string
		expectMatched    bool
		expectPattern    string
		expectConfidence float64
	}{
		{
			name:             "match primus lowercase",
			logLine:          "Starting primus training",
			expectMatched:    true,
			expectPattern:    "primus-identifier",
			expectConfidence: 0.7,
		},
		{
			name:             "match Primus capitalized",
			logLine:          "Starting Primus training",
			expectMatched:    true,
			expectPattern:    "primus-identifier",
			expectConfidence: 0.7,
		},
		{
			name:          "no match",
			logLine:       "Starting training",
			expectMatched: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := matcher.MatchIdentify(tt.logLine)

			if result.Matched != tt.expectMatched {
				t.Errorf("Matched = %v, want %v", result.Matched, tt.expectMatched)
			}

			if result.Matched {
				if result.Pattern != tt.expectPattern {
					t.Errorf("Pattern = %v, want %v", result.Pattern, tt.expectPattern)
				}
				if result.Confidence != tt.expectConfidence {
					t.Errorf("Confidence = %v, want %v", result.Confidence, tt.expectConfidence)
				}
			}
		})
	}
}

func TestPatternMatcher_MatchPerformance(t *testing.T) {
	framework := &FrameworkLogPatterns{
		Name: "primus",
		PerformancePatterns: []PatternConfig{
			{
				Name:       "iteration-pattern",
				Pattern:    "iteration\\s+(?P<Iteration>\\d+)",
				Enabled:    true,
				Confidence: 0.9,
			},
		},
	}

	matcher, err := NewPatternMatcher(framework)
	if err != nil {
		t.Fatalf("Failed to create pattern matcher: %v", err)
	}

	tests := []struct {
		name          string
		logLine       string
		expectMatched bool
		expectGroups  map[string]string
	}{
		{
			name:          "match with iteration",
			logLine:       "iteration 100 completed",
			expectMatched: true,
			expectGroups: map[string]string{
				"Iteration": "100",
			},
		},
		{
			name:          "no match",
			logLine:       "training completed",
			expectMatched: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := matcher.MatchPerformance(tt.logLine)

			if result.Matched != tt.expectMatched {
				t.Errorf("Matched = %v, want %v", result.Matched, tt.expectMatched)
			}

			if result.Matched && tt.expectGroups != nil {
				for key, expectedValue := range tt.expectGroups {
					if result.Groups[key] != expectedValue {
						t.Errorf("Group[%s] = %v, want %v", key, result.Groups[key], expectedValue)
					}
				}
			}
		})
	}
}

func TestPatternMatcher_MatchTrainingEvent(t *testing.T) {
	framework := &FrameworkLogPatterns{
		Name: "primus",
		TrainingEvents: TrainingEventPatterns{
			StartTraining: []PatternConfig{
				{
					Name:       "start-training",
					Pattern:    "training\\s*\\.\\.\\.",
					Enabled:    true,
					Confidence: 0.9,
				},
			},
		},
	}

	matcher, err := NewPatternMatcher(framework)
	if err != nil {
		t.Fatalf("Failed to create pattern matcher: %v", err)
	}

	tests := []struct {
		name          string
		logLine       string
		eventType     string
		expectMatched bool
	}{
		{
			name:          "match start training",
			logLine:       "training ...",
			eventType:     "start_training",
			expectMatched: true,
		},
		{
			name:          "no match",
			logLine:       "starting model",
			eventType:     "start_training",
			expectMatched: false,
		},
		{
			name:          "wrong event type",
			logLine:       "training ...",
			eventType:     "end_training",
			expectMatched: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := matcher.MatchTrainingEvent(tt.logLine, tt.eventType)

			if result.Matched != tt.expectMatched {
				t.Errorf("Matched = %v, want %v", result.Matched, tt.expectMatched)
			}
		})
	}
}

func TestPatternMatcher_MatchCheckpointEvent(t *testing.T) {
	framework := &FrameworkLogPatterns{
		Name: "primus",
		CheckpointEvents: CheckpointEventPatterns{
			StartSaving: []PatternConfig{
				{
					Name:       "checkpoint-start",
					Pattern:    "saving checkpoint at iteration (?P<Iteration>\\d+) to (?P<Path>\\S+)",
					Enabled:    true,
					Confidence: 0.95,
				},
			},
		},
	}

	matcher, err := NewPatternMatcher(framework)
	if err != nil {
		t.Fatalf("Failed to create pattern matcher: %v", err)
	}

	tests := []struct {
		name          string
		logLine       string
		eventType     string
		expectMatched bool
		expectGroups  map[string]string
	}{
		{
			name:          "match checkpoint start",
			logLine:       "saving checkpoint at iteration 100 to /tmp/ckpt",
			eventType:     "start_saving",
			expectMatched: true,
			expectGroups: map[string]string{
				"Iteration": "100",
				"Path":      "/tmp/ckpt",
			},
		},
		{
			name:          "no match",
			logLine:       "checkpoint saved",
			eventType:     "start_saving",
			expectMatched: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := matcher.MatchCheckpointEvent(tt.logLine, tt.eventType)

			if result.Matched != tt.expectMatched {
				t.Errorf("Matched = %v, want %v", result.Matched, tt.expectMatched)
			}

			if result.Matched && tt.expectGroups != nil {
				for key, expectedValue := range tt.expectGroups {
					if result.Groups[key] != expectedValue {
						t.Errorf("Group[%s] = %v, want %v", key, result.Groups[key], expectedValue)
					}
				}
			}
		})
	}
}

func TestPatternMatcher_CalculateMatchScore(t *testing.T) {
	framework := &FrameworkLogPatterns{
		Name: "primus",
		IdentifyPatterns: []PatternConfig{
			{
				Name:       "primus-identifier",
				Pattern:    "primus",
				Enabled:    true,
				Confidence: 0.8,
			},
		},
	}

	matcher, err := NewPatternMatcher(framework)
	if err != nil {
		t.Fatalf("Failed to create pattern matcher: %v", err)
	}

	tests := []struct {
		name        string
		logLines    []string
		expectScore float64
	}{
		{
			name: "all match",
			logLines: []string{
				"primus training",
				"primus checkpoint",
			},
			expectScore: 1.0,
		},
		{
			name: "half match",
			logLines: []string{
				"primus training",
				"other log",
			},
			expectScore: 0.5,
		},
		{
			name:        "no match",
			logLines:    []string{"other log", "another log"},
			expectScore: 0.0,
		},
		{
			name:        "empty input",
			logLines:    []string{},
			expectScore: 0.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := matcher.CalculateMatchScore(tt.logLines)
			if score != tt.expectScore {
				t.Errorf("CalculateMatchScore() = %v, want %v", score, tt.expectScore)
			}
		})
	}
}

func TestPatternMatcher_GetFrameworkName(t *testing.T) {
	framework := &FrameworkLogPatterns{
		Name: "primus",
	}

	matcher, err := NewPatternMatcher(framework)
	if err != nil {
		t.Fatalf("Failed to create pattern matcher: %v", err)
	}

	if matcher.GetFrameworkName() != "primus" {
		t.Errorf("GetFrameworkName() = %v, want %v", matcher.GetFrameworkName(), "primus")
	}
}

func TestPatternMatcher_DisabledPatterns(t *testing.T) {
	framework := &FrameworkLogPatterns{
		Name: "primus",
		IdentifyPatterns: []PatternConfig{
			{
				Name:       "disabled-pattern",
				Pattern:    "test",
				Enabled:    false,
				Confidence: 0.8,
			},
		},
	}

	matcher, err := NewPatternMatcher(framework)
	if err != nil {
		t.Fatalf("Failed to create pattern matcher: %v", err)
	}

	// Disabled patterns should not be compiled
	if len(matcher.identifyRegexps) != 0 {
		t.Errorf("Expected 0 compiled patterns, got %d", len(matcher.identifyRegexps))
	}
}

func TestPatternMatcher_InvalidRegex(t *testing.T) {
	framework := &FrameworkLogPatterns{
		Name: "primus",
		IdentifyPatterns: []PatternConfig{
			{
				Name:       "invalid-pattern",
				Pattern:    "[invalid(regex",
				Enabled:    true,
				Confidence: 0.8,
			},
		},
	}

	// Should not return error, but pattern should be skipped
	matcher, err := NewPatternMatcher(framework)
	if err != nil {
		t.Fatalf("Failed to create pattern matcher: %v", err)
	}

	// Invalid pattern should be skipped
	if len(matcher.identifyRegexps) != 0 {
		t.Errorf("Expected 0 compiled patterns (invalid pattern skipped), got %d", len(matcher.identifyRegexps))
	}
}

func BenchmarkPatternMatcher_MatchIdentify(b *testing.B) {
	framework := &FrameworkLogPatterns{
		Name: "primus",
		IdentifyPatterns: []PatternConfig{
			{
				Name:       "primus-identifier",
				Pattern:    "primus|Primus|PRIMUS",
				Enabled:    true,
				Confidence: 0.7,
			},
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	matcher, _ := NewPatternMatcher(framework)
	logLine := "Starting primus training with iteration 100"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = matcher.MatchIdentify(logLine)
	}
}

func BenchmarkPatternMatcher_MatchPerformance(b *testing.B) {
	framework := &FrameworkLogPatterns{
		Name: "primus",
		PerformancePatterns: []PatternConfig{
			{
				Name:       "iteration-pattern",
				Pattern:    "iteration\\s+(?P<Iteration>\\d+).*consumed samples:\\s+(?P<Samples>\\d+)",
				Enabled:    true,
				Confidence: 0.9,
			},
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	matcher, _ := NewPatternMatcher(framework)
	logLine := "iteration 100 / 1000 | consumed samples: 12800 | elapsed time per iteration (ms): 234.5"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = matcher.MatchPerformance(logLine)
	}
}
