/*
 * Copyright © AMD. 2025-2026. All rights reserved.
 */

package daemon

import (
	"fmt"
	"os"

	apiserver "k8s.io/apiserver/pkg/server"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"

	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/log"
	"github.com/AMD-AIG-AIMA/SAFE/node-agent/pkg/exporters"
	"github.com/AMD-AIG-AIMA/SAFE/node-agent/pkg/monitors"
	"github.com/AMD-AIG-AIMA/SAFE/node-agent/pkg/node"
	"github.com/AMD-AIG-AIMA/SAFE/node-agent/pkg/types"
)

type Daemon struct {
	opts      *types.Options
	queue     types.MonitorQueue
	monitors  *monitors.MonitorManager
	node      *node.Node
	exporters *exporters.ExporterManager
	logfile   *os.File
	isInited  bool
}

func NewDaemon() (*Daemon, error) {
	d := &Daemon{
		opts: &types.Options{},
		queue: workqueue.NewTypedRateLimitingQueueWithConfig(
			workqueue.DefaultTypedControllerRateLimiter[*types.MonitorMessage](),
			workqueue.TypedRateLimitingQueueConfig[*types.MonitorMessage]{Name: "daemon"}),
	}

	var err error
	if err = d.opts.Init(); err != nil {
		return nil, fmt.Errorf("failed to parse options, err: %s", err.Error())
	}
	if err = log.Init(d.opts.LogfilePath, d.opts.LogFileSize); err != nil {
		return nil, fmt.Errorf("failed to init logs. %s", err.Error())
	}
	if d.node, err = node.NewNode(d.opts); err != nil {
		return nil, fmt.Errorf("failed to init node. %s", err.Error())
	}
	d.monitors = monitors.NewMonitorManager(&d.queue, d.opts, d.node)
	d.exporters = exporters.NewExporterManager(&d.queue, d.node)
	d.isInited = true
	return d, nil
}

func (d *Daemon) Start() {
	if !d.isInited {
		klog.Errorf("Please initialize the daemon first")
		return
	}
	ctx := apiserver.SetupSignalContext()
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
	<-ctx.Done()
	klog.Infof("node-agent daemon stopped")
}

func (d *Daemon) Stop() {
	if d.node != nil {
		d.node.Stop()
	}
	if d.monitors != nil {
		d.monitors.Stop()
	}
	d.queue.ShutDown()
	if d.exporters != nil {
		d.exporters.Stop()
	}
	klog.Infof("node-agent daemon stopped")
	klog.Flush()
	if d.logfile != nil {
		if err := d.logfile.Close(); err != nil {
			klog.ErrorS(err, "failed to close log")
		}
	}
}
