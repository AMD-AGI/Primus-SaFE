/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package ops_job

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/golang/mock/gomock"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	mockclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/mock"
)

func assertErr() error { return errDataset }

var errDataset = errorsNew("db error")

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

func workspaceWithPath(name, mountPath string) *v1.Workspace {
	return &v1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: v1.WorkspaceSpec{
			Cluster: "c1",
			Volumes: []v1.WorkspaceVolume{{Type: v1.PFS, MountPath: mountPath}},
		},
	}
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
	scheme := opsScheme(t)
	cl := ctrlfake.NewClientBuilder().WithScheme(scheme).WithObjects(ws1, ws2).Build()
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
	scheme := opsScheme(t)
	cl := ctrlfake.NewClientBuilder().WithScheme(scheme).WithObjects(ws).Build()
	r := &DatasetDownloadController{Client: cl}
	failed := &v1.OpsJob{
		ObjectMeta: metav1.ObjectMeta{Name: "j1", Labels: map[string]string{v1.UserIdLabel: "u1"}},
		Spec:       v1.OpsJobSpec{Type: v1.OpsJobDownloadType, Inputs: []v1.Parameter{{Name: v1.ParameterWorkspace, Value: "ws1"}}},
	}
	err := r.createFailoverOpsJob(context.Background(), &dbclient.Dataset{DatasetId: "ds1"}, failed, "ws2")
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
