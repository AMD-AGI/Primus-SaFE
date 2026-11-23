/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package workload

import (
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

func TestFormatRuntime(t *testing.T) {
	for seconds, want := range map[int]string{0: "unlimited", -1: "unlimited", 90: "1m30s", 1800: "30m",
		3600: "1h", 5400: "1h30m", 10800: "3h", 3661: "1h1m1s"} {
		assert.Equal(t, want, FormatRuntime(seconds), "%d", seconds)
	}
}

func TestNewRuntimeExtendedCondition(t *testing.T) {
	c := NewRuntimeExtendedCondition("runtime set to 3h by alice, above the qos limit 1h")
	assert.Equal(t, string(v1.RuntimeExtended), c.Type)
	assert.Equal(t, metav1.ConditionTrue, c.Status)
	assert.Equal(t, "Extended", c.Reason)
	assert.Equal(t, "runtime set to 3h by alice, above the qos limit 1h", c.Message)
	assert.False(t, c.LastTransitionTime.IsZero())
}
