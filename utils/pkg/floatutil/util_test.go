/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package floatutil

import (
	"testing"

	"gotest.tools/assert"
)

func TestFloatEqual(t *testing.T) {
	type Ratio struct {
		ratio float64
	}
	r := Ratio{}
	assert.Equal(t, FloatEqual(r.ratio, 0), true)
}
