/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package node

import (
	"context"
	"fmt"
	"runtime"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	testifyassert "github.com/stretchr/testify/assert"

	"gotest.tools/assert"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes/fake"
	ktesting "k8s.io/client-go/testing"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	"github.com/AMD-AIG-AIMA/SAFE/node-agent/pkg/types"
)

func genNode() *corev1.Node {
	return &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
			Labels: map[string]string{
				common.AMDGpuIdentification: "true",
			},
		},
		Status: corev1.NodeStatus{
			Allocatable: corev1.ResourceList{
				common.AmdGpu: resource.MustParse("8"),
			},
		},
	}
}

func newNode(t *testing.T) (*Node, *fake.Clientset) {
	testNode := genNode()
	// create fake clientSet
	fakeClientSet := fake.NewClientset(testNode)
	opts := &types.Options{
		NodeName: testNode.Name,
	}
	savedRetry := WATCH_RETRY_INTERVAL
	WATCH_RETRY_INTERVAL = time.Millisecond * 100
	t.Cleanup(func() { WATCH_RETRY_INTERVAL = savedRetry })
	n, err := NewNodeWithClientSet(context.Background(), opts, fakeClientSet)
	assert.NilError(t, err)
	return n, fakeClientSet
}

func waitForNodeLabel(t *testing.T, n *Node, key, want string) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if n.GetK8sNode().Labels[key] == want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("node label %s=%q, want %q", key, n.GetK8sNode().Labels[key], want)
}

func TestWatchNode(t *testing.T) {
	n, fakeClientSet := newNode(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	n.ctx = ctx

	watcher := watch.NewFake()
	var fieldSelector string
	fakeClientSet.PrependWatchReactor("nodes", func(action ktesting.Action) (bool, watch.Interface, error) {
		wa := action.(ktesting.WatchAction)
		if fields := wa.GetWatchRestrictions().Fields; fields != nil {
			fieldSelector = fields.String()
		}
		return true, watcher, nil
	})

	done := make(chan error, 1)
	go func() {
		done <- n.watchK8sNode()
	}()

	updated := n.GetK8sNode().DeepCopy()
	updated.Labels["test.key"] = "test.val"
	watcher.Modify(updated)
	waitForNodeLabel(t, n, "test.key", "test.val")
	assert.Equal(t, fieldSelector, "metadata.name=test-node")

	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("watchK8sNode did not return after cancel")
	}
}

func TestApplyWatchEventModified(t *testing.T) {
	n, _ := newNode(t)
	updated := n.GetK8sNode().DeepCopy()
	updated.Labels["test.key"] = "test.val"
	assert.NilError(t, n.applyWatchEvent(watch.Event{Type: watch.Modified, Object: updated}))
	assert.Equal(t, n.GetK8sNode().Labels["test.key"], "test.val")
}

func TestApplyWatchEventUnexpectedObject(t *testing.T) {
	n, _ := newNode(t)
	err := n.applyWatchEvent(watch.Event{Type: watch.Added, Object: &corev1.Pod{}})
	assert.Assert(t, err != nil)
}

func TestApplyWatchEventDeletedKeepsCache(t *testing.T) {
	n, _ := newNode(t)
	name := n.GetK8sNode().Name
	assert.NilError(t, n.applyWatchEvent(watch.Event{Type: watch.Deleted, Object: n.GetK8sNode()}))
	assert.Equal(t, n.GetK8sNode().Name, name)
}

func TestApplyWatchEventError(t *testing.T) {
	n, _ := newNode(t)
	err := n.applyWatchEvent(watch.Event{Type: watch.Error, Object: &metav1.Status{
		Status:  metav1.StatusFailure,
		Code:    410,
		Reason:  metav1.StatusReasonExpired,
		Message: "too old resource version",
	}})
	assert.Assert(t, err != nil)
	assert.Equal(t, apierrors.ReasonForError(err), metav1.StatusReasonExpired)
	assert.Equal(t, apierrors.IsResourceExpired(err), true)
}

func TestWatchTimeoutSecondsIsRandomizedAboveMinimum(t *testing.T) {
	saved := WATCH_MIN_TIMEOUT_SECONDS
	WATCH_MIN_TIMEOUT_SECONDS = 300
	t.Cleanup(func() { WATCH_MIN_TIMEOUT_SECONDS = saved })

	seen := map[int64]struct{}{}
	for i := 0; i < 100; i++ {
		got := watchTimeoutSeconds()
		assert.Assert(t, got >= WATCH_MIN_TIMEOUT_SECONDS, "got=%d", got)
		assert.Assert(t, got < 2*WATCH_MIN_TIMEOUT_SECONDS, "got=%d", got)
		seen[got] = struct{}{}
	}
	assert.Assert(t, len(seen) > 1, "timeout was not randomized")
}

func TestResourceVersionOlderUsesDecimalOrder(t *testing.T) {
	assert.Equal(t, resourceVersionOlder("9", "10"), true)
	assert.Equal(t, resourceVersionOlder("10", "9"), false)
	assert.Equal(t, resourceVersionOlder("12", "12"), false)
	assert.Equal(t, resourceVersionOlder("", "12"), true)
	assert.Equal(t, resourceVersionOlder("12", ""), false)
}

func TestSetK8sNodeIfNewerSkipsOlderResourceVersion(t *testing.T) {
	n, _ := newNode(t)
	watched := n.GetK8sNode().DeepCopy()
	watched.ResourceVersion = "12"
	watched.Labels["source"] = "watch"
	n.setK8sNode(watched)

	stale := n.GetK8sNode().DeepCopy()
	stale.ResourceVersion = "11"
	stale.Labels["source"] = "update"
	assert.Equal(t, n.setK8sNodeIfNewer(stale), false)
	assert.Equal(t, n.GetK8sNode().ResourceVersion, "12")
	assert.Equal(t, n.GetK8sNode().Labels["source"], "watch")

	fresh := n.GetK8sNode().DeepCopy()
	fresh.ResourceVersion = "13"
	fresh.Labels["source"] = "update"
	assert.Equal(t, n.setK8sNodeIfNewer(fresh), true)
	assert.Equal(t, n.GetK8sNode().ResourceVersion, "13")
}

func TestCacheKeepsAheadGetWhenDelayedWatchArrives(t *testing.T) {
	n, _ := newNode(t)
	n.setK8sNode(nodeWithRV(n.GetK8sNode(), "10", "watch-10"))
	assert.Equal(t, n.setK8sNodeIfNewer(nodeWithRV(n.GetK8sNode(), "11", "get-11")), true)
	assert.Equal(t, n.setK8sNodeIfNewer(nodeWithRV(n.GetK8sNode(), "12", "update-12")), true)

	assert.NilError(t, n.applyWatchEvent(watch.Event{
		Type:   watch.Modified,
		Object: nodeWithRV(n.GetK8sNode(), "10", "watch-10"),
	}))
	assert.NilError(t, n.applyWatchEvent(watch.Event{
		Type:   watch.Modified,
		Object: nodeWithRV(n.GetK8sNode(), "11", "get-11"),
	}))
	assert.Equal(t, n.GetK8sNode().ResourceVersion, "12")
	assert.Equal(t, n.GetK8sNode().Labels["source"], "update-12")
}

func TestUpdateStatusAppliesAfterStaleWatchEvent(t *testing.T) {
	n, _ := newNode(t)
	n.setK8sNode(nodeWithRV(n.GetK8sNode(), "12", "update-12"))

	staleWatch := nodeWithRV(n.GetK8sNode(), "10", "watch-10")
	assert.NilError(t, n.applyWatchEvent(watch.Event{Type: watch.Modified, Object: staleWatch}))
	assert.Equal(t, n.GetK8sNode().ResourceVersion, "12")
	assert.Equal(t, n.GetK8sNode().Labels["source"], "update-12")

	response := nodeWithRV(n.GetK8sNode(), "12", "update-12")
	assert.Equal(t, n.setK8sNodeIfNewer(response), true)
	assert.Equal(t, n.GetK8sNode().ResourceVersion, "12")

	n.setK8sNode(nodeWithRV(n.GetK8sNode(), "10", "watch-10"))
	assert.Equal(t, n.setK8sNodeIfNewer(response), true)
	assert.Equal(t, n.GetK8sNode().ResourceVersion, "12")
	assert.Equal(t, n.GetK8sNode().Labels["source"], "update-12")
}

func nodeWithRV(node *corev1.Node, rv, source string) *corev1.Node {
	out := node.DeepCopy()
	out.ResourceVersion = rv
	if out.Labels == nil {
		out.Labels = map[string]string{}
	}
	out.Labels["source"] = source
	return out
}

func TestUpdateConditionsKeepsNewerWatchState(t *testing.T) {
	n, fakeClientSet := newNode(t)
	fakeClientSet.PrependReactor("update", "nodes", func(action ktesting.Action) (bool, kruntime.Object, error) {
		watched := n.GetK8sNode().DeepCopy()
		watched.ResourceVersion = "12"
		watched.Labels["source"] = "watch"
		n.setK8sNode(watched)

		returned := action.(ktesting.UpdateAction).GetObject().(*corev1.Node).DeepCopy()
		returned.ResourceVersion = "11"
		return true, returned, nil
	})

	assert.NilError(t, n.UpdateConditions([]corev1.NodeCondition{
		{Type: "safe.stale", Status: corev1.ConditionTrue},
	}))
	assert.Equal(t, n.GetK8sNode().ResourceVersion, "12")
	assert.Equal(t, n.GetK8sNode().Labels["source"], "watch")
}

func TestWatchK8sNodeWatchError(t *testing.T) {
	n, fakeClientSet := newNode(t)
	fakeClientSet.PrependWatchReactor("nodes", func(action ktesting.Action) (bool, watch.Interface, error) {
		return true, nil, fmt.Errorf("watch failed")
	})
	err := n.watchK8sNode()
	assert.Assert(t, err != nil)
}

func TestWatchK8sNodeClosed(t *testing.T) {
	n, fakeClientSet := newNode(t)
	watcher := watch.NewFake()
	fakeClientSet.PrependWatchReactor("nodes", func(action ktesting.Action) (bool, watch.Interface, error) {
		return true, watcher, nil
	})
	done := make(chan error, 1)
	go func() {
		done <- n.watchK8sNode()
	}()
	watcher.Stop()
	select {
	case err := <-done:
		assert.NilError(t, err)
	case <-time.After(time.Second):
		t.Fatal("watchK8sNode did not return after watch closed")
	}
}

func TestWatchK8sNodeClosedAfterEvent(t *testing.T) {
	n, fakeClientSet := newNode(t)
	watcher := watch.NewFake()
	fakeClientSet.PrependWatchReactor("nodes", func(action ktesting.Action) (bool, watch.Interface, error) {
		return true, watcher, nil
	})
	done := make(chan error, 1)
	go func() {
		done <- n.watchK8sNode()
	}()
	updated := n.GetK8sNode().DeepCopy()
	updated.Labels["test.key"] = "test.val"
	watcher.Modify(updated)
	waitForNodeLabel(t, n, "test.key", "test.val")
	watcher.Stop()
	select {
	case err := <-done:
		assert.NilError(t, err)
	case <-time.After(time.Second):
		t.Fatal("watchK8sNode did not return after watch closed")
	}
}

func TestWatchRetryDelayGrowsWithConsecutiveShortWatches(t *testing.T) {
	savedInterval := WATCH_RETRY_INTERVAL
	savedMax := WATCH_RETRY_MAX
	WATCH_RETRY_INTERVAL = 100 * time.Millisecond
	WATCH_RETRY_MAX = 400 * time.Millisecond
	t.Cleanup(func() {
		WATCH_RETRY_INTERVAL = savedInterval
		WATCH_RETRY_MAX = savedMax
	})

	first := watchRetryDelay(1)
	assert.Assert(t, first >= WATCH_RETRY_INTERVAL, "first=%s", first)
	assert.Assert(t, first <= WATCH_RETRY_INTERVAL+WATCH_RETRY_INTERVAL/2, "first=%s", first)

	capped := watchRetryDelay(8)
	assert.Assert(t, capped >= WATCH_RETRY_MAX, "capped=%s", capped)
	assert.Assert(t, capped <= WATCH_RETRY_MAX+WATCH_RETRY_MAX/2, "capped=%s", capped)
}

func TestUpdateBacksOffOnShortWatchClose(t *testing.T) {
	n, fakeClientSet := newNode(t)
	savedRetry := WATCH_RETRY_INTERVAL
	savedHealthy := WATCH_HEALTHY_DURATION
	WATCH_RETRY_INTERVAL = 300 * time.Millisecond
	WATCH_HEALTHY_DURATION = time.Second
	t.Cleanup(func() {
		WATCH_RETRY_INTERVAL = savedRetry
		WATCH_HEALTHY_DURATION = savedHealthy
	})

	var watches atomic.Int32
	fakeClientSet.PrependWatchReactor("nodes", func(action ktesting.Action) (bool, watch.Interface, error) {
		watches.Add(1)
		w := watch.NewFake()
		w.Stop()
		return true, w, nil
	})

	ctx, cancel := context.WithCancel(context.Background())
	n.ctx = ctx
	done := make(chan struct{})
	go func() {
		n.update()
		close(done)
	}()

	deadline := time.Now().Add(150 * time.Millisecond)
	for time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	assert.Equal(t, int32(1), watches.Load())

	deadline = time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if watches.Load() >= 2 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("update did not stop after cancel")
	}
	assert.Assert(t, watches.Load() >= 2, "watches=%d", watches.Load())
}

func TestUpdateReconnectsImmediatelyAfterLongWatch(t *testing.T) {
	n, fakeClientSet := newNode(t)
	savedRetry := WATCH_RETRY_INTERVAL
	savedHealthy := WATCH_HEALTHY_DURATION
	WATCH_RETRY_INTERVAL = time.Second
	WATCH_HEALTHY_DURATION = 30 * time.Millisecond
	t.Cleanup(func() {
		WATCH_RETRY_INTERVAL = savedRetry
		WATCH_HEALTHY_DURATION = savedHealthy
	})

	var watches atomic.Int32
	watcher := watch.NewFake()
	fakeClientSet.PrependWatchReactor("nodes", func(action ktesting.Action) (bool, watch.Interface, error) {
		if watches.Add(1) == 1 {
			return true, watcher, nil
		}
		w := watch.NewFake()
		return true, w, nil
	})

	ctx, cancel := context.WithCancel(context.Background())
	n.ctx = ctx
	done := make(chan struct{})
	go func() {
		n.update()
		close(done)
	}()

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if watches.Load() >= 1 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	time.Sleep(WATCH_HEALTHY_DURATION + 20*time.Millisecond)
	watcher.Stop()

	deadline = time.Now().Add(400 * time.Millisecond)
	for time.Now().Before(deadline) {
		if watches.Load() >= 2 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("update did not stop after cancel")
	}
	assert.Assert(t, watches.Load() >= 2, "watches=%d", watches.Load())
}

func TestStartWatchesNode(t *testing.T) {
	savedNSENTER := NSENTER
	NSENTER = ""
	defer func() { NSENTER = savedNSENTER }()

	n, fakeClientSet := newNode(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	n.ctx = ctx

	watchStarted := make(chan struct{})
	var once sync.Once
	fakeClientSet.PrependWatchReactor("nodes", func(action ktesting.Action) (bool, watch.Interface, error) {
		once.Do(func() { close(watchStarted) })
		return false, nil, nil
	})

	assert.NilError(t, n.Start())
	select {
	case <-watchStarted:
	case <-time.After(time.Second):
		t.Fatal("watch reactor did not start")
	}

	updated := n.GetK8sNode().DeepCopy()
	updated.Labels["test.key"] = "test.val"
	_, err := fakeClientSet.CoreV1().Nodes().Update(context.Background(), updated, metav1.UpdateOptions{})
	assert.NilError(t, err)
	waitForNodeLabel(t, n, "test.key", "test.val")
	cancel()
}

func TestGetK8sNodeReturnsSnapshot(t *testing.T) {
	n, _ := newNode(t)
	got := n.GetK8sNode()
	got.Labels["mutated"] = "1"
	assert.Equal(t, n.GetK8sNode().Labels["mutated"], "")
}

func TestGetGpuQuantity(t *testing.T) {
	n, _ := newNode(t)
	quantity := n.GetGpuQuantity()
	assert.Equal(t, quantity.Value(), int64(8))
	assert.Equal(t, n.IsMatchGpuChip(string(v1.AmdGpuChip)), true)
	assert.Equal(t, n.IsMatchGpuChip(string(v1.NvidiaGpuChip)), false)
}

func TestUpdateCondition(t *testing.T) {
	n, _ := newNode(t)
	condition := corev1.NodeCondition{
		Type:   "safe.101",
		Status: "True",
	}
	resp := n.FindConditionByType(string(condition.Type))
	assert.Equal(t, resp != nil, false)
	err := n.UpdateConditions([]corev1.NodeCondition{condition})
	assert.NilError(t, err)
	resp = n.FindConditionByType(string(condition.Type))
	assert.Equal(t, resp != nil, true)
}

func TestUpdateStartTime(t *testing.T) {
	n, _ := newNode(t)
	nowTime := time.Now()
	err := n.updateNodeStartTime(nowTime)
	assert.NilError(t, err)
	nowTimeStr := strconv.FormatInt(nowTime.Unix(), 10)
	assert.Equal(t, v1.GetNodeStartupTime(n.GetK8sNode()), nowTimeStr)
}

// --- merged from node_extra_test.go ---

// TestFindCondition locates a condition using a custom equality function.
func TestFindCondition(t *testing.T) {
	n, _ := newNode(t)
	cond := corev1.NodeCondition{Type: "safe.find", Status: corev1.ConditionTrue}
	assert.NilError(t, n.UpdateConditions([]corev1.NodeCondition{cond}))
	found := n.FindCondition(&cond, func(a, b *corev1.NodeCondition) bool {
		return a.Type == b.Type
	})
	assert.Assert(t, found != nil)
	assert.Equal(t, string(found.Type), "safe.find")
}

// TestGetEphemeralStorage returns allocatable ephemeral storage quantity.
func TestGetEphemeralStorage(t *testing.T) {
	n, _ := newNode(t)
	q := n.GetEphemeralStorage()
	assert.Equal(t, q.IsZero(), true)

	node := n.GetK8sNode()
	node.Status.Allocatable = corev1.ResourceList{
		corev1.ResourceEphemeralStorage: resource.MustParse("100Gi"),
	}
	n.k8sNode = node
	q = n.GetEphemeralStorage()
	expected := resource.MustParse("100Gi")
	assert.Equal(t, q.Value(), expected.Value())
}

// TestIsMatchGpuChipNvidia matches nodes labeled for NVIDIA GPUs.
func TestIsMatchGpuChipNvidia(t *testing.T) {
	testNode := genNode()
	delete(testNode.Labels, common.AMDGpuIdentification)
	testNode.Labels[common.NvidiaIdentification] = "true"
	fakeClientSet := fake.NewClientset(testNode)
	opts := &types.Options{NodeName: testNode.Name}
	n, err := NewNodeWithClientSet(context.Background(), opts, fakeClientSet)
	assert.NilError(t, err)
	assert.Equal(t, n.IsMatchGpuChip(string(v1.NvidiaGpuChip)), true)
	assert.Equal(t, n.IsMatchGpuChip(string(v1.AmdGpuChip)), false)
}

// TestIsMatchGpuChipEmpty matches any chip when filter is empty.
func TestIsMatchGpuChipEmpty(t *testing.T) {
	n, _ := newNode(t)
	assert.Equal(t, n.IsMatchGpuChip(""), true)
	assert.Equal(t, n.IsMatchGpuChip("unknown"), false)
}

// TestGetGpuQuantityNvidia reads NVIDIA GPU allocatable resources.
func TestGetGpuQuantityNvidia(t *testing.T) {
	testNode := genNode()
	delete(testNode.Labels, common.AMDGpuIdentification)
	testNode.Labels[common.NvidiaIdentification] = "true"
	testNode.Status.Allocatable = corev1.ResourceList{
		common.NvidiaGpu: resource.MustParse("4"),
	}
	fakeClientSet := fake.NewClientset(testNode)
	opts := &types.Options{NodeName: testNode.Name}
	n, err := NewNodeWithClientSet(context.Background(), opts, fakeClientSet)
	assert.NilError(t, err)
	gpuQty := n.GetGpuQuantity()
	assert.Equal(t, gpuQty.Value(), int64(4))
}

// TestSyncK8sNode refreshes the cached node object from the API.
func TestSyncK8sNode(t *testing.T) {
	n, fakeClientSet := newNode(t)
	_, err := fakeClientSet.CoreV1().Nodes().Update(context.Background(), n.GetK8sNode(), metav1.UpdateOptions{})
	assert.NilError(t, err)
	assert.NilError(t, n.syncK8sNode())
}

// TestStartUninitializedNode returns error when node is nil.
func TestStartUninitializedNode(t *testing.T) {
	var n *Node
	err := n.Start()
	testifyassert.Error(t, err, "please initialize node first")
}

// TestUpdateConditionsUninitialized returns error when k8s node is nil.
func TestUpdateConditionsUninitialized(t *testing.T) {
	n := &Node{}
	err := n.UpdateConditions(nil)
	testifyassert.Error(t, err, "please initialize node first")
}

// TestGetLocation reads system timezone when nsenter is disabled.
func TestGetLocation(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires unix shell commands")
	}
	saved := NSENTER
	NSENTER = ""
	defer func() { NSENTER = saved }()
	loc, err := getLocation()
	assert.NilError(t, err)
	assert.Assert(t, loc != nil)
}

// TestGetUptime parses host boot time when nsenter is disabled.
func TestGetUptime(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires unix shell commands")
	}
	saved := NSENTER
	NSENTER = ""
	defer func() { NSENTER = saved }()
	loc, err := getLocation()
	assert.NilError(t, err)
	start, err := getUptime(loc)
	assert.NilError(t, err)
	assert.Assert(t, !start.IsZero())
}

// TestUpdateStartTimeViaNode exercises updateStartTime on a live node object.
func TestUpdateStartTimeViaNode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires unix shell commands")
	}
	saved := NSENTER
	NSENTER = ""
	defer func() { NSENTER = saved }()
	n, _ := newNode(t)
	err := n.updateStartTime()
	if err != nil {
		t.Skip("host uptime command unavailable")
	}
	assert.Assert(t, v1.GetNodeStartupTime(n.GetK8sNode()) != "")
}

// TestFindConditionByTypeNilNode returns nil when the node is uninitialized.
func TestFindConditionByTypeNilNode(t *testing.T) {
	n := &Node{}
	assert.Assert(t, n.FindConditionByType("safe.none") == nil)
}

// TestFindConditionNilNode returns nil when the node is uninitialized.
func TestFindConditionNilNode(t *testing.T) {
	n := &Node{}
	cond := corev1.NodeCondition{Type: "safe.none"}
	assert.Assert(t, n.FindCondition(&cond, func(a, b *corev1.NodeCondition) bool {
		return a.Type == b.Type
	}) == nil)
}

// TestGetGpuQuantityNilNode returns zero when the node is uninitialized.
func TestGetGpuQuantityNilNode(t *testing.T) {
	n := &Node{}
	qty := n.GetGpuQuantity()
	assert.Equal(t, qty.IsZero(), true)
}

// TestGetEphemeralStorageNilNode returns zero when the node is uninitialized.
func TestGetEphemeralStorageNilNode(t *testing.T) {
	n := &Node{}
	storage := n.GetEphemeralStorage()
	assert.Equal(t, storage.IsZero(), true)
}

// TestSyncK8sNodeError propagates API errors from the Kubernetes client.
func TestSyncK8sNodeError(t *testing.T) {
	n, fakeClientSet := newNode(t)
	assert.NilError(t, fakeClientSet.CoreV1().Nodes().Delete(context.Background(), n.GetK8sNode().Name, metav1.DeleteOptions{}))
	err := n.syncK8sNode()
	assert.Assert(t, err != nil)
}

// TestUpdateConditionsConflict retries after a Kubernetes conflict error.
func TestUpdateConditionsConflict(t *testing.T) {
	n, fakeClientSet := newNode(t)
	attempts := 0
	fakeClientSet.PrependReactor("update", "nodes", func(action ktesting.Action) (bool, kruntime.Object, error) {
		attempts++
		if attempts == 1 {
			return true, nil, apierrors.NewConflict(
				schema.GroupResource{Resource: "nodes"},
				n.GetK8sNode().Name,
				fmt.Errorf("conflict"),
			)
		}
		return false, nil, nil
	})
	cond := corev1.NodeCondition{Type: "safe.conflict", Status: corev1.ConditionTrue}
	assert.NilError(t, n.UpdateConditions([]corev1.NodeCondition{cond}))
	assert.Assert(t, attempts > 1)
}

// TestUpdateNodeStartTimeNoChange skips patch when label already matches.
func TestUpdateNodeStartTimeNoChange(t *testing.T) {
	n, _ := newNode(t)
	now := time.Unix(1700000000, 0).UTC()
	assert.NilError(t, n.updateNodeStartTime(now))
	err := n.updateNodeStartTime(now)
	assert.NilError(t, err)
}

// --- merged from node_init_test.go ---

// TestNewNodeWithClientSetNotFound returns an error when the node does not exist.
func TestNewNodeWithClientSetNotFound(t *testing.T) {
	fakeClientSet := fake.NewClientset()
	opts := &types.Options{NodeName: "missing-node"}
	_, err := NewNodeWithClientSet(context.Background(), opts, fakeClientSet)
	assert.Assert(t, err != nil)
}

// TestIsMatchGpuChipInvalidAmdLabel treats non-true AMD labels as mismatched.
func TestIsMatchGpuChipInvalidAmdLabel(t *testing.T) {
	testNode := genNode()
	testNode.Labels[common.AMDGpuIdentification] = "false"
	fakeClientSet := fake.NewClientset(testNode)
	opts := &types.Options{NodeName: testNode.Name}
	n, err := NewNodeWithClientSet(context.Background(), opts, fakeClientSet)
	assert.NilError(t, err)
	assert.Equal(t, n.IsMatchGpuChip(string(v1.AmdGpuChip)), false)
}

// TestNodeUpdateStopsOnCancel exits the watch loop when the context is cancelled.
func TestNodeUpdateStopsOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	n, _ := newNode(t)
	n.ctx = ctx
	done := make(chan struct{})
	go func() {
		n.update()
		close(done)
	}()
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("update did not stop after cancel")
	}
}

// --- merged from node_new_test.go ---

// TestNewNodeOutsideCluster returns an error when no in-cluster config is available.
func TestNewNodeOutsideCluster(t *testing.T) {
	ctx := context.Background()
	opts := &types.Options{NodeName: "test-node"}
	_, err := NewNode(ctx, opts)
	assert.Assert(t, err != nil)
}

// --- merged from node_shell_test.go ---

// failingNSENTERPrefix forces the shell pipeline to exit non-zero before host tools run.
// Empty NSENTER runs commands on the local host; CI runners often have timedatectl/uptime,
// so we cannot assert failure from an empty prefix.
const failingNSENTERPrefix = `false && `

// TestGetLocationFailsWhenHostCommandFails returns an error when the wrapped command fails.
func TestGetLocationFailsWhenHostCommandFails(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires unix shell commands")
	}
	saved := NSENTER
	NSENTER = failingNSENTERPrefix
	defer func() { NSENTER = saved }()
	_, err := getLocation()
	assert.Assert(t, err != nil)
}

// TestGetUptimeFailsWhenHostCommandFails returns an error when uptime cannot be read.
func TestGetUptimeFailsWhenHostCommandFails(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires unix shell commands")
	}
	saved := NSENTER
	NSENTER = failingNSENTERPrefix
	defer func() { NSENTER = saved }()
	_, err := getUptime(time.UTC)
	assert.Assert(t, err != nil)
}

// TestUpdateStartTimeFailsWhenHostCommandFails propagates failures from getLocation/getUptime.
func TestUpdateStartTimeFailsWhenHostCommandFails(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires unix shell commands")
	}
	saved := NSENTER
	NSENTER = failingNSENTERPrefix
	defer func() { NSENTER = saved }()
	n, _ := newNode(t)
	err := n.updateStartTime()
	assert.Assert(t, err != nil)
}
