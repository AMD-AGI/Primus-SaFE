/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package faults

import (
	corev1 "k8s.io/api/core/v1"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/sets"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/stringutil"
)

func GenerateFaultName(adminNodeName, monitorId string) string {
	name := adminNodeName + "-" + monitorId
	return stringutil.NormalizeName(name)
}

func GenerateTaintKey(id string) string {
	key := v1.PrimusSafePrefix + id
	return stringutil.NormalizeName(key)
}

func GetIdByTaintKey(taintKey string) string {
	if len(taintKey) <= len(v1.PrimusSafePrefix) {
		return ""
	}
	return taintKey[len(v1.PrimusSafePrefix):]
}

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

func HasTaintKey(taints []corev1.Taint, key string) bool {
	for _, t := range taints {
		if t.Key == key {
			return true
		}
	}
	return false
}
