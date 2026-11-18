/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package scheduler

import (
	"context"
	"strings"
	"sync"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonqos "github.com/AMD-AIG-AIMA/SAFE/common/pkg/qos"
)

// qosWorkload returns a queued workload of one user that asks for gpus cards on cluster c1.
func qosWorkload(name, user, gpus string) *v1.Workload {
	return &v1.Workload{
		ObjectMeta: metav1.ObjectMeta{Name: name, Labels: map[string]string{
			v1.ClusterIdLabel: "c1",
			v1.UserIdLabel:    user,
		}},
		Spec: v1.WorkloadSpec{
			GroupVersionKind: v1.GroupVersionKind{Kind: common.PytorchJobKind},
			Resources: []v1.WorkloadResource{{
				Replica: 1, CPU: "1", Memory: "1Gi", GPU: gpus, GPUName: common.AmdGpu,
			}},
		},
	}
}

func TestUserQuotaHoldDoesNotBlockQueue(t *testing.T) {
	reason, err := commonqos.QuotaReason("alice", "c1", nil, nil, commonqos.Limits{})
	if err != nil || reason != "" {
		t.Fatalf("setup: %q %v", reason, err)
	}
	held := commonqos.UserQuotaReasonPrefix + ": gpu used 8 + request 8 > limit 8 (user alice, cluster c1)"
	if !isSkippableReason(held) {
		t.Fatalf("a user quota hold must not stop the queue: %q", held)
	}
	failed := commonqos.UserQuotaCheckFailedPrefix + ": invalid cpu qos limit"
	if !isSkippableReason(failed) {
		t.Fatalf("a failed quota check must not stop the queue: %q", failed)
	}
	if isSkippableReason("insufficient resources") {
		t.Fatal("a capacity shortage keeps the queue order")
	}
}

func TestUserQuotaCountsWorkloadsAdmittedThisPass(t *testing.T) {
	qos := &v1.Qos{
		ObjectMeta: metav1.ObjectMeta{Name: "c1"},
		Spec:       v1.QosSpec{MaxResources: &v1.ResourceCap{GPU: "8"}},
	}
	first := qosWorkload("first", "alice", "8")
	second := qosWorkload("second", "alice", "8")
	cl := ctrlfake.NewClientBuilder().WithScheme(ttlScheme(t)).WithObjects(qos, first, second).Build()
	r := &SchedulerReconciler{Client: cl}

	reason := r.userQuotaHold(context.Background(), second, []*v1.Workload{first})
	if !strings.HasPrefix(reason, commonqos.UserQuotaReasonPrefix) {
		t.Fatalf("the workload admitted earlier in this pass must count, reason %q", reason)
	}
	reason = r.userQuotaHold(context.Background(), second, []*v1.Workload{qosWorkload("bob-job", "bob", "8")})
	if reason != "" {
		t.Fatalf("another user's admission does not count, reason %q", reason)
	}
}

func TestUserQuotaErrorHoldsOnlyThatWorkload(t *testing.T) {
	qos := &v1.Qos{
		ObjectMeta: metav1.ObjectMeta{Name: "c1"},
		Spec:       v1.QosSpec{MaxResources: &v1.ResourceCap{CPU: "abc"}},
	}
	w := qosWorkload("w", "alice", "1")
	cl := ctrlfake.NewClientBuilder().WithScheme(ttlScheme(t)).WithObjects(qos, w).Build()
	r := &SchedulerReconciler{Client: cl}

	reason := r.userQuotaHold(context.Background(), w, nil)
	if !strings.HasPrefix(reason, commonqos.UserQuotaCheckFailedPrefix) {
		t.Fatalf("a bad qos value holds this workload with a reason, got %q", reason)
	}
}

func TestConcurrentQuotaReserveAdmitsOne(t *testing.T) {
	qos := &v1.Qos{
		ObjectMeta: metav1.ObjectMeta{Name: "c1"},
		Spec:       v1.QosSpec{MaxResources: &v1.ResourceCap{GPU: "8"}},
	}
	first := qosWorkload("first", "alice", "8")
	second := qosWorkload("second", "alice", "8")
	cl := ctrlfake.NewClientBuilder().WithScheme(ttlScheme(t)).WithObjects(qos, first, second).Build()
	r := &SchedulerReconciler{Client: cl}

	start := make(chan struct{})
	reasons := make([]string, 2)
	var wg sync.WaitGroup
	for i, w := range []*v1.Workload{first, second} {
		wg.Add(1)
		go func(i int, w *v1.Workload) {
			defer wg.Done()
			<-start
			reasons[i] = r.reserveUserQuota(context.Background(), w)
		}(i, w)
	}
	close(start)
	wg.Wait()

	admitted := 0
	for _, reason := range reasons {
		if reason == "" {
			admitted++
			continue
		}
		if !strings.HasPrefix(reason, commonqos.UserQuotaReasonPrefix) {
			t.Fatalf("reason %q", reason)
		}
	}
	if admitted != 1 {
		t.Fatalf("one admission must be reserved, got %d (%q)", admitted, reasons)
	}
	r.releaseUserQuota(first)
	r.releaseUserQuota(second)
	if reason := r.reserveUserQuota(context.Background(), second); reason != "" {
		t.Fatalf("releasing the reservation frees the quota, reason %q", reason)
	}
}

func TestQuotaReservationDropsWhenWorkloadIsGone(t *testing.T) {
	qos := &v1.Qos{
		ObjectMeta: metav1.ObjectMeta{Name: "c1"},
		Spec:       v1.QosSpec{MaxResources: &v1.ResourceCap{GPU: "8"}},
	}
	first := qosWorkload("first", "alice", "8")
	second := qosWorkload("second", "alice", "8")
	cl := ctrlfake.NewClientBuilder().WithScheme(ttlScheme(t)).WithObjects(qos, first, second).Build()
	r := &SchedulerReconciler{Client: cl}
	ctx := context.Background()
	if reason := r.reserveUserQuota(ctx, first); reason != "" {
		t.Fatalf("first fits the empty quota, reason %q", reason)
	}
	if err := cl.Delete(ctx, first); err != nil {
		t.Fatal(err)
	}
	if reason := r.reserveUserQuota(ctx, second); reason != "" {
		t.Fatalf("a deleted workload must not keep its reservation, reason %q", reason)
	}
}

func TestReserveUserQuotaEdges(t *testing.T) {
	cl := ctrlfake.NewClientBuilder().WithScheme(ttlScheme(t)).Build()
	r := &SchedulerReconciler{Client: cl}
	ctx := context.Background()
	plain := qosWorkload("plain", "alice", "1")
	if reason := r.reserveUserQuota(ctx, plain); reason != "" {
		t.Fatalf("no qos means no ceiling, reason %q", reason)
	}
	plain.Labels[v1.UserIdLabel] = ""
	if reason := r.reserveUserQuota(ctx, plain); reason != "" {
		t.Fatalf("a workload without a user is not counted, reason %q", reason)
	}
	bad := &v1.Qos{
		ObjectMeta: metav1.ObjectMeta{Name: "c1"},
		Spec:       v1.QosSpec{MaxResources: &v1.ResourceCap{CPU: "abc"}},
	}
	cl = ctrlfake.NewClientBuilder().WithScheme(ttlScheme(t)).WithObjects(bad, qosWorkload("w", "alice", "1")).Build()
	r = &SchedulerReconciler{Client: cl}
	reason := r.reserveUserQuota(ctx, qosWorkload("w", "alice", "1"))
	if !strings.HasPrefix(reason, commonqos.UserQuotaCheckFailedPrefix) {
		t.Fatalf("a bad ceiling holds the workload, reason %q", reason)
	}
	r.releaseUserQuota(nil)
}
