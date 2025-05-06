/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package monitors

import (
	"testing"
	"time"

	"gotest.tools/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/util/workqueue"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	"github.com/AMD-AIG-AIMA/SAFE/node-agent/pkg/node"
	"github.com/AMD-AIG-AIMA/SAFE/node-agent/pkg/types"
)

func newNode(t *testing.T) *node.Node {
	testNode := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
			Labels: map[string]string{
				v1.NodeGpuCountLabel:        "8",
				common.AMDGpuIdentification: "true",
			},
		},
		Status: corev1.NodeStatus{
			Allocatable: corev1.ResourceList{
				common.AmdGpu: resource.MustParse("4"),
			},
		},
	}
	fakeClientSet := fake.NewClientset(testNode)
	opts := &types.Options{
		NodeName: testNode.Name,
	}
	n, err := node.NewNodeWithClientSet(opts, fakeClientSet)
	assert.NilError(t, err)
	return n
}

func newMonitorConfig(id, script string) *MonitorConfig {
	return &MonitorConfig{
		Id:               id,
		Script:           script,
		Cronjob:          "@every 1s",
		TimeoutSecond:    60,
		ConsecutiveCount: 1,
		Toggle:           "on",
	}
}

func newMonitor(t *testing.T, id, script string) *Monitor {
	var queue types.MonitorQueue
	queue = workqueue.NewTypedRateLimitingQueueWithConfig(
		workqueue.DefaultTypedControllerRateLimiter[*types.MonitorMessage](),
		workqueue.TypedRateLimitingQueueConfig[*types.MonitorMessage]{Name: "monitor"})
	n := newNode(t)
	return NewMonitorWithScript(newMonitorConfig(id, "test.sh"), &queue, n, []byte(script))
}

func TestRunWithStatusOk(t *testing.T) {
	TmpPath = "."
	monitor := newMonitor(t, "test.id", "echo hello;exit 0")
	assert.Equal(t, monitor != nil, true)
	monitor.Start()
	time.Sleep(time.Millisecond * 1100)
	monitor.Stop()
	assert.Equal(t, (*monitor.queue).Len(), 0)
	assert.Equal(t, monitor.lastStatusCode, types.StatusOk)
}

func TestRunWithStatusError(t *testing.T) {
	TmpPath = "."
	monitor := newMonitor(t, "test.id", "echo hello;exit 1")
	assert.Equal(t, monitor != nil, true)

	monitor.Start()
	time.Sleep(time.Millisecond * 1100)
	monitor.Stop()
	assert.Equal(t, (*monitor.queue).Len() > 0, true)
	message, ok := (*monitor.queue).Get()
	assert.Equal(t, ok, false)
	assert.Equal(t, message.Id, "test.id")
	assert.Equal(t, message.StatusCode, types.StatusError)
	assert.Equal(t, message.Value, "hello")
	(*monitor.queue).Done(message)
}

func TestRunWithStatusUnknown(t *testing.T) {
	TmpPath = "."
	monitor := newMonitor(t, "test.id", "echo hello;exit 2")
	assert.Equal(t, monitor != nil, true)
	monitor.Start()
	time.Sleep(time.Millisecond * 1100)
	monitor.Stop()
	assert.Equal(t, (*monitor.queue).Len(), 0)
	assert.Equal(t, monitor.lastStatusCode, types.StatusOk)
}

func TestNewNodeInfo(t *testing.T) {
	TmpPath = "."
	monitor := newMonitor(t, "test.id", "echo hello;exit 0")

	nodeInfo := monitor.genNodeInfo()
	assert.Equal(t, nodeInfo != nil, true)
	assert.Equal(t, nodeInfo.ExpectedGpuCount, 8)
	assert.Equal(t, nodeInfo.ObservedGpuCount, 4)
	assert.Equal(t, nodeInfo.NodeName, monitor.node.GetK8sNode().Name)
}
