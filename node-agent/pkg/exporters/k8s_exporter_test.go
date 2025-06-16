/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
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
	assert.Equal(t, k8sNode.Status.Conditions[0].Type, corev1.NodeConditionType("safe.001"))
	assert.Equal(t, k8sNode.Status.Conditions[0].Status, corev1.ConditionTrue)
	assert.Equal(t, k8sNode.Status.Conditions[0].Message, "error001")

	(*manager.queue).ShutDown()
	manager.Stop()
	assert.Equal(t, manager.IsExited(), true)
}

func TestDeleteCondition(t *testing.T) {
	manager, n := newExporterManager(t)
	err := n.UpdateConditions([]corev1.NodeCondition{{
		Type:   "safe.001",
		Status: corev1.ConditionTrue,
	}})
	assert.NilError(t, err)

	k8sNode := n.GetK8sNode()
	assert.Equal(t, len(k8sNode.Status.Conditions), 1)
	assert.Equal(t, k8sNode.Status.Conditions[0].Type, corev1.NodeConditionType("safe.001"))
	assert.Equal(t, k8sNode.Status.Conditions[0].Status, corev1.ConditionTrue)

	msg := &types.MonitorMessage{
		Id:         "safe.001",
		StatusCode: types.StatusOk,
	}
	manager.Start()
	(*manager.queue).Add(msg)

	time.Sleep(time.Millisecond * 200)
	assert.Equal(t, len(k8sNode.Status.Conditions), 0)

	(*manager.queue).ShutDown()
	manager.Stop()
	assert.Equal(t, manager.IsExited(), true)
}
