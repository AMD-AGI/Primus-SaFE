/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package ops_job

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/agiledragon/gomonkey/v2"
	"golang.org/x/crypto/ssh"
	k8sfake "k8s.io/client-go/kubernetes/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonctrl "github.com/AMD-AIG-AIMA/SAFE/common/pkg/controller"
	commonclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/k8sclient"
	commonsearch "github.com/AMD-AIG-AIMA/SAFE/common/pkg/opensearch"
	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
	rmutils "github.com/AMD-AIG-AIMA/SAFE/resource-manager/pkg/utils"
)

// ---- exportimage controller ----

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

// ---- dumplog controller ----

func TestDumpLogObserveFilter(t *testing.T) {
	r := &DumpLogJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t)}
	job := &v1.OpsJob{Spec: v1.OpsJobSpec{Type: v1.OpsJobDumpLogType}}
	quit, err := r.observe(context.Background(), job)
	assert.NoError(t, err)
	assert.False(t, quit)
	assert.False(t, r.filter(context.Background(), job))
	assert.True(t, r.filter(context.Background(), &v1.OpsJob{Spec: v1.OpsJobSpec{Type: v1.OpsJobRebootType}}))
}

func TestDumpLogHandlePending(t *testing.T) {
	job := &v1.OpsJob{
		ObjectMeta: metav1.ObjectMeta{Name: "j1"},
		Spec:       v1.OpsJobSpec{Type: v1.OpsJobDumpLogType},
	}
	r := &DumpLogJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t, job)}
	r.Controller = commonctrl.NewController[string](nil, 1)
	_, err := r.handle(context.Background(), job)
	assert.NoError(t, err)
	assert.Equal(t, v1.OpsJobRunning, job.Status.Phase)
}

func TestBuildSearchBody(t *testing.T) {
	job := &v1.OpsJob{Spec: v1.OpsJobSpec{Inputs: []v1.Parameter{
		{Name: v1.ParameterNode, Value: "node1"},
	}}}
	wl := &workloadInfo{
		workloadId: "wl1",
		startTime:  time.Now().Add(-time.Hour),
		endTime:    time.Now(),
	}
	body := buildSearchBody(job, wl)
	assert.NotEmpty(t, body)
	assert.Contains(t, string(body), "wl1")
}

func TestDumpLogGetInputWorkloadNoParam(t *testing.T) {
	r := &DumpLogJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t)}
	_, err := r.getInputWorkload(context.Background(), &v1.OpsJob{})
	assert.Error(t, err)
}

func TestDumpLogGetInputWorkloadFromK8s(t *testing.T) {
	wl := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "wl1", Labels: map[string]string{v1.ClusterIdLabel: "c1"}}}
	r := &DumpLogJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t, wl)}
	job := &v1.OpsJob{Spec: v1.OpsJobSpec{Inputs: []v1.Parameter{{Name: v1.ParameterWorkload, Value: "wl1"}}}}
	info, err := r.getInputWorkload(context.Background(), job)
	assert.NoError(t, err)
	assert.Equal(t, "wl1", info.workloadId)
	assert.Equal(t, "c1", info.cluster)
	assert.False(t, info.endTime.IsZero())
}

func TestSerializeSearchResponse(t *testing.T) {
	raw := `{"hits":{"total":{"value":1},"hits":[{"_id":"1","_source":{"@timestamp":"t1","message":"hello"}}]}}`
	resp := &commonsearch.OpenSearchLogResponse{}
	assert.NoError(t, json.Unmarshal([]byte(raw), resp))
	out := serializeSearchResponse(resp)
	assert.Contains(t, out, "t1")
	assert.Contains(t, out, "hello")
}

// ---- prewarm controller ----

func TestPrewarmObserveFilter(t *testing.T) {
	r := &PrewarmJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t)}
	job := &v1.OpsJob{Spec: v1.OpsJobSpec{Type: v1.OpsJobPrewarmType}}
	quit, err := r.observe(context.Background(), job)
	assert.NoError(t, err)
	assert.False(t, quit)
	assert.False(t, r.filter(context.Background(), job))
	assert.True(t, r.filter(context.Background(), &v1.OpsJob{Spec: v1.OpsJobSpec{Type: v1.OpsJobRebootType}}))
}

func TestPrewarmHandlePending(t *testing.T) {
	job := &v1.OpsJob{
		ObjectMeta: metav1.ObjectMeta{Name: "j1"},
		Spec:       v1.OpsJobSpec{Type: v1.OpsJobPrewarmType},
	}
	r := &PrewarmJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t, job)}
	r.Controller = commonctrl.NewController[string](nil, 1)
	res, err := r.handle(context.Background(), job)
	assert.NoError(t, err)
	assert.Equal(t, v1.OpsJobRunning, job.Status.Phase)
	assert.True(t, res.RequeueAfter > 0)
}
