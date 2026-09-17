/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package ops_job

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/event"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	mockclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/mock"
)

var errDataset = errorsNew("db error")

func assertErr() error { return errDataset }

func errorsNew(s string) error { return &simpleErr{s} }

type simpleErr struct{ s string }

func (e *simpleErr) Error() string { return e.s }

func newDatasetController(t *testing.T, db dbclient.Interface, objs ...*v1.OpsJob) *DatasetDownloadController {
	t.Helper()
	builder := ctrlfake.NewClientBuilder().WithScheme(opsScheme(t))
	for _, o := range objs {
		builder = builder.WithObjects(o)
	}
	return &DatasetDownloadController{Client: builder.Build(), dbClient: db}
}

func workspaceWithPath(name, mountPath string) *v1.Workspace {
	return &v1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: v1.WorkspaceSpec{
			Cluster: "c1",
			Volumes: []v1.WorkspaceVolume{{Type: v1.PFS, MountPath: mountPath}},
		},
	}
}

func TestMapOpsJobPhaseToDatasetStatus(t *testing.T) {
	tests := []struct {
		name     string
		phase    v1.OpsJobPhase
		expected dbclient.DatasetStatus
	}{
		{
			name:     "pending phase",
			phase:    v1.OpsJobPending,
			expected: dbclient.DatasetStatusPending,
		},
		{
			name:     "running phase",
			phase:    v1.OpsJobRunning,
			expected: dbclient.DatasetStatusDownloading,
		},
		{
			name:     "succeeded phase",
			phase:    v1.OpsJobSucceeded,
			expected: dbclient.DatasetStatusReady,
		},
		{
			name:     "failed phase",
			phase:    v1.OpsJobFailed,
			expected: dbclient.DatasetStatusFailed,
		},
		{
			name:     "unknown phase returns pending",
			phase:    v1.OpsJobPhase("Unknown"),
			expected: dbclient.DatasetStatusPending,
		},
		{
			name:     "empty phase returns pending",
			phase:    v1.OpsJobPhase(""),
			expected: dbclient.DatasetStatusPending,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mapOpsJobPhaseToDatasetStatus(tt.phase)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExtractOpsJobFailureMessage(t *testing.T) {
	tests := []struct {
		name     string
		job      *v1.OpsJob
		expected string
	}{
		{
			name: "extract failure message from conditions",
			job: &v1.OpsJob{
				Status: v1.OpsJobStatus{
					Conditions: []metav1.Condition{
						{
							Type:    "Failed",
							Message: "Download failed: connection timeout",
						},
					},
				},
			},
			expected: "Download failed: connection timeout",
		},
		{
			name: "no failure condition returns default message",
			job: &v1.OpsJob{
				Status: v1.OpsJobStatus{
					Conditions: []metav1.Condition{
						{
							Type:    "Running",
							Message: "Job is running",
						},
					},
				},
			},
			expected: "Download failed",
		},
		{
			name: "empty conditions returns default message",
			job: &v1.OpsJob{
				Status: v1.OpsJobStatus{
					Conditions: []metav1.Condition{},
				},
			},
			expected: "Download failed",
		},
		{
			name: "failed condition with empty message returns default",
			job: &v1.OpsJob{
				Status: v1.OpsJobStatus{
					Conditions: []metav1.Condition{
						{
							Type:    "Failed",
							Message: "",
						},
					},
				},
			},
			expected: "Download failed",
		},
		{
			name: "multiple conditions picks failed one",
			job: &v1.OpsJob{
				Status: v1.OpsJobStatus{
					Conditions: []metav1.Condition{
						{
							Type:    "Running",
							Message: "Was running",
						},
						{
							Type:    "Failed",
							Message: "S3 bucket not accessible",
						},
					},
				},
			},
			expected: "S3 bucket not accessible",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractOpsJobFailureMessage(tt.job)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDatasetOpsJobPredicateLogic(t *testing.T) {
	// Test the underlying logic used in datasetOpsJobPredicate
	tests := []struct {
		name     string
		job      *v1.OpsJob
		expected bool
	}{
		{
			name: "job with dataset-id label should match",
			job: &v1.OpsJob{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-job",
					Labels: map[string]string{
						dbclient.DatasetIdLabel: "dataset-abc123",
					},
				},
			},
			expected: true,
		},
		{
			name: "job without dataset-id label should not match",
			job: &v1.OpsJob{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-job",
					Labels: map[string]string{
						"other-label": "value",
					},
				},
			},
			expected: false,
		},
		{
			name: "job with nil labels should not match",
			job: &v1.OpsJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "test-job",
					Labels: nil,
				},
			},
			expected: false,
		},
		{
			name: "job with empty labels should not match",
			job: &v1.OpsJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "test-job",
					Labels: map[string]string{},
				},
			},
			expected: false,
		},
		{
			name: "job with dataset-id and other labels should match",
			job: &v1.OpsJob{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-job",
					Labels: map[string]string{
						dbclient.DatasetIdLabel: "dataset-xyz789",
						v1.WorkspaceIdLabel:     "ws-1",
						"other-label":           "value",
					},
				},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test the underlying logic that datasetOpsJobPredicate uses
			labels := tt.job.GetLabels()
			var hasDatasetId bool
			if labels != nil {
				_, hasDatasetId = labels[dbclient.DatasetIdLabel]
			}
			assert.Equal(t, tt.expected, hasDatasetId)
		})
	}
}

func TestDatasetPredicates(t *testing.T) {
	p := datasetOpsJobPredicate()
	withLabel := &v1.OpsJob{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{dbclient.DatasetIdLabel: "d1"}}}
	assert.True(t, p.Create(event.CreateEvent{Object: withLabel}))
	assert.False(t, p.Create(event.CreateEvent{Object: &v1.OpsJob{}}))

	pp := opsJobPhaseChangedPredicate()
	assert.True(t, pp.Create(event.CreateEvent{Object: &v1.OpsJob{}}))
	oldJob := &v1.OpsJob{}
	newJob := &v1.OpsJob{Status: v1.OpsJobStatus{Phase: v1.OpsJobRunning}}
	assert.True(t, pp.Update(event.UpdateEvent{ObjectOld: oldJob, ObjectNew: newJob}))
	assert.False(t, pp.Update(event.UpdateEvent{ObjectOld: oldJob, ObjectNew: oldJob.DeepCopy()}))
	assert.False(t, pp.Delete(event.DeleteEvent{Object: &v1.OpsJob{}}))
}

func TestDatasetReconcileNotFound(t *testing.T) {
	r := newDatasetController(t, nil)
	res, err := r.Reconcile(context.Background(), ctrlruntime.Request{NamespacedName: types.NamespacedName{Name: "missing"}})
	assert.NoError(t, err)
	assert.Equal(t, ctrlruntime.Result{}, res)
}

func TestDatasetReconcileNoDatasetId(t *testing.T) {
	job := &v1.OpsJob{ObjectMeta: metav1.ObjectMeta{Name: "j1"}}
	r := newDatasetController(t, nil, job)
	_, err := r.Reconcile(context.Background(), ctrlruntime.Request{NamespacedName: types.NamespacedName{Name: "j1"}})
	assert.NoError(t, err)
}

func TestDatasetReconcileNoWorkspace(t *testing.T) {
	job := &v1.OpsJob{ObjectMeta: metav1.ObjectMeta{
		Name:   "j1",
		Labels: map[string]string{dbclient.DatasetIdLabel: "ds1"},
	}}
	r := newDatasetController(t, nil, job)
	_, err := r.Reconcile(context.Background(), ctrlruntime.Request{NamespacedName: types.NamespacedName{Name: "j1"}})
	assert.NoError(t, err)
}

func TestDatasetReconcileUpdateSuccess(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	db := mockclient.NewMockInterface(ctrl)
	db.EXPECT().UpdateDatasetLocalPath(gomock.Any(), "ds1", "ws1", dbclient.DatasetStatusReady, "").Return(nil)

	job := &v1.OpsJob{
		ObjectMeta: metav1.ObjectMeta{
			Name: "j1",
			Labels: map[string]string{
				dbclient.DatasetIdLabel: "ds1",
				v1.WorkspaceIdLabel:     "ws1",
			},
		},
		Status: v1.OpsJobStatus{Phase: v1.OpsJobSucceeded},
	}
	r := newDatasetController(t, db, job)
	res, err := r.Reconcile(context.Background(), ctrlruntime.Request{NamespacedName: types.NamespacedName{Name: "j1"}})
	assert.NoError(t, err)
	assert.Equal(t, ctrlruntime.Result{}, res)
}

func TestDatasetTryFailoverNoLocalPath(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	db := mockclient.NewMockInterface(ctrl)
	db.EXPECT().GetDataset(gomock.Any(), "ds1").Return(&dbclient.Dataset{DatasetId: "ds1"}, nil)

	r := newDatasetController(t, db)
	failed := &v1.OpsJob{ObjectMeta: metav1.ObjectMeta{Name: "j1"}}
	ok, err := r.tryDatasetFailover(context.Background(), "ds1", "ws1", failed)
	assert.NoError(t, err)
	assert.False(t, ok)
}

func TestDatasetTryFailoverGetError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	db := mockclient.NewMockInterface(ctrl)
	db.EXPECT().GetDataset(gomock.Any(), "ds1").Return(nil, assertErr())

	r := newDatasetController(t, db)
	failed := &v1.OpsJob{ObjectMeta: metav1.ObjectMeta{Name: "j1"}}
	_, err := r.tryDatasetFailover(context.Background(), "ds1", "ws1", failed)
	assert.Error(t, err)
}

func TestDatasetTryFailoverSuccess(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	db := mockclient.NewMockInterface(ctrl)
	localPaths := `[{"workspace":"ws1","path":"/wekafs/datasets/d1"}]`
	db.EXPECT().GetDataset(gomock.Any(), "ds1").Return(&dbclient.Dataset{
		DatasetId:  "ds1",
		LocalPaths: localPaths,
	}, nil)
	db.EXPECT().UpsertDataset(gomock.Any(), gomock.Any()).Return(nil)

	// Two workspaces share /wekafs so a failover candidate exists.
	ws1 := workspaceWithPath("ws1", "/wekafs")
	ws2 := workspaceWithPath("ws2", "/wekafs")
	cl := ctrlfake.NewClientBuilder().WithScheme(opsScheme(t)).WithObjects(ws1, ws2).Build()
	r := &DatasetDownloadController{Client: cl, dbClient: db}
	failed := &v1.OpsJob{
		ObjectMeta: metav1.ObjectMeta{Name: "j1", Labels: map[string]string{v1.UserIdLabel: "u1"}},
		Spec:       v1.OpsJobSpec{Type: v1.OpsJobDownloadType, Inputs: []v1.Parameter{{Name: v1.ParameterWorkspace, Value: "ws1"}}},
	}
	ok, err := r.tryDatasetFailover(context.Background(), "ds1", "ws1", failed)
	assert.NoError(t, err)
	assert.True(t, ok)
}

func TestDatasetCreateFailoverOpsJob(t *testing.T) {
	ws := &v1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: "ws2"},
		Spec:       v1.WorkspaceSpec{Cluster: "c1"},
	}
	cl := ctrlfake.NewClientBuilder().WithScheme(opsScheme(t)).WithObjects(ws).Build()
	r := &DatasetDownloadController{Client: cl}
	failed := &v1.OpsJob{
		ObjectMeta: metav1.ObjectMeta{Name: "j1", Labels: map[string]string{v1.UserIdLabel: "u1"}},
		Spec:       v1.OpsJobSpec{Type: v1.OpsJobDownloadType, Inputs: []v1.Parameter{{Name: v1.ParameterWorkspace, Value: "ws1"}}},
	}
	err := r.createFailoverOpsJob(context.Background(), &dbclient.Dataset{DatasetId: "ds1"}, failed, "ws2")
	assert.NoError(t, err)
}

func TestDatasetSaveTriedWorkspaces(t *testing.T) {
	r := &DatasetDownloadController{}
	ds := &dbclient.Dataset{DatasetId: "ds1"}
	r.saveTriedWorkspaces(context.Background(), ds, map[string][]string{"/wekafs": {"ws1"}})
	assert.Contains(t, ds.TriedWorkspaces, "ws1")
}
