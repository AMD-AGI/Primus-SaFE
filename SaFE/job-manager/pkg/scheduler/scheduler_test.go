/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package scheduler

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/agiledragon/gomonkey/v2"
	testifyassert "github.com/stretchr/testify/assert"

	"gotest.tools/assert"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/event"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/controller"
	commonworkload "github.com/AMD-AIG-AIMA/SAFE/common/pkg/workload"
	jobutils "github.com/AMD-AIG-AIMA/SAFE/job-manager/pkg/utils"
)

func schedWorkload(name string) *v1.Workload {
	return &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: name}}
}

func TestGetWorkspaceNotFound(t *testing.T) {
	cl := ctrlfake.NewClientBuilder().WithScheme(ttlScheme(t)).Build()
	r := &SchedulerReconciler{Client: cl}
	ws, err := r.getWorkspace(context.Background(), "missing")
	assert.NilError(t, err)
	assert.Assert(t, ws == nil)
}

func TestGetWorkspaceFound(t *testing.T) {
	ws := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws"}}
	cl := ctrlfake.NewClientBuilder().WithScheme(ttlScheme(t)).WithObjects(ws).Build()
	r := &SchedulerReconciler{Client: cl}
	got, err := r.getWorkspace(context.Background(), "ws")
	assert.NilError(t, err)
	assert.Assert(t, got != nil)
	assert.Equal(t, got.Name, "ws")
}

func TestUpdateStatusAlreadyScheduled(t *testing.T) {
	w := schedWorkload("w")
	// Pre-add the scheduled condition so updateStatus is a no-op.
	reason := commonworkload.GenerateDispatchReason(v1.GetWorkloadDispatchCnt(w) + 1)
	w.Status.Conditions = []metav1.Condition{{
		Type:   string(v1.AdminScheduled),
		Reason: reason,
	}}
	r := &SchedulerReconciler{}
	err := r.updateStatus(context.Background(), w)
	assert.NilError(t, err)
}

func TestUpdateStatusPatch(t *testing.T) {
	w := schedWorkload("w")
	cl := ctrlfake.NewClientBuilder().
		WithScheme(ttlScheme(t)).
		WithObjects(w).
		WithStatusSubresource(&v1.Workload{}).
		Build()
	r := &SchedulerReconciler{Client: cl}
	err := r.updateStatus(context.Background(), w)
	assert.NilError(t, err)
}

func TestMarkAsScheduled(t *testing.T) {
	w := schedWorkload("w")
	cl := ctrlfake.NewClientBuilder().
		WithScheme(ttlScheme(t)).
		WithObjects(w).
		WithStatusSubresource(&v1.Workload{}).
		Build()
	r := &SchedulerReconciler{Client: cl}
	err := r.markAsScheduled(context.Background(), w)
	assert.NilError(t, err)
	assert.Assert(t, v1.IsWorkloadScheduled(w))
}

func TestCascadeStopChildrenEmpty(t *testing.T) {
	owner := schedWorkload("owner")
	cl := ctrlfake.NewClientBuilder().WithScheme(ttlScheme(t)).Build()
	r := &SchedulerReconciler{Client: cl}
	err := r.cascadeStopChildren(context.Background(), owner)
	assert.NilError(t, err)
}

func TestCascadeStopChildrenWithChild(t *testing.T) {
	owner := schedWorkload("owner")
	child := schedWorkload("child")
	child.Labels = map[string]string{v1.OwnerLabel: "owner"}
	cl := ctrlfake.NewClientBuilder().
		WithScheme(ttlScheme(t)).
		WithObjects(child).
		WithStatusSubresource(&v1.Workload{}).
		Build()
	r := &SchedulerReconciler{Client: cl}
	err := r.cascadeStopChildren(context.Background(), owner)
	assert.NilError(t, err)
}

func TestSetDependencyPhaseSucceeded(t *testing.T) {
	dep := schedWorkload("dep")
	depended := schedWorkload("depended")
	depended.Spec.Dependencies = []string{"dep"}
	dep.Status.Phase = v1.WorkloadSucceeded
	cl := ctrlfake.NewClientBuilder().
		WithScheme(ttlScheme(t)).
		WithObjects(depended).
		WithStatusSubresource(&v1.Workload{}).
		Build()
	r := &SchedulerReconciler{Client: cl}
	err := r.setDependencyPhase(context.Background(), dep, depended)
	assert.NilError(t, err)
}

func TestSetDependencyPhaseFailed(t *testing.T) {
	dep := schedWorkload("dep")
	depended := schedWorkload("depended")
	depended.Spec.Dependencies = []string{"dep"}
	dep.Status.Phase = v1.WorkloadFailed
	cl := ctrlfake.NewClientBuilder().
		WithScheme(ttlScheme(t)).
		WithObjects(depended).
		WithStatusSubresource(&v1.Workload{}).
		Build()
	r := &SchedulerReconciler{Client: cl}
	err := r.setDependencyPhase(context.Background(), dep, depended)
	assert.NilError(t, err)
}

func TestUpdateDependentsPhaseNotEnded(t *testing.T) {
	// A not-ended workload returns nil without listing dependents.
	r := &SchedulerReconciler{}
	err := r.updateDependentsPhase(context.Background(), schedWorkload("w"))
	assert.NilError(t, err)
}

func TestCheckWorkloadDependenciesNoDeps(t *testing.T) {
	cl := ctrlfake.NewClientBuilder().WithScheme(ttlScheme(t)).Build()
	r := &SchedulerReconciler{Client: cl}
	ready, err := r.checkWorkloadDependencies(context.Background(), schedWorkload("w"))
	assert.NilError(t, err)
	assert.Equal(t, ready, true)
}

func TestCheckWorkloadDependenciesNotFound(t *testing.T) {
	w := schedWorkload("w")
	w.Spec.Dependencies = []string{"missing-dep"}
	cl := ctrlfake.NewClientBuilder().
		WithScheme(ttlScheme(t)).
		WithObjects(w).
		WithStatusSubresource(&v1.Workload{}).
		Build()
	r := &SchedulerReconciler{Client: cl}
	// Dependency not found -> the workload is marked failed; when that status
	// update succeeds the function returns (false, nil) per its control flow.
	ready, err := r.checkWorkloadDependencies(context.Background(), w)
	assert.Equal(t, ready, false)
	assert.NilError(t, err)
	assert.Equal(t, w.Status.Phase, v1.WorkloadFailed)
}

// --- merged from scheduler_keyed_test.go ---

// countingHandler records how many times Do runs per workspace key.
type countingHandler struct {
	client client.Client
	mu     sync.Mutex
	counts map[string]int
}

func (h *countingHandler) Do(_ context.Context, m *SchedulerMessage) (ctrlruntime.Result, error) {
	key := schedulerMessageKey(m)
	h.mu.Lock()
	h.counts[key]++
	h.mu.Unlock()
	// Exercise the real path: missing workspace is a no-op error-wise.
	r := &SchedulerReconciler{Client: h.client}
	return r.Do(context.Background(), m)
}

func (h *countingHandler) get(key string) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.counts[key]
}

// TestSchedulerKeyedCoalesce verifies duplicate workspace events collapse to one run.
func TestSchedulerKeyedCoalesce(t *testing.T) {
	cl := ctrlfake.NewClientBuilder().WithScheme(ttlScheme(t)).Build()
	h := &countingHandler{client: cl, counts: make(map[string]int)}
	c := controller.NewKeyedController[*SchedulerMessage](h, schedulerMessageKey, nil, 1)

	msg := &SchedulerMessage{ClusterId: "c1", WorkspaceId: "ws1"}
	c.Add(msg)
	c.Add(msg)
	c.Add(msg)
	assert.Equal(t, 1, c.GetQueueSize())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c.Run(ctx)
	deadline := time.Now().Add(2 * time.Second)
	for c.GetQueueSize() > 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	time.Sleep(50 * time.Millisecond)
	cancel()
	time.Sleep(50 * time.Millisecond)

	assert.Equal(t, 1, h.get("c1|ws1"))
}

// TestSchedulerKeyedParallelWorkspaces verifies distinct workspace keys fan out.
func TestSchedulerKeyedParallelWorkspaces(t *testing.T) {
	cl := ctrlfake.NewClientBuilder().WithScheme(ttlScheme(t)).Build()
	var maxParallel int32
	var inFlight int32
	h := &countingHandler{client: cl, counts: make(map[string]int)}
	wrapped := &parallelHandler{
		inner:       h,
		inFlight:    &inFlight,
		maxParallel: &maxParallel,
	}
	c := controller.NewKeyedController[*SchedulerMessage](wrapped, schedulerMessageKey, nil, 4)

	for i := 0; i < 4; i++ {
		c.Add(&SchedulerMessage{ClusterId: "c1", WorkspaceId: "ws-a"})
		c.Add(&SchedulerMessage{ClusterId: "c1", WorkspaceId: "ws-b"})
		c.Add(&SchedulerMessage{ClusterId: "c1", WorkspaceId: "ws-c"})
		c.Add(&SchedulerMessage{ClusterId: "c1", WorkspaceId: "ws-d"})
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	for i := 0; i < 4; i++ {
		c.Run(ctx)
	}
	deadline := time.Now().Add(3 * time.Second)
	for c.GetQueueSize() > 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	time.Sleep(100 * time.Millisecond)
	cancel()
	time.Sleep(50 * time.Millisecond)

	testifyassert.GreaterOrEqual(t, int(atomic.LoadInt32(&maxParallel)), 2)
	assert.Equal(t, 1, h.get("c1|ws-a"))
	assert.Equal(t, 1, h.get("c1|ws-b"))
	assert.Equal(t, 1, h.get("c1|ws-c"))
	assert.Equal(t, 1, h.get("c1|ws-d"))
}

type parallelHandler struct {
	inner       *countingHandler
	inFlight    *int32
	maxParallel *int32
}

func (h *parallelHandler) Do(ctx context.Context, m *SchedulerMessage) (ctrlruntime.Result, error) {
	cur := atomic.AddInt32(h.inFlight, 1)
	defer atomic.AddInt32(h.inFlight, -1)
	for {
		old := atomic.LoadInt32(h.maxParallel)
		if cur <= old || atomic.CompareAndSwapInt32(h.maxParallel, old, cur) {
			break
		}
	}
	time.Sleep(30 * time.Millisecond)
	return h.inner.Do(ctx, m)
}

func TestSchedulerMessageKey(t *testing.T) {
	assert.Equal(t, "cluster-a|ws-1", schedulerMessageKey(&SchedulerMessage{ClusterId: "cluster-a", WorkspaceId: "ws-1"}))
}

// --- merged from scheduler_monkey_test.go ---

// TestScheduleWorkloadsFullPath patches the heavy helpers so scheduleWorkloads runs its
// orchestration: one scheduling workload that passes canScheduleWorkload is marked scheduled.
func TestScheduleWorkloadsFullPath(t *testing.T) {
	ws := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws"}}
	cl := ctrlfake.NewClientBuilder().WithScheme(ttlScheme(t)).WithObjects(ws).Build()
	r := &SchedulerReconciler{Client: cl}

	w := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "w"}}
	w.Spec.Resources = []v1.WorkloadResource{{Replica: 1, CPU: "1", Memory: "1Gi"}}

	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyPrivateMethod(reflect.TypeOf(&SchedulerReconciler{}), "getUnfinishedWorkloads",
		func(_ *SchedulerReconciler, _ context.Context, _ *v1.Workspace) ([]*v1.Workload, []*v1.Workload, error) {
			return []*v1.Workload{w}, nil, nil
		})
	patches.ApplyPrivateMethod(reflect.TypeOf(&SchedulerReconciler{}), "getLeftTotalResources",
		func(_ *SchedulerReconciler, _ context.Context, _ *v1.Workspace, _ []*v1.Workload) (corev1.ResourceList, corev1.ResourceList, error) {
			return corev1.ResourceList{}, corev1.ResourceList{}, nil
		})
	patches.ApplyPrivateMethod(reflect.TypeOf(&SchedulerReconciler{}), "canScheduleWorkload",
		func(_ *SchedulerReconciler, _ context.Context, _ *v1.Workload, _ *v1.Workspace, _ []*v1.Workload, _, _ corev1.ResourceList) (bool, string, error) {
			return true, "", nil
		})
	patches.ApplyPrivateMethod(reflect.TypeOf(&SchedulerReconciler{}), "markAsScheduled",
		func(_ *SchedulerReconciler, _ context.Context, _ *v1.Workload) error { return nil })

	err := r.scheduleWorkloads(context.Background(), &SchedulerMessage{WorkspaceId: "ws"})
	assert.NilError(t, err)
}

// --- merged from scheduler_predicate_test.go ---

func TestRelevantChangePredicateCreate(t *testing.T) {
	cl := ctrlfake.NewClientBuilder().WithScheme(ttlScheme(t)).Build()
	r := &SchedulerReconciler{Client: cl}
	p := r.relevantChangePredicate()

	// Workload without cronjobs -> Create returns true.
	assert.Equal(t, p.Create(event.CreateEvent{Object: &v1.Workload{}}), true)

	// Wrong type -> false.
	assert.Equal(t, p.Create(event.CreateEvent{Object: &corev1.Pod{}}), false)
}

func TestRelevantChangePredicateUpdate(t *testing.T) {
	cl := ctrlfake.NewClientBuilder().WithScheme(ttlScheme(t)).Build()
	r := &SchedulerReconciler{Client: cl}
	p := r.relevantChangePredicate()

	// Wrong types -> false.
	assert.Equal(t, p.Update(event.UpdateEvent{ObjectOld: &corev1.Pod{}, ObjectNew: &corev1.Pod{}}), false)

	// Scheduled-state change -> true.
	oldW := &v1.Workload{}
	newW := &v1.Workload{ObjectMeta: metav1.ObjectMeta{
		Annotations: map[string]string{v1.WorkloadScheduledAnnotation: "true"},
	}}
	assert.Equal(t, p.Update(event.UpdateEvent{ObjectOld: oldW, ObjectNew: newW}), true)
}

func TestNotifyDependentWorkspacesEmpty(t *testing.T) {
	cl := ctrlfake.NewClientBuilder().WithScheme(ttlScheme(t)).Build()
	r := &SchedulerReconciler{Client: cl}
	// No dependent workspaces -> no-op, no panic.
	r.notifyDependentWorkspaces(&v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "w"}})
}

func TestHandleWorkspaceEventWrongTypes(t *testing.T) {
	r := &SchedulerReconciler{}
	h := r.handleWorkspaceEvent()
	// Create/Delete are no-ops; Update with wrong types returns without enqueue.
	h.Create(context.Background(), event.CreateEvent{Object: &v1.Workspace{}}, nil)
	h.Update(context.Background(), event.UpdateEvent{ObjectOld: &corev1.Pod{}, ObjectNew: &corev1.Pod{}}, nil)
	h.Delete(context.Background(), event.DeleteEvent{Object: &v1.Workspace{}}, nil)
}

func TestPreemptNotEnabled(t *testing.T) {
	cl := ctrlfake.NewClientBuilder().WithScheme(ttlScheme(t)).Build()
	r := &SchedulerReconciler{Client: cl}
	// Request workload without preempt enabled -> no preemption.
	ok, err := r.preempt(context.Background(), &v1.Workload{}, nil, corev1.ResourceList{})
	assert.NilError(t, err)
	assert.Equal(t, ok, false)
}

// --- merged from scheduler_reconcile_test.go ---

func TestSchedulerReconcileNotFound(t *testing.T) {
	cl := ctrlfake.NewClientBuilder().WithScheme(ttlScheme(t)).Build()
	r := &SchedulerReconciler{Client: cl}
	_, err := r.Reconcile(context.Background(), ctrlruntime.Request{
		NamespacedName: ctrlclient.ObjectKey{Name: "missing"},
	})
	assert.NilError(t, err)
}

func TestSchedulerReconcileDefaultWorkspace(t *testing.T) {
	// A workload in the default namespace short-circuits to nil.
	w := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "w"}}
	w.Spec.Workspace = corev1.NamespaceDefault
	cl := ctrlfake.NewClientBuilder().WithScheme(ttlScheme(t)).WithObjects(w).Build()
	r := &SchedulerReconciler{Client: cl}
	_, err := r.Reconcile(context.Background(), ctrlruntime.Request{
		NamespacedName: ctrlclient.ObjectKey{Name: "w"},
	})
	assert.NilError(t, err)
}

func TestDeleteRelatedSecretsEmpty(t *testing.T) {
	cl := ctrlfake.NewClientBuilder().WithScheme(ttlScheme(t)).Build()
	r := &SchedulerReconciler{Client: cl}
	err := r.deleteRelatedSecrets(context.Background(), schedWorkload("w"))
	assert.NilError(t, err)
}

func TestDeleteRelatedSecretsWithSecret(t *testing.T) {
	sec := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{
		Name:      "tok",
		Namespace: common.PrimusSafeNamespace,
		Labels:    map[string]string{v1.OwnerLabel: "w"},
	}}
	cl := ctrlfake.NewClientBuilder().WithScheme(ttlScheme(t)).WithObjects(sec).Build()
	r := &SchedulerReconciler{Client: cl}
	err := r.deleteRelatedSecrets(context.Background(), schedWorkload("w"))
	assert.NilError(t, err)

	// Secret should be deleted.
	got := &corev1.Secret{}
	gErr := cl.Get(context.Background(), ctrlclient.ObjectKey{Name: "tok", Namespace: common.PrimusSafeNamespace}, got)
	assert.Assert(t, gErr != nil)
}

func TestDeleteRelatedEphemeralRunnerNoId(t *testing.T) {
	r := &SchedulerReconciler{}
	// No scale-runner id label -> no-op, no client interaction.
	err := r.deleteRelatedEphemeralRunner(context.Background(), schedWorkload("w"), nil)
	assert.NilError(t, err)
}

func TestScheduleWorkloadsWorkspaceNotFound(t *testing.T) {
	cl := ctrlfake.NewClientBuilder().WithScheme(ttlScheme(t)).Build()
	r := &SchedulerReconciler{Client: cl}
	err := r.scheduleWorkloads(context.Background(), &SchedulerMessage{WorkspaceId: "missing"})
	assert.NilError(t, err)
}

func TestSchedulerDo(t *testing.T) {
	cl := ctrlfake.NewClientBuilder().WithScheme(ttlScheme(t)).Build()
	r := &SchedulerReconciler{Client: cl}
	// Workspace not found -> scheduleWorkloads returns nil -> Do returns nil.
	_, err := r.Do(context.Background(), &SchedulerMessage{WorkspaceId: "missing"})
	assert.NilError(t, err)
}

// --- merged from scheduler_resources_test.go ---

func TestGetLeftTotalResourcesEmpty(t *testing.T) {
	cl := ctrlfake.NewClientBuilder().WithScheme(ttlScheme(t)).Build()
	r := &SchedulerReconciler{Client: cl}

	ws := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws"}}
	ws.Status.AvailableResources = corev1.ResourceList{
		corev1.ResourceCPU:    resource.MustParse("10"),
		corev1.ResourceMemory: resource.MustParse("20Gi"),
	}
	ws.Status.TotalResources = corev1.ResourceList{
		corev1.ResourceCPU:    resource.MustParse("10"),
		corev1.ResourceMemory: resource.MustParse("20Gi"),
	}

	avail, total, err := r.getLeftTotalResources(context.Background(), ws, nil)
	assert.NilError(t, err)
	assert.Assert(t, avail != nil)
	assert.Assert(t, total != nil)
}

func TestGetLeftTotalResourcesWithPendingWorkload(t *testing.T) {
	cl := ctrlfake.NewClientBuilder().WithScheme(ttlScheme(t)).Build()
	r := &SchedulerReconciler{Client: cl}

	ws := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws"}}
	ws.Status.AvailableResources = corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("10")}
	ws.Status.TotalResources = corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("10")}

	w := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "w"}}
	w.Spec.Resources = []v1.WorkloadResource{{Replica: 1, CPU: "2", Memory: "4Gi"}}
	// Pending (not running) -> uses GetTotalResourceList.
	avail, _, err := r.getLeftTotalResources(context.Background(), ws, []*v1.Workload{w})
	assert.NilError(t, err)
	assert.Assert(t, avail != nil)
}

func TestGetUnfinishedWorkloadsEmpty(t *testing.T) {
	cl := ctrlfake.NewClientBuilder().WithScheme(ttlScheme(t)).Build()
	r := &SchedulerReconciler{Client: cl}

	ws := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws"}}
	ws.Spec.Cluster = "cluster-1"
	scheduling, scheduled, err := r.getUnfinishedWorkloads(context.Background(), ws)
	assert.NilError(t, err)
	assert.Equal(t, len(scheduling), 0)
	assert.Equal(t, len(scheduled), 0)
}

func TestSchedulerMessage_DoesNotEraseConcurrentFailure(t *testing.T) {
	w := schedWorkload("example-workload")
	w.Spec.Workspace = "test-workspace"
	w.Status.Phase = v1.WorkloadPending
	cli := ctrlfake.NewClientBuilder().WithScheme(ttlScheme(t)).WithStatusSubresource(w).Build()
	assert.NilError(t, cli.Create(context.Background(), w))
	stale := w.DeepCopy()
	assert.NilError(t, jobutils.SetWorkloadFailed(context.Background(), cli, w, "registration failed"))
	r := &SchedulerReconciler{Client: cli}
	assert.Assert(t, apierrors.IsConflict(r.updateStatus(context.Background(), stale)))
	r.updateUnScheduled(context.Background(), []*v1.Workload{stale}, map[string]string{w.Name: "capacity unavailable"}, &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: w.Spec.Workspace}})
	fresh := &v1.Workload{}
	assert.NilError(t, cli.Get(context.Background(), client.ObjectKeyFromObject(w), fresh))
	assert.Equal(t, fresh.Status.Phase, v1.WorkloadFailed)
	assert.Equal(t, fresh.Status.Message, "registration failed")
	assert.NilError(t, r.updateStatus(context.Background(), fresh))
	r.updateUnScheduled(context.Background(), []*v1.Workload{fresh}, map[string]string{}, &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: w.Spec.Workspace}})
	assert.NilError(t, cli.Get(context.Background(), client.ObjectKeyFromObject(w), fresh))
	assert.Equal(t, fresh.Status.Message, "registration failed")
}

func TestUpdateUnScheduledRetriesQueuePositionAfterConflict(t *testing.T) {
	w := schedWorkload("queued-wl")
	w.Spec.Workspace = "ws-q"
	w.Status.Phase = v1.WorkloadPending
	cli := ctrlfake.NewClientBuilder().WithScheme(ttlScheme(t)).WithStatusSubresource(w).Build()
	assert.NilError(t, cli.Create(context.Background(), w))
	stale := w.DeepCopy()
	assert.NilError(t, jobutils.PatchWorkloadStatusFields(context.Background(), cli, w, map[string]any{
		"message": "touched",
	}))
	r := &SchedulerReconciler{Client: cli}
	ws := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws-q"}}
	reason := "In queue - waiting for earlier or higher priority workloads"
	r.updateUnScheduled(context.Background(), []*v1.Workload{stale}, map[string]string{w.Name: reason}, ws)
	fresh := &v1.Workload{}
	assert.NilError(t, cli.Get(context.Background(), client.ObjectKeyFromObject(w), fresh))
	assert.Equal(t, fresh.Status.Phase, v1.WorkloadPending)
	assert.Equal(t, fresh.Status.QueuePosition, 1)
	assert.Equal(t, fresh.Status.Message, reason)
	found := false
	for _, c := range fresh.Status.Conditions {
		if c.Type == string(v1.AdminScheduling) {
			found = true
			assert.Equal(t, c.Message, reason)
		}
	}
	assert.Assert(t, found, "AdminScheduling condition required after a conflict retry")
}

func TestUpdateUnScheduledConflictWithScheduledStatusKeepsItOutOfQueue(t *testing.T) {
	first := schedWorkload("scheduled-wl")
	first.Spec.Workspace = "ws-q"
	first.Status.Phase = v1.WorkloadPending
	second := schedWorkload("waiting-wl")
	second.Spec.Workspace = "ws-q"
	second.Status.Phase = v1.WorkloadPending
	cli := ctrlfake.NewClientBuilder().WithScheme(ttlScheme(t)).WithStatusSubresource(first, second).Build()
	assert.NilError(t, cli.Create(context.Background(), first))
	assert.NilError(t, cli.Create(context.Background(), second))
	stale := first.DeepCopy()
	r := &SchedulerReconciler{Client: cli}
	assert.NilError(t, r.updateStatus(context.Background(), first))
	assert.Assert(t, !v1.IsWorkloadScheduled(first))
	ws := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws-q"}}
	r.updateUnScheduled(context.Background(), []*v1.Workload{stale, second}, map[string]string{
		first.Name:  "In queue - waiting for external capacity",
		second.Name: "In queue - waiting for earlier or higher priority workloads",
	}, ws)

	storedFirst := &v1.Workload{}
	assert.NilError(t, cli.Get(context.Background(), client.ObjectKeyFromObject(first), storedFirst))
	assert.Equal(t, storedFirst.Status.QueuePosition, 0)
	assert.Equal(t, storedFirst.Status.Message, "")
	scheduled, scheduling := 0, 0
	for _, c := range storedFirst.Status.Conditions {
		if c.Type == string(v1.AdminScheduled) {
			scheduled++
		}
		if c.Type == string(v1.AdminScheduling) {
			scheduling++
		}
	}
	assert.Equal(t, scheduled, 1)
	assert.Equal(t, scheduling, 0)

	storedSecond := &v1.Workload{}
	assert.NilError(t, cli.Get(context.Background(), client.ObjectKeyFromObject(second), storedSecond))
	assert.Equal(t, storedSecond.Status.QueuePosition, 1)
}

func testSchedulerQueue(t *testing.T) *controller.KeyedController[*SchedulerMessage] {
	t.Helper()
	q := controller.NewKeyedController[*SchedulerMessage](nil, schedulerMessageKey, nil, 1)
	t.Cleanup(q.ShutDown)
	return q
}

func TestRequeueWorkspace(t *testing.T) {
	r := &SchedulerReconciler{}
	r.requeueWorkspace(nil)
	r.requeueWorkspace(&v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws-q"}})

	r.KeyedController = testSchedulerQueue(t)
	r.requeueWorkspace(nil)
	r.requeueWorkspace(&v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws-q"}, Spec: v1.WorkspaceSpec{Cluster: "cluster-1"}})
	assert.Equal(t, r.GetQueueSize(), 0)
}

func TestWriteQueueStatusNonConflictErrorRequeues(t *testing.T) {
	w := schedWorkload("patch-err")
	w.Spec.Workspace = "ws-q"
	w.Status.Phase = v1.WorkloadPending
	base := ctrlfake.NewClientBuilder().WithScheme(ttlScheme(t)).WithStatusSubresource(w).Build()
	assert.NilError(t, base.Create(context.Background(), w))
	r := &SchedulerReconciler{
		Client: interceptor.NewClient(base, interceptor.Funcs{SubResourcePatch: func(context.Context, client.Client, string, client.Object, client.Patch, ...client.SubResourcePatchOption) error {
			return fmt.Errorf("status unavailable")
		}}),
		KeyedController: testSchedulerQueue(t),
	}
	r.updateUnScheduled(context.Background(), []*v1.Workload{w}, map[string]string{w.Name: "In queue - waiting"}, &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws-q"}, Spec: v1.WorkspaceSpec{Cluster: "cluster-1"}})
	stored := &v1.Workload{}
	assert.NilError(t, base.Get(context.Background(), client.ObjectKeyFromObject(w), stored))
	assert.Equal(t, stored.Status.QueuePosition, 0)
}

func TestWriteQueueStatusGoneAfterConflict(t *testing.T) {
	w := schedWorkload("gone-wl")
	w.Spec.Workspace = "ws-q"
	w.Status.Phase = v1.WorkloadPending
	waiting := schedWorkload("still-waiting")
	waiting.Spec.Workspace = "ws-q"
	waiting.Status.Phase = v1.WorkloadPending
	base := ctrlfake.NewClientBuilder().WithScheme(ttlScheme(t)).WithStatusSubresource(w, waiting).Build()
	assert.NilError(t, base.Create(context.Background(), w))
	assert.NilError(t, base.Create(context.Background(), waiting))
	r := &SchedulerReconciler{Client: interceptor.NewClient(base, interceptor.Funcs{SubResourcePatch: func(ctx context.Context, cli client.Client, sub string, obj client.Object, patch client.Patch, opts ...client.SubResourcePatchOption) error {
		if obj.GetName() == w.Name {
			assert.NilError(t, base.Delete(ctx, w))
			return apierrors.NewConflict(schema.GroupResource{Group: "amd.com", Resource: "workloads"}, w.Name, errors.New("object was modified"))
		}
		return cli.SubResource(sub).Patch(ctx, obj, patch, opts...)
	}})}
	r.updateUnScheduled(context.Background(), []*v1.Workload{w, waiting}, map[string]string{
		w.Name:       "In queue - waiting",
		waiting.Name: "In queue - waiting for earlier or higher priority workloads",
	}, &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws-q"}})
	err := base.Get(context.Background(), client.ObjectKeyFromObject(w), &v1.Workload{})
	assert.Assert(t, apierrors.IsNotFound(err))
	stored := &v1.Workload{}
	assert.NilError(t, base.Get(context.Background(), client.ObjectKeyFromObject(waiting), stored))
	assert.Equal(t, stored.Status.QueuePosition, 1)
}

func TestWriteQueueStatusGetFailureRequeues(t *testing.T) {
	w := schedWorkload("get-err")
	w.Spec.Workspace = "ws-q"
	w.Status.Phase = v1.WorkloadPending
	base := ctrlfake.NewClientBuilder().WithScheme(ttlScheme(t)).WithStatusSubresource(w).Build()
	assert.NilError(t, base.Create(context.Background(), w))
	r := &SchedulerReconciler{
		Client: interceptor.NewClient(base, interceptor.Funcs{
			SubResourcePatch: func(context.Context, client.Client, string, client.Object, client.Patch, ...client.SubResourcePatchOption) error {
				return apierrors.NewConflict(schema.GroupResource{Group: "amd.com", Resource: "workloads"}, w.Name, errors.New("object was modified"))
			},
			Get: func(context.Context, client.WithWatch, client.ObjectKey, client.Object, ...client.GetOption) error {
				return fmt.Errorf("apiserver down")
			},
		}),
		KeyedController: testSchedulerQueue(t),
	}
	r.updateUnScheduled(context.Background(), []*v1.Workload{w}, map[string]string{w.Name: "In queue - waiting"}, &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws-q"}, Spec: v1.WorkspaceSpec{Cluster: "cluster-1"}})
	stored := &v1.Workload{}
	assert.NilError(t, base.Get(context.Background(), client.ObjectKeyFromObject(w), stored))
	assert.Equal(t, stored.Status.QueuePosition, 0)
}

func TestWriteQueueStatusSecondPatchErrorRequeues(t *testing.T) {
	w := schedWorkload("second-patch")
	w.Spec.Workspace = "ws-q"
	w.Status.Phase = v1.WorkloadPending
	base := ctrlfake.NewClientBuilder().WithScheme(ttlScheme(t)).WithStatusSubresource(w).Build()
	assert.NilError(t, base.Create(context.Background(), w))
	patches := 0
	r := &SchedulerReconciler{
		Client: interceptor.NewClient(base, interceptor.Funcs{SubResourcePatch: func(ctx context.Context, cli client.Client, sub string, obj client.Object, patch client.Patch, opts ...client.SubResourcePatchOption) error {
			patches++
			if patches == 1 {
				return apierrors.NewConflict(schema.GroupResource{Group: "amd.com", Resource: "workloads"}, w.Name, errors.New("object was modified"))
			}
			return fmt.Errorf("status unavailable")
		}}),
		KeyedController: testSchedulerQueue(t),
	}
	r.updateUnScheduled(context.Background(), []*v1.Workload{w}, map[string]string{w.Name: "In queue - waiting"}, &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws-q"}, Spec: v1.WorkspaceSpec{Cluster: "cluster-1"}})
	assert.Equal(t, patches, 2)
	stored := &v1.Workload{}
	assert.NilError(t, base.Get(context.Background(), client.ObjectKeyFromObject(w), stored))
	assert.Equal(t, stored.Status.QueuePosition, 0)
}

func TestWriteQueueStatusReloadAlreadyCurrent(t *testing.T) {
	w := schedWorkload("current-wl")
	w.Spec.Workspace = "ws-q"
	w.Status.Phase = v1.WorkloadPending
	base := ctrlfake.NewClientBuilder().WithScheme(ttlScheme(t)).WithStatusSubresource(w).Build()
	assert.NilError(t, base.Create(context.Background(), w))
	stale := w.DeepCopy()
	r := &SchedulerReconciler{Client: base}
	ws := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws-q"}}
	reason := "In queue - waiting for earlier or higher priority workloads"
	r.updateUnScheduled(context.Background(), []*v1.Workload{w}, map[string]string{w.Name: reason}, ws)
	r.updateUnScheduled(context.Background(), []*v1.Workload{stale}, map[string]string{w.Name: reason}, ws)
	stored := &v1.Workload{}
	assert.NilError(t, base.Get(context.Background(), client.ObjectKeyFromObject(w), stored))
	assert.Equal(t, stored.Status.QueuePosition, 1)
	assert.Equal(t, stored.Status.Message, reason)
	count := 0
	for _, c := range stored.Status.Conditions {
		if c.Type == string(v1.AdminScheduling) {
			count++
		}
	}
	assert.Equal(t, count, 1)
}

func TestUpdateUnScheduledWritesAdminScheduling(t *testing.T) {
	w := schedWorkload("pending-wl")
	w.Spec.Workspace = "ws-q"
	w.Status.Phase = v1.WorkloadPending
	cli := ctrlfake.NewClientBuilder().WithScheme(ttlScheme(t)).WithStatusSubresource(w).WithObjects(w).Build()
	r := &SchedulerReconciler{Client: cli}
	ws := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws-q"}}

	r.updateUnScheduled(context.Background(), []*v1.Workload{w},
		map[string]string{w.Name: "In queue - waiting for external capacity"}, ws)
	stored := &v1.Workload{}
	assert.NilError(t, cli.Get(context.Background(), client.ObjectKeyFromObject(w), stored))
	assert.Equal(t, stored.Status.Message, "In queue - waiting for external capacity")
	assert.Equal(t, stored.Status.QueuePosition, 1)
	found := false
	for _, c := range stored.Status.Conditions {
		if c.Type == string(v1.AdminScheduling) {
			found = true
			assert.Equal(t, c.Message, "In queue - waiting for external capacity")
		}
	}
	assert.Assert(t, found, "AdminScheduling condition required before AdminScheduled")

	// Idempotent: same inputs must not invent a second AdminScheduling entry.
	r.updateUnScheduled(context.Background(), []*v1.Workload{stored},
		map[string]string{w.Name: "In queue - waiting for external capacity"}, ws)
	assert.NilError(t, cli.Get(context.Background(), client.ObjectKeyFromObject(w), stored))
	count := 0
	for _, c := range stored.Status.Conditions {
		if c.Type == string(v1.AdminScheduling) {
			count++
		}
	}
	assert.Equal(t, count, 1)
}

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
