// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package model

import "time"

// ReuseInfo represents reuse information (embedded in FrameworkDetection)
type ReuseInfo struct {
	ReusedFrom         string    `json:"reused_from"`          // Source workload_uid
	ReusedAt           time.Time `json:"reused_at"`            // Reuse timestamp
	SimilarityScore    float64   `json:"similarity_score"`     // Similarity score
	OriginalConfidence float64   `json:"original_confidence"`  // Original confidence
}

