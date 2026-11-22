/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resource

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/model"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/robustclient"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/usage"
)

const (
	// How far back the hourly rollup is completed when the collector starts and the samples are
	// kept forever.
	usageBackfillWindow = 24 * time.Hour
	// The longest a sample waits for Robust, across all clusters; at most half the interval.
	usageRobustTimeout = 30 * time.Second

	usageStageList      = "list"
	usageStageRobust    = "robust"
	usageStageUpsert    = "upsert"
	usageStageRollup    = "rollup"
	usageStageRetention = "retention"
)

var (
	usageCollectorErrors = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "safe_workspace_usage_collector_errors_total",
		Help: "Total number of failed workspace usage collector operations",
	}, []string{"stage"})

	usageCollectorLastSuccess = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "safe_workspace_usage_collector_last_success_timestamp_seconds",
		Help: "Unix time of the last workspace usage sample that was written",
	})

	errUsageNoRobustClient = errors.New("no Robust client for the cluster")

	// How long the collector waits before it tries the database again, doubling up to the max.
	usageStoreRetryMin = 10 * time.Second
	usageStoreRetryMax = 5 * time.Minute
)

// workspaceUsageStore is the part of the database the usage collector writes.
type workspaceUsageStore interface {
	UpsertWorkspaceUsageSamples(ctx context.Context, rows []*model.WorkspaceUsageSample) error
	ListWorkspaceUsageSamples(ctx context.Context, filter dbclient.WorkspaceUsageFilter) ([]*model.WorkspaceUsageSample, error)
	DeleteWorkspaceUsageSamplesBefore(ctx context.Context, before time.Time) (int64, error)
	UpsertWorkspaceUsageHourly(ctx context.Context, rows []*model.WorkspaceUsageHourly) error
	ListWorkspaceUsageHours(ctx context.Context, start, end time.Time) ([]time.Time, error)
	DeleteWorkspaceUsageHourlyBefore(ctx context.Context, before time.Time) (int64, error)
}

// usageGPUWindowSource returns the GPU utilization windows of workloads in one cluster.
type usageGPUWindowSource interface {
	GetWorkloadGPUWindow(ctx context.Context, cluster string,
		uids []string, window, step string) ([]robustclient.WorkloadGPUWindow, error)
}

type robustUsageWindowSource struct {
	rc *robustclient.Client
}

func (s robustUsageWindowSource) GetWorkloadGPUWindow(ctx context.Context, cluster string,
	uids []string, window, step string) ([]robustclient.WorkloadGPUWindow, error) {
	if s.rc == nil {
		return nil, errUsageNoRobustClient
	}
	cc := s.rc.ForCluster(cluster)
	if cc == nil {
		return nil, errUsageNoRobustClient
	}
	return cc.GetWorkloadGPUWindow(ctx, uids, window, step)
}

// WorkspaceUsageCollector samples the quota, allocation, queue and GPU busy of every workspace,
// rolls the samples up by hour and prunes both past their retention.
type WorkspaceUsageCollector struct {
	client.Client
	source          usageGPUWindowSource
	store           workspaceUsageStore
	connect         func() workspaceUsageStore
	interval        time.Duration
	sampleRetention time.Duration
	hourlyRetention time.Duration
	now             func() time.Time

	// The first hour that is not rolled up yet, and the last time retention ran.
	rolledUpTo   time.Time
	lastPrunedAt time.Time
	// The sample time of the last sample written; a tick never writes the same one twice.
	lastSampleTime time.Time
}

// SetupWorkspaceUsageCollector registers the collector. It is skipped when the database is not
// enabled, that is when connect is nil. connect returns the store, or nil while the database
// cannot be reached.
func SetupWorkspaceUsageCollector(mgr manager.Manager, rc *robustclient.Client, connect func() workspaceUsageStore) error {
	if connect == nil {
		klog.Info("[workspace-usage-collector] database not enabled, skipping")
		return nil
	}
	interval := time.Duration(commonconfig.GetWorkloadUsageSampleIntervalSecond()) * time.Second
	if interval <= 0 {
		interval = 300 * time.Second
	}
	c := &WorkspaceUsageCollector{
		Client:          mgr.GetClient(),
		source:          robustUsageWindowSource{rc: rc},
		connect:         connect,
		interval:        interval,
		sampleRetention: days(commonconfig.GetWorkloadUsageSampleRetentionDay()),
		hourlyRetention: days(commonconfig.GetWorkloadUsageHourlyRetentionDay()),
		now:             time.Now,
	}
	if err := mgr.Add(c); err != nil {
		return err
	}
	klog.Info("[workspace-usage-collector] registered")
	return nil
}

func days(n int) time.Duration {
	return time.Duration(n) * 24 * time.Hour
}

// NeedLeaderElection makes the collector run only on the elected leader.
func (c *WorkspaceUsageCollector) NeedLeaderElection() bool { return true }

// Start waits for the database, completes the hourly rollup within the sample retention, then
// samples every interval.
func (c *WorkspaceUsageCollector) Start(ctx context.Context) error {
	if !c.waitForStore(ctx) {
		return nil
	}
	c.backfill(ctx)
	c.tick(ctx)
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			c.tick(ctx)
		}
	}
}

// waitForStore connects to the database, trying again with backoff until it succeeds. It returns
// false when ctx ends first.
func (c *WorkspaceUsageCollector) waitForStore(ctx context.Context) bool {
	delay := usageStoreRetryMin
	for c.store == nil {
		if c.store = c.connect(); c.store != nil {
			break
		}
		klog.Warningf("[workspace-usage-collector] database not reachable, retrying in %s", delay)
		select {
		case <-ctx.Done():
			return false
		case <-time.After(delay):
		}
		delay = min(2*delay, usageStoreRetryMax)
	}
	return true
}

// tick writes one sample, rolls up the hours that ended since the last rollup and applies the
// retention at most once an hour.
func (c *WorkspaceUsageCollector) tick(ctx context.Context) {
	now := c.now().UTC()
	if sampleTime := now.Truncate(c.interval); !sampleTime.After(c.lastSampleTime) {
		klog.Infof("[workspace-usage-collector] sample at %s already written", sampleTime.Format(time.RFC3339))
	} else if err := c.sample(ctx, sampleTime); err != nil {
		klog.ErrorS(err, "[workspace-usage-collector] failed to sample")
	} else {
		c.lastSampleTime = sampleTime
		usageCollectorLastSuccess.Set(float64(now.Unix()))
	}
	c.rollupPending(ctx, now.Truncate(time.Hour))
	if now.Sub(c.lastPrunedAt) >= time.Hour {
		c.prune(ctx, now)
		c.lastPrunedAt = now
	}
}

// sample writes the usage of every workspace at sampleTime.
func (c *WorkspaceUsageCollector) sample(ctx context.Context, sampleTime time.Time) error {
	workspaces := &v1.WorkspaceList{}
	if err := c.List(ctx, workspaces); err != nil {
		usageCollectorErrors.WithLabelValues(usageStageList).Inc()
		return fmt.Errorf("failed to list workspaces: %w", err)
	}
	workloads := &v1.WorkloadList{}
	if err := c.List(ctx, workloads); err != nil {
		usageCollectorErrors.WithLabelValues(usageStageList).Inc()
		return fmt.Errorf("failed to list workloads: %w", err)
	}
	snapshot := usage.NewSnapshot(sampleTime, workspaces.Items, workloads.Items)
	if snapshot.Skipped > 0 {
		klog.Warningf("[workspace-usage-collector] skipped %d workloads whose request could not be read", snapshot.Skipped)
	}
	utilization := c.clusterUtilization(ctx, snapshot)
	for _, cluster := range snapshot.Clusters() {
		snapshot.ApplyGPUUtilization(cluster, utilization[cluster])
	}
	if err := c.store.UpsertWorkspaceUsageSamples(ctx, snapshot.SortedRows()); err != nil {
		usageCollectorErrors.WithLabelValues(usageStageUpsert).Inc()
		return err
	}
	return nil
}

// clusterUtilization asks Robust about every cluster of snapshot in parallel within
// robustTimeout. A cluster that has not answered by then has no entry, so its utilization is
// unknown.
func (c *WorkspaceUsageCollector) clusterUtilization(ctx context.Context, snapshot *usage.Snapshot) map[string]map[string]float64 {
	type answer struct {
		cluster     string
		utilization map[string]float64
	}
	ctx, cancel := context.WithTimeout(ctx, c.robustTimeout())
	defer cancel()
	clusters := snapshot.Clusters()
	answers := make(chan answer, len(clusters))
	for _, cluster := range clusters {
		uids := snapshot.GPUWorkloadUIDs(cluster)
		go func() {
			answers <- answer{cluster: cluster, utilization: c.gpuUtilization(ctx, cluster, uids)}
		}()
	}
	result := make(map[string]map[string]float64, len(clusters))
	for range clusters {
		select {
		case a := <-answers:
			result[a.cluster] = a.utilization
		case <-ctx.Done():
			usageCollectorErrors.WithLabelValues(usageStageRobust).Add(float64(len(clusters) - len(result)))
			klog.Warningf("[workspace-usage-collector] %d of %d clusters did not report GPU utilization within %s",
				len(clusters)-len(result), len(clusters), c.robustTimeout())
			return result
		}
	}
	return result
}

func (c *WorkspaceUsageCollector) robustTimeout() time.Duration {
	return min(usageRobustTimeout, c.interval/2)
}

// gpuUtilization returns the average GPU utilization over the last interval of each workload
// whose window is covered, or nil when Robust cannot be asked about the cluster.
func (c *WorkspaceUsageCollector) gpuUtilization(ctx context.Context, cluster string, uids []string) map[string]float64 {
	result := map[string]float64{}
	if len(uids) == 0 {
		return result
	}
	windows, err := c.source.GetWorkloadGPUWindow(ctx, cluster, uids,
		robustclient.WindowFor(c.interval), robustclient.GPUWindowStep)
	if err != nil {
		usageCollectorErrors.WithLabelValues(usageStageRobust).Inc()
		klog.Warningf("[workspace-usage-collector] failed to get the GPU utilization of cluster %s: %v", cluster, err)
		return nil
	}
	for i := range windows {
		if avg, ok := windows[i].Average(); ok {
			result[windows[i].UID] = avg
		}
	}
	return result
}

// rollup writes the hourly rows of hour from its samples.
func (c *WorkspaceUsageCollector) rollup(ctx context.Context, hour time.Time) error {
	samples, err := c.store.ListWorkspaceUsageSamples(ctx, dbclient.WorkspaceUsageFilter{
		Start: hour,
		End:   hour.Add(time.Hour),
	})
	if err == nil {
		err = c.store.UpsertWorkspaceUsageHourly(ctx, usage.RollupHour(hour, samples, c.interval))
	}
	if err != nil {
		usageCollectorErrors.WithLabelValues(usageStageRollup).Inc()
		return err
	}
	return nil
}

// rollupPending rolls up every hour from the first one not rolled up to the one before current,
// in order. It stops at the first failure, which is tried again on the next tick.
func (c *WorkspaceUsageCollector) rollupPending(ctx context.Context, current time.Time) {
	if c.rolledUpTo.IsZero() {
		c.rolledUpTo = current.Add(-time.Hour)
	}
	for _, hour := range usage.MissingHours(c.rolledUpTo, current, nil) {
		if err := c.rollup(ctx, hour); err != nil {
			klog.ErrorS(err, "[workspace-usage-collector] failed to roll up", "hour", hour)
			return
		}
		c.rolledUpTo = hour.Add(time.Hour)
	}
}

// backfill rolls up the hours within the sample retention that have no hourly rows, for example
// because the collector was not running when they ended. The oldest hour, whose first samples
// may be pruned already, is left out. A failed hour does not stop the others; it stays pending
// for the next tick.
func (c *WorkspaceUsageCollector) backfill(ctx context.Context) {
	current := c.now().UTC().Truncate(time.Hour)
	window := c.sampleRetention
	if window <= 0 {
		window = usageBackfillWindow
	}
	from := current.Add(-window).Truncate(time.Hour).Add(time.Hour)
	present, err := c.store.ListWorkspaceUsageHours(ctx, from, current)
	if err != nil {
		usageCollectorErrors.WithLabelValues(usageStageRollup).Inc()
		klog.ErrorS(err, "[workspace-usage-collector] failed to list the rolled up hours")
		return
	}
	c.rolledUpTo = current
	for _, hour := range usage.MissingHours(from, current, present) {
		if err = c.rollup(ctx, hour); err != nil {
			klog.ErrorS(err, "[workspace-usage-collector] failed to roll up", "hour", hour)
			if hour.Before(c.rolledUpTo) {
				c.rolledUpTo = hour
			}
		}
	}
}

// prune deletes the samples and hourly rows past their retention.
func (c *WorkspaceUsageCollector) prune(ctx context.Context, now time.Time) {
	sampleCutoff, hourlyCutoff := usageRetentionCutoffs(now, c.sampleRetention, c.hourlyRetention)
	if !sampleCutoff.IsZero() {
		if n, err := c.store.DeleteWorkspaceUsageSamplesBefore(ctx, sampleCutoff); err != nil {
			usageCollectorErrors.WithLabelValues(usageStageRetention).Inc()
			klog.ErrorS(err, "[workspace-usage-collector] failed to prune samples")
		} else if n > 0 {
			klog.Infof("[workspace-usage-collector] pruned %d samples before %s", n, sampleCutoff.Format(time.RFC3339))
		}
	}
	if !hourlyCutoff.IsZero() {
		if n, err := c.store.DeleteWorkspaceUsageHourlyBefore(ctx, hourlyCutoff); err != nil {
			usageCollectorErrors.WithLabelValues(usageStageRetention).Inc()
			klog.ErrorS(err, "[workspace-usage-collector] failed to prune hourly rows")
		} else if n > 0 {
			klog.Infof("[workspace-usage-collector] pruned %d hourly rows before %s", n, hourlyCutoff.Format(time.RFC3339))
		}
	}
}

// usageRetentionCutoffs returns the times before which samples and hourly rows are deleted. A
// retention that is not positive keeps the rows forever and has a zero cutoff.
func usageRetentionCutoffs(now time.Time, sampleRetention, hourlyRetention time.Duration) (time.Time, time.Time) {
	var sampleCutoff, hourlyCutoff time.Time
	if sampleRetention > 0 {
		sampleCutoff = now.Add(-sampleRetention)
	}
	if hourlyRetention > 0 {
		hourlyCutoff = now.Add(-hourlyRetention).Truncate(time.Hour)
	}
	return sampleCutoff, hourlyCutoff
}
