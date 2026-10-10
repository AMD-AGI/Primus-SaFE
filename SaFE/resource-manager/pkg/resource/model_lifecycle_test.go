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
	"strings"
	"testing"
	"time"

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
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	commonopsjob "github.com/AMD-AIG-AIMA/SAFE/common/pkg/ops_job"
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
	for _, in := range []string{"Org--Repo_V1.5", "0abc", "cleanup-a-very-long-model-name-that-goes-beyond-the-limit"} {
		name := jobDisplayName(in)
		assert.LessOrEqual(t, len(name), 41)
		assert.Regexp(t, `^[a-z][-a-z0-9]*[a-z0-9]$`, name)
	}
}

// TestModelPrivateWorkspaceWithoutVolume: no storage means no download target, the
// model fails instead of downloading into the container's own filesystem.
func TestModelPrivateWorkspaceWithoutVolume(t *testing.T) {
	ws := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}, Spec: v1.WorkspaceSpec{Cluster: "c1"}}
	cl := lifecycleClient(t, ws)
	r := newMockModelReconciler(cl)
	assert.Empty(t, r.initializeLocalPaths(context.Background(), lifecycleModel("m1")))
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
	paths := r.initializeLocalPaths(context.Background(), model)
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
	legacy := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: downloadJobName(model, "ws1"), Labels: map[string]string{
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
