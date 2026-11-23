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
	assert.InDelta(t, 900, h.ObservedSeconds, 1e-9)
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

	hourly := NewPoint(FromHourly(&model.WorkspaceUsageHourly{Hour: t0, QuotaAvg: 8, AllocatedAvg: 4, QueuedAvg: 2,
		QueuedQuotaAvg: 1.5, QueuedOtherAvg: 0.5}))
	assert.Equal(t, 2.0, hourly.Queued)
	require.NotNil(t, hourly.QueuedQuota, "hourly rows keep the split")
	assert.Equal(t, 1.5, *hourly.QueuedQuota)
	assert.Equal(t, 0.5, *hourly.QueuedOther)
	assert.Zero(t, *hourly.QueuedUserQos)
}

// splitSample is a sample queuing quota, user QoS, external and other in that order.
func splitSample(at time.Time, workspace string, split [4]float64) *model.WorkspaceUsageSample {
	s := sample(at, workspace, 32, 8, f(4), f(1), split[0])
	s.QueuedUserQos, s.QueuedExternal, s.QueuedOther = split[1], split[2], split[3]
	return s
}

func TestRollupKeepsQueuedSplit(t *testing.T) {
	samples := []*model.WorkspaceUsageSample{
		splitSample(t0, "ws", [4]float64{8, 0, 4, 0}),
		splitSample(t0.Add(5*time.Minute), "ws", [4]float64{0, 8, 4, 2}),
		splitSample(t0.Add(10*time.Minute), "ws", [4]float64{4, 4, 16, 1}),
	}
	h := Rollup(t0, samples, interval)
	assert.InDelta(t, 4, h.QueuedQuotaAvg, 1e-9)
	assert.InDelta(t, 4, h.QueuedUserQosAvg, 1e-9)
	assert.InDelta(t, 8, h.QueuedExternalAvg, 1e-9)
	assert.InDelta(t, 1, h.QueuedOtherAvg, 1e-9)
	assert.Equal(t, 8.0, h.QueuedQuotaMax)
	assert.Equal(t, 8.0, h.QueuedUserQosMax)
	assert.Equal(t, 16.0, h.QueuedExternalMax)
	assert.Equal(t, 2.0, h.QueuedOtherMax)
	assert.InDelta(t, 17, h.QueuedAvg, 1e-9)
	assert.Equal(t, 25.0, h.QueuedMax, "the total max is that of the totals, not the sum of the maxima")
}

func TestLongRangePointsCarryTheSplitLikeShortRangePoints(t *testing.T) {
	var samples []*model.WorkspaceUsageSample
	for i := 0; i < 24; i++ {
		at := t0.Add(time.Duration(i) * interval)
		samples = append(samples, splitSample(at, "a", [4]float64{float64(i), 1, float64(i % 3), 0}))
		samples = append(samples, splitSample(at, "b", [4]float64{2, float64(i % 5), 0, 3}))
	}
	var short []Record
	for _, s := range samples {
		short = append(short, FromSample(s, interval))
	}
	var long []Record
	for _, hour := range []time.Time{t0, t0.Add(time.Hour)} {
		var inHour []*model.WorkspaceUsageSample
		for _, s := range samples {
			if s.SampleTime.Truncate(time.Hour).Equal(hour) {
				inHour = append(inHour, s)
			}
		}
		for _, h := range RollupHour(hour, inHour, interval) {
			long = append(long, FromHourly(h))
		}
	}
	want := Bucket(SumByTime(short), t0, 2*time.Hour)
	got := Bucket(SumByTime(long), t0, 2*time.Hour)
	require.Len(t, want, 1)
	require.Len(t, got, 1)
	for name, pair := range map[string][2]*float64{
		"quota":    {want[0].QueuedQuota, got[0].QueuedQuota},
		"user qos": {want[0].QueuedUserQos, got[0].QueuedUserQos},
		"external": {want[0].QueuedExternal, got[0].QueuedExternal},
		"other":    {want[0].QueuedOther, got[0].QueuedOther},
	} {
		require.NotNil(t, pair[1], name)
		assert.InDelta(t, *pair[0], *pair[1], 1e-9, name)
	}
	assert.InDelta(t, want[0].Queued, got[0].Queued, 1e-9)

	sums := SumByTime(long)
	assert.Equal(t, [4]float64{11 + 2, 1 + 4, 2, 3}, sums[0].QueuedSplitMax, "the maxima of a sum add up")
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

func TestClusterQueuedMaxIsTheSimultaneousMax(t *testing.T) {
	// a and b queue 10 each, never at the same time.
	samples := []*model.WorkspaceUsageSample{
		sample(t0, "a", 8, 0, nil, nil, 10),
		sample(t0, "b", 8, 0, nil, nil, 0),
		sample(t0.Add(interval), "a", 8, 0, nil, nil, 0),
		sample(t0.Add(interval), "b", 8, 0, nil, nil, 10),
	}
	rows := RollupHour(t0, samples, interval)
	require.Len(t, rows, 2)
	var records []Record
	for _, h := range rows {
		require.NotNil(t, h.ClusterQueuedMax)
		assert.Equal(t, 10.0, *h.ClusterQueuedMax)
		assert.Equal(t, 10.0, h.QueuedMax)
		records = append(records, FromHourly(h))
	}
	sums := SumByTime(records)
	assert.Equal(t, 10.0, sums[0].QueuedMax, "not the sum of the workspace maxima")
	assert.Equal(t, 10.0, Summarize(sums).QueuedMax)

	legacy := FromHourly(&model.WorkspaceUsageHourly{Hour: t0, Cluster: "c1", Workspace: "c", QueuedMax: 4})
	assert.Equal(t, 24.0, SumByTime(append(records, legacy))[0].QueuedMax,
		"a row without the cluster max leaves the sum of the maxima")

	other := rows[0]
	other.Cluster = "c2"
	assert.Equal(t, 20.0, SumByTime(append(records, FromHourly(other)))[0].QueuedMax,
		"the maxima of several clusters add up")
}

func TestHourlyAveragesAreWeightedByObservedTime(t *testing.T) {
	hourly := func(hour time.Time, workspace string, allocated, observed float64) Record {
		return FromHourly(&model.WorkspaceUsageHourly{
			Hour: hour, Cluster: "c1", Workspace: workspace, QuotaAvg: 8, AllocatedAvg: allocated,
			BusyAvg: f(allocated / 2), UtilCoverageAvg: f(1), QueuedAvg: allocated, ObservedSeconds: observed,
		})
	}
	// b has data for a quarter of the hour only.
	sums := SumByTime([]Record{hourly(t0, "a", 8, 3600), hourly(t0, "b", 8, 900)})
	require.Len(t, sums, 1)
	assert.InDelta(t, 10, sums[0].Quota, 1e-9)
	assert.InDelta(t, 10, sums[0].Allocated, 1e-9)
	assert.InDelta(t, 10, sums[0].Covered, 1e-9)
	assert.InDelta(t, 10, sums[0].Queued, 1e-9)
	assert.InDelta(t, 5, *sums[0].Busy, 1e-9)
	assert.InDelta(t, 3600, sums[0].Weight, 1e-9)

	s := Summarize([]Record{hourly(t0, "a", 4, 3600), hourly(t0.Add(time.Hour), "a", 8, 1200)})
	assert.InDelta(t, 5.0/8.0, *s.Occupancy, 1e-9)
	assert.InDelta(t, 5, s.QueuedAvg, 1e-9)

	assert.InDelta(t, 3600, FromHourly(&model.WorkspaceUsageHourly{Hour: t0}).Weight, 1e-9,
		"a row without observed seconds weighs a whole hour")
}

func TestClip(t *testing.T) {
	r := FromSample(sample(t0, "ws", 32, 12, f(6), f(1), 0), interval)

	inside, ok := r.Clip(interval, t0, t0.Add(time.Hour))
	require.True(t, ok)
	assert.Equal(t, r, inside)

	atEnd, ok := r.Clip(interval, t0.Add(-time.Hour), t0.Add(time.Minute))
	require.True(t, ok)
	assert.Equal(t, t0, atEnd.Time)
	assert.InDelta(t, 60, atEnd.Weight, 1e-9)
	assert.InDelta(t, 12.0/60, atEnd.AllocatedHours, 1e-9)
	assert.InDelta(t, 6.0/60, *atEnd.BusyHours, 1e-9)
	assert.Equal(t, 12.0, atEnd.Allocated, "the averages stay")

	atStart, ok := r.Clip(interval, t0.Add(4*time.Minute), t0.Add(time.Hour))
	require.True(t, ok)
	assert.Equal(t, t0.Add(4*time.Minute), atStart.Time)
	assert.InDelta(t, 60, atStart.Weight, 1e-9)

	_, ok = r.Clip(interval, t0.Add(interval), t0.Add(time.Hour))
	assert.False(t, ok, "the end of a record is not in it")
	_, ok = r.Clip(interval, t0.Add(-time.Hour), t0)
	assert.False(t, ok, "the end of a range is not in it")
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
