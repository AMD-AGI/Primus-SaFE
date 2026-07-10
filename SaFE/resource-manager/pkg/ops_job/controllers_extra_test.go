/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package ops_job

import (
	"context"
	"testing"
	"time"

	"github.com/agiledragon/gomonkey/v2"
	"github.com/stretchr/testify/assert"
	"golang.org/x/crypto/ssh"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	rmutils "github.com/AMD-AIG-AIMA/SAFE/resource-manager/pkg/utils"
)

func fullScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	if err := v1.AddToScheme(s); err != nil {
		t.Fatal(err)
	}
	if err := corev1.AddToScheme(s); err != nil {
		t.Fatal(err)
	}
	if err := appsv1.AddToScheme(s); err != nil {
		t.Fatal(err)
	}
	if err := batchv1.AddToScheme(s); err != nil {
		t.Fatal(err)
	}
	return s
}

func newBaseWithObjs(t *testing.T, objs ...client.Object) *OpsJobBaseReconciler {
	t.Helper()
	cl := ctrlfake.NewClientBuilder().
		WithScheme(fullScheme(t)).
		WithStatusSubresource(&v1.OpsJob{}, &v1.Workload{}, &v1.Model{}).
		WithObjects(objs...).
		Build()
	return &OpsJobBaseReconciler{Client: cl}
}

// ---- download controller ----

func downloadJob(name string) *v1.OpsJob {
	return &v1.OpsJob{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Labels: map[string]string{
				v1.WorkspaceIdLabel: "ws1",
				v1.ClusterIdLabel:   "c1",
			},
		},
		Spec: v1.OpsJobSpec{
			Type:  v1.OpsJobDownloadType,
			Image: pointer.String("img:latest"),
			Inputs: []v1.Parameter{
				{Name: v1.ParameterSecret, Value: "sec"},
				{Name: v1.ParameterEndpoint, Value: "http://x"},
				{Name: v1.ParameterDestPath, Value: "/data"},
			},
		},
	}
}

func TestIsDownloadWorkload(t *testing.T) {
	wl := &v1.Workload{ObjectMeta: metav1.ObjectMeta{
		Labels: map[string]string{
			v1.OpsJobIdLabel:   "j1",
			v1.OpsJobTypeLabel: string(v1.OpsJobDownloadType),
		},
	}}
	assert.True(t, isDownloadWorkload(wl))
	assert.False(t, isDownloadWorkload(&v1.Workload{}))
}

func TestDownloadObserveFilter(t *testing.T) {
	r := &DownloadJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t)}
	job := downloadJob("j1")
	quit, err := r.observe(context.Background(), job)
	assert.NoError(t, err)
	assert.False(t, quit)
	assert.False(t, r.filter(context.Background(), job))

	other := &v1.OpsJob{Spec: v1.OpsJobSpec{Type: v1.OpsJobRebootType}}
	assert.True(t, r.filter(context.Background(), other))
}

func TestDownloadGenerateWorkload(t *testing.T) {
	job := downloadJob("j1")
	ws := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}}
	r := &DownloadJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t, job, ws)}
	wl, err := r.generateDownloadWorkload(context.Background(), job)
	assert.NoError(t, err)
	assert.Equal(t, "j1", wl.Name)
	assert.Equal(t, "http://x", wl.Spec.Env["INPUT_URL"])
	assert.Equal(t, "/data", wl.Spec.Env["DEST_PATH"])
}

func TestDownloadGenerateWorkloadNoWorkspaceId(t *testing.T) {
	job := downloadJob("j1")
	job.Labels = nil
	r := &DownloadJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t)}
	_, err := r.generateDownloadWorkload(context.Background(), job)
	assert.Error(t, err)
}

func TestDownloadHandleSetsPending(t *testing.T) {
	job := downloadJob("j1")
	ws := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}}
	r := &DownloadJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t, job, ws)}
	// First handle: phase empty -> set Pending and requeue.
	res, err := r.handle(context.Background(), job)
	assert.NoError(t, err)
	assert.Equal(t, v1.OpsJobPending, job.Status.Phase)
	_ = res
}

func TestDownloadHandleCreatesWorkload(t *testing.T) {
	job := downloadJob("j1")
	job.Status.Phase = v1.OpsJobPending
	ws := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}}
	r := &DownloadJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t, job, ws)}
	_, err := r.handle(context.Background(), job)
	assert.NoError(t, err)
	// Workload should now exist.
	wl := &v1.Workload{}
	assert.NoError(t, r.Get(context.Background(), client.ObjectKey{Name: "j1"}, wl))
}

// ---- CD controller ----

func cdJob(name string) *v1.OpsJob {
	return &v1.OpsJob{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec:       v1.OpsJobSpec{Type: v1.OpsJobCDType},
	}
}

func TestIsCDWorkload(t *testing.T) {
	wl := &v1.Workload{ObjectMeta: metav1.ObjectMeta{
		Labels: map[string]string{
			v1.OpsJobIdLabel:   "j1",
			v1.OpsJobTypeLabel: string(v1.OpsJobCDType),
		},
	}}
	assert.True(t, isCDWorkload(wl))
	assert.False(t, isCDWorkload(&v1.Workload{}))
}

func TestCDObserveFilter(t *testing.T) {
	r := &CDJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t)}
	job := cdJob("j1")
	quit, err := r.observe(context.Background(), job)
	assert.NoError(t, err)
	assert.False(t, quit)
	assert.False(t, r.filter(context.Background(), job))
	assert.True(t, r.filter(context.Background(), &v1.OpsJob{Spec: v1.OpsJobSpec{Type: v1.OpsJobRebootType}}))
}

func TestCDGenerateSafeAndLensWorkload(t *testing.T) {
	r := &CDJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t)}
	job := cdJob("j1")
	safe := r.generateSafeCDWorkload(job, "c1", "main")
	assert.Equal(t, "j1", safe.Name)
	assert.Equal(t, PrimusSaFERepoURL, safe.Spec.Env["REPO_URL"])

	lens := r.generateLensCDWorkload(job, "c1", "dev")
	assert.Equal(t, "j1", lens.Name)
	assert.Equal(t, "dev", lens.Spec.Env["DEPLOY_BRANCH"])
}

func TestCDGenerateCDWorkloadNoControlPlane(t *testing.T) {
	job := cdJob("j1")
	r := &CDJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t, job)}
	_, err := r.generateCDWorkload(context.Background(), job)
	assert.Error(t, err)
}

func TestCDGenerateCDWorkloadWithControlPlane(t *testing.T) {
	job := cdJob("j1")
	cluster := &v1.Cluster{ObjectMeta: metav1.ObjectMeta{
		Name:   "ctrl",
		Labels: map[string]string{v1.ClusterControlPlaneLabel: ""},
	}}
	r := &CDJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t, job, cluster)}
	wl, err := r.generateCDWorkload(context.Background(), job)
	assert.NoError(t, err)
	assert.Equal(t, "j1", wl.Name)
	assert.NotNil(t, wl.Spec.Timeout)
}

// ---- reboot controller ----

func rebootJob(name string, nodes ...string) *v1.OpsJob {
	inputs := make([]v1.Parameter, 0, len(nodes))
	for _, n := range nodes {
		inputs = append(inputs, v1.Parameter{Name: v1.ParameterNode, Value: n})
	}
	return &v1.OpsJob{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec:       v1.OpsJobSpec{Type: v1.OpsJobRebootType, Inputs: inputs},
	}
}

func TestRebootFilter(t *testing.T) {
	r := &RebootJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t)}
	assert.False(t, r.filter(context.Background(), rebootJob("j1", "n1")))
	assert.True(t, r.filter(context.Background(), &v1.OpsJob{Spec: v1.OpsJobSpec{Type: v1.OpsJobCDType}}))
}

func TestRebootGetTheUnprocessedNodes(t *testing.T) {
	r := &RebootJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t)}

	// No nodes left -> succeeded.
	job := rebootJob("j1")
	nodes, phase := r.getTheUnprocessedNodes(job)
	assert.Empty(t, nodes)
	assert.Equal(t, v1.OpsJobSucceeded, phase)

	// Pending phase with unprocessed nodes.
	job = rebootJob("j1", "n1")
	job.Status.Phase = v1.OpsJobPending
	nodes, phase = r.getTheUnprocessedNodes(job)
	assert.Empty(t, nodes)
	assert.Equal(t, v1.OpsJobPending, phase)

	// Running phase -> returns nodes.
	job.Status.Phase = v1.OpsJobRunning
	nodes, phase = r.getTheUnprocessedNodes(job)
	assert.Equal(t, []string{"n1"}, nodes)
	assert.Equal(t, v1.OpsJobRunning, phase)
}

func TestRebootObserveSucceeded(t *testing.T) {
	job := rebootJob("j1") // no nodes -> succeeded
	r := &RebootJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t, job)}
	quit, err := r.observe(context.Background(), job)
	assert.NoError(t, err)
	assert.True(t, quit)
}

func TestRebootHandlePending(t *testing.T) {
	job := rebootJob("j1", "n1")
	job.Status.Phase = v1.OpsJobPending
	r := &RebootJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t, job)}
	res, err := r.handle(context.Background(), job)
	assert.NoError(t, err)
	assert.Equal(t, v1.OpsJobRunning, job.Status.Phase)
	assert.Equal(t, time.Second, res.RequeueAfter)
}

func TestRebootSetJobOutput(t *testing.T) {
	job := rebootJob("j1", "n1")
	r := &RebootJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t, job)}
	err := r.setJobOutput(context.Background(), "j1", "n1")
	assert.NoError(t, err)
	updated := &v1.OpsJob{}
	assert.NoError(t, r.Get(context.Background(), client.ObjectKey{Name: "j1"}, updated))
	assert.Len(t, updated.Status.Outputs, 1)
}

func TestRebootExecRebootNodeNotFound(t *testing.T) {
	r := &RebootJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t)}
	err := r.execReboot(context.Background(), "j1", "missing")
	assert.Error(t, err)
}

func TestRebootHandleRunning(t *testing.T) {
	sshClient, cleanup := startInMemorySSHServer(t)
	defer cleanup()
	node := &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n1"}}
	job := rebootJob("j1", "n1")
	job.Status.Phase = v1.OpsJobRunning
	job.Status.StartedAt = &metav1.Time{Time: time.Now()}
	r := &RebootJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t, node, job)}

	patches := gomonkey.ApplyFunc(rmutils.GetSSHClient,
		func(_ context.Context, _ client.Client, _ *v1.Node) (*ssh.Client, error) {
			return sshClient, nil
		})
	defer patches.Reset()

	res, err := r.handle(context.Background(), job)
	assert.NoError(t, err)
	assert.Equal(t, time.Minute, res.RequeueAfter)
}

func TestRebootExecuteSSHCommand(t *testing.T) {
	sshClient, cleanup := startInMemorySSHServer(t)
	defer cleanup()
	r := &RebootJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t)}
	_, err := r.executeSSHCommand(sshClient, "echo hi")
	assert.NoError(t, err)
}

func TestRebootExecRebootViaSSH(t *testing.T) {
	sshClient, cleanup := startInMemorySSHServer(t)
	defer cleanup()

	node := &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n1"}}
	job := rebootJob("j1", "n1")
	r := &RebootJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t, node, job)}

	patches := gomonkey.ApplyFunc(rmutils.GetSSHClient,
		func(_ context.Context, _ client.Client, _ *v1.Node) (*ssh.Client, error) {
			return sshClient, nil
		})
	defer patches.Reset()

	err := r.execReboot(context.Background(), "j1", "n1")
	assert.NoError(t, err)
}

func TestRebootExecRebootSSHCommandError(t *testing.T) {
	sshClient, cleanup := startInMemorySSHServer(t)
	defer cleanup()
	sshClient.Close()

	node := &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n1"}}
	job := rebootJob("j1", "n1")
	r := &RebootJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t, node, job)}

	patches := gomonkey.ApplyFunc(rmutils.GetSSHClient,
		func(_ context.Context, _ client.Client, _ *v1.Node) (*ssh.Client, error) {
			return sshClient, nil
		})
	defer patches.Reset()

	err := r.execReboot(context.Background(), "j1", "n1")
	assert.Error(t, err)

	updated := &v1.OpsJob{}
	assert.NoError(t, r.Get(context.Background(), client.ObjectKey{Name: "j1"}, updated))
	assert.Len(t, updated.Status.Outputs, 0)
}

func TestRebootHandleSSHCommandErrorMarksJobFailed(t *testing.T) {
	sshClient, cleanup := startInMemorySSHServer(t)
	defer cleanup()
	sshClient.Close()

	node := &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n1"}}
	job := rebootJob("j1", "n1")
	job.Status.Phase = v1.OpsJobRunning
	job.Status.StartedAt = &metav1.Time{Time: time.Now()}
	r := &RebootJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t, node, job)}

	patches := gomonkey.ApplyFunc(rmutils.GetSSHClient,
		func(_ context.Context, _ client.Client, _ *v1.Node) (*ssh.Client, error) {
			return sshClient, nil
		})
	defer patches.Reset()

	_, err := r.handle(context.Background(), job)
	assert.NoError(t, err)

	updated := &v1.OpsJob{}
	assert.NoError(t, r.Get(context.Background(), client.ObjectKey{Name: "j1"}, updated))
	assert.Equal(t, v1.OpsJobFailed, updated.Status.Phase)
	assert.Len(t, updated.Status.Outputs, 0)
}

func TestRebootHandlePreservesCompletedOutputsOnLaterFailure(t *testing.T) {
	sshClient, cleanup := startInMemorySSHServer(t)
	defer cleanup()

	node1 := &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n1"}}
	node2 := &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n2"}}
	job := rebootJob("j1", "n1", "n2")
	job.Status.Phase = v1.OpsJobRunning
	job.Status.StartedAt = &metav1.Time{Time: time.Now()}
	r := &RebootJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t, node1, node2, job)}

	patches := gomonkey.ApplyFunc(rmutils.GetSSHClient,
		func(_ context.Context, _ client.Client, _ *v1.Node) (*ssh.Client, error) {
			return sshClient, nil
		})
	defer patches.Reset()

	_, err := r.handle(context.Background(), job)
	assert.NoError(t, err)

	updated := &v1.OpsJob{}
	assert.NoError(t, r.Get(context.Background(), client.ObjectKey{Name: "j1"}, updated))
	assert.Equal(t, v1.OpsJobFailed, updated.Status.Phase)
	assert.Equal(t, []v1.Parameter{{Name: v1.ParameterNode, Value: "n1"}}, updated.Status.Outputs)
}
