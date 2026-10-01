/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package dispatcher

import (
	"context"
	"net/http"
	"testing"

	"gotest.tools/v3/assert"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
)

const workerDispatchImage = "docker.io/team/app@sha256:fedcba9876543210fedcba9876543210fedcba9876543210fedcba9876543210"

// externalGangWorkload returns an admitted RDMA PyTorchJob with one master and two workers.
func externalGangWorkload() *v1.Workload {
	w := claimWorkload()
	w.Spec.GroupVersionKind = v1.GroupVersionKind{Kind: common.PytorchJobKind, Version: "v1"}
	w.Spec.Resources = []v1.WorkloadResource{
		{Replica: 1, CPU: "8", Memory: "64Gi", GPU: "1", GPUName: "amd.com/gpu", RdmaResource: "1k"},
		{Replica: 2, CPU: "8", Memory: "64Gi", GPU: "1", GPUName: "amd.com/gpu", RdmaResource: "1k"},
	}
	w.Status.ExternalExecution.Placements = []v1.WorkloadExternalPlacement{
		{UnitKey: v1.ExternalSingleUnitKey, NodeName: "vk-a", ImageRef: pinnedDispatchImage, CPUMillis: 8000},
		{UnitKey: "worker/0", NodeName: "vk-b", ImageRef: pinnedDispatchImage, CPUMillis: 8000},
		{UnitKey: "worker/1", NodeName: "vk-c", ImageRef: pinnedDispatchImage, CPUMillis: 8000},
	}
	return w
}

// pinnedNodes returns the node named by each term's metadata.name pin.
func pinnedNodes(t *testing.T, obj *unstructured.Unstructured) []string {
	terms, _, err := unstructured.NestedSlice(obj.Object, "spec", "template", "spec", "affinity",
		"nodeAffinity", "requiredDuringSchedulingIgnoredDuringExecution", "nodeSelectorTerms")
	assert.NilError(t, err)
	var nodes []string
	for _, raw := range terms {
		for _, f := range raw.(map[string]interface{})["matchFields"].([]interface{}) {
			m := f.(map[string]interface{})
			if m["key"] != "metadata.name" || m["operator"] != "In" {
				continue
			}
			values := m["values"].([]interface{})
			assert.Equal(t, len(values), 1, "a node field selector takes exactly one name")
			nodes = append(nodes, values[0].(string))
		}
	}
	return nodes
}

// The API server accepts one name per metadata.name selector, so each role's approved
// nodes are pinned in separate terms.
func TestApplyExternalNodePinGangOneNodePerTerm(t *testing.T) {
	w := externalGangWorkload()
	master := &unstructured.Unstructured{Object: map[string]interface{}{}}
	assert.NilError(t, applyExternalNodePin(master, w, externalShapeSpec(), 0))
	assert.DeepEqual(t, pinnedNodes(t, master), []string{"vk-a"})

	worker := &unstructured.Unstructured{Object: map[string]interface{}{}}
	assert.NilError(t, applyExternalNodePin(worker, w, externalShapeSpec(), 1))
	assert.DeepEqual(t, pinnedNodes(t, worker), []string{"vk-b", "vk-c"})
}

// The gang is decided by the workload shape, not by how many units the claim returned.
func TestIsExternalGangFollowsWorkloadShape(t *testing.T) {
	short := externalGangWorkload()
	short.Status.ExternalExecution.Placements = short.Status.ExternalExecution.Placements[:1]
	assert.Assert(t, isExternalGang(short))

	single := claimWorkload()
	single.Status.ExternalExecution.Placements = append(single.Status.ExternalExecution.Placements,
		v1.WorkloadExternalPlacement{UnitKey: "worker/0", NodeName: "vk-2"})
	assert.Assert(t, !isExternalGang(single))
}

// A later host-network pass must not re-enable the host network on a single-unit external pod.
func TestUpdateHostNetworkKeepsExternalDecision(t *testing.T) {
	w := claimWorkload()
	w.Spec.Resources = []v1.WorkloadResource{{Replica: 1}}
	w.Annotations = map[string]string{v1.ForceHostNetworkAnnotation: v1.TrueStr}
	obj := &unstructured.Unstructured{Object: map[string]interface{}{}}
	assert.NilError(t, updateHostNetwork(w, obj, externalShapeSpec(), 0))
	enabled, _, _ := unstructured.NestedBool(obj.Object, "spec", "template", "spec", "hostNetwork")
	assert.Assert(t, !enabled)
}

// A worker template carries the worker unit's approval, not the master's.
func TestExternalRoleUnitKey(t *testing.T) {
	w := externalGangWorkload()
	w.Status.ExternalExecution.Placements[1].ImageRef = workerDispatchImage
	w.Status.ExternalExecution.Placements[1].CPUMillis = 4000
	assert.Equal(t, externalApprovedImage(w, externalRoleUnitKey(w, 0)), pinnedDispatchImage)
	assert.Equal(t, externalApprovedImage(w, externalRoleUnitKey(w, 1)), workerDispatchImage)
	resources, err := externalApprovedResourceMap(w, externalRoleUnitKey(w, 1))
	assert.NilError(t, err)
	assert.Equal(t, resources["cpu"], "4000m")
	assert.Equal(t, externalRoleUnitKey(claimWorkload(), 0), v1.ExternalSingleUnitKey)
}

// The dispatch recheck covers every unit of a gang, not only master/0.
func TestVerifyExternalClaimChecksEveryGangUnit(t *testing.T) {
	r := &DispatcherReconciler{}
	tagged := externalGangWorkload()
	tagged.Status.ExternalExecution.Placements[2].ImageRef = "docker.io/team/app:v1"
	missing := externalGangWorkload()
	missing.Status.ExternalExecution.Placements = missing.Status.ExternalExecution.Placements[:2]
	shared := externalGangWorkload()
	shared.Status.ExternalExecution.Placements[2].NodeName = "vk-b"
	for name, w := range map[string]*v1.Workload{"tag image": tagged, "missing unit": missing, "shared node": shared} {
		useClaimServer(t, claimBody("Active", string(w.UID), 1, "2099-01-01T00:00:00.000Z"), http.StatusOK)
		if err := r.verifyExternalClaim(context.Background(), w); !isClaimGone(err) {
			t.Errorf("%s: err = %v, want claim gone", name, err)
		}
	}
	good := externalGangWorkload()
	useClaimServer(t, claimBody("Active", string(good.UID), 1, "2099-01-01T00:00:00.000Z"), http.StatusOK)
	assert.NilError(t, r.verifyExternalClaim(context.Background(), good))
}

// externalEnvObject returns a pod object whose main container carries the given env names.
func externalEnvObject(names ...string) *unstructured.Unstructured {
	envs := make([]interface{}, 0, len(names))
	for _, n := range names {
		envs = append(envs, map[string]interface{}{"name": n, "value": "site"})
	}
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"spec": map[string]interface{}{"template": map[string]interface{}{"spec": map[string]interface{}{
			"containers": []interface{}{map[string]interface{}{"name": "main", "env": envs}},
		}}},
	}}
}

// envValues maps the main container env names to their values.
func envValues(t *testing.T, obj *unstructured.Unstructured) map[string]string {
	containers, _, err := unstructured.NestedSlice(obj.Object, "spec", "template", "spec", "containers")
	assert.NilError(t, err)
	result := map[string]string{}
	for _, raw := range containers[0].(map[string]interface{})["env"].([]interface{}) {
		m := raw.(map[string]interface{})
		result[m["name"].(string)], _ = m["value"].(string)
	}
	return result
}

// A gang leaves fabric settings to the node's site configuration and disables MSCCL.
func TestApplyExternalCommEnvGang(t *testing.T) {
	obj := externalEnvObject(append(append([]string{}, externalSiteCommEnvs...), "NCCL_DEBUG")...)
	assert.NilError(t, applyExternalCommEnv(obj, externalGangWorkload(), externalShapeSpec()))
	envs := envValues(t, obj)
	for _, name := range externalSiteCommEnvs {
		_, found := envs[name]
		assert.Assert(t, !found, "%s must be left to the site", name)
	}
	assert.Equal(t, envs["NCCL_DEBUG"], "site")
	assert.Equal(t, envs["GIT_CONFIG_COUNT"], "1")
	assert.Equal(t, envs["GIT_CONFIG_KEY_0"], "safe.directory")
	assert.Equal(t, envs["GIT_CONFIG_VALUE_0"], "*")
}

// A single-unit pod keeps its env; a value already set for MSCCL is kept.
func TestApplyExternalCommEnvSingleUnit(t *testing.T) {
	obj := externalEnvObject("NCCL_IB_HCA")
	assert.NilError(t, applyExternalCommEnv(obj, claimWorkload(), externalShapeSpec()))
	envs := envValues(t, obj)
	assert.Equal(t, envs["NCCL_IB_HCA"], "site")
	assert.Equal(t, envs["GIT_CONFIG_COUNT"], "1")
}

// A workload outside external capacity is left untouched.
func TestApplyExternalCommEnvSkipsNativeWorkload(t *testing.T) {
	native := externalGangWorkload()
	native.Status.ExternalExecution = nil
	obj := externalEnvObject("NCCL_IB_HCA")
	assert.NilError(t, applyExternalCommEnv(obj, native, externalShapeSpec()))
	assert.DeepEqual(t, envValues(t, obj), map[string]string{"NCCL_IB_HCA": "site"})
}
