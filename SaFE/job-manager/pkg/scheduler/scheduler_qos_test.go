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
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonqos "github.com/AMD-AIG-AIMA/SAFE/common/pkg/qos"
)

// qosWorkload returns a queued workload of one user that asks for gpus cards in workspace c1.
func qosWorkload(name, user, gpus string) *v1.Workload {
	return &v1.Workload{
		ObjectMeta: metav1.ObjectMeta{Name: name, Labels: map[string]string{
			v1.WorkspaceIdLabel: "c1",
			v1.UserIdLabel:      user,
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
	held := commonqos.UserQuotaReasonPrefix + ": gpu used 8 + request 8 > limit 8 (user alice, workspace c1)"
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
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	older := qosWorkload("older", "alice", "8")
	older.CreationTimestamp = metav1.NewTime(base)
	later := qosWorkload("later", "alice", "8")
	later.CreationTimestamp = metav1.NewTime(base.Add(time.Minute))
	cl := ctrlfake.NewClientBuilder().WithScheme(ttlScheme(t)).WithObjects(qos, older, later).Build()
	r := &SchedulerReconciler{Client: cl}

	reason := r.userQuotaHold(context.Background(), older, []*v1.Workload{later})
	if !strings.HasPrefix(reason, commonqos.UserQuotaReasonPrefix) {
		t.Fatalf("a workload admitted earlier in this pass must count, reason %q", reason)
	}
	reason = r.userQuotaHold(context.Background(), older, []*v1.Workload{qosWorkload("bob-job", "bob", "8")})
	if reason != "" {
		t.Fatalf("a later pending workload and another user's admission do not count, reason %q", reason)
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

func TestUserQuotaCountsOlderPendingWork(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	qos := &v1.Qos{
		ObjectMeta: metav1.ObjectMeta{Name: "c1"},
		Spec:       v1.QosSpec{MaxResources: &v1.ResourceCap{GPU: "8"}},
	}
	older := qosWorkload("older", "alice", "8")
	older.CreationTimestamp = metav1.NewTime(base)
	newer := qosWorkload("newer", "alice", "8")
	newer.CreationTimestamp = metav1.NewTime(base.Add(time.Minute))
	newer.Spec.Priority = 100
	cl := ctrlfake.NewClientBuilder().WithScheme(ttlScheme(t)).WithObjects(qos, older, newer).Build()
	r := &SchedulerReconciler{Client: cl}
	ctx := context.Background()

	reason := r.userQuotaHold(ctx, newer, nil)
	if !strings.HasPrefix(reason, commonqos.UserQuotaReasonPrefix) {
		t.Fatalf("an older pending workload must count, reason %q", reason)
	}
	if reason := r.userQuotaHold(ctx, older, nil); reason != "" {
		t.Fatalf("a newer pending workload must not block the older one, reason %q", reason)
	}
}

func TestUserQuotaSkipsQueuedWorkAboveTheCeiling(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	qos := &v1.Qos{
		ObjectMeta: metav1.ObjectMeta{Name: "c1"},
		Spec:       v1.QosSpec{MaxResources: &v1.ResourceCap{GPU: "4"}},
	}
	older := qosWorkload("older", "alice", "8")
	older.CreationTimestamp = metav1.NewTime(base)
	newer := qosWorkload("newer", "alice", "2")
	newer.CreationTimestamp = metav1.NewTime(base.Add(time.Minute))
	cl := ctrlfake.NewClientBuilder().WithScheme(ttlScheme(t)).WithObjects(qos, older, newer).Build()
	r := &SchedulerReconciler{Client: cl}

	if reason := r.userQuotaHold(context.Background(), newer, nil); reason != "" {
		t.Fatalf("a queued workload above the ceiling by itself must not count, reason %q", reason)
	}
}

func TestUserQuotaCountsAnOlderCron(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	qos := &v1.Qos{
		ObjectMeta: metav1.ObjectMeta{Name: "c1"},
		Spec:       v1.QosSpec{MaxResources: &v1.ResourceCap{GPU: "8"}},
	}
	cron := qosWorkload("cron", "alice", "8")
	cron.CreationTimestamp = metav1.NewTime(base)
	cron.Spec.CronJobs = []v1.CronJob{{Action: v1.CronStart, Schedule: "2026-06-01T00:00:00Z"}}
	later := qosWorkload("later", "alice", "1")
	later.CreationTimestamp = metav1.NewTime(base.Add(time.Minute))
	cl := ctrlfake.NewClientBuilder().WithScheme(ttlScheme(t)).WithObjects(qos, cron, later).Build()
	r := &SchedulerReconciler{Client: cl}

	reason := r.userQuotaHold(context.Background(), later, nil)
	if !strings.HasPrefix(reason, commonqos.UserQuotaReasonPrefix) {
		t.Fatalf("an older cron workload counts toward the ceiling, reason %q", reason)
	}
}

func TestUserQuotaSameTimeKeepsHigherPriorityThenName(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	qos := &v1.Qos{
		ObjectMeta: metav1.ObjectMeta{Name: "c1"},
		Spec:       v1.QosSpec{MaxResources: &v1.ResourceCap{GPU: "8"}},
	}
	high := qosWorkload("z-high", "alice", "8")
	high.CreationTimestamp = metav1.NewTime(base)
	high.Spec.Priority = 10
	low := qosWorkload("a-low", "alice", "8")
	low.CreationTimestamp = metav1.NewTime(base)
	low.Spec.Priority = 1
	cl := ctrlfake.NewClientBuilder().WithScheme(ttlScheme(t)).WithObjects(qos, high, low).Build()
	r := &SchedulerReconciler{Client: cl}
	ctx := context.Background()
	if reason := r.userQuotaHold(ctx, high, nil); reason != "" {
		t.Fatalf("the higher priority keeps the quota, reason %q", reason)
	}
	reason := r.userQuotaHold(ctx, low, nil)
	if !strings.HasPrefix(reason, commonqos.UserQuotaReasonPrefix) {
		t.Fatalf("the lower priority waits, reason %q", reason)
	}

	left := qosWorkload("m-left", "alice", "8")
	left.CreationTimestamp = metav1.NewTime(base)
	left.Spec.Priority = 5
	right := qosWorkload("m-right", "alice", "8")
	right.CreationTimestamp = metav1.NewTime(base)
	right.Spec.Priority = 5
	cl = ctrlfake.NewClientBuilder().WithScheme(ttlScheme(t)).WithObjects(qos, left, right).Build()
	r = &SchedulerReconciler{Client: cl}
	if reason := r.userQuotaHold(ctx, left, nil); reason != "" {
		t.Fatalf("the earlier name keeps the quota, reason %q", reason)
	}
	reason = r.userQuotaHold(ctx, right, nil)
	if !strings.HasPrefix(reason, commonqos.UserQuotaReasonPrefix) {
		t.Fatalf("the later name waits, reason %q", reason)
	}
}

func TestUserQuotaCountsScheduledWorkRegardlessOfOrder(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	qos := &v1.Qos{
		ObjectMeta: metav1.ObjectMeta{Name: "c1"},
		Spec:       v1.QosSpec{MaxResources: &v1.ResourceCap{GPU: "8"}},
	}
	older := qosWorkload("older", "alice", "8")
	older.CreationTimestamp = metav1.NewTime(base)
	running := qosWorkload("running", "alice", "8")
	running.CreationTimestamp = metav1.NewTime(base.Add(time.Minute))
	running.Annotations = map[string]string{v1.WorkloadScheduledAnnotation: "true"}
	cl := ctrlfake.NewClientBuilder().WithScheme(ttlScheme(t)).WithObjects(qos, older, running).Build()
	r := &SchedulerReconciler{Client: cl}

	reason := r.userQuotaHold(context.Background(), older, nil)
	if !strings.HasPrefix(reason, commonqos.UserQuotaReasonPrefix) {
		t.Fatalf("a running workload counts even when it is newer, reason %q", reason)
	}
}

func TestConcurrentQuotaChecksAdmitTheEarlierWorkload(t *testing.T) {
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
			reasons[i] = r.userQuotaHold(context.Background(), w, nil)
		}(i, w)
	}
	close(start)
	wg.Wait()

	if reasons[0] != "" {
		t.Fatalf("the earlier name is admitted, reason %q", reasons[0])
	}
	if !strings.HasPrefix(reasons[1], commonqos.UserQuotaReasonPrefix) {
		t.Fatalf("the later name waits, reason %q", reasons[1])
	}
}

func TestUserQuotaDropsWhenWorkloadIsGone(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	qos := &v1.Qos{
		ObjectMeta: metav1.ObjectMeta{Name: "c1"},
		Spec:       v1.QosSpec{MaxResources: &v1.ResourceCap{GPU: "8"}},
	}
	older := qosWorkload("older", "alice", "8")
	older.CreationTimestamp = metav1.NewTime(base)
	newer := qosWorkload("newer", "alice", "8")
	newer.CreationTimestamp = metav1.NewTime(base.Add(time.Minute))
	cl := ctrlfake.NewClientBuilder().WithScheme(ttlScheme(t)).WithObjects(qos, older, newer).Build()
	r := &SchedulerReconciler{Client: cl}
	ctx := context.Background()
	if reason := r.userQuotaHold(ctx, newer, nil); !strings.HasPrefix(reason, commonqos.UserQuotaReasonPrefix) {
		t.Fatalf("the older workload holds the quota, reason %q", reason)
	}
	if err := cl.Delete(ctx, older); err != nil {
		t.Fatal(err)
	}
	if reason := r.userQuotaHold(ctx, newer, nil); reason != "" {
		t.Fatalf("a deleted workload must leave the quota, reason %q", reason)
	}
}

func TestUserQuotaHoldEdges(t *testing.T) {
	cl := ctrlfake.NewClientBuilder().WithScheme(ttlScheme(t)).Build()
	r := &SchedulerReconciler{Client: cl}
	ctx := context.Background()
	plain := qosWorkload("plain", "alice", "1")
	if reason := r.userQuotaHold(ctx, plain, nil); reason != "" {
		t.Fatalf("no qos means no ceiling, reason %q", reason)
	}
	plain.Labels[v1.UserIdLabel] = ""
	if reason := r.userQuotaHold(ctx, plain, nil); reason != "" {
		t.Fatalf("a workload without a user is not counted, reason %q", reason)
	}
	bad := &v1.Qos{
		ObjectMeta: metav1.ObjectMeta{Name: "c1"},
		Spec:       v1.QosSpec{MaxResources: &v1.ResourceCap{CPU: "abc"}},
	}
	cl = ctrlfake.NewClientBuilder().WithScheme(ttlScheme(t)).WithObjects(bad, qosWorkload("w", "alice", "1")).Build()
	r = &SchedulerReconciler{Client: cl}
	reason := r.userQuotaHold(ctx, qosWorkload("w", "alice", "1"), nil)
	if !strings.HasPrefix(reason, commonqos.UserQuotaCheckFailedPrefix) {
		t.Fatalf("a bad ceiling holds the workload, reason %q", reason)
	}
}

func TestUserQuotaIgnoresAnotherWorkspace(t *testing.T) {
	qos := &v1.Qos{
		ObjectMeta: metav1.ObjectMeta{Name: "ws1"},
		Spec:       v1.QosSpec{MaxResources: &v1.ResourceCap{GPU: "8"}},
	}
	otherQos := &v1.Qos{
		ObjectMeta: metav1.ObjectMeta{Name: "ws2"},
		Spec:       v1.QosSpec{MaxResources: &v1.ResourceCap{GPU: "8"}},
	}
	running := qosWorkload("running", "alice", "8")
	running.Labels[v1.WorkspaceIdLabel] = "ws1"
	running.Annotations = map[string]string{v1.WorkloadScheduledAnnotation: "true"}
	other := qosWorkload("other", "alice", "8")
	other.Labels[v1.WorkspaceIdLabel] = "ws2"
	cl := ctrlfake.NewClientBuilder().WithScheme(ttlScheme(t)).WithObjects(qos, otherQos, running, other).Build()
	r := &SchedulerReconciler{Client: cl}

	if reason := r.userQuotaHold(context.Background(), other, nil); reason != "" {
		t.Fatalf("usage in another workspace does not count, reason %q", reason)
	}
	same := qosWorkload("same", "alice", "1")
	same.Labels[v1.WorkspaceIdLabel] = "ws1"
	if reason := r.userQuotaHold(context.Background(), same, nil); !strings.HasPrefix(reason, commonqos.UserQuotaReasonPrefix) {
		t.Fatalf("the same workspace counts, reason %q", reason)
	}
}
