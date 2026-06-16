/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package webhooks

import (
	"context"
	"testing"

	"gotest.tools/assert"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
)

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
			v1.WorkloadKindLabel:       common.CICDEphemeralRunnerKind,
			v1.CICDScaleRunnerIdLabel:  "r1",
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
