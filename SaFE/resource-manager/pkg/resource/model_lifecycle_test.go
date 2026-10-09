/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resource

import (
	"context"
	"encoding/base64"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonsecret "github.com/AMD-AIG-AIMA/SAFE/common/pkg/secret"
)

const (
	lifecycleRoot    = "/data"
	lifecycleSubpath = "team/models/hf"
	lifecyclePath    = "/data/team/models/hf/org--repo"
)

func lifecycleScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	require.NoError(t, v1.AddToScheme(s))
	require.NoError(t, corev1.AddToScheme(s))
	require.NoError(t, batchv1.AddToScheme(s))
	return s
}

func lifecycleWorkspace(name, cluster, root string) *v1.Workspace {
	return &v1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: v1.WorkspaceSpec{
			Cluster: cluster,
			Volumes: []v1.WorkspaceVolume{{Id: 1, Type: v1.HOSTPATH, HostPath: root, MountPath: root}},
		},
	}
}

// lifecycleModel is a private HuggingFace model owned by user "alice".
func lifecycleModel(name string) *v1.Model {
	return &v1.Model{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Labels:      map[string]string{v1.UserIdLabel: "alice"},
			Annotations: map[string]string{v1.UserNameAnnotation: "alice"},
		},
		Spec: v1.ModelSpec{
			DisplayName:   "org/repo",
			Workspace:     "ws1",
			TargetSubpath: lifecycleSubpath,
			Source:        v1.ModelSource{URL: "https://huggingface.co/org/repo", AccessMode: v1.AccessModeLocal},
		},
	}
}

func lifecycleClient(t *testing.T, objs ...client.Object) client.Client {
	t.Helper()
	return fake.NewClientBuilder().
		WithScheme(lifecycleScheme(t)).
		WithStatusSubresource(&v1.Model{}, &v1.OpsJob{}).
		WithObjects(objs...).
		Build()
}

func reconcileModel(t *testing.T, r *ModelReconciler, name string) {
	t.Helper()
	_, err := r.Reconcile(context.Background(), reconcileReq(name))
	require.NoError(t, err)
}

func getModel(t *testing.T, cl client.Client, name string) *v1.Model {
	t.Helper()
	m := &v1.Model{}
	require.NoError(t, cl.Get(context.Background(), client.ObjectKey{Name: name}, m))
	return m
}

func setOpsJobPhase(t *testing.T, cl client.Client, name string, phase v1.OpsJobPhase, outputs ...v1.Parameter) {
	t.Helper()
	job := &v1.OpsJob{}
	require.NoError(t, cl.Get(context.Background(), client.ObjectKey{Name: name}, job))
	job.Status.Phase = phase
	job.Status.Outputs = outputs
	if phase == v1.OpsJobFailed {
		job.Status.Conditions = []metav1.Condition{{Type: "Failed", Status: metav1.ConditionTrue, Reason: "Error", Message: "boom"}}
	}
	require.NoError(t, cl.Status().Update(context.Background(), job))
}

func listOpsJobs(t *testing.T, cl client.Client, jobType v1.OpsJobType) []v1.OpsJob {
	t.Helper()
	jobs := &v1.OpsJobList{}
	require.NoError(t, cl.List(context.Background(), jobs))
	var out []v1.OpsJob
	for _, j := range jobs.Items {
		if j.Spec.Type == jobType {
			out = append(out, j)
		}
	}
	return out
}

// TestModelLifecycleWithoutS3 drives a model from creation to deletion with S3 disabled:
// download straight into workspace storage, Ready with path and size, then a deletion
// that removes the directory before the finalizer is released.
func TestModelLifecycleWithoutS3(t *testing.T) {
	setViper(t, map[string]any{"s3.enable": false, "model.downloader_image": "downloader:1",
		"model.cleanup_image": "cleanup:1"})
	model := lifecycleModel("m1")
	cl := lifecycleClient(t, model, lifecycleWorkspace("ws1", "c1", lifecycleRoot))
	r := newMockModelReconciler(cl)
	ctx := context.Background()

	reconcileModel(t, r, "m1") // finalizer
	reconcileModel(t, r, "m1") // Pending
	reconcileModel(t, r, "m1") // Downloading, path recorded
	m := getModel(t, cl, "m1")
	assert.Equal(t, v1.ModelPhaseDownloading, m.Status.Phase)
	assert.Empty(t, m.Status.S3Path)
	require.Len(t, m.Status.LocalPaths, 1)
	assert.Equal(t, lifecyclePath, m.Status.LocalPaths[0].Path)

	reconcileModel(t, r, "m1") // download job
	downloads := listOpsJobs(t, cl, v1.OpsJobDownloadType)
	require.Len(t, downloads, 1)
	job := downloads[0]
	assert.Equal(t, "downloader:1", *job.Spec.Image)
	assert.Equal(t, lifecyclePath, job.GetParameter(v1.ParameterDestPath).Value)
	assert.Equal(t, "https://huggingface.co/org/repo", job.GetParameter(v1.ParameterEndpoint).Value)
	assert.Nil(t, job.GetParameter(v1.ParameterSecret), "a public model needs no secret")
	assert.Equal(t, "org/repo", job.Spec.Env["HF_REPO_ID"])
	require.NotNil(t, job.Spec.EntryPoint)
	script, err := base64.StdEncoding.DecodeString(*job.Spec.EntryPoint)
	require.NoError(t, err)
	assert.Contains(t, string(script), "--local-dir \"$DEST_PATH\"")
	assert.Contains(t, string(script), modelSizeMarker)
	assert.Equal(t, "alice", v1.GetUserId(&job), "the download runs as the model owner")
	assert.Equal(t, "alice", v1.GetUserName(&job))
	assert.Equal(t, "c1", v1.GetClusterId(&job))

	setOpsJobPhase(t, cl, job.Name, v1.OpsJobSucceeded, v1.Parameter{Name: "result", Value: "done\n" + modelSizeMarker + "12345\n"})
	reconcileModel(t, r, "m1")
	m = getModel(t, cl, "m1")
	assert.Equal(t, v1.ModelPhaseReady, m.Status.Phase)
	assert.Equal(t, v1.LocalPathStatusReady, m.Status.LocalPaths[0].Status)
	assert.Equal(t, int64(12345), m.Status.LocalPaths[0].SizeBytes)

	// Delete: the directory is removed by a cleanup job in the workspace.
	require.NoError(t, cl.Delete(ctx, m))
	reconcileModel(t, r, "m1")
	cleanups := listOpsJobs(t, cl, v1.OpsJobModelCleanupType)
	require.Len(t, cleanups, 1)
	cleanup := cleanups[0]
	assert.Equal(t, lifecyclePath, cleanup.GetParameter(v1.ParameterDestPath).Value)
	assert.Equal(t, "ws1", cleanup.GetParameter(v1.ParameterWorkspace).Value)
	assert.Equal(t, "alice", v1.GetUserId(&cleanup))
	assert.Equal(t, "cleanup:1", *cleanup.Spec.Image)
	assert.True(t, controllerutil.ContainsFinalizer(getModel(t, cl, "m1"), ModelFinalizer))

	setOpsJobPhase(t, cl, cleanup.Name, v1.OpsJobSucceeded)
	reconcileModel(t, r, "m1") // directory gone -> entry dropped
	reconcileModel(t, r, "m1") // finalizer released
	err = cl.Get(ctx, client.ObjectKey{Name: "m1"}, &v1.Model{})
	assert.True(t, errors.IsNotFound(err), "model should be gone once its files are, got %v", err)
}

// TestModelHFDownloadWithToken mounts the token secret and shares it with the workspace.
func TestModelHFDownloadWithToken(t *testing.T) {
	setViper(t, map[string]any{"s3.enable": false})
	model := lifecycleModel("m1")
	model.Spec.Source.Token = &corev1.LocalObjectReference{Name: "m1-token"}
	secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "m1-token", Namespace: common.PrimusSafeNamespace}}
	cl := lifecycleClient(t, model, secret, lifecycleWorkspace("ws1", "c1", lifecycleRoot))
	r := newMockModelReconciler(cl)

	lp := &v1.ModelLocalPath{Workspace: "ws1", Path: lifecyclePath}
	job, err := r.constructLocalDownloadOpsJob(context.Background(), model, lp)
	require.NoError(t, err)
	require.NotNil(t, job.GetParameter(v1.ParameterSecret))
	assert.Equal(t, "m1-token", job.GetParameter(v1.ParameterSecret).Value)

	got := &corev1.Secret{}
	require.NoError(t, cl.Get(context.Background(), client.ObjectKey{Name: "m1-token", Namespace: common.PrimusSafeNamespace}, got))
	assert.Equal(t, []string{"ws1"}, commonsecret.GetSecretWorkspaces(got))
}

func deletingModel(t *testing.T, name string, lps ...v1.ModelLocalPath) *v1.Model {
	t.Helper()
	m := lifecycleModel(name)
	m.Finalizers = []string{ModelFinalizer}
	now := metav1.Now()
	m.DeletionTimestamp = &now
	m.Status.Phase = v1.ModelPhaseReady
	m.Status.LocalPaths = lps
	return m
}

// TestModelDeleteCleanupFailureKeepsFinalizer: a failed cleanup is retried, the model
// is not released with its files on disk.
func TestModelDeleteCleanupFailureKeepsFinalizer(t *testing.T) {
	model := deletingModel(t, "m1", v1.ModelLocalPath{Workspace: "ws1", Path: lifecyclePath, Status: v1.LocalPathStatusReady})
	cl := lifecycleClient(t, model, lifecycleWorkspace("ws1", "c1", lifecycleRoot))
	r := newMockModelReconciler(cl)

	reconcileModel(t, r, "m1")
	jobName := cleanupJobName(model, lifecyclePath)
	setOpsJobPhase(t, cl, jobName, v1.OpsJobFailed)

	res, err := r.Reconcile(context.Background(), reconcileReq("m1"))
	require.NoError(t, err)
	assert.Equal(t, cleanupRetryInterval, res.RequeueAfter)
	m := getModel(t, cl, "m1")
	assert.True(t, controllerutil.ContainsFinalizer(m, ModelFinalizer), "a failed cleanup must not release the model")
	require.Len(t, m.Status.LocalPaths, 1, "the path is still on disk")
	assert.Contains(t, m.Status.Message, "failed, retrying")
	// The failed job is removed so the next pass runs a fresh one.
	err = cl.Get(context.Background(), client.ObjectKey{Name: jobName}, &v1.OpsJob{})
	assert.True(t, errors.IsNotFound(err))

	reconcileModel(t, r, "m1")
	require.Len(t, listOpsJobs(t, cl, v1.OpsJobModelCleanupType), 1, "cleanup is retried")
	assert.True(t, controllerutil.ContainsFinalizer(getModel(t, cl, "m1"), ModelFinalizer))
}

// TestModelDeleteCleanupCreateFailureKeepsFinalizer: a cleanup that cannot even be
// created (here: the workspace has no cluster) keeps the finalizer.
func TestModelDeleteCleanupCreateFailureKeepsFinalizer(t *testing.T) {
	model := deletingModel(t, "m1", v1.ModelLocalPath{Workspace: "ws1", Path: lifecyclePath})
	cl := lifecycleClient(t, model, lifecycleWorkspace("ws1", "", lifecycleRoot))
	r := newMockModelReconciler(cl)

	res, err := r.Reconcile(context.Background(), reconcileReq("m1"))
	require.NoError(t, err)
	assert.Equal(t, cleanupRetryInterval, res.RequeueAfter)
	m := getModel(t, cl, "m1")
	assert.True(t, controllerutil.ContainsFinalizer(m, ModelFinalizer))
	assert.Contains(t, m.Status.Message, "cannot start")
}

// TestModelDeleteS3CleanupFailureKeepsFinalizer: the S3 copy is treated the same way.
func TestModelDeleteS3CleanupFailureKeepsFinalizer(t *testing.T) {
	patchS3Config(t)
	model := deletingModel(t, "m1")
	model.Status.S3Path = "models/m1"
	failed := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: "cleanup-m1", Namespace: common.PrimusSafeNamespace},
		Status:     batchv1.JobStatus{Failed: 1},
	}
	cl := lifecycleClient(t, model, failed)
	r := newMockModelReconciler(cl)

	res, err := r.Reconcile(context.Background(), reconcileReq("m1"))
	require.NoError(t, err)
	assert.Equal(t, cleanupRetryInterval, res.RequeueAfter)
	m := getModel(t, cl, "m1")
	assert.True(t, controllerutil.ContainsFinalizer(m, ModelFinalizer))
	assert.Equal(t, "models/m1", m.Status.S3Path)

	job, err := r.constructCleanupJob(m)
	require.NoError(t, err)
	assert.Contains(t, job.Spec.Template.Spec.Containers[0].Command[2], "set -e")
}

// TestModelDeleteStopsDownloadFirst: no cleanup runs while a download still writes.
func TestModelDeleteStopsDownloadFirst(t *testing.T) {
	model := deletingModel(t, "m1", v1.ModelLocalPath{Workspace: "ws1", Path: lifecyclePath, Status: v1.LocalPathStatusDownloading})
	download := &v1.OpsJob{
		ObjectMeta: metav1.ObjectMeta{Name: downloadJobName(model, "ws1"), Labels: map[string]string{v1.ModelIdLabel: "m1"}},
		Spec:       v1.OpsJobSpec{Type: v1.OpsJobDownloadType},
	}
	cl := lifecycleClient(t, model, download, lifecycleWorkspace("ws1", "c1", lifecycleRoot))
	r := newMockModelReconciler(cl)

	res, err := r.Reconcile(context.Background(), reconcileReq("m1"))
	require.NoError(t, err)
	assert.True(t, res.RequeueAfter > 0)
	err = cl.Get(context.Background(), client.ObjectKey{Name: download.Name}, &v1.OpsJob{})
	assert.True(t, errors.IsNotFound(err), "the download is stopped")
	assert.Empty(t, listOpsJobs(t, cl, v1.OpsJobModelCleanupType), "no cleanup while the download may still write")

	// Its workload is still terminating: keep waiting.
	wl := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: download.Name}}
	require.NoError(t, cl.Create(context.Background(), wl))
	reconcileModel(t, r, "m1")
	assert.Empty(t, listOpsJobs(t, cl, v1.OpsJobModelCleanupType))

	// Once everything is gone the cleanup starts.
	reconcileModel(t, r, "m1")
	assert.Len(t, listOpsJobs(t, cl, v1.OpsJobModelCleanupType), 1)
}

// TestModelDeleteKeepsSharedPath: a directory another live model points at stays.
func TestModelDeleteKeepsSharedPath(t *testing.T) {
	model := deletingModel(t, "m1", v1.ModelLocalPath{Workspace: "ws1", Path: lifecyclePath})
	other := lifecycleModel("m2")
	other.Status.LocalPaths = []v1.ModelLocalPath{{Workspace: "ws1", Path: lifecyclePath, Status: v1.LocalPathStatusReady}}
	cl := lifecycleClient(t, model, other, lifecycleWorkspace("ws1", "c1", lifecycleRoot))
	r := newMockModelReconciler(cl)

	reconcileModel(t, r, "m1")
	reconcileModel(t, r, "m1")
	assert.Empty(t, listOpsJobs(t, cl, v1.OpsJobModelCleanupType), "the shared directory must not be removed")
	err := cl.Get(context.Background(), client.ObjectKey{Name: "m1"}, &v1.Model{})
	assert.True(t, errors.IsNotFound(err))
}

// TestModelDeleteRefusesUnsafePath never removes a volume root or a non-model path.
func TestModelDeleteRefusesUnsafePath(t *testing.T) {
	ws := lifecycleWorkspace("ws1", "c1", lifecycleRoot)
	for _, p := range []string{"/data", "/data/models", "/data/other/x", "/elsewhere/models/x", "/data/models/../x", "data/models/x"} {
		assert.Error(t, validateCleanupPath(ws, p), p)
	}
	assert.NoError(t, validateCleanupPath(ws, "/data/models/org--repo"))
	assert.NoError(t, validateCleanupPath(ws, lifecyclePath))
}

// TestModelPendingWaitsForDeletionOnSamePath: a model whose directory is still being
// cleaned up by another model's deletion waits instead of downloading into it.
func TestModelPendingWaitsForDeletionOnSamePath(t *testing.T) {
	setViper(t, map[string]any{"s3.enable": false})
	old := deletingModel(t, "old", v1.ModelLocalPath{Workspace: "ws1", Path: lifecyclePath})
	model := lifecycleModel("m1")
	model.Finalizers = []string{ModelFinalizer}
	model.Status.Phase = v1.ModelPhasePending
	cl := lifecycleClient(t, old, model, lifecycleWorkspace("ws1", "c1", lifecycleRoot))
	r := newMockModelReconciler(cl)

	res, err := r.handlePending(context.Background(), getModel(t, cl, "m1"))
	require.NoError(t, err)
	assert.True(t, res.RequeueAfter > 0)
	m := getModel(t, cl, "m1")
	assert.Equal(t, v1.ModelPhasePending, m.Status.Phase)
	assert.Empty(t, m.Status.LocalPaths)
	assert.Contains(t, m.Status.Message, "Waiting for model old")

	// A live model owning the same directory fails the new one instead.
	oldLive := getModel(t, cl, "old")
	oldLive.Finalizers = nil
	require.NoError(t, cl.Update(context.Background(), oldLive)) // old is now gone
	other := lifecycleModel("m2")
	other.Status.LocalPaths = []v1.ModelLocalPath{{Workspace: "ws1", Path: lifecyclePath}}
	require.NoError(t, cl.Create(context.Background(), other))
	require.NoError(t, cl.Status().Update(context.Background(), other))
	_, err = r.handlePending(context.Background(), getModel(t, cl, "m1"))
	require.NoError(t, err)
	assert.Equal(t, v1.ModelPhaseFailed, getModel(t, cl, "m1").Status.Phase)
}

// TestModelDownloadConcurrencyLimit: downloads beyond the global limit wait for a slot.
func TestModelDownloadConcurrencyLimit(t *testing.T) {
	setViper(t, map[string]any{"s3.enable": false, "model.max_concurrent_downloads": 1})
	running := &v1.OpsJob{
		ObjectMeta: metav1.ObjectMeta{Name: "download--other-ws1", Labels: map[string]string{
			v1.ModelIdLabel: "other", v1.OpsJobTypeLabel: string(v1.OpsJobDownloadType)}},
		Spec:   v1.OpsJobSpec{Type: v1.OpsJobDownloadType},
		Status: v1.OpsJobStatus{Phase: v1.OpsJobRunning},
	}
	model := lifecycleModel("m1")
	model.Status.Phase = v1.ModelPhaseDownloading
	model.Status.LocalPaths = []v1.ModelLocalPath{{Workspace: "ws1", Path: lifecyclePath, Status: v1.LocalPathStatusPending}}
	cl := lifecycleClient(t, running, model, lifecycleWorkspace("ws1", "c1", lifecycleRoot))
	r := newMockModelReconciler(cl)
	r.apiReader = cl

	res, err := r.handleDownloading(context.Background(), getModel(t, cl, "m1"))
	require.NoError(t, err)
	assert.Equal(t, downloadSlotWaitInterval, res.RequeueAfter)
	m := getModel(t, cl, "m1")
	assert.Equal(t, v1.LocalPathStatusPending, m.Status.LocalPaths[0].Status)
	assert.Contains(t, m.Status.LocalPaths[0].Message, "Waiting for a download slot (1/1 in use)")
	assert.Equal(t, v1.ModelPhaseDownloading, m.Status.Phase)
	assert.Len(t, listOpsJobs(t, cl, v1.OpsJobDownloadType), 1, "no second download beyond the limit")

	// The slot frees up once the running download finishes.
	setOpsJobPhase(t, cl, running.Name, v1.OpsJobSucceeded)
	_, err = r.handleDownloading(context.Background(), getModel(t, cl, "m1"))
	require.NoError(t, err)
	assert.Equal(t, v1.LocalPathStatusDownloading, getModel(t, cl, "m1").Status.LocalPaths[0].Status)
	assert.Len(t, listOpsJobs(t, cl, v1.OpsJobDownloadType), 2)
}

// TestModelFailoverStaysOnCluster: failover only moves to a workspace of the same
// cluster that mounts the same volume.
func TestModelFailoverStaysOnCluster(t *testing.T) {
	model := lifecycleModel("m1")
	lp := &v1.ModelLocalPath{Workspace: "ws1", Path: lifecyclePath}

	// Only another cluster mounts the same path: no failover.
	cl := lifecycleClient(t, model,
		lifecycleWorkspace("ws1", "c1", lifecycleRoot),
		lifecycleWorkspace("ws-other-cluster", "c2", lifecycleRoot))
	r := newMockModelReconciler(cl)
	assert.False(t, r.tryFailover(context.Background(), model, lp))
	assert.Equal(t, "ws1", lp.Workspace)

	// A workspace of the same cluster on the same volume is picked.
	cl = lifecycleClient(t, model,
		lifecycleWorkspace("ws1", "c1", lifecycleRoot),
		lifecycleWorkspace("ws-other-cluster", "c2", lifecycleRoot),
		lifecycleWorkspace("ws-other-volume", "c1", "/other"),
		lifecycleWorkspace("ws2", "c1", lifecycleRoot))
	r = newMockModelReconciler(cl)
	assert.True(t, r.tryFailover(context.Background(), model, lp))
	assert.Equal(t, "ws2", lp.Workspace)
	assert.Equal(t, v1.LocalPathStatusPending, lp.Status)
}

func TestReportedModelSize(t *testing.T) {
	job := &v1.OpsJob{Status: v1.OpsJobStatus{Outputs: []v1.Parameter{{Name: "result", Value: "x " + modelSizeMarker + "1\n" + modelSizeMarker + "42"}}}}
	assert.Equal(t, int64(42), reportedModelSize(job))
	assert.Equal(t, int64(0), reportedModelSize(&v1.OpsJob{}))
}

func TestJobDisplayName(t *testing.T) {
	for _, in := range []string{"Org--Repo_V1.5", "0abc", "cleanup-a-very-long-model-name-that-goes-beyond-the-limit"} {
		name := jobDisplayName(in)
		assert.LessOrEqual(t, len(name), 41)
		assert.Regexp(t, `^[a-z][-a-z0-9]*[a-z0-9]$`, name)
	}
}
