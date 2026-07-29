/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package scheduler

import (
	"context"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/agiledragon/gomonkey/v2"
	testifyassert "github.com/stretchr/testify/assert"

	"gotest.tools/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/event"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/controller"
	commonworkload "github.com/AMD-AIG-AIMA/SAFE/common/pkg/workload"
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
