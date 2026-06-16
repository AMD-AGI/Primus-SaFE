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
	assert.Equal(t, rayCPUForTP(3), defaultRayCPU)
}

func TestRayMemoryForTP(t *testing.T) {
	assert.Equal(t, rayMemoryForTP(1), 128)
	assert.Equal(t, rayMemoryForTP(2), 256)
	assert.Equal(t, rayMemoryForTP(4), 512)
	assert.Equal(t, rayMemoryForTP(8), 1024)
	assert.Equal(t, rayMemoryForTP(5), defaultRayMemoryGi)
}
