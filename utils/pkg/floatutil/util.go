/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
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
