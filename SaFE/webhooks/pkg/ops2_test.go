/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package webhooks

import (
	"context"
	"testing"

	"gotest.tools/assert"
	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

// runningOpsJob builds a non-ended ops job with cluster and type labels.
func runningOpsJob(name, cluster string, jobType v1.OpsJobType, inputs []v1.Parameter) *v1.OpsJob {
	job := &v1.OpsJob{
		ObjectMeta: metav1.ObjectMeta{Name: name, Labels: map[string]string{
			v1.ClusterIdLabel:   cluster,
			v1.OpsJobTypeLabel:  string(jobType),
		}},
		Spec: v1.OpsJobSpec{Type: jobType, Inputs: inputs},
	}
	return job
}

// TestOpsJobValidateNodeDuplicatedConflict covers duplicate node detection across jobs.
func TestOpsJobValidateNodeDuplicatedConflict(t *testing.T) {
	scheme := newScheme(t)
	other := runningOpsJob("other", "cluster1", v1.OpsJobPreflightType,
		[]v1.Parameter{{Name: v1.ParameterNode, Value: "n1"}})
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(other).Build()
	v := &OpsJobValidator{Client: k8sClient}
	job := &v1.OpsJob{
		ObjectMeta: metav1.ObjectMeta{Name: "job1", Labels: map[string]string{v1.ClusterIdLabel: "cluster1"}},
		Spec:       v1.OpsJobSpec{Type: v1.OpsJobPreflightType, Inputs: []v1.Parameter{{Name: v1.ParameterNode, Value: "n1"}}},
	}
	assert.Assert(t, v.validateNodeDuplicated(context.Background(), job) != nil)
}

// TestOpsJobValidatePreflightBranches covers preflight image/entrypoint error branches.
func TestOpsJobValidatePreflightBranches(t *testing.T) {
	scheme := newScheme(t)
	node := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "n1", Labels: map[string]string{v1.ClusterIdLabel: "cluster1"}},
		Spec:       v1.NodeSpec{NodeFlavor: &corev1.ObjectReference{Name: "flavor1"}},
	}
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(node).Build()
	v := &OpsJobValidator{Client: k8sClient}

	mk := func() *v1.OpsJob {
		return &v1.OpsJob{Spec: v1.OpsJobSpec{
			Type:     v1.OpsJobPreflightType,
			Inputs:   []v1.Parameter{{Name: v1.ParameterNode, Value: "n1"}},
			Resource: &v1.WorkloadResource{CPU: "1", Memory: "2Gi", EphemeralStorage: "3Gi", Replica: 1},
		}}
	}
	// missing image
	assert.Assert(t, v.validatePreflight(context.Background(), mk()) != nil)
	// missing entrypoint
	withImage := mk()
	withImage.Spec.Image = pointer.String("img")
	assert.Assert(t, v.validatePreflight(context.Background(), withImage) != nil)
}

// TestOpsJobValidateDumplingConflict covers duplicate dumplog detection.
func TestOpsJobValidateDumplingConflict(t *testing.T) {
	scheme := newScheme(t)
	other := runningOpsJob("other", "cluster1", v1.OpsJobDumpLogType,
		[]v1.Parameter{{Name: v1.ParameterWorkload, Value: "w1"}})
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(other).Build()
	v := &OpsJobValidator{Client: k8sClient}
	job := &v1.OpsJob{
		ObjectMeta: metav1.ObjectMeta{Name: "job1", Labels: map[string]string{v1.ClusterIdLabel: "cluster1"}},
		Spec:       v1.OpsJobSpec{Type: v1.OpsJobDumpLogType, Inputs: []v1.Parameter{{Name: v1.ParameterWorkload, Value: "w1"}}},
	}
	assert.Assert(t, v.validateDumpling(context.Background(), job) != nil)
}

// TestOpsJobValidateDownloadConflict covers duplicate download detection.
func TestOpsJobValidateDownloadConflict(t *testing.T) {
	scheme := newScheme(t)
	other := runningOpsJob("other", "cluster1", v1.OpsJobDownloadType,
		[]v1.Parameter{{Name: v1.ParameterDestPath, Value: "/data"}})
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(other).Build()
	v := &OpsJobValidator{Client: k8sClient}
	job := &v1.OpsJob{
		ObjectMeta: metav1.ObjectMeta{Name: "job1", Labels: map[string]string{v1.ClusterIdLabel: "cluster1"}},
		Spec: v1.OpsJobSpec{Type: v1.OpsJobDownloadType, Inputs: []v1.Parameter{
			{Name: v1.ParameterEndpoint, Value: "http://x"},
			{Name: v1.ParameterDestPath, Value: "/data"},
			{Name: v1.ParameterSecret, Value: "secret"},
			{Name: v1.ParameterWorkspace, Value: "ws1"},
		}},
	}
	assert.Assert(t, v.validateDownload(context.Background(), job) != nil)
}

// TestOpsJobValidateImmutableFieldsBranches covers immutable field error branches.
func TestOpsJobValidateImmutableFieldsBranches(t *testing.T) {
	v := &OpsJobValidator{}
	oldJob := &v1.OpsJob{
		ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{v1.ClusterIdLabel: "cluster1"}},
		Spec:       v1.OpsJobSpec{Type: v1.OpsJobDownloadType, Inputs: []v1.Parameter{{Name: "a", Value: "b"}}},
	}
	clusterChanged := &v1.OpsJob{
		ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{v1.ClusterIdLabel: "cluster2"}},
		Spec:       v1.OpsJobSpec{Type: v1.OpsJobDownloadType, Inputs: []v1.Parameter{{Name: "a", Value: "b"}}},
	}
	assert.Assert(t, v.validateImmutableFields(clusterChanged, oldJob) != nil)

	inputsChanged := &v1.OpsJob{
		ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{v1.ClusterIdLabel: "cluster1"}},
		Spec:       v1.OpsJobSpec{Type: v1.OpsJobDownloadType, Inputs: []v1.Parameter{{Name: "x", Value: "y"}}},
	}
	assert.Assert(t, v.validateImmutableFields(inputsChanged, oldJob) != nil)
}

// TestOpsJobValidateNodesMismatch covers node cluster/flavor mismatch branches.
func TestOpsJobValidateNodesMismatch(t *testing.T) {
	scheme := newScheme(t)
	n1 := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "n1", Labels: map[string]string{v1.ClusterIdLabel: "cluster1"}},
		Spec:       v1.NodeSpec{NodeFlavor: &corev1.ObjectReference{Name: "f1"}},
	}
	n2 := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "n2", Labels: map[string]string{v1.ClusterIdLabel: "cluster2"}},
		Spec:       v1.NodeSpec{NodeFlavor: &corev1.ObjectReference{Name: "f2"}},
	}
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(n1, n2).Build()
	v := &OpsJobValidator{Client: k8sClient}
	job := &v1.OpsJob{Spec: v1.OpsJobSpec{Inputs: []v1.Parameter{
		{Name: v1.ParameterNode, Value: "n1"}, {Name: v1.ParameterNode, Value: "n2"},
	}}}
	assert.Assert(t, v.validateNodes(context.Background(), job) != nil)
}

// TestOpsJobMutatorHandleUpdate covers ops job mutator update (no-op) handler branch.
func TestOpsJobMutatorHandleUpdate(t *testing.T) {
	scheme := newScheme(t)
	m := &OpsJobMutator{Client: fake.NewClientBuilder().WithScheme(scheme).Build(), decoder: newDecoder(t)}
	job := opsJobWithDisplayName("job1", v1.OpsJobDownloadType)
	resp := m.Handle(context.Background(), newRequest(t, admissionv1.Update, job, nil))
	assert.Assert(t, resp.Allowed)

	resp = m.Handle(context.Background(), newRequest(t, admissionv1.Create, nil, nil))
	assert.Assert(t, !resp.Allowed)
}
