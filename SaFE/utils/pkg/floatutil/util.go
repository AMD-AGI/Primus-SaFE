/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package floatutil

import (
	"math"
)

const (
	epsilon = 1e-9
)

// FloatEqual compares two float64 values for equality within a small epsilon tolerance.
// It returns true if the absolute difference between f1 and f2 is less than or equal to epsilon (1e-9).
func FloatEqual(f1, f2 float64) bool {
	if math.Abs(f1-f2) <= epsilon {
		return true
	}
	return false
}
