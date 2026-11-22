/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resources

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority/authtest"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/resources/view"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	mock_client "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/mock"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/model"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/robustclient"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/usage"
	commonuser "github.com/AMD-AIG-AIMA/SAFE/common/pkg/user"
)

func TestParseUsageRange(t *testing.T) {
	now := time.Date(2026, 11, 20, 10, 7, 0, 0, time.UTC)
	interval := 5 * time.Minute

	r, err := parseUsageRange(&view.UsageQuery{}, now, interval)
	require.NoError(t, err)
	assert.Equal(t, common.AmdGpu, r.resource)
	assert.Equal(t, now, r.end)
	assert.Equal(t, now.Add(-24*time.Hour), r.start)
	assert.False(t, r.hourly)
	assert.Equal(t, interval, r.step)
	assert.Equal(t, view.UsageSourceSample, r.view().Source)

	r, err = parseUsageRange(&view.UsageQuery{Start: "2026-11-18T10:07:00Z", End: "2026-11-20T10:07:00Z", Step: "1m"}, now, interval)
	require.NoError(t, err)
	assert.False(t, r.hourly, "two days are read from the samples")
	assert.Equal(t, interval, r.step, "a step finer than the samples is raised")

	r, err = parseUsageRange(&view.UsageQuery{Start: "2026-11-10T10:07:00Z", Resource: "cpu", Step: "24h"}, now, interval)
	require.NoError(t, err)
	assert.True(t, r.hourly)
	assert.Equal(t, "cpu", r.resource)
	assert.Equal(t, time.Date(2026, 11, 10, 10, 7, 0, 0, time.UTC), r.start, "the start is kept; the hour it is in is clipped")
	assert.Equal(t, time.Hour, r.interval)
	assert.Equal(t, 5*time.Minute, r.sampleInterval)
	assert.Equal(t, 24*time.Hour, r.step)
	assert.Equal(t, time.Date(2026, 11, 10, 0, 0, 0, 0, time.UTC), r.origin())
	assert.Equal(t, view.UsageSourceHourly, r.view().Source)

	r, err = parseUsageRange(&view.UsageQuery{Start: "2026-08-22T10:00:00Z", Step: "1h"}, now, interval)
	require.NoError(t, err)
	assert.Equal(t, 3*time.Hour, r.step, "90 days of hours are too many points")
	r, err = parseUsageRange(&view.UsageQuery{Step: "7m"}, now, interval)
	require.NoError(t, err)
	assert.Equal(t, 10*time.Minute, r.step, "a step is whole samples")

	for _, query := range []view.UsageQuery{
		{Start: "yesterday"},
		{End: "2026-11-20"},
		{Start: "2026-11-20T10:07:00Z", End: "2026-11-20T10:07:00Z"},
		{Step: "0s"},
		{Step: "often"},
	} {
		_, err = parseUsageRange(&query, now, interval)
		assert.Error(t, err, query)
	}
}

func TestSortUsageWorkspaces(t *testing.T) {
	f := func(v float64) *float64 { return &v }
	items := func() []view.UsageWorkspaceItem {
		return []view.UsageWorkspaceItem{
			{Workspace: "a", Summary: usageSummary(f(0.2), nil, 30)},
			{Workspace: "b", Summary: usageSummary(nil, f(0.9), 10)},
			{Workspace: "c", Summary: usageSummary(f(0.8), f(0.1), 20)},
		}
	}
	names := func(items []view.UsageWorkspaceItem) []string {
		var result []string
		for _, item := range items {
			result = append(result, item.Workspace)
		}
		return result
	}

	got := items()
	sortUsageWorkspaces(got, view.UsageSortOccupancy, false)
	assert.Equal(t, []string{"c", "a", "b"}, names(got), "null last")
	got = items()
	sortUsageWorkspaces(got, view.UsageSortOccupancy, true)
	assert.Equal(t, []string{"a", "c", "b"}, names(got), "null last when ascending too")
	got = items()
	sortUsageWorkspaces(got, view.UsageSortUtilization, false)
	assert.Equal(t, []string{"b", "c", "a"}, names(got))
	got = items()
	sortUsageWorkspaces(got, view.UsageSortAllocatedHours, false)
	assert.Equal(t, []string{"a", "c", "b"}, names(got))
	got = items()
	sortUsageWorkspaces(got, "", false)
	assert.Equal(t, []string{"a", "b", "c"}, names(got))

	assert.Error(t, validateUsageSort(&view.UsageQuery{SortBy: "name"}))
	assert.Error(t, validateUsageSort(&view.UsageQuery{Order: "up"}))
	assert.NoError(t, validateUsageSort(&view.UsageQuery{SortBy: view.UsageSortEffectiveUtilization, Order: "ASC"}))
}

func usageSummary(occupancy, utilization *float64, allocatedHours float64) usage.Summary {
	return usage.Summary{Occupancy: occupancy, Utilization: utilization, AllocatedHours: allocatedHours}
}

func TestRankUsageWorkloads(t *testing.T) {
	f := func(v float64) *float64 { return &v }
	items := []view.UsageWorkloadItem{
		{WorkloadId: "a", Allocated: 8, GpuUtilization: f(50)},
		{WorkloadId: "b", Allocated: 32},
		{WorkloadId: "c", Allocated: 16, GpuUtilization: f(5)},
		{WorkloadId: "d", Allocated: 16, GpuUtilization: f(0)},
	}
	top, lowest := rankUsageWorkloads(items, 3, true)
	assert.Equal(t, []string{"b", "c", "d"}, workloadIds(top))
	assert.Equal(t, []string{"d", "c", "a"}, workloadIds(lowest), "a workload without data is not the least utilized")

	top, lowest = rankUsageWorkloads(items, 10, false)
	assert.Len(t, top, 4)
	assert.Empty(t, lowest)
	assert.Equal(t, "a", items[0].WorkloadId, "the input is not reordered")
}

func workloadIds(items []view.UsageWorkloadItem) []string {
	var result []string
	for _, item := range items {
		result = append(result, item.WorkloadId)
	}
	return result
}

// usageAccessFixture has a system administrator, alice who is a member of ws-a, bob who manages
// ws-a and ws-b, and dave who is a member of ws-c only. ws-a has a workload of alice and one of
// carol.
func usageAccessFixture(t *testing.T) (*Handler, *mock_client.MockInterface) {
	t.Helper()
	admin := usageUser("admin", v1.SystemAdminRole)
	alice := usageUser("alice", v1.DefaultRole)
	commonuser.AssignWorkspace(alice, "ws-a")
	bob := usageUser("bob", v1.DefaultRole)
	commonuser.AssignManagedWorkspace(bob, "ws-a", "ws-b")
	dave := usageUser("dave", v1.DefaultRole)
	commonuser.AssignWorkspace(dave, "ws-c")

	objs := append(authtest.ChartRoles(t), admin, alice, bob, dave,
		usageWorkspace("ws-a", "c1"), usageWorkspace("ws-b", "c1"), usageWorkspace("ws-c", "c2"),
		usageGPUWorkload("w-alice", "ws-a", "alice", 1), usageGPUWorkload("w-carol", "ws-a", "carol", 2))
	scheme := runtime.NewScheme()
	require.NoError(t, v1.AddToScheme(scheme))
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build()
	mockDB := mock_client.NewMockInterface(gomock.NewController(t))
	return &Handler{
		Client:           fakeClient,
		dbClient:         mockDB,
		accessController: &authority.AccessController{Client: fakeClient},
		gpuWindows:       &fakeGPUWindows{},
	}, mockDB
}

type fakeGPUWindows struct {
	windows []robustclient.WorkloadGPUWindow
	err     error
	calls   []gpuWindowCall
}

type gpuWindowCall struct {
	cluster, window, step string
	uids                  []string
	hasDeadline           bool
}

func (f *fakeGPUWindows) GetWorkloadGPUWindow(ctx context.Context, cluster string,
	uids []string, window, step string) ([]robustclient.WorkloadGPUWindow, error) {
	_, hasDeadline := ctx.Deadline()
	f.calls = append(f.calls, gpuWindowCall{cluster: cluster, window: window, step: step, uids: uids, hasDeadline: hasDeadline})
	return f.windows, f.err
}

func usageUser(name string, role v1.UserRole) *v1.User {
	return &v1.User{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Labels:      map[string]string{v1.UserIdLabel: name},
			Annotations: map[string]string{v1.UserNameAnnotation: name},
		},
		Spec: v1.UserSpec{Type: v1.DefaultUserType, Roles: []v1.UserRole{role}},
	}
}

func usageWorkspace(name, cluster string) *v1.Workspace {
	return &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: name}, Spec: v1.WorkspaceSpec{Cluster: cluster}}
}

// usageGPUWorkload is a running workload of owner with pods of 8 GPUs, all on nodes.
func usageGPUWorkload(name, workspace, owner string, pods int) *v1.Workload {
	w := &v1.Workload{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			UID:  types.UID(name + "-uid"),
			Labels: map[string]string{
				v1.WorkspaceIdLabel: workspace,
				v1.UserIdLabel:      owner,
			},
		},
		Spec: v1.WorkloadSpec{
			Workspace: workspace,
			Resources: []v1.WorkloadResource{{Replica: pods, CPU: "8", Memory: "32Gi", GPU: "8", GPUName: common.AmdGpu}},
		},
		Status: v1.WorkloadStatus{Phase: v1.WorkloadRunning},
	}
	for i := 0; i < pods; i++ {
		w.Status.NodeUsage = append(w.Status.NodeUsage, v1.NodePodUsage{
			Node: name + "-node-" + string(rune('a'+i)), Active: map[string]int{"0": 1}, Running: map[string]int{"0": 1},
		})
	}
	return w
}

func usageRequest(user, name, query string) (*gin.Context, *httptest.ResponseRecorder) {
	rsp := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rsp)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/usage?"+query, nil)
	c.Set(common.UserId, user)
	if name != "" {
		c.Set(common.Name, name)
	}
	return c, rsp
}

func TestVisibleUsageWorkspaces(t *testing.T) {
	h, _ := usageAccessFixture(t)
	ctx := context.Background()
	for _, tc := range []struct {
		user    string
		cluster string
		want    []string
	}{
		{"admin", "", nil},
		{"alice", "", []string{"ws-a"}},
		{"bob", "", []string{"ws-a", "ws-b"}},
		{"bob", "c2", []string{}},
		{"dave", "", []string{"ws-c"}},
		{"dave", "c1", []string{}},
	} {
		user, err := h.accessController.GetRequestUser(ctx, tc.user)
		require.NoError(t, err)
		viewer := &usageViewer{user: user, roles: h.accessController.GetRoles(ctx, user),
			all: user.IsSystemAdmin() || user.IsSystemAdminReadonly()}
		got, err := h.visibleUsageWorkspaces(ctx, viewer, tc.cluster)
		require.NoError(t, err)
		assert.Equal(t, tc.want, got, "%s in %q", tc.user, tc.cluster)
	}
}

func TestListUsageClustersSumsVisibleWorkspaces(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h, mockDB := usageAccessFixture(t)
	at := time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC)
	busy := 4.0
	coverage := 1.0
	var filters []dbclient.WorkspaceUsageFilter
	mockDB.EXPECT().ListWorkspaceUsageSamples(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, filter dbclient.WorkspaceUsageFilter) ([]*model.WorkspaceUsageSample, error) {
			filters = append(filters, filter)
			return []*model.WorkspaceUsageSample{
				{SampleTime: at, Cluster: "c1", Workspace: "ws-a", Resource: common.AmdGpu, Quota: 16, Allocated: 8,
					Busy: &busy, UtilCoverage: &coverage},
				{SampleTime: at, Cluster: "c1", Workspace: "ws-b", Resource: common.AmdGpu, Quota: 16, Allocated: 0,
					Busy: new(float64)},
			}, nil
		})
	mockDB.EXPECT().ListLatestWorkspaceUsageSamples(gomock.Any(), gomock.Any()).Return(nil, nil)

	c, rsp := usageRequest("bob", "", "start=2026-11-20T09:00:00Z&end=2026-11-20T11:00:00Z")
	h.ListUsageClusters(c)
	require.Equal(t, http.StatusOK, rsp.Code, rsp.Body.String())
	require.Len(t, filters, 1)
	assert.Equal(t, []string{"ws-a", "ws-b"}, filters[0].Workspaces)
	assert.Equal(t, common.AmdGpu, filters[0].Resource)

	var result view.ListUsageClustersResponse
	require.NoError(t, json.Unmarshal(rsp.Body.Bytes(), &result))
	assert.Equal(t, view.UsageSourceSample, result.Source)
	require.Len(t, result.Items, 1)
	item := result.Items[0]
	assert.Equal(t, "c1", item.Cluster)
	assert.Equal(t, 2, item.Workspaces)
	assert.Nil(t, item.Current)
	require.Len(t, item.Series, 1)
	assert.Equal(t, 32.0, item.Series[0].Quota)
	assert.InDelta(t, 0.25, *item.Series[0].Occupancy, 1e-9)
	assert.InDelta(t, 0.5, *item.Series[0].Utilization, 1e-9)
	assert.InDelta(t, 0.125, *item.Series[0].EffectiveUtilization, 1e-9)
}

func TestListUsageClustersWithoutVisibleWorkspaces(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h, mockDB := usageAccessFixture(t)
	mockDB.EXPECT().ListWorkspaceUsageSamples(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, filter dbclient.WorkspaceUsageFilter) ([]*model.WorkspaceUsageSample, error) {
			assert.Equal(t, []string{}, filter.Workspaces, "an empty filter, not every workspace")
			return nil, nil
		})
	mockDB.EXPECT().ListLatestWorkspaceUsageSamples(gomock.Any(), gomock.Any()).Return(nil, nil)
	c, rsp := usageRequest("dave", "c1", "")
	h.ListUsageWorkspaces(c)
	require.Equal(t, http.StatusOK, rsp.Code, rsp.Body.String())
	var result view.ListUsageWorkspacesResponse
	require.NoError(t, json.Unmarshal(rsp.Body.Bytes(), &result))
	assert.Empty(t, result.Items)
}

func TestGetUsageWorkspaceAccess(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		user       string
		wantCode   int
		wantListed []string
	}{
		{"admin", http.StatusOK, []string{"w-carol", "w-alice"}},
		{"bob", http.StatusOK, []string{"w-carol", "w-alice"}},
		{"alice", http.StatusOK, []string{"w-alice"}},
		{"dave", http.StatusForbidden, nil},
	} {
		t.Run(tc.user, func(t *testing.T) {
			h, mockDB := usageAccessFixture(t)
			h.gpuWindows = &fakeGPUWindows{windows: []robustclient.WorkloadGPUWindow{
				{UID: "w-alice-uid", Overall: robustclient.GPUWindowOverall{Max: 30, Avg: 12, Samples: 5, ExpectedSamples: 5}},
				{UID: "w-carol-uid", Overall: robustclient.GPUWindowOverall{Max: 30, Avg: 1, Samples: 4, ExpectedSamples: 5}},
			}}
			if tc.wantCode == http.StatusOK {
				mockDB.EXPECT().ListWorkspaceUsageSamples(gomock.Any(), gomock.Any()).Return(nil, nil)
				mockDB.EXPECT().ListLatestWorkspaceUsageSamples(gomock.Any(), gomock.Any()).Return(nil, nil)
			}
			c, rsp := usageRequest(tc.user, "ws-a", "")
			h.GetUsageWorkspace(c)
			require.Equal(t, tc.wantCode, rsp.Code, rsp.Body.String())
			if tc.wantCode != http.StatusOK {
				return
			}
			var result view.GetUsageWorkspaceResponse
			require.NoError(t, json.Unmarshal(rsp.Body.Bytes(), &result))
			assert.Equal(t, "c1", result.Cluster)
			assert.Equal(t, tc.wantListed, workloadIds(result.TopAllocated))
			require.Len(t, result.LowestUtilization, 1, "the uncovered window of w-carol is not ranked")
			assert.Equal(t, "w-alice", result.LowestUtilization[0].WorkloadId)
			require.NotNil(t, result.LowestUtilization[0].GpuUtilization)
			assert.Equal(t, 12.0, *result.LowestUtilization[0].GpuUtilization)
			for _, item := range result.TopAllocated {
				if item.WorkloadId == "w-carol" {
					assert.Nil(t, item.GpuUtilization)
				}
			}
		})
	}
}

func TestGetUsageWorkspaceAsksRobustLive(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h, mockDB := usageAccessFixture(t)
	windows := &fakeGPUWindows{}
	h.gpuWindows = windows
	mockDB.EXPECT().ListWorkspaceUsageSamples(gomock.Any(), gomock.Any()).Return(nil, nil)
	mockDB.EXPECT().ListLatestWorkspaceUsageSamples(gomock.Any(), gomock.Any()).Return(nil, nil)

	c, rsp := usageRequest("admin", "ws-a", "")
	h.GetUsageWorkspace(c)
	require.Equal(t, http.StatusOK, rsp.Code, rsp.Body.String())
	require.Len(t, windows.calls, 1, "one batched request for the listed workloads")
	call := windows.calls[0]
	assert.Equal(t, "c1", call.cluster)
	assert.Equal(t, robustclient.GPUWindowStep, call.step)
	assert.Equal(t, "5m", call.window, "the window is the sample interval")
	assert.ElementsMatch(t, []string{"w-alice-uid", "w-carol-uid"}, call.uids)
	assert.True(t, call.hasDeadline)
}

func TestGetUsageWorkspaceWithoutRobust(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h, mockDB := usageAccessFixture(t)
	h.gpuWindows = &fakeGPUWindows{err: errors.New("robust down")}
	mockDB.EXPECT().ListWorkspaceUsageSamples(gomock.Any(), gomock.Any()).Return(nil, nil)
	mockDB.EXPECT().ListLatestWorkspaceUsageSamples(gomock.Any(), gomock.Any()).Return(nil, nil)

	c, rsp := usageRequest("admin", "ws-a", "")
	h.GetUsageWorkspace(c)
	require.Equal(t, http.StatusOK, rsp.Code, rsp.Body.String())
	var result view.GetUsageWorkspaceResponse
	require.NoError(t, json.Unmarshal(rsp.Body.Bytes(), &result))
	require.Len(t, result.TopAllocated, 2)
	for _, item := range result.TopAllocated {
		assert.Nil(t, item.GpuUtilization, item.WorkloadId)
	}
	assert.Empty(t, result.LowestUtilization)
	assert.Contains(t, rsp.Body.String(), `"gpuUtilization":null`)
}

func TestGetUsageWorkspaceLongRangeKeepsQueuedSplit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h, mockDB := usageAccessFixture(t)
	hour := time.Date(2026, 11, 10, 10, 0, 0, 0, time.UTC)
	mockDB.EXPECT().AggregateWorkspaceUsageHourly(gomock.Any(), gomock.Any(), hour, 2*time.Hour).Return([]*model.WorkspaceUsageHourly{
		{Hour: hour, Cluster: "c1", Workspace: "ws-a", Resource: common.AmdGpu, QuotaAvg: 16, AllocatedAvg: 8,
			QueuedAvg: 6, QueuedMax: 10, QueuedQuotaAvg: 3, QueuedUserQosAvg: 2, QueuedExternalAvg: 1,
			QueuedQuotaMax: 6, QueuedUserQosMax: 4, QueuedExternalMax: 2, AllocatedHours: 8, Samples: 12},
		{Hour: hour.Add(time.Hour), Cluster: "c1", Workspace: "ws-a", Resource: common.AmdGpu, QuotaAvg: 16,
			AllocatedAvg: 8, QueuedAvg: 2, QueuedMax: 2, QueuedOtherAvg: 2, QueuedOtherMax: 2, AllocatedHours: 8, Samples: 12},
	}, nil)
	mockDB.EXPECT().ListLatestWorkspaceUsageSamples(gomock.Any(), gomock.Any()).Return(nil, nil)

	c, rsp := usageRequest("admin", "ws-a", "start=2026-11-10T10:00:00Z&end=2026-11-13T10:00:00Z&step=2h")
	h.GetUsageWorkspace(c)
	require.Equal(t, http.StatusOK, rsp.Code, rsp.Body.String())
	var result view.GetUsageWorkspaceResponse
	require.NoError(t, json.Unmarshal(rsp.Body.Bytes(), &result))
	assert.Equal(t, view.UsageSourceHourly, result.Source)
	require.Len(t, result.Series, 1)
	point := result.Series[0]
	assert.Equal(t, 4.0, point.Queued)
	for name, pair := range map[string]struct {
		got  *float64
		want float64
	}{
		"quota":    {point.QueuedQuota, 1.5},
		"user qos": {point.QueuedUserQos, 1},
		"external": {point.QueuedExternal, 0.5},
		"other":    {point.QueuedOther, 1},
	} {
		require.NotNil(t, pair.got, name)
		assert.InDelta(t, pair.want, *pair.got, 1e-9, name)
	}
}

func TestGetUsageWorkspaceAggregatesHourlyRowsInTheDatabase(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h, mockDB := usageAccessFixture(t)
	hour := time.Date(2026, 11, 10, 10, 0, 0, 0, time.UTC)
	end := time.Date(2026, 11, 13, 10, 0, 0, 0, time.UTC)
	mockDB.EXPECT().AggregateWorkspaceUsageHourly(gomock.Any(), gomock.Any(), hour, time.Hour).DoAndReturn(
		func(_ context.Context, filter dbclient.WorkspaceUsageFilter, _ time.Time, _ time.Duration) ([]*model.WorkspaceUsageHourly, error) {
			assert.Equal(t, hour.Add(30*time.Minute), filter.Start, "the database clips the first hour")
			assert.Equal(t, end, filter.End)
			return []*model.WorkspaceUsageHourly{
				{Hour: hour, Cluster: "c1", Workspace: "ws-a", Resource: common.AmdGpu, QuotaAvg: 16, AllocatedAvg: 8,
					AllocatedHours: 4, Samples: 12, ObservedSeconds: 1800},
				{Hour: hour.Add(time.Hour), Cluster: "c1", Workspace: "ws-a", Resource: common.AmdGpu, QuotaAvg: 16,
					AllocatedAvg: 8, AllocatedHours: 8, Samples: 12, ObservedSeconds: 3600},
			}, nil
		})
	mockDB.EXPECT().ListLatestWorkspaceUsageSamples(gomock.Any(), gomock.Any()).Return(nil, nil)

	c, rsp := usageRequest("admin", "ws-a", "start=2026-11-10T10:30:00Z&end=2026-11-13T10:00:00Z")
	h.GetUsageWorkspace(c)
	require.Equal(t, http.StatusOK, rsp.Code, rsp.Body.String())
	var result view.GetUsageWorkspaceResponse
	require.NoError(t, json.Unmarshal(rsp.Body.Bytes(), &result))
	assert.InDelta(t, 4+8, result.Summary.AllocatedHours, 1e-9, "only the half of the first hour in the range counts")
	assert.InDelta(t, 0.5, *result.Summary.Occupancy, 1e-9)
	require.Len(t, result.Series, 2)
	assert.Equal(t, hour, result.Series[0].Time)
}

func TestUsageRequiresDatabase(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h, _ := usageAccessFixture(t)
	h.dbClient = nil
	c, rsp := usageRequest("admin", "", "")
	h.ListUsageClusters(c)
	assert.Equal(t, http.StatusInternalServerError, rsp.Code)
}
