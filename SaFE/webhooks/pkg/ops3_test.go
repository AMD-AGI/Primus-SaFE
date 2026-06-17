/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package webhooks

import (
	"context"
	"testing"

	"gotest.tools/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

// TestOpsJobMutateOnCreationDestPathError covers download dest path generation error.
func TestOpsJobMutateOnCreationDestPathError(t *testing.T) {
	scheme := newScheme(t)
	ws := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}} // no volumes
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ws).Build()
	m := &OpsJobMutator{Client: c}
	job := &v1.OpsJob{
		ObjectMeta: metav1.ObjectMeta{Name: "job1"},
		Spec: v1.OpsJobSpec{Type: v1.OpsJobDownloadType, Inputs: []v1.Parameter{
			{Name: v1.ParameterDestPath, Value: "data/file"},
			{Name: v1.ParameterWorkspace, Value: "ws1"},
		}},
	}
	assert.Assert(t, m.mutateOnCreation(context.Background(), job) != nil)
}

// TestOpsJobGenerateAddonTemplatesMissing covers missing node template no-op.
func TestOpsJobGenerateAddonTemplatesMissing(t *testing.T) {
	scheme := newScheme(t)
	m := &OpsJobMutator{Client: fake.NewClientBuilder().WithScheme(scheme).Build()}
	job := &v1.OpsJob{Spec: v1.OpsJobSpec{Inputs: []v1.Parameter{
		{Name: v1.ParameterNodeTemplate, Value: "missing"},
	}}}
	m.generateAddonTemplates(context.Background(), job)
	assert.Assert(t, job.GetParameter(v1.ParameterAddonTemplate) == nil)
}

// TestOpsJobFilterUnhealthyNodesTaint covers tainted node filtering.
func TestOpsJobFilterUnhealthyNodesTaint(t *testing.T) {
	scheme := newScheme(t)
	tainted := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "n1"},
		Status: v1.NodeStatus{
			MachineStatus: v1.MachineStatus{Phase: v1.NodeReady},
			Taints:        []corev1.Taint{{Key: "custom.taint", Effect: corev1.TaintEffectNoSchedule}},
		},
	}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(tainted).Build()
	m := &OpsJobMutator{Client: c}
	job := &v1.OpsJob{Spec: v1.OpsJobSpec{
		Type:   v1.OpsJobPreflightType,
		Inputs: []v1.Parameter{{Name: v1.ParameterNode, Value: "n1"}},
	}}
	m.filterUnhealthyNodes(context.Background(), job)
	assert.Equal(t, len(job.GetParameters(v1.ParameterNode)), 0)
}

// TestOpsJobGenerateDestPathBranches covers dest path no-op branches.
func TestOpsJobGenerateDestPathBranches(t *testing.T) {
	scheme := newScheme(t)
	m := &OpsJobMutator{Client: fake.NewClientBuilder().WithScheme(scheme).Build()}

	// no dest param
	noDest := &v1.OpsJob{Spec: v1.OpsJobSpec{Type: v1.OpsJobDownloadType,
		Inputs: []v1.Parameter{{Name: v1.ParameterWorkspace, Value: "ws1"}}}}
	assert.NilError(t, m.generateDestPath(context.Background(), noDest))

	// no workspace param
	noWs := &v1.OpsJob{Spec: v1.OpsJobSpec{Type: v1.OpsJobDownloadType,
		Inputs: []v1.Parameter{{Name: v1.ParameterDestPath, Value: "data/f"}}}}
	assert.NilError(t, m.generateDestPath(context.Background(), noWs))

	// absolute path is trusted as-is
	ws := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ws).Build()
	m2 := &OpsJobMutator{Client: c}
	abs := &v1.OpsJob{Spec: v1.OpsJobSpec{Type: v1.OpsJobDownloadType, Inputs: []v1.Parameter{
		{Name: v1.ParameterDestPath, Value: "/abs/path"},
		{Name: v1.ParameterWorkspace, Value: "ws1"},
	}}}
	assert.NilError(t, m2.generateDestPath(context.Background(), abs))
}

// TestOpsJobValidateRequiredParamsAddonNode covers addon node-required and other branches.
func TestOpsJobValidateRequiredParamsAddonNode(t *testing.T) {
	scheme := newScheme(t)
	v := &OpsJobValidator{Client: fake.NewClientBuilder().WithScheme(scheme).Build()}

	// type empty + inputs empty
	empty := &v1.OpsJob{}
	v1.SetLabel(empty, v1.DisplayNameLabel, "my-job")
	assert.Assert(t, v.validateRequiredParams(context.Background(), empty) != nil)

	// addon without node param
	addon := &v1.OpsJob{Spec: v1.OpsJobSpec{Type: v1.OpsJobAddonType, Inputs: []v1.Parameter{{Name: "x", Value: "y"}}}}
	v1.SetLabel(addon, v1.DisplayNameLabel, "my-job")
	assert.Assert(t, v.validateRequiredParams(context.Background(), addon) != nil)
}

// TestOpsJobValidateNodesWorkspaceMismatch covers workspace/flavor mismatch branches.
func TestOpsJobValidateNodesWorkspaceMismatch(t *testing.T) {
	scheme := newScheme(t)
	node := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "n1", Labels: map[string]string{
			v1.ClusterIdLabel: "cluster1", v1.WorkspaceIdLabel: "wsOther",
		}},
		Spec: v1.NodeSpec{NodeFlavor: &corev1.ObjectReference{Name: "f1"}},
	}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(node).Build()
	v := &OpsJobValidator{Client: c}
	job := &v1.OpsJob{
		ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{v1.WorkspaceIdLabel: "ws1"}},
		Spec:       v1.OpsJobSpec{Inputs: []v1.Parameter{{Name: v1.ParameterNode, Value: "n1"}}},
	}
	assert.Assert(t, v.validateNodes(context.Background(), job) != nil)

	// flavor mismatch across two nodes in same cluster
	n1 := &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "a", Labels: map[string]string{v1.ClusterIdLabel: "cluster1"}},
		Spec: v1.NodeSpec{NodeFlavor: &corev1.ObjectReference{Name: "f1"}}}
	n2 := &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "b", Labels: map[string]string{v1.ClusterIdLabel: "cluster1"}},
		Spec: v1.NodeSpec{NodeFlavor: &corev1.ObjectReference{Name: "f2"}}}
	c2 := fake.NewClientBuilder().WithScheme(scheme).WithObjects(n1, n2).Build()
	v2 := &OpsJobValidator{Client: c2}
	job2 := &v1.OpsJob{Spec: v1.OpsJobSpec{Inputs: []v1.Parameter{
		{Name: v1.ParameterNode, Value: "a"}, {Name: v1.ParameterNode, Value: "b"},
	}}}
	assert.Assert(t, v2.validateNodes(context.Background(), job2) != nil)
}

// TestOpsJobListRelatedRunningJobsFilter covers the ended-job filter branch.
func TestOpsJobListRelatedRunningJobsFilter(t *testing.T) {
	scheme := newScheme(t)
	now := metav1.Now()
	ended := runningOpsJob("ended", "cluster1", v1.OpsJobPreflightType, nil)
	ended.Status.FinishedAt = &now
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ended).Build()
	v := &OpsJobValidator{Client: c}
	jobs, err := v.listRelatedRunningJobs(context.Background(), "cluster1", []string{string(v1.OpsJobPreflightType)})
	assert.NilError(t, err)
	assert.Equal(t, len(jobs), 0)
}
