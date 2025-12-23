package model

// SimilarityResult represents the result of similarity calculation
type SimilarityResult struct {
	WorkloadUID string             `json:"workload_uid"`
	Score       float64            `json:"score"`   // Overall similarity score 0-1
	Details     *SimilarityDetails `json:"details"` // Detailed scores
}

// SimilarityDetails represents detailed similarity scores
type SimilarityDetails struct {
	ImageScore   float64 `json:"image_score"`   // Image similarity
	CommandScore float64 `json:"command_score"` // Command similarity
	EnvScore     float64 `json:"env_score"`     // Environment variables similarity
	ArgsScore    float64 `json:"args_score"`    // Arguments similarity
	LabelScore   float64 `json:"label_score"`   // Labels similarity
}

