/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resource

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/robustclient"
)

func TestBuildWorkloadSyncPayload(t *testing.T) {
	wl := &v1.Workload{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "wl1",
			Labels:            map[string]string{v1.ClusterIdLabel: "c1"},
			CreationTimestamp: metav1.NewTime(time.Now()),
			ResourceVersion:   "4711",
		},
		Spec: v1.WorkloadSpec{
			Workspace: "ws1",
			Resources: []v1.WorkloadResource{{GPU: "4", Replica: 2}},
		},
		Status: v1.WorkloadStatus{
			Phase:   v1.WorkloadRunning,
			EndTime: &metav1.Time{Time: time.Now()},
		},
	}
	payload := buildWorkloadSyncPayload(wl)
	assert.Equal(t, "wl1", payload.Name)
	assert.Equal(t, "ws1", payload.Workspace)
	assert.Equal(t, 8, payload.GPURequest)
	assert.Equal(t, "4711", payload.ResourceVersion)
	assert.NotNil(t, payload.CreatedAt)
	assert.NotNil(t, payload.EndAt)
}

func TestBuildHistorySyncPayload(t *testing.T) {
	row := historyRow("h1", "Succeeded")
	row.Workspace = "ws1"
	row.Resources = sql.NullString{String: `[{"gpu":"8","replica":2}]`, Valid: true}
	payload := buildHistorySyncPayload(row, "9")
	assert.Equal(t, "h1", payload.UID)
	assert.Equal(t, "wl-h1", payload.Name)
	assert.Equal(t, "ws1", payload.Workspace)
	assert.Equal(t, "Succeeded", payload.Phase)
	assert.Equal(t, 16, payload.GPURequest)
	assert.Equal(t, "9", payload.ResourceVersion)
	assert.Nil(t, payload.EndAt)
}

func newRobustSyncer(t *testing.T, objs ...client.Object) *WorkloadRobustSyncer {
	t.Helper()
	scheme, _ := genMockScheme()
	cl := ctrlfake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build()
	return &WorkloadRobustSyncer{
		Client:       cl,
		robustClient: robustclient.NewClient(robustclient.ClientConfig{}),
		retention:    7 * 24 * time.Hour,
		now:          time.Now,
	}
}

func TestRobustReconcileNotFound(t *testing.T) {
	r := newRobustSyncer(t)
	res, err := r.Reconcile(context.Background(), ctrlruntime.Request{NamespacedName: types.NamespacedName{Name: "missing"}})
	assert.NoError(t, err)
	assert.Equal(t, ctrlruntime.Result{}, res)
}

func TestRobustSyncToRobustNoCluster(t *testing.T) {
	r := newRobustSyncer(t)
	// Workload with no cluster -> early return, no panic.
	assert.NoError(t, r.syncToRobust(context.Background(), &v1.Workload{}))
}

func TestRobustSyncToRobustClusterNotRegistered(t *testing.T) {
	r := newRobustSyncer(t)
	wl := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "wl1", Labels: map[string]string{v1.ClusterIdLabel: "c1"}}}
	// Cluster not registered in robust client -> ForCluster nil -> early return.
	assert.NoError(t, r.syncToRobust(context.Background(), wl))
}

func TestRobustSyncDeleteToRobust(t *testing.T) {
	r := newRobustSyncer(t)
	wl := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "wl1", Labels: map[string]string{v1.ClusterIdLabel: "c1"}}}
	assert.NoError(t, r.syncDeleteToRobust(context.Background(), wl))
	// No cluster on workload -> also covered.
	assert.NoError(t, r.syncDeleteToRobust(context.Background(), &v1.Workload{}))
}

// Without a Robust client for the cluster the workload is neither failed nor requeued: the
// periodic reconcileAll syncs it once the cluster has a client.
func TestRobustReconcileWithoutClientSkips(t *testing.T) {
	now := metav1.Now()
	wl := &v1.Workload{ObjectMeta: metav1.ObjectMeta{
		Name:              "wl1",
		DeletionTimestamp: &now,
		Finalizers:        []string{"f"},
		Labels:            map[string]string{v1.ClusterIdLabel: "c1"},
	}}
	r := newRobustSyncer(t, wl)
	res, err := r.Reconcile(context.Background(), ctrlruntime.Request{NamespacedName: types.NamespacedName{Name: "wl1"}})
	assert.NoError(t, err)
	assert.Equal(t, ctrlruntime.Result{}, res)
}

func TestRobustReconcileReturnsSyncErrors(t *testing.T) {
	fr := &fakeRobust{failPush: true}
	wl := liveWorkload("u1", "1", v1.WorkloadRunning)
	r := newRobustSyncer(t, wl)
	r.robustClient.RegisterCluster("c1", fr.serve(t).URL)

	_, err := r.Reconcile(context.Background(), ctrlruntime.Request{NamespacedName: types.NamespacedName{Name: wl.Name}})
	assert.Error(t, err)

	fr.mu.Lock()
	fr.failPush = false
	fr.mu.Unlock()
	res, err := r.Reconcile(context.Background(), ctrlruntime.Request{NamespacedName: types.NamespacedName{Name: wl.Name}})
	assert.NoError(t, err)
	assert.Equal(t, ctrlruntime.Result{}, res)
	fr.mu.Lock()
	defer fr.mu.Unlock()
	require.Len(t, fr.single, 1)
	assert.NotEmpty(t, fr.single[0].ResourceVersion)
}

func TestRobustReconcileSyncDisabledForCluster(t *testing.T) {
	fr := &fakeRobust{}
	wl := liveWorkload("u1", "1", v1.WorkloadRunning)
	cluster := &v1.Cluster{ObjectMeta: metav1.ObjectMeta{
		Name:        "c1",
		Annotations: map[string]string{annotationRobustWorkloadSync: "false"},
	}}
	r := newRobustSyncer(t, wl, cluster)
	r.robustClient.RegisterCluster("c1", fr.serve(t).URL)

	res, err := r.Reconcile(context.Background(), ctrlruntime.Request{NamespacedName: types.NamespacedName{Name: wl.Name}})
	assert.NoError(t, err)
	assert.Equal(t, ctrlruntime.Result{}, res)
	r.reconcileAll(context.Background())
	assert.Empty(t, fr.requests())
}

func TestRobustReconcileAllWithoutClients(t *testing.T) {
	wl := &v1.Workload{
		ObjectMeta: metav1.ObjectMeta{Name: "wl1", Labels: map[string]string{v1.ClusterIdLabel: "c1"}},
		Status:     v1.WorkloadStatus{Phase: v1.WorkloadRunning},
	}
	r := newRobustSyncer(t, wl)
	// Lists workloads, no cluster registered -> nothing to do. No panic.
	r.reconcileAll(context.Background())
}

func liveWorkload(uid, version string, phase v1.WorkloadPhase) *v1.Workload {
	return &v1.Workload{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "wl-" + uid,
			UID:             types.UID(uid),
			ResourceVersion: version,
			Labels:          map[string]string{v1.ClusterIdLabel: "c1"},
		},
		Status: v1.WorkloadStatus{Phase: phase},
	}
}

func historyRow(uid, phase string) *dbclient.Workload {
	return &dbclient.Workload{
		WorkloadId:  "wl-" + uid,
		Cluster:     "c1",
		Phase:       sql.NullString{String: phase, Valid: true},
		WorkloadUId: sql.NullString{String: uid, Valid: true},
	}
}

func digestEntry(uid, version, phase string, deleted bool) robustclient.WorkloadSyncDigestEntry {
	return robustclient.WorkloadSyncDigestEntry{UID: uid, SyncVersion: version, Phase: phase, Deleted: deleted}
}

func pushedReasons(plan robustSyncPlan) map[string]string {
	got := map[string]string{}
	for i, p := range plan.push {
		got[p.UID] = plan.reasons[i] + "@" + p.ResourceVersion
	}
	return got
}

func TestPlanRobustSync(t *testing.T) {
	now := metav1.Now()
	terminating := liveWorkload("terminating", "3", v1.WorkloadRunning)
	terminating.DeletionTimestamp = &now
	workloads := []*v1.Workload{
		liveWorkload("missing", "5", v1.WorkloadRunning),
		liveWorkload("stale", "10", v1.WorkloadRunning),
		liveWorkload("ended", "5", v1.WorkloadSucceeded),
		liveWorkload("current", "5", v1.WorkloadRunning),
		liveWorkload("robust-deleted", "5", v1.WorkloadRunning),
		terminating,
	}
	history := []*dbclient.Workload{
		historyRow("missing", "Failed"), // also in etcd: etcd wins
		historyRow("h-missing", "Succeeded"),
		historyRow("h-ended", "Stopped"),
		historyRow("h-current", "Failed"),
		historyRow("h-deleted", "Failed"),
		historyRow("h-ended-unversioned", "Failed"),
		historyRow("h-ended-opaque", "Failed"),
		{WorkloadId: "no-uid"},
	}
	digest := []robustclient.WorkloadSyncDigestEntry{
		digestEntry("stale", "5", "Pending", false),
		digestEntry("ended", "5", "Running", false),
		digestEntry("current", "5", "Running", false),
		digestEntry("robust-deleted", "5", "Running", true),
		digestEntry("terminating", "2", "Running", false),
		digestEntry("h-ended", "7", "Running", false),
		digestEntry("h-current", "7", "Failed", false),
		digestEntry("h-deleted", "7", "Running", true),
		digestEntry("h-ended-unversioned", "", "Running", false),
		digestEntry("h-ended-opaque", "abc", "Running", false),
		digestEntry("orphan", "1", "Running", false),
		digestEntry("orphan-deleted", "1", "Running", true),
	}

	plan := planRobustSync(workloads, history, digest)
	assert.Equal(t, map[string]string{
		"missing":             "missing@5",
		"stale":               "stale@10",
		"ended":               "ended@5",
		"robust-deleted":      "missing@5",
		"h-missing":           "missing@1",
		"h-ended":             "ended@8",
		"h-ended-unversioned": "ended@1",
		"h-ended-opaque":      "ended@1",
	}, pushedReasons(plan))
	assert.Equal(t, []string{"orphan"}, plan.deleteCandidates)
	for _, p := range plan.push {
		assert.NotEmpty(t, p.ResourceVersion, p.UID)
	}
}

func TestIsNewerVersion(t *testing.T) {
	assert.True(t, isNewerVersion("10", "9"))
	assert.False(t, isNewerVersion("9", "10"))
	assert.False(t, isNewerVersion("9", "9"))
	assert.False(t, isNewerVersion("", "9"))
	assert.True(t, isNewerVersion("9", ""))
	assert.True(t, isNewerVersion("abc", "abd"))
	assert.False(t, isNewerVersion("abc", "abc"))
	assert.Equal(t, "8", nextVersion("7"))
	assert.Equal(t, "1", nextVersion("0"))
	assert.Equal(t, "1", nextVersion(""))
	assert.Equal(t, "1", nextVersion("x"))
}

// fakeRobust answers the workload sync routes of the Robust API.
type fakeRobust struct {
	mu       sync.Mutex
	digest   *robustclient.WorkloadSyncDigest // nil: the digest route is missing
	failPush bool
	// batchFailed is the failed count batch pushes answer with.
	batchFailed int
	log         []string
	batches     [][]robustclient.WorkloadSyncPayload
	single      []robustclient.WorkloadSyncPayload
	deleted     []string
}

func (f *fakeRobust) serve(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.log = append(f.log, r.Method+" "+r.URL.Path)
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/workloads/sync/digest":
			if f.digest == nil {
				http.NotFound(w, r)
				return
			}
			_ = json.NewEncoder(w).Encode(f.digest)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/workloads/sync/batch":
			var batch robustclient.WorkloadSyncBatchPayload
			_ = json.NewDecoder(r.Body).Decode(&batch)
			f.batches = append(f.batches, batch.Workloads)
			_ = json.NewEncoder(w).Encode(robustclient.WorkloadSyncBatchResponse{
				Synced: len(batch.Workloads) - f.batchFailed, Failed: f.batchFailed,
			})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/workloads/sync":
			if f.failPush {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			var p robustclient.WorkloadSyncPayload
			_ = json.NewDecoder(r.Body).Decode(&p)
			f.single = append(f.single, p)
			_, _ = w.Write([]byte(`{}`))
		case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/api/v1/workloads/sync/"):
			f.deleted = append(f.deleted, strings.TrimPrefix(r.URL.Path, "/api/v1/workloads/sync/"))
			_, _ = w.Write([]byte(`{}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func (f *fakeRobust) requests() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.log...)
}

func (f *fakeRobust) deletedUIDs() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.deleted...)
}

func (f *fakeRobust) pushedUIDs() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var uids []string
	for _, b := range f.batches {
		for _, p := range b {
			uids = append(uids, p.UID)
		}
	}
	sort.Strings(uids)
	return uids
}

type fakeHistory struct {
	ended    []*dbclient.Workload
	existing map[string]bool
	since    time.Time
}

func (f *fakeHistory) SelectWorkloadsEndedSince(_ context.Context, _ string, since time.Time,
	_, offset int) ([]*dbclient.Workload, error) {
	f.since = since
	if offset > 0 {
		return nil, nil
	}
	return f.ended, nil
}

func (f *fakeHistory) SelectWorkloadsByUIDs(_ context.Context, uids []string) ([]*dbclient.Workload, error) {
	var rows []*dbclient.Workload
	for _, uid := range uids {
		if f.existing[uid] {
			rows = append(rows, historyRow(uid, "Succeeded"))
		}
	}
	return rows, nil
}

func TestReconcileClusterWithDigest(t *testing.T) {
	fr := &fakeRobust{digest: &robustclient.WorkloadSyncDigest{Workloads: []robustclient.WorkloadSyncDigestEntry{
		digestEntry("stale", "1", "Running", false),
		digestEntry("h-ended", "4", "Running", false),
		digestEntry("orphan", "1", "Running", false),
		digestEntry("old-history", "1", "Succeeded", false),
	}}}
	stale := liveWorkload("stale", "2", v1.WorkloadRunning)
	missing := liveWorkload("missing", "1", v1.WorkloadPending)
	r := newRobustSyncer(t, stale, missing)
	fixed := time.Date(2026, 11, 19, 9, 0, 0, 0, time.UTC)
	r.now = func() time.Time { return fixed }
	history := &fakeHistory{
		ended:    []*dbclient.Workload{historyRow("h-ended", "Failed")},
		existing: map[string]bool{"old-history": true},
	}
	r.dbClient = history
	r.robustClient.RegisterCluster("c1", fr.serve(t).URL)

	r.reconcileAll(context.Background())
	assert.Equal(t, []string{"h-ended", "missing", "stale"}, fr.pushedUIDs())
	// Still in the database outside the retention window: kept.
	assert.Equal(t, []string{"orphan"}, fr.deletedUIDs())
	assert.Equal(t, fixed.Add(-7*24*time.Hour), history.since)
}

// Against a Robust without the digest the loop pushes what has not ended, as before.
func TestReconcileClusterDigestNotSupported(t *testing.T) {
	fr := &fakeRobust{}
	r := newRobustSyncer(t,
		liveWorkload("running", "1", v1.WorkloadRunning),
		liveWorkload("done", "1", v1.WorkloadSucceeded))
	r.dbClient = &fakeHistory{ended: []*dbclient.Workload{historyRow("h1", "Failed")}}
	r.robustClient.RegisterCluster("c1", fr.serve(t).URL)

	r.reconcileAll(context.Background())
	assert.Equal(t, []string{"running"}, fr.pushedUIDs())
	assert.Empty(t, fr.deletedUIDs())
}

func TestReconcileClusterDigestError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	r := newRobustSyncer(t, liveWorkload("running", "1", v1.WorkloadRunning))
	r.robustClient.RegisterCluster("c1", srv.URL)
	err := r.reconcileCluster(context.Background(), r.robustClient.ForCluster("c1"), nil)
	require.Error(t, err)
	assert.False(t, robustclient.IsNotSupported(err))
}

// Large pushes are split into batches.
func TestReconcileClusterBatchesPushes(t *testing.T) {
	fr := &fakeRobust{digest: &robustclient.WorkloadSyncDigest{}}
	var workloads []*v1.Workload
	for i := 0; i < robustSyncBatchSize+1; i++ {
		workloads = append(workloads, liveWorkload(string(rune('a'+i%26))+strings.Repeat("x", i/26), "1", v1.WorkloadRunning))
	}
	r := newRobustSyncer(t)
	r.robustClient.RegisterCluster("c1", fr.serve(t).URL)
	require.NoError(t, r.reconcileCluster(context.Background(), r.robustClient.ForCluster("c1"), workloads))
	fr.mu.Lock()
	defer fr.mu.Unlock()
	require.Len(t, fr.batches, 2)
	assert.Len(t, fr.batches[0], robustSyncBatchSize)
	assert.Len(t, fr.batches[1], 1)
}

// A history row Robust does not hold is pushed with a version Robust accepts,
// and items a batch reports as failed are counted.
func TestReconcileClusterPushesMissingHistoryWithVersion(t *testing.T) {
	fr := &fakeRobust{digest: &robustclient.WorkloadSyncDigest{}, batchFailed: 1}
	r := newRobustSyncer(t)
	r.dbClient = &fakeHistory{ended: []*dbclient.Workload{historyRow("h1", "Failed"), historyRow("h2", "Succeeded")}}
	r.robustClient.RegisterCluster("c-batch", fr.serve(t).URL)
	failed := testutil.ToFloat64(robustSyncErrors.WithLabelValues("c-batch", "batch_item"))

	require.NoError(t, r.reconcileCluster(context.Background(), r.robustClient.ForCluster("c-batch"), nil))
	fr.mu.Lock()
	defer fr.mu.Unlock()
	require.Len(t, fr.batches, 1)
	require.Len(t, fr.batches[0], 2)
	for _, p := range fr.batches[0] {
		assert.Equal(t, "1", p.ResourceVersion, p.UID)
	}
	assert.Equal(t, failed+1, testutil.ToFloat64(robustSyncErrors.WithLabelValues("c-batch", "batch_item")))
}
