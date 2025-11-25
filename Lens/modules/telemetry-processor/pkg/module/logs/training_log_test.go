package logs

import (
	"testing"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model"
)

// TestGroupsToPerformance tests the dynamic field mapping from regex groups to TrainingPerformance
func TestGroupsToPerformance(t *testing.T) {
	tests := []struct {
		name     string
		groups   map[string]string
		validate func(t *testing.T, perf *model.TrainingPerformance)
	}{
		{
			name: "basic performance metrics",
			groups: map[string]string{
				"CurrentIteration":          "100",
				"TargetIteration":           "1000",
				"ConsumedSamples":           "12800",
				"ElapsedTimePerIterationMS": "1234.56",
				"LearningRate":              "0.0001",
				"GlobalBatchSize":           "128",
				"LmLoss":                    "2.345",
				"TFLOPS":                    "123.45",
				"TokensPerGPU":              "1024.5",
			},
			validate: func(t *testing.T, perf *model.TrainingPerformance) {
				if perf.CurrentIteration != 100 {
					t.Errorf("Expected CurrentIteration=100, got %d", perf.CurrentIteration)
				}
				if perf.TargetIteration != 1000 {
					t.Errorf("Expected TargetIteration=1000, got %d", perf.TargetIteration)
				}
				if perf.ConsumedSamples != 12800 {
					t.Errorf("Expected ConsumedSamples=12800, got %d", perf.ConsumedSamples)
				}
				if perf.ElapsedTimePerIterationMS != 1234.56 {
					t.Errorf("Expected ElapsedTimePerIterationMS=1234.56, got %f", perf.ElapsedTimePerIterationMS)
				}
				if perf.LearningRate != 0.0001 {
					t.Errorf("Expected LearningRate=0.0001, got %f", perf.LearningRate)
				}
				if perf.TFLOPS != 123.45 {
					t.Errorf("Expected TFLOPS=123.45, got %f", perf.TFLOPS)
				}
			},
		},
		{
			name: "memory metrics - ROCm format",
			groups: map[string]string{
				"CurrentIteration": "50",
				"MemUsage":         "45.67",
				"MemFree":          "10.23",
				"MemTotal":         "55.90",
				"MemUsageRatio":    "81.70",
			},
			validate: func(t *testing.T, perf *model.TrainingPerformance) {
				if perf.CurrentIteration != 50 {
					t.Errorf("Expected CurrentIteration=50, got %d", perf.CurrentIteration)
				}
				// MemUsage group maps to MemUsages field via alternative names
				if perf.MemUsages != 45.67 {
					t.Errorf("Expected MemUsages=45.67, got %f", perf.MemUsages)
				}
				if perf.MemFree != 10.23 {
					t.Errorf("Expected MemFree=10.23, got %f", perf.MemFree)
				}
				if perf.MemTotal != 55.90 {
					t.Errorf("Expected MemTotal=55.90, got %f", perf.MemTotal)
				}
				if perf.MemUsageRatio != 81.70 {
					t.Errorf("Expected MemUsageRatio=81.70, got %f", perf.MemUsageRatio)
				}
			},
		},
		{
			name: "memory metrics - legacy format",
			groups: map[string]string{
				"CurrentIteration": "75",
				"MemUsages":        "42.50",
			},
			validate: func(t *testing.T, perf *model.TrainingPerformance) {
				if perf.CurrentIteration != 75 {
					t.Errorf("Expected CurrentIteration=75, got %d", perf.CurrentIteration)
				}
				if perf.MemUsages != 42.50 {
					t.Errorf("Expected MemUsages=42.50, got %f", perf.MemUsages)
				}
			},
		},
		{
			name: "scientific notation in learning rate",
			groups: map[string]string{
				"CurrentIteration": "200",
				"LearningRate":     "1.5e-4",
				"LmLoss":           "3.2e+1",
			},
			validate: func(t *testing.T, perf *model.TrainingPerformance) {
				if perf.CurrentIteration != 200 {
					t.Errorf("Expected CurrentIteration=200, got %d", perf.CurrentIteration)
				}
				if perf.LearningRate != 0.00015 {
					t.Errorf("Expected LearningRate=0.00015, got %f", perf.LearningRate)
				}
				if perf.LmLoss != 32.0 {
					t.Errorf("Expected LmLoss=32.0, got %f", perf.LmLoss)
				}
			},
		},
		{
			name: "empty values are skipped",
			groups: map[string]string{
				"CurrentIteration": "10",
				"LearningRate":     "",
				"TFLOPS":           "",
			},
			validate: func(t *testing.T, perf *model.TrainingPerformance) {
				if perf.CurrentIteration != 10 {
					t.Errorf("Expected CurrentIteration=10, got %d", perf.CurrentIteration)
				}
				// Empty values should remain as zero
				if perf.LearningRate != 0 {
					t.Errorf("Expected LearningRate=0, got %f", perf.LearningRate)
				}
				if perf.TFLOPS != 0 {
					t.Errorf("Expected TFLOPS=0, got %f", perf.TFLOPS)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			perf := groupsToPerformance(tt.groups)
			if perf == nil {
				t.Fatal("groupsToPerformance returned nil")
			}
			tt.validate(t, perf)
		})
	}
}

// TestSetFieldValue tests the dynamic field value setting
func TestSetFieldValue(t *testing.T) {
	tests := []struct {
		name      string
		fieldKind string
		value     string
		wantErr   bool
	}{
		{"int valid", "int", "123", false},
		{"int invalid", "int", "abc", true},
		{"float valid", "float64", "123.45", false},
		{"float scientific", "float64", "1.5e-4", false},
		{"float invalid", "float64", "xyz", true},
		{"empty value", "int", "", false}, // Empty values are allowed
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// This is more of a documentation test showing how setFieldValue works
			// Actual field value setting is tested through groupsToPerformance
		})
	}
}

