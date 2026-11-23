/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resource

import (
	"context"
	"encoding/base64"
	stderrors "errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	mockclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/mock"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	commonopsjob "github.com/AMD-AIG-AIMA/SAFE/common/pkg/ops_job"
	commonsecret "github.com/AMD-AIG-AIMA/SAFE/common/pkg/secret"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/stringutil"
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
		job.Status.Conditions = []metav1.Condition{failedJobCondition("[ERROR] boom\n")}
	}
	require.NoError(t, cl.Status().Update(context.Background(), job))
}

// failedJobCondition is the condition the OpsJob controller writes when a job fails
// after its workload printed log: the job-manager keeps the marked lines of the log, and
// a job that kept none says "unknown". The literals are the platform's, not the
// controller's constants, so a drift between the two shows up here.
func failedJobCondition(log string) metav1.Condition {
	message := commonopsjob.FilterResultLog([]byte(log))
	if message == "" {
		message = "unknown"
	}
	return metav1.Condition{Type: "JobCompleted", Status: metav1.ConditionFalse, Reason: "JobFailed", Message: message}
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
	assert.Equal(t, lifecycleRoot, cleanup.Spec.Env["VOLUME_ROOT"], "the script checks the path against this volume")
	assert.True(t, controllerutil.ContainsFinalizer(getModel(t, cl, "m1"), ModelFinalizer))

	setOpsJobPhase(t, cl, cleanup.Name, v1.OpsJobSucceeded)
	reconcileModel(t, r, "m1") // directory gone -> entry dropped
	reconcileModel(t, r, "m1") // finalizer released
	err = cl.Get(ctx, client.ObjectKey{Name: "m1"}, &v1.Model{})
	assert.True(t, errors.IsNotFound(err), "model should be gone once its files are, got %v", err)
}

// TestModelHFDownloadWithToken: the download mounts a copy of the owner's token made for
// that one download, never the owner's secret. The copy is shared with the download's
// workspace only, carries no user (so no workspace member can attach it to a workload),
// is owned by the model and is deleted, through the secret finalizer that removes the
// mirrored copy, as soon as the download has finished. A token secret an earlier version
// shared with the workspace is taken back.
func TestModelHFDownloadWithToken(t *testing.T) {
	setViper(t, map[string]any{"s3.enable": false})
	ctx := context.Background()
	model := lifecycleModel("m1")
	model.Spec.Source.Token = &corev1.LocalObjectReference{Name: "m1-token"}
	owned := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "m1-token", Namespace: common.PrimusSafeNamespace,
			Labels:      map[string]string{v1.ModelIdLabel: "m1"},
			Annotations: map[string]string{v1.WorkspaceIdsAnnotation: `["ws1"]`}},
		Data: map[string][]byte{"token": []byte("hf_secret")},
	}
	cl := lifecycleClient(t, model, owned, lifecycleWorkspace("ws1", "c1", lifecycleRoot))
	r := newMockModelReconciler(cl)

	reconcileModel(t, r, "m1") // finalizer; the earlier sharing is taken back
	got := &corev1.Secret{}
	require.NoError(t, cl.Get(ctx, client.ObjectKey{Name: "m1-token", Namespace: common.PrimusSafeNamespace}, got))
	assert.Empty(t, commonsecret.GetSecretWorkspaces(got), "the owner's secret is not shared with any workspace")

	reconcileModel(t, r, "m1") // Pending
	reconcileModel(t, r, "m1") // Downloading
	reconcileModel(t, r, "m1") // download job
	downloads := listOpsJobs(t, cl, v1.OpsJobDownloadType)
	require.Len(t, downloads, 1)
	param := downloads[0].GetParameter(v1.ParameterSecret)
	require.NotNil(t, param)
	require.NotEqual(t, "m1-token", param.Value, "the download never mounts the owner's secret")
	assert.LessOrEqual(t, len(param.Value), 63, "the secret is mounted as a volume named after it")

	copied := &corev1.Secret{}
	require.NoError(t, cl.Get(ctx, client.ObjectKey{Name: param.Value, Namespace: common.PrimusSafeNamespace}, copied))
	assert.Equal(t, map[string][]byte{"token": []byte("hf_secret")}, copied.Data, "the copy holds the token only")
	assert.Equal(t, []string{"ws1"}, commonsecret.GetSecretWorkspaces(copied), "mirrored to the download's workspace only")
	assert.Empty(t, v1.GetUserId(copied), "no user owns the copy, so no member can attach it")
	assert.Contains(t, copied.Finalizers, v1.SecretFinalizer, "the secret controller removes the mirror first")
	require.NotNil(t, metav1.GetControllerOf(copied))
	assert.Equal(t, "m1", metav1.GetControllerOf(copied).Name)
	require.NoError(t, cl.Get(ctx, client.ObjectKey{Name: "m1-token", Namespace: common.PrimusSafeNamespace}, got))
	assert.Empty(t, commonsecret.GetSecretWorkspaces(got))

	// The download finishes: the copy goes.
	setOpsJobPhase(t, cl, downloads[0].Name, v1.OpsJobSucceeded)
	reconcileModel(t, r, "m1")
	assert.Equal(t, v1.ModelPhaseReady, getModel(t, cl, "m1").Status.Phase)
	assertSecretGoing(t, cl, param.Value)
}

// TestModelDeleteRemovesDownloadToken: deleting a model whose download still runs stops
// the download and deletes the token copy with it.
func TestModelDeleteRemovesDownloadToken(t *testing.T) {
	setViper(t, map[string]any{"s3.enable": false})
	ctx := context.Background()
	model := lifecycleModel("m1")
	model.Spec.Source.Token = &corev1.LocalObjectReference{Name: "m1-token"}
	owned := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "m1-token", Namespace: common.PrimusSafeNamespace,
			Labels: map[string]string{v1.ModelIdLabel: "m1"}},
		Data: map[string][]byte{"token": []byte("hf_secret")},
	}
	cl := lifecycleClient(t, model, owned, lifecycleWorkspace("ws1", "c1", lifecycleRoot))
	r := newMockModelReconciler(cl)
	for i := 0; i < 4; i++ {
		reconcileModel(t, r, "m1")
	}
	downloads := listOpsJobs(t, cl, v1.OpsJobDownloadType)
	require.Len(t, downloads, 1)
	name := downloads[0].GetParameter(v1.ParameterSecret).Value
	require.NoError(t, cl.Get(ctx, client.ObjectKey{Name: name, Namespace: common.PrimusSafeNamespace}, &corev1.Secret{}))

	require.NoError(t, cl.Delete(ctx, getModel(t, cl, "m1")))
	reconcileModel(t, r, "m1") // stops the download
	reconcileModel(t, r, "m1") // download gone: the copy goes
	assertSecretGoing(t, cl, name)
}

// assertSecretGoing checks that the secret is deleted or, held by its finalizer until the
// secret controller removed the mirrored copies, being deleted.
func assertSecretGoing(t *testing.T, cl client.Client, name string) {
	t.Helper()
	secret := &corev1.Secret{}
	err := cl.Get(context.Background(), client.ObjectKey{Name: name, Namespace: common.PrimusSafeNamespace}, secret)
	if errors.IsNotFound(err) {
		return
	}
	require.NoError(t, err)
	assert.False(t, secret.GetDeletionTimestamp().IsZero(), "the token copy %s outlives its download", name)
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
	jobName := cleanupJobName(model, "ws1", lifecyclePath)
	setOpsJobPhase(t, cl, jobName, v1.OpsJobFailed)

	res, err := r.Reconcile(context.Background(), reconcileReq("m1"))
	require.NoError(t, err)
	assert.Equal(t, cleanupRetryInterval, res.RequeueAfter)
	m := getModel(t, cl, "m1")
	assert.True(t, controllerutil.ContainsFinalizer(m, ModelFinalizer), "a failed cleanup must not release the model")
	require.Len(t, m.Status.LocalPaths, 1, "the path is still on disk")
	assert.Contains(t, m.Status.Message, `failed: ["[ERROR] boom"] (failure 1), retrying in 30s`)
	assert.Equal(t, int32(1), m.Status.LocalPaths[0].CleanupFailures)
	// The failed job is removed so the next pass runs a fresh one.
	err = cl.Get(context.Background(), client.ObjectKey{Name: jobName}, &v1.OpsJob{})
	assert.True(t, errors.IsNotFound(err))

	// The next attempt waits for the backoff, however often the model is reconciled.
	res, err = r.Reconcile(context.Background(), reconcileReq("m1"))
	require.NoError(t, err)
	assert.Empty(t, listOpsJobs(t, cl, v1.OpsJobModelCleanupType), "no retry before the backoff")
	assert.True(t, res.RequeueAfter > 0 && res.RequeueAfter <= cleanupRetryInterval, "requeue %s", res.RequeueAfter)

	ageCleanupFailure(t, cl, "m1", cleanupRetryInterval)
	reconcileModel(t, r, "m1")
	require.Len(t, listOpsJobs(t, cl, v1.OpsJobModelCleanupType), 1, "cleanup is retried")
	assert.True(t, controllerutil.ContainsFinalizer(getModel(t, cl, "m1"), ModelFinalizer))
}

// ageCleanupFailure moves the last cleanup failure of every path of a model back by d.
func ageCleanupFailure(t *testing.T, cl client.Client, name string, d time.Duration) {
	t.Helper()
	m := getModel(t, cl, name)
	for i := range m.Status.LocalPaths {
		if at := m.Status.LocalPaths[i].LastCleanupFailureTime; at != nil {
			m.Status.LocalPaths[i].LastCleanupFailureTime = &metav1.Time{Time: at.Add(-d)}
		}
	}
	require.NoError(t, cl.Status().Update(context.Background(), m))
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
	wl := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: download.Name, Labels: map[string]string{
		v1.ModelIdLabel: "m1", v1.OpsJobTypeLabel: string(v1.OpsJobDownloadType)}}}
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
	// A single-character display name, such as "a", gives a one-character name that the
	// OpsJob webhook refuses: its rule takes at least two characters.
	for _, in := range []string{"Org--Repo_V1.5", "0abc", "cleanup-a-very-long-model-name-that-goes-beyond-the-limit", "a", "A", "_"} {
		name := jobDisplayName(in)
		assert.LessOrEqual(t, len(name), 46)
		assert.Regexp(t, `^[a-z][-a-z0-9.]{0,44}[a-z0-9]$`, name, "input %q", in)
	}
}

// TestModelPendingSingleCharacterDisplayName: a model named "a" gets a download job the
// webhook's display name rule accepts.
func TestModelPendingSingleCharacterDisplayName(t *testing.T) {
	setViper(t, map[string]any{"s3.enable": false})
	model := lifecycleModel("m1")
	// Not a HuggingFace repo: the directory, and so the job, is named after the display name.
	model.Labels[v1.ModelS3ImportLabel] = v1.TrueStr
	model.Annotations[v1.ModelS3SourceEndpointAnn] = "http://minio:9000"
	model.Spec.Source.URL = "s3://bucket/a"
	model.Spec.DisplayName = "a"
	model.Finalizers = []string{ModelFinalizer}
	model.Status.Phase = v1.ModelPhasePending
	cl := lifecycleClient(t, model, lifecycleWorkspace("ws1", "c1", lifecycleRoot))
	r := newMockModelReconciler(cl)

	reconcileModel(t, r, "m1")
	reconcileModel(t, r, "m1")
	downloads := listOpsJobs(t, cl, v1.OpsJobDownloadType)
	require.Len(t, downloads, 1)
	assert.Regexp(t, `^[a-z][-a-z0-9.]{0,44}[a-z0-9]$`, v1.GetDisplayName(&downloads[0]))
}

// TestModelPendingRetriesWorkspaceReadError: a failure to read the workspaces is
// retried, it is not taken for "no storage" and the model is not failed.
func TestModelPendingRetriesWorkspaceReadError(t *testing.T) {
	setViper(t, map[string]any{"s3.enable": false})
	for _, public := range []bool{false, true} {
		model := lifecycleModel("m1")
		if public {
			model.Spec.Workspace = ""
		}
		model.Status.Phase = v1.ModelPhasePending
		fail := true
		cl := fake.NewClientBuilder().WithScheme(lifecycleScheme(t)).
			WithStatusSubresource(&v1.Model{}, &v1.OpsJob{}).
			WithObjects(model, lifecycleWorkspace("ws1", "c1", lifecycleRoot)).
			WithInterceptorFuncs(interceptor.Funcs{
				Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
					if _, ok := obj.(*v1.Workspace); ok && fail {
						return errors.NewServiceUnavailable("etcd leader changed")
					}
					return c.Get(ctx, key, obj, opts...)
				},
				List: func(ctx context.Context, c client.WithWatch, list client.ObjectList, opts ...client.ListOption) error {
					if _, ok := list.(*v1.WorkspaceList); ok && fail {
						return errors.NewServiceUnavailable("etcd leader changed")
					}
					return c.List(ctx, list, opts...)
				},
			}).Build()
		r := newMockModelReconciler(cl)

		_, err := r.handlePending(context.Background(), getModel(t, cl, "m1"))
		require.Error(t, err, "public=%v: the read error is retried with backoff", public)
		m := getModel(t, cl, "m1")
		assert.Equal(t, v1.ModelPhasePending, m.Status.Phase, "public=%v: %s", public, m.Status.Message)

		fail = false
		_, err = r.handlePending(context.Background(), getModel(t, cl, "m1"))
		require.NoError(t, err)
		m = getModel(t, cl, "m1")
		assert.Equal(t, v1.ModelPhaseDownloading, m.Status.Phase, "public=%v: %s", public, m.Status.Message)
		require.Len(t, m.Status.LocalPaths, 1)
	}
}

// TestModelPendingNoVolumeFailsWithReason: a model with no workspace storage to
// download into fails at once, and says why.
func TestModelPendingNoVolumeFailsWithReason(t *testing.T) {
	setViper(t, map[string]any{"s3.enable": false})
	noVolume := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}, Spec: v1.WorkspaceSpec{Cluster: "c1"}}
	for _, public := range []bool{false, true} {
		model := lifecycleModel("m1")
		if public {
			model.Spec.Workspace = ""
		}
		model.Status.Phase = v1.ModelPhasePending
		cl := lifecycleClient(t, model, noVolume.DeepCopy())
		r := newMockModelReconciler(cl)

		_, err := r.handlePending(context.Background(), getModel(t, cl, "m1"))
		require.NoError(t, err)
		m := getModel(t, cl, "m1")
		assert.Equal(t, v1.ModelPhaseFailed, m.Status.Phase, "public=%v", public)
		assert.Contains(t, m.Status.Message, "storage volume", "public=%v", public)
	}
}

// TestModelUploadJobOutlivesPathWait: a succeeded upload job waits up to
// deletingPathWaitTimeout for another model's cleanup and reads its result from the
// job meanwhile, so the job must not be reaped by its TTL before that wait ends.
func TestModelUploadJobOutlivesPathWait(t *testing.T) {
	patchS3Config(t)
	model := lifecycleModel("m1")
	model.Status.Phase = v1.ModelPhaseUploading
	job, err := newMockModelReconciler(lifecycleClient(t)).constructDownloadJob(model)
	require.NoError(t, err)
	job.Status.Succeeded = 1
	old := deletingModel(t, "old", v1.ModelLocalPath{Workspace: "ws1", Path: lifecyclePath, Status: v1.LocalPathStatusReady})
	cl := lifecycleClient(t, model, old, job, lifecycleWorkspace("ws1", "c1", lifecycleRoot))
	r := newMockModelReconciler(cl)

	res, err := r.handleUploading(context.Background(), getModel(t, cl, "m1"))
	require.NoError(t, err)
	require.Positive(t, res.RequeueAfter, "the model waits for the cleanup")
	assert.Equal(t, v1.ModelPhaseUploading, getModel(t, cl, "m1").Status.Phase)
	require.NotNil(t, job.Spec.TTLSecondsAfterFinished)
	ttl := time.Duration(*job.Spec.TTLSecondsAfterFinished) * time.Second
	assert.Greater(t, ttl, deletingPathWaitTimeout, "the job outlives the wait it is read during")
}

// TestModelPrivateWorkspaceWithoutVolume: no storage means no download target, the
// model fails instead of downloading into the container's own filesystem.
func TestModelPrivateWorkspaceWithoutVolume(t *testing.T) {
	ws := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}, Spec: v1.WorkspaceSpec{Cluster: "c1"}}
	cl := lifecycleClient(t, ws)
	r := newMockModelReconciler(cl)
	paths, err := r.initializeLocalPaths(context.Background(), lifecycleModel("m1"))
	require.NoError(t, err)
	assert.Empty(t, paths)
}

// TestModelPublicDownloadPerCluster: the same mount path on two clusters is two
// filesystems, so a public model is downloaded once per cluster.
func TestModelPublicDownloadPerCluster(t *testing.T) {
	cl := lifecycleClient(t,
		lifecycleWorkspace("ws-a1", "a", lifecycleRoot),
		lifecycleWorkspace("ws-a2", "a", lifecycleRoot),
		lifecycleWorkspace("ws-b1", "b", lifecycleRoot))
	r := newMockModelReconciler(cl)
	model := lifecycleModel("m1")
	model.Spec.Workspace = ""
	paths, err := r.initializeLocalPaths(context.Background(), model)
	require.NoError(t, err)
	require.Len(t, paths, 2)
	clusters := map[string]bool{}
	for _, lp := range paths {
		assert.Equal(t, lifecyclePath, lp.Path)
		ws := &v1.Workspace{}
		require.NoError(t, cl.Get(context.Background(), client.ObjectKey{Name: lp.Workspace}, ws))
		clusters[ws.Spec.Cluster] = true
	}
	assert.Equal(t, map[string]bool{"a": true, "b": true}, clusters)
	assert.NotEqual(t, cleanupJobName(model, paths[0].Workspace, lifecyclePath),
		cleanupJobName(model, paths[1].Workspace, lifecyclePath))
}

// TestModelDeleteUnreachablePathReleases: a recorded path that no workspace mounts any
// more cannot be removed, and nothing can write into it either. It is never handed to a
// cleanup job, and the model is released with a warning instead of staying stuck.
func TestModelDeleteUnreachablePathReleases(t *testing.T) {
	model := deletingModel(t, "m1", v1.ModelLocalPath{Workspace: "ws1", Path: lifecyclePath})
	cl := lifecycleClient(t, model, lifecycleWorkspace("ws1", "c1", "/elsewhere"),
		lifecycleWorkspace("ws-other-cluster", "c2", lifecycleRoot))
	r := newMockModelReconciler(cl)
	events := record.NewFakeRecorder(8)
	r.recorder = events

	reconcileModel(t, r, "m1")
	reconcileModel(t, r, "m1")
	assert.Empty(t, listOpsJobs(t, cl, v1.OpsJobModelCleanupType))
	err := cl.Get(context.Background(), client.ObjectKey{Name: "m1"}, &v1.Model{})
	assert.True(t, errors.IsNotFound(err), "the model is released, got %v", err)
	require.Len(t, events.Events, 1)
	event := <-events.Events
	assert.Contains(t, event, "CleanupSkipped")
	assert.Contains(t, event, "Leaving "+lifecyclePath+" on disk")
	assert.Contains(t, event, "not inside a volume")
}

// TestModelDeleteWorkspaceGoneReleases: without its workspace the directory cannot be
// reached; the model is released with a warning.
func TestModelDeleteWorkspaceGoneReleases(t *testing.T) {
	model := deletingModel(t, "m1", v1.ModelLocalPath{Workspace: "gone", Path: lifecyclePath})
	cl := lifecycleClient(t, model, lifecycleWorkspace("ws1", "c1", lifecycleRoot))
	r := newMockModelReconciler(cl)
	events := record.NewFakeRecorder(8)
	r.recorder = events

	reconcileModel(t, r, "m1")
	reconcileModel(t, r, "m1")
	assert.Empty(t, listOpsJobs(t, cl, v1.OpsJobModelCleanupType))
	err := cl.Get(context.Background(), client.ObjectKey{Name: "m1"}, &v1.Model{})
	assert.True(t, errors.IsNotFound(err), "the model is released, got %v", err)
	require.Len(t, events.Events, 1)
	assert.Contains(t, <-events.Events, "workspace gone no longer exists")
}

// TestModelDeleteCleansFromAnotherWorkspace: when the recorded workspace no longer
// mounts the directory, another workspace of the same cluster that does runs the cleanup.
func TestModelDeleteCleansFromAnotherWorkspace(t *testing.T) {
	model := deletingModel(t, "m1", v1.ModelLocalPath{Workspace: "ws1", Path: lifecyclePath})
	cl := lifecycleClient(t, model,
		lifecycleWorkspace("ws1", "c1", "/elsewhere"),
		lifecycleWorkspace("ws-other-cluster", "c2", lifecycleRoot),
		lifecycleWorkspace("ws2", "c1", lifecycleRoot))
	r := newMockModelReconciler(cl)

	reconcileModel(t, r, "m1")
	cleanups := listOpsJobs(t, cl, v1.OpsJobModelCleanupType)
	require.Len(t, cleanups, 1)
	assert.Equal(t, "ws2", cleanups[0].GetParameter(v1.ParameterWorkspace).Value)
	assert.Equal(t, "c1", v1.GetClusterId(&cleanups[0]))
	assert.True(t, controllerutil.ContainsFinalizer(getModel(t, cl, "m1"), ModelFinalizer))
}

// TestModelDeleteCleanupKeepsFailing: failures back off, and from the threshold on the
// model says that it needs an administrator and how to release it. The abandon
// annotation releases it and leaves the files.
func TestModelDeleteCleanupKeepsFailing(t *testing.T) {
	model := deletingModel(t, "m1", v1.ModelLocalPath{Workspace: "ws1", Path: lifecyclePath})
	cl := lifecycleClient(t, model, lifecycleWorkspace("ws1", "", lifecycleRoot)) // no cluster: cannot start
	r := newMockModelReconciler(cl)
	events := record.NewFakeRecorder(16)
	r.recorder = events

	var delays []time.Duration
	for i := 1; i <= cleanupFailureAlertThreshold; i++ {
		res, err := r.Reconcile(context.Background(), reconcileReq("m1"))
		require.NoError(t, err)
		delays = append(delays, res.RequeueAfter)
		m := getModel(t, cl, "m1")
		require.Equal(t, int32(i), m.Status.LocalPaths[0].CleanupFailures)
		// Reconciling again before the backoff has passed does not count a failure.
		reconcileModel(t, r, "m1")
		require.Equal(t, int32(i), getModel(t, cl, "m1").Status.LocalPaths[0].CleanupFailures)
		ageCleanupFailure(t, cl, "m1", cleanupMaxRetryInterval)
	}
	assert.Equal(t, []time.Duration{30 * time.Second, time.Minute, 2 * time.Minute, 4 * time.Minute, 8 * time.Minute}, delays)
	m := getModel(t, cl, "m1")
	assert.Contains(t, m.Status.Message, "needs an administrator")
	assert.Contains(t, m.Status.Message, AbandonCleanupAnnotation+"=true")
	assert.True(t, controllerutil.ContainsFinalizer(m, ModelFinalizer))
	require.Len(t, events.Events, 1)
	assert.Contains(t, <-events.Events, "CleanupStuck")

	// The backoff is capped.
	for i := 0; i < 3; i++ {
		res, err := r.Reconcile(context.Background(), reconcileReq("m1"))
		require.NoError(t, err)
		assert.LessOrEqual(t, res.RequeueAfter, cleanupMaxRetryInterval)
		ageCleanupFailure(t, cl, "m1", cleanupMaxRetryInterval)
	}

	m = getModel(t, cl, "m1")
	m.Annotations[AbandonCleanupAnnotation] = v1.TrueStr
	require.NoError(t, cl.Update(context.Background(), m))
	reconcileModel(t, r, "m1")
	err := cl.Get(context.Background(), client.ObjectKey{Name: "m1"}, &v1.Model{})
	assert.True(t, errors.IsNotFound(err), "the abandon annotation releases the model, got %v", err)
	assert.Empty(t, listOpsJobs(t, cl, v1.OpsJobModelCleanupType))
	require.Len(t, events.Events, 1)
	assert.Contains(t, <-events.Events, "CleanupAbandoned")
}

// TestModelPublicSkipsHeldPath: a public model gives up only the directory another
// live model holds; its other directories are downloaded. The same mount path on
// another cluster is another directory.
func TestModelPublicSkipsHeldPath(t *testing.T) {
	setViper(t, map[string]any{"s3.enable": false})
	model := lifecycleModel("m1")
	model.Spec.Workspace = ""
	model.Finalizers = []string{ModelFinalizer}
	model.Status.Phase = v1.ModelPhasePending
	holder := lifecycleModel("m2")
	holder.Status.Phase = v1.ModelPhaseReady
	holder.Status.LocalPaths = []v1.ModelLocalPath{{Workspace: "ws-a", Path: lifecyclePath, Status: v1.LocalPathStatusReady}}
	cl := lifecycleClient(t, model, holder,
		lifecycleWorkspace("ws-a", "a", lifecycleRoot),
		lifecycleWorkspace("ws-b", "b", lifecycleRoot))
	r := newMockModelReconciler(cl)

	_, err := r.handlePending(context.Background(), getModel(t, cl, "m1"))
	require.NoError(t, err)
	m := getModel(t, cl, "m1")
	assert.Equal(t, v1.ModelPhaseDownloading, m.Status.Phase)
	require.Len(t, m.Status.LocalPaths, 2)
	byWorkspace := map[string]v1.ModelLocalPath{}
	for _, lp := range m.Status.LocalPaths {
		byWorkspace[lp.Workspace] = lp
	}
	assert.Equal(t, v1.LocalPathStatusFailed, byWorkspace["ws-a"].Status)
	assert.Contains(t, byWorkspace["ws-a"].Message, "already used by model m2")
	assert.Equal(t, v1.LocalPathStatusPending, byWorkspace["ws-b"].Status, "the other cluster is downloaded")

	reconcileModel(t, r, "m1")
	downloads := listOpsJobs(t, cl, v1.OpsJobDownloadType)
	require.Len(t, downloads, 1)
	assert.Equal(t, "ws-b", v1.GetWorkspaceId(&downloads[0]))
}

// TestModelPrivateHeldPathFails: a private model whose only directory is held fails
// with the owner named.
func TestModelPrivateHeldPathFails(t *testing.T) {
	setViper(t, map[string]any{"s3.enable": false})
	model := lifecycleModel("m1")
	model.Status.Phase = v1.ModelPhasePending
	holder := lifecycleModel("m2")
	holder.Spec.Workspace = "ws2"
	holder.Status.LocalPaths = []v1.ModelLocalPath{{Workspace: "ws2", Path: lifecyclePath, Status: v1.LocalPathStatusReady}}
	cl := lifecycleClient(t, model, holder,
		lifecycleWorkspace("ws1", "c1", lifecycleRoot),
		lifecycleWorkspace("ws2", "c1", lifecycleRoot))
	r := newMockModelReconciler(cl)

	_, err := r.handlePending(context.Background(), getModel(t, cl, "m1"))
	require.NoError(t, err)
	m := getModel(t, cl, "m1")
	assert.Equal(t, v1.ModelPhaseFailed, m.Status.Phase)
	assert.Contains(t, m.Status.Message, lifecyclePath+" is already used by model m2")
	assert.Empty(t, listOpsJobs(t, cl, v1.OpsJobDownloadType))
}

// TestModelPendingGivesUpOnStuckDeletion: a deletion that has not freed the directory
// after deletingPathWaitTimeout no longer holds the new model; the directory fails
// with the reason and what to do.
func TestModelPendingGivesUpOnStuckDeletion(t *testing.T) {
	setViper(t, map[string]any{"s3.enable": false})
	old := deletingModel(t, "old", v1.ModelLocalPath{Workspace: "ws1", Path: lifecyclePath})
	old.DeletionTimestamp = &metav1.Time{Time: time.Now().Add(-deletingPathWaitTimeout - time.Minute)}
	model := lifecycleModel("m1")
	model.Status.Phase = v1.ModelPhasePending
	cl := lifecycleClient(t, old, model, lifecycleWorkspace("ws1", "c1", lifecycleRoot))
	r := newMockModelReconciler(cl)

	res, err := r.handlePending(context.Background(), getModel(t, cl, "m1"))
	require.NoError(t, err)
	assert.Zero(t, res.RequeueAfter)
	m := getModel(t, cl, "m1")
	assert.Equal(t, v1.ModelPhaseFailed, m.Status.Phase)
	assert.Contains(t, m.Status.Message, "still being cleaned up by the deletion of model old")
	assert.Contains(t, m.Status.Message, "retry this model")
}

// TestModelDeleteWaitsForFailedOverDownload: after a failover the download workload of
// the earlier workspace may still be terminating; no cleanup runs until it is gone.
func TestModelDeleteWaitsForFailedOverDownload(t *testing.T) {
	model := deletingModel(t, "m1", v1.ModelLocalPath{Workspace: "ws2", Path: lifecyclePath, Status: v1.LocalPathStatusFailed})
	downloadLabels := func(modelId string) map[string]string {
		l := map[string]string{v1.OpsJobTypeLabel: string(v1.OpsJobDownloadType)}
		if modelId != "" {
			l[v1.ModelIdLabel] = modelId
		}
		return l
	}
	now := metav1.Now()
	terminating := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: downloadJobName(model, "ws1"),
		Labels: downloadLabels("m1"), DeletionTimestamp: &now, Finalizers: []string{"test"}},
		Spec: v1.WorkloadSpec{Workspace: "ws1"}}
	unrelated := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "download--other-ws1", Labels: downloadLabels("")}}
	cl := lifecycleClient(t, model, terminating, unrelated,
		lifecycleWorkspace("ws1", "c1", lifecycleRoot),
		lifecycleWorkspace("ws2", "c1", lifecycleRoot))
	r := newMockModelReconciler(cl)

	reconcileModel(t, r, "m1")
	reconcileModel(t, r, "m1")
	assert.Empty(t, listOpsJobs(t, cl, v1.OpsJobModelCleanupType), "no cleanup while the earlier workspace's download terminates")
	assert.Equal(t, "Waiting for the model's downloads to stop before removing its files", getModel(t, cl, "m1").Status.Message)

	// A deletion stuck on it for long says what an administrator can do.
	stale := getModel(t, cl, "m1")
	stale.DeletionTimestamp = &metav1.Time{Time: time.Now().Add(-deletingPathWaitTimeout - time.Minute)}
	_, err := r.handleDelete(context.Background(), stale)
	require.NoError(t, err)
	assert.Contains(t, getModel(t, cl, "m1").Status.Message, AbandonCleanupAnnotation+"=true")

	// It terminates: the cleanup starts. The unrelated workload never held it up.
	wl := &v1.Workload{}
	require.NoError(t, cl.Get(context.Background(), client.ObjectKey{Name: terminating.Name}, wl))
	wl.Finalizers = nil
	require.NoError(t, cl.Update(context.Background(), wl))
	reconcileModel(t, r, "m1")
	assert.Len(t, listOpsJobs(t, cl, v1.OpsJobModelCleanupType), 1)
	require.NoError(t, cl.Get(context.Background(), client.ObjectKey{Name: unrelated.Name}, &v1.Workload{}))
}

// TestModelStopDownloadsAcrossWorkspaces: a live labelled download workload in another
// workspace is deleted; an unlabelled one matched by name only (created before the
// label existed, or another model's) is waited for but never deleted.
func TestModelStopDownloadsAcrossWorkspaces(t *testing.T) {
	model := deletingModel(t, "m1", v1.ModelLocalPath{Workspace: "ws2", Path: lifecyclePath})
	labelled := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "download-elsewhere", Labels: map[string]string{
		v1.OpsJobTypeLabel: string(v1.OpsJobDownloadType), v1.ModelIdLabel: "m1"}}}
	legacy := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: legacyDownloadJobName(model, "ws1"), Labels: map[string]string{
		v1.OpsJobTypeLabel: string(v1.OpsJobDownloadType)}}}
	cl := lifecycleClient(t, model, labelled, legacy,
		lifecycleWorkspace("ws1", "c1", lifecycleRoot),
		lifecycleWorkspace("ws2", "c1", lifecycleRoot))
	r := newMockModelReconciler(cl)

	stopped, err := r.stopDownloads(context.Background(), getModel(t, cl, "m1"))
	require.NoError(t, err)
	assert.False(t, stopped)
	err = cl.Get(context.Background(), client.ObjectKey{Name: labelled.Name}, &v1.Workload{})
	assert.True(t, errors.IsNotFound(err), "the model's own workload is stopped")
	require.NoError(t, cl.Get(context.Background(), client.ObjectKey{Name: legacy.Name}, &v1.Workload{}),
		"a workload matched by name only is not deleted")
	stopped, err = r.stopDownloads(context.Background(), getModel(t, cl, "m1"))
	require.NoError(t, err)
	assert.False(t, stopped, "the unlabelled workload of the earlier workspace is waited for")

	require.NoError(t, cl.Delete(context.Background(), legacy))
	stopped, err = r.stopDownloads(context.Background(), getModel(t, cl, "m1"))
	require.NoError(t, err)
	assert.True(t, stopped)
}

// TestModelReadyDrivesWaitingPaths: a model is Ready once one directory is; a directory
// that was still waiting for a download slot then gets its download when a slot frees.
func TestModelReadyDrivesWaitingPaths(t *testing.T) {
	setViper(t, map[string]any{"s3.enable": false, "model.max_concurrent_downloads": 1})
	model := lifecycleModel("m1")
	model.Spec.Workspace = ""
	model.Finalizers = []string{ModelFinalizer}
	model.Status.Phase = v1.ModelPhaseDownloading
	model.Status.LocalPaths = []v1.ModelLocalPath{
		{Workspace: "ws-a", Path: lifecyclePath, Status: v1.LocalPathStatusPending},
		{Workspace: "ws-b", Path: lifecyclePath, Status: v1.LocalPathStatusPending},
	}
	cl := lifecycleClient(t, model, lifecycleWorkspace("ws-a", "a", lifecycleRoot), lifecycleWorkspace("ws-b", "b", lifecycleRoot))
	r := newMockModelReconciler(cl)
	r.apiReader = cl
	ctx := context.Background()

	reconcileModel(t, r, "m1")
	jobs := listOpsJobs(t, cl, v1.OpsJobDownloadType)
	require.Len(t, jobs, 1, "one slot: ws-b waits")

	// Another model's download takes the slot as ws-a's finishes.
	other := &v1.OpsJob{ObjectMeta: metav1.ObjectMeta{Name: "download--other-ws1", Labels: map[string]string{
		v1.ModelIdLabel: "other", v1.OpsJobTypeLabel: string(v1.OpsJobDownloadType)}},
		Spec: v1.OpsJobSpec{Type: v1.OpsJobDownloadType}}
	require.NoError(t, cl.Create(ctx, other))
	setOpsJobPhase(t, cl, other.Name, v1.OpsJobRunning)
	setOpsJobPhase(t, cl, jobs[0].Name, v1.OpsJobSucceeded)
	res, err := r.Reconcile(ctx, reconcileReq("m1"))
	require.NoError(t, err)
	m := getModel(t, cl, "m1")
	require.Equal(t, v1.ModelPhaseReady, m.Status.Phase)
	require.Equal(t, v1.LocalPathStatusPending, m.Status.LocalPaths[1].Status)
	assert.Equal(t, downloadSlotWaitInterval, res.RequeueAfter, "a Ready model with a waiting directory is requeued")

	// The slot frees: the Ready model's waiting directory is downloaded.
	setOpsJobPhase(t, cl, other.Name, v1.OpsJobSucceeded)
	reconcileModel(t, r, "m1")
	var mine []v1.OpsJob
	for _, j := range listOpsJobs(t, cl, v1.OpsJobDownloadType) {
		if j.Labels[v1.ModelIdLabel] == "m1" {
			mine = append(mine, j)
		}
	}
	require.Len(t, mine, 1, "ws-b's download starts once a slot is free")
	assert.Equal(t, "ws-b", v1.GetWorkspaceId(&mine[0]))
	assert.Equal(t, v1.LocalPathStatusDownloading, getModel(t, cl, "m1").Status.LocalPaths[1].Status)

	setOpsJobPhase(t, cl, mine[0].Name, v1.OpsJobSucceeded)
	reconcileModel(t, r, "m1")
	m = getModel(t, cl, "m1")
	assert.Equal(t, v1.LocalPathStatusReady, m.Status.LocalPaths[1].Status)
	assert.Equal(t, "Model is ready in 2 workspaces", m.Status.Message)

	// A fully Ready model is left alone.
	res, err = r.Reconcile(ctx, reconcileReq("m1"))
	require.NoError(t, err)
	assert.Zero(t, res.RequeueAfter)
}

// TestModelDeleteSamePathOtherCluster: the same mount path on another cluster is
// another directory; it does not keep this model's directory on disk.
func TestModelDeleteSamePathOtherCluster(t *testing.T) {
	model := deletingModel(t, "m1", v1.ModelLocalPath{Workspace: "ws1", Path: lifecyclePath, Status: v1.LocalPathStatusReady})
	other := lifecycleModel("m2")
	other.Spec.Workspace = "ws2"
	other.Status.Phase = v1.ModelPhaseReady
	other.Status.LocalPaths = []v1.ModelLocalPath{{Workspace: "ws2", Path: lifecyclePath, Status: v1.LocalPathStatusReady}}
	cl := lifecycleClient(t, model, other, lifecycleWorkspace("ws1", "c1", lifecycleRoot), lifecycleWorkspace("ws2", "c2", lifecycleRoot))
	r := newMockModelReconciler(cl)

	reconcileModel(t, r, "m1")
	cleanups := listOpsJobs(t, cl, v1.OpsJobModelCleanupType)
	require.Len(t, cleanups, 1, "the directory on c1 is removed")
	assert.Equal(t, "ws1", cleanups[0].GetParameter(v1.ParameterWorkspace).Value)
	assert.True(t, controllerutil.ContainsFinalizer(getModel(t, cl, "m1"), ModelFinalizer))
}

// TestModelDeleteIgnoresFailedEntry: a Failed entry of another model (here the one
// saying the directory is already used by the model being deleted) does not hold the
// directory, which is removed.
func TestModelDeleteIgnoresFailedEntry(t *testing.T) {
	model := deletingModel(t, "m2", v1.ModelLocalPath{Workspace: "ws1", Path: lifecyclePath, Status: v1.LocalPathStatusReady})
	other := lifecycleModel("m1")
	other.Spec.Workspace = ""
	other.Status.Phase = v1.ModelPhaseReady
	other.Status.LocalPaths = []v1.ModelLocalPath{
		{Workspace: "ws1", Path: lifecyclePath, Status: v1.LocalPathStatusFailed, Message: lifecyclePath + " is already used by model m2"},
		{Workspace: "ws2", Path: lifecyclePath, Status: v1.LocalPathStatusReady},
	}
	cl := lifecycleClient(t, model, other, lifecycleWorkspace("ws1", "c1", lifecycleRoot), lifecycleWorkspace("ws2", "c2", lifecycleRoot))
	r := newMockModelReconciler(cl)

	reconcileModel(t, r, "m2")
	require.Len(t, listOpsJobs(t, cl, v1.OpsJobModelCleanupType), 1, "m2's directory is removed")
	assert.True(t, controllerutil.ContainsFinalizer(getModel(t, cl, "m2"), ModelFinalizer))
}

// TestModelPendingIgnoresLiveFailedEntry: a live model's Failed entry does not make the
// directory taken; a deleting model's Failed entry is still waited for, its cleanup
// removes that directory.
func TestModelPendingIgnoresLiveFailedEntry(t *testing.T) {
	setViper(t, map[string]any{"s3.enable": false})
	failed := []v1.ModelLocalPath{{Workspace: "ws1", Path: lifecyclePath, Status: v1.LocalPathStatusFailed, Message: "download failed"}}

	live := lifecycleModel("old")
	live.Status.Phase = v1.ModelPhaseFailed
	live.Status.LocalPaths = failed
	model := lifecycleModel("m1")
	model.Status.Phase = v1.ModelPhasePending
	cl := lifecycleClient(t, live, model, lifecycleWorkspace("ws1", "c1", lifecycleRoot))
	r := newMockModelReconciler(cl)
	paths, waitFor, err := r.planTargetPaths(context.Background(), getModel(t, cl, "m1"))
	require.NoError(t, err)
	assert.Zero(t, waitFor)
	require.Len(t, paths, 1)
	assert.Equal(t, v1.LocalPathStatusPending, paths[0].Status, paths[0].Message)

	deleting := deletingModel(t, "old", failed...)
	cl = lifecycleClient(t, deleting, model, lifecycleWorkspace("ws1", "c1", lifecycleRoot))
	r = newMockModelReconciler(cl)
	_, waitFor, err = r.planTargetPaths(context.Background(), getModel(t, cl, "m1"))
	require.NoError(t, err)
	assert.Positive(t, waitFor, "the deleting model's cleanup still removes the directory")
}

// TestModelDeleteS3SharedPath: the S3 path is derived from the display name, so two
// models can share it; the copy another live model uses is left in place.
func TestModelDeleteS3SharedPath(t *testing.T) {
	patchS3Config(t)
	model := deletingModel(t, "m1")
	model.Status.S3Path = "models/org-repo"
	other := lifecycleModel("m2")
	other.Status.Phase = v1.ModelPhaseReady
	other.Status.S3Path = "models/org-repo"
	cl := lifecycleClient(t, model, other)
	r := newMockModelReconciler(cl)
	ctx := context.Background()

	reconcileModel(t, r, "m1")
	err := cl.Get(ctx, client.ObjectKey{Name: "cleanup-m1", Namespace: common.PrimusSafeNamespace}, &batchv1.Job{})
	assert.True(t, errors.IsNotFound(err), "no S3 cleanup of a copy m2 uses")
	reconcileModel(t, r, "m1")
	err = cl.Get(ctx, client.ObjectKey{Name: "m1"}, &v1.Model{})
	assert.True(t, errors.IsNotFound(err), "the model is released, got %v", err)

	// Not shared: the cleanup runs, and only below the model's own prefix.
	model = deletingModel(t, "m1")
	model.Status.S3Path = "models/org-repo"
	other = lifecycleModel("m2")
	other.Spec.DisplayName = "org/repo-instruct"
	other.Status.S3Path = "models/org-repo-instruct"
	cl = lifecycleClient(t, model, other)
	r = newMockModelReconciler(cl)
	reconcileModel(t, r, "m1")
	job := &batchv1.Job{}
	require.NoError(t, cl.Get(ctx, client.ObjectKey{Name: "cleanup-m1", Namespace: common.PrimusSafeNamespace}, job))
	var s3Path string
	for _, env := range job.Spec.Template.Spec.Containers[0].Env {
		if env.Name == "S3_PATH" {
			s3Path = env.Value
		}
	}
	assert.True(t, strings.HasSuffix(s3Path, "/models/org-repo/"), "aws s3 rm --recursive matches key prefixes: %q", s3Path)
}

// TestModelDeleteS3PathNotHeldWithoutUpload: a live model that never uploaded to S3 (its
// status records no S3 path) does not hold the S3 copy of a model being deleted, even
// when both derive the same path from one HuggingFace repository. Were it taken as the
// holder, the deleted model would skip its cleanup and forget its path, and since the
// other model has no path to clean either, models/<name> would stay in the bucket.
func TestModelDeleteS3PathNotHeldWithoutUpload(t *testing.T) {
	patchS3Config(t)
	model := deletingModel(t, "m1")
	model.Status.S3Path = "models/org-repo"
	other := lifecycleModel("m2")
	other.Status.Phase = v1.ModelPhaseFailed
	require.Equal(t, "models/org-repo", other.GetS3Path(), "both models derive the same S3 path")
	cl := lifecycleClient(t, model, other)
	r := newMockModelReconciler(cl)
	ctx := context.Background()

	reconcileModel(t, r, "m1")
	job := &batchv1.Job{}
	require.NoError(t, cl.Get(ctx, client.ObjectKey{Name: "cleanup-m1", Namespace: common.PrimusSafeNamespace}, job),
		"the S3 copy is removed: m2 never uploaded and holds nothing")
	m := getModel(t, cl, "m1")
	assert.Equal(t, "models/org-repo", m.Status.S3Path, "the path is kept until its cleanup succeeds")
	assert.True(t, controllerutil.ContainsFinalizer(m, ModelFinalizer))
}

// TestModelUnsafeDirNameFails: an S3 import is stored under its display name; one that is
// not a safe directory name ("..") would put the files at the volume root, which no
// cleanup removes. The model fails before any directory is planned or written.
func TestModelUnsafeDirNameFails(t *testing.T) {
	setViper(t, map[string]any{"s3.enable": false})
	model := lifecycleModel("m1")
	model.Labels[v1.ModelS3ImportLabel] = v1.TrueStr
	model.Spec.DisplayName = ".."
	model.Spec.TargetSubpath = ""
	model.Spec.Source.URL = "s3://bucket/prefix"
	cl := lifecycleClient(t, model, lifecycleWorkspace("ws1", "c1", lifecycleRoot))
	r := newMockModelReconciler(cl)

	reconcileModel(t, r, "m1") // finalizer
	reconcileModel(t, r, "m1") // Pending
	reconcileModel(t, r, "m1")
	m := getModel(t, cl, "m1")
	assert.Equal(t, v1.ModelPhaseFailed, m.Status.Phase)
	assert.Contains(t, m.Status.Message, "cannot be used as the model directory name")
	assert.Empty(t, m.Status.LocalPaths)
	reconcileModel(t, r, "m1")
	assert.Empty(t, listOpsJobs(t, cl, v1.OpsJobDownloadType), "nothing is downloaded")

	// The same model with a usable name is planned below the models directory.
	model = lifecycleModel("m2")
	model.Labels[v1.ModelS3ImportLabel] = v1.TrueStr
	model.Spec.DisplayName = "my model"
	model.Spec.TargetSubpath = ""
	model.Spec.Source.URL = "s3://bucket/prefix"
	cl = lifecycleClient(t, model, lifecycleWorkspace("ws1", "c1", lifecycleRoot))
	r = newMockModelReconciler(cl)
	for i := 0; i < 3; i++ {
		reconcileModel(t, r, "m2")
	}
	m = getModel(t, cl, "m2")
	require.Len(t, m.Status.LocalPaths, 1, m.Status.Message)
	assert.Equal(t, lifecycleRoot+"/models/my-model", m.Status.LocalPaths[0].Path)
}

// TestModelDeleteCleansVolumeAtRoot: a workspace volume mounted at "/" holds its models
// at /models/<dir>. The controller accepts that path, and the cleanup job it creates
// is accepted by the script it runs, so the model is released once the files are gone.
func TestModelDeleteCleansVolumeAtRoot(t *testing.T) {
	model := deletingModel(t, "m1", v1.ModelLocalPath{Workspace: "ws1", Path: "/models/org--repo", Status: v1.LocalPathStatusReady})
	cl := lifecycleClient(t, model, lifecycleWorkspace("ws1", "c1", "/"))
	r := newMockModelReconciler(cl)

	reconcileModel(t, r, "m1")
	cleanups := listOpsJobs(t, cl, v1.OpsJobModelCleanupType)
	require.Len(t, cleanups, 1, getModel(t, cl, "m1").Status.Message)
	job := cleanups[0]
	assert.Equal(t, "/", job.Spec.Env["VOLUME_ROOT"])

	// What the workload runs: the job's own script with the job's environment.
	script, err := base64.StdEncoding.DecodeString(*job.Spec.EntryPoint)
	require.NoError(t, err)
	log := filepath.Join(t.TempDir(), "rm.log")
	cmd := exec.Command("sh", "-c", `rm() { echo "rm $*" >> "$RM_LOG"; }`+"\n"+string(script))
	cmd.Env = append(os.Environ(), "DEST_PATH="+job.GetParameter(v1.ParameterDestPath).Value,
		"VOLUME_ROOT="+job.Spec.Env["VOLUME_ROOT"], "RM_LOG="+log)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "the script refuses what the controller accepted: %s", out)
	removed, err := os.ReadFile(log)
	require.NoError(t, err)
	assert.Equal(t, "rm -rf -- /models/org--repo\n", string(removed))
}

// TestModelDownloadCreateRetriesPassingRefusal: the OpsJob webhook refuses a download
// while a cleanup of the same directory still runs (ResourceProcessing, a 409). That
// passes, so the directory waits and the download starts on a later pass; a refusal
// that will never pass (an invalid job) fails the directory.
func TestModelDownloadCreateRetriesPassingRefusal(t *testing.T) {
	setViper(t, map[string]any{"s3.enable": false})
	model := lifecycleModel("m1")
	model.Finalizers = []string{ModelFinalizer}
	model.Status.Phase = v1.ModelPhaseDownloading
	model.Status.LocalPaths = []v1.ModelLocalPath{{Workspace: "ws1", Path: lifecyclePath, Status: v1.LocalPathStatusPending}}
	var refuse error
	cl := fake.NewClientBuilder().WithScheme(lifecycleScheme(t)).
		WithStatusSubresource(&v1.Model{}, &v1.OpsJob{}).
		WithObjects(model, lifecycleWorkspace("ws1", "c1", lifecycleRoot)).
		WithInterceptorFuncs(interceptor.Funcs{
			Create: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
				if _, ok := obj.(*v1.OpsJob); ok && refuse != nil {
					return refuse
				}
				return c.Create(ctx, obj, opts...)
			},
		}).Build()
	r := newMockModelReconciler(cl)

	for _, passing := range []error{
		commonerrors.NewResourceProcessing("another ops job (cleanup-m0-1234) with the same input is processing"),
		errors.NewServiceUnavailable("etcd leader changed"),
		errors.NewTimeoutError("create", 1),
		stderrors.New("connection reset by peer"),
	} {
		refuse = passing
		_, err := r.Reconcile(context.Background(), reconcileReq("m1"))
		require.Error(t, err, "the refusal is retried with backoff: %v", passing)
		m := getModel(t, cl, "m1")
		assert.Equal(t, v1.ModelPhaseDownloading, m.Status.Phase)
		assert.Equal(t, v1.LocalPathStatusPending, m.Status.LocalPaths[0].Status, "%v", passing)
		assert.Contains(t, m.Status.LocalPaths[0].Message, "retrying")
	}

	refuse = nil
	reconcileModel(t, r, "m1")
	m := getModel(t, cl, "m1")
	assert.Equal(t, v1.LocalPathStatusDownloading, m.Status.LocalPaths[0].Status, m.Status.LocalPaths[0].Message)
	require.Len(t, listOpsJobs(t, cl, v1.OpsJobDownloadType), 1)

	// A refusal that does not pass fails the directory.
	model2 := model.DeepCopy()
	model2.ResourceVersion = ""
	model2.Name = "m2"
	require.NoError(t, cl.Create(context.Background(), model2))
	refuse = commonerrors.NewBadRequest("DEST_PATH must be an absolute path")
	_, err := r.Reconcile(context.Background(), reconcileReq("m2"))
	require.NoError(t, err)
	m = getModel(t, cl, "m2")
	assert.Equal(t, v1.LocalPathStatusFailed, m.Status.LocalPaths[0].Status)
	assert.Equal(t, v1.ModelPhaseFailed, m.Status.Phase)
}

// TestModelDownloadNoLimit: max_concurrent_downloads of 0 or below means no limit, so a
// download starts however many others run.
func TestModelDownloadNoLimit(t *testing.T) {
	for _, limit := range []int{0, -1} {
		setViper(t, map[string]any{"s3.enable": false, "model.max_concurrent_downloads": limit})
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

		_, err := r.handleDownloading(context.Background(), getModel(t, cl, "m1"))
		require.NoError(t, err)
		assert.Equal(t, v1.LocalPathStatusDownloading, getModel(t, cl, "m1").Status.LocalPaths[0].Status, "limit %d", limit)
		assert.Len(t, listOpsJobs(t, cl, v1.OpsJobDownloadType), 2, "limit %d", limit)
	}
}

// TestModelFailoverTriedIsSaved: the workspaces a failover has tried are saved on the
// model, so a download that fails in every workspace of the volume stops after each was
// tried once instead of moving between them for ever.
func TestModelFailoverTriedIsSaved(t *testing.T) {
	setViper(t, map[string]any{"s3.enable": false})
	model := lifecycleModel("m1")
	model.Finalizers = []string{ModelFinalizer}
	model.Status.Phase = v1.ModelPhaseDownloading
	model.Status.LocalPaths = []v1.ModelLocalPath{{Workspace: "ws1", Path: lifecyclePath, Status: v1.LocalPathStatusPending}}
	cl := lifecycleClient(t, model,
		lifecycleWorkspace("ws1", "c1", lifecycleRoot), lifecycleWorkspace("ws2", "c1", lifecycleRoot))
	r := newMockModelReconciler(cl)

	fail := func() {
		t.Helper()
		reconcileModel(t, r, "m1") // download job
		jobs := listOpsJobs(t, cl, v1.OpsJobDownloadType)
		require.Len(t, jobs, 1)
		setOpsJobPhase(t, cl, jobs[0].Name, v1.OpsJobFailed)
		reconcileModel(t, r, "m1") // failed: fail over, or give up
	}
	fail()
	m := getModel(t, cl, "m1")
	assert.Equal(t, "ws2", m.Status.LocalPaths[0].Workspace)
	assert.Equal(t, []string{"ws1"}, r.getTriedWorkspaces(m, lifecycleRoot), "the tried workspace is saved")

	fail()
	m = getModel(t, cl, "m1")
	assert.Equal(t, v1.LocalPathStatusFailed, m.Status.LocalPaths[0].Status, "ws1 was tried already: no way back")
	assert.Equal(t, v1.ModelPhaseFailed, m.Status.Phase)
	assert.Equal(t, []string{"ws1", "ws2"}, r.getTriedWorkspaces(m, lifecycleRoot))
}

// TestModelJobNamesStayDistinctForLongNames: a long model name (common for HuggingFace
// repositories) does not push the part that tells two workspaces or two directories
// apart out of the job name.
func TestModelJobNamesStayDistinctForLongNames(t *testing.T) {
	model := &v1.Model{ObjectMeta: metav1.ObjectMeta{Name: "meta-llama-llama-3-1-405b-instruct-fp8-dynamic-abcdef"}}
	ws1 := downloadJobName(model, "workspace-one")
	ws2 := downloadJobName(model, "workspace-two")
	assert.NotEqual(t, ws1, ws2)
	assert.Equal(t, ws1, downloadJobName(model, "workspace-one"), "the name is stable")
	c1 := cleanupJobName(model, "ws1", "/mnt/a/models/org--repo")
	c2 := cleanupJobName(model, "ws1", "/mnt/b/models/org--repo")
	assert.NotEqual(t, c1, c2)
	other := &v1.Model{ObjectMeta: metav1.ObjectMeta{Name: "meta-llama-llama-3-1-405b-instruct-fp8-dynamic-zzzzzz"}}
	assert.NotEqual(t, c1, cleanupJobName(other, "ws1", "/mnt/a/models/org--repo"), "another model's cleanup of the same directory")
	for _, n := range []string{ws1, ws2, c1, c2} {
		assert.LessOrEqual(t, len(n), 45, n)
		assert.Equal(t, stringutil.NormalizeForDNS(n), n, "a valid name")
	}
	assert.True(t, strings.HasPrefix(ws1, "download-"), ws1)
	assert.True(t, strings.HasPrefix(c1, "cleanup-"), c1)
}

// TestModelDownloadSavesFinishedJobOnLaterError: a finished download job is deleted in
// the pass that sees it finish, so its result lives only in the status. An error later
// in the same pass (counting the running downloads, pruning the token copies) must not
// lose that status, or the next pass finds no job and downloads the model again.
func TestModelDownloadSavesFinishedJobOnLaterError(t *testing.T) {
	ctx := context.Background()
	for _, step := range []string{"count", "prune"} {
		setViper(t, map[string]any{"s3.enable": false, "model.max_concurrent_downloads": 1})
		model := lifecycleModel("m1")
		model.Finalizers = []string{ModelFinalizer}
		model.Spec.Source.Token = &corev1.LocalObjectReference{Name: "m1-token"}
		model.Status.Phase = v1.ModelPhaseDownloading
		model.Status.LocalPaths = []v1.ModelLocalPath{{Workspace: "ws1", Path: lifecyclePath, Status: v1.LocalPathStatusPending}}
		if step == "count" {
			// A second directory waits for a slot, so the pass that sees ws1 finish
			// counts the running downloads.
			model.Status.LocalPaths = append(model.Status.LocalPaths,
				v1.ModelLocalPath{Workspace: "ws2", Path: "/other/models/org--repo", Status: v1.LocalPathStatusPending})
		}
		owned := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "m1-token", Namespace: common.PrimusSafeNamespace,
				Labels: map[string]string{v1.ModelIdLabel: "m1"}},
			Data: map[string][]byte{"token": []byte("hf_secret")},
		}
		fail := false
		cl := fake.NewClientBuilder().WithScheme(lifecycleScheme(t)).
			WithStatusSubresource(&v1.Model{}, &v1.OpsJob{}).
			WithObjects(model, owned, lifecycleWorkspace("ws1", "c1", lifecycleRoot), lifecycleWorkspace("ws2", "c1", "/other")).
			WithInterceptorFuncs(interceptor.Funcs{
				List: func(ctx context.Context, c client.WithWatch, list client.ObjectList, opts ...client.ListOption) error {
					if fail {
						switch list.(type) {
						case *v1.OpsJobList:
							if step == "count" {
								return errors.NewServiceUnavailable("etcd leader changed")
							}
						case *corev1.SecretList:
							if step == "prune" {
								return errors.NewServiceUnavailable("etcd leader changed")
							}
						}
					}
					return c.List(ctx, list, opts...)
				},
			}).Build()
		r := newMockModelReconciler(cl)
		r.apiReader = cl

		reconcileModel(t, r, "m1") // ws1 starts; ws2, if any, waits for the slot
		downloads := listOpsJobs(t, cl, v1.OpsJobDownloadType)
		require.Len(t, downloads, 1, step)
		require.Equal(t, downloadJobName(model, "ws1"), downloads[0].Name, step)
		setOpsJobPhase(t, cl, downloads[0].Name, v1.OpsJobSucceeded)

		fail = true
		_, err := r.Reconcile(ctx, reconcileReq("m1"))
		require.Error(t, err, "%s: the error is retried with backoff", step)
		fail = false
		require.Empty(t, listOpsJobs(t, cl, v1.OpsJobDownloadType), "%s: the finished job is deleted", step)
		m := getModel(t, cl, "m1")
		assert.Equal(t, v1.LocalPathStatusReady, m.Status.LocalPaths[0].Status, "%s: %s", step, m.Status.LocalPaths[0].Message)
		assert.Equal(t, v1.ModelPhaseReady, m.Status.Phase, step)

		// The retry does not download ws1 again.
		_, _ = r.Reconcile(ctx, reconcileReq("m1"))
		for _, job := range listOpsJobs(t, cl, v1.OpsJobDownloadType) {
			assert.NotEqual(t, downloadJobName(model, "ws1"), job.Name, "%s: ws1 is downloaded again", step)
		}
		assert.Equal(t, v1.LocalPathStatusReady, getModel(t, cl, "m1").Status.LocalPaths[0].Status, step)
	}
}

// enablePostTraining turns the Post Training switch on for one test.
func enablePostTraining(t *testing.T) {
	t.Helper()
	setViper(t, map[string]any{"post_training.enable": true})
}

// The reported model config is parsed with its rope theta and dtype, next to the size line.
func TestReportedModelConfig(t *testing.T) {
	lines := `["[SUCCESS] MODEL_CONFIG={\"architectures\":[\"Qwen3ForCausalLM\"],\"model_type\":\"qwen3\",` +
		`\"num_hidden_layers\":36,\"num_key_value_heads\":8,\"rope_theta\":1000000.0,\"torch_dtype\":\"bfloat16\"}",` +
		`"[SUCCESS] MODEL_SIZE_BYTES=42"]`
	job := &v1.OpsJob{Status: v1.OpsJobStatus{Outputs: []v1.Parameter{{Name: "result", Value: lines}}}}
	cfg, err := reportedModelConfig(job)
	require.NoError(t, err)
	assert.Equal(t, []string{"Qwen3ForCausalLM"}, cfg.Architectures)
	assert.Equal(t, 36, cfg.NumHiddenLayers)
	assert.Equal(t, "1000000.0", cfg.RopeTheta)
	assert.Equal(t, "bfloat16", cfg.TorchDtype)
	assert.Equal(t, int64(42), reportedModelSize(job), "the size line next to it still parses")

	cfg, err = reportedModelConfig(&v1.OpsJob{Status: v1.OpsJobStatus{Outputs: []v1.Parameter{
		{Name: "result", Value: "[SUCCESS] MODEL_CONFIG={}"}}}})
	assert.Nil(t, cfg)
	assert.ErrorContains(t, err, "neither its model_type nor its architectures")
	_, err = reportedModelConfig(&v1.OpsJob{Status: v1.OpsJobStatus{Outputs: []v1.Parameter{
		{Name: "result", Value: "[SUCCESS] MODEL_SIZE_BYTES=42"}}}})
	assert.ErrorContains(t, err, "reported no MODEL_CONFIG line")
}

// A field of an unexpected type is left out without failing the others.
func TestParseModelConfigReadsEachFieldOnItsOwn(t *testing.T) {
	cfg, err := parseModelConfig([]byte(`{"model_type":"qwen3","rope_theta":"1e6","num_experts":[8],` +
		`"num_hidden_layers":36,"hidden_size":"big"}`))
	require.NoError(t, err)
	assert.Equal(t, "qwen3", cfg.ModelType)
	assert.Equal(t, "1e6", cfg.RopeTheta, "a rope_theta written as a string is kept")
	assert.Equal(t, 36, cfg.NumHiddenLayers)
	assert.Zero(t, cfg.NumExperts, "a field of an unexpected type is left out")
	assert.Zero(t, cfg.HiddenSize)

	cfg, err = parseModelConfig([]byte(`{"architectures":["X"],"rope_theta":"linear"}`))
	require.NoError(t, err)
	assert.Equal(t, "linear", cfg.RopeTheta)
	_, err = parseModelConfig([]byte(`[1]`))
	assert.ErrorContains(t, err, "not a JSON object")
}

// A Post Training output's weights stay while runs train from it, and go with it after. A run counts while its
// workload exists and has not ended, or its row changed a moment ago; a run left Running without a workload, or
// with one that ended, does not hold the model.
func TestPostTrainingOutputCleanupWaitsForBasedRuns(t *testing.T) {
	model := genMockModel("qwen-sft-out", v1.AccessModeLocalPath, "ws1")
	model.Spec.PostTrainingRunId = "pt-qwen-abc12"
	r := &ModelReconciler{}
	assert.True(t, r.needsCleanup(model), "a Post Training output is deleted with its weights")
	assert.False(t, r.needsCleanup(genMockModel("registered", v1.AccessModeLocalPath, "ws1")))

	enablePostTraining(t)
	now := metav1.Now()
	model.DeletionTimestamp = &now
	model.Finalizers = []string{ModelFinalizer}
	workload := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "pt-running"}}
	ended := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "pt-ended"}}
	ended.Status.Phase = v1.WorkloadSucceeded
	cli := lifecycleClient(t, model, workload, ended)
	db := mockclient.NewMockInterface(gomock.NewController(t))
	run := func(workload string, updated time.Time) *dbclient.PostTrainingRun {
		r := &dbclient.PostTrainingRun{RunID: workload, WorkloadID: workload}
		r.UpdatedAt.Time, r.UpdatedAt.Valid = updated, true
		return r
	}
	long := time.Now().Add(-time.Hour)
	db.EXPECT().ListPostTrainingRuns(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, f dbclient.PostTrainingRunFilter) ([]*dbclient.PostTrainingRun, int64, error) {
			assert.Equal(t, "qwen-sft-out", f.BaseModelID)
			return []*dbclient.PostTrainingRun{
				run("pt-running", long),         // its workload runs
				run("pt-submitted", time.Now()), // its workload is about to be created
				run("pt-orphaned", long),        // left Running after its workload went away
				run("pt-ended", long),           // left Running after its workload ended
			}, 4, nil
		})
	r = &ModelReconciler{ClusterBaseReconciler: &ClusterBaseReconciler{Client: cli}, db: db}
	ctx := context.Background()
	res, err := r.handleDelete(ctx, model)
	require.NoError(t, err)
	assert.Equal(t, baseInUseRequeue, res.RequeueAfter)
	got := getModel(t, cli, model.Name)
	assert.Contains(t, got.Finalizers, ModelFinalizer, "the weights stay while runs train from them")
	assert.Contains(t, got.Status.Message, "Waiting for 2 unfinished Post Training runs",
		"the runs whose workload is gone or ended do not count")
	assert.Contains(t, got.Status.Message, AbandonCleanupAnnotation, "the way out is named")

	// A database that cannot answer keeps the files.
	db.EXPECT().ListPostTrainingRuns(gomock.Any(), gomock.Any()).Return(nil, int64(0), stderrors.New("db down"))
	_, err = r.handleDelete(ctx, got)
	assert.ErrorContains(t, err, "db down")
	assert.Contains(t, getModel(t, cli, model.Name).Finalizers, ModelFinalizer)

	// With Post Training off the database is not asked, so the deletion does not depend on it.
	setViper(t, map[string]any{"post_training.enable": false})
	_, err = r.handleDelete(ctx, getModel(t, cli, model.Name))
	require.NoError(t, err)
	assert.True(t, errors.IsNotFound(cli.Get(ctx, client.ObjectKeyFromObject(model), &v1.Model{})),
		"the model is released")
}

// The download and inspect scripts are valid shell.
func TestModelScriptsAreValidShell(t *testing.T) {
	for name, script := range map[string]string{"download": hfDownloadScript, "inspect": modelInspectScript} {
		out, err := exec.Command("sh", "-n", "-c", script).CombinedOutput()
		require.NoError(t, err, "%s: %s", name, out)
	}
	assert.Contains(t, hfDownloadScript, modelConfigShell)
}

// The inspect script reports the model's config.json, also the fields a multimodal model keeps in the config of
// its language model, or an error when it cannot.
func TestModelInspectScript(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 is not installed")
	}
	dir := t.TempDir()
	run := func() (string, error) {
		cmd := exec.Command("sh", "-c", modelInspectScript)
		cmd.Env = append(os.Environ(), "DEST_PATH="+dir)
		out, err := cmd.CombinedOutput()
		return string(out), err
	}
	read := func(config string) *v1.ModelConfig {
		t.Helper()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "config.json"), []byte(config), 0o644))
		out, err := run()
		require.NoError(t, err, out)
		job := &v1.OpsJob{Status: v1.OpsJobStatus{Outputs: []v1.Parameter{{Name: "result",
			Value: commonopsjob.FilterResultLog([]byte(out))}}}}
		cfg, err := reportedModelConfig(job)
		require.NoError(t, err)
		return cfg
	}
	out, err := run()
	assert.Error(t, err)
	assert.Contains(t, out, "[ERROR] "+dir+"/config.json does not exist")

	cfg := read(`{"architectures":["Qwen3ForCausalLM"],"model_type":"qwen3","num_hidden_layers":36}`)
	assert.Equal(t, 36, cfg.NumHiddenLayers)

	cfg = read(`{"architectures":["InternVLChatModel"],"model_type":"internvl_chat",` +
		`"llm_config":{"num_hidden_layers":28,"hidden_size":3584,"rope_theta":1000000.0}}`)
	assert.Equal(t, "internvl_chat", cfg.ModelType)
	assert.Equal(t, 28, cfg.NumHiddenLayers, "read from llm_config")
	assert.Equal(t, "1000000.0", cfg.RopeTheta)
	cfg = read(`{"model_type":"llava","language_config":{"num_attention_heads":32}}`)
	assert.Equal(t, 32, cfg.NumAttentionHeads, "read from language_config")
	cfg = read(`{"model_type":"qwen3","rope_parameters":{"rope_theta":5000000,"rope_type":"default"}}`)
	assert.Equal(t, "5000000", cfg.RopeTheta, "read from rope_parameters")
	cfg = read(`{"model_type":"gemma3_text","rope_parameters":{"sliding_attention":{"rope_theta":10000},` +
		`"full_attention":{"rope_theta":1000000}}}`)
	assert.Equal(t, "1000000", cfg.RopeTheta, "the full attention's, per attention type")

	require.NoError(t, os.WriteFile(filepath.Join(dir, "config.json"), []byte(`{"num_hidden_layers":`), 0o644))
	out, err = run()
	assert.Error(t, err)
	assert.Contains(t, out, "is not a readable model config")
}

// inspectFixture is a Ready local_path model in workspace ws1 of cluster c1, with Post Training on.
func inspectFixture(t *testing.T, name string, intercept *interceptor.Funcs) (*ModelReconciler, client.Client, *v1.Model) {
	t.Helper()
	enablePostTraining(t)
	model := genMockModel(name, v1.AccessModeLocalPath, "ws1")
	model.Labels = map[string]string{v1.UserIdLabel: "alice"}
	model.Spec.Source.LocalPath = "/data/models/" + name
	model.Status.Phase = v1.ModelPhaseReady
	ws := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}, Spec: v1.WorkspaceSpec{Cluster: "c1"}}
	builder := fake.NewClientBuilder().WithScheme(lifecycleScheme(t)).WithObjects(model, ws).
		WithStatusSubresource(model)
	if intercept != nil {
		builder = builder.WithInterceptorFuncs(*intercept)
	}
	cli := builder.Build()
	r := &ModelReconciler{ClusterBaseReconciler: &ClusterBaseReconciler{Client: cli}, apiReader: cli,
		db: mockclient.NewMockInterface(gomock.NewController(t))}
	return r, cli, model
}

// failInspectJob marks the model's inspect job failed with message, after it ended long ago.
func failInspectJob(t *testing.T, cli client.Client, model *v1.Model, message string) {
	t.Helper()
	job := &v1.OpsJob{}
	require.NoError(t, cli.Get(context.Background(), client.ObjectKey{Name: inspectJobName(model)}, job))
	long := metav1.NewTime(time.Now().Add(-time.Hour))
	job.Status.Phase, job.Status.FinishedAt = v1.OpsJobFailed, &long
	job.Status.Conditions = []metav1.Condition{failedJobCondition(message)}
	require.NoError(t, cli.Update(context.Background(), job))
}

func inspectJobExists(t *testing.T, cli client.Client, model *v1.Model) bool {
	t.Helper()
	err := cli.Get(context.Background(), client.ObjectKey{Name: inspectJobName(model)}, &v1.OpsJob{})
	if errors.IsNotFound(err) {
		return false
	}
	require.NoError(t, err)
	return true
}

// A model's config is read once, a failed read is tried again, and a recorded failure is read again later.
func TestInspectModelReadsConfigOnce(t *testing.T) {
	r, cli, model := inspectFixture(t, "registered", nil)
	ctx := context.Background()

	_, err := r.inspectModel(ctx, model)
	require.NoError(t, err)
	job := &v1.OpsJob{}
	require.NoError(t, cli.Get(ctx, client.ObjectKey{Name: inspectJobName(model)}, job))
	assert.Equal(t, v1.OpsJobModelInspectType, job.Spec.Type)
	assert.Equal(t, "/data/models/registered", job.GetParameter(v1.ParameterDestPath).Value)
	assert.Equal(t, common.UserSystem, v1.GetUserId(job), "the owner did not start it and is not notified")
	assert.Equal(t, common.UserSystem, v1.GetUserName(job))

	job.Status.Phase = v1.OpsJobSucceeded
	job.Status.Outputs = []v1.Parameter{{Name: "result", Value: `["[SUCCESS] MODEL_CONFIG={\"model_type\":\"qwen3\"}"]`}}
	require.NoError(t, cli.Update(ctx, job))
	_, err = r.inspectModel(ctx, getModel(t, cli, model.Name))
	require.NoError(t, err)
	got := getModel(t, cli, model.Name)
	assert.Equal(t, "qwen3", got.Status.ModelConfig.ModelType)
	assert.Equal(t, inspectDone, got.Annotations[v1.ModelInspectAnnotation])
	assert.Empty(t, got.Annotations[inspectAttemptsAnnotation])
	assert.False(t, inspectJobExists(t, cli, model))

	failed := genMockModel("broken", v1.AccessModeLocalPath, "ws1")
	failed.Spec.Source.LocalPath = "/data/models/broken"
	require.NoError(t, cli.Create(ctx, failed))
	for attempt := 1; attempt <= inspectAttempts; attempt++ {
		failed = getModel(t, cli, failed.Name)
		_, err = r.inspectModel(ctx, failed)
		require.NoError(t, err)
		assert.True(t, inspectJobExists(t, cli, failed), "attempt %d", attempt)
		failInspectJob(t, cli, failed, "[ERROR] config.json does not exist\n")
		_, err = r.inspectModel(ctx, getModel(t, cli, failed.Name))
		require.NoError(t, err)
		got = getModel(t, cli, failed.Name)
		if attempt < inspectAttempts {
			assert.Empty(t, got.Annotations[v1.ModelInspectAnnotation], "a failed read is tried again, attempt %d", attempt)
			assert.False(t, inspectJobExists(t, cli, failed))
		}
	}
	assert.Equal(t, inspectFailed, got.Annotations[v1.ModelInspectAnnotation])
	assert.Contains(t, got.Annotations[v1.ModelInspectErrorAnnotation], "config.json does not exist")
	assert.NotEmpty(t, got.Annotations[inspectFailedAtAnnotation])
	res, err := r.inspectModel(ctx, got)
	require.NoError(t, err)
	assert.InDelta(t, inspectFailedRetryInterval.Seconds(), res.RequeueAfter.Seconds(), 60,
		"a recorded failure is read again after a long while")
	assert.False(t, inspectJobExists(t, cli, failed), "until then the failure is kept")

	// Removing the failure time reads the model again at once.
	patch := client.MergeFrom(got.DeepCopy())
	v1.RemoveAnnotation(got, inspectFailedAtAnnotation)
	require.NoError(t, cli.Patch(ctx, got, patch))
	_, err = r.inspectModel(ctx, got)
	require.NoError(t, err)
	assert.True(t, inspectJobExists(t, cli, failed))
	got = getModel(t, cli, failed.Name)
	assert.Empty(t, got.Annotations[v1.ModelInspectAnnotation])
	assert.Equal(t, "1", got.Annotations[inspectAttemptsAnnotation])
}

// Each job counts once created, so jobs the TTL controller deleted before their failure was seen still bound the
// attempts.
func TestInspectModelCountsJobsDeletedBeforeTheirFailureWasSeen(t *testing.T) {
	r, cli, model := inspectFixture(t, "m1", nil)
	ctx := context.Background()
	for attempt := 1; attempt <= inspectAttempts; attempt++ {
		_, err := r.inspectModel(ctx, getModel(t, cli, model.Name))
		require.NoError(t, err)
		assert.Equal(t, strconv.Itoa(attempt), getModel(t, cli, model.Name).Annotations[inspectAttemptsAnnotation])
		job := &v1.OpsJob{}
		require.NoError(t, cli.Get(ctx, client.ObjectKey{Name: inspectJobName(model)}, job))
		require.NoError(t, cli.Delete(ctx, job), "deleted after its TTL")
	}
	_, err := r.inspectModel(ctx, getModel(t, cli, model.Name))
	require.NoError(t, err)
	assert.False(t, inspectJobExists(t, cli, model), "no job beyond the attempts")
	got := getModel(t, cli, model.Name)
	assert.Equal(t, inspectFailed, got.Annotations[v1.ModelInspectAnnotation])
	assert.Contains(t, got.Annotations[v1.ModelInspectErrorAnnotation], "could not be read in 3 attempts")
}

// An image without python3 is not the model's failure: the reason is shown and the read is tried again later.
func TestInspectModelRetriesWhenTheImageHasNoPython(t *testing.T) {
	r, cli, model := inspectFixture(t, "m1", nil)
	ctx := context.Background()
	_, err := r.inspectModel(ctx, model)
	require.NoError(t, err)
	failInspectJob(t, cli, model, "[ERROR] "+inspectNoPython+"\n")
	res, err := r.inspectModel(ctx, getModel(t, cli, model.Name))
	require.NoError(t, err)
	assert.Equal(t, inspectEnvRetryInterval, res.RequeueAfter)
	got := getModel(t, cli, model.Name)
	assert.Empty(t, got.Annotations[v1.ModelInspectAnnotation], "not recorded as the model's failure")
	assert.Contains(t, got.Annotations[v1.ModelInspectErrorAnnotation], inspectNoPython, "the reason is shown")
	assert.Empty(t, got.Annotations[inspectAttemptsAnnotation], "the attempt does not count")
	assert.False(t, inspectJobExists(t, cli, model))

	res, err = r.inspectModel(ctx, got)
	require.NoError(t, err)
	assert.Positive(t, res.RequeueAfter)
	assert.False(t, inspectJobExists(t, cli, model), "not before the interval")

	patch := client.MergeFrom(got.DeepCopy())
	v1.SetAnnotation(got, inspectFailedAtAnnotation, time.Now().Add(-2*inspectEnvRetryInterval).UTC().Format(time.RFC3339))
	require.NoError(t, cli.Patch(ctx, got, patch))
	_, err = r.inspectModel(ctx, got)
	require.NoError(t, err)
	assert.True(t, inspectJobExists(t, cli, model), "read again after the interval")
}

// Without Post Training no model is read; a job that cannot be built, or whose workspace is gone, fails the read
// with its reason.
func TestInspectModelWaitsForPostTrainingAndFailsAJobItCannotBuild(t *testing.T) {
	model := genMockModel("registered", v1.AccessModeLocalPath, "ws1")
	model.Spec.Source.LocalPath = "/data/models/org--repo"
	model.Status.Phase = v1.ModelPhaseReady
	gone := genMockModel("gone", v1.AccessModeLocalPath, "ws-gone")
	gone.Spec.Source.LocalPath = "/data/models/gone"
	ws := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}}
	cli := fake.NewClientBuilder().WithScheme(lifecycleScheme(t)).WithObjects(model, gone, ws).
		WithStatusSubresource(model, gone).Build()
	r := &ModelReconciler{ClusterBaseReconciler: &ClusterBaseReconciler{Client: cli}, apiReader: cli,
		db: mockclient.NewMockInterface(gomock.NewController(t))}
	ctx := context.Background()
	setViper(t, map[string]any{"post_training.enable": false})
	res, err := r.inspectModel(ctx, model)
	require.NoError(t, err)
	assert.Zero(t, res.RequeueAfter, "with Post Training off nothing is read or requeued")
	assert.False(t, inspectJobExists(t, cli, model))

	enablePostTraining(t)
	_, err = r.inspectModel(ctx, model)
	require.NoError(t, err)
	got := getModel(t, cli, model.Name)
	assert.Equal(t, inspectFailed, got.Annotations[v1.ModelInspectAnnotation],
		"a job that cannot be built, here for a workspace without a cluster, fails the read")
	assert.Contains(t, got.Annotations[v1.ModelInspectErrorAnnotation], "no cluster")

	_, err = r.inspectModel(ctx, gone)
	require.NoError(t, err)
	got = getModel(t, cli, gone.Name)
	assert.Equal(t, inspectFailed, got.Annotations[v1.ModelInspectAnnotation], "a missing workspace is recorded")
	assert.Contains(t, got.Annotations[v1.ModelInspectErrorAnnotation], "workspace ws-gone")
}

// A model's files are read in the first of its workspaces that still exists; the read fails only when none does.
func TestInspectModelSkipsAWorkspaceThatIsGone(t *testing.T) {
	r, cli, model := inspectFixture(t, "two-paths", nil)
	model.Status.LocalPaths = []v1.ModelLocalPath{
		{Workspace: "ws-gone", Path: "/data/gone/two-paths", Status: v1.LocalPathStatusReady},
		{Workspace: "ws1", Path: "/data/ws1/two-paths", Status: v1.LocalPathStatusReady},
	}
	ctx := context.Background()
	_, err := r.inspectModel(ctx, model)
	require.NoError(t, err)
	job := &v1.OpsJob{}
	require.NoError(t, cli.Get(ctx, client.ObjectKey{Name: inspectJobName(model)}, job))
	assert.Equal(t, "/data/ws1/two-paths", job.GetParameter(v1.ParameterDestPath).Value,
		"the path in the workspace that exists is read")
	assert.Empty(t, getModel(t, cli, model.Name).Annotations[v1.ModelInspectAnnotation], "nothing is recorded")
}

// A model-inspect job the API server did not take is tried again, and the read is not recorded as failed.
func TestInspectModelRetriesAJobTheAPIServerDidNotTake(t *testing.T) {
	r, cli, model := inspectFixture(t, "registered", &interceptor.Funcs{
		Create: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
			if _, ok := obj.(*v1.OpsJob); ok {
				return errors.NewServiceUnavailable("the API server is busy")
			}
			return c.Create(ctx, obj, opts...)
		},
	})
	ctx := context.Background()
	_, err := r.inspectModel(ctx, model)
	require.Error(t, err, "the controller tries again with its backoff")
	got := getModel(t, cli, model.Name)
	assert.Empty(t, got.Annotations[v1.ModelInspectAnnotation])
	assert.Empty(t, got.Annotations[inspectAttemptsAnnotation], "a job not created does not count")
}

// A model-inspect job the OpsJob webhook refuses fails the read with the webhook's reason instead of being tried
// for ever.
func TestInspectModelRecordsAJobTheWebhookRefuses(t *testing.T) {
	denied := commonerrors.NewBadRequest(`admission webhook "vopsjob" denied the request: ` +
		"dest.path must be an absolute path below a models directory")
	r, cli, model := inspectFixture(t, "registered", &interceptor.Funcs{
		Create: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
			if _, ok := obj.(*v1.OpsJob); ok {
				return denied
			}
			return c.Create(ctx, obj, opts...)
		},
	})
	_, err := r.inspectModel(context.Background(), model)
	require.NoError(t, err)
	got := getModel(t, cli, model.Name)
	assert.Equal(t, inspectFailed, got.Annotations[v1.ModelInspectAnnotation])
	assert.Contains(t, got.Annotations[v1.ModelInspectErrorAnnotation], "below a models directory")
}

// A download keeps the structure summary its job reported only while Post Training is on.
func TestDownloadKeepsModelConfigOnlyForPostTraining(t *testing.T) {
	for _, on := range []bool{false, true} {
		setViper(t, map[string]any{"s3.enable": false, "post_training.enable": on})
		cl := lifecycleClient(t, lifecycleModel("m1"), lifecycleWorkspace("ws1", "c1", lifecycleRoot))
		r := newMockModelReconciler(cl)
		if on {
			r.db = mockclient.NewMockInterface(gomock.NewController(t))
		}
		for i := 0; i < 4; i++ { // finalizer, Pending, Downloading, download job
			reconcileModel(t, r, "m1")
		}
		downloads := listOpsJobs(t, cl, v1.OpsJobDownloadType)
		require.Len(t, downloads, 1)
		setOpsJobPhase(t, cl, downloads[0].Name, v1.OpsJobSucceeded, v1.Parameter{Name: "result",
			Value: "[SUCCESS] " + modelConfigMarker + `{"model_type":"qwen3"}` + "\n[SUCCESS] " + modelSizeMarker + "1\n"})
		reconcileModel(t, r, "m1")
		m := getModel(t, cl, "m1")
		require.Equal(t, v1.ModelPhaseReady, m.Status.Phase)
		if on {
			require.NotNil(t, m.Status.ModelConfig)
			assert.Equal(t, "qwen3", m.Status.ModelConfig.ModelType)
		} else {
			assert.Nil(t, m.Status.ModelConfig, "nothing is kept for Post Training while it is off")
		}
	}
}

// A directory cleanup job refuses a path outside the workspace volume.
func TestNewDirectoryCleanupOpsJob(t *testing.T) {
	ws := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}, Spec: v1.WorkspaceSpec{Cluster: "c1",
		Volumes: []v1.WorkspaceVolume{{Type: v1.PFS, MountPath: "/wekafs"}}}}
	dir := "/wekafs/custom/models/post-training-cache/slime-qwen3-8B-m1-540ea91"
	job, err := NewDirectoryCleanupOpsJob(ws, dir, map[string]string{v1.PostTrainingConvertCacheLabel: "7"})
	require.NoError(t, err)
	assert.Equal(t, DirectoryCleanupJobName("ws1", dir), job.Name)
	assert.Equal(t, v1.OpsJobModelCleanupType, job.Spec.Type)
	assert.Equal(t, dir, job.GetParameter(v1.ParameterDestPath).Value)
	assert.Equal(t, "/wekafs", job.Spec.Env["VOLUME_ROOT"])
	assert.Equal(t, "7", job.Labels[v1.PostTrainingConvertCacheLabel])
	_, err = NewDirectoryCleanupOpsJob(ws, "/etc/models/x", nil)
	assert.Error(t, err, "a path outside the workspace volume is refused")
}
