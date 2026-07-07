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

func TestApplyInferaRoleFields(t *testing.T) {
	// Frontend -> server component, role removed.
	frontend := map[string]interface{}{"role": "old"}
	applyInferaRoleFields(frontend, common.DynamoRoleFrontend, "nixl")
	assert.Equal(t, frontend["componentType"], "server")
	_, hasRole := frontend["role"]
	assert.Equal(t, hasRole, false)

	// Worker -> worker component with mixed role.
	worker := map[string]interface{}{}
	applyInferaRoleFields(worker, common.DynamoRoleWorker, "nixl")
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
	existingMount := map[string]interface{}{"mountPath": "/pre-existing", "name": "pre-existing"}
	obj := &unstructured.Unstructured{Object: map[string]interface{}{
		"spec": map[string]interface{}{
			"spec": map[string]interface{}{
				"initContainers": []interface{}{
					map[string]interface{}{
						"name":         "an-init-container",
						"volumeMounts": []interface{}{existingMount},
					},
				},
			},
		},
	}}
	w := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "runner"}}
	// Two arbitrary persistent host paths; the test does not assume any
	// particular volume name or mount path, only that whatever
	// buildPersistentVolumeMounts produces is appended to the init container.
	w.Spec.Hostpath = []string{"/data-a", "/data-b"}

	err := modifyInitContainerVolumeMounts(obj, w, nil, path)
	assert.NilError(t, err)

	// The init container's mounts must be its original mounts followed by
	// exactly the workload's persistent mounts (nothing dropped, nothing extra).
	expected := append([]interface{}{existingMount}, buildPersistentVolumeMounts(w, nil)...)
	initContainers, _, err := jobutils.NestedSlice(obj.Object, path)
	assert.NilError(t, err)
	got := initContainers[0].(map[string]interface{})["volumeMounts"].([]interface{})
	assert.DeepEqual(t, got, expected)
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
