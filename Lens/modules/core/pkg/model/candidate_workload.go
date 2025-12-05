package model

import "time"

// CandidateWorkload represents a candidate workload for similarity matching
type CandidateWorkload struct {
	WorkloadUID string                `json:"workload_uid"`
	Detection   *FrameworkDetection   `json:"detection"`
	Signature   *WorkloadSignature    `json:"signature"`
	CreatedAt   time.Time             `json:"created_at"`
	Confidence  float64               `json:"confidence"`
}

