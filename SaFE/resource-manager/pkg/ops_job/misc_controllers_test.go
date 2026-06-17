/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package ops_job

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/agiledragon/gomonkey/v2"
	"github.com/stretchr/testify/assert"
	"golang.org/x/crypto/ssh"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	rmutils "github.com/AMD-AIG-AIMA/SAFE/resource-manager/pkg/utils"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/sets"
)

func ptrStr(s string) *string { return &s }

// ---- evaluation controller ----

func evalJob(name string) *v1.OpsJob {
	return &v1.OpsJob{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: v1.OpsJobSpec{
			Type: v1.OpsJobEvaluationType,
			Inputs: []v1.Parameter{
				{Name: v1.ParameterModelEndpoint, Value: "http://m"},
				{Name: v1.ParameterModelName, Value: "model"},
				{Name: v1.ParameterEvalBenchmarks, Value: `[{"datasetName":"math_500","datasetLocalDir":"/data/math_500"}]`},
			},
		},
	}
}

func TestEvalObserveFilter(t *testing.T) {
	r := &EvaluationJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t)}
	job := evalJob("j1")
	quit, err := r.observe(context.Background(), job)
	assert.NoError(t, err)
	assert.False(t, quit)
	assert.False(t, r.filter(context.Background(), job))
	assert.True(t, r.filter(context.Background(), &v1.OpsJob{Spec: v1.OpsJobSpec{Type: v1.OpsJobCDType}}))
}

func TestEvalGenerateWorkload(t *testing.T) {
	job := evalJob("j1")
	r := &EvaluationJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t, job)}
	wl, err := r.generateEvaluationWorkload(context.Background(), job)
	assert.NoError(t, err)
	assert.Equal(t, "j1", wl.Name)
	assert.NotEmpty(t, wl.Spec.EntryPoints)
}

func TestEvalHandleSetsPending(t *testing.T) {
	job := evalJob("j1")
	r := &EvaluationJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t, job)}
	_, err := r.handle(context.Background(), job)
	assert.NoError(t, err)
	assert.Equal(t, v1.OpsJobPending, job.Status.Phase)
}

func TestEvalHandleWorkloadExists(t *testing.T) {
	job := evalJob("j1")
	job.Status.Phase = v1.OpsJobPending
	wl := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "j1"}}
	r := &EvaluationJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t, job, wl)}
	// Workload already exists -> early return.
	_, err := r.handle(context.Background(), job)
	assert.NoError(t, err)
}

func TestPreflightHandleWorkloadExists(t *testing.T) {
	job := &v1.OpsJob{ObjectMeta: metav1.ObjectMeta{Name: "j1"}, Spec: v1.OpsJobSpec{Type: v1.OpsJobPreflightType}}
	job.Status.Phase = v1.OpsJobPending
	wl := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "j1"}}
	r := &PreflightJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t, job, wl)}
	_, err := r.handle(context.Background(), job)
	assert.NoError(t, err)
}

func TestCDHandleWorkloadExists(t *testing.T) {
	job := cdJob("j1")
	job.Status.Phase = v1.OpsJobPending
	wl := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "j1"}}
	r := &CDJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t, job, wl)}
	_, err := r.handle(context.Background(), job)
	assert.NoError(t, err)
}

func TestCDHandleGeneratesWorkload(t *testing.T) {
	job := cdJob("j1")
	job.Status.Phase = v1.OpsJobPending
	cluster := &v1.Cluster{ObjectMeta: metav1.ObjectMeta{
		Name:   "ctrl",
		Labels: map[string]string{v1.ClusterControlPlaneLabel: ""},
	}}
	r := &CDJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t, job, cluster)}
	_, err := r.handle(context.Background(), job)
	assert.NoError(t, err)
	wl := &v1.Workload{}
	assert.NoError(t, r.Get(context.Background(), client.ObjectKey{Name: "j1"}, wl))
}

func TestBuildEvalCommand(t *testing.T) {
	r := &EvaluationJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t)}
	benchmarks := `[{"datasetName":"math_500","datasetLocalDir":"/data/math_500","limit":10}]`
	cmd, err := r.buildEvalCommand(context.Background(), "http://m", "model", "", benchmarks, "task1", "", "", "", "", 7200, 32)
	assert.NoError(t, err)
	assert.Contains(t, cmd, "Pre-flight check")
	assert.Contains(t, cmd, "math_500")
}

func TestBuildEvalCommandMultiWithUpload(t *testing.T) {
	r := &EvaluationJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t)}
	benchmarks := `[{"datasetName":"a","datasetLocalDir":"/d/a"},{"datasetName":"b","datasetLocalDir":"/d/b"}]`
	cmd, err := r.buildEvalCommand(context.Background(), "http://m", "model", "", benchmarks, "task1", "http://put", "", "", "", 0, 16)
	assert.NoError(t, err)
	assert.Contains(t, cmd, "http://put")
}

func TestBuildEvalCommandErrors(t *testing.T) {
	r := &EvaluationJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t)}
	// Invalid JSON.
	_, err := r.buildEvalCommand(context.Background(), "m", "n", "", "bad", "t", "", "", "", "", 0, 1)
	assert.Error(t, err)
	// Empty benchmarks.
	_, err = r.buildEvalCommand(context.Background(), "m", "n", "", "[]", "t", "", "", "", "", 0, 1)
	assert.Error(t, err)
}

func TestBuildReportUploadScript(t *testing.T) {
	r := &EvaluationJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t)}
	script := r.buildReportUploadScript("/out", "http://put")
	assert.Contains(t, script, "/out")
	assert.Contains(t, script, "http://put")
}

// ---- preflight controller ----

func TestPreflightObserveFilter(t *testing.T) {
	r := &PreflightJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t)}
	job := &v1.OpsJob{Spec: v1.OpsJobSpec{Type: v1.OpsJobPreflightType}}
	quit, err := r.observe(context.Background(), job)
	assert.NoError(t, err)
	assert.False(t, quit)
	assert.False(t, r.filter(context.Background(), job))
	assert.True(t, r.filter(context.Background(), &v1.OpsJob{Spec: v1.OpsJobSpec{Type: v1.OpsJobCDType}}))
}

func TestPreflightHandleSetsPending(t *testing.T) {
	job := &v1.OpsJob{
		ObjectMeta: metav1.ObjectMeta{Name: "j1"},
		Spec:       v1.OpsJobSpec{Type: v1.OpsJobPreflightType},
	}
	r := &PreflightJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t, job)}
	_, err := r.handle(context.Background(), job)
	assert.NoError(t, err)
	assert.Equal(t, v1.OpsJobPending, job.Status.Phase)
}

func TestPreflightGenerateWorkloadNoResource(t *testing.T) {
	job := &v1.OpsJob{ObjectMeta: metav1.ObjectMeta{Name: "j1"}}
	r := &PreflightJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t)}
	_, err := r.generatePreflightWorkload(context.Background(), job)
	assert.Error(t, err)
}

// ---- addon controller helpers ----

func TestExecuteCommandViaSSH(t *testing.T) {
	sshClient, cleanup := startInMemorySSHServer(t)
	defer cleanup()
	out, err := executeCommand(sshClient, "myaddon", "ZWNobyBoaQ==")
	assert.NoError(t, err)
	assert.Contains(t, out, "myaddon")
}

func TestShellSingleQuote(t *testing.T) {
	assert.Equal(t, "'abc'", shellSingleQuote("abc"))
	assert.Equal(t, `'a'"'"'b'`, shellSingleQuote("a'b"))
}

func TestNormalizeMessage(t *testing.T) {
	assert.Equal(t, "", normalizeMessage(""))
	assert.Equal(t, "a b c", normalizeMessage("a\nb\tc"))
	long := strings.Repeat("x", maxMessageLen+10)
	assert.Equal(t, maxMessageLen, len(normalizeMessage(long)))
}

func TestIsMatchGpuChip(t *testing.T) {
	node := &v1.Node{}
	assert.True(t, isMatchGpuChip("", node))
	assert.False(t, isMatchGpuChip("unknown", node))
}

func TestAddonGetJobPhase(t *testing.T) {
	r := &AddonJobReconciler{
		OpsJobBaseReconciler: newBaseWithObjs(t),
		allJobs:              map[string]*AddonJob{},
	}
	// Unknown job -> pending.
	phase, _ := r.getJobPhase("missing")
	assert.Equal(t, v1.OpsJobPending, phase)

	// All nodes succeeded -> succeeded.
	r.allJobs["j1"] = &AddonJob{
		nodes: map[string]AddonJobPhase{
			"n1": {Phase: v1.OpsJobSucceeded, Message: "ok"},
		},
		maxFailCount: 2,
	}
	phase, _ = r.getJobPhase("j1")
	assert.Equal(t, v1.OpsJobSucceeded, phase)

	// Fail threshold reached -> failed.
	r.allJobs["j2"] = &AddonJob{
		nodes: map[string]AddonJobPhase{
			"n1": {Phase: v1.OpsJobFailed},
			"n2": {Phase: v1.OpsJobFailed},
		},
		maxFailCount: 2,
	}
	phase, _ = r.getJobPhase("j2")
	assert.Equal(t, v1.OpsJobFailed, phase)

	// Still running.
	r.allJobs["j3"] = &AddonJob{
		nodes: map[string]AddonJobPhase{
			"n1": {Phase: v1.OpsJobSucceeded},
			"n2": {Phase: v1.OpsJobRunning},
		},
		maxFailCount: 2,
	}
	phase, _ = r.getJobPhase("j3")
	assert.Equal(t, v1.OpsJobRunning, phase)
}

func TestAddonObserveFilter(t *testing.T) {
	r := &AddonJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t), allJobs: map[string]*AddonJob{}}
	job := &v1.OpsJob{ObjectMeta: metav1.ObjectMeta{Name: "j1"}, Spec: v1.OpsJobSpec{Type: v1.OpsJobAddonType}}
	// Unknown job in allJobs -> pending phase -> not quit.
	quit, err := r.observe(context.Background(), job)
	assert.NoError(t, err)
	assert.False(t, quit)
	assert.False(t, r.filter(context.Background(), job))
	assert.True(t, r.filter(context.Background(), &v1.OpsJob{Spec: v1.OpsJobSpec{Type: v1.OpsJobRebootType}}))
}

func TestAddonObserveEnded(t *testing.T) {
	r := &AddonJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t), allJobs: map[string]*AddonJob{}}
	job := &v1.OpsJob{
		ObjectMeta: metav1.ObjectMeta{Name: "j1"},
		Status:     v1.OpsJobStatus{FinishedAt: &metav1.Time{Time: time.Now()}},
	}
	quit, err := r.observe(context.Background(), job)
	assert.NoError(t, err)
	assert.True(t, quit)
}

func TestAddonGetInputAddonTemplates(t *testing.T) {
	tmpl := &v1.AddonTemplate{
		ObjectMeta: metav1.ObjectMeta{Name: "t1"},
		Spec:       v1.AddonTemplateSpec{Action: "echo hi"},
	}
	base := newBaseWithObjs(t, tmpl)
	r := &AddonJobReconciler{OpsJobBaseReconciler: base, allJobs: map[string]*AddonJob{}}

	// No params -> nil.
	res, err := r.getInputAddonTemplates(context.Background(), &v1.OpsJob{})
	assert.NoError(t, err)
	assert.Nil(t, res)

	// With param -> resolved.
	job := &v1.OpsJob{Spec: v1.OpsJobSpec{Inputs: []v1.Parameter{
		{Name: v1.ParameterAddonTemplate, Value: "t1"},
	}}}
	res, err = r.getInputAddonTemplates(context.Background(), job)
	assert.NoError(t, err)
	assert.Len(t, res, 1)
}

func TestAddonReconcileEntry(t *testing.T) {
	job := &v1.OpsJob{
		ObjectMeta: metav1.ObjectMeta{Name: "j1", Finalizers: []string{v1.OpsJobFinalizer}},
		Spec:       v1.OpsJobSpec{Type: v1.OpsJobAddonType},
	}
	r := &AddonJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t, job), allJobs: map[string]*AddonJob{}}
	_, err := r.Reconcile(context.Background(), ctrlruntime.Request{NamespacedName: types.NamespacedName{Name: "j1"}})
	assert.NoError(t, err)
}

func TestAddonHandleNodeViaSSH(t *testing.T) {
	sshClient, cleanup := startInMemorySSHServer(t)
	defer cleanup()

	adminNode := &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node1"}}
	adminNode.Spec.Cluster = ptrStr("c1")
	adminNode.Status.MachineStatus.Phase = v1.NodeReady
	faultCM := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "primus-safe-fault", Namespace: "primus-safe"},
		Data:       map[string]string{"addon": `{"id":"501","toggle":"on","action":"taint"}`},
	}
	base := newBaseWithObjs(t, adminNode, faultCM)
	r := &AddonJobReconciler{
		OpsJobBaseReconciler: base,
		allJobs: map[string]*AddonJob{
			"j1": {
				nodes:          map[string]AddonJobPhase{"node1": {Phase: v1.OpsJobPending}},
				addonTemplates: []*v1.AddonTemplate{{ObjectMeta: metav1.ObjectMeta{Name: "t1"}, Spec: v1.AddonTemplateSpec{Action: "ZWNobyBoaQ=="}}},
				batchCount:     1,
			},
		},
	}
	job := &v1.OpsJob{ObjectMeta: metav1.ObjectMeta{Name: "j1"}, Spec: v1.OpsJobSpec{Type: v1.OpsJobAddonType}}

	patches := gomonkey.ApplyFunc(rmutils.GetSSHClient,
		func(_ context.Context, _ client.Client, _ *v1.Node) (*ssh.Client, error) {
			return dialOpsJobSSH()
		})
	defer patches.Reset()
	_ = sshClient

	ok, out, err := r.handleNode(context.Background(), job, "node1", sets.NewSet())
	assert.NoError(t, err)
	assert.True(t, ok)
	assert.Contains(t, out, "t1")
}

func TestAddonHandleNodesViaSSH(t *testing.T) {
	sshClient, cleanup := startInMemorySSHServer(t)
	defer cleanup()

	adminNode := &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node1"}}
	adminNode.Spec.Cluster = ptrStr("c1")
	adminNode.Status.MachineStatus.Phase = v1.NodeReady
	faultCM := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "primus-safe-fault", Namespace: "primus-safe"},
		Data:       map[string]string{"addon": `{"id":"501","toggle":"on","action":"taint"}`},
	}
	base := newBaseWithObjs(t, adminNode, faultCM)
	r := &AddonJobReconciler{
		OpsJobBaseReconciler: base,
		allJobs: map[string]*AddonJob{
			"j1": {
				nodes:          map[string]AddonJobPhase{"node1": {Phase: v1.OpsJobPending}},
				addonTemplates: []*v1.AddonTemplate{{ObjectMeta: metav1.ObjectMeta{Name: "t1"}, Spec: v1.AddonTemplateSpec{Action: "ZWNobyBoaQ=="}}},
				batchCount:     1,
				maxFailCount:   1,
			},
		},
	}
	job := &v1.OpsJob{ObjectMeta: metav1.ObjectMeta{Name: "j1"}, Spec: v1.OpsJobSpec{Type: v1.OpsJobAddonType}}

	patches := gomonkey.ApplyFunc(rmutils.GetSSHClient,
		func(_ context.Context, _ client.Client, _ *v1.Node) (*ssh.Client, error) {
			return dialOpsJobSSH()
		})
	defer patches.Reset()
	_ = sshClient

	err := r.handleNodes(context.Background(), job, []string{"node1"})
	assert.NoError(t, err)
	// Node should be marked succeeded after handling.
	phase, _ := r.getJobPhase("j1")
	assert.Equal(t, v1.OpsJobSucceeded, phase)
}

func TestAddonHandlePending(t *testing.T) {
	node := &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node1"}}
	node.Spec.Cluster = ptrStr("c1")
	node.Status.MachineStatus.Phase = v1.NodeReady
	tmpl := &v1.AddonTemplate{ObjectMeta: metav1.ObjectMeta{Name: "t1"}, Spec: v1.AddonTemplateSpec{Action: "ZWNobyBoaQ=="}}
	job := &v1.OpsJob{
		ObjectMeta: metav1.ObjectMeta{Name: "j1"},
		Spec: v1.OpsJobSpec{
			Type: v1.OpsJobAddonType,
			Inputs: []v1.Parameter{
				{Name: v1.ParameterNode, Value: "node1"},
				{Name: v1.ParameterAddonTemplate, Value: "t1"},
			},
		},
	}
	base := newBaseWithObjs(t, node, tmpl, job)
	r := &AddonJobReconciler{OpsJobBaseReconciler: base, allJobs: map[string]*AddonJob{}}
	// First handle: addJob + set pending->running, requeue.
	_, err := r.handle(context.Background(), job)
	assert.NoError(t, err)
	assert.Equal(t, v1.OpsJobRunning, job.Status.Phase)
	assert.NotNil(t, r.getJob("j1"))
}

func TestAddonHandleRunning(t *testing.T) {
	sshClient, cleanup := startInMemorySSHServer(t)
	defer cleanup()

	node := &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node1"}}
	node.Spec.Cluster = ptrStr("c1")
	node.Status.MachineStatus.Phase = v1.NodeReady
	faultCM := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "primus-safe-fault", Namespace: "primus-safe"},
		Data:       map[string]string{"addon": `{"id":"501","toggle":"on","action":"taint"}`},
	}
	job := &v1.OpsJob{
		ObjectMeta: metav1.ObjectMeta{Name: "j1"},
		Spec:       v1.OpsJobSpec{Type: v1.OpsJobAddonType},
		Status:     v1.OpsJobStatus{Phase: v1.OpsJobRunning, StartedAt: &metav1.Time{Time: time.Now()}},
	}
	base := newBaseWithObjs(t, node, faultCM, job)
	r := &AddonJobReconciler{
		OpsJobBaseReconciler: base,
		allJobs: map[string]*AddonJob{
			"j1": {
				nodes:          map[string]AddonJobPhase{"node1": {Phase: v1.OpsJobPending}},
				addonTemplates: []*v1.AddonTemplate{{ObjectMeta: metav1.ObjectMeta{Name: "t1"}, Spec: v1.AddonTemplateSpec{Action: "ZWNobyBoaQ=="}}},
				batchCount:     1,
				maxFailCount:   1,
			},
		},
	}
	patches := gomonkey.ApplyFunc(rmutils.GetSSHClient,
		func(_ context.Context, _ client.Client, _ *v1.Node) (*ssh.Client, error) {
			return dialOpsJobSSH()
		})
	defer patches.Reset()
	_ = sshClient

	res, err := r.handle(context.Background(), job)
	assert.NoError(t, err)
	assert.True(t, res.Requeue)
}

func TestAddonUpdateNodeTemplatePhase(t *testing.T) {
	node := &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node1"}}
	base := newBaseWithObjs(t, node)
	r := &AddonJobReconciler{OpsJobBaseReconciler: base, allJobs: map[string]*AddonJob{}}
	job := &v1.OpsJob{
		ObjectMeta: metav1.ObjectMeta{Name: "j1"},
		Spec:       v1.OpsJobSpec{Inputs: []v1.Parameter{{Name: v1.ParameterNodeTemplate, Value: "tmpl1"}}},
	}
	// NodeTemplate param present -> sets annotation + patches node.
	err := r.updateNodeTemplatePhase(context.Background(), job, node, true)
	assert.NoError(t, err)
}

func TestAddonUpdateNodeTemplatePhaseNoParam(t *testing.T) {
	node := &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node1"}}
	r := &AddonJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t, node), allJobs: map[string]*AddonJob{}}
	// No NodeTemplate param -> no-op.
	assert.NoError(t, r.updateNodeTemplatePhase(context.Background(), &v1.OpsJob{}, node, true))
}

func TestAddonGetJobRemoveJob(t *testing.T) {
	r := &AddonJobReconciler{
		OpsJobBaseReconciler: newBaseWithObjs(t),
		allJobs:              map[string]*AddonJob{},
	}
	assert.Nil(t, r.getJob("j1"))
	r.allJobs["j1"] = &AddonJob{nodes: map[string]AddonJobPhase{}}
	assert.NotNil(t, r.getJob("j1"))
	assert.NoError(t, r.removeJob(context.Background(), &v1.OpsJob{ObjectMeta: metav1.ObjectMeta{Name: "j1"}}))
	assert.Nil(t, r.getJob("j1"))
}

func TestAddonSetNodePhase(t *testing.T) {
	r := &AddonJobReconciler{
		OpsJobBaseReconciler: newBaseWithObjs(t),
		allJobs: map[string]*AddonJob{
			"j1": {nodes: map[string]AddonJobPhase{
				"n1": {Phase: v1.OpsJobPending},
				"n2": {Phase: v1.OpsJobSucceeded},
			}},
		},
	}
	assert.False(t, r.setNodePhase("missing", "n1", v1.OpsJobRunning, ""))
	assert.False(t, r.setNodePhase("j1", "missing", v1.OpsJobRunning, ""))
	// Already finished node -> false.
	assert.False(t, r.setNodePhase("j1", "n2", v1.OpsJobRunning, ""))
	// Pending node -> true.
	assert.True(t, r.setNodePhase("j1", "n1", v1.OpsJobRunning, "go"))
}

func TestAddonGetNodesToProcess(t *testing.T) {
	r := &AddonJobReconciler{
		OpsJobBaseReconciler: newBaseWithObjs(t),
		allJobs: map[string]*AddonJob{
			"j1": {
				nodes: map[string]AddonJobPhase{
					"n1": {Phase: v1.OpsJobPending},
					"n2": {Phase: v1.OpsJobPending},
				},
				batchCount: 1,
			},
		},
	}
	job := &v1.OpsJob{ObjectMeta: metav1.ObjectMeta{Name: "j1"}}
	nodes := r.getNodesToProcess(job)
	assert.Len(t, nodes, 1)

	// Unknown job -> nil.
	assert.Nil(t, r.getNodesToProcess(&v1.OpsJob{ObjectMeta: metav1.ObjectMeta{Name: "missing"}}))
}

func TestAddonAddJobNoNodes(t *testing.T) {
	r := &AddonJobReconciler{
		OpsJobBaseReconciler: newBaseWithObjs(t),
		allJobs:              map[string]*AddonJob{},
	}
	job := &v1.OpsJob{ObjectMeta: metav1.ObjectMeta{Name: "j1"}}
	// No input nodes -> error.
	assert.Error(t, r.addJob(context.Background(), job))
}

// ---- Reconcile entry points ----

func TestDownloadReconcileEntry(t *testing.T) {
	job := downloadJob("j1")
	ws := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}}
	r := &DownloadJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t, job, ws)}
	_, err := r.Reconcile(context.Background(), ctrlruntime.Request{NamespacedName: types.NamespacedName{Name: "j1"}})
	assert.NoError(t, err)
}

func TestRebootReconcileEntry(t *testing.T) {
	job := rebootJob("j1", "n1")
	r := &RebootJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t, job)}
	_, err := r.Reconcile(context.Background(), ctrlruntime.Request{NamespacedName: types.NamespacedName{Name: "j1"}})
	assert.NoError(t, err)
}

func TestPreflightReconcileEntry(t *testing.T) {
	job := &v1.OpsJob{ObjectMeta: metav1.ObjectMeta{Name: "j1"}, Spec: v1.OpsJobSpec{Type: v1.OpsJobPreflightType}}
	r := &PreflightJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t, job)}
	_, err := r.Reconcile(context.Background(), ctrlruntime.Request{NamespacedName: types.NamespacedName{Name: "j1"}})
	assert.NoError(t, err)
}

func TestCDReconcileEntry(t *testing.T) {
	job := cdJob("j1")
	cluster := &v1.Cluster{ObjectMeta: metav1.ObjectMeta{
		Name:   "ctrl",
		Labels: map[string]string{v1.ClusterControlPlaneLabel: ""},
	}}
	r := &CDJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t, job, cluster)}
	_, err := r.Reconcile(context.Background(), ctrlruntime.Request{NamespacedName: types.NamespacedName{Name: "j1"}})
	assert.NoError(t, err)
}

func TestEvalReconcileEntry(t *testing.T) {
	job := evalJob("j1")
	r := &EvaluationJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t, job)}
	_, err := r.Reconcile(context.Background(), ctrlruntime.Request{NamespacedName: types.NamespacedName{Name: "j1"}})
	assert.NoError(t, err)
}

func TestCDHandleWorkloadEventImpl(t *testing.T) {
	job := newTestOpsJob("j1")
	r := &CDJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t, job)}
	wl := &v1.Workload{
		ObjectMeta: metav1.ObjectMeta{Name: "wl1", Labels: map[string]string{v1.OpsJobIdLabel: "j1"}},
		Status:     v1.WorkloadStatus{Phase: v1.WorkloadRunning},
	}
	r.handleWorkloadEventImpl(context.Background(), wl)
	updated := &v1.OpsJob{}
	assert.NoError(t, r.Get(context.Background(), client.ObjectKey{Name: "j1"}, updated))
}

func TestIsCDWorkloadAndEvalReconcileEnd(t *testing.T) {
	// Ended job -> observe returns quit, Reconcile completes without creating workload.
	job := cdJob("j1")
	job.Status.FinishedAt = &metav1.Time{Time: time.Now()}
	r := &CDJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t, job)}
	quit, err := r.observe(context.Background(), job)
	assert.NoError(t, err)
	assert.True(t, quit)
}

func TestBaseHandleWorkloadEventImpl(t *testing.T) {
	job := newTestOpsJob("j1")
	r := newBaseWithObjs(t, job)
	wl := &v1.Workload{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "wl1",
			Labels: map[string]string{v1.OpsJobIdLabel: "j1"},
		},
		Status: v1.WorkloadStatus{Phase: v1.WorkloadRunning},
	}
	// Running workload -> sets job phase running. Should not panic.
	r.handleWorkloadEventImpl(context.Background(), wl)
	updated := &v1.OpsJob{}
	assert.NoError(t, r.Get(context.Background(), client.ObjectKey{Name: "j1"}, updated))
}

// ---- exportimage controller helpers ----

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

// ---- dumplog controller helpers ----

func TestBuildLogName(t *testing.T) {
	assert.Equal(t, "wl1.log", buildLogName("wl1"))
}

// ---- prewarm controller helpers ----

func TestBuildJobOutputs(t *testing.T) {
	r := &PrewarmJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t)}
	outputs := r.buildJobOutputs("done", "msg", 2, 4)
	assert.Len(t, outputs, 5)
	var progress string
	for _, o := range outputs {
		if o.Name == "prewarm_progress" {
			progress = o.Value
		}
	}
	assert.Equal(t, "50%", progress)
}

// ---- job ttl controller ----

func TestJobTTLReconcileNotFound(t *testing.T) {
	r := &JobTTLController{Client: newBaseWithObjs(t).Client}
	res, err := r.Reconcile(context.Background(), ctrlruntime.Request{NamespacedName: types.NamespacedName{Name: "missing"}})
	assert.NoError(t, err)
	assert.Equal(t, ctrlruntime.Result{}, res)
}

func TestJobTTLReconcileNotEnded(t *testing.T) {
	job := &v1.OpsJob{ObjectMeta: metav1.ObjectMeta{Name: "j1"}, Spec: v1.OpsJobSpec{TTLSecondsAfterFinished: 10}}
	r := &JobTTLController{Client: newBaseWithObjs(t, job).Client}
	_, err := r.Reconcile(context.Background(), ctrlruntime.Request{NamespacedName: types.NamespacedName{Name: "j1"}})
	assert.NoError(t, err)
}

func TestJobTTLDeleteExpired(t *testing.T) {
	job := &v1.OpsJob{
		ObjectMeta: metav1.ObjectMeta{Name: "j1"},
		Spec:       v1.OpsJobSpec{TTLSecondsAfterFinished: 1},
		Status:     v1.OpsJobStatus{FinishedAt: &metav1.Time{Time: time.Now().Add(-time.Hour)}},
	}
	r := &JobTTLController{Client: newBaseWithObjs(t, job).Client}
	res, err := r.deleteExpiredJob(context.Background(), job)
	assert.NoError(t, err)
	assert.Equal(t, ctrlruntime.Result{}, res)
	// Job should be deleted.
	err = r.Get(context.Background(), client.ObjectKey{Name: "j1"}, &v1.OpsJob{})
	assert.Error(t, err)
}

func TestJobTTLDeleteNotYetExpired(t *testing.T) {
	job := &v1.OpsJob{
		ObjectMeta: metav1.ObjectMeta{Name: "j1"},
		Spec:       v1.OpsJobSpec{TTLSecondsAfterFinished: 3600},
		Status:     v1.OpsJobStatus{FinishedAt: &metav1.Time{Time: time.Now()}},
	}
	r := &JobTTLController{Client: newBaseWithObjs(t, job).Client}
	res, err := r.deleteExpiredJob(context.Background(), job)
	assert.NoError(t, err)
	assert.True(t, res.RequeueAfter > 0)
}

func TestJobTTLRelevantChangePredicate(t *testing.T) {
	r := &JobTTLController{Client: newBaseWithObjs(t).Client}
	p := r.relevantChangePredicate()
	oldJob := &v1.OpsJob{}
	newJob := &v1.OpsJob{
		Spec:   v1.OpsJobSpec{TTLSecondsAfterFinished: 10},
		Status: v1.OpsJobStatus{FinishedAt: &metav1.Time{Time: time.Now()}},
	}
	assert.True(t, p.Update(event.UpdateEvent{ObjectOld: oldJob, ObjectNew: newJob}))
	assert.False(t, p.Update(event.UpdateEvent{ObjectOld: oldJob, ObjectNew: oldJob}))
}
