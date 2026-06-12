/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package monitors

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

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

func unitTestNode(t *testing.T) *node.Node {
	t.Helper()
	testNode := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "unit-node",
			Labels: map[string]string{
				v1.NodeGpuCountLabel:        "4",
				common.AMDGpuIdentification: v1.TrueStr,
			},
			Annotations: map[string]string{
				v1.NodeDiskAnnotation: `{"ephemeralStorage":1024,"type":"ssd","count":2}`,
			},
		},
		Status: corev1.NodeStatus{
			Allocatable: corev1.ResourceList{
				common.AmdGpu:                      resource.MustParse("2"),
				corev1.ResourceEphemeralStorage:    resource.MustParse("50Gi"),
			},
		},
	}
	fakeClientSet := fake.NewClientset(testNode)
	opts := &types.Options{NodeName: testNode.Name}
	n, err := node.NewNodeWithClientSet(context.Background(), opts, fakeClientSet)
	assert.NilError(t, err)
	return n
}

func unitTestQueue(t *testing.T) types.MonitorQueue {
	t.Helper()
	return workqueue.NewTypedRateLimitingQueueWithConfig(
		workqueue.DefaultTypedControllerRateLimiter[*types.MonitorMessage](),
		workqueue.TypedRateLimitingQueueConfig[*types.MonitorMessage]{Name: "unit-monitor"})
}

// TestNewMonitorMissingScript returns nil when the script file does not exist.
func TestNewMonitorMissingScript(t *testing.T) {
	q := unitTestQueue(t)
	n := unitTestNode(t)
	m := NewMonitor(newMonitorConfig("safe.missing", "missing.sh"), &q, n, t.TempDir())
	assert.Assert(t, m == nil)
}

// TestMonitorStartDisabled skips cron when monitor toggle is off.
func TestMonitorStartDisabled(t *testing.T) {
	q := unitTestQueue(t)
	n := unitTestNode(t)
	conf := newMonitorConfig("safe.off", "noop.sh")
	conf.Disabled()
	m := &Monitor{
		config: conf,
		queue:  &q,
		node:   n,
		tomb:   nil,
		isExited: true,
	}
	m.Start()
	assert.Equal(t, m.IsExited(), true)
}

// TestMonitorIsExited reports the initial exited state and idempotent stop.
func TestMonitorIsExited(t *testing.T) {
	path := "./unit-exit.sh"
	assert.NilError(t, os.WriteFile(path, []byte("#!/bin/sh\nexit 0"), 0777))
	defer os.Remove(path)

	q := unitTestQueue(t)
	n := unitTestNode(t)
	m := NewMonitor(newMonitorConfig("safe.exit", "unit-exit.sh"), &q, n, ".")
	assert.Assert(t, m != nil)
	assert.Equal(t, m.IsExited(), true)
	m.Stop()
	assert.Equal(t, m.IsExited(), true)
}

// TestConvertReservedWord expands $Node into JSON node info.
func TestConvertReservedWord(t *testing.T) {
	path := "./unit-node-arg.sh"
	assert.NilError(t, os.WriteFile(path, []byte("#!/bin/sh\nexit 0"), 0777))
	defer os.Remove(path)

	q := unitTestQueue(t)
	n := unitTestNode(t)
	m := NewMonitor(newMonitorConfig("safe.node", "unit-node-arg.sh"), &q, n, ".")
	assert.Assert(t, m != nil)
	out := m.convertReservedWord("$Node")
	assert.Assert(t, strings.Contains(out, "unit-node"))
	var info NodeInfo
	assert.NilError(t, json.Unmarshal([]byte(out), &info))
	assert.Equal(t, info.NodeName, "unit-node")
	assert.Equal(t, m.convertReservedWord("plain"), "plain")
}

// TestConvertReservedWordNilNode returns empty string when node is missing.
func TestConvertReservedWordNilNode(t *testing.T) {
	m := &Monitor{node: nil}
	assert.Equal(t, m.convertReservedWord("$Node"), "")
}

// TestKlogPrintf forwards formatted messages to klog.
func TestKlogPrintf(t *testing.T) {
	klogPrintf{}.Printf("monitor %s", "ok")
}

// TestGenerateNodeInfoEmptyAnnotation tolerates missing disk annotation JSON.
func TestGenerateNodeInfoEmptyAnnotation(t *testing.T) {
	q := unitTestQueue(t)
	n := newNode(t)
	delete(n.GetK8sNode().Annotations, v1.NodeDiskAnnotation)
	path := "./unit-empty-ann.sh"
	assert.NilError(t, os.WriteFile(path, []byte("#!/bin/sh\nexit 0"), 0777))
	defer os.Remove(path)
	m := NewMonitor(newMonitorConfig("safe.ann", "unit-empty-ann.sh"), &q, n, ".")
	assert.Assert(t, m != nil)
	info := m.generateNodeInfo()
	assert.Assert(t, info != nil)
	assert.Equal(t, info.NodeName, "test-node")
}
