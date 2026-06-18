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
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
)

// dynamoWorkload builds a DynamoDeployment workload with the given roles annotation.
func dynamoWorkload(kindAnno, fw, kv, roles string, resourceCount int) *v1.Workload {
	res := make([]v1.WorkloadResource, resourceCount)
	for i := range res {
		res[i] = wlResource()
	}
	w := &v1.Workload{Spec: v1.WorkloadSpec{
		GroupVersionKind: v1.GroupVersionKind{Kind: kindAnno},
		Resources:        res,
	}}
	if kindAnno == common.DynamoDeploymentKind {
		v1.SetAnnotation(w, v1.DynamoBackendFrameworkAnnotation, fw)
		v1.SetAnnotation(w, v1.DynamoKVTransferBackendAnnotation, kv)
		v1.SetAnnotation(w, v1.DynamoServiceRolesAnnotation, roles)
	} else {
		v1.SetAnnotation(w, v1.OptimusBackendFrameworkAnnotation, fw)
		v1.SetAnnotation(w, v1.OptimusKVTransferBackendAnnotation, kv)
		v1.SetAnnotation(w, v1.OptimusServiceRolesAnnotation, roles)
	}
	return w
}

// TestWorkloadRequiredParamsBranches covers required-param error branches.
func TestWorkloadRequiredParamsBranches(t *testing.T) {
	v := &WorkloadValidator{}
	w := &v1.Workload{Spec: v1.WorkloadSpec{
		Workspace:        "ws1",
		GroupVersionKind: v1.GroupVersionKind{Kind: common.DeploymentKind, Version: "v1"},
		Resources:        []v1.WorkloadResource{{Replica: 0}},
	}}
	v1.SetLabel(w, v1.ClusterIdLabel, "cluster1")
	v1.SetLabel(w, v1.DisplayNameLabel, "Bad_Name")
	assert.Assert(t, v.validateRequiredParams(w) != nil)
}

// TestWorkloadServiceBranches covers service validation port error branches.
func TestWorkloadServiceBranches(t *testing.T) {
	scheme := newScheme(t)
	v := &WorkloadValidator{Client: fake.NewClientBuilder().WithScheme(scheme).Build()}

	cases := []*v1.Service{
		{Port: 0, TargetPort: 80, Protocol: corev1.ProtocolTCP, ServiceType: corev1.ServiceTypeClusterIP},
		{Port: 80, TargetPort: 0, Protocol: corev1.ProtocolTCP, ServiceType: corev1.ServiceTypeClusterIP},
		{Port: 80, TargetPort: 80, NodePort: 70000, Protocol: corev1.ProtocolTCP, ServiceType: corev1.ServiceTypeNodePort},
	}
	for _, svc := range cases {
		w := &v1.Workload{Spec: v1.WorkloadSpec{Service: svc}}
		assert.Assert(t, v.validateService(context.Background(), w) != nil)
	}
}

// TestWorkloadHealthCheckBranches covers health check port error branches.
func TestWorkloadHealthCheckBranches(t *testing.T) {
	v := &WorkloadValidator{}
	badLivePort := &v1.Workload{Spec: v1.WorkloadSpec{Liveness: &v1.HealthCheck{Path: "/h", Port: 0}}}
	assert.Assert(t, v.validateHealthCheck(badLivePort) != nil)
	badReadyPort := &v1.Workload{Spec: v1.WorkloadSpec{Readiness: &v1.HealthCheck{Path: "/h", Port: 0}}}
	assert.Assert(t, v.validateHealthCheck(badReadyPort) != nil)
	badReadyPath := &v1.Workload{Spec: v1.WorkloadSpec{Readiness: &v1.HealthCheck{Port: 80}}}
	assert.Assert(t, v.validateHealthCheck(badReadyPath) != nil)
}

// TestWorkloadImmutableFieldsBranches covers immutable field error branches.
func TestWorkloadImmutableFieldsBranches(t *testing.T) {
	v := &WorkloadValidator{}
	base := validWorkload()

	gvkChanged := validWorkload()
	gvkChanged.Spec.Version = "v2"
	assert.Assert(t, v.validateImmutableFields(gvkChanged, base) != nil)

	ownerChanged := validWorkload()
	v1.SetLabel(ownerChanged, v1.OwnerLabel, "other")
	assert.Assert(t, v.validateImmutableFields(ownerChanged, base) != nil)
}

// TestWorkloadDynamoBranches covers dynamo deployment validation error branches.
func TestWorkloadDynamoBranches(t *testing.T) {
	v := &WorkloadValidator{}
	k := common.DynamoDeploymentKind
	// too many resources
	assert.Assert(t, v.validateDynamoDeployment(dynamoWorkload(k, "vllm", "nixl", "frontend,worker,prefill,decode,planner,epp", 6)) != nil)
	// unknown framework + kv
	assert.Assert(t, v.validateDynamoDeployment(dynamoWorkload(k, "bad", "bad", "frontend", 1)) != nil)
	// roles length mismatch
	assert.Assert(t, v.validateDynamoDeployment(dynamoWorkload(k, "vllm", "nixl", "frontend,worker", 1)) != nil)
	// unknown role
	assert.Assert(t, v.validateDynamoDeployment(dynamoWorkload(k, "vllm", "nixl", "weird", 1)) != nil)
	// worker mixed with prefill + prefill!=decode
	assert.Assert(t, v.validateDynamoDeployment(dynamoWorkload(k, "vllm", "nixl", "frontend,worker,prefill", 3)) != nil)
	// multinode undeclared role
	w := dynamoWorkload(k, "vllm", "nixl", "frontend,worker", 2)
	v1.SetAnnotation(w, v1.DynamoMultinodeRolesAnnotation, "decode")
	assert.Assert(t, v.validateDynamoDeployment(w) != nil)
}

// TestWorkloadOptimusBranches covers optimus deployment validation error branches.
func TestWorkloadOptimusBranches(t *testing.T) {
	v := &WorkloadValidator{}
	k := common.OptimusDeploymentKind
	assert.Assert(t, v.validateOptimusDeployment(dynamoWorkload(k, "vllm", "nixl", "frontend,worker,prefill,decode,planner,epp", 6)) != nil)
	assert.Assert(t, v.validateOptimusDeployment(dynamoWorkload(k, "bad", "bad", "frontend", 1)) != nil)
	assert.Assert(t, v.validateOptimusDeployment(dynamoWorkload(k, "vllm", "nixl", "frontend,worker", 1)) != nil)
	assert.Assert(t, v.validateOptimusDeployment(dynamoWorkload(k, "vllm", "nixl", "weird", 1)) != nil)
	assert.Assert(t, v.validateOptimusDeployment(dynamoWorkload(k, "vllm", "nixl", "frontend,worker,prefill", 3)) != nil)
}

// TestWorkloadSpecChangedBranches covers spec change detection branches.
func TestWorkloadSpecChangedBranches(t *testing.T) {
	v := &WorkloadValidator{}
	mk := func() *v1.Workload {
		w := &v1.Workload{Spec: v1.WorkloadSpec{
			GroupVersionKind: v1.GroupVersionKind{Kind: common.PytorchJobKind},
			Resources:        []v1.WorkloadResource{wlResource()},
			Service:          &v1.Service{Port: 80},
		}}
		v1.SetAnnotation(w, v1.WorkloadDispatchedAnnotation, v1.TrueStr)
		return w
	}
	// service removed
	oldW := mk()
	newW := mk()
	newW.Spec.Service = nil
	assert.Assert(t, v.validateSpecChanged(newW, oldW) != nil)

	// service added
	oldW2 := mk()
	oldW2.Spec.Service = nil
	assert.Assert(t, v.validateSpecChanged(mk(), oldW2) != nil)

	// nil args
	assert.NilError(t, v.validateSpecChanged(nil, oldW))
}

// TestWorkloadOwnerWorkloadBranches covers owner workload validation branches.
func TestWorkloadOwnerWorkloadBranches(t *testing.T) {
	scheme := newScheme(t)
	owner := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "owner"}, Spec: v1.WorkloadSpec{Workspace: "wsX"}}
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(owner).Build()
	v := &WorkloadValidator{Client: k8sClient}

	// owner in different workspace
	w := validWorkload()
	v1.SetLabel(w, v1.OwnerLabel, "owner")
	assert.Assert(t, v.validateOwnerWorkload(context.Background(), w) != nil)

	// owner not found is tolerated: admission must not block when the owner
	// workload is not yet persisted (issue #588).
	w2 := validWorkload()
	v1.SetLabel(w2, v1.OwnerLabel, "missing")
	assert.NilError(t, v.validateOwnerWorkload(context.Background(), w2))

	// self-referential owner is rejected
	w3 := validWorkload()
	v1.SetLabel(w3, v1.OwnerLabel, w3.Name)
	assert.Assert(t, v.validateOwnerWorkload(context.Background(), w3) != nil)
}

// TestWorkloadAuthoringBranch covers authoring multi-node rejection.
func TestWorkloadAuthoringBranch(t *testing.T) {
	v := &WorkloadValidator{}
	w := &v1.Workload{Spec: v1.WorkloadSpec{
		CustomerLabels: map[string]string{common.SpecifiedNodes: "n1 n2"},
	}}
	v1.SetAnnotation(w, v1.NodesAffinityAnnotation, common.NodesAffinityRequired)
	assert.Assert(t, v.validateAuthoring(w) != nil)
}

// TestWorkloadSandboxBranch covers sandbox display name length rejection.
func TestWorkloadSandboxBranch(t *testing.T) {
	v := &WorkloadValidator{}
	w := &v1.Workload{}
	long := make([]byte, 300)
	for i := range long {
		long[i] = 'a'
	}
	v1.SetLabel(w, v1.DisplayNameLabel, string(long))
	assert.Assert(t, v.validateSandbox(w) != nil)
}

// TestOpsJobValidateNodesBranches covers ops job node validation error branches.
func TestOpsJobValidateNodesBranches(t *testing.T) {
	scheme := newScheme(t)
	// node missing cluster id
	noCluster := &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n1"},
		Spec: v1.NodeSpec{NodeFlavor: &corev1.ObjectReference{Name: "f1"}}}
	c1 := fake.NewClientBuilder().WithScheme(scheme).WithObjects(noCluster).Build()
	v1v := &OpsJobValidator{Client: c1}
	job := &v1.OpsJob{Spec: v1.OpsJobSpec{Inputs: []v1.Parameter{{Name: v1.ParameterNode, Value: "n1"}}}}
	assert.Assert(t, v1v.validateNodes(context.Background(), job) != nil)

	// node missing flavor
	noFlavor := &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n2",
		Labels: map[string]string{v1.ClusterIdLabel: "cluster1"}}}
	c2 := fake.NewClientBuilder().WithScheme(scheme).WithObjects(noFlavor).Build()
	v2 := &OpsJobValidator{Client: c2}
	job2 := &v1.OpsJob{Spec: v1.OpsJobSpec{Inputs: []v1.Parameter{{Name: v1.ParameterNode, Value: "n2"}}}}
	assert.Assert(t, v2.validateNodes(context.Background(), job2) != nil)
}

// TestOpsJobValidateAddonBranches covers addon validation error branches.
func TestOpsJobValidateAddonBranches(t *testing.T) {
	scheme := newScheme(t)
	v := &OpsJobValidator{Client: fake.NewClientBuilder().WithScheme(scheme).Build()}

	emptyScript := &v1.OpsJob{Spec: v1.OpsJobSpec{Type: v1.OpsJobAddonType,
		Inputs: []v1.Parameter{{Name: v1.ParameterScript, Value: ""}}}}
	assert.Assert(t, v.validateAddon(context.Background(), emptyScript) != nil)

	notBase64 := &v1.OpsJob{Spec: v1.OpsJobSpec{Type: v1.OpsJobAddonType,
		Inputs: []v1.Parameter{{Name: v1.ParameterScript, Value: "not base64!!"}}}}
	assert.Assert(t, v.validateAddon(context.Background(), notBase64) != nil)

	// helm addon template not allowed
	helm := &v1.AddonTemplate{ObjectMeta: metav1.ObjectMeta{Name: "helm1"},
		Spec: v1.AddonTemplateSpec{Type: v1.AddonTemplateHelm}}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(helm).Build()
	v2 := &OpsJobValidator{Client: c}
	helmJob := &v1.OpsJob{Spec: v1.OpsJobSpec{Type: v1.OpsJobAddonType,
		Inputs: []v1.Parameter{{Name: v1.ParameterAddonTemplate, Value: "helm1"}}}}
	assert.Assert(t, v2.validateAddon(context.Background(), helmJob) != nil)
}

// TestOpsJobValidateDownloadBranches covers download validation error branches.
func TestOpsJobValidateDownloadBranches(t *testing.T) {
	scheme := newScheme(t)
	v := &OpsJobValidator{Client: fake.NewClientBuilder().WithScheme(scheme).Build()}
	// missing dest path
	job := &v1.OpsJob{Spec: v1.OpsJobSpec{Type: v1.OpsJobDownloadType,
		Inputs: []v1.Parameter{{Name: v1.ParameterEndpoint, Value: "http://x"}}}}
	assert.Assert(t, v.validateDownload(context.Background(), job) != nil)
}

// TestOpsJobValidateRequiredParamsBranches covers ops job required-param error branches.
func TestOpsJobValidateRequiredParamsBranches(t *testing.T) {
	scheme := newScheme(t)
	v := &OpsJobValidator{Client: fake.NewClientBuilder().WithScheme(scheme).Build()}
	// addon type requires node param
	job := &v1.OpsJob{Spec: v1.OpsJobSpec{Type: v1.OpsJobAddonType, Inputs: []v1.Parameter{{Name: "x", Value: "y"}}}}
	v1.SetLabel(job, v1.DisplayNameLabel, "my-job")
	assert.Assert(t, v.validateRequiredParams(context.Background(), job) != nil)
}
