/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package dispatcher

import (
	"testing"

	"gotest.tools/v3/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

func externalShapeWorkload() *v1.Workload {
	return &v1.Workload{
		ObjectMeta: metav1.ObjectMeta{Name: "w"},
		Status: v1.WorkloadStatus{ExternalExecution: &v1.WorkloadExternalExecution{
			ClaimId: "claim-1",
			Placements: []v1.WorkloadExternalPlacement{{
				UnitKey: v1.ExternalSingleUnitKey, NodeName: "vk-approved",
			}},
		}},
	}
}

func externalShapeSpec() v1.ResourceSpec {
	return v1.ResourceSpec{PrePaths: []string{"spec"}, PodSpecPaths: []string{"template", "spec"}}
}

// The claim approves one vector; a second container would request it again.
func TestValidateExternalPodShapeRejectsSidecars(t *testing.T) {
	obj := &unstructured.Unstructured{Object: map[string]interface{}{
		"spec": map[string]interface{}{"template": map[string]interface{}{"spec": map[string]interface{}{
			"containers": []interface{}{
				map[string]interface{}{"name": "main"},
				map[string]interface{}{"name": "sidecar"},
			},
		}}},
	}}
	assert.ErrorContains(t, validateExternalPodShape(obj, externalShapeWorkload(), externalShapeSpec()),
		"exactly one container")
}

// The submitter's uid has no passwd entry, so the runtime leaves HOME at "/". A HOME the
// user set is kept.
func TestApplyExternalHome(t *testing.T) {
	obj := &unstructured.Unstructured{Object: map[string]interface{}{
		"spec": map[string]interface{}{"template": map[string]interface{}{"spec": map[string]interface{}{
			"containers": []interface{}{map[string]interface{}{"name": "main"}},
		}}},
	}}
	assert.NilError(t, applyExternalHome(obj, externalShapeWorkload(), externalShapeSpec()))
	containers, _, _ := unstructured.NestedSlice(obj.Object, "spec", "template", "spec", "containers")
	envs := containers[0].(map[string]interface{})["env"].([]interface{})
	assert.DeepEqual(t, envs, []interface{}{map[string]interface{}{"name": "HOME", "value": externalHomeDir}})

	custom := &unstructured.Unstructured{Object: map[string]interface{}{
		"spec": map[string]interface{}{"template": map[string]interface{}{"spec": map[string]interface{}{
			"containers": []interface{}{map[string]interface{}{"name": "main", "env": []interface{}{
				map[string]interface{}{"name": "HOME", "value": "/work"}}}},
		}}},
	}}
	assert.NilError(t, applyExternalHome(custom, externalShapeWorkload(), externalShapeSpec()))
	containers, _, _ = unstructured.NestedSlice(custom.Object, "spec", "template", "spec", "containers")
	envs = containers[0].(map[string]interface{})["env"].([]interface{})
	assert.Equal(t, len(envs), 1)
	assert.Equal(t, envs[0].(map[string]interface{})["value"], "/work")
}

// Node selector terms are ORed, so every term has to carry the pin, and a term's own
// matchFields must be kept rather than overwritten.
func TestApplyExternalNodePinCoversEveryTerm(t *testing.T) {
	userField := map[string]interface{}{"key": "metadata.name", "operator": "NotIn", "values": []interface{}{"x"}}
	obj := &unstructured.Unstructured{Object: map[string]interface{}{
		"spec": map[string]interface{}{"template": map[string]interface{}{"spec": map[string]interface{}{
			"affinity": map[string]interface{}{"nodeAffinity": map[string]interface{}{
				"requiredDuringSchedulingIgnoredDuringExecution": map[string]interface{}{
					"nodeSelectorTerms": []interface{}{
						map[string]interface{}{"matchFields": []interface{}{userField}},
						map[string]interface{}{"matchExpressions": []interface{}{
							map[string]interface{}{"key": "gpu", "operator": "Exists"},
						}},
					},
				},
			}},
		}}},
	}}
	assert.NilError(t, applyExternalNodePin(obj, externalShapeWorkload(), externalShapeSpec(), 0))
	terms, _, err := unstructured.NestedSlice(obj.Object, "spec", "template", "spec", "affinity",
		"nodeAffinity", "requiredDuringSchedulingIgnoredDuringExecution", "nodeSelectorTerms")
	assert.NilError(t, err)
	assert.Equal(t, len(terms), 2)
	for i, raw := range terms {
		fields := raw.(map[string]interface{})["matchFields"].([]interface{})
		pinned := false
		for _, f := range fields {
			m := f.(map[string]interface{})
			if m["operator"] == "In" && m["key"] == "metadata.name" {
				pinned = true
				assert.DeepEqual(t, m["values"], []interface{}{"vk-approved"})
			}
		}
		assert.Assert(t, pinned, "term %d is not pinned", i)
	}
	first := terms[0].(map[string]interface{})["matchFields"].([]interface{})
	assert.Equal(t, len(first), 2, "user matchFields must be kept")
}
