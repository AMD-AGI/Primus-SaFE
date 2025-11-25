package reconciler

import (
	"testing"

	primusSafeV1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TestWorkloadAnalyzer_Analyze_Primus tests Primus image recognition
func TestWorkloadAnalyzer_Analyze_Primus(t *testing.T) {
	analyzer := NewWorkloadAnalyzer(nil)

	workload := &primusSafeV1.Workload{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "primus-workload",
			Namespace: "default",
		},
		Spec: primusSafeV1.WorkloadSpec{
			Template: primusSafeV1.Template{
				Containers: []corev1.Container{
					{
						Image:   "registry.example.com/primus-training:v1.2.3",
						Command: []string{"python", "train.py"},
						Env: []corev1.EnvVar{
							{Name: "PRIMUS_CONFIG", Value: "/config/primus.yaml"},
						},
					},
				},
			},
		},
	}

	result := analyzer.Analyze(workload)

	assert.NotNil(t, result)
	assert.Equal(t, "primus", result.Framework)
	assert.Equal(t, "training", result.Type)
	assert.Greater(t, result.Confidence, 0.6) // Should be high confidence
	assert.Contains(t, result.Reason, "image:")
	assert.Contains(t, result.Reason, "env:")
	assert.Equal(t, []string{"python", "train.py"}, result.MatchedCommand)
	assert.Contains(t, result.MatchedEnvVars, "PRIMUS_CONFIG")
}

// TestWorkloadAnalyzer_Analyze_DeepSpeed tests DeepSpeed environment variable recognition
func TestWorkloadAnalyzer_Analyze_DeepSpeed(t *testing.T) {
	analyzer := NewWorkloadAnalyzer(nil)

	workload := &primusSafeV1.Workload{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "deepspeed-workload",
			Namespace: "default",
		},
		Spec: primusSafeV1.WorkloadSpec{
			Template: primusSafeV1.Template{
				Containers: []corev1.Container{
					{
						Image: "registry.example.com/deepspeed:v0.9.0",
						Env: []corev1.EnvVar{
							{Name: "DEEPSPEED_CONFIG", Value: "/config/ds_config.json"},
						},
					},
				},
			},
		},
	}

	result := analyzer.Analyze(workload)

	assert.NotNil(t, result)
	assert.Equal(t, "deepspeed", result.Framework)
	assert.Greater(t, result.Confidence, 0.6)
	assert.Contains(t, result.MatchedEnvVars, "DEEPSPEED_CONFIG")
}

// TestWorkloadAnalyzer_Analyze_Megatron tests Megatron image + env recognition
func TestWorkloadAnalyzer_Analyze_Megatron(t *testing.T) {
	analyzer := NewWorkloadAnalyzer(nil)

	workload := &primusSafeV1.Workload{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "megatron-workload",
			Namespace: "default",
		},
		Spec: primusSafeV1.WorkloadSpec{
			Template: primusSafeV1.Template{
				Containers: []corev1.Container{
					{
						Image: "registry.example.com/megatron-lm:v2.0",
						Env: []corev1.EnvVar{
							{Name: "MEGATRON_CONFIG", Value: "/config/megatron.yaml"},
						},
					},
				},
			},
		},
	}

	result := analyzer.Analyze(workload)

	assert.NotNil(t, result)
	assert.Equal(t, "megatron", result.Framework)
	assert.Greater(t, result.Confidence, 0.6)
}

// TestWorkloadAnalyzer_Analyze_Unknown tests unknown framework (no matching features)
func TestWorkloadAnalyzer_Analyze_Unknown(t *testing.T) {
	analyzer := NewWorkloadAnalyzer(nil)

	workload := &primusSafeV1.Workload{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "unknown-workload",
			Namespace: "default",
		},
		Spec: primusSafeV1.WorkloadSpec{
			Template: primusSafeV1.Template{
				Containers: []corev1.Container{
					{
						Image: "registry.example.com/unknown:v1.0",
					},
				},
			},
		},
	}

	result := analyzer.Analyze(workload)

	assert.NotNil(t, result)
	assert.Equal(t, "unknown", result.Framework)
	assert.Equal(t, 0.0, result.Confidence)
	assert.Equal(t, "no_pattern_matched", result.Reason)
}

// TestWorkloadAnalyzer_Analyze_CommandAndArgs verifies command and args are correctly recorded
func TestWorkloadAnalyzer_Analyze_CommandAndArgs(t *testing.T) {
	analyzer := NewWorkloadAnalyzer(nil)

	workload := &primusSafeV1.Workload{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-workload",
			Namespace: "default",
		},
		Spec: primusSafeV1.WorkloadSpec{
			Template: primusSafeV1.Template{
				Containers: []corev1.Container{
					{
						Image:   "registry.example.com/primus:v1.2.3",
						Command: []string{"python", "-u", "train.py"},
						Args:    []string{"--config", "config.yaml", "--epochs", "100"},
						Env: []corev1.EnvVar{
							{Name: "PRIMUS_CONFIG", Value: "/config/primus.yaml"},
						},
					},
				},
			},
		},
	}

	result := analyzer.Analyze(workload)

	assert.NotNil(t, result)
	assert.Equal(t, "primus", result.Framework)
	assert.Equal(t, []string{"python", "-u", "train.py"}, result.MatchedCommand)
	assert.Equal(t, []string{"--config", "config.yaml", "--epochs", "100"}, result.MatchedArgs)
}

// TestMatchImagePattern tests image pattern matching
func TestMatchImagePattern(t *testing.T) {
	analyzer := NewWorkloadAnalyzer(nil)

	tests := []struct {
		name     string
		image    string
		pattern  string
		expected bool
	}{
		{
			name:     "Exact match",
			image:    "primus-training:v1",
			pattern:  "primus",
			expected: true,
		},
		{
			name:     "Case insensitive",
			image:    "PRIMUS-TRAINING:v1",
			pattern:  "primus",
			expected: true,
		},
		{
			name:     "Wildcard pattern",
			image:    "registry.example.com/primus-training:v1.2.3",
			pattern:  "primus*",
			expected: true,
		},
		{
			name:     "No match",
			image:    "deepspeed:v0.9",
			pattern:  "primus",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := analyzer.matchImagePattern(tt.image, tt.pattern)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestGetSortedRules tests rule sorting by priority
func TestGetSortedRules(t *testing.T) {
	analyzer := NewWorkloadAnalyzer(nil)

	rules := analyzer.getSortedRules()

	assert.NotEmpty(t, rules)
	// Verify rules are sorted by priority (descending)
	for i := 0; i < len(rules)-1; i++ {
		assert.GreaterOrEqual(t, rules[i].Priority, rules[i+1].Priority)
	}
}

