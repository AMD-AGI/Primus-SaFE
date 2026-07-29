/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package exporters

import (
	"context"
	"testing"
	"time"

	"gotest.tools/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/util/workqueue"

	commonfaults "github.com/AMD-AIG-AIMA/SAFE/common/pkg/faults"
	"github.com/AMD-AIG-AIMA/SAFE/node-agent/pkg/node"
	"github.com/AMD-AIG-AIMA/SAFE/node-agent/pkg/types"
)

func newNode(t *testing.T) *node.Node {
	testNode := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
		},
	}
	fakeClientSet := fake.NewClientset(testNode)
	opts := &types.Options{
		NodeName: testNode.Name,
	}
	n, err := node.NewNodeWithClientSet(context.Background(), opts, fakeClientSet)
	assert.NilError(t, err)
	return n
}

func newExporterManager(t *testing.T) (*ExporterManager, *node.Node) {
	var queue types.MonitorQueue
	queue = workqueue.NewTypedRateLimitingQueueWithConfig(
		workqueue.DefaultTypedControllerRateLimiter[*types.MonitorMessage](),
		workqueue.TypedRateLimitingQueueConfig[*types.MonitorMessage]{Name: "exporters"})
	n := newNode(t)
	return NewExporterManager(&queue, n), n
}

func TestAddCondition(t *testing.T) {
	manager, n := newExporterManager(t)
	msg := &types.MonitorMessage{
		Id:         "safe.001",
		StatusCode: types.StatusError,
		Value:      "error001",
	}
	manager.Start()
	(*manager.queue).Add(msg)
	time.Sleep(time.Millisecond * 200)

	k8sNode := n.GetK8sNode()
	assert.Equal(t, len(k8sNode.Status.Conditions), 1)
	assert.Equal(t, k8sNode.Status.Conditions[0].Type,
		corev1.NodeConditionType(commonfaults.GenerateTaintKey("safe.001")))
	assert.Equal(t, k8sNode.Status.Conditions[0].Status, corev1.ConditionTrue)
	assert.Equal(t, k8sNode.Status.Conditions[0].Message, "error001")

	(*manager.queue).ShutDown()
	manager.Stop()
	assert.Equal(t, manager.IsExited(), true)
}

func TestDeleteCondition(t *testing.T) {
	manager, n := newExporterManager(t)
	key := commonfaults.GenerateTaintKey("safe.001")
	err := n.UpdateConditions([]corev1.NodeCondition{{
		Type:   corev1.NodeConditionType(key),
		Status: corev1.ConditionTrue,
	}})
	assert.NilError(t, err)

	k8sNode := n.GetK8sNode()
	assert.Equal(t, len(k8sNode.Status.Conditions), 1)
	assert.Equal(t, k8sNode.Status.Conditions[0].Type, corev1.NodeConditionType(key))
	assert.Equal(t, k8sNode.Status.Conditions[0].Status, corev1.ConditionTrue)

	msg := &types.MonitorMessage{
		Id:         "safe.001",
		StatusCode: types.StatusOk,
	}
	manager.Start()
	(*manager.queue).Add(msg)

	time.Sleep(time.Millisecond * 200)
	k8sNode = n.GetK8sNode()
	assert.Equal(t, len(k8sNode.Status.Conditions), 0)

	(*manager.queue).ShutDown()
	manager.Stop()
	assert.Equal(t, manager.IsExited(), true)
}

// --- merged from k8s_exporter_extra_test.go ---

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
