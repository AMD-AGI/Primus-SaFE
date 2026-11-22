/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package usage

import (
	"sort"
	"time"

	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/model"
)

// Record is the usage of one workspace, or the sum of several, at one time. It is read from a
// sample or from an hourly row, whose values are the averages of its hour.
type Record struct {
	Time      time.Time
	Cluster   string
	Workspace string
	Quota     float64
	Allocated float64
	// Busy is nil when the utilization is unknown.
	Busy *float64
	// Covered is the allocation that had utilization data: allocated x coverage.
	Covered float64
	Queued  float64
	// The queued split, nil when unknown.
	QueuedQuota    *float64
	QueuedUserQos  *float64
	QueuedExternal *float64
	QueuedOther    *float64
	QueuedMax      float64
	// QueuedSplitMax are the maxima of the split: quota, user QoS, external and other.
	QueuedSplitMax [4]float64
	// The allocation and busy integrated over the time the record stands for.
	AllocatedHours float64
	BusyHours      *float64
	// Weight is the time in seconds the record has data for. The averages of several records
	// are weighted by it.
	Weight float64
}

// weight returns the weight of r; a record without one weighs as much as any other.
func (r Record) weight() float64 {
	if r.Weight > 0 {
		return r.Weight
	}
	return 1
}

// scaled returns r with its averaged values multiplied by k.
func (r Record) scaled(k float64) Record {
	r.Quota *= k
	r.Allocated *= k
	r.Covered *= k
	r.Queued *= k
	r.Busy = scale(r.Busy, k)
	r.QueuedQuota = scale(r.QueuedQuota, k)
	r.QueuedUserQos = scale(r.QueuedUserQos, k)
	r.QueuedExternal = scale(r.QueuedExternal, k)
	r.QueuedOther = scale(r.QueuedOther, k)
	return r
}

// FromSample reads a sample that stands for interval.
func FromSample(s *model.WorkspaceUsageSample, interval time.Duration) Record {
	queued := s.QueuedQuota + s.QueuedUserQos + s.QueuedExternal + s.QueuedOther
	r := Record{
		Time:           s.SampleTime,
		Cluster:        s.Cluster,
		Workspace:      s.Workspace,
		Quota:          s.Quota,
		Allocated:      s.Allocated,
		Busy:           s.Busy,
		Queued:         queued,
		QueuedQuota:    float64Ptr(s.QueuedQuota),
		QueuedUserQos:  float64Ptr(s.QueuedUserQos),
		QueuedExternal: float64Ptr(s.QueuedExternal),
		QueuedOther:    float64Ptr(s.QueuedOther),
		QueuedMax:      queued,
		QueuedSplitMax: [4]float64{s.QueuedQuota, s.QueuedUserQos, s.QueuedExternal, s.QueuedOther},
		AllocatedHours: s.Allocated * interval.Hours(),
		Weight:         interval.Seconds(),
	}
	if s.UtilCoverage != nil {
		r.Covered = s.Allocated * *s.UtilCoverage
	}
	if s.Busy != nil {
		r.BusyHours = float64Ptr(*s.Busy * interval.Hours())
	}
	return r
}

// FromHourly reads an hourly row. A row without observed seconds weighs a whole hour.
func FromHourly(h *model.WorkspaceUsageHourly) Record {
	r := Record{
		Time:           h.Hour,
		Cluster:        h.Cluster,
		Workspace:      h.Workspace,
		Quota:          h.QuotaAvg,
		Allocated:      h.AllocatedAvg,
		Busy:           h.BusyAvg,
		Queued:         h.QueuedAvg,
		QueuedQuota:    float64Ptr(h.QueuedQuotaAvg),
		QueuedUserQos:  float64Ptr(h.QueuedUserQosAvg),
		QueuedExternal: float64Ptr(h.QueuedExternalAvg),
		QueuedOther:    float64Ptr(h.QueuedOtherAvg),
		QueuedMax:      h.QueuedMax,
		QueuedSplitMax: [4]float64{h.QueuedQuotaMax, h.QueuedUserQosMax, h.QueuedExternalMax, h.QueuedOtherMax},
		AllocatedHours: h.AllocatedHours,
		BusyHours:      h.BusyHours,
		Weight:         h.ObservedSeconds,
	}
	if r.Weight <= 0 {
		r.Weight = time.Hour.Seconds()
	}
	if h.UtilCoverageAvg != nil {
		r.Covered = h.AllocatedAvg * *h.UtilCoverageAvg
	}
	return r
}

// SumByTime adds the records of the same time, so the figure of a cluster is the sum of its
// workspaces. The result is ordered by time and keeps the cluster of the first record of each
// time. The averages of a sum are over the longest weight among its records, so a workspace with
// data for part of the time counts for that part only. QueuedMax of a sum is the sum of the
// maxima.
func SumByTime(records []Record) []Record {
	byTime := map[time.Time]*Record{}
	var times []time.Time
	for _, r := range records {
		w := r.weight()
		weighted := r.scaled(w)
		sum, ok := byTime[r.Time]
		if !ok {
			weighted.Workspace = ""
			weighted.Weight = w
			weighted.BusyHours = copyPtr(r.BusyHours)
			byTime[r.Time] = &weighted
			times = append(times, r.Time)
			continue
		}
		sum.Weight = max(sum.Weight, w)
		sum.Quota += weighted.Quota
		sum.Allocated += weighted.Allocated
		sum.Covered += weighted.Covered
		sum.Queued += weighted.Queued
		sum.QueuedMax += r.QueuedMax
		for i := range sum.QueuedSplitMax {
			sum.QueuedSplitMax[i] += r.QueuedSplitMax[i]
		}
		sum.AllocatedHours += r.AllocatedHours
		sum.Busy = addKnown(sum.Busy, weighted.Busy)
		sum.BusyHours = addKnown(sum.BusyHours, r.BusyHours)
		sum.QueuedQuota = addBoth(sum.QueuedQuota, weighted.QueuedQuota)
		sum.QueuedUserQos = addBoth(sum.QueuedUserQos, weighted.QueuedUserQos)
		sum.QueuedExternal = addBoth(sum.QueuedExternal, weighted.QueuedExternal)
		sum.QueuedOther = addBoth(sum.QueuedOther, weighted.QueuedOther)
	}
	sort.Slice(times, func(i, j int) bool { return times[i].Before(times[j]) })
	result := make([]Record, 0, len(times))
	for _, t := range times {
		sum := byTime[t]
		*sum = sum.scaled(1 / sum.Weight)
		if busyUnknown(sum.Allocated, sum.Covered) {
			sum.Busy, sum.BusyHours = nil, nil
		}
		result = append(result, *sum)
	}
	return result
}

// busyUnknown reports whether GPUs are allocated but none of them had utilization data, so a
// busy that adds up to zero must not be read as idle.
func busyUnknown(allocated, covered float64) bool {
	return allocated > 0 && covered <= 0
}

// addKnown adds the known values; the sum is unknown only when both are.
func addKnown(a, b *float64) *float64 {
	switch {
	case a == nil:
		return copyPtr(b)
	case b == nil:
		return a
	default:
		return float64Ptr(*a + *b)
	}
}

// addBoth adds two values; the sum is unknown when either is.
func addBoth(a, b *float64) *float64 {
	if a == nil || b == nil {
		return nil
	}
	return float64Ptr(*a + *b)
}

func copyPtr(v *float64) *float64 {
	return scale(v, 1)
}

func scale(v *float64, k float64) *float64 {
	if v == nil {
		return nil
	}
	return float64Ptr(*v * k)
}

// Point is the usage at one time, or the average over one bucket, with the derived ratios.
// A ratio is null when its denominator is zero or its numerator is unknown.
type Point struct {
	Time                 time.Time `json:"time"`
	Quota                float64   `json:"quota"`
	Allocated            float64   `json:"allocated"`
	Busy                 *float64  `json:"busy"`
	Coverage             *float64  `json:"coverage"`
	Queued               float64   `json:"queued"`
	QueuedQuota          *float64  `json:"queuedQuota"`
	QueuedUserQos        *float64  `json:"queuedUserQos"`
	QueuedExternal       *float64  `json:"queuedExternal"`
	QueuedOther          *float64  `json:"queuedOther"`
	Occupancy            *float64  `json:"occupancy"`
	Utilization          *float64  `json:"utilization"`
	EffectiveUtilization *float64  `json:"effectiveUtilization"`
}

// accumulator averages records, weighted by their weights. Busy is averaged over every record,
// the unknown ones adding nothing, and the utilization divides it by the allocation that had
// data, so a record without data lowers the coverage, not the utilization.
type accumulator struct {
	n              int
	w              float64
	quota          float64
	allocated      float64
	allocatedMax   float64
	covered        float64
	queued         float64
	queuedMax      float64
	split          [4]float64
	splitMax       [4]float64
	splitN         int
	busy           float64
	busyN          int
	allocatedHours float64
	busyHours      float64
	busyHoursN     int
}

func (a *accumulator) add(r Record) {
	w := r.weight()
	a.n++
	a.w += w
	a.quota += r.Quota * w
	a.allocated += r.Allocated * w
	a.allocatedMax = max(a.allocatedMax, r.Allocated)
	a.covered += r.Covered * w
	a.queued += r.Queued * w
	a.queuedMax = max(a.queuedMax, r.QueuedMax)
	for i := range a.splitMax {
		a.splitMax[i] = max(a.splitMax[i], r.QueuedSplitMax[i])
	}
	if r.QueuedQuota != nil && r.QueuedUserQos != nil && r.QueuedExternal != nil && r.QueuedOther != nil {
		a.split[0] += *r.QueuedQuota * w
		a.split[1] += *r.QueuedUserQos * w
		a.split[2] += *r.QueuedExternal * w
		a.split[3] += *r.QueuedOther * w
		a.splitN++
	}
	if r.Busy != nil {
		a.busy += *r.Busy * w
		a.busyN++
	}
	a.allocatedHours += r.AllocatedHours
	if r.BusyHours != nil {
		a.busyHours += *r.BusyHours
		a.busyHoursN++
	}
}

// busyKnown reports whether the accumulated busy can be used.
func (a *accumulator) busyKnown() bool {
	return a.busyN > 0 && !busyUnknown(a.allocated, a.covered)
}

func (a *accumulator) point(t time.Time) Point {
	n := a.w
	p := Point{
		Time:      t,
		Quota:     a.quota / n,
		Allocated: a.allocated / n,
		Queued:    a.queued / n,
		Coverage:  ratio(a.covered, a.allocated),
		Occupancy: ratio(a.allocated, a.quota),
	}
	if a.splitN == a.n {
		p.QueuedQuota = float64Ptr(a.split[0] / n)
		p.QueuedUserQos = float64Ptr(a.split[1] / n)
		p.QueuedExternal = float64Ptr(a.split[2] / n)
		p.QueuedOther = float64Ptr(a.split[3] / n)
	}
	if a.busyKnown() {
		p.Busy = float64Ptr(a.busy / n)
		p.Utilization = ratio(a.busy, a.covered)
		p.EffectiveUtilization = ratio(a.busy, a.quota)
	}
	return p
}

// ratio returns num/den, or nil when den is not positive.
func ratio(num, den float64) *float64 {
	if den <= 0 {
		return nil
	}
	return float64Ptr(num / den)
}

// NewPoint returns the point of one record.
func NewPoint(r Record) Point {
	a := &accumulator{}
	a.add(r)
	return a.point(r.Time)
}

// Bucket averages records, one per time, into points step apart from start. A bucket holds the
// records in [its time, its time + step); buckets without records are left out.
func Bucket(records []Record, start time.Time, step time.Duration) []Point {
	if step <= 0 {
		step = time.Nanosecond
	}
	buckets := map[int64]*accumulator{}
	var indexes []int64
	for _, r := range records {
		if r.Time.Before(start) {
			continue
		}
		index := int64(r.Time.Sub(start) / step)
		a, ok := buckets[index]
		if !ok {
			a = &accumulator{}
			buckets[index] = a
			indexes = append(indexes, index)
		}
		a.add(r)
	}
	sort.Slice(indexes, func(i, j int) bool { return indexes[i] < indexes[j] })
	result := make([]Point, 0, len(indexes))
	for _, index := range indexes {
		result = append(result, buckets[index].point(start.Add(time.Duration(index)*step)))
	}
	return result
}

// Summary is the usage of a workspace over a range of records, one per time.
type Summary struct {
	Occupancy            *float64 `json:"occupancy"`
	Utilization          *float64 `json:"utilization"`
	EffectiveUtilization *float64 `json:"effectiveUtilization"`
	AllocatedHours       float64  `json:"allocatedHours"`
	BusyHours            *float64 `json:"busyHours"`
	QueuedAvg            float64  `json:"queuedAvg"`
	QueuedMax            float64  `json:"queuedMax"`
}

// Summarize returns the range aggregates of records. The ratios are those of the range
// averages, so a short spike weighs as long as it lasted.
func Summarize(records []Record) Summary {
	if len(records) == 0 {
		return Summary{}
	}
	a := &accumulator{}
	for _, r := range records {
		a.add(r)
	}
	p := a.point(time.Time{})
	s := Summary{
		Occupancy:            p.Occupancy,
		Utilization:          p.Utilization,
		EffectiveUtilization: p.EffectiveUtilization,
		AllocatedHours:       a.allocatedHours,
		QueuedAvg:            p.Queued,
		QueuedMax:            a.queuedMax,
	}
	if a.busyHoursN > 0 && a.busyKnown() {
		s.BusyHours = float64Ptr(a.busyHours)
	}
	return s
}

// Rollup aggregates the samples of one key in one hour, taken every interval, into its hourly
// row. It returns nil without samples.
func Rollup(hour time.Time, samples []*model.WorkspaceUsageSample, interval time.Duration) *model.WorkspaceUsageHourly {
	if len(samples) == 0 {
		return nil
	}
	a := &accumulator{}
	for _, s := range samples {
		a.add(FromSample(s, interval))
	}
	p := a.point(hour)
	first := samples[0]
	h := &model.WorkspaceUsageHourly{
		Hour:              hour,
		Cluster:           first.Cluster,
		Workspace:         first.Workspace,
		Resource:          first.Resource,
		QuotaAvg:          p.Quota,
		AllocatedAvg:      p.Allocated,
		AllocatedMax:      a.allocatedMax,
		BusyAvg:           p.Busy,
		UtilCoverageAvg:   p.Coverage,
		QueuedAvg:         p.Queued,
		QueuedMax:         a.queuedMax,
		QueuedQuotaMax:    a.splitMax[0],
		QueuedUserQosMax:  a.splitMax[1],
		QueuedExternalMax: a.splitMax[2],
		QueuedOtherMax:    a.splitMax[3],
		AllocatedHours:    a.allocatedHours,
		Samples:           int32(a.n),
		ObservedSeconds:   a.w,
	}
	if p.QueuedQuota != nil {
		h.QueuedQuotaAvg = *p.QueuedQuota
		h.QueuedUserQosAvg = *p.QueuedUserQos
		h.QueuedExternalAvg = *p.QueuedExternal
		h.QueuedOtherAvg = *p.QueuedOther
	}
	if p.Busy != nil {
		h.BusyHours = float64Ptr(a.busyHours)
	}
	return h
}

// RollupHour groups the samples of one hour by key and rolls each group up, ordered by key.
func RollupHour(hour time.Time, samples []*model.WorkspaceUsageSample, interval time.Duration) []*model.WorkspaceUsageHourly {
	groups := map[Key][]*model.WorkspaceUsageSample{}
	var keys []Key
	for _, s := range samples {
		key := Key{Cluster: s.Cluster, Workspace: s.Workspace, Resource: s.Resource}
		if _, ok := groups[key]; !ok {
			keys = append(keys, key)
		}
		groups[key] = append(groups[key], s)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].Cluster != keys[j].Cluster {
			return keys[i].Cluster < keys[j].Cluster
		}
		if keys[i].Workspace != keys[j].Workspace {
			return keys[i].Workspace < keys[j].Workspace
		}
		return keys[i].Resource < keys[j].Resource
	})
	result := make([]*model.WorkspaceUsageHourly, 0, len(keys))
	for _, key := range keys {
		result = append(result, Rollup(hour, groups[key], interval))
	}
	return result
}

// MissingHours returns the whole hours in [from, to) that are not in present, oldest first.
func MissingHours(from, to time.Time, present []time.Time) []time.Time {
	have := make(map[int64]bool, len(present))
	for _, h := range present {
		have[h.Truncate(time.Hour).Unix()] = true
	}
	var result []time.Time
	for h := from.Truncate(time.Hour); h.Before(to); h = h.Add(time.Hour) {
		if !have[h.Unix()] {
			result = append(result, h)
		}
	}
	return result
}
