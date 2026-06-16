/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package scheduler

import (
	"context"
	"testing"

	"gotest.tools/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

func TestCanScheduleWorkloadEnoughQuota(t *testing.T) {
	cl := ctrlfake.NewClientBuilder().WithScheme(ttlScheme(t)).Build()
	r := &SchedulerReconciler{Client: cl}

	ws := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws"}}
	w := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "w"}}
	request := corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("2")}
	left := corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("10")}

	ok, reason, err := r.canScheduleWorkload(context.Background(), w, ws, nil, request, left)
	assert.NilError(t, err)
	assert.Equal(t, ok, true)
	assert.Equal(t, reason, "")
}

func TestCanScheduleWorkloadInsufficientNoPreempt(t *testing.T) {
	cl := ctrlfake.NewClientBuilder().WithScheme(ttlScheme(t)).Build()
	r := &SchedulerReconciler{Client: cl}

	ws := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws"}}
	w := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "w"}}
	request := corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("20")}
	left := corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("1")}

	ok, reason, err := r.canScheduleWorkload(context.Background(), w, ws, nil, request, left)
	assert.NilError(t, err)
	assert.Equal(t, ok, false)
	assert.Assert(t, reason != "")
}
