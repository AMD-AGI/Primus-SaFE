package framework

import (
	"strings"

	coreModel "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model"
)

// SimilarityCalculator calculates similarity between workloads
type SimilarityCalculator struct {
	// Weight configuration
	weights SimilarityWeights

	// List of key environment variables
	keyEnvVars []string

	// List of framework-related labels
	frameworkLabels []string
}

// SimilarityWeights represents the similarity weight configuration
type SimilarityWeights struct {
	Image   float64 `json:"image"`   // Image weight (default: 0.40)
	Command float64 `json:"command"` // Command weight (default: 0.25)
	Env     float64 `json:"env"`     // Environment variables weight (default: 0.20)
	Args    float64 `json:"args"`    // Arguments weight (default: 0.10)
	Labels  float64 `json:"labels"`  // Labels weight (default: 0.05)
}

// DefaultSimilarityWeights returns the default weights
func DefaultSimilarityWeights() SimilarityWeights {
	return SimilarityWeights{
		Image:   0.40,
		Command: 0.25,
		Env:     0.20,
		Args:    0.10,
		Labels:  0.05,
	}
}

// NewSimilarityCalculator creates a new similarity calculator
func NewSimilarityCalculator() *SimilarityCalculator {
	return &SimilarityCalculator{
		weights: DefaultSimilarityWeights(),
		keyEnvVars: []string{
			"FRAMEWORK", "TRAINING_FRAMEWORK",
			"DEEPSPEED_CONFIG", "MEGATRON_CONFIG", "PRIMUS_CONFIG",
			"WORLD_SIZE", "RANK", "LOCAL_RANK",
		},
		frameworkLabels: []string{
			"ai.amd.com/framework",
			"ai.amd.com/training-type",
		},
	}
}

// CalculateSimilarity calculates the similarity between two workloads (0-1)
func (c *SimilarityCalculator) CalculateSimilarity(
	w1, w2 *coreModel.WorkloadSignature,
) *coreModel.SimilarityResult {
	details := &coreModel.SimilarityDetails{}

	// 1. Image similarity
	details.ImageScore = c.calculateImageSimilarity(w1.Image, w2.Image)

	// 2. Command similarity
	details.CommandScore = c.calculateSliceSimilarity(w1.Command, w2.Command)

	// 3. Environment variables similarity
	details.EnvScore = c.calculateMapSimilarity(w1.Env, w2.Env, c.keyEnvVars)

	// 4. Arguments similarity
	details.ArgsScore = c.calculateSliceSimilarity(w1.Args, w2.Args)

	// 5. Labels similarity
	details.LabelScore = c.calculateMapSimilarity(w1.Labels, w2.Labels, c.frameworkLabels)

	// 6. Weighted sum
	totalScore := details.ImageScore*c.weights.Image +
		details.CommandScore*c.weights.Command +
		details.EnvScore*c.weights.Env +
		details.ArgsScore*c.weights.Args +
		details.LabelScore*c.weights.Labels

	return &coreModel.SimilarityResult{
		Score:   totalScore,
		Details: details,
	}
}

// calculateImageSimilarity calculates image similarity
func (c *SimilarityCalculator) calculateImageSimilarity(img1, img2 string) float64 {
	// Exact match
	if img1 == img2 {
		return 1.0
	}

	// Same image different tag
	if c.sameImageDifferentTag(img1, img2) {
		return 0.9
	}

	// Same repository different image
	if c.sameImageRepo(img1, img2) {
		return 0.5
	}

	return 0.0
}

// sameImageDifferentTag checks if two images are the same but with different tags
func (c *SimilarityCalculator) sameImageDifferentTag(img1, img2 string) bool {
	// registry.example.com/primus:v1.2.3 vs registry.example.com/primus:v1.2.4
	parts1 := strings.Split(img1, ":")
	parts2 := strings.Split(img2, ":")

	if len(parts1) != 2 || len(parts2) != 2 {
		return false
	}

	return parts1[0] == parts2[0] && parts1[1] != parts2[1]
}

// sameImageRepo checks if two images are from the same repository
func (c *SimilarityCalculator) sameImageRepo(img1, img2 string) bool {
	repo1 := ExtractImageRepo(img1)
	repo2 := ExtractImageRepo(img2)

	// Extract repository domain
	// registry.example.com/primus -> registry.example.com
	parts1 := strings.Split(repo1, "/")
	parts2 := strings.Split(repo2, "/")

	if len(parts1) > 0 && len(parts2) > 0 {
		return parts1[0] == parts2[0]
	}

	return false
}

// calculateSliceSimilarity calculates the similarity between string slices
func (c *SimilarityCalculator) calculateSliceSimilarity(s1, s2 []string) float64 {
	if len(s1) == 0 && len(s2) == 0 {
		return 1.0
	}

	if len(s1) == 0 || len(s2) == 0 {
		return 0.0
	}

	// Check for exact match
	if c.slicesEqual(s1, s2) {
		return 1.0
	}

	// Jaccard similarity (intersection / union)
	set1 := c.toSet(s1)
	set2 := c.toSet(s2)

	intersection := 0
	for item := range set1 {
		if set2[item] {
			intersection++
		}
	}

	union := len(set1) + len(set2) - intersection
	if union == 0 {
		return 0.0
	}

	return float64(intersection) / float64(union)
}

// calculateMapSimilarity calculates map similarity (focusing on key fields)
func (c *SimilarityCalculator) calculateMapSimilarity(
	m1, m2 map[string]string,
	keyFields []string,
) float64 {
	if len(m1) == 0 && len(m2) == 0 {
		return 1.0
	}

	if len(keyFields) == 0 {
		// No key fields defined, use Jaccard similarity
		return c.calculateMapJaccardSimilarity(m1, m2)
	}

	// Key field matching rate
	matched := 0
	total := 0

	for _, key := range keyFields {
		v1, ok1 := m1[key]
		v2, ok2 := m2[key]

		if ok1 || ok2 {
			total++
			if ok1 && ok2 && v1 == v2 {
				matched++
			}
		}
	}

	if total == 0 {
		return 1.0 // Neither has key fields, consider them the same
	}

	return float64(matched) / float64(total)
}

// calculateMapJaccardSimilarity calculates the Jaccard similarity of maps
func (c *SimilarityCalculator) calculateMapJaccardSimilarity(
	m1, m2 map[string]string,
) float64 {
	if len(m1) == 0 && len(m2) == 0 {
		return 1.0
	}

	matched := 0
	for k, v1 := range m1 {
		if v2, ok := m2[k]; ok && v1 == v2 {
			matched++
		}
	}

	union := len(m1) + len(m2) - matched
	if union == 0 {
		return 0.0
	}

	return float64(matched) / float64(union)
}

// Helper methods
func (c *SimilarityCalculator) slicesEqual(s1, s2 []string) bool {
	if len(s1) != len(s2) {
		return false
	}
	for i := range s1 {
		if s1[i] != s2[i] {
			return false
		}
	}
	return true
}

func (c *SimilarityCalculator) toSet(slice []string) map[string]bool {
	set := make(map[string]bool)
	for _, item := range slice {
		set[item] = true
	}
	return set
}
