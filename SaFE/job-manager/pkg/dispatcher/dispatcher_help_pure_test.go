/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package dispatcher

import (
	"testing"

	"gotest.tools/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	jobutils "github.com/AMD-AIG-AIMA/SAFE/job-manager/pkg/utils"
)

func TestBuildSecretVolume(t *testing.T) {
	vol := buildSecretVolume("my-secret").(map[string]interface{})
	assert.Equal(t, vol["name"], "my-secret")
	secret := vol["secret"].(map[string]interface{})
	assert.Equal(t, secret["secretName"], "my-secret")
}

func TestApplyOptimusRoleFields(t *testing.T) {
	// Frontend -> server component, role removed.
	frontend := map[string]interface{}{"role": "old"}
	applyOptimusRoleFields(frontend, common.DynamoRoleFrontend, "nixl")
	assert.Equal(t, frontend["componentType"], "server")
	_, hasRole := frontend["role"]
	assert.Equal(t, hasRole, false)

	// Worker -> worker component with mixed role.
	worker := map[string]interface{}{}
	applyOptimusRoleFields(worker, common.DynamoRoleWorker, "nixl")
	assert.Equal(t, worker["componentType"], "worker")
	assert.Equal(t, worker["role"], "mixed")
}

func TestBuildRequiredMatchExpression(t *testing.T) {
	// Non-default workspace contributes a workspace match expression.
	w := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "w"}}
	w.Spec.Workspace = "ws-1"
	exprs := buildRequiredMatchExpression(w)
	assert.Assert(t, len(exprs) >= 1)

	// Default-namespace workspace with no customer labels -> no expressions.
	w2 := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "w2"}}
	w2.Spec.Workspace = corev1.NamespaceDefault
	exprs2 := buildRequiredMatchExpression(w2)
	assert.Equal(t, len(exprs2), 0)
}

func TestModifyInitContainerVolumeMounts(t *testing.T) {
	path := []string{"spec", "spec", "initContainers"}
	obj := &unstructured.Unstructured{Object: map[string]interface{}{
		"spec": map[string]interface{}{
			"spec": map[string]interface{}{
				"initContainers": []interface{}{
					map[string]interface{}{
						"name": "dind",
						"volumeMounts": []interface{}{
							map[string]interface{}{"mountPath": "/home/runner/_work", "name": "work"},
						},
					},
				},
			},
		},
	}}
	w := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "runner"}}
	w.Spec.Hostpath = []string{"/apps", "/wekafs"}

	err := modifyInitContainerVolumeMounts(obj, w, nil, path)
	assert.NilError(t, err)

	initContainers, _, err := jobutils.NestedSlice(obj.Object, path)
	assert.NilError(t, err)
	mounts := initContainers[0].(map[string]interface{})["volumeMounts"].([]interface{})
	var paths []string
	for _, m := range mounts {
		paths = append(paths, m.(map[string]interface{})["mountPath"].(string))
	}
	// Existing mount preserved; persistent host paths appended so the daemon sees them.
	assert.DeepEqual(t, paths, []string{"/home/runner/_work", "/apps", "/wekafs"})
}

func TestBuildRequiredMatchExpressionExcludedNodes(t *testing.T) {
	w := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "w"}}
	w.Spec.Workspace = corev1.NamespaceDefault
	w.Spec.CustomerLabels = map[string]string{common.ExcludedNodes: "node-a node-b"}
	exprs := buildRequiredMatchExpression(w)
	// Excluded nodes produce a NotIn host-name expression.
	assert.Assert(t, len(exprs) >= 1)
	m := exprs[0].(map[string]interface{})
	assert.Equal(t, m["operator"], "NotIn")
}
