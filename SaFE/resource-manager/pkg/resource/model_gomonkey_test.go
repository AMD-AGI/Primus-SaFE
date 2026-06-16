/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resource

import (
	"context"
	"testing"

	"github.com/agiledragon/gomonkey/v2"
	"github.com/stretchr/testify/assert"
	batchv1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apis/pkg/client/clientset/versioned/scheme"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
)

// patchS3Config patches the S3-related config getters so the construct* helpers succeed.
func patchS3Config(t *testing.T) *gomonkey.Patches {
	t.Helper()
	p := gomonkey.NewPatches()
	p.ApplyFunc(commonconfig.IsS3Enable, func() bool { return true })
	p.ApplyFunc(commonconfig.GetS3Endpoint, func() string { return "https://minio:9000" })
	p.ApplyFunc(commonconfig.GetS3AccessKey, func() string { return "ak" })
	p.ApplyFunc(commonconfig.GetS3SecretKey, func() string { return "sk" })
	p.ApplyFunc(commonconfig.GetS3Bucket, func() string { return "bucket" })
	p.ApplyFunc(commonconfig.GetModelDownloaderImage, func() string { return "downloader:1" })
	p.ApplyFunc(commonconfig.GetDownloadJoImage, func() string { return "download:1" })
	return p
}

func TestConstructDownloadJobFull(t *testing.T) {
	patches := patchS3Config(t)
	defer patches.Reset()

	model := genMockModel("m1", v1.AccessModeLocal, "ws1")
	model.Spec.Source.URL = "hf://org/repo"
	r := newMockModelReconciler(fake.NewClientBuilder().WithScheme(scheme.Scheme).Build())

	job, err := r.constructDownloadJob(model)
	assert.NoError(t, err)
	assert.NotNil(t, job)
}

func TestConstructCleanupJobFull(t *testing.T) {
	patches := patchS3Config(t)
	defer patches.Reset()

	model := genMockModel("m1", v1.AccessModeLocal, "ws1")
	r := newMockModelReconciler(fake.NewClientBuilder().WithScheme(scheme.Scheme).Build())

	job, err := r.constructCleanupJob(model)
	assert.NoError(t, err)
	assert.NotNil(t, job)
}

func TestConstructLocalDownloadOpsJob(t *testing.T) {
	patches := patchS3Config(t)
	defer patches.Reset()

	model := genMockModel("m1", v1.AccessModeLocal, "ws1")
	model.Status.S3Path = "models/m1"
	workspace := &v1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: "ws1"},
		Spec:       v1.WorkspaceSpec{Cluster: "c1"},
	}
	cl := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(workspace).Build()
	r := newMockModelReconciler(cl)

	lp := &v1.ModelLocalPath{Workspace: "ws1", Path: "models/m1"}
	job, err := r.constructLocalDownloadOpsJob(context.Background(), model, lp)
	assert.NoError(t, err)
	assert.Equal(t, v1.OpsJobDownloadType, job.Spec.Type)
}

func TestModelHandleDeleteCreatesCleanupJob(t *testing.T) {
	patches := patchS3Config(t)
	defer patches.Reset()

	model := genMockModel("m-del", v1.AccessModeLocal, "ws1")
	now := metav1.Now()
	model.DeletionTimestamp = &now
	model.Finalizers = []string{ModelFinalizer}
	s := runtime.NewScheme()
	assert.NoError(t, v1.AddToScheme(s))
	assert.NoError(t, batchv1.AddToScheme(s))
	cl := fake.NewClientBuilder().
		WithScheme(s).
		WithStatusSubresource(model).
		WithObjects(model).
		Build()
	r := newMockModelReconciler(cl)
	res, err := r.handleDelete(context.Background(), model)
	assert.NoError(t, err)
	assert.True(t, res.RequeueAfter > 0)
}

func TestModelHandlePendingCreatesJob(t *testing.T) {
	patches := patchS3Config(t)
	defer patches.Reset()

	model := genMockModel("m-pend", v1.AccessModeLocal, "ws1")
	model.Spec.Source.URL = "hf://org/repo"
	model.Status.Phase = v1.ModelPhasePending
	s := runtime.NewScheme()
	assert.NoError(t, v1.AddToScheme(s))
	assert.NoError(t, batchv1.AddToScheme(s))
	cl := fake.NewClientBuilder().
		WithScheme(s).
		WithStatusSubresource(model).
		WithObjects(model).
		Build()
	r := newMockModelReconciler(cl)
	_, err := r.handlePending(context.Background(), model)
	assert.NoError(t, err)
	assert.Equal(t, v1.ModelPhaseUploading, model.Status.Phase)
}

func TestModelHandlePendingS3Import(t *testing.T) {
	model := genMockModel("m-s3", v1.AccessModeLocal, "ws1")
	model.Labels = map[string]string{v1.ModelS3ImportLabel: v1.TrueStr}
	model.Status.Phase = v1.ModelPhasePending
	ws := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}}
	cl := fake.NewClientBuilder().
		WithScheme(scheme.Scheme).
		WithStatusSubresource(model).
		WithObjects(model, ws).
		Build()
	r := newMockModelReconciler(cl)
	_, err := r.handlePending(context.Background(), model)
	assert.NoError(t, err)
	assert.Equal(t, v1.ModelPhaseDownloading, model.Status.Phase)
}

func TestModelHandleUploadingJobLost(t *testing.T) {
	model := genMockModel("m-up", v1.AccessModeLocal, "ws1")
	model.Status.Phase = v1.ModelPhaseUploading
	s := runtime.NewScheme()
	assert.NoError(t, v1.AddToScheme(s))
	assert.NoError(t, batchv1.AddToScheme(s))
	cl := fake.NewClientBuilder().WithScheme(s).WithStatusSubresource(model).WithObjects(model).Build()
	r := newMockModelReconciler(cl)
	// No upload job -> phase Failed.
	_, err := r.handleUploading(context.Background(), model)
	assert.NoError(t, err)
	assert.Equal(t, v1.ModelPhaseFailed, model.Status.Phase)
}

func TestModelHandleUploadingSucceeded(t *testing.T) {
	model := genMockModel("m-up3", v1.AccessModeLocal, "ws1")
	model.Status.Phase = v1.ModelPhaseUploading
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: "m-up3", Namespace: "primus-safe"},
		Status:     batchv1.JobStatus{Succeeded: 1},
	}
	ws := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}}
	s := runtime.NewScheme()
	assert.NoError(t, v1.AddToScheme(s))
	assert.NoError(t, batchv1.AddToScheme(s))
	cl := fake.NewClientBuilder().WithScheme(s).WithStatusSubresource(model).WithObjects(model, job, ws).Build()
	r := newMockModelReconciler(cl)
	_, err := r.handleUploading(context.Background(), model)
	assert.NoError(t, err)
	assert.Equal(t, v1.ModelPhaseDownloading, model.Status.Phase)
}

func TestModelHandleUploadingFailed(t *testing.T) {
	model := genMockModel("m-up4", v1.AccessModeLocal, "ws1")
	model.Status.Phase = v1.ModelPhaseUploading
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: "m-up4", Namespace: "primus-safe"},
		Status:     batchv1.JobStatus{Failed: 1, Active: 0},
	}
	s := runtime.NewScheme()
	assert.NoError(t, v1.AddToScheme(s))
	assert.NoError(t, batchv1.AddToScheme(s))
	cl := fake.NewClientBuilder().WithScheme(s).WithStatusSubresource(model).WithObjects(model, job).Build()
	r := newMockModelReconciler(cl)
	_, err := r.handleUploading(context.Background(), model)
	assert.NoError(t, err)
	assert.Equal(t, v1.ModelPhaseFailed, model.Status.Phase)
}

func TestModelHandleDownloadingCreatesOpsJob(t *testing.T) {
	patches := patchS3Config(t)
	defer patches.Reset()

	model := genMockModel("m-dl", v1.AccessModeLocal, "ws1")
	model.Status.Phase = v1.ModelPhaseDownloading
	model.Status.S3Path = "models/m-dl"
	model.Status.LocalPaths = []v1.ModelLocalPath{
		{Workspace: "ws1", Path: "/data/models/m-dl", Status: v1.LocalPathStatusPending},
	}
	workspace := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}, Spec: v1.WorkspaceSpec{Cluster: "c1"}}
	cl := fake.NewClientBuilder().
		WithScheme(scheme.Scheme).
		WithStatusSubresource(model).
		WithObjects(model, workspace).
		Build()
	r := newMockModelReconciler(cl)
	_, err := r.handleDownloading(context.Background(), model)
	assert.NoError(t, err)
}

func TestModelHandleUploadingInProgress(t *testing.T) {
	model := genMockModel("m-up2", v1.AccessModeLocal, "ws1")
	model.Status.Phase = v1.ModelPhaseUploading
	jobName := "m-up2"
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: jobName, Namespace: "primus-safe"},
		Status:     batchv1.JobStatus{Active: 1},
	}
	s := runtime.NewScheme()
	assert.NoError(t, v1.AddToScheme(s))
	assert.NoError(t, batchv1.AddToScheme(s))
	cl := fake.NewClientBuilder().WithScheme(s).WithStatusSubresource(model).WithObjects(model, job).Build()
	r := newMockModelReconciler(cl)
	res, err := r.handleUploading(context.Background(), model)
	assert.NoError(t, err)
	assert.True(t, res.RequeueAfter > 0)
}

func TestModelHandleDeleteNoFinalizer(t *testing.T) {
	model := genMockModel("m-del", v1.AccessModeLocal, "ws1")
	now := metav1.Now()
	model.DeletionTimestamp = &now
	model.Finalizers = []string{"other-finalizer"}
	cl := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(model).Build()
	r := newMockModelReconciler(cl)
	// No model finalizer -> nothing to do.
	res, err := r.handleDelete(context.Background(), model)
	assert.NoError(t, err)
	assert.Equal(t, int64(0), res.RequeueAfter.Nanoseconds())
}

func TestConstructLocalDownloadOpsJobNoCluster(t *testing.T) {
	patches := patchS3Config(t)
	defer patches.Reset()

	model := genMockModel("m1", v1.AccessModeLocal, "ws1")
	workspace := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}}
	cl := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(workspace).Build()
	r := newMockModelReconciler(cl)
	lp := &v1.ModelLocalPath{Workspace: "ws1"}
	_, err := r.constructLocalDownloadOpsJob(context.Background(), model, lp)
	assert.Error(t, err)
}
