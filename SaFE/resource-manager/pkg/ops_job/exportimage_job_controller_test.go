/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package ops_job

import (
	"context"
	"encoding/base64"
	"testing"
	"time"

	"github.com/agiledragon/gomonkey/v2"
	"github.com/stretchr/testify/assert"
	"golang.org/x/crypto/ssh"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonctrl "github.com/AMD-AIG-AIMA/SAFE/common/pkg/controller"
	commonclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/k8sclient"
	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
	rmutils "github.com/AMD-AIG-AIMA/SAFE/resource-manager/pkg/utils"
)

func exportJob(name, workloadId, image string) *v1.OpsJob {
	job := &v1.OpsJob{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: v1.OpsJobSpec{
			Type: v1.OpsJobExportImageType,
			Inputs: []v1.Parameter{
				{Name: v1.ParameterWorkload, Value: workloadId},
				{Name: v1.ParameterImage, Value: image},
			},
		},
		Status: v1.OpsJobStatus{Phase: v1.OpsJobRunning},
	}
	return job
}

func TestGenerateTargetImageName(t *testing.T) {
	out, err := generateTargetImageName("rocm/7.0-preview:tag")
	assert.NoError(t, err)
	assert.Contains(t, out, "rocm/7.0-preview")

	out, err = generateTargetImageName("nginx")
	assert.NoError(t, err)
	assert.Contains(t, out, "library/nginx")

	out, err = generateTargetImageName("docker.io/library/nginx:1.0")
	assert.NoError(t, err)
	assert.Contains(t, out, "library/nginx")
}

func TestGetWorkloadIdAndSourceImageFromJob(t *testing.T) {
	job := &v1.OpsJob{Spec: v1.OpsJobSpec{Inputs: []v1.Parameter{
		{Name: v1.ParameterWorkload, Value: "wl1"},
		{Name: v1.ParameterImage, Value: "img:1"},
	}}}
	assert.Equal(t, "wl1", getWorkloadIdFromJob(job))
	assert.Equal(t, "img:1", getSourceImageFromJob(job))
	assert.Equal(t, "", getWorkloadIdFromJob(&v1.OpsJob{}))
	assert.Equal(t, "", getSourceImageFromJob(&v1.OpsJob{}))
}

func TestExportImageObserveFilter(t *testing.T) {
	r := &ExportImageJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t)}
	job := &v1.OpsJob{Spec: v1.OpsJobSpec{Type: v1.OpsJobExportImageType}}
	quit, err := r.observe(context.Background(), job)
	assert.NoError(t, err)
	assert.False(t, quit)
	assert.False(t, r.filter(context.Background(), job))
	assert.True(t, r.filter(context.Background(), &v1.OpsJob{Spec: v1.OpsJobSpec{Type: v1.OpsJobRebootType}}))
}

func TestExportImageHandlePending(t *testing.T) {
	job := &v1.OpsJob{
		ObjectMeta: metav1.ObjectMeta{Name: "j1"},
		Spec:       v1.OpsJobSpec{Type: v1.OpsJobExportImageType},
	}
	r := &ExportImageJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t, job)}
	r.Controller = commonctrl.NewController[string](nil, 1)
	_, err := r.handle(context.Background(), job)
	assert.NoError(t, err)
	assert.Equal(t, v1.OpsJobRunning, job.Status.Phase)
}

func TestExportImageHandleNonPending(t *testing.T) {
	job := &v1.OpsJob{
		ObjectMeta: metav1.ObjectMeta{Name: "j1"},
		Spec:       v1.OpsJobSpec{Type: v1.OpsJobExportImageType},
		Status:     v1.OpsJobStatus{Phase: v1.OpsJobRunning, StartedAt: &metav1.Time{Time: time.Now()}},
	}
	r := &ExportImageJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t, job)}
	res, err := r.handle(context.Background(), job)
	assert.NoError(t, err)
	assert.Equal(t, int64(0), res.RequeueAfter.Nanoseconds())
}

func TestExportImageDoNoWorkloadId(t *testing.T) {
	job := &v1.OpsJob{
		ObjectMeta: metav1.ObjectMeta{Name: "j1"},
		Spec:       v1.OpsJobSpec{Type: v1.OpsJobExportImageType},
	}
	r := &ExportImageJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t, job)}
	_, err := r.Do(context.Background(), "j1")
	assert.NoError(t, err)
	updated := &v1.OpsJob{}
	assert.NoError(t, r.Get(context.Background(), client.ObjectKey{Name: "j1"}, updated))
	assert.Equal(t, v1.OpsJobFailed, updated.Status.Phase)
}

func TestExportImageDoJobNotFound(t *testing.T) {
	r := &ExportImageJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t)}
	_, err := r.Do(context.Background(), "missing")
	assert.Error(t, err)
}

func TestExportImageDoMissingWorkloadId(t *testing.T) {
	job := &v1.OpsJob{
		ObjectMeta: metav1.ObjectMeta{Name: "j1"},
		Spec:       v1.OpsJobSpec{Type: v1.OpsJobExportImageType},
		Status:     v1.OpsJobStatus{Phase: v1.OpsJobRunning},
	}
	r := &ExportImageJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t, job)}
	_, err := r.Do(context.Background(), "j1")
	assert.NoError(t, err)
	updated := &v1.OpsJob{}
	assert.NoError(t, r.Get(context.Background(), types.NamespacedName{Name: "j1"}, updated))
	assert.Equal(t, v1.OpsJobFailed, updated.Status.Phase)
}

func TestExportImageGetHarborCredentials(t *testing.T) {
	authStr := base64.StdEncoding.EncodeToString([]byte("admin:secret"))
	configJSON := `{"auths":{"harbor.local":{"auth":"` + authStr + `"}}}`
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: common.ImageImportSecretName, Namespace: common.PrimusSafeNamespace},
		Data:       map[string][]byte{"config.json": []byte(configJSON)},
	}
	r := &ExportImageJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t, secret)}
	user, pass, err := r.getHarborCredentials(context.Background(), "harbor.local")
	assert.NoError(t, err)
	assert.Equal(t, "admin", user)
	assert.Equal(t, "secret", pass)
}

func TestExportImageGetHarborCredentialsNoSecret(t *testing.T) {
	r := &ExportImageJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t)}
	_, _, err := r.getHarborCredentials(context.Background(), "harbor.local")
	assert.Error(t, err)
}

func TestExportImageGetContainerIDFromPod(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "p1", Namespace: "ws1"},
		Status: corev1.PodStatus{
			ContainerStatuses: []corev1.ContainerStatus{
				{ContainerID: "containerd://abc123"},
			},
		},
	}
	cs := k8sfake.NewSimpleClientset(pod)
	r := &ExportImageJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t)}
	patches := gomonkey.ApplyFunc(rmutils.GetK8sClientFactory,
		func(_ *commonutils.ObjectManager, _ string) (*commonclient.ClientFactory, error) {
			return commonclient.NewClientFactoryWithOnlyClient(context.Background(), "c1", cs), nil
		})
	defer patches.Reset()
	id, err := r.getContainerIDFromPod(context.Background(), "p1", "c1", "ws1")
	assert.NoError(t, err)
	assert.Equal(t, "abc123", id)
}

func TestExportImageCommitAndPushViaSSH(t *testing.T) {
	sshClient, cleanup := startInMemorySSHServer(t)
	defer cleanup()
	r := &ExportImageJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t)}
	assert.NoError(t, r.commitContainerToImage(sshClient, "cid", "img:1"))
	assert.NoError(t, r.pushImage(sshClient, "img:1"))
}

func TestExportImageLoginHarborAndDelete(t *testing.T) {
	sshClient, cleanup := startInMemorySSHServer(t)
	defer cleanup()
	r := &ExportImageJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t)}
	// loginHarbor: server replies with empty output -> "unexpected output" error path.
	_ = r.loginHarbor(sshClient, "harbor.local", "u", "p")
	// deleteImage: best-effort, server returns success.
	_ = r.deleteImage(context.Background(), sshClient, "img:1")
}

func TestExportImageViaSSHCommitOnly(t *testing.T) {
	sshClient, cleanup := startInMemorySSHServer(t)
	defer cleanup()
	node := &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n1"}}
	r := &ExportImageJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t, node)}
	patches := gomonkey.ApplyFunc(rmutils.GetSSHClient,
		func(_ context.Context, _ client.Client, _ *v1.Node) (*ssh.Client, error) {
			return sshClient, nil
		})
	defer patches.Reset()
	// commit succeeds, login fails on empty output -> returns error; exercises the SSH path.
	_ = r.exportImageViaSSH(context.Background(), node, "img:1", "cid", "harbor.local", "u", "p")
}

func TestExportImageDoWorkloadNotFound(t *testing.T) {
	job := &v1.OpsJob{
		ObjectMeta: metav1.ObjectMeta{Name: "j1"},
		Spec: v1.OpsJobSpec{
			Type: v1.OpsJobExportImageType,
			Inputs: []v1.Parameter{
				{Name: v1.ParameterWorkload, Value: "wl1"},
				{Name: v1.ParameterImage, Value: "img:1"},
			},
		},
	}
	r := &ExportImageJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t, job)}
	_, err := r.Do(context.Background(), "j1")
	assert.NoError(t, err)
	updated := &v1.OpsJob{}
	assert.NoError(t, r.Get(context.Background(), client.ObjectKey{Name: "j1"}, updated))
	assert.Equal(t, v1.OpsJobFailed, updated.Status.Phase)
}

func TestExportImageDoWorkloadBranches(t *testing.T) {
	ctx := context.Background()

	// workload missing -> failed
	t.Run("workload missing", func(t *testing.T) {
		job := exportJob("e1", "wl-missing", "img:1")
		r := &ExportImageJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t, job)}
		_, err := r.Do(ctx, "e1")
		assert.NoError(t, err)
		updated := &v1.OpsJob{}
		assert.NoError(t, r.Get(ctx, types.NamespacedName{Name: "e1"}, updated))
		assert.Equal(t, v1.OpsJobFailed, updated.Status.Phase)
	})

	// workload with no pods -> failed
	t.Run("workload no pods", func(t *testing.T) {
		job := exportJob("e2", "wl2", "img:1")
		wl := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "wl2"}}
		r := &ExportImageJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t, job, wl)}
		_, err := r.Do(ctx, "e2")
		assert.NoError(t, err)
		updated := &v1.OpsJob{}
		assert.NoError(t, r.Get(ctx, types.NamespacedName{Name: "e2"}, updated))
		assert.Equal(t, v1.OpsJobFailed, updated.Status.Phase)
	})

	// workload pod scheduled to no node -> failed
	t.Run("pod empty node", func(t *testing.T) {
		job := exportJob("e3", "wl3", "img:1")
		wl := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "wl3"}}
		wl.Status.Pods = []v1.WorkloadPod{{AdminNodeName: ""}}
		r := &ExportImageJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t, job, wl)}
		_, err := r.Do(ctx, "e3")
		assert.NoError(t, err)
		updated := &v1.OpsJob{}
		assert.NoError(t, r.Get(ctx, types.NamespacedName{Name: "e3"}, updated))
		assert.Equal(t, v1.OpsJobFailed, updated.Status.Phase)
	})

	// admin node missing -> failed
	t.Run("node missing", func(t *testing.T) {
		job := exportJob("e4", "wl4", "img:1")
		wl := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "wl4"}}
		wl.Status.Pods = []v1.WorkloadPod{{AdminNodeName: "n-missing"}}
		r := &ExportImageJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t, job, wl)}
		_, err := r.Do(ctx, "e4")
		assert.NoError(t, err)
		updated := &v1.OpsJob{}
		assert.NoError(t, r.Get(ctx, types.NamespacedName{Name: "e4"}, updated))
		assert.Equal(t, v1.OpsJobFailed, updated.Status.Phase)
	})
}

func TestExportImageReconcileEntry(t *testing.T) {
	job := &v1.OpsJob{
		ObjectMeta: metav1.ObjectMeta{Name: "j1", Finalizers: []string{v1.OpsJobFinalizer}},
		Spec:       v1.OpsJobSpec{Type: v1.OpsJobExportImageType},
	}
	r := &ExportImageJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t, job)}
	r.Controller = commonctrl.NewController[string](nil, 1)
	_, err := r.Reconcile(context.Background(), ctrlruntime.Request{NamespacedName: types.NamespacedName{Name: "j1"}})
	assert.NoError(t, err)
}

func TestExportImageQueueControllerStarts(t *testing.T) {
	ei := &ExportImageJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t)}
	ei.Controller = commonctrl.NewController[string](ei, 0)
	ei.start(context.Background())
}
