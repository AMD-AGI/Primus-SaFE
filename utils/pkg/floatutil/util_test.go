/*
 * Copyright © AMD. 2025-2026. All rights reserved.
 */

package floatutil

import (
	"testing"

	"gotest.tools/assert"
)

func TestFloatEqual(t *testing.T) {
	type Ratio struct {
		ratio float64 `json:"targetRatio"`
	}
	r := Ratio{}
	assert.Equal(t, FloatEqual(r.ratio, 0), true)
}
