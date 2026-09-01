/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package webhooks

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
	"gotest.tools/assert"
	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

func TestValidateDisplayName(t *testing.T) {
	tests := []struct {
		name         string
		displayName  string
		workloadKind string
		wantErr      bool
	}{
		{
			name:         "empty name is valid",
			displayName:  "",
			workloadKind: common.DeploymentKind,
			wantErr:      false,
		},
		{
			name:         "valid simple name",
			displayName:  "prod-29pvc",
			workloadKind: common.DeploymentKind,
			wantErr:      false,
		},
		{
			name:         "valid name with dots",
			displayName:  "my.app.v1",
			workloadKind: common.DeploymentKind,
			wantErr:      false,
		},
		{
			name:         "valid name with hyphens",
			displayName:  "my-app-v1",
			workloadKind: common.PytorchJobKind,
			wantErr:      false,
		},
		{
			name:         "valid minimum length name",
			displayName:  "ab",
			workloadKind: common.DeploymentKind,
			wantErr:      false,
		},
		{
			name:         "invalid - starts with number",
			displayName:  "1abc",
			workloadKind: common.DeploymentKind,
			wantErr:      true,
		},
		{
			name:         "invalid - starts with hyphen",
			displayName:  "-abc",
			workloadKind: common.DeploymentKind,
			wantErr:      true,
		},
		{
			name:         "invalid - ends with hyphen",
			displayName:  "abc-",
			workloadKind: common.DeploymentKind,
			wantErr:      true,
		},
		{
			name:         "invalid - ends with dot",
			displayName:  "abc.",
			workloadKind: common.DeploymentKind,
			wantErr:      true,
		},
		{
			name:         "invalid - uppercase letters",
			displayName:  "MyApp",
			workloadKind: common.DeploymentKind,
			wantErr:      true,
		},
		{
			name:         "invalid - contains underscore",
			displayName:  "my_app",
			workloadKind: common.DeploymentKind,
			wantErr:      true,
		},
		{
			name:         "invalid - single character",
			displayName:  "a",
			workloadKind: common.DeploymentKind,
			wantErr:      true,
		},
		{
			name:         "valid max length for deployment",
			displayName:  "a" + strings.Repeat("b", commonutils.MaxDeploymentNameLen-1) + "c",
			workloadKind: common.DeploymentKind,
			wantErr:      false,
		},
		{
			name:         "invalid - exceeds max length for deployment",
			displayName:  "a" + strings.Repeat("b", commonutils.MaxDeploymentNameLen+1) + "c",
			workloadKind: common.DeploymentKind,
			wantErr:      true,
		},
		{
			name:         "valid max length for pytorchjob",
			displayName:  "a" + strings.Repeat("b", commonutils.MaxPytorchJobNameLen-1) + "c",
			workloadKind: common.PytorchJobKind,
			wantErr:      false,
		},
		{
			name:         "invalid - exceeds max length for pytorchjob",
			displayName:  "a" + strings.Repeat("b", commonutils.MaxPytorchJobNameLen+1) + "c",
			workloadKind: common.PytorchJobKind,
			wantErr:      true,
		},
		{
			name:         "valid max length for torchft",
			displayName:  "a" + strings.Repeat("b", commonutils.MaxTorchFTNameLen-1) + "c",
			workloadKind: common.TorchFTKind,
			wantErr:      false,
		},
		{
			name:         "invalid - exceeds max length for torchft",
			displayName:  "a" + strings.Repeat("b", commonutils.MaxTorchFTNameLen+1) + "c",
			workloadKind: common.TorchFTKind,
			wantErr:      true,
		},
		{
			name:         "valid for unknown workload kind uses default length",
			displayName:  "a" + strings.Repeat("b", commonutils.MaxGeneratedNameLength-1) + "c",
			workloadKind: "UnknownKind",
			wantErr:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateDisplayName(tt.displayName, tt.workloadKind)
			if tt.wantErr {
				assert.Assert(t, err != nil, "expected error but got nil")
				assert.Assert(t, commonerrors.IsBadRequest(err), "expected BadRequest error")
			} else {
				assert.NilError(t, err)
			}
		})
	}
}

// TestGeneratePaths verifies mutate/validate webhook path generation.
func TestGeneratePaths(t *testing.T) {
	assert.Equal(t, "/mutate-"+WebhookPathPrefix+"cluster", generateMutatePath("Cluster"))
	assert.Equal(t, "/validate-"+WebhookPathPrefix+"cluster", generateValidatePath("Cluster"))
}

// TestHandleError verifies error responses for nil and non-nil errors.
func TestHandleError(t *testing.T) {
	resp := handleError("test", nil)
	assert.Assert(t, resp.Allowed)

	resp = handleError("test", fmt.Errorf("boom"))
	assert.Assert(t, !resp.Allowed)

	resp = handleError("test", commonerrors.NewBadRequest("bad"))
	assert.Assert(t, !resp.Allowed)
}

// TestValidateDNSName verifies DNS name validation rules.
func TestValidateDNSName(t *testing.T) {
	assert.NilError(t, validateDNSName("", common.DeploymentKind))
	assert.NilError(t, validateDNSName("my-app1", common.DeploymentKind))
	assert.Assert(t, validateDNSName("my.app", common.DeploymentKind) != nil)
	assert.Assert(t, validateDNSName("-bad", common.DeploymentKind) != nil)
}

// TestGetMaxNameLength verifies max name length resolution per workload kind.
func TestGetMaxNameLength(t *testing.T) {
	assert.Equal(t, commonutils.MaxPytorchJobNameLen, getMaxNameLength(common.PytorchJobKind))
	assert.Equal(t, commonutils.MaxDeploymentNameLen, getMaxNameLength(common.DeploymentKind))
	assert.Equal(t, commonutils.MaxTorchFTNameLen, getMaxNameLength(common.TorchFTKind))
	assert.Equal(t, commonutils.MaxCICDScaleSetNameLen, getMaxNameLength(common.CICDScaleRunnerSetKind))
	assert.Equal(t, commonutils.MaxRayJobNameLen, getMaxNameLength(common.RayJobKind))
	assert.Equal(t, commonutils.MaxMonarchJobNameLen, getMaxNameLength(common.MonarchJob))
	assert.Equal(t, commonutils.MaxGeneratedNameLength, getMaxNameLength("Unknown"))
}

// TestValidateLabels verifies label map validation.
func TestValidateLabels(t *testing.T) {
	assert.NilError(t, validateLabels(map[string]string{"key": "value"}))
	assert.Assert(t, validateLabels(map[string]string{"": "value"}) != nil)
}

// TestValidateLabelKey verifies label key validation including '/' handling.
func TestValidateLabelKey(t *testing.T) {
	assert.NilError(t, validateLabelKey("valid-key"))
	assert.NilError(t, validateLabelKey("prefix/name"))
	assert.Assert(t, validateLabelKey("") != nil)
	assert.Assert(t, validateLabelKey("a/b/c") != nil)
	assert.Assert(t, validateLabelKey("/bad") != nil)
}

// TestValidatePort verifies port range validation.
func TestValidatePort(t *testing.T) {
	assert.NilError(t, validatePort("svc", 8080))
	assert.Assert(t, validatePort("svc", 0) != nil)
	assert.Assert(t, validatePort("svc", 70000) != nil)
}

// --- merged from errors2_test.go ---

// TestWorkloadMutateOwnerReferenceCICD covers cicd ephemeral runner owner reference branch.
func TestWorkloadMutateOwnerReferenceCICD(t *testing.T) {
	scheme := newScheme(t)
	set := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "set1", UID: "uid-set"}}
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(set).Build()
	m := &WorkloadMutator{Client: k8sClient}
	w := &v1.Workload{
		ObjectMeta: metav1.ObjectMeta{Name: "w1"},
		Spec: v1.WorkloadSpec{
			GroupVersionKind: v1.GroupVersionKind{Kind: common.CICDEphemeralRunnerKind},
			Env:              map[string]string{common.ScaleRunnerSetID: "set1"},
		},
	}
	m.mutateOwnerReference(context.Background(), w, nil)
	assert.Assert(t, len(w.OwnerReferences) > 0)
}

// TestWorkloadMutateOwnerReferenceUnified covers unified job owner reference branch.
func TestWorkloadMutateOwnerReferenceUnified(t *testing.T) {
	scheme := newScheme(t)
	runner := &v1.Workload{ObjectMeta: metav1.ObjectMeta{
		Name: "runner1", UID: "uid-runner",
		Labels: map[string]string{
			v1.WorkloadKindLabel:      common.CICDEphemeralRunnerKind,
			v1.CICDScaleRunnerIdLabel: "r1",
		},
	}}
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(runner).Build()
	m := &WorkloadMutator{Client: k8sClient}
	w := &v1.Workload{
		ObjectMeta: metav1.ObjectMeta{Name: "w1"},
		Spec: v1.WorkloadSpec{
			GroupVersionKind: v1.GroupVersionKind{Kind: common.UnifiedJobKind},
			Env:              map[string]string{common.ScaleRunnerID: "r1"},
		},
	}
	m.mutateOwnerReference(context.Background(), w, nil)
	assert.Assert(t, len(w.OwnerReferences) > 0)
}

// TestWorkloadValidateScopeBranches covers scope validation branches.
func TestWorkloadValidateScopeBranches(t *testing.T) {
	scheme := newScheme(t)

	// unknown kind -> scope empty -> error
	v := &WorkloadValidator{Client: fake.NewClientBuilder().WithScheme(scheme).Build()}
	unknown := &v1.Workload{Spec: v1.WorkloadSpec{
		GroupVersionKind: v1.GroupVersionKind{Kind: "Weird"}, Workspace: "ws1",
	}}
	assert.Assert(t, v.validateScope(context.Background(), unknown) != nil)

	// workspace with non-matching scope -> forbidden
	ws := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"},
		Spec: v1.WorkspaceSpec{Scopes: []v1.WorkspaceScope{v1.InferScope}}}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ws).Build()
	v2 := &WorkloadValidator{Client: c}
	train := &v1.Workload{Spec: v1.WorkloadSpec{
		GroupVersionKind: v1.GroupVersionKind{Kind: common.PytorchJobKind}, Workspace: "ws1",
	}}
	assert.Assert(t, v2.validateScope(context.Background(), train) != nil)
}

// TestWorkloadValidateReplicaCountWithOld covers replica count update branches.
func TestWorkloadValidateReplicaCountWithOld(t *testing.T) {
	v := &WorkloadValidator{}
	mk := func() *v1.Workload {
		return &v1.Workload{Spec: v1.WorkloadSpec{
			Resources: []v1.WorkloadResource{wlResource(), {Replica: 4}},
			Env:       replicaEnv(),
		}}
	}
	assert.NilError(t, v.validateReplicaCount(mk(), mk()))

	// changed max -> error
	changed := mk()
	changed.Spec.Env[common.MaxReplicaCount] = "8"
	assert.Assert(t, v.validateReplicaCount(changed, mk()) != nil)
}

// TestWorkloadValidateResourceEnoughSharedMem covers shared memory validation branch.
func TestWorkloadValidateResourceEnoughSharedMem(t *testing.T) {
	nf := &v1.NodeFlavor{Spec: v1.NodeFlavorSpec{
		Cpu:    v1.CpuChip{Quantity: resource.MustParse("8")},
		Memory: resource.MustParse("16Gi"),
	}}
	// shared memory larger than node memory -> invalid
	res := &v1.WorkloadResource{Replica: 1, CPU: "1", Memory: "2Gi", SharedMemory: "64Gi", EphemeralStorage: "3Gi"}
	assert.Assert(t, validateResourceEnough(nf, res) != nil)
}

// TestWorkloadValidateMonarchEntrypoint covers monarch missing entrypoint branch.
func TestWorkloadValidateMonarchEntrypoint(t *testing.T) {
	v := &WorkloadValidator{}
	w := &v1.Workload{Spec: v1.WorkloadSpec{
		Resources: []v1.WorkloadResource{wlResource(), wlResource()},
	}}
	assert.Assert(t, v.validateMonarchJob(w, nil) != nil)
}

// TestWorkloadValidateTorchFTImage covers torchFT empty image branch.
func TestWorkloadValidateTorchFTImage(t *testing.T) {
	v := &WorkloadValidator{}
	w := &v1.Workload{Spec: v1.WorkloadSpec{
		Resources: []v1.WorkloadResource{wlResource(), wlResource()},
		Images:    []string{""},
	}}
	assert.Assert(t, v.validateTorchFT(w, nil) != nil)
}

// TestWorkloadValidateRayJobBranches covers rayJob resource count and entrypoint branches.
func TestWorkloadValidateRayJobBranches(t *testing.T) {
	v := &WorkloadValidator{}
	tooMany := &v1.Workload{Spec: v1.WorkloadSpec{
		Resources: []v1.WorkloadResource{wlResource(), wlResource(), wlResource(), wlResource(), wlResource()},
		Images:    []string{"a", "b"},
	}}
	assert.Assert(t, v.validateRayJob(tooMany, nil) != nil)

	noEntry := &v1.Workload{Spec: v1.WorkloadSpec{
		Resources: []v1.WorkloadResource{wlResource(), wlResource()},
		Images:    []string{"a", "b"},
	}}
	assert.Assert(t, v.validateRayJob(noEntry, nil) != nil)
}

// TestWorkloadValidateCICDBranches covers cicd validation error branches.
func TestWorkloadValidateCICDBranches(t *testing.T) {
	v := &WorkloadValidator{}
	missingKey := &v1.Workload{Spec: v1.WorkloadSpec{Env: map[string]string{ResourcesEnv: "x"}}}
	assert.Assert(t, v.validateCICDScalingRunnerSet(missingKey) != nil)

	badJSON := &v1.Workload{Spec: v1.WorkloadSpec{Env: map[string]string{
		ResourcesEnv:           "not-json",
		EntrypointEnv:          "cmd",
		ImageEnv:               "img",
		common.GithubConfigUrl: "http://x",
	}}}
	assert.Assert(t, v.validateCICDScalingRunnerSet(badJSON) != nil)
}

// TestWorkloadValidateImmutableCICDEnv covers cicd unified-job-enable immutability branch.
func TestWorkloadValidateImmutableCICDEnv(t *testing.T) {
	v := &WorkloadValidator{}
	oldW := &v1.Workload{Spec: v1.WorkloadSpec{
		GroupVersionKind: v1.GroupVersionKind{Kind: common.CICDScaleRunnerSetKind},
		Workspace:        "ws1",
		Env:              map[string]string{common.UnifiedJobEnable: v1.TrueStr},
	}}
	newW := &v1.Workload{Spec: v1.WorkloadSpec{
		GroupVersionKind: v1.GroupVersionKind{Kind: common.CICDScaleRunnerSetKind},
		Workspace:        "ws1",
		Env:              map[string]string{},
	}}
	assert.Assert(t, v.validateImmutableFields(newW, oldW) != nil)
}

// --- merged from errors3_test.go ---

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
	m.mutateService(nil, w)
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

	// partial gpu use is still multi-node (replica > 1) -> rdma added (default "1")
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
	assert.Equal(t, partial.Spec.Resources[0].RdmaResource, "1")

	// gpu fully used, flavor has no rdma resource -> default "1"
	full := &v1.Workload{Spec: v1.WorkloadSpec{Resources: []v1.WorkloadResource{
		{Replica: 2, CPU: "1", GPU: "8", Memory: "2Gi"},
	}}}
	v1.SetLabel(full, v1.NodeFlavorIdLabel, "flavor1")
	m.mutateRdmaResource(context.Background(), full)
	assert.Equal(t, full.Spec.Resources[0].RdmaResource, "1")

	// single-node (replica == 1) GPU role -> no cross-node rdma added
	single := &v1.Workload{Spec: v1.WorkloadSpec{Resources: []v1.WorkloadResource{
		{Replica: 1, CPU: "1", GPU: "8", Memory: "2Gi"},
	}}}
	v1.SetLabel(single, v1.NodeFlavorIdLabel, "flavor1")
	m.mutateRdmaResource(context.Background(), single)
	assert.Equal(t, single.Spec.Resources[0].RdmaResource, "")

	// caller-provided rdma on a multi-node role is preserved (not overwritten)
	userSet := &v1.Workload{Spec: v1.WorkloadSpec{Resources: []v1.WorkloadResource{
		{Replica: 2, CPU: "1", GPU: "4", Memory: "2Gi", RdmaResource: "1k"},
	}}}
	v1.SetLabel(userSet, v1.NodeFlavorIdLabel, "flavor1")
	m.mutateRdmaResource(context.Background(), userSet)
	assert.Equal(t, userSet.Spec.Resources[0].RdmaResource, "1k")

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

// --- merged from errors_test.go ---

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
		v1.SetAnnotation(w, v1.InferaBackendFrameworkAnnotation, fw)
		v1.SetAnnotation(w, v1.InferaKVTransferBackendAnnotation, kv)
		v1.SetAnnotation(w, v1.InferaServiceRolesAnnotation, roles)
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

// TestWorkloadInferaBranches covers infera deployment validation error branches.
func TestWorkloadInferaBranches(t *testing.T) {
	v := &WorkloadValidator{}
	k := common.InferaDeploymentKind
	assert.Assert(t, v.validateInferaDeployment(dynamoWorkload(k, "vllm", "nixl", "frontend,worker,prefill,decode,planner,epp", 6)) != nil)
	assert.Assert(t, v.validateInferaDeployment(dynamoWorkload(k, "bad", "bad", "frontend", 1)) != nil)
	assert.Assert(t, v.validateInferaDeployment(dynamoWorkload(k, "vllm", "nixl", "frontend,worker", 1)) != nil)
	assert.Assert(t, v.validateInferaDeployment(dynamoWorkload(k, "vllm", "nixl", "weird", 1)) != nil)
	assert.Assert(t, v.validateInferaDeployment(dynamoWorkload(k, "vllm", "nixl", "frontend,worker,prefill", 3)) != nil)
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

	// owner not found is tolerated (issue #588): the apiserver may create an
	// owner-labeled child before its owner workload is persisted, so a NotFound
	// owner must not block admission.
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

// --- merged from helpers_test.go ---

// newScheme builds a runtime scheme with the project and client-go types.
func newScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	assert.NilError(t, clientscheme.AddToScheme(s))
	assert.NilError(t, v1.AddToScheme(s))
	return s
}

// newDecoder builds an admission decoder for tests.
func newDecoder(t *testing.T) admission.Decoder {
	t.Helper()
	return admission.NewDecoder(newScheme(t))
}

// newRequest builds an admission request for the given operation and objects.
func newRequest(t *testing.T, op admissionv1.Operation, obj, oldObj interface{}) admission.Request {
	t.Helper()
	req := admission.Request{
		AdmissionRequest: admissionv1.AdmissionRequest{
			Operation: op,
		},
	}
	if obj != nil {
		raw, err := json.Marshal(obj)
		assert.NilError(t, err)
		req.Object = runtime.RawExtension{Raw: raw}
	}
	if oldObj != nil {
		raw, err := json.Marshal(oldObj)
		assert.NilError(t, err)
		req.OldObject = runtime.RawExtension{Raw: raw}
	}
	return req
}

// --- merged from infra_test.go ---

// fakeManager is a minimal manager.Manager implementation for webhook registration tests.
type fakeManager struct {
	manager.Manager
	client client.Client
	scheme *runtime.Scheme
}

// GetClient returns the embedded fake client.
func (m *fakeManager) GetClient() client.Client { return m.client }

// GetScheme returns the embedded scheme.
func (m *fakeManager) GetScheme() *runtime.Scheme { return m.scheme }

// TestSetUpWebhooks verifies all webhooks register without error.
func TestSetUpWebhooks(t *testing.T) {
	s := newScheme(t)
	mgr := &fakeManager{
		client: fake.NewClientBuilder().WithScheme(s).Build(),
		scheme: s,
	}
	server := webhook.NewServer(webhook.Options{})
	setUpWebhooks(mgr, server)
}

// TestNewServer verifies server construction fails without required flags.
func TestNewServer(t *testing.T) {
	s, err := NewServer()
	assert.Assert(t, err != nil)
	assert.Assert(t, s == nil)
}

// TestServerInitConfig verifies config initialization error handling.
func TestServerInitConfig(t *testing.T) {
	s := &Server{opts: &Options{Config: "nonexistent-config.yaml"}}
	assert.Assert(t, s.initConfig() != nil)
}

// TestServerNewCtrlManager verifies controller manager creation fails out of cluster.
func TestServerNewCtrlManager(t *testing.T) {
	s := &Server{opts: &Options{}}
	assert.Assert(t, s.newCtrlManager() != nil)
}

// TestServerInitLogs verifies log initialization runs.
func TestServerInitLogs(t *testing.T) {
	s := &Server{opts: &Options{}}
	_ = s.initLogs()
}

// TestServerStop verifies stop runs without panic.
func TestServerStop(t *testing.T) {
	s := &Server{}
	s.Stop()
}

// TestServerStartNotInited verifies start is a no-op when not initialized.
func TestServerStartNotInited(t *testing.T) {
	s := &Server{}
	s.Start()
}

// --- merged from kinds_test.go ---

// workloadOfKind builds a minimal workload of the given kind for mutation tests.
func workloadOfKind(kind string) *v1.Workload {
	return &v1.Workload{
		ObjectMeta: metav1.ObjectMeta{Name: "w1"},
		Spec: v1.WorkloadSpec{
			GroupVersionKind: v1.GroupVersionKind{Kind: kind, Version: "v1"},
			Env:              map[string]string{},
			Images:           []string{"img"},
			EntryPoints:      []string{"cmd"},
			Resources:        []v1.WorkloadResource{wlResource()},
		},
	}
}

// TestWorkloadMutateCommonAllKinds covers the kind-specific mutation switch arms.
func TestWorkloadMutateCommonAllKinds(t *testing.T) {
	scheme := newScheme(t)
	m := &WorkloadMutator{Client: fake.NewClientBuilder().WithScheme(scheme).Build()}
	kinds := []string{
		common.DeploymentKind, common.StatefulSetKind, common.AuthoringKind,
		common.CICDScaleRunnerSetKind, common.MonarchJob, common.RayJobKind,
		common.TorchFTKind, common.SandboxKind, common.DynamoDeploymentKind,
		common.InferaDeploymentKind,
	}
	for _, k := range kinds {
		w := workloadOfKind(k)
		assert.NilError(t, m.mutateCommon(context.Background(), nil, w, nil))
	}
}

// TestWorkloadValidateCommonAllKinds covers the kind-specific validation switch arms.
func TestWorkloadValidateCommonAllKinds(t *testing.T) {
	scheme := newScheme(t)
	v := &WorkloadValidator{Client: fake.NewClientBuilder().WithScheme(scheme).Build()}
	kinds := []string{
		common.AuthoringKind, common.CICDScaleRunnerSetKind, common.TorchFTKind,
		common.RayJobKind, common.MonarchJob, common.SandboxKind,
		common.DynamoDeploymentKind, common.InferaDeploymentKind,
	}
	for _, k := range kinds {
		w := workloadOfKind(k)
		// most kinds fail downstream validation; we only need the switch arm to execute
		_ = v.validateCommon(context.Background(), w, nil)
	}
}

// --- merged from small_test.go ---

// TestHandlersDecodeErrors covers decode-error branches across webhooks.
func TestHandlersDecodeErrors(t *testing.T) {
	d := newDecoder(t)
	bad := newRequest(t, admissionv1.Create, nil, nil)

	assert.Assert(t, !(&AddOnTemplateMutator{decoder: d}).Handle(context.Background(), bad).Allowed)
	assert.Assert(t, !(&AddOnTemplateValidator{decoder: d}).Handle(context.Background(), bad).Allowed)
	assert.Assert(t, !(&ResourceTemplateMutator{decoder: d}).Handle(context.Background(), bad).Allowed)
	assert.Assert(t, !(&ResourceTemplateValidator{decoder: d}).Handle(context.Background(), bad).Allowed)
	assert.Assert(t, !(&RoleMutator{decoder: d}).Handle(context.Background(), bad).Allowed)
	assert.Assert(t, !(&RoleValidator{decoder: d}).Handle(context.Background(), bad).Allowed)
	assert.Assert(t, !(&NodeFlavorMutator{decoder: d}).Handle(context.Background(), bad).Allowed)
	assert.Assert(t, !(&NodeFlavorValidator{decoder: d}).Handle(context.Background(), bad).Allowed)
	assert.Assert(t, !(&FaultMutator{decoder: d}).Handle(context.Background(), bad).Allowed)
	assert.Assert(t, !(&ClusterMutator{decoder: d}).Handle(context.Background(), bad).Allowed)
}

// TestHandlersDeletionTimestamp covers deletion-timestamp short-circuit branches.
func TestHandlersDeletionTimestamp(t *testing.T) {
	d := newDecoder(t)
	now := metav1.Now()
	withDeletion := func(o interface {
		SetDeletionTimestamp(*metav1.Time)
		SetFinalizers([]string)
	}) {
		o.SetDeletionTimestamp(&now)
		o.SetFinalizers([]string{"x"})
	}

	addon := &v1.AddonTemplate{ObjectMeta: metav1.ObjectMeta{Name: "a"}}
	withDeletion(addon)
	assert.Assert(t, (&AddOnTemplateValidator{decoder: d}).Handle(context.Background(), newRequest(t, admissionv1.Update, addon, nil)).Allowed)

	rt := &v1.ResourceTemplate{ObjectMeta: metav1.ObjectMeta{Name: "r"}}
	withDeletion(rt)
	assert.Assert(t, (&ResourceTemplateValidator{decoder: d}).Handle(context.Background(), newRequest(t, admissionv1.Update, rt, nil)).Allowed)

	role := &v1.Role{ObjectMeta: metav1.ObjectMeta{Name: "r"}}
	withDeletion(role)
	assert.Assert(t, (&RoleValidator{decoder: d}).Handle(context.Background(), newRequest(t, admissionv1.Update, role, nil)).Allowed)

	nf := &v1.NodeFlavor{ObjectMeta: metav1.ObjectMeta{Name: "nf"}}
	withDeletion(nf)
	assert.Assert(t, (&NodeFlavorValidator{decoder: d}).Handle(context.Background(), newRequest(t, admissionv1.Update, nf, nil)).Allowed)

	cl := &v1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "c"}}
	withDeletion(cl)
	assert.Assert(t, (&ClusterValidator{decoder: d}).Handle(context.Background(), newRequest(t, admissionv1.Create, cl, nil)).Allowed)
	assert.Assert(t, (&ClusterValidator{decoder: d}).Handle(context.Background(), newRequest(t, admissionv1.Update, cl, cl)).Allowed)
}

// TestOptionsInitFlagsNil covers the nil receiver branch.
func TestOptionsInitFlagsNil(t *testing.T) {
	var opt *Options
	assert.Assert(t, opt.InitFlags() != nil)
}

// TestNodeFlavorValidateCommonQuantities covers memory/gpu quantity error branches.
func TestNodeFlavorValidateCommonQuantities(t *testing.T) {
	v := &NodeFlavorValidator{}
	badMem := &v1.NodeFlavor{Spec: v1.NodeFlavorSpec{Cpu: v1.CpuChip{Quantity: resource.MustParse("8")}}}
	assert.Assert(t, v.validateCommon(badMem) != nil)

	badGpuQty := &v1.NodeFlavor{Spec: v1.NodeFlavorSpec{
		Cpu:    v1.CpuChip{Quantity: resource.MustParse("8")},
		Memory: resource.MustParse("16Gi"),
		Gpu:    &v1.GpuChip{ResourceName: common.AmdGpu, Quantity: resource.MustParse("0")},
	}}
	assert.Assert(t, v.validateCommon(badGpuQty) != nil)
}

// TestNodeFlavorImmutableProductQuantity covers gpu product/quantity immutability branches.
func TestNodeFlavorImmutableProductQuantity(t *testing.T) {
	v := &NodeFlavorValidator{}
	old := &v1.NodeFlavor{Spec: v1.NodeFlavorSpec{Gpu: &v1.GpuChip{
		ResourceName: common.AmdGpu, Quantity: resource.MustParse("8"), Product: "MI300X",
	}}}
	qtyChanged := &v1.NodeFlavor{Spec: v1.NodeFlavorSpec{Gpu: &v1.GpuChip{
		ResourceName: common.AmdGpu, Quantity: resource.MustParse("4"), Product: "MI300X",
	}}}
	assert.Assert(t, v.validateImmutableFields(old, qtyChanged) != nil)

	productChanged := &v1.NodeFlavor{Spec: v1.NodeFlavorSpec{Gpu: &v1.GpuChip{
		ResourceName: common.AmdGpu, Quantity: resource.MustParse("8"), Product: "MI325X",
	}}}
	assert.Assert(t, v.validateImmutableFields(old, productChanged) != nil)
}

// TestNodeFlavorValidateCommonRdma covers the rdma extend-resource error branch.
func TestNodeFlavorValidateCommonRdma(t *testing.T) {
	commonconfig.SetValue("net.rdma_name", "rdma/hca")
	defer commonconfig.SetValue("net.rdma_name", "")
	v := &NodeFlavorValidator{}
	nf := &v1.NodeFlavor{Spec: v1.NodeFlavorSpec{
		Cpu:    v1.CpuChip{Quantity: resource.MustParse("8")},
		Memory: resource.MustParse("16Gi"),
		ExtendResources: corev1.ResourceList{
			"rdma/hca": resource.MustParse("0"),
		},
	}}
	assert.Assert(t, v.validateCommon(nf) != nil)
}

// TestClusterValidateOnCreationLabels covers the label validation error branch.
func TestClusterValidateOnCreationLabels(t *testing.T) {
	scheme := newScheme(t)
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(readyNode("node1")).Build()
	v := &ClusterValidator{Client: c}
	cl := validControlPlaneCluster()
	v1.SetLabel(cl, v1.DisplayNameLabel, "my-cluster")
	cl.Labels["Bad Key"] = "v"
	assert.Assert(t, v.validateOnCreation(context.Background(), cl) != nil)
}

// TestGetClusterError covers the cluster retrieval error path.
func TestGetClusterError(t *testing.T) {
	scheme := newScheme(t)
	c := fake.NewClientBuilder().WithScheme(scheme).Build()
	_, err := getCluster(context.Background(), c, "missing")
	assert.Assert(t, err != nil)
}

// TestFaultValidateOnCreationDisplayName covers display name validation in fault creation.
func TestFaultValidateOnCreationDisplayName(t *testing.T) {
	v := &FaultValidator{}
	fault := &v1.Fault{Spec: v1.FaultSpec{MonitorId: "m1"}}
	v1.SetLabel(fault, v1.DisplayNameLabel, "Bad_Name")
	assert.Assert(t, v.validateOnCreation(fault) != nil)
}
