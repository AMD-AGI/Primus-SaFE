//go:build !windows

/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package monitors

import (
	"os"
	"testing"
	"time"

	"gotest.tools/assert"
	"k8s.io/client-go/util/workqueue"

	"github.com/AMD-AIG-AIMA/SAFE/node-agent/pkg/types"
)

const (
	TestScriptPath = "test.sh"
)

func newMonitor(t *testing.T, id, script string) *Monitor {
	var queue types.MonitorQueue
	queue = workqueue.NewTypedRateLimitingQueueWithConfig(
		workqueue.DefaultTypedControllerRateLimiter[*types.MonitorMessage](),
		workqueue.TypedRateLimitingQueueConfig[*types.MonitorMessage]{Name: "monitor"})
	n := newNode(t)
	err := os.WriteFile(TestScriptPath, []byte(script), 0777)
	assert.NilError(t, err)
	m := NewMonitor(newMonitorConfig(id, TestScriptPath), &queue, n, ".")
	if m != nil {
		m.config.IsDebug = true
	}
	return m
}

func TestRunWithStatusOk(t *testing.T) {
	monitor := newMonitor(t, "test.id", "echo hello;exit 0")
	defer os.Remove(TestScriptPath)
	assert.Equal(t, monitor != nil, true)
	monitor.Start()
	time.Sleep(time.Millisecond * 1100)
	monitor.Stop()

	assert.Equal(t, (*monitor.queue).Len() > 0, true)
	message, ok := (*monitor.queue).Get()
	assert.Equal(t, ok, false)
	assert.Equal(t, message.Id, "test.id")
	assert.Equal(t, message.StatusCode, types.StatusOk)
	assert.Equal(t, message.Value, "hello")
	(*monitor.queue).Done(message)
}

func TestRunWithStatusError(t *testing.T) {
	monitor := newMonitor(t, "test.id", "echo hello;exit 1")
	defer os.Remove(TestScriptPath)
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
	monitor := newMonitor(t, "test.id", "echo hello;exit 2")
	defer os.Remove(TestScriptPath)
	assert.Equal(t, monitor != nil, true)
	monitor.Start()
	time.Sleep(time.Millisecond * 1100)
	monitor.Stop()
	assert.Equal(t, (*monitor.queue).Len(), 0)
}

func TestNewNodeInfo(t *testing.T) {
	monitor := newMonitor(t, "test.id", "echo hello;exit 0")
	defer os.Remove(TestScriptPath)

	nodeInfo := monitor.generateNodeInfo()
	assert.Equal(t, nodeInfo != nil, true)
	assert.Equal(t, nodeInfo.ExpectedGpuCount, 8)
	assert.Equal(t, nodeInfo.ObservedGpuCount, 4)
	assert.Equal(t, nodeInfo.NodeName, monitor.node.GetK8sNode().Name)
}
