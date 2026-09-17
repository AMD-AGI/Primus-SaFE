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

func TestAddonExecuteCommandViaSSH(t *testing.T) {
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

func TestAddonAddFailedNodeCondition(t *testing.T) {
	job := &v1.OpsJob{ObjectMeta: metav1.ObjectMeta{Name: "j1"}}
	r := &AddonJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t, job), allJobs: map[string]*AddonJob{}}
	r.addFailedNodeCondition(context.Background(), "j1", "node1", "boom")
	updated := &v1.OpsJob{}
	assert.NoError(t, r.Get(context.Background(), types.NamespacedName{Name: "j1"}, updated))
}

func TestAddonHandleWorkloadEvent(t *testing.T) {
	job := &v1.OpsJob{
		ObjectMeta: metav1.ObjectMeta{Name: "j1", Labels: map[string]string{
			v1.ClusterIdLabel:  "c1",
			v1.OpsJobTypeLabel: string(v1.OpsJobAddonType),
		}},
	}
	r := &AddonJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t, job), allJobs: map[string]*AddonJob{}}
	h := r.handleWorkloadEvent().(interface {
		Create(context.Context, event.CreateEvent, v1.RequestWorkQueue)
		Update(context.Context, event.UpdateEvent, v1.RequestWorkQueue)
	})
	q := opsWorkQueue()
	defer q.ShutDown()
	wl := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "wl1", Labels: map[string]string{v1.ClusterIdLabel: "c1"}}}
	h.Create(context.Background(), event.CreateEvent{Object: wl}, q)
	h.Update(context.Background(), event.UpdateEvent{ObjectOld: wl, ObjectNew: wl.DeepCopy()}, q)
}

func TestAddonHandleNodeEvent(t *testing.T) {
	job := &v1.OpsJob{
		ObjectMeta: metav1.ObjectMeta{Name: "j1", Labels: map[string]string{
			v1.ClusterIdLabel:  "c1",
			v1.OpsJobTypeLabel: string(v1.OpsJobAddonType),
		}},
	}
	r := &AddonJobReconciler{
		OpsJobBaseReconciler: newBaseWithObjs(t, job),
		allJobs: map[string]*AddonJob{
			"j1": {nodes: map[string]AddonJobPhase{"n1": {Phase: v1.OpsJobRunning}}, maxFailCount: 1},
		},
	}
	h := r.handleNodeEvent().(interface {
		Update(context.Context, event.UpdateEvent, v1.RequestWorkQueue)
	})
	q := opsWorkQueue()
	defer q.ShutDown()
	oldNode := &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n1"}}
	oldNode.Spec.Cluster = ptrStr("c1")
	newNode := oldNode.DeepCopy()
	newNode.Spec.Cluster = nil
	// Node unmanaged -> handleNodeRemovedEvent path.
	h.Update(context.Background(), event.UpdateEvent{ObjectOld: oldNode, ObjectNew: newNode}, q)
}

func TestAddonCleanupJobRelatedInfo(t *testing.T) {
	job := &v1.OpsJob{ObjectMeta: metav1.ObjectMeta{Name: "j1"}}
	r := &AddonJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t, job), allJobs: map[string]*AddonJob{}}
	assert.NoError(t, r.cleanupJobRelatedInfo(context.Background(), job))
}
