/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resource

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/model"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/robustclient"
)

type fakeUsageStore struct {
	samples       []*model.WorkspaceUsageSample
	hourly        []*model.WorkspaceUsageHourly
	hours         []time.Time
	rolledUp      []time.Time
	sampleCutoffs []time.Time
	hourlyCutoffs []time.Time
	upsertErr     error
	// failRollups is how many more times listing the samples of an hour fails.
	failRollups map[time.Time]int
}

func (s *fakeUsageStore) UpsertWorkspaceUsageSamples(_ context.Context, rows []*model.WorkspaceUsageSample) error {
	if s.upsertErr != nil {
		return s.upsertErr
	}
	s.samples = append(s.samples, rows...)
	return nil
}

func (s *fakeUsageStore) ListWorkspaceUsageSamples(_ context.Context,
	filter dbclient.WorkspaceUsageFilter) ([]*model.WorkspaceUsageSample, error) {
	s.rolledUp = append(s.rolledUp, filter.Start)
	if s.failRollups[filter.Start] > 0 {
		s.failRollups[filter.Start]--
		return nil, errors.New("db down")
	}
	var result []*model.WorkspaceUsageSample
	for _, row := range s.samples {
		if !row.SampleTime.Before(filter.Start) && row.SampleTime.Before(filter.End) {
			result = append(result, row)
		}
	}
	return result, nil
}

func (s *fakeUsageStore) DeleteWorkspaceUsageSamplesBefore(_ context.Context, before time.Time) (int64, error) {
	s.sampleCutoffs = append(s.sampleCutoffs, before)
	return 0, nil
}

func (s *fakeUsageStore) UpsertWorkspaceUsageHourly(_ context.Context, rows []*model.WorkspaceUsageHourly) error {
	s.hourly = append(s.hourly, rows...)
	return nil
}

func (s *fakeUsageStore) ListWorkspaceUsageHours(_ context.Context, _, _ time.Time) ([]time.Time, error) {
	return s.hours, nil
}

func (s *fakeUsageStore) DeleteWorkspaceUsageHourlyBefore(_ context.Context, before time.Time) (int64, error) {
	s.hourlyCutoffs = append(s.hourlyCutoffs, before)
	return 0, nil
}

type fakeUsageWindows struct {
	byCluster map[string][]robustclient.WorkloadGPUWindow
	failing   map[string]bool
	// hanging clusters answer only when the context ends.
	hanging map[string]bool

	mu      sync.Mutex
	windows []string
	steps   []string
}

func (f *fakeUsageWindows) GetWorkloadGPUWindow(ctx context.Context, cluster string,
	_ []string, window, step string) ([]robustclient.WorkloadGPUWindow, error) {
	f.mu.Lock()
	f.windows = append(f.windows, window)
	f.steps = append(f.steps, step)
	f.mu.Unlock()
	if f.hanging[cluster] {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	if f.failing[cluster] {
		return nil, errors.New("robust down")
	}
	return f.byCluster[cluster], nil
}

func usageTestWorkspace(name, cluster string) *v1.Workspace {
	return &v1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec:       v1.WorkspaceSpec{Cluster: cluster},
		Status: v1.WorkspaceStatus{TotalResources: corev1.ResourceList{
			common.AmdGpu: *resource.NewQuantity(16, resource.DecimalSI),
		}},
	}
}

// usageTestWorkload is a running workload of one pod with 8 GPUs on a node.
func usageTestWorkload(name, workspace string) *v1.Workload {
	return &v1.Workload{
		ObjectMeta: metav1.ObjectMeta{Name: name, UID: types.UID(name + "-uid")},
		Spec: v1.WorkloadSpec{
			Workspace: workspace,
			Resources: []v1.WorkloadResource{{Replica: 1, CPU: "8", Memory: "32Gi", GPU: "8", GPUName: common.AmdGpu}},
		},
		Status: v1.WorkloadStatus{
			Phase:     v1.WorkloadRunning,
			NodeUsage: []v1.NodePodUsage{{Node: name + "-node", Active: map[string]int{"0": 1}, Running: map[string]int{"0": 1}}},
		},
	}
}

func newTestUsageCollector(t *testing.T, now time.Time, objs ...client.Object) (*WorkspaceUsageCollector, *fakeUsageStore, *fakeUsageWindows) {
	t.Helper()
	scheme, err := genMockScheme()
	require.NoError(t, err)
	store := &fakeUsageStore{}
	windows := &fakeUsageWindows{byCluster: map[string][]robustclient.WorkloadGPUWindow{}, failing: map[string]bool{}}
	c := &WorkspaceUsageCollector{
		Client:          ctrlfake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build(),
		source:          windows,
		store:           store,
		interval:        5 * time.Minute,
		sampleRetention: 14 * 24 * time.Hour,
		hourlyRetention: 400 * 24 * time.Hour,
		now:             func() time.Time { return now },
	}
	return c, store, windows
}

// tickNow runs the tick of the slot that now is in.
func tickNow(c *WorkspaceUsageCollector) {
	c.tick(context.Background(), c.now().UTC().Truncate(c.interval))
}

func usageRowOf(rows []*model.WorkspaceUsageSample, workspace, resource string) *model.WorkspaceUsageSample {
	for _, row := range rows {
		if row.Workspace == workspace && row.Resource == resource {
			return row
		}
	}
	return nil
}

func TestUsageCollectorSampleIsolatesClusterFailures(t *testing.T) {
	now := time.Date(2026, 11, 20, 10, 7, 30, 0, time.UTC)
	c, store, windows := newTestUsageCollector(t, now,
		usageTestWorkspace("ws1", "c1"), usageTestWorkspace("ws2", "c2"),
		usageTestWorkload("a", "ws1"), usageTestWorkload("b", "ws2"))
	windows.byCluster["c1"] = []robustclient.WorkloadGPUWindow{
		{UID: "a-uid", Overall: robustclient.GPUWindowOverall{Avg: 25, Samples: 5, ExpectedSamples: 5}},
	}
	windows.failing["c2"] = true

	require.NoError(t, c.sample(context.Background(), now.Truncate(c.interval)))

	gpu1 := usageRowOf(store.samples, "ws1", common.AmdGpu)
	require.NotNil(t, gpu1)
	assert.Equal(t, time.Date(2026, 11, 20, 10, 5, 0, 0, time.UTC), gpu1.SampleTime)
	assert.Equal(t, "c1", gpu1.Cluster)
	assert.Equal(t, 16.0, gpu1.Quota)
	assert.Equal(t, 8.0, gpu1.Allocated)
	require.NotNil(t, gpu1.Busy)
	assert.InDelta(t, 2.0, *gpu1.Busy, 1e-9)
	require.NotNil(t, gpu1.UtilCoverage)
	assert.Equal(t, 1.0, *gpu1.UtilCoverage)

	gpu2 := usageRowOf(store.samples, "ws2", common.AmdGpu)
	require.NotNil(t, gpu2)
	assert.Equal(t, 8.0, gpu2.Allocated, "the failing cluster is still sampled")
	assert.Nil(t, gpu2.Busy)
	assert.Nil(t, gpu2.UtilCoverage)

	assert.NotNil(t, usageRowOf(store.samples, "ws1", string(corev1.ResourceCPU)))
	assert.Equal(t, []string{"5m", "5m"}, windows.windows)
}

func TestUsageCollectorSampleBoundsRobust(t *testing.T) {
	now := time.Date(2026, 11, 20, 10, 7, 30, 0, time.UTC)
	c, store, windows := newTestUsageCollector(t, now,
		usageTestWorkspace("ws1", "c1"), usageTestWorkspace("ws2", "c2"),
		usageTestWorkload("a", "ws1"), usageTestWorkload("b", "ws2"))
	c.interval = 100 * time.Millisecond
	windows.byCluster["c1"] = []robustclient.WorkloadGPUWindow{
		{UID: "a-uid", Overall: robustclient.GPUWindowOverall{Avg: 25, Samples: 5, ExpectedSamples: 5}},
	}
	windows.hanging = map[string]bool{"c2": true}

	start := time.Now()
	require.NoError(t, c.sample(context.Background(), now))
	assert.Less(t, time.Since(start), c.interval, "a hanging cluster does not hold the sample past its budget")

	gpu1 := usageRowOf(store.samples, "ws1", common.AmdGpu)
	require.NotNil(t, gpu1.Busy)
	assert.InDelta(t, 2.0, *gpu1.Busy, 1e-9)
	gpu2 := usageRowOf(store.samples, "ws2", common.AmdGpu)
	assert.Nil(t, gpu2.Busy, "a cluster that timed out has unknown utilization")
	assert.Nil(t, gpu2.UtilCoverage)
}

func TestUsageCollectorTickLabelsTheSampleWithItsSlot(t *testing.T) {
	ten := time.Date(2026, 11, 20, 10, 10, 0, 0, time.UTC)
	c, store, _ := newTestUsageCollector(t, ten.Add(-time.Millisecond), usageTestWorkspace("ws1", "c1"))
	c.tick(context.Background(), ten)
	written := len(store.samples)
	require.NotZero(t, written)
	assert.Equal(t, ten, store.samples[0].SampleTime, "a tick a little early is still its own slot")

	c.now = func() time.Time { return ten.Add(c.interval + 4*time.Minute) }
	c.tick(context.Background(), ten.Add(c.interval))
	require.Len(t, store.samples, 2*written)
	assert.Equal(t, ten.Add(c.interval), store.samples[written].SampleTime, "a late tick is not labeled with the next slot")

	c.tick(context.Background(), ten.Add(c.interval))
	assert.Len(t, store.samples, 2*written, "a slot is written once")
}

func TestUsageCollectorNextSlot(t *testing.T) {
	ten := time.Date(2026, 11, 20, 10, 10, 0, 0, time.UTC)
	c, _, _ := newTestUsageCollector(t, ten.Add(time.Second))
	assert.Equal(t, ten.Add(5*time.Minute), c.nextSlot(ten))

	c.now = func() time.Time { return ten.Add(5*time.Minute + time.Second) }
	assert.Equal(t, ten.Add(5*time.Minute), c.nextSlot(ten), "a tick that ran into the next slot still writes it")

	c.now = func() time.Time { return ten.Add(17 * time.Minute) }
	assert.Equal(t, ten.Add(15*time.Minute), c.nextSlot(ten), "the slots a long tick ran past are skipped")
}

func TestUsageCollectorStartSamplesAtTheBoundaries(t *testing.T) {
	c, store, _ := newTestUsageCollector(t, time.Time{}, usageTestWorkspace("ws1", "c1"))
	c.interval = 100 * time.Millisecond
	c.now = time.Now
	started := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 450*time.Millisecond)
	defer cancel()
	require.NoError(t, c.Start(ctx))

	var times []time.Time
	for _, row := range store.samples {
		if len(times) == 0 || !row.SampleTime.Equal(times[len(times)-1]) {
			times = append(times, row.SampleTime)
		}
	}
	require.NotEmpty(t, times)
	assert.True(t, times[0].After(started), "the first sample waits for the next boundary")
	for i, at := range times {
		assert.Equal(t, at.Truncate(c.interval), at, "a sample is labeled with its boundary")
		if i > 0 {
			assert.Equal(t, times[i-1].Add(c.interval), at, "every slot is written once")
		}
	}
}

func TestUsageCollectorTickRollsUpAndPrunesOncePerHour(t *testing.T) {
	now := time.Date(2026, 11, 20, 11, 0, 10, 0, time.UTC)
	c, store, _ := newTestUsageCollector(t, now, usageTestWorkspace("ws1", "c1"))
	previous := time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC)
	store.samples = []*model.WorkspaceUsageSample{
		{SampleTime: previous, Cluster: "c1", Workspace: "ws1", Resource: common.AmdGpu, Quota: 16, Allocated: 8},
		{SampleTime: previous.Add(55 * time.Minute), Cluster: "c1", Workspace: "ws1", Resource: common.AmdGpu, Quota: 16, Allocated: 16},
	}

	tickNow(c)
	require.Equal(t, []time.Time{previous}, store.rolledUp)
	require.Len(t, store.hourly, 1)
	assert.Equal(t, previous, store.hourly[0].Hour)
	assert.Equal(t, 12.0, store.hourly[0].AllocatedAvg)
	assert.Equal(t, int32(2), store.hourly[0].Samples)
	assert.Len(t, store.sampleCutoffs, 1)

	c.now = func() time.Time { return now.Add(5 * time.Minute) }
	tickNow(c)
	assert.Len(t, store.rolledUp, 1, "the previous hour is rolled up once")
	assert.Len(t, store.sampleCutoffs, 1, "retention runs at most once an hour")

	c.now = func() time.Time { return now.Add(time.Hour) }
	tickNow(c)
	assert.Equal(t, []time.Time{previous, previous.Add(time.Hour)}, store.rolledUp)
	assert.Len(t, store.sampleCutoffs, 2)
	assert.Len(t, store.hourlyCutoffs, 2)
}

func TestUsageCollectorRetriesAFailedRollupOnTheNextTick(t *testing.T) {
	now := time.Date(2026, 11, 20, 11, 0, 10, 0, time.UTC)
	c, store, _ := newTestUsageCollector(t, now, usageTestWorkspace("ws1", "c1"))
	ten := time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC)
	store.failRollups = map[time.Time]int{ten: 1}

	tickNow(c)
	assert.Equal(t, ten.Add(time.Hour), c.rolledUpTo, "the watermark moves past the failed hour")
	assert.Equal(t, map[time.Time]int{ten: 1}, c.failedHours)

	c.now = func() time.Time { return now.Add(5 * time.Minute) }
	tickNow(c)
	assert.Equal(t, []time.Time{ten, ten}, store.rolledUp)
	assert.Empty(t, c.failedHours)

	c.now = func() time.Time { return now.Add(time.Hour) }
	tickNow(c)
	assert.Equal(t, []time.Time{ten, ten, ten.Add(time.Hour)}, store.rolledUp)
}

func TestUsageCollectorGivesUpAnHourThatKeepsFailing(t *testing.T) {
	now := time.Date(2026, 11, 20, 11, 0, 10, 0, time.UTC)
	c, store, _ := newTestUsageCollector(t, now, usageTestWorkspace("ws1", "c1"))
	ten := time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC)
	store.failRollups = map[time.Time]int{ten: 100}

	for i := 0; i <= 12; i++ {
		c.now = func() time.Time { return now.Add(time.Duration(i) * 5 * time.Minute) }
		tickNow(c)
	}
	attempts := 0
	for _, hour := range store.rolledUp {
		if hour.Equal(ten) {
			attempts++
		}
	}
	assert.Equal(t, usageRollupAttempts, attempts)
	assert.Empty(t, c.failedHours)
	assert.Contains(t, store.rolledUp, ten.Add(time.Hour), "the failing hour does not block the next one")
	assert.Equal(t, ten.Add(2*time.Hour), c.rolledUpTo)
}

func TestUsageCollectorGivesUpAFailedHourPastTheSampleRetention(t *testing.T) {
	now := time.Date(2026, 11, 20, 11, 0, 10, 0, time.UTC)
	c, store, _ := newTestUsageCollector(t, now, usageTestWorkspace("ws1", "c1"))
	c.sampleRetention = 2 * time.Hour
	ten := time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC)
	store.failRollups = map[time.Time]int{ten: 1}

	tickNow(c)
	require.Equal(t, map[time.Time]int{ten: 1}, c.failedHours)

	c.now = func() time.Time { return now.Add(time.Hour + 30*time.Minute) }
	tickNow(c)
	assert.Equal(t, []time.Time{ten, ten.Add(time.Hour)}, store.rolledUp, "the hour is not tried once its samples are pruned")
	assert.Empty(t, c.failedHours)
}

func TestUsageCollectorTickKeepsRollingUpAfterSampleFailure(t *testing.T) {
	now := time.Date(2026, 11, 20, 11, 0, 10, 0, time.UTC)
	c, store, _ := newTestUsageCollector(t, now, usageTestWorkspace("ws1", "c1"))
	store.upsertErr = errors.New("db down")
	tickNow(c)
	assert.Len(t, store.rolledUp, 1)
}

func TestUsageCollectorBackfill(t *testing.T) {
	now := time.Date(2026, 11, 20, 11, 20, 0, 0, time.UTC)
	c, store, _ := newTestUsageCollector(t, now)
	current := now.Truncate(time.Hour)
	// Every hour of the 14 day sample retention but the last is rolled up.
	for h := 2; h < 14*24; h++ {
		store.hours = append(store.hours, current.Add(-time.Duration(h)*time.Hour))
	}
	c.backfill(context.Background())
	assert.Equal(t, []time.Time{current.Add(-time.Hour)}, store.rolledUp)
	assert.Equal(t, current, c.rolledUpTo)

	// The first tick in the same hour does not roll up the previous hour again.
	tickNow(c)
	assert.Len(t, store.rolledUp, 1)
}

func TestUsageCollectorBackfillContinuesPastAFailedHour(t *testing.T) {
	now := time.Date(2026, 11, 20, 11, 20, 0, 0, time.UTC)
	c, store, _ := newTestUsageCollector(t, now)
	c.sampleRetention = 4 * time.Hour
	current := now.Truncate(time.Hour)
	store.failRollups = map[time.Time]int{current.Add(-2 * time.Hour): 1}

	c.backfill(context.Background())
	assert.Equal(t, []time.Time{current.Add(-3 * time.Hour), current.Add(-2 * time.Hour), current.Add(-time.Hour)},
		store.rolledUp, "the partly pruned oldest hour is left out and the failure does not stop the rest")
	assert.Equal(t, current, c.rolledUpTo)
	assert.Equal(t, map[time.Time]int{current.Add(-2 * time.Hour): 1}, c.failedHours)

	tickNow(c)
	assert.Equal(t, []time.Time{current.Add(-2 * time.Hour)}, store.rolledUp[3:], "the next tick retries only the failed hour")
	assert.Empty(t, c.failedHours)
	assert.Equal(t, current, c.rolledUpTo)
}

func TestUsageRetentionCutoffs(t *testing.T) {
	now := time.Date(2026, 11, 20, 11, 20, 0, 0, time.UTC)
	sample, hourly := usageRetentionCutoffs(now, 14*24*time.Hour, 400*24*time.Hour)
	assert.Equal(t, time.Date(2026, 11, 6, 11, 20, 0, 0, time.UTC), sample)
	assert.Equal(t, time.Date(2025, 10, 16, 11, 0, 0, 0, time.UTC), hourly)

	sample, hourly = usageRetentionCutoffs(now, 0, -1)
	assert.True(t, sample.IsZero(), "no retention keeps the samples")
	assert.True(t, hourly.IsZero())
}

func TestUsageCollectorUsesTheSharedWindowRule(t *testing.T) {
	now := time.Date(2026, 11, 20, 10, 7, 30, 0, time.UTC)
	c, store, windows := newTestUsageCollector(t, now,
		usageTestWorkspace("ws1", "c1"), usageTestWorkload("a", "ws1"))
	windows.byCluster["c1"] = []robustclient.WorkloadGPUWindow{
		{UID: "a-uid", Overall: robustclient.GPUWindowOverall{Max: 90, Avg: 25, Samples: 4, ExpectedSamples: 5}},
	}

	require.NoError(t, c.sample(context.Background(), now.Truncate(c.interval)))

	gpu := usageRowOf(store.samples, "ws1", common.AmdGpu)
	require.NotNil(t, gpu)
	assert.Nil(t, gpu.Busy, "80% of the expected samples is not covered")
	require.NotNil(t, gpu.UtilCoverage)
	assert.Zero(t, *gpu.UtilCoverage)
	assert.Equal(t, []string{robustclient.GPUWindowStep}, windows.steps)
}

func TestUsageCollectorWaitsForTheDatabase(t *testing.T) {
	minDelay, maxDelay := usageStoreRetryMin, usageStoreRetryMax
	usageStoreRetryMin, usageStoreRetryMax = time.Millisecond, 2*time.Millisecond
	defer func() { usageStoreRetryMin, usageStoreRetryMax = minDelay, maxDelay }()

	store := &fakeUsageStore{}
	attempts := 0
	c := &WorkspaceUsageCollector{connect: func() workspaceUsageStore {
		if attempts++; attempts < 3 {
			return nil
		}
		return store
	}}
	require.True(t, c.waitForStore(context.Background()))
	assert.Equal(t, 3, attempts, "a failed connection at startup is tried again")
	assert.Same(t, store, c.store)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	never := &WorkspaceUsageCollector{connect: func() workspaceUsageStore { return nil }}
	assert.False(t, never.waitForStore(ctx))
}

func TestUsageCollectorRunsOnLeaderOnly(t *testing.T) {
	assert.True(t, (&WorkspaceUsageCollector{}).NeedLeaderElection())
}
