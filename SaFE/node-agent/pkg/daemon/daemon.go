/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package daemon

import (
	"context"
	"fmt"

	"github.com/opencontainers/runtime-tools/filepath"
	apiserver "k8s.io/apiserver/pkg/server"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"

	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	commonlog "github.com/AMD-AIG-AIMA/SAFE/common/pkg/klog"
	"github.com/AMD-AIG-AIMA/SAFE/node-agent/pkg/exporters"
	"github.com/AMD-AIG-AIMA/SAFE/node-agent/pkg/monitors"
	"github.com/AMD-AIG-AIMA/SAFE/node-agent/pkg/node"
	"github.com/AMD-AIG-AIMA/SAFE/node-agent/pkg/types"
)

// Daemon represents the main daemon process for the node agent
// It coordinates monitors, exporters, and manages the overall lifecycle
type Daemon struct {
	// Context for managing lifecycle
	ctx context.Context
	// Configuration options for the daemon
	opts *types.Options
	// Work queue for monitor result
	queue types.MonitorQueue
	// Manager for all monitors
	monitors *monitors.MonitorManager
	// The node being monitored
	node *node.Node
	// Manager for all exporters
	exporters *exporters.ExporterManager
	// Flag indicating if daemon is initialized
	isInited bool
}

// NewDaemon creates and returns a new Daemon instance.
func NewDaemon() (*Daemon, error) {
	d := &Daemon{
		opts: &types.Options{},
		queue: workqueue.NewTypedRateLimitingQueueWithConfig(
			workqueue.DefaultTypedControllerRateLimiter[*types.MonitorMessage](),
			workqueue.TypedRateLimitingQueueConfig[*types.MonitorMessage]{Name: "daemon"}),
		ctx: apiserver.SetupSignalContext(),
	}

	var err error
	if err = d.opts.Init(); err != nil {
		return nil, fmt.Errorf("failed to parse options, err: %s", err.Error())
	}
	if err = commonlog.Init(d.opts.LogfilePath, d.opts.LogFileSize); err != nil {
		return nil, fmt.Errorf("failed to init logs. %s", err.Error())
	}
	if d.node, err = node.NewNode(d.ctx, d.opts); err != nil {
		return nil, fmt.Errorf("failed to init node. %s", err.Error())
	}
	if err = d.initConfig(d.opts.ConfigMapPath); err != nil {
		return nil, fmt.Errorf("failed to init config. %s", err.Error())
	}
	d.monitors = monitors.NewMonitorManager(&d.queue, d.opts, d.node)
	d.exporters = exporters.NewExporterManager(&d.queue, d.node)
	d.isInited = true
	return d, nil
}

// Start begins the daemon operation by starting all components and waiting for shutdown signal.
func (d *Daemon) Start() {
	if !d.isInited {
		klog.Errorf("Please initialize the daemon first")
		return
	}

	klog.Infof("start node-agent daemon")
	defer d.Stop()
	if err := d.node.Start(); err != nil {
		klog.ErrorS(err, "failed to start node")
		return
	}
	if err := d.monitors.Start(); err != nil {
		klog.ErrorS(err, "failed to start monitor manager")
		return
	}
	d.exporters.Start()
	<-d.ctx.Done()
	klog.Infof("node-agent daemon stopped")
}

// Stop gracefully shuts down the daemon and all its components.
func (d *Daemon) Stop() {
	if d.monitors != nil {
		d.monitors.Stop()
	}
	if d.queue != nil {
		d.queue.ShutDownWithDrain()
	}
	if d.exporters != nil {
		d.exporters.Stop()
	}
	klog.Infof("node-agent daemon stopped")
	klog.Flush()
}

// initConfig loads the server configuration from the specified config file path.
func (d *Daemon) initConfig(configPath string) error {
	fullPath := filepath.Join(configPath, types.AppConfig)
	if err := commonconfig.LoadConfig(fullPath); err != nil {
		return fmt.Errorf("config path: %s, err: %v", fullPath, err)
	}
	klog.Infof("config loaded from %s", fullPath)
	return nil
}
