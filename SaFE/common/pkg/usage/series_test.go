/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package usage

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/model"
)

var (
	t0       = time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC)
	interval = 5 * time.Minute
)

func sample(at time.Time, workspace string, quota, allocated float64, busy, coverage *float64, queued float64) *model.WorkspaceUsageSample {
	return &model.WorkspaceUsageSample{
		SampleTime: at, Cluster: "c1", Workspace: workspace, Resource: common.AmdGpu,
		Quota: quota, Allocated: allocated, Busy: busy, UtilCoverage: coverage, QueuedQuota: queued,
	}
}

func f(v float64) *float64 { return &v }

func TestRollup(t *testing.T) {
	samples := []*model.WorkspaceUsageSample{
		sample(t0, "ws", 32, 16, f(8), f(1), 8),
		sample(t0.Add(5*time.Minute), "ws", 32, 24, f(6), f(0.5), 0),
		// Robust was down: busy and coverage unknown.
		sample(t0.Add(10*time.Minute), "ws", 32, 8, nil, nil, 16),
	}
	h := Rollup(t0, samples, interval)
	require.NotNil(t, h)
	assert.Equal(t, t0, h.Hour)
	assert.Equal(t, int32(3), h.Samples)
	assert.InDelta(t, 32, h.QuotaAvg, 1e-9)
	assert.InDelta(t, 16, h.AllocatedAvg, 1e-9)
	assert.InDelta(t, 24, h.AllocatedMax, 1e-9)
	assert.InDelta(t, 8, h.QueuedAvg, 1e-9)
	assert.InDelta(t, 16, h.QueuedMax, 1e-9)
	// 48 GPUs over three 5 minute samples.
	assert.InDelta(t, 4, h.AllocatedHours, 1e-9)
	require.NotNil(t, h.BusyHours)
	assert.InDelta(t, 14.0/12.0, *h.BusyHours, 1e-9)
	require.NotNil(t, h.BusyAvg)
	assert.InDelta(t, 14.0/3.0, *h.BusyAvg, 1e-9)
	// 16 + 12 covered out of 48 allocated.
	require.NotNil(t, h.UtilCoverageAvg)
	assert.InDelta(t, 28.0/48.0, *h.UtilCoverageAvg, 1e-9)

	// The utilization derived from the hourly row is the busy over the covered allocation.
	p := NewPoint(FromHourly(h))
	require.NotNil(t, p.Utilization)
	assert.InDelta(t, 14.0/28.0, *p.Utilization, 1e-9)

	assert.Nil(t, Rollup(t0, nil, interval))
}

func TestRollupWithoutAnyData(t *testing.T) {
	h := Rollup(t0, []*model.WorkspaceUsageSample{
		sample(t0, "ws", 32, 16, nil, f(0), 0),
		sample(t0.Add(5*time.Minute), "ws", 32, 16, nil, nil, 0),
	}, interval)
	assert.Nil(t, h.BusyAvg)
	assert.Nil(t, h.BusyHours)
	require.NotNil(t, h.UtilCoverageAvg)
	assert.Zero(t, *h.UtilCoverageAvg)
}

func TestRollupIdleWorkspace(t *testing.T) {
	h := Rollup(t0, []*model.WorkspaceUsageSample{sample(t0, "ws", 32, 0, f(0), nil, 0)}, interval)
	require.NotNil(t, h.BusyAvg)
	assert.Zero(t, *h.BusyAvg)
	assert.Nil(t, h.UtilCoverageAvg)
}

func TestRollupHourGroupsByKey(t *testing.T) {
	rows := RollupHour(t0, []*model.WorkspaceUsageSample{
		sample(t0, "b", 8, 8, f(4), f(1), 0),
		sample(t0, "a", 8, 4, f(1), f(1), 0),
		sample(t0.Add(5*time.Minute), "a", 8, 8, f(2), f(1), 0),
	}, interval)
	require.Len(t, rows, 2)
	assert.Equal(t, "a", rows[0].Workspace)
	assert.Equal(t, int32(2), rows[0].Samples)
	assert.InDelta(t, 6, rows[0].AllocatedAvg, 1e-9)
	assert.Equal(t, "b", rows[1].Workspace)
}

func TestPointRatios(t *testing.T) {
	p := NewPoint(FromSample(sample(t0, "ws", 32, 16, f(6), f(0.75), 0), interval))
	require.NotNil(t, p.Occupancy)
	assert.InDelta(t, 0.5, *p.Occupancy, 1e-9)
	require.NotNil(t, p.Utilization)
	assert.InDelta(t, 0.5, *p.Utilization, 1e-9)
	require.NotNil(t, p.EffectiveUtilization)
	assert.InDelta(t, 6.0/32.0, *p.EffectiveUtilization, 1e-9)
	require.NotNil(t, p.QueuedQuota)

	zero := NewPoint(FromSample(sample(t0, "ws", 0, 0, f(0), nil, 0), interval))
	assert.Nil(t, zero.Occupancy, "no quota")
	assert.Nil(t, zero.Utilization, "nothing allocated")
	assert.Nil(t, zero.EffectiveUtilization, "no quota")
	assert.Nil(t, zero.Coverage)

	unknown := NewPoint(FromSample(sample(t0, "ws", 32, 16, nil, nil, 0), interval))
	assert.Nil(t, unknown.Busy)
	assert.Nil(t, unknown.Utilization)
	assert.Nil(t, unknown.EffectiveUtilization, "unknown busy is not 0%")
	assert.NotNil(t, unknown.Occupancy)

	hourly := NewPoint(FromHourly(&model.WorkspaceUsageHourly{Hour: t0, QuotaAvg: 8, AllocatedAvg: 4, QueuedAvg: 2}))
	assert.Nil(t, hourly.QueuedQuota, "hourly rows keep no split")
	assert.Equal(t, 2.0, hourly.Queued)
}

func TestSumByTime(t *testing.T) {
	records := []Record{
		FromSample(sample(t0.Add(5*time.Minute), "a", 8, 8, f(4), f(1), 0), interval),
		FromSample(sample(t0, "a", 16, 8, f(2), f(0.5), 1), interval),
		FromSample(sample(t0, "b", 16, 8, nil, nil, 2), interval),
		FromSample(sample(t0, "c", 16, 0, f(0), nil, 0), interval),
	}
	sums := SumByTime(records)
	require.Len(t, sums, 2)
	first := sums[0]
	assert.Equal(t, t0, first.Time)
	assert.Equal(t, 48.0, first.Quota)
	assert.Equal(t, 16.0, first.Allocated)
	assert.Equal(t, 4.0, first.Covered)
	assert.Equal(t, 3.0, first.Queued)
	require.NotNil(t, first.Busy)
	assert.Equal(t, 2.0, *first.Busy)
	p := NewPoint(first)
	assert.InDelta(t, 0.25, *p.Coverage, 1e-9)
	assert.InDelta(t, 0.5, *p.Utilization, 1e-9)
	assert.Equal(t, "", first.Workspace)
	assert.NotNil(t, records[1].Busy)
	assert.Equal(t, 2.0, *records[1].Busy, "the input is not changed")

	noData := SumByTime([]Record{
		FromSample(sample(t0, "b", 16, 8, nil, nil, 0), interval),
		FromSample(sample(t0, "c", 16, 0, f(0), nil, 0), interval),
	})
	assert.Nil(t, noData[0].Busy, "an idle workspace's zero does not make the cluster idle")
}

func TestBucket(t *testing.T) {
	var records []Record
	for i := 0; i < 6; i++ {
		busy := f(float64(i))
		coverage := f(1)
		if i == 2 {
			busy, coverage = nil, nil
		}
		records = append(records, FromSample(sample(t0.Add(time.Duration(i)*interval), "ws", 32, 8, busy, coverage, float64(i)), interval))
	}
	points := Bucket(records, t0, 15*time.Minute)
	require.Len(t, points, 2)
	assert.Equal(t, t0, points[0].Time)
	assert.Equal(t, t0.Add(15*time.Minute), points[1].Time)
	assert.InDelta(t, 1, points[0].Queued, 1e-9)
	require.NotNil(t, points[0].QueuedQuota)
	assert.InDelta(t, 1, *points[0].QueuedQuota, 1e-9)
	// Busy 0 and 1 known, the third sample unknown: averaged over the bucket.
	assert.InDelta(t, 1.0/3.0, *points[0].Busy, 1e-9)
	assert.InDelta(t, 16.0/24.0, *points[0].Coverage, 1e-9)
	assert.InDelta(t, 1.0/16.0, *points[0].Utilization, 1e-9)
	assert.InDelta(t, 4, *points[1].Busy, 1e-9)
	assert.InDelta(t, 0.5, *points[1].Utilization, 1e-9)

	assert.Len(t, Bucket(records, t0.Add(time.Hour), 15*time.Minute), 0, "records before start are left out")
	gap := Bucket([]Record{records[0], records[5]}, t0, 5*time.Minute)
	assert.Len(t, gap, 2, "empty buckets are left out")
}

func TestSummarize(t *testing.T) {
	var records []Record
	for i := 0; i < 4; i++ {
		records = append(records, FromHourly(&model.WorkspaceUsageHourly{
			Hour: t0.Add(time.Duration(i) * time.Hour), QuotaAvg: 10, AllocatedAvg: float64(2 * i), AllocatedMax: float64(2 * i),
			BusyAvg: f(float64(i)), UtilCoverageAvg: f(1), QueuedAvg: float64(i), QueuedMax: float64(3 * i),
			AllocatedHours: float64(2 * i), BusyHours: f(float64(i)), Samples: 12,
		}))
	}
	s := Summarize(records)
	assert.InDelta(t, 0.3, *s.Occupancy, 1e-9)
	assert.InDelta(t, 0.5, *s.Utilization, 1e-9)
	assert.InDelta(t, 0.15, *s.EffectiveUtilization, 1e-9)
	assert.InDelta(t, 12, s.AllocatedHours, 1e-9)
	assert.InDelta(t, 6, *s.BusyHours, 1e-9)
	assert.InDelta(t, 1.5, s.QueuedAvg, 1e-9)
	assert.InDelta(t, 9, s.QueuedMax, 1e-9)

	empty := Summarize(nil)
	assert.Nil(t, empty.Occupancy)
	assert.Nil(t, empty.BusyHours)
}

func TestMissingHours(t *testing.T) {
	from := t0.Add(-3 * time.Hour)
	got := MissingHours(from, t0, []time.Time{t0.Add(-2 * time.Hour)})
	assert.Equal(t, []time.Time{t0.Add(-3 * time.Hour), t0.Add(-time.Hour)}, got)
	assert.Empty(t, MissingHours(t0, t0, nil))
}
