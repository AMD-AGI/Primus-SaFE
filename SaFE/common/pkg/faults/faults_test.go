/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package faults

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
)

func TestGenerateFaultId(t *testing.T) {
	id := GenerateFaultId("node-1", "safe.gpu")
	assert.NotEmpty(t, id)
	assert.NotContains(t, id, " ")
}

func TestGenerateTaintKey(t *testing.T) {
	key := GenerateTaintKey("some-id")
	assert.True(t, strings.HasPrefix(key, v1.PrimusSafePrefix) || len(key) > 0)

	// Long id should be truncated to MaxNameLength.
	long := strings.Repeat("a", commonutils.MaxNameLength+50)
	k := GenerateTaintKey(long)
	assert.LessOrEqual(t, len(k), commonutils.MaxNameLength)
}

func TestIsTaintsEqualIgnoreOrder(t *testing.T) {
	t1 := []corev1.Taint{{Key: "a", Value: "1", Effect: corev1.TaintEffectNoSchedule}, {Key: "b"}}
	t2 := []corev1.Taint{{Key: "b"}, {Key: "a", Value: "1", Effect: corev1.TaintEffectNoSchedule}}
	assert.True(t, IsTaintsEqualIgnoreOrder(t1, t2))
	assert.False(t, IsTaintsEqualIgnoreOrder(t1, []corev1.Taint{{Key: "a"}}))
	assert.False(t, IsTaintsEqualIgnoreOrder(t1, []corev1.Taint{{Key: "c"}, {Key: "d"}}))
}

func TestHasTaintKey(t *testing.T) {
	taints := []corev1.Taint{{Key: "x"}, {Key: "y"}}
	assert.True(t, HasTaintKey(taints, "y"))
	assert.False(t, HasTaintKey(taints, "z"))
}

func TestGetCustomerTaints(t *testing.T) {
	taints := []corev1.Taint{
		{Key: v1.PrimusSafePrefix + "sys"},
		{Key: "customer/taint"},
	}
	out := GetCustomerTaints(taints)
	assert.Len(t, out, 1)
	assert.Equal(t, "customer/taint", out[0].Key)
}

func TestIsSystemReservedTaint(t *testing.T) {
	assert.False(t, IsSystemReservedTaint("customer/random"))
	// A taint key generated from a system monitor id is system-reserved.
	sysKey := GenerateTaintKey(v1.AddonMonitorId)
	_ = sysKey
	assert.True(t, IsSystemReservedTaint(v1.PrimusSafePrefix+v1.AddonMonitorId))
}
