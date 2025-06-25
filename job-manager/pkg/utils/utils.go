/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
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

func IsNonRetryableError(err error) bool {
	if err == nil {
		return false
	}
	if commonerrors.IsBadRequest(err) || commonerrors.IsInternal(err) || commonerrors.IsNotFound(err) {
		return true
	}
	if apierrors.IsForbidden(err) || apierrors.IsNotFound(err) {
		return true
	}
	return false
}

// Find the condition of the workload and check if there is one with the same type and reason.
func FindCondition(workload *v1.Workload, cond *metav1.Condition) *metav1.Condition {
	for i, currentCondition := range workload.Status.Conditions {
		if currentCondition.Type == cond.Type && currentCondition.Reason == cond.Reason {
			return &workload.Status.Conditions[i]
		}
	}
	return nil
}

func NewCondition(conditionType, message, reason string) *metav1.Condition {
	result := &metav1.Condition{
		Type:               conditionType,
		Status:             metav1.ConditionTrue,
		Message:            message,
		Reason:             reason,
		LastTransitionTime: metav1.NewTime(time.Now().UTC()),
	}
	return result
}

func SetWorkloadFailed(ctx context.Context, cli client.Client, workload *v1.Workload, message string) error {
	workload.Status.Phase = v1.WorkloadFailed
	if workload.Status.EndTime == nil {
		workload.Status.EndTime = &metav1.Time{Time: time.Now().UTC()}
	}

	cnt := v1.GetWorkloadDispatchCnt(workload)
	if cnt == 0 {
		cnt = 1
	}
	cond := NewCondition(string(v1.AdminFailed), message, commonworkload.GenerateDispatchReason(cnt))
	workload.Status.Conditions = append(workload.Status.Conditions, *cond)
	if err := cli.Status().Update(ctx, workload); err != nil {
		klog.ErrorS(err, "failed to update workload status", "name", workload.Name)
		return err
	}
	return nil
}
