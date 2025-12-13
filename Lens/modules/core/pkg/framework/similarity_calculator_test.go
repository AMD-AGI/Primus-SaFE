package framework

import (
	"testing"

	"github.com/stretchr/testify/assert"

	coreModel "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model"
)

func TestSimilarityCalculator_CalculateSimilarity(t *testing.T) {
	calc := NewSimilarityCalculator()

	tests := []struct {
		name        string
		workload1   *coreModel.WorkloadSignature
		workload2   *coreModel.WorkloadSignature
		minScore    float64
		maxScore    float64
		description string
	}{
		{
			name: "exact match",
			workload1: &coreModel.WorkloadSignature{
				Image:     "registry.example.com/primus:v1.2.3",
				Command:   []string{"python", "train.py"},
				Args:      []string{"--epochs", "100"},
				Env:       map[string]string{"FRAMEWORK": "PyTorch"},
				Labels:    map[string]string{"app": "training"},
				Namespace: "default",
			},
			workload2: &coreModel.WorkloadSignature{
				Image:     "registry.example.com/primus:v1.2.3",
				Command:   []string{"python", "train.py"},
				Args:      []string{"--epochs", "100"},
				Env:       map[string]string{"FRAMEWORK": "PyTorch"},
				Labels:    map[string]string{"app": "training"},
				Namespace: "default",
			},
			minScore:    1.0,
			maxScore:    1.0,
			description: "identical workloads should have score 1.0",
		},
		{
			name: "same image different tag",
			workload1: &coreModel.WorkloadSignature{
				Image:     "registry.example.com/primus:v1.2.3",
				Command:   []string{"python", "train.py"},
				Args:      []string{"--epochs", "100"},
				Env:       map[string]string{"FRAMEWORK": "PyTorch"},
				Labels:    map[string]string{"app": "training"},
				Namespace: "default",
			},
			workload2: &coreModel.WorkloadSignature{
				Image:     "registry.example.com/primus:v1.2.4",
				Command:   []string{"python", "train.py"},
				Args:      []string{"--epochs", "100"},
				Env:       map[string]string{"FRAMEWORK": "PyTorch"},
				Labels:    map[string]string{"app": "training"},
				Namespace: "default",
			},
			minScore:    0.95,
			maxScore:    0.97,
			description: "same image different tag should have high score ~0.96",
		},
		{
			name: "different command",
			workload1: &coreModel.WorkloadSignature{
				Image:     "registry.example.com/primus:v1.2.3",
				Command:   []string{"python", "train.py"},
				Args:      []string{"--epochs", "100"},
				Env:       map[string]string{"FRAMEWORK": "PyTorch"},
				Labels:    map[string]string{"app": "training"},
				Namespace: "default",
			},
			workload2: &coreModel.WorkloadSignature{
				Image:     "registry.example.com/primus:v1.2.3",
				Command:   []string{"python", "test.py"},
				Args:      []string{"--epochs", "100"},
				Env:       map[string]string{"FRAMEWORK": "PyTorch"},
				Labels:    map[string]string{"app": "training"},
				Namespace: "default",
			},
			minScore:    0.80,
			maxScore:    0.85,
			description: "different command should reduce score",
		},
		{
			name: "different framework",
			workload1: &coreModel.WorkloadSignature{
				Image:     "registry.example.com/primus:v1.2.3",
				Command:   []string{"python", "train.py"},
				Args:      []string{"--epochs", "100"},
				Env:       map[string]string{"FRAMEWORK": "PyTorch"},
				Labels:    map[string]string{"app": "training"},
				Namespace: "default",
			},
			workload2: &coreModel.WorkloadSignature{
				Image:     "registry.example.com/primus:v1.2.3",
				Command:   []string{"python", "train.py"},
				Args:      []string{"--epochs", "100"},
				Env:       map[string]string{"FRAMEWORK": "TensorFlow"},
				Labels:    map[string]string{"app": "training"},
				Namespace: "default",
			},
			minScore:    0.75,
			maxScore:    0.85,
			description: "different framework should reduce score",
		},
		{
			name: "completely different",
			workload1: &coreModel.WorkloadSignature{
				Image:     "registry.example.com/primus:v1.2.3",
				Command:   []string{"python", "train.py"},
				Args:      []string{"--epochs", "100"},
				Env:       map[string]string{"FRAMEWORK": "PyTorch"},
				Labels:    map[string]string{"app": "training"},
				Namespace: "default",
			},
			workload2: &coreModel.WorkloadSignature{
				Image:     "registry.other.com/other:v2.0.0",
				Command:   []string{"java", "Main.java"},
				Args:      []string{},
				Env:       map[string]string{},
				Labels:    map[string]string{},
				Namespace: "production",
			},
			minScore:    0.0,
			maxScore:    0.1,
			description: "completely different workloads should have low score",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := calc.CalculateSimilarity(tt.workload1, tt.workload2)
			assert.NotNil(t, result)
			assert.GreaterOrEqual(t, result.Score, tt.minScore, tt.description)
			assert.LessOrEqual(t, result.Score, tt.maxScore, tt.description)
			assert.NotNil(t, result.Details)

			// Verify all detail scores are between 0 and 1
			assert.GreaterOrEqual(t, result.Details.ImageScore, 0.0)
			assert.LessOrEqual(t, result.Details.ImageScore, 1.0)
			assert.GreaterOrEqual(t, result.Details.CommandScore, 0.0)
			assert.LessOrEqual(t, result.Details.CommandScore, 1.0)
			assert.GreaterOrEqual(t, result.Details.EnvScore, 0.0)
			assert.LessOrEqual(t, result.Details.EnvScore, 1.0)
			assert.GreaterOrEqual(t, result.Details.ArgsScore, 0.0)
			assert.LessOrEqual(t, result.Details.ArgsScore, 1.0)
			assert.GreaterOrEqual(t, result.Details.LabelScore, 0.0)
			assert.LessOrEqual(t, result.Details.LabelScore, 1.0)
		})
	}
}

func TestSimilarityCalculator_CalculateImageSimilarity(t *testing.T) {
	calc := NewSimilarityCalculator()

	tests := []struct {
		name     string
		img1     string
		img2     string
		expected float64
	}{
		{
			name:     "exact match",
			img1:     "registry.example.com/primus:v1.2.3",
			img2:     "registry.example.com/primus:v1.2.3",
			expected: 1.0,
		},
		{
			name:     "same image different tag",
			img1:     "registry.example.com/primus:v1.2.3",
			img2:     "registry.example.com/primus:v1.2.4",
			expected: 0.9,
		},
		{
			name:     "same registry different image",
			img1:     "registry.example.com/primus:v1.2.3",
			img2:     "registry.example.com/other:v1.0.0",
			expected: 0.5,
		},
		{
			name:     "completely different",
			img1:     "registry.example.com/primus:v1.2.3",
			img2:     "other.registry.com/other:v1.0.0",
			expected: 0.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := calc.calculateImageSimilarity(tt.img1, tt.img2)
			assert.Equal(t, tt.expected, score)
		})
	}
}

func TestSimilarityCalculator_CalculateSliceSimilarity(t *testing.T) {
	calc := NewSimilarityCalculator()

	tests := []struct {
		name     string
		slice1   []string
		slice2   []string
		expected float64
	}{
		{
			name:     "exact match",
			slice1:   []string{"python", "train.py"},
			slice2:   []string{"python", "train.py"},
			expected: 1.0,
		},
		{
			name:     "both empty",
			slice1:   []string{},
			slice2:   []string{},
			expected: 1.0,
		},
		{
			name:     "one empty",
			slice1:   []string{"python"},
			slice2:   []string{},
			expected: 0.0,
		},
		{
			name:     "partial overlap",
			slice1:   []string{"python", "train.py", "--epochs"},
			slice2:   []string{"python", "test.py", "--epochs"},
			expected: 0.5, // 2 common out of 4 total
		},
		{
			name:     "no overlap",
			slice1:   []string{"python", "train.py"},
			slice2:   []string{"java", "Main.java"},
			expected: 0.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := calc.calculateSliceSimilarity(tt.slice1, tt.slice2)
			assert.Equal(t, tt.expected, score)
		})
	}
}

func TestSimilarityCalculator_CalculateMapSimilarity(t *testing.T) {
	calc := NewSimilarityCalculator()

	keyVars := []string{"FRAMEWORK", "WORLD_SIZE"}

	tests := []struct {
		name     string
		map1     map[string]string
		map2     map[string]string
		keyVars  []string
		expected float64
	}{
		{
			name: "all key vars match",
			map1: map[string]string{
				"FRAMEWORK":  "PyTorch",
				"WORLD_SIZE": "8",
				"OTHER":      "value1",
			},
			map2: map[string]string{
				"FRAMEWORK":  "PyTorch",
				"WORLD_SIZE": "8",
				"OTHER":      "value2",
			},
			keyVars:  keyVars,
			expected: 1.0,
		},
		{
			name: "partial key vars match",
			map1: map[string]string{
				"FRAMEWORK":  "PyTorch",
				"WORLD_SIZE": "8",
			},
			map2: map[string]string{
				"FRAMEWORK":  "PyTorch",
				"WORLD_SIZE": "16",
			},
			keyVars:  keyVars,
			expected: 0.5, // 1 out of 2 key vars match
		},
		{
			name: "no key vars match",
			map1: map[string]string{
				"FRAMEWORK":  "PyTorch",
				"WORLD_SIZE": "8",
			},
			map2: map[string]string{
				"FRAMEWORK":  "TensorFlow",
				"WORLD_SIZE": "16",
			},
			keyVars:  keyVars,
			expected: 0.0,
		},
		{
			name:     "both empty",
			map1:     map[string]string{},
			map2:     map[string]string{},
			keyVars:  keyVars,
			expected: 1.0,
		},
		{
			name: "no key vars defined",
			map1: map[string]string{
				"A": "1",
				"B": "2",
			},
			map2: map[string]string{
				"A": "1",
				"C": "3",
			},
			keyVars:  []string{},
			expected: 0.33, // Jaccard: 1 common out of 3 total (approximation)
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := calc.calculateMapSimilarity(tt.map1, tt.map2, tt.keyVars)
			assert.InDelta(t, tt.expected, score, 0.01)
		})
	}
}

func TestDefaultSimilarityWeights(t *testing.T) {
	weights := DefaultSimilarityWeights()

	// Verify default values
	assert.Equal(t, 0.40, weights.Image)
	assert.Equal(t, 0.25, weights.Command)
	assert.Equal(t, 0.20, weights.Env)
	assert.Equal(t, 0.10, weights.Args)
	assert.Equal(t, 0.05, weights.Labels)

	// Verify sum equals 1.0
	sum := weights.Image + weights.Command + weights.Env + weights.Args + weights.Labels
	assert.Equal(t, 1.0, sum)
}

func TestSimilarityCalculator_WeightedScore(t *testing.T) {
	calc := NewSimilarityCalculator()

	// Create two workloads where only image matches perfectly
	sig1 := &coreModel.WorkloadSignature{
		Image:     "registry.example.com/primus:v1.2.3",
		Command:   []string{},
		Args:      []string{},
		Env:       map[string]string{},
		Labels:    map[string]string{},
		Namespace: "default",
	}

	sig2 := &coreModel.WorkloadSignature{
		Image:     "registry.example.com/primus:v1.2.3",
		Command:   []string{},
		Args:      []string{},
		Env:       map[string]string{},
		Labels:    map[string]string{},
		Namespace: "default",
	}

	result := calc.CalculateSimilarity(sig1, sig2)

	// Score should be 1.0 * 0.40 (image) + 1.0 * 0.25 (command empty) + 1.0 * 0.20 (env empty) + 1.0 * 0.10 (args empty) + 1.0 * 0.05 (labels empty) = 1.0
	assert.Equal(t, 1.0, result.Score)
	assert.Equal(t, 1.0, result.Details.ImageScore)
	assert.Equal(t, 1.0, result.Details.CommandScore)
	assert.Equal(t, 1.0, result.Details.EnvScore)
	assert.Equal(t, 1.0, result.Details.ArgsScore)
	assert.Equal(t, 1.0, result.Details.LabelScore)
}
