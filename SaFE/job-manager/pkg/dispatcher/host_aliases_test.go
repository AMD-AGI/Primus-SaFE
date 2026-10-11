/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package dispatcher

import (
	"testing"

	"github.com/spf13/viper"
	"gotest.tools/assert"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

// The cluster's registry host aliases go into every Pod the platform creates there, on a
// virtual kubelet as on a kubelet, after any the template already has.
func TestInitializeObjectAddsTheClustersHostAliases(t *testing.T) {
	viper.Set("save_image.clusters", []map[string]any{{
		"cluster": "edge",
		"host_aliases": []map[string]any{
			{"ip": "10.0.0.5", "hostnames": []string{"registry.example.com"}},
		},
	}})
	t.Cleanup(viper.Reset)

	obj, workload, spec := externalAuthoringPod()
	workload.Labels[v1.ClusterIdLabel] = "edge"
	workload.Spec.Resources = []v1.WorkloadResource{{Replica: 1, CPU: "1", Memory: "1Gi"}}
	path := podSpecPath(workload, &spec, "hostAliases")
	assert.NilError(t, unstructured.SetNestedSlice(obj.Object, []interface{}{
		map[string]interface{}{"ip": "10.0.0.1", "hostnames": []interface{}{"keep.example.com"}},
	}, path...))
	assert.NilError(t, initializeObject(obj, workload, nil, &spec, 0))

	got, _, err := unstructured.NestedSlice(obj.Object, path...)
	assert.NilError(t, err)
	assert.DeepEqual(t, []interface{}{
		map[string]interface{}{"ip": "10.0.0.1", "hostnames": []interface{}{"keep.example.com"}},
		map[string]interface{}{"ip": "10.0.0.5", "hostnames": []interface{}{"registry.example.com"}},
	}, got)

	// Another cluster gets none.
	obj, workload, spec = externalAuthoringPod()
	workload.Labels[v1.ClusterIdLabel] = "other"
	workload.Spec.Resources = []v1.WorkloadResource{{Replica: 1, CPU: "1", Memory: "1Gi"}}
	assert.NilError(t, initializeObject(obj, workload, nil, &spec, 0))
	_, found, _ := unstructured.NestedSlice(obj.Object, podSpecPath(workload, &spec, "hostAliases")...)
	assert.Assert(t, !found)
}

func TestModifyHostAliasesRefusesUnreadableSettings(t *testing.T) {
	viper.Set("save_image.clusters", "not a list")
	t.Cleanup(viper.Reset)
	obj, workload, spec := externalAuthoringPod()
	assert.Assert(t, modifyHostAliases(obj, workload, podSpecPath(workload, &spec, "hostAliases")) != nil)
}
