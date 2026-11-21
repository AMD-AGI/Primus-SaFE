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
	usageGPUStep = "1m"
	// How far back the hourly rollup is completed when the collector starts.
	usageBackfillWindow = 24 * time.Hour

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
	interval        time.Duration
	sampleRetention time.Duration
	hourlyRetention time.Duration
	now             func() time.Time

	// The hour whose previous hour has been rolled up, and the last time retention ran.
	rolledUpTo   time.Time
	lastPrunedAt time.Time
}

// SetupWorkspaceUsageCollector registers the collector. It is skipped without a database.
func SetupWorkspaceUsageCollector(mgr manager.Manager, rc *robustclient.Client, store workspaceUsageStore) error {
	if store == nil {
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
		store:           store,
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

// Start completes the hourly rollup of the last day, then samples every interval.
func (c *WorkspaceUsageCollector) Start(ctx context.Context) error {
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

// tick writes one sample, rolls up the previous hour once a new hour has begun and applies the
// retention at most once an hour.
func (c *WorkspaceUsageCollector) tick(ctx context.Context) {
	now := c.now().UTC()
	if err := c.sample(ctx, now.Truncate(c.interval)); err != nil {
		klog.ErrorS(err, "[workspace-usage-collector] failed to sample")
	} else {
		usageCollectorLastSuccess.Set(float64(now.Unix()))
	}
	if hour := now.Truncate(time.Hour); hour.After(c.rolledUpTo) {
		if err := c.rollup(ctx, hour.Add(-time.Hour)); err != nil {
			klog.ErrorS(err, "[workspace-usage-collector] failed to roll up", "hour", hour.Add(-time.Hour))
		} else {
			c.rolledUpTo = hour
		}
	}
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
	for _, cluster := range snapshot.Clusters() {
		snapshot.ApplyGPUUtilization(cluster, c.gpuUtilization(ctx, cluster, snapshot.GPUWorkloadUIDs(cluster)))
	}
	if err := c.store.UpsertWorkspaceUsageSamples(ctx, snapshot.SortedRows()); err != nil {
		usageCollectorErrors.WithLabelValues(usageStageUpsert).Inc()
		return err
	}
	return nil
}

// gpuUtilization returns the average GPU utilization over the last interval of each workload
// that has enough data, or nil when Robust cannot be asked about the cluster.
func (c *WorkspaceUsageCollector) gpuUtilization(ctx context.Context, cluster string, uids []string) map[string]float64 {
	result := map[string]float64{}
	if len(uids) == 0 {
		return result
	}
	windows, err := c.source.GetWorkloadGPUWindow(ctx, cluster, uids, usageWindow(c.interval), usageGPUStep)
	if err != nil {
		usageCollectorErrors.WithLabelValues(usageStageRobust).Inc()
		klog.Warningf("[workspace-usage-collector] failed to get the GPU utilization of cluster %s: %v", cluster, err)
		return nil
	}
	for i := range windows {
		if avg, ok := usage.WindowAverage(&windows[i]); ok {
			result[windows[i].UID] = avg
		}
	}
	return result
}

// usageWindow is the Robust window covering one interval, in whole minutes since the step is a minute.
func usageWindow(interval time.Duration) string {
	minutes := int((interval + time.Minute - 1) / time.Minute)
	return fmt.Sprintf("%dm", max(1, minutes))
}

// rollup writes the hourly rows of hour from its samples.
func (c *WorkspaceUsageCollector) rollup(ctx context.Context, hour time.Time) error {
	samples, err := c.store.ListWorkspaceUsageSamples(ctx, dbclient.WorkspaceUsageFilter{
		Start: hour,
		End:   hour.Add(time.Hour - time.Microsecond),
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

// backfill rolls up the hours of the last day that have no hourly rows, for example because the
// collector was not running when they ended.
func (c *WorkspaceUsageCollector) backfill(ctx context.Context) {
	current := c.now().UTC().Truncate(time.Hour)
	from := current.Add(-usageBackfillWindow)
	present, err := c.store.ListWorkspaceUsageHours(ctx, from, current.Add(-time.Microsecond))
	if err != nil {
		usageCollectorErrors.WithLabelValues(usageStageRollup).Inc()
		klog.ErrorS(err, "[workspace-usage-collector] failed to list the rolled up hours")
		return
	}
	for _, hour := range usage.MissingHours(from, current, present) {
		if err = c.rollup(ctx, hour); err != nil {
			klog.ErrorS(err, "[workspace-usage-collector] failed to roll up", "hour", hour)
			return
		}
	}
	c.rolledUpTo = current
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
