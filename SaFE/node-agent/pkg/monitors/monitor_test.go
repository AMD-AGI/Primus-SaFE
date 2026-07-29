//go:build !windows

/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package monitors

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
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
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/channel"
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

// --- merged from monitor_cron_test.go ---

// TestStartCronJobInvalidSchedule exits when cron expression parsing fails.
func TestStartCronJobInvalidSchedule(t *testing.T) {
	path := "./unit-bad-cron.sh"
	assert.NilError(t, os.WriteFile(path, []byte("#!/bin/sh\nexit 0"), 0777))
	defer os.Remove(path)

	q := unitTestQueue(t)
	n := unitTestNode(t)
	conf := newMonitorConfig("safe.bad-cron", "unit-bad-cron.sh")
	conf.Cronjob = "invalid cron"
	// Skip the startup jitter sleep (up to 30s) so the invalid-schedule parse
	// error path is reached immediately within the test timeout.
	conf.IsDebug = true
	m := NewMonitor(conf, &q, n, ".")
	assert.Assert(t, m != nil)
	m.tomb = channel.NewTomb()
	done := make(chan struct{})
	go func() {
		m.startCronJob()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("startCronJob did not return on invalid schedule")
	}
}

// --- merged from monitor_run_test.go ---

// TestMonitorRun enqueues a message when the script exits successfully.
func TestMonitorRun(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "ok.sh")
	assert.NilError(t, os.WriteFile(script, []byte("#!/bin/bash\necho ok\nexit 0"), 0777))

	q := unitTestQueue(t)
	n := unitTestNode(t)
	m := &Monitor{
		config:     newMonitorConfig("safe.run", "ok.sh"),
		queue:      &q,
		scriptPath: script,
		node:       n,
	}
	m.Run()

	msg, shutdown := q.Get()
	assert.Equal(t, shutdown, false)
	assert.Equal(t, msg.Id, "safe.run")
	assert.Equal(t, msg.StatusCode, types.StatusOk)
	q.Done(msg)
}

// TestMonitorRunConsecutiveError waits until consecutive failures reach the threshold.
func TestMonitorRunConsecutiveError(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "fail.sh")
	assert.NilError(t, os.WriteFile(script, []byte("#!/bin/bash\necho err\nexit 1"), 0777))

	q := unitTestQueue(t)
	n := unitTestNode(t)
	conf := newMonitorConfig("safe.fail", "fail.sh")
	conf.ConsecutiveCount = 2
	m := &Monitor{
		config:     conf,
		queue:      &q,
		scriptPath: script,
		node:       n,
	}
	m.Run()
	assert.Equal(t, q.Len(), 0)
	m.Run()
	msg, shutdown := q.Get()
	assert.Equal(t, shutdown, false)
	assert.Equal(t, msg.StatusCode, types.StatusError)
	q.Done(msg)
}

// TestGenerateNodeInfo builds node metadata from labels and allocatable resources.
func TestGenerateNodeInfo(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "node.sh")
	assert.NilError(t, os.WriteFile(script, []byte("#!/bin/bash\nexit 0"), 0777))

	q := unitTestQueue(t)
	n := unitTestNode(t)
	m := NewMonitor(newMonitorConfig("safe.info", "node.sh"), &q, n, dir)
	assert.Assert(t, m != nil)
	info := m.generateNodeInfo()
	assert.Assert(t, info != nil)
	assert.Equal(t, info.NodeName, "unit-node")
	assert.Equal(t, info.ObservedGpuCount, 2)
}

// TestGenerateNodeInfoNilNode returns nil when the node reference is missing.
func TestGenerateNodeInfoNilNode(t *testing.T) {
	m := &Monitor{node: nil}
	assert.Assert(t, m.generateNodeInfo() == nil)
}

// --- merged from monitor_run_unknown_test.go ---

// TestMonitorRunSkipsUnknownStatus ignores non-reportable script exit codes.
func TestMonitorRunSkipsUnknownStatus(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "unknown.sh")
	// exit 2 maps to StatusUnknown, which Run must discard (only Ok/Error are reported).
	assert.NilError(t, os.WriteFile(script, []byte("#!/bin/sh\nexit 2"), 0777))

	q := unitTestQueue(t)
	n := unitTestNode(t)
	m := &Monitor{
		config:     newMonitorConfig("safe.unknown", "unknown.sh"),
		queue:      &q,
		scriptPath: script,
		node:       n,
	}
	m.Run()
	assert.Equal(t, q.Len(), 0)
}

// TestMonitorRunProcessesArguments expands reserved words before executing the script.
func TestMonitorRunProcessesArguments(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "args.sh")
	assert.NilError(t, os.WriteFile(script, []byte("#!/bin/sh\nexit 0"), 0777))

	q := unitTestQueue(t)
	n := unitTestNode(t)
	conf := newMonitorConfig("safe.args", "args.sh")
	conf.Arguments = []string{"$Node", "plain"}
	m := &Monitor{
		config:     conf,
		queue:      &q,
		scriptPath: script,
		node:       n,
	}
	m.Run()
	// exit 0 maps to StatusOk, so a single result message is enqueued.
	assert.Equal(t, q.Len(), 1)
}

// --- merged from monitor_unit_test.go ---

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
				common.AmdGpu:                   resource.MustParse("2"),
				corev1.ResourceEphemeralStorage: resource.MustParse("50Gi"),
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
		config:   conf,
		queue:    &q,
		node:     n,
		tomb:     nil,
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

// --- merged from monitor_watch_test.go ---

// TestMonitorManagerUpdateConfigRetries backs off when the config path cannot be watched.
func TestMonitorManagerUpdateConfigRetries(t *testing.T) {
	manager := newMonitorManager(t)
	manager.configPath = filepath.Join(t.TempDir(), "missing-subdir")

	done := make(chan struct{})
	go func() {
		manager.updateConfig()
		close(done)
	}()

	time.Sleep(1200 * time.Millisecond)
	manager.tomb.Stop()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("updateConfig did not stop")
	}
}
