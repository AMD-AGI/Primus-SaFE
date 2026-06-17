/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package ops_job

import (
	"context"
	"errors"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	batchv1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/event"

	"github.com/agiledragon/gomonkey/v2"
	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	mockclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/mock"
	commonworkspace "github.com/AMD-AIG-AIMA/SAFE/common/pkg/workspace"
)

func patchHFS3Config() *gomonkey.Patches {
	p := gomonkey.NewPatches()
	p.ApplyFunc(commonconfig.IsS3Enable, func() bool { return true })
	p.ApplyFunc(commonconfig.GetS3Endpoint, func() string { return "https://minio:9000" })
	p.ApplyFunc(commonconfig.GetS3Bucket, func() string { return "bucket" })
	p.ApplyFunc(commonconfig.GetDownloadJoImage, func() string { return "download:1" })
	return p
}

func newHFController(t *testing.T, db dbclient.Interface, objs ...client.Object) *HFDatasetDownloadController {
	t.Helper()
	builder := ctrlfake.NewClientBuilder().WithScheme(fullScheme(t))
	for _, o := range objs {
		builder = builder.WithObjects(o)
	}
	return &HFDatasetDownloadController{Client: builder.Build(), dbClient: db}
}

func TestHFDatasetJobPredicate(t *testing.T) {
	p := hfDatasetJobPredicate()
	withLabel := &batchv1.Job{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{HFDatasetJobLabel: "true"}}}
	assert.True(t, p.Create(event.CreateEvent{Object: withLabel}))
	assert.False(t, p.Create(event.CreateEvent{Object: &batchv1.Job{}}))
}

func TestHFJobStatusChangedPredicate(t *testing.T) {
	p := hfJobStatusChangedPredicate()
	old := &batchv1.Job{}
	upd := &batchv1.Job{Status: batchv1.JobStatus{Succeeded: 1}}
	assert.True(t, p.Update(event.UpdateEvent{ObjectOld: old, ObjectNew: upd}))
	assert.False(t, p.Update(event.UpdateEvent{ObjectOld: old, ObjectNew: old.DeepCopy()}))
}

func TestSetupHFDatasetDownloadControllerDBDisabled(t *testing.T) {
	// DB disabled by default -> returns nil without touching manager.
	assert.NoError(t, SetupHFDatasetDownloadController(context.Background(), nil))
}

func TestHFReconcileNotFound(t *testing.T) {
	r := newHFController(t, nil)
	res, err := r.Reconcile(context.Background(), ctrlruntime.Request{NamespacedName: types.NamespacedName{Name: "missing"}})
	assert.NoError(t, err)
	assert.Equal(t, ctrlruntime.Result{}, res)
}

func TestHFReconcileNoDatasetId(t *testing.T) {
	job := &batchv1.Job{ObjectMeta: metav1.ObjectMeta{Name: "j1"}}
	r := newHFController(t, nil, job)
	_, err := r.Reconcile(context.Background(), ctrlruntime.Request{NamespacedName: types.NamespacedName{Name: "j1"}})
	assert.NoError(t, err)
}

func TestHFReconcileGetDatasetError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	db := mockclient.NewMockInterface(ctrl)
	db.EXPECT().GetDataset(gomock.Any(), "ds1").Return(nil, errors.New("db error"))

	job := &batchv1.Job{ObjectMeta: metav1.ObjectMeta{
		Name:   "j1",
		Labels: map[string]string{HFDatasetIdLabel: "ds1"},
	}}
	r := newHFController(t, db, job)
	_, err := r.Reconcile(context.Background(), ctrlruntime.Request{NamespacedName: types.NamespacedName{Name: "j1"}})
	assert.Error(t, err)
}

func TestHFReconcileInProgress(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	db := mockclient.NewMockInterface(ctrl)
	db.EXPECT().GetDataset(gomock.Any(), "ds1").Return(&dbclient.Dataset{DatasetId: "ds1"}, nil)

	job := &batchv1.Job{ObjectMeta: metav1.ObjectMeta{
		Name:   "j1",
		Labels: map[string]string{HFDatasetIdLabel: "ds1"},
	}}
	r := newHFController(t, db, job)
	res, err := r.Reconcile(context.Background(), ctrlruntime.Request{NamespacedName: types.NamespacedName{Name: "j1"}})
	assert.NoError(t, err)
	assert.True(t, res.RequeueAfter > 0)
}

func TestHFInitializeLocalPathsPublic(t *testing.T) {
	r := newHFController(t, nil)
	// Public dataset (no workspace), no workspaces in cluster -> empty targets.
	jsonStr, targets, err := r.initializeLocalPaths(context.Background(), &dbclient.Dataset{DatasetId: "ds1", DisplayName: "d"})
	assert.NoError(t, err)
	assert.Equal(t, "[]", jsonStr)
	assert.Empty(t, targets)
}

func TestHFInitializeLocalPathsPrivateMissingWorkspace(t *testing.T) {
	r := newHFController(t, nil)
	_, _, err := r.initializeLocalPaths(context.Background(), &dbclient.Dataset{DatasetId: "ds1", Workspace: "missing"})
	assert.Error(t, err)
}

func TestHFGetS3FileInfoDisabled(t *testing.T) {
	r := newHFController(t, nil)
	size, count := r.getS3FileInfo(context.Background(), "path")
	assert.Equal(t, int64(0), size)
	assert.Equal(t, 0, count)
}

func TestHFCreateLocalDownloadOpsJobs(t *testing.T) {
	patches := patchHFS3Config()
	defer patches.Reset()

	ws := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}, Spec: v1.WorkspaceSpec{Cluster: "c1"}}
	r := newHFController(t, nil, ws)
	targets := []commonworkspace.DownloadTarget{{Workspace: "ws1", Path: "/data"}}
	err := r.createLocalDownloadOpsJobs(context.Background(), &dbclient.Dataset{DatasetId: "ds1", DisplayName: "d", S3Path: "datasets/d"}, targets)
	assert.NoError(t, err)
}

func TestHFHandleJobSucceeded(t *testing.T) {
	patches := patchHFS3Config()
	defer patches.Reset()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	db := mockclient.NewMockInterface(ctrl)
	db.EXPECT().UpsertDataset(gomock.Any(), gomock.Any()).Return(nil)

	ws := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}, Spec: v1.WorkspaceSpec{Cluster: "c1"}}
	job := &batchv1.Job{ObjectMeta: metav1.ObjectMeta{Name: "j1"}}
	r := newHFController(t, db, ws, job)
	// Private dataset bound to ws1 -> initializes local paths + creates OpsJobs.
	dataset := &dbclient.Dataset{DatasetId: "ds1", DisplayName: "d", Workspace: "ws1", S3Path: "datasets/d"}
	_, err := r.handleJobSucceeded(context.Background(), dataset, job)
	assert.NoError(t, err)
}

func TestHFHandleJobFailed(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	db := mockclient.NewMockInterface(ctrl)
	db.EXPECT().UpsertDataset(gomock.Any(), gomock.Any()).Return(nil)

	job := &batchv1.Job{ObjectMeta: metav1.ObjectMeta{Name: "j1"}}
	r := newHFController(t, db, job)
	_, err := r.handleJobFailed(context.Background(), &dbclient.Dataset{DatasetId: "ds1"}, job)
	assert.NoError(t, err)
}

func TestHFReconcileFailed(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	db := mockclient.NewMockInterface(ctrl)
	db.EXPECT().GetDataset(gomock.Any(), "ds1").Return(&dbclient.Dataset{DatasetId: "ds1"}, nil)
	db.EXPECT().UpsertDataset(gomock.Any(), gomock.Any()).Return(nil)

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: "j1", Labels: map[string]string{HFDatasetIdLabel: "ds1"}},
		Status:     batchv1.JobStatus{Failed: 1, Active: 0},
	}
	r := newHFController(t, db, job)
	_, err := r.Reconcile(context.Background(), ctrlruntime.Request{NamespacedName: types.NamespacedName{Name: "j1"}})
	assert.NoError(t, err)
}
