/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package faults

import (
	"strings"

	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/slice"
	corev1 "k8s.io/api/core/v1"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/sets"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/stringutil"
)

// GenerateFaultId creates a normalized fault identifier by input admin node ID and monitor ID.
func GenerateFaultId(adminNodeId, monitorId string) string {
	id := adminNodeId + "-" + monitorId
	return stringutil.NormalizeName(id)
}

// GenerateTaintKey creates a taint key by prefixing the given ID with PrimusSafe prefix.
func GenerateTaintKey(id string) string {
	key := v1.PrimusSafePrefix + id
	key = stringutil.NormalizeName(key)
	if len(key) > commonutils.MaxNameLength {
		key = key[0:commonutils.MaxNameLength]
	}
	return key
}

// IsTaintsEqualIgnoreOrder compares two taint slices for equality, ignoring the order of elements.
// Returns true if both slices contain the same taints (same key, value, and effect), false otherwise.
func IsTaintsEqualIgnoreOrder(taints1, taints2 []corev1.Taint) bool {
	if len(taints1) != len(taints2) {
		return false
	}
	taintsMap := sets.NewSet()
	for _, t := range taints1 {
		taintsMap.Insert(t.ToString())
	}
	for _, t := range taints2 {
		if !taintsMap.Has(t.ToString()) {
			return false
		}
	}
	return true
}

// HasTaintKey checks if any taint in the provided taints slice has a key that exactly matches the specified key.
func HasTaintKey(taints []corev1.Taint, key string) bool {
	for _, t := range taints {
		if t.Key == key {
			return true
		}
	}
	return false
}

// GetCustomerTaints get all user-generated taints in the taints slice
func GetCustomerTaints(taints []corev1.Taint) []corev1.Taint {
	result := make([]corev1.Taint, 0, len(taints))
	for i, t := range taints {
		if !strings.HasPrefix(t.Key, v1.PrimusSafePrefix) {
			result = append(result, taints[i])
		}
	}
	return result
}

// IsSystemReservedTaint checks if the given taint key belongs to system-reserved taints
// These are special taints managed internally by the system for specific purposes like monitoring
func IsSystemReservedTaint(taintKey string) bool {
	id := v1.GetIdByTaintKey(taintKey)
	allIds := []string{v1.AddonMonitorId, v1.StickyNodesMonitorId}
	return slice.Contains(allIds, id)
}
