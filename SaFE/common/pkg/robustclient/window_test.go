/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package robustclient

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestWindowCovered(t *testing.T) {
	assert.True(t, WindowCovered(54, 60))
	assert.False(t, WindowCovered(53, 60))
	assert.False(t, WindowCovered(0, 0), "no expected samples")
}

func TestWindowAverage(t *testing.T) {
	var absent *WorkloadGPUWindow
	_, ok := absent.Average()
	assert.False(t, ok)

	w := &WorkloadGPUWindow{Overall: GPUWindowOverall{Max: 90, Avg: 40, Samples: 4, ExpectedSamples: 5}}
	_, ok = w.Average()
	assert.False(t, ok, "too few samples")

	w.Overall.ExpectedSamples = 0
	_, ok = w.Average()
	assert.False(t, ok, "no expected samples")

	w.Overall.Samples, w.Overall.ExpectedSamples = 5, 5
	avg, ok := w.Average()
	assert.True(t, ok)
	assert.Equal(t, 40.0, avg, "the average, not the peak")
}

func TestWindowFor(t *testing.T) {
	assert.Equal(t, "5m", WindowFor(5*time.Minute))
	assert.Equal(t, "2m", WindowFor(90*time.Second))
	assert.Equal(t, "1m", WindowFor(10*time.Second))
}
