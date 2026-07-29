/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package daemon

import (
	"context"
	"flag"
	"os"
	"path/filepath"
	"testing"
	"time"

	"gotest.tools/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/util/workqueue"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	"github.com/AMD-AIG-AIMA/SAFE/node-agent/pkg/exporters"
	"github.com/AMD-AIG-AIMA/SAFE/node-agent/pkg/monitors"
	"github.com/AMD-AIG-AIMA/SAFE/node-agent/pkg/node"
	"github.com/AMD-AIG-AIMA/SAFE/node-agent/pkg/types"
)

// TestDaemonInitConfig loads yaml config from the config map directory.
func TestDaemonInitConfig(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, types.AppConfig)
	assert.NilError(t, os.WriteFile(cfg, []byte("log_level: info\n"), 0644))
	d := &Daemon{}
	assert.NilError(t, d.initConfig(dir))
}

// TestDaemonInitConfigMissingFile returns error when config file is absent.
func TestDaemonInitConfigMissingFile(t *testing.T) {
	d := &Daemon{}
	err := d.initConfig(t.TempDir())
	assert.Assert(t, err != nil)
}

// TestDaemonStartWithoutInit logs and returns when daemon is not initialized.
func TestDaemonStartWithoutInit(t *testing.T) {
	d := &Daemon{}
	d.Start()
}

// TestDaemonStopWithoutComponents shuts down safely with nil subsystems.
func TestDaemonStopWithoutComponents(t *testing.T) {
	d := &Daemon{}
	d.Stop()
}

// TestDaemonStopWithMonitors shuts down the monitor manager and work queue.
func TestDaemonStopWithMonitors(t *testing.T) {
	manager, _, queue := newDaemonTestComponents(t)
	d := &Daemon{
		monitors: manager,
		queue:    queue,
		isInited: true,
	}
	d.Stop()
}

// TestDaemonStartUninitializedNode returns early when node startup fails.
func TestDaemonStartUninitializedNode(t *testing.T) {
	d := &Daemon{
		isInited: true,
		ctx:      context.Background(),
	}
	d.Start()
}

// TestDaemonStartMonitorLoadFails returns early when monitor configs cannot be loaded.
func TestDaemonStartMonitorLoadFails(t *testing.T) {
	_, n, queue := newDaemonTestComponents(t)
	opts := &types.Options{
		NodeName:      n.GetK8sNode().Name,
		ConfigMapPath: filepath.Join(t.TempDir(), "missing-config-dir"),
		ScriptPath:    t.TempDir(),
	}
	manager := monitors.NewMonitorManager(&queue, opts, n)
	d := &Daemon{
		ctx:      context.Background(),
		node:     n,
		monitors: manager,
		queue:    queue,
		isInited: true,
	}
	d.Start()
}

// TestDaemonStartCancelledContext stops when the root context is cancelled.
func TestDaemonStartCancelledContext(t *testing.T) {
	manager, n, queue := newDaemonTestComponents(t)
	ctx, cancel := context.WithCancel(context.Background())
	exp := exporters.NewExporterManager(&queue, n)
	go func() {
		time.Sleep(50 * time.Millisecond)
		queue.ShutDown()
		cancel()
	}()
	d := &Daemon{
		ctx:       ctx,
		opts:      &types.Options{NodeName: n.GetK8sNode().Name},
		queue:     queue,
		monitors:  manager,
		node:      n,
		exporters: exp,
		isInited:  true,
	}
	d.Start()
}

// --- merged from daemon_helpers_test.go ---

// newDaemonTestComponents builds monitor manager dependencies for daemon stop tests.
func newDaemonTestComponents(t *testing.T) (*monitors.MonitorManager, *node.Node, types.MonitorQueue) {
	t.Helper()
	testNode := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "daemon-node",
			Labels: map[string]string{
				common.AMDGpuIdentification: v1.TrueStr,
			},
		},
	}
	fakeClientSet := k8sfake.NewClientset(testNode)
	opts := &types.Options{NodeName: testNode.Name, ConfigMapPath: t.TempDir(), ScriptPath: t.TempDir()}
	n, err := node.NewNodeWithClientSet(context.Background(), opts, fakeClientSet)
	assert.NilError(t, err)

	var queue types.MonitorQueue
	queue = workqueue.NewTypedRateLimitingQueueWithConfig(
		workqueue.DefaultTypedControllerRateLimiter[*types.MonitorMessage](),
		workqueue.TypedRateLimitingQueueConfig[*types.MonitorMessage]{Name: "daemon-test"})
	manager := monitors.NewMonitorManager(&queue, opts, n)
	return manager, n, queue
}

// --- merged from daemon_new_test.go ---

// TestNewDaemonFailsOnNodeInit stops when the in-cluster Kubernetes client cannot be created.
func TestNewDaemonFailsOnNodeInit(t *testing.T) {
	dir := t.TempDir()
	flag.CommandLine = flag.NewFlagSet("daemon-new-test", flag.ContinueOnError)
	os.Args = []string{
		"daemon-new-test",
		"-node_name=test-node",
		"-configmap_path=" + dir,
		"-script_path=" + dir,
		"-log_file_path=" + os.DevNull,
	}
	_, err := NewDaemon()
	assert.Assert(t, err != nil)
}
