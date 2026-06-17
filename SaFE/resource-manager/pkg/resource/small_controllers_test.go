/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resource

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/robustclient"
	githubpkg "github.com/AMD-AIG-AIMA/SAFE/resource-manager/pkg/github"
)

// ---- github_workflow_controller ----

func TestIsEphemeralRunnerWorkload(t *testing.T) {
	wl := &v1.Workload{}
	wl.Spec.GroupVersionKind.Kind = common.CICDEphemeralRunnerKind
	assert.True(t, isEphemeralRunnerWorkload(wl))
	assert.False(t, isEphemeralRunnerWorkload(&v1.Workload{}))
	assert.False(t, isEphemeralRunnerWorkload(&corev1.Pod{}))
}

func TestSetupGitHubWorkflowControllerDBDisabled(t *testing.T) {
	// DB disabled by default -> returns nil without touching manager.
	assert.NoError(t, SetupGitHubWorkflowController(nil))
}

func TestGitHubWorkflowReconcileNotFound(t *testing.T) {
	scheme, _ := genMockScheme()
	cl := ctrlfake.NewClientBuilder().WithScheme(scheme).Build()
	r := &GitHubWorkflowReconciler{Client: cl}
	res, err := r.Reconcile(context.Background(), ctrlruntime.Request{NamespacedName: types.NamespacedName{Name: "missing"}})
	assert.NoError(t, err)
	assert.Equal(t, ctrlruntime.Result{}, res)
}

func TestGitHubWorkflowReconcileNonEphemeral(t *testing.T) {
	scheme, _ := genMockScheme()
	wl := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "wl1"}}
	cl := ctrlfake.NewClientBuilder().WithScheme(scheme).WithObjects(wl).Build()
	r := &GitHubWorkflowReconciler{Client: cl}
	_, err := r.Reconcile(context.Background(), ctrlruntime.Request{NamespacedName: types.NamespacedName{Name: "wl1"}})
	assert.NoError(t, err)
}

func TestGitHubWorkflowReconcileNoRunID(t *testing.T) {
	scheme, _ := genMockScheme()
	wl := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "wl1", Annotations: map[string]string{"x": "y"}}}
	wl.Spec.GroupVersionKind.Kind = common.CICDEphemeralRunnerKind
	cl := ctrlfake.NewClientBuilder().WithScheme(scheme).WithObjects(wl).Build()
	r := &GitHubWorkflowReconciler{Client: cl}
	_, err := r.Reconcile(context.Background(), ctrlruntime.Request{NamespacedName: types.NamespacedName{Name: "wl1"}})
	assert.NoError(t, err)
}

func TestGitHubWorkflowReconcileWithRunID(t *testing.T) {
	scheme, _ := genMockScheme()
	wl := &v1.Workload{ObjectMeta: metav1.ObjectMeta{
		Name: "wl1",
		Annotations: map[string]string{
			"actions.github.com/run-id":     "100",
			"actions.github.com/repository": "owner/repo",
		},
	}}
	wl.Spec.GroupVersionKind.Kind = common.CICDEphemeralRunnerKind
	cl := ctrlfake.NewClientBuilder().WithScheme(scheme).WithObjects(wl).Build()

	db, mock, _ := sqlmock.New()
	defer db.Close()
	mock.ExpectExec("INSERT INTO github_workflow_runs").WillReturnResult(sqlmock.NewResult(1, 1))

	r := &GitHubWorkflowReconciler{Client: cl, tracker: githubpkg.NewWorkflowTracker(githubpkg.NewStore(db))}
	_, err := r.Reconcile(context.Background(), ctrlruntime.Request{NamespacedName: types.NamespacedName{Name: "wl1"}})
	assert.NoError(t, err)
}

// ---- workload_robust_syncer ----

func TestBuildWorkloadSyncPayload(t *testing.T) {
	wl := &v1.Workload{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "wl1",
			Labels:            map[string]string{v1.ClusterIdLabel: "c1"},
			CreationTimestamp: metav1.NewTime(time.Now()),
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
	assert.NotNil(t, payload.CreatedAt)
	assert.NotNil(t, payload.EndAt)
}

// ---- image_import_job ----

func TestFilterImageImportJob(t *testing.T) {
	withLabel := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"image-import": "x"}}}
	assert.True(t, filterImageImportJob(withLabel))
	assert.False(t, filterImageImportJob(&corev1.Pod{}))
}

// ---- grafana_datasource_syncer ----

func TestIsNoGrafanaCRD(t *testing.T) {
	assert.False(t, isNoGrafanaCRD(nil))
	assert.True(t, isNoGrafanaCRD(errors.New("no matches for kind GrafanaDatasource")))
	assert.True(t, isNoGrafanaCRD(errors.New("the server could not find the requested resource")))
	assert.False(t, isNoGrafanaCRD(errors.New("some other error")))
}

func TestBuildDatasource(t *testing.T) {
	s := &GrafanaDatasourceSyncer{namespace: "monitoring"}
	ds := s.buildDatasource("c1-prometheus", "prometheus", "http://x", "c1", "c1", nil)
	assert.Equal(t, "GrafanaDatasource", ds.GetKind())
	assert.Equal(t, "c1-prometheus", ds.GetName())
	assert.Equal(t, "monitoring", ds.GetNamespace())
}

// ---- workspace_controller_helper service accounts ----

func TestCreateAndDeleteServiceAccount(t *testing.T) {
	cs := k8sfake.NewSimpleClientset()
	ws := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}}
	assert.NoError(t, createServiceAccount(context.Background(), ws, cs, "sa1"))
	// Created.
	_, err := cs.CoreV1().ServiceAccounts("ws1").Get(context.Background(), "sa1", metav1.GetOptions{})
	assert.NoError(t, err)
	// Idempotent create.
	assert.NoError(t, createServiceAccount(context.Background(), ws, cs, "sa1"))
	// Delete.
	assert.NoError(t, deleteServiceAccount(context.Background(), ws, cs, "sa1"))
	// Delete again -> not found ignored.
	assert.NoError(t, deleteServiceAccount(context.Background(), ws, cs, "sa1"))
}

func TestCreateAndDeleteMonarchRoleBinding(t *testing.T) {
	cs := k8sfake.NewSimpleClientset()
	ws := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}}
	assert.NoError(t, createMonarchRoleBinding(context.Background(), ws, cs, "role1"))
	_, err := cs.RbacV1().RoleBindings("ws1").Get(context.Background(), "role1", metav1.GetOptions{})
	assert.NoError(t, err)
	// Idempotent.
	assert.NoError(t, createMonarchRoleBinding(context.Background(), ws, cs, "role1"))
	// Delete.
	assert.NoError(t, deleteRoleBinding(context.Background(), ws, cs, "role1"))
	assert.NoError(t, deleteRoleBinding(context.Background(), ws, cs, "role1"))
}

func TestCICDAndMonarchServiceAccountGated(t *testing.T) {
	cs := k8sfake.NewSimpleClientset()
	ws := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}}
	// CI/CD and Monarch are disabled by default -> these return nil without touching cluster.
	assert.NoError(t, createCICDServiceAccount(context.Background(), ws, cs))
	assert.NoError(t, deleteCICDServiceAccount(context.Background(), ws, cs))
	assert.NoError(t, createMonarchServiceAccount(context.Background(), ws, cs))
	assert.NoError(t, deleteMonarchServiceAccount(context.Background(), ws, cs))
}

var _ = client.IgnoreNotFound

// ---- workload_robust_syncer ----

func newRobustSyncer(t *testing.T, objs ...client.Object) *WorkloadRobustSyncer {
	t.Helper()
	scheme, _ := genMockScheme()
	cl := ctrlfake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build()
	return &WorkloadRobustSyncer{Client: cl, robustClient: robustclient.NewClient(robustclient.ClientConfig{})}
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
	r.syncToRobust(context.Background(), &v1.Workload{})
}

func TestRobustSyncToRobustClusterNotRegistered(t *testing.T) {
	r := newRobustSyncer(t)
	wl := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "wl1", Labels: map[string]string{v1.ClusterIdLabel: "c1"}}}
	// Cluster not registered in robust client -> ForCluster nil -> early return.
	r.syncToRobust(context.Background(), wl)
}

func TestRobustSyncDeleteToRobust(t *testing.T) {
	r := newRobustSyncer(t)
	wl := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "wl1", Labels: map[string]string{v1.ClusterIdLabel: "c1"}}}
	r.syncDeleteToRobust(context.Background(), wl)
	// No cluster on workload -> also covered.
	r.syncDeleteToRobust(context.Background(), &v1.Workload{})
}

func TestRobustReconcileDeletion(t *testing.T) {
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

// ---- stats_robust_syncer ----

type fakeStatsDBWriter struct{}

func (fakeStatsDBWriter) UpsertWorkloadStatistic(_ context.Context, _ string, _ []WorkloadHourlyStats) error {
	return nil
}
func (fakeStatsDBWriter) UpsertNodeStatistic(_ context.Context, _ string, _ []NodeStats) error {
	return nil
}

func TestSetupStatsRobustSyncerNil(t *testing.T) {
	// Nil deps -> skip.
	assert.NoError(t, SetupStatsRobustSyncer(nil, nil, nil))
}

func TestStatsSyncNoClusters(t *testing.T) {
	s := &StatsRobustSyncer{
		robustClient: robustclient.NewClient(robustclient.ClientConfig{}),
		dbWriter:     fakeStatsDBWriter{},
	}
	// No clusters registered -> loops do nothing.
	s.syncWorkloadStats(context.Background())
	s.syncNodeStats(context.Background())
}

func TestRobustRunCatchUp(t *testing.T) {
	wl := &v1.Workload{
		ObjectMeta: metav1.ObjectMeta{Name: "wl1", Labels: map[string]string{v1.ClusterIdLabel: "c1"}},
		Status:     v1.WorkloadStatus{Phase: v1.WorkloadRunning},
	}
	r := newRobustSyncer(t, wl)
	// Lists workloads, batches per cluster, ForCluster nil -> continue. No panic.
	r.runCatchUp(context.Background())
}
