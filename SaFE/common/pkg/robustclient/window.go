/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package robustclient

import (
	"fmt"
	"time"
)

const (
	// GPUWindowStep is the resolution every GPU utilization window is read at.
	GPUWindowStep = "1m"
	// MinWindowCoverage is the share of the expected samples a window must hold
	// before its utilization is judged.
	MinWindowCoverage = 0.9
)

// WindowCovered reports whether samples cover enough of the expected samples.
func WindowCovered(samples, expected int) bool {
	return expected > 0 && float64(samples) >= MinWindowCoverage*float64(expected)
}

// Covered reports whether the window holds enough samples to be judged.
func (w *WorkloadGPUWindow) Covered() bool {
	return w != nil && WindowCovered(w.Overall.Samples, w.Overall.ExpectedSamples)
}

// Average returns the average utilization (percent) over the window, and false
// when the window is absent or not covered.
func (w *WorkloadGPUWindow) Average() (float64, bool) {
	if !w.Covered() {
		return 0, false
	}
	return w.Overall.Avg, true
}

// WindowFor returns the window covering d in whole steps, at least one step.
func WindowFor(d time.Duration) string {
	minutes := int((d + time.Minute - 1) / time.Minute)
	return fmt.Sprintf("%dm", max(1, minutes))
}
