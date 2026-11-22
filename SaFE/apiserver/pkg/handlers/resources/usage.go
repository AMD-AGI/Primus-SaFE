/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resources

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"k8s.io/klog/v2"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/resources/view"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/robustclient"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/usage"
	commonworkload "github.com/AMD-AIG-AIMA/SAFE/common/pkg/workload"
)

const (
	defaultUsageRange = 24 * time.Hour
	// A longer range is read from the hourly rollup.
	maxUsageSampleRange = 48 * time.Hour
	// The most buckets a series has; a longer range gets a coarser step.
	maxUsagePoints = 1000
	// A sample older than this many intervals is not current.
	usageCurrentIntervals = 3
	defaultUsageLimit     = 10
	maxUsageLimit         = 100
	// How long the workload list waits for Robust before it is returned without utilization.
	usageRobustTimeout = 10 * time.Second
)

var errUsageNoRobustClient = errors.New("no Robust client for the cluster")

// gpuWindowSource returns the GPU utilization windows of workloads in one cluster.
type gpuWindowSource interface {
	GetWorkloadGPUWindow(ctx context.Context, cluster string,
		uids []string, window, step string) ([]robustclient.WorkloadGPUWindow, error)
}

type robustGPUWindowSource struct {
	rc *robustclient.Client
}

func (s robustGPUWindowSource) GetWorkloadGPUWindow(ctx context.Context, cluster string,
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

// ListUsageClusters returns the usage of every cluster over a time range, summed over the
// workspaces the caller may see.
func (h *Handler) ListUsageClusters(c *gin.Context) {
	handle(c, h.listUsageClusters)
}

// ListUsageWorkspaces returns the usage of the workspaces of one cluster over a time range.
func (h *Handler) ListUsageWorkspaces(c *gin.Context) {
	handle(c, h.listUsageWorkspaces)
}

// GetUsageWorkspace returns the usage series of one workspace and its running workloads.
func (h *Handler) GetUsageWorkspace(c *gin.Context) {
	handle(c, h.getUsageWorkspace)
}

// usageRange is a parsed usage query over [start, end).
type usageRange struct {
	resource string
	start    time.Time
	end      time.Time
	step     time.Duration
	// interval is the finest step: the sample interval, or an hour for a long range.
	interval       time.Duration
	sampleInterval time.Duration
	hourly         bool
	// [start, sampleStart) is read from the hourly rollup and [sampleStart, end) from the samples.
	sampleStart time.Time
}

// parseUsageRange reads the resource and time range of a query. now is the default end. The part
// of the range older than the sample retention, a whole hour past now - sampleRetention, is read
// from the hourly rollup; a retention that is not positive keeps the samples forever.
func parseUsageRange(query *view.UsageQuery, now time.Time,
	sampleInterval, sampleRetention time.Duration) (*usageRange, error) {
	r := &usageRange{resource: query.Resource, end: now.UTC(), interval: sampleInterval, sampleInterval: sampleInterval}
	if r.resource == "" {
		r.resource = usage.DefaultResource
	}
	var err error
	if query.End != "" {
		if r.end, err = time.Parse(time.RFC3339, query.End); err != nil {
			return nil, commonerrors.NewBadRequest(fmt.Sprintf("invalid end: %v", err))
		}
		r.end = r.end.UTC()
	}
	r.start = r.end.Add(-defaultUsageRange)
	if query.Start != "" {
		if r.start, err = time.Parse(time.RFC3339, query.Start); err != nil {
			return nil, commonerrors.NewBadRequest(fmt.Sprintf("invalid start: %v", err))
		}
		r.start = r.start.UTC()
	}
	if !r.start.Before(r.end) {
		return nil, commonerrors.NewBadRequest("start must be before end")
	}
	if r.end.Sub(r.start) > maxUsageSampleRange {
		r.hourly = true
		r.interval = time.Hour
	}
	r.step = r.interval
	if query.Step != "" {
		step, err := time.ParseDuration(query.Step)
		if err != nil || step <= 0 {
			return nil, commonerrors.NewBadRequest(fmt.Sprintf("invalid step: %s", query.Step))
		}
		r.step = max(r.step, step)
	}
	r.step = max(r.step, r.end.Sub(r.start)/maxUsagePoints)
	// A bucket holds whole records.
	r.step = (r.step + r.interval - 1) / r.interval * r.interval
	r.sampleStart = r.start
	if sampleRetention > 0 {
		cutoff := now.UTC().Add(-sampleRetention).Add(time.Hour - 1).Truncate(time.Hour)
		r.setSampleStart(cutoff)
	}
	return r, nil
}

// setSampleStart moves the start of the samples later, to t within the range.
func (r *usageRange) setSampleStart(t time.Time) {
	if t.After(r.end) {
		t = r.end
	}
	if t.After(r.sampleStart) {
		r.sampleStart = t
	}
}

// readRolledUpHours makes a long range whose samples are kept forever read the hourly rollup up
// to its first hour that is not rolled up, and the samples from then on, so an hour that never
// got rolled up is still read. With a sample retention the samples are already read from the
// oldest hour they cover, rolled up or not, and an older hour has only the rollup. The rollup
// writes every row of an hour at once, so any row marks the hour rolled up.
func (h *Handler) readRolledUpHours(ctx context.Context, r *usageRange, sampleRetention time.Duration) error {
	if !r.hourly || sampleRetention > 0 {
		return nil
	}
	from := r.start.Truncate(time.Hour)
	hours, err := h.dbClient.ListWorkspaceUsageHours(ctx, from, r.end)
	if err != nil {
		return err
	}
	if missing := usage.MissingHours(from, r.end, hours); len(missing) > 0 {
		r.setSampleStart(missing[0])
	} else {
		r.setSampleStart(r.end)
	}
	return nil
}

// origin is the start of the first series bucket.
func (r *usageRange) origin() time.Time {
	return r.start.Truncate(r.step)
}

func (r *usageRange) filter(cluster string, workspaces []string) dbclient.WorkspaceUsageFilter {
	return dbclient.WorkspaceUsageFilter{
		Resource:   r.resource,
		Cluster:    cluster,
		Workspaces: workspaces,
		Start:      r.start,
		End:        r.end,
	}
}

func (r *usageRange) view() view.UsageRange {
	source := view.UsageSourceMixed
	switch {
	case !r.sampleStart.After(r.start):
		source = view.UsageSourceSample
	case !r.sampleStart.Before(r.end):
		source = view.UsageSourceHourly
	}
	return view.UsageRange{Resource: r.resource, Start: r.start, End: r.end, Step: r.step.String(), Source: source}
}

// usageViewer is the caller of a usage API and what it may see.
type usageViewer struct {
	user  *v1.User
	roles []*v1.Role
	// all is true for a system administrator, who also sees deleted workspaces.
	all bool
}

func (h *Handler) prepareUsageRequest(c *gin.Context) (*usageViewer, *view.UsageQuery, *usageRange, error) {
	if h.dbClient == nil {
		return nil, nil, nil, commonerrors.NewInternalError("database is not enabled")
	}
	user, err := h.getAndSetUsername(c)
	if err != nil {
		return nil, nil, nil, err
	}
	if user == nil {
		return nil, nil, nil, commonerrors.NewUnauthorized("user is not found")
	}
	if user.IsRestricted() {
		return nil, nil, nil, commonerrors.NewForbidden(
			fmt.Sprintf("The user is restricted. type: %d", user.Spec.RestrictedType))
	}
	query := &view.UsageQuery{}
	if err = c.ShouldBindWith(query, binding.Query); err != nil {
		return nil, nil, nil, commonerrors.NewBadRequest(fmt.Sprintf("invalid query: %v", err))
	}
	interval := time.Duration(commonconfig.GetWorkloadUsageSampleIntervalSecond()) * time.Second
	if interval <= 0 {
		interval = 300 * time.Second
	}
	retention := time.Duration(commonconfig.GetWorkloadUsageSampleRetentionDay()) * 24 * time.Hour
	r, err := parseUsageRange(query, time.Now(), interval, retention)
	if err != nil {
		return nil, nil, nil, err
	}
	if err = h.readRolledUpHours(c.Request.Context(), r, retention); err != nil {
		return nil, nil, nil, err
	}
	viewer := &usageViewer{
		user:  user,
		roles: h.accessController.GetRoles(c.Request.Context(), user),
		all:   user.IsSystemAdmin() || user.IsSystemAdminReadonly(),
	}
	return viewer, query, r, nil
}

// workspaceInput is the access check of reading the usage of a workspace, which its members and
// managers may.
func workspaceInput(ctx context.Context, viewer *usageViewer, workspace string) authority.AccessInput {
	return authority.AccessInput{
		Context:      ctx,
		ResourceKind: v1.WorkspaceKind,
		ResourceName: workspace,
		Verb:         v1.GetVerb,
		Workspaces:   []string{workspace},
		User:         viewer.user,
		Roles:        viewer.roles,
	}
}

func (h *Handler) canSeeWorkspaceUsage(ctx context.Context, viewer *usageViewer, workspace string) bool {
	if viewer.all {
		return true
	}
	ok, err := h.accessController.Allowed(workspaceInput(ctx, viewer, workspace))
	return err == nil && ok
}

// canSeeWorkloadUsage reports whether the viewer may see a workload in the lists of a workspace:
// its owner, the managers of its workspace and system administrators may.
func (h *Handler) canSeeWorkloadUsage(ctx context.Context, viewer *usageViewer, w *v1.Workload) bool {
	if viewer.all {
		return true
	}
	ok, err := h.accessController.Allowed(authority.AccessInput{
		Context:      ctx,
		ResourceKind: v1.WorkloadKind,
		Resource:     w,
		Verb:         v1.GetVerb,
		Workspaces:   []string{w.Spec.Workspace},
		User:         viewer.user,
		Roles:        viewer.roles,
	})
	return err == nil && ok
}

// visibleUsageWorkspaces returns nil when the viewer sees every workspace, otherwise the
// workspaces of cluster ("" for any) it may see, which can be empty.
func (h *Handler) visibleUsageWorkspaces(ctx context.Context, viewer *usageViewer, cluster string) ([]string, error) {
	if viewer.all {
		return nil, nil
	}
	list := &v1.WorkspaceList{}
	if err := h.List(ctx, list); err != nil {
		return nil, err
	}
	result := []string{}
	for i := range list.Items {
		ws := &list.Items[i]
		if cluster != "" && ws.Spec.Cluster != cluster {
			continue
		}
		if h.canSeeWorkspaceUsage(ctx, viewer, ws.Name) {
			result = append(result, ws.Name)
		}
	}
	sort.Strings(result)
	return result, nil
}

// usageRecords reads the records the filter selects: the hourly rows before r.sampleStart,
// aggregated by series bucket, and the samples from then on, each clipped to its part. The
// samples of a long range are aggregated by series bucket as well, so a long part of samples
// does not read every sample.
func (h *Handler) usageRecords(ctx context.Context, r *usageRange, filter dbclient.WorkspaceUsageFilter) ([]usage.Record, error) {
	var result []usage.Record
	if r.sampleStart.After(r.start) {
		hourly := filter
		hourly.End = r.sampleStart
		origin, step := r.origin(), r.step
		if step%time.Hour != 0 {
			origin, step = origin.Truncate(time.Hour), time.Hour
		}
		rows, err := h.dbClient.AggregateWorkspaceUsageHourly(ctx, hourly, origin, step)
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			record := usage.FromHourly(row)
			if record.Time.Before(r.origin()) {
				record.Time = r.origin()
			}
			result = append(result, record)
		}
	}
	if r.sampleStart.Before(r.end) && r.hourly {
		samples := filter
		samples.Start = r.sampleStart
		rows, err := h.dbClient.AggregateWorkspaceUsageSamples(ctx, samples, r.origin(), r.step, r.sampleInterval)
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			record := usage.FromHourly(row)
			if record.Time.Before(r.sampleStart) {
				record.Time = r.sampleStart
			}
			result = append(result, record)
		}
	} else if r.sampleStart.Before(r.end) {
		samples := filter
		samples.Start = r.sampleStart
		rows, err := h.dbClient.ListWorkspaceUsageSamples(ctx, samples)
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			record := usage.FromSample(row, r.sampleInterval)
			if clipped, ok := record.Clip(r.sampleInterval, r.sampleStart, r.end); ok {
				result = append(result, clipped)
			}
		}
	}
	return result, nil
}

// currentUsage reads the latest recent sample of each cluster, as records.
func (h *Handler) currentUsage(ctx context.Context, r *usageRange, cluster string, workspaces []string) ([]usage.Record, error) {
	rows, err := h.dbClient.ListLatestWorkspaceUsageSamples(ctx, dbclient.WorkspaceUsageFilter{
		Resource:   r.resource,
		Cluster:    cluster,
		Workspaces: workspaces,
		Start:      time.Now().Add(-usageCurrentIntervals * r.sampleInterval),
	})
	if err != nil {
		return nil, err
	}
	result := make([]usage.Record, 0, len(rows))
	for _, row := range rows {
		result = append(result, usage.FromSample(row, r.sampleInterval))
	}
	return result, nil
}

// currentPoint sums the latest records into one point, nil without records.
func currentPoint(records []usage.Record) *usage.Point {
	sums := usage.SumByTime(records)
	if len(sums) == 0 {
		return nil
	}
	p := usage.NewPoint(sums[len(sums)-1])
	return &p
}

func groupUsageRecords(records []usage.Record, key func(usage.Record) string) map[string][]usage.Record {
	result := map[string][]usage.Record{}
	for _, r := range records {
		result[key(r)] = append(result[key(r)], r)
	}
	return result
}

func byCluster(r usage.Record) string   { return r.Cluster }
func byWorkspace(r usage.Record) string { return r.Workspace }

func sortedKeys(groups ...map[string][]usage.Record) []string {
	seen := map[string]bool{}
	var result []string
	for _, group := range groups {
		for key := range group {
			if !seen[key] {
				seen[key] = true
				result = append(result, key)
			}
		}
	}
	sort.Strings(result)
	return result
}

func (h *Handler) listUsageClusters(c *gin.Context) (interface{}, error) {
	viewer, _, r, err := h.prepareUsageRequest(c)
	if err != nil {
		return nil, err
	}
	ctx := c.Request.Context()
	workspaces, err := h.visibleUsageWorkspaces(ctx, viewer, "")
	if err != nil {
		return nil, err
	}
	records, err := h.usageRecords(ctx, r, r.filter("", workspaces))
	if err != nil {
		return nil, err
	}
	current, err := h.currentUsage(ctx, r, "", workspaces)
	if err != nil {
		return nil, err
	}
	recordsByCluster := groupUsageRecords(records, byCluster)
	currentByCluster := groupUsageRecords(current, byCluster)
	result := &view.ListUsageClustersResponse{UsageRange: r.view(), Items: []view.UsageClusterItem{}}
	for _, cluster := range sortedKeys(recordsByCluster, currentByCluster) {
		sums := usage.SumByTime(recordsByCluster[cluster])
		result.Items = append(result.Items, view.UsageClusterItem{
			Cluster: cluster,
			Workspaces: len(sortedKeys(groupUsageRecords(recordsByCluster[cluster], byWorkspace),
				groupUsageRecords(currentByCluster[cluster], byWorkspace))),
			Current: currentPoint(currentByCluster[cluster]),
			Summary: usage.Summarize(sums),
			Series:  usage.Bucket(sums, r.origin(), r.step),
		})
	}
	return result, nil
}

func (h *Handler) listUsageWorkspaces(c *gin.Context) (interface{}, error) {
	viewer, query, r, err := h.prepareUsageRequest(c)
	if err != nil {
		return nil, err
	}
	if err = validateUsageSort(query); err != nil {
		return nil, err
	}
	cluster := c.GetString(common.Name)
	ctx := c.Request.Context()
	workspaces, err := h.visibleUsageWorkspaces(ctx, viewer, cluster)
	if err != nil {
		return nil, err
	}
	records, err := h.usageRecords(ctx, r, r.filter(cluster, workspaces))
	if err != nil {
		return nil, err
	}
	current, err := h.currentUsage(ctx, r, cluster, workspaces)
	if err != nil {
		return nil, err
	}
	recordsByWorkspace := groupUsageRecords(records, byWorkspace)
	currentByWorkspace := groupUsageRecords(current, byWorkspace)
	result := &view.ListUsageWorkspacesResponse{
		UsageRange: r.view(),
		Cluster:    cluster,
		Total:      usage.Summarize(usage.SumByTime(records)),
		Items:      []view.UsageWorkspaceItem{},
	}
	for _, workspace := range sortedKeys(recordsByWorkspace, currentByWorkspace) {
		result.Items = append(result.Items, view.UsageWorkspaceItem{
			Workspace: workspace,
			Current:   currentPoint(currentByWorkspace[workspace]),
			Summary:   usage.Summarize(recordsByWorkspace[workspace]),
		})
	}
	sortUsageWorkspaces(result.Items, query.SortBy, strings.EqualFold(query.Order, "asc"))
	return result, nil
}

func validateUsageSort(query *view.UsageQuery) error {
	switch query.SortBy {
	case "", view.UsageSortOccupancy, view.UsageSortUtilization, view.UsageSortEffectiveUtilization, view.UsageSortAllocatedHours:
	default:
		return commonerrors.NewBadRequest(fmt.Sprintf("invalid sortBy: %s", query.SortBy))
	}
	if query.Order != "" && !strings.EqualFold(query.Order, "asc") && !strings.EqualFold(query.Order, "desc") {
		return commonerrors.NewBadRequest(fmt.Sprintf("invalid order: %s", query.Order))
	}
	return nil
}

// sortUsageWorkspaces orders the items by sortBy, highest first unless ascending, with the
// items whose value is null last. Without sortBy the items keep their order by name.
func sortUsageWorkspaces(items []view.UsageWorkspaceItem, sortBy string, ascending bool) {
	value := func(item view.UsageWorkspaceItem) *float64 {
		switch sortBy {
		case view.UsageSortOccupancy:
			return item.Summary.Occupancy
		case view.UsageSortUtilization:
			return item.Summary.Utilization
		case view.UsageSortEffectiveUtilization:
			return item.Summary.EffectiveUtilization
		case view.UsageSortAllocatedHours:
			return &item.Summary.AllocatedHours
		}
		return nil
	}
	if sortBy == "" {
		return
	}
	sort.SliceStable(items, func(i, j int) bool {
		a, b := value(items[i]), value(items[j])
		switch {
		case a == nil || b == nil:
			return a != nil && b == nil
		case ascending:
			return *a < *b
		default:
			return *a > *b
		}
	})
}

func (h *Handler) getUsageWorkspace(c *gin.Context) (interface{}, error) {
	viewer, query, r, err := h.prepareUsageRequest(c)
	if err != nil {
		return nil, err
	}
	limit := query.Limit
	if limit == 0 {
		limit = defaultUsageLimit
	}
	if limit < 0 || limit > maxUsageLimit {
		return nil, commonerrors.NewBadRequest(fmt.Sprintf("limit must be between 1 and %d", maxUsageLimit))
	}
	name := c.GetString(common.Name)
	ctx := c.Request.Context()
	if !viewer.all {
		if err = h.accessController.Authorize(workspaceInput(ctx, viewer, name)); err != nil {
			return nil, err
		}
	}
	workspaces := []string{name}
	records, err := h.usageRecords(ctx, r, r.filter("", workspaces))
	if err != nil {
		return nil, err
	}
	current, err := h.currentUsage(ctx, r, "", workspaces)
	if err != nil {
		return nil, err
	}
	result := &view.GetUsageWorkspaceResponse{
		UsageRange:        r.view(),
		Workspace:         name,
		Current:           currentPoint(current),
		Summary:           usage.Summarize(records),
		Series:            usage.Bucket(usage.SumByTime(records), r.origin(), r.step),
		TopAllocated:      []view.UsageWorkloadItem{},
		LowestUtilization: []view.UsageWorkloadItem{},
	}
	if ws, err := h.getAdminWorkspace(ctx, name); err == nil {
		result.Cluster = ws.Spec.Cluster
	} else if len(records) > 0 {
		result.Cluster = records[len(records)-1].Cluster
	}
	workloads, err := h.runningUsageWorkloads(ctx, viewer, result.Cluster, name, r)
	if err != nil {
		return nil, err
	}
	result.TopAllocated, result.LowestUtilization = rankUsageWorkloads(workloads, limit, usage.IsGPUResource(r.resource))
	return result, nil
}

// runningUsageWorkloads returns the workloads of workspace holding the resource of r on nodes that
// the viewer may see, with their GPU utilization when the resource is a GPU.
func (h *Handler) runningUsageWorkloads(ctx context.Context,
	viewer *usageViewer, cluster, workspace string, r *usageRange) ([]view.UsageWorkloadItem, error) {
	resource := r.resource
	workloads, err := commonworkload.GetWorkloadsOfWorkspace(ctx, h.Client, "", []string{workspace}, nil)
	if err != nil {
		return nil, err
	}
	var result []view.UsageWorkloadItem
	var uids []string
	for _, w := range workloads {
		u, err := usage.ComputeWorkload(w)
		if err != nil {
			klog.ErrorS(err, "failed to compute the usage of workload", "workload", w.Name)
			continue
		}
		if u == nil || u.Bound[resource] <= 0 || !h.canSeeWorkloadUsage(ctx, viewer, w) {
			continue
		}
		result = append(result, view.UsageWorkloadItem{
			WorkloadId:  w.Name,
			DisplayName: v1.GetDisplayName(w),
			UserName:    v1.GetUserName(w),
			Phase:       string(w.Status.Phase),
			Allocated:   u.Bound[resource],
		})
		uids = append(uids, string(w.UID))
	}
	if len(result) == 0 || !usage.IsGPUResource(resource) || cluster == "" {
		return result, nil
	}
	utilization := h.gpuUtilization(ctx, cluster, uids, r.sampleInterval)
	for i := range result {
		if value, ok := utilization[uids[i]]; ok {
			result[i].GpuUtilization = &value
		}
	}
	return result, nil
}

// gpuUtilization asks Robust for the average GPU utilization over the last interval of each
// workload uid whose window is covered, by the rule the statistics are sampled with. It returns
// nothing when Robust cannot be asked in time.
func (h *Handler) gpuUtilization(ctx context.Context,
	cluster string, uids []string, interval time.Duration) map[string]float64 {
	result := map[string]float64{}
	if h.gpuWindows == nil {
		return result
	}
	ctx, cancel := context.WithTimeout(ctx, usageRobustTimeout)
	defer cancel()
	windows, err := h.gpuWindows.GetWorkloadGPUWindow(ctx, cluster, uids,
		robustclient.WindowFor(interval), robustclient.GPUWindowStep)
	if err != nil {
		klog.ErrorS(err, "failed to get the GPU utilization of workloads", "cluster", cluster)
		return result
	}
	for i := range windows {
		if avg, ok := windows[i].Average(); ok {
			result[windows[i].UID] = avg
		}
	}
	return result
}

// rankUsageWorkloads returns the limit workloads holding the most of the resource and, for a GPU,
// the limit workloads with the lowest utilization among those that have one.
func rankUsageWorkloads(items []view.UsageWorkloadItem, limit int, gpu bool) ([]view.UsageWorkloadItem, []view.UsageWorkloadItem) {
	top := append([]view.UsageWorkloadItem{}, items...)
	sort.SliceStable(top, func(i, j int) bool {
		if top[i].Allocated != top[j].Allocated {
			return top[i].Allocated > top[j].Allocated
		}
		return top[i].WorkloadId < top[j].WorkloadId
	})
	lowest := []view.UsageWorkloadItem{}
	if gpu {
		for _, item := range items {
			if item.GpuUtilization != nil {
				lowest = append(lowest, item)
			}
		}
		sort.SliceStable(lowest, func(i, j int) bool {
			if *lowest[i].GpuUtilization != *lowest[j].GpuUtilization {
				return *lowest[i].GpuUtilization < *lowest[j].GpuUtilization
			}
			return lowest[i].WorkloadId < lowest[j].WorkloadId
		})
	}
	return top[:min(limit, len(top))], lowest[:min(limit, len(lowest))]
}
