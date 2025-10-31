/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package faults

import (
	"strings"

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
	return stringutil.NormalizeName(key)
}

// GetIdByTaintKey extracts the ID from a taint key by removing the PrimusSafe prefix.
func GetIdByTaintKey(taintKey string) string {
	if len(taintKey) <= len(v1.PrimusSafePrefix) {
		return ""
	}
	return taintKey[len(v1.PrimusSafePrefix):]
}

// IsTaintsEqualIgnoreOrder returns true if the condition is met.
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

// HasPrimusSafeTaint checks if any taint in the provided taints slice has a key that starts with the PrimusSafe prefix.
func HasPrimusSafeTaint(taints []corev1.Taint) bool {
	for _, t := range taints {
		if strings.HasPrefix(t.Key, v1.PrimusSafePrefix) {
			return true
		}
	}
	return false
}
