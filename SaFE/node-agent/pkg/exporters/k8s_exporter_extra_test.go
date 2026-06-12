/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package exporters

import (
	"testing"

	"gotest.tools/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	commonfaults "github.com/AMD-AIG-AIMA/SAFE/common/pkg/faults"
	"github.com/AMD-AIG-AIMA/SAFE/node-agent/pkg/types"
)

// TestK8sExporterName returns the exporter identifier.
func TestK8sExporterName(t *testing.T) {
	ke := &K8sExporter{}
	assert.Equal(t, ke.Name(), "k8sExporter")
}

// TestK8sExporterHandleNilNode rejects messages when node is unset.
func TestK8sExporterHandleNilNode(t *testing.T) {
	ke := &K8sExporter{}
	err := ke.Handle(&types.MonitorMessage{Id: "safe.nil", StatusCode: types.StatusError})
	assert.ErrorContains(t, err, "empty")
}

// TestGenerateAddConditionsCreatesCondition appends a new node condition.
func TestGenerateAddConditionsCreatesCondition(t *testing.T) {
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n1"}}
	msg := &types.MonitorMessage{Id: "safe.add", Value: "fault"}
	conds, changed := generateAddConditions(node, msg)
	assert.Equal(t, changed, true)
	assert.Equal(t, len(conds), 1)
	assert.Equal(t, conds[0].Status, corev1.ConditionTrue)
}

// TestGenerateAddConditionsAlreadyTrue skips update when condition is already true.
func TestGenerateAddConditionsAlreadyTrue(t *testing.T) {
	key := commonfaults.GenerateTaintKey("safe.dup")
	node := &corev1.Node{
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{{
				Type:   corev1.NodeConditionType(key),
				Status: corev1.ConditionTrue,
			}},
		},
	}
	_, changed := generateAddConditions(node, &types.MonitorMessage{Id: "safe.dup", Value: "x"})
	assert.Equal(t, changed, false)
}

// TestGenerateAddConditionsUpdatesExisting flips an existing condition to true.
func TestGenerateAddConditionsUpdatesExisting(t *testing.T) {
	key := commonfaults.GenerateTaintKey("safe.flip")
	node := &corev1.Node{
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{{
				Type:   corev1.NodeConditionType(key),
				Status: corev1.ConditionFalse,
			}},
		},
	}
	conds, changed := generateAddConditions(node, &types.MonitorMessage{Id: "safe.flip", Value: "err"})
	assert.Equal(t, changed, true)
	assert.Equal(t, conds[0].Status, corev1.ConditionTrue)
}

// TestGenerateDeleteConditionsMissing returns false when condition is absent.
func TestGenerateDeleteConditionsMissing(t *testing.T) {
	_, changed := generateDeleteConditions(&corev1.Node{}, &types.MonitorMessage{Id: "safe.none"})
	assert.Equal(t, changed, false)
}

// TestK8sExporterHandleOk removes conditions for successful monitor messages.
func TestK8sExporterHandleOk(t *testing.T) {
	_, n := newExporterManager(t)
	key := commonfaults.GenerateTaintKey("safe.ok")
	assert.NilError(t, n.UpdateConditions([]corev1.NodeCondition{{
		Type:   corev1.NodeConditionType(key),
		Status: corev1.ConditionTrue,
	}}))
	ke := &K8sExporter{node: n}
	err := ke.Handle(&types.MonitorMessage{Id: "safe.ok", StatusCode: types.StatusOk})
	assert.NilError(t, err)
	assert.Equal(t, len(n.GetK8sNode().Status.Conditions), 0)
}

// TestK8sExporterHandleDisable removes conditions for non-error statuses.
func TestK8sExporterHandleDisable(t *testing.T) {
	manager, n := newExporterManager(t)
	key := commonfaults.GenerateTaintKey("safe.disable")
	assert.NilError(t, n.UpdateConditions([]corev1.NodeCondition{{
		Type:   corev1.NodeConditionType(key),
		Status: corev1.ConditionTrue,
	}}))
	ke := &K8sExporter{node: n}
	err := ke.Handle(&types.MonitorMessage{Id: "safe.disable", StatusCode: types.StatusDisable})
	assert.NilError(t, err)
	assert.Equal(t, len(n.GetK8sNode().Status.Conditions), 0)
	_ = manager
}
