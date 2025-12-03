/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package utils

import (
	"context"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	commonworkload "github.com/AMD-AIG-AIMA/SAFE/common/pkg/workload"
)

// IsUnrecoverableError checks if an error is non-retryable based on error type.
func IsUnrecoverableError(err error) bool {
	if err == nil {
		return false
	}
	if commonerrors.IsBadRequest(err) || commonerrors.IsInternal(err) || commonerrors.IsNotFound(err) {
		return true
	}
	if apierrors.IsNotFound(err) {
		return true
	}
	return false
}

// FindCondition finds the condition of the workload and checks if there is one with the same type and reason.
func FindCondition(workload *v1.Workload, condition *metav1.Condition) *metav1.Condition {
	for i, currentCondition := range workload.Status.Conditions {
		if currentCondition.Type == condition.Type && currentCondition.Reason == condition.Reason {
			return &workload.Status.Conditions[i]
		}
	}
	return nil
}

// NewCondition creates a new condition with the specified type, message, and reason.
func NewCondition(conditionType, message, reason string) *metav1.Condition {
	return &metav1.Condition{
		Type:               conditionType,
		Status:             metav1.ConditionTrue,
		Message:            message,
		Reason:             reason,
		LastTransitionTime: metav1.NewTime(time.Now().UTC()),
	}
}

// SetWorkloadFailed sets the workload to failed state and updates its status.
// It adds a failure condition and sets the end time if not already set.
func SetWorkloadFailed(ctx context.Context, cli client.Client, workload *v1.Workload, message string) error {
	workload.Status.Phase = v1.WorkloadFailed
	if workload.Status.EndTime == nil {
		workload.Status.EndTime = &metav1.Time{Time: time.Now().UTC()}
	}

	dispatchCount := v1.GetWorkloadDispatchCnt(workload)
	if dispatchCount == 0 {
		// Default to 1 for initial failure
		dispatchCount = 1
	}
	condition := NewCondition(string(v1.AdminFailed), message, commonworkload.GenerateDispatchReason(dispatchCount))
	workload.Status.Conditions = append(workload.Status.Conditions, *condition)
	if err := cli.Status().Update(ctx, workload); err != nil {
		klog.ErrorS(err, "failed to update workload status", "name", workload.Name)
		return err
	}
	return nil
}
