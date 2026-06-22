/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package optimization

import (
	"testing"

	"gotest.tools/assert"
)

func TestRayCPUForTP(t *testing.T) {
	assert.Equal(t, rayCPUForTP(1), 12)
	assert.Equal(t, rayCPUForTP(2), 24)
	assert.Equal(t, rayCPUForTP(4), 48)
	assert.Equal(t, rayCPUForTP(8), 96)
	// Non-power-of-two TP scales linearly at 12 CPU per GPU.
	assert.Equal(t, rayCPUForTP(3), 36)
	assert.Equal(t, rayCPUForTP(5), 60)
	assert.Equal(t, rayCPUForTP(6), 72)
	assert.Equal(t, rayCPUForTP(7), 84)
	// Non-positive TP falls back to the fixed default.
	assert.Equal(t, rayCPUForTP(0), defaultRayCPU)
}

func TestRayMemoryForTP(t *testing.T) {
	assert.Equal(t, rayMemoryForTP(1), 128)
	assert.Equal(t, rayMemoryForTP(2), 256)
	assert.Equal(t, rayMemoryForTP(4), 512)
	assert.Equal(t, rayMemoryForTP(8), 1024)
	// Non-power-of-two TP scales linearly at 128Gi per GPU.
	assert.Equal(t, rayMemoryForTP(3), 384)
	assert.Equal(t, rayMemoryForTP(5), 640)
	assert.Equal(t, rayMemoryForTP(6), 768)
	assert.Equal(t, rayMemoryForTP(7), 896)
	// Non-positive TP falls back to the fixed default.
	assert.Equal(t, rayMemoryForTP(0), defaultRayMemoryGi)
}
