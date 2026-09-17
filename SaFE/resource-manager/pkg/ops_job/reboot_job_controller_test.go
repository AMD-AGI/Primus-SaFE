/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package ops_job

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/agiledragon/gomonkey/v2"
	"github.com/stretchr/testify/assert"
	"golang.org/x/crypto/ssh"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	rmutils "github.com/AMD-AIG-AIMA/SAFE/resource-manager/pkg/utils"
)

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

func TestRebootExecRebootTreatsMissingExitStatusAsSuccess(t *testing.T) {
	sshClient, cleanup := startInMemorySSHServerWithoutExitStatus(t)
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

	updated := &v1.OpsJob{}
	assert.NoError(t, r.Get(context.Background(), client.ObjectKey{Name: "j1"}, updated))
	assert.Equal(t, []v1.Parameter{{Name: v1.ParameterNode, Value: "n1"}}, updated.Status.Outputs)
}

func TestRebootExpectedDisconnectMatching(t *testing.T) {
	assert.True(t, isExpectedRebootDisconnect(io.EOF))
	assert.True(t, isExpectedRebootDisconnect(errors.New("read tcp: use of closed network connection")))
	assert.False(t, isExpectedRebootDisconnect(errors.New("geo_failure")))
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

func TestRebootReconcileEntry(t *testing.T) {
	job := rebootJob("j1", "n1")
	r := &RebootJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t, job)}
	_, err := r.Reconcile(context.Background(), ctrlruntime.Request{NamespacedName: types.NamespacedName{Name: "j1"}})
	assert.NoError(t, err)
}
