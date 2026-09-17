/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package workload

import (
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

const WorkloadFailureFallback = "Workload failed; no failure details were reported."

func WorkloadFailureConditionIndex(conditions []metav1.Condition, dispatchCount int) int {
	reason := GenerateDispatchReason(max(1, dispatchCount))
	selected := -1
	for i, condition := range conditions {
		if condition.Status != metav1.ConditionTrue || condition.Reason != reason ||
			(condition.Type != string(v1.AdminFailed) && condition.Type != string(v1.K8sFailed)) ||
			strings.TrimSpace(condition.Message) == "" {
			continue
		}
		if selected < 0 || !condition.LastTransitionTime.Before(&conditions[selected].LastTransitionTime) {
			selected = i
		}
	}
	return selected
}

func GetWorkloadFailureMessage(conditions []metav1.Condition, dispatchCount int) string {
	if index := WorkloadFailureConditionIndex(conditions, dispatchCount); index >= 0 {
		return strings.TrimSpace(conditions[index].Message)
	}
	return WorkloadFailureFallback
}
