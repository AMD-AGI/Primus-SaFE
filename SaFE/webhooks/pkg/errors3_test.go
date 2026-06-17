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
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
)

// TestWorkloadMutatorHandleBranches covers mutator decode/delete/deletion-timestamp branches.
func TestWorkloadMutatorHandleBranches(t *testing.T) {
	scheme := newScheme(t)
	m := &WorkloadMutator{Client: fake.NewClientBuilder().WithScheme(scheme).Build(), decoder: newDecoder(t)}

	// decode error
	resp := m.Handle(context.Background(), newRequest(t, admissionv1.Create, nil, nil))
	assert.Assert(t, !resp.Allowed)

	// deletion timestamp -> allowed without mutation
	now := metav1.Now()
	deleting := validWorkload()
	deleting.DeletionTimestamp = &now
	deleting.Finalizers = []string{"x"}
	resp = m.Handle(context.Background(), newRequest(t, admissionv1.Create, deleting, nil))
	assert.Assert(t, resp.Allowed)
}

// TestWorkloadValidatorHandleBranches covers validator decode/deletion-timestamp branches.
func TestWorkloadValidatorHandleBranches(t *testing.T) {
	scheme := newScheme(t)
	v := &WorkloadValidator{Client: fake.NewClientBuilder().WithScheme(scheme).Build(), decoder: newDecoder(t)}

	resp := v.Handle(context.Background(), newRequest(t, admissionv1.Create, nil, nil))
	assert.Assert(t, !resp.Allowed)

	now := metav1.Now()
	deleting := validWorkload()
	deleting.DeletionTimestamp = &now
	deleting.Finalizers = []string{"x"}
	resp = v.Handle(context.Background(), newRequest(t, admissionv1.Update, deleting, deleting))
	assert.Assert(t, resp.Allowed)
}

// TestWorkloadMutateMetaPreempt covers preempt annotation and scale-runner labeling.
func TestWorkloadMutateMetaPreempt(t *testing.T) {
	scheme := newScheme(t)
	m := &WorkloadMutator{Client: fake.NewClientBuilder().WithScheme(scheme).Build()}
	w := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "w1"}, Spec: v1.WorkloadSpec{
		Workspace:        "ws1",
		GroupVersionKind: v1.GroupVersionKind{Kind: common.PytorchJobKind, Version: "v1"},
		Env:              map[string]string{common.ScaleRunnerID: "r1"},
	}}
	ws := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}, Spec: v1.WorkspaceSpec{
		Cluster: "cluster1", EnablePreempt: true,
	}}
	m.mutateMeta(context.Background(), w, ws)
	assert.Equal(t, v1.GetAnnotation(w, v1.WorkloadEnablePreemptAnnotation), v1.TrueStr)
	assert.Equal(t, v1.GetLabel(w, v1.CICDScaleRunnerIdLabel), "r1")
}

// TestWorkloadMutateOwnerReferenceEarlyReturns covers owner-label and empty-env branches.
func TestWorkloadMutateOwnerReferenceEarlyReturns(t *testing.T) {
	scheme := newScheme(t)
	m := &WorkloadMutator{Client: fake.NewClientBuilder().WithScheme(scheme).Build()}

	// owner label already set -> early return
	withOwner := validWorkload()
	v1.SetLabel(withOwner, v1.OwnerLabel, "owner")
	m.mutateOwnerReference(context.Background(), withOwner, nil)

	// cicd ephemeral with empty scale runner set id -> break
	cicd := &v1.Workload{Spec: v1.WorkloadSpec{
		GroupVersionKind: v1.GroupVersionKind{Kind: common.CICDEphemeralRunnerKind},
		Env:              map[string]string{},
	}}
	m.mutateOwnerReference(context.Background(), cicd, nil)

	// unified with empty scale runner id -> break
	unified := &v1.Workload{Spec: v1.WorkloadSpec{
		GroupVersionKind: v1.GroupVersionKind{Kind: common.UnifiedJobKind},
		Env:              map[string]string{},
	}}
	m.mutateOwnerReference(context.Background(), unified, nil)
}

// TestWorkloadMutateServiceProtocolSet covers the protocol-already-set branch.
func TestWorkloadMutateServiceProtocolSet(t *testing.T) {
	m := &WorkloadMutator{}
	w := &v1.Workload{Spec: v1.WorkloadSpec{Service: &v1.Service{Protocol: "tcp", TargetPort: 80, Port: 90}}}
	m.mutateService(w)
	assert.Equal(t, w.Spec.Service.Protocol, corev1.ProtocolTCP)
}

// TestWorkloadMutateEnvGithubSecret covers the github secret annotation branch.
func TestWorkloadMutateEnvGithubSecret(t *testing.T) {
	m := &WorkloadMutator{}
	w := &v1.Workload{Spec: v1.WorkloadSpec{Env: map[string]string{"GITHUB_SECRET_ID": "sec"}}}
	m.mutateEnv(nil, w)
	assert.Equal(t, v1.GetAnnotation(w, v1.GithubSecretIdAnnotation), "sec")
}

// TestWorkloadMutateRdmaResourceBranches covers partial-gpu and default rdma branches.
func TestWorkloadMutateRdmaResourceBranches(t *testing.T) {
	commonconfig.SetValue("net.rdma_name", "rdma/hca")
	defer commonconfig.SetValue("net.rdma_name", "")
	scheme := newScheme(t)

	// partial gpu use -> rdma cleared
	flavor := &v1.NodeFlavor{ObjectMeta: metav1.ObjectMeta{Name: "flavor1"}, Spec: v1.NodeFlavorSpec{
		Cpu: v1.CpuChip{Quantity: resource.MustParse("8")}, Memory: resource.MustParse("16Gi"),
		Gpu: &v1.GpuChip{ResourceName: common.AmdGpu, Quantity: resource.MustParse("8")},
	}}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(flavor).Build()
	m := &WorkloadMutator{Client: c}
	partial := &v1.Workload{Spec: v1.WorkloadSpec{Resources: []v1.WorkloadResource{
		{Replica: 2, CPU: "1", GPU: "4", Memory: "2Gi"},
	}}}
	v1.SetLabel(partial, v1.NodeFlavorIdLabel, "flavor1")
	m.mutateRdmaResource(context.Background(), partial)
	assert.Equal(t, partial.Spec.Resources[0].RdmaResource, "")

	// gpu fully used, flavor has no rdma resource -> default "1"
	full := &v1.Workload{Spec: v1.WorkloadSpec{Resources: []v1.WorkloadResource{
		{Replica: 2, CPU: "1", GPU: "8", Memory: "2Gi"},
	}}}
	v1.SetLabel(full, v1.NodeFlavorIdLabel, "flavor1")
	m.mutateRdmaResource(context.Background(), full)
	assert.Equal(t, full.Spec.Resources[0].RdmaResource, "1")

	// flavor not found -> early return
	missing := &v1.Workload{Spec: v1.WorkloadSpec{Resources: []v1.WorkloadResource{wlResource()}}}
	v1.SetLabel(missing, v1.NodeFlavorIdLabel, "missing")
	m.mutateRdmaResource(context.Background(), missing)
}

// TestWorkloadValidateOwnerWorkloadCycle covers the owner cycle detection branch.
func TestWorkloadValidateOwnerWorkloadCycle(t *testing.T) {
	scheme := newScheme(t)
	owner := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "owner"}, Spec: v1.WorkloadSpec{Workspace: "ws1"}}
	v1.SetLabel(owner, v1.OwnerLabel, "w1")
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(owner).Build()
	v := &WorkloadValidator{Client: c}
	w := validWorkload()
	w.Spec.Workspace = "ws1"
	v1.SetLabel(w, v1.OwnerLabel, "owner")
	assert.Assert(t, v.validateOwnerWorkload(context.Background(), w) != nil)
}

// TestWorkloadValidateResourceEnoughEphemeralExceed covers the ephemeral exceed branch.
func TestWorkloadValidateResourceEnoughEphemeralExceed(t *testing.T) {
	commonconfig.SetValue("workload.max_ephemeral_store_percent", "0.01")
	defer commonconfig.SetValue("workload.max_ephemeral_store_percent", "0")
	nf := &v1.NodeFlavor{Spec: v1.NodeFlavorSpec{
		Cpu: v1.CpuChip{Quantity: resource.MustParse("8")}, Memory: resource.MustParse("16Gi"),
		ExtendResources: corev1.ResourceList{corev1.ResourceEphemeralStorage: resource.MustParse("100Gi")},
	}}
	res := &v1.WorkloadResource{Replica: 1, CPU: "1", Memory: "2Gi", EphemeralStorage: "50Gi"}
	assert.Assert(t, validateResourceEnough(nf, res) != nil)
}

// TestWorkloadValidateScopeOpsJob covers the opsjob scope short-circuit branch.
func TestWorkloadValidateScopeOpsJob(t *testing.T) {
	scheme := newScheme(t)
	v := &WorkloadValidator{Client: fake.NewClientBuilder().WithScheme(scheme).Build()}
	w := &v1.Workload{Spec: v1.WorkloadSpec{GroupVersionKind: v1.GroupVersionKind{Kind: common.PytorchJobKind}}}
	v1.SetLabel(w, v1.OpsJobIdLabel, "ops1")
	assert.NilError(t, v.validateScope(context.Background(), w))
}

// TestWorkloadValidateCronJobsError covers cron schedule validation error.
func TestWorkloadValidateCronJobsError(t *testing.T) {
	v := &WorkloadValidator{}
	w := &v1.Workload{Spec: v1.WorkloadSpec{CronJobs: []v1.CronJob{{Schedule: "", Action: v1.CronStart}}}}
	assert.Assert(t, v.validateCronJobs(w) != nil)
}

// TestGetWorkloadError covers the workload retrieval error path.
func TestGetWorkloadError(t *testing.T) {
	scheme := newScheme(t)
	c := fake.NewClientBuilder().WithScheme(scheme).Build()
	_, err := getWorkload(context.Background(), c, "missing")
	assert.Assert(t, err != nil)
}

// TestWorkloadDynamoPlannerBranch covers the planner-count error branch.
func TestWorkloadDynamoPlannerBranch(t *testing.T) {
	v := &WorkloadValidator{}
	w := dynamoWorkload(common.DynamoDeploymentKind, "vllm", "nixl",
		"frontend,planner,planner", 3)
	assert.Assert(t, v.validateDynamoDeployment(w) != nil)
}
