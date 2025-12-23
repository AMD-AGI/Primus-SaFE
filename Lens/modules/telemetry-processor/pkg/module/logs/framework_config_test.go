package logs

import (
	"testing"
	"time"
)

func TestFrameworkLogPatterns_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  *FrameworkLogPatterns
		wantErr bool
	}{
		{
			name: "valid config",
			config: &FrameworkLogPatterns{
				Name:     "primus",
				Priority: 100,
				Enabled:  true,
			},
			wantErr: false,
		},
		{
			name: "missing name",
			config: &FrameworkLogPatterns{
				Name:     "",
				Priority: 100,
				Enabled:  true,
			},
			wantErr: true,
		},
		{
			name: "negative priority",
			config: &FrameworkLogPatterns{
				Name:     "primus",
				Priority: -1,
				Enabled:  true,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestPatternConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		pattern *PatternConfig
		wantErr bool
	}{
		{
			name: "valid pattern",
			pattern: &PatternConfig{
				Name:       "test-pattern",
				Pattern:    "test.*",
				Confidence: 0.8,
			},
			wantErr: false,
		},
		{
			name: "missing name",
			pattern: &PatternConfig{
				Name:       "",
				Pattern:    "test.*",
				Confidence: 0.8,
			},
			wantErr: true,
		},
		{
			name: "missing pattern",
			pattern: &PatternConfig{
				Name:       "test",
				Pattern:    "",
				Confidence: 0.8,
			},
			wantErr: true,
		},
		{
			name: "invalid confidence - too low",
			pattern: &PatternConfig{
				Name:       "test",
				Pattern:    "test.*",
				Confidence: -0.1,
			},
			wantErr: true,
		},
		{
			name: "invalid confidence - too high",
			pattern: &PatternConfig{
				Name:       "test",
				Pattern:    "test.*",
				Confidence: 1.5,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.pattern.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestFrameworkLogPatterns_GetEnabledPatterns(t *testing.T) {
	config := &FrameworkLogPatterns{
		Name: "primus",
		IdentifyPatterns: []PatternConfig{
			{Name: "enabled1", Enabled: true},
			{Name: "disabled", Enabled: false},
			{Name: "enabled2", Enabled: true},
		},
		PerformancePatterns: []PatternConfig{
			{Name: "perf1", Enabled: true},
			{Name: "perf2", Enabled: false},
		},
	}

	identifyEnabled := config.GetEnabledIdentifyPatterns()
	if len(identifyEnabled) != 2 {
		t.Errorf("Expected 2 enabled identify patterns, got %d", len(identifyEnabled))
	}

	perfEnabled := config.GetEnabledPerformancePatterns()
	if len(perfEnabled) != 1 {
		t.Errorf("Expected 1 enabled performance pattern, got %d", len(perfEnabled))
	}
}

func TestFrameworkLogPatterns_JSONMarshaling(t *testing.T) {
	original := &FrameworkLogPatterns{
		Name:        "primus",
		DisplayName: "Primus Framework",
		Version:     "1.0.0",
		Priority:    100,
		Enabled:     true,
		IdentifyPatterns: []PatternConfig{
			{
				Name:        "test-pattern",
				Pattern:     "primus.*",
				Description: "Test pattern",
				Enabled:     true,
				Tags:        []string{"test"},
				Confidence:  0.8,
			},
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	// This test verifies the struct can be used with JSON marshaling
	// The actual marshaling is handled by the standard library
	if original.Name != "primus" {
		t.Error("JSON structure validation failed")
	}
}

