/*
 * Copyright © AMD. 2025-2026. All rights reserved.
 */

package floatutil

import (
	"math"
)

const (
	epsilon = 1e-9
)

func FloatEqual(f1, f2 float64) bool {
	if math.Abs(f1-f2) < epsilon {
		return true
	}
	return false
}
