// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package tensorboard

import (
	"time"
)

// StreamConfig configures streaming behavior
type StreamConfig struct {
	// Poll interval for checking new data
	PollInterval time.Duration

	// Chunk size for each read
	ChunkSize int64

	// Whether to follow file rotations
	FollowRotation bool
}

// DefaultStreamConfig returns default streaming configuration
func DefaultStreamConfig() *StreamConfig {
	return &StreamConfig{
		PollInterval:   5 * time.Second,
		ChunkSize:      64 * 1024, // 64KB per read
		FollowRotation: true,
	}
}
