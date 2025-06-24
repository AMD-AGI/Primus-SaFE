/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package monitors

import (
	"path/filepath"
	"time"

	"github.com/robfig/cron/v3"
	"k8s.io/klog/v2"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	commonfaults "github.com/AMD-AIG-AIMA/SAFE/common/pkg/faults"
	"github.com/AMD-AIG-AIMA/SAFE/node-agent/pkg/node"
	"github.com/AMD-AIG-AIMA/SAFE/node-agent/pkg/types"
	"github.com/AMD-AIG-AIMA/SAFE/node-agent/pkg/utils"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/channel"
	jsonutils "github.com/AMD-AIG-AIMA/SAFE/utils/pkg/json"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/timeutil"
)

type Monitor struct {
	config *MonitorConfig
	queue  *types.MonitorQueue
	// the full path of script
	scriptPath string
	// it can control whether to exit this monitor
	tomb *channel.Tomb
	// The node where the agent is currently running
	node *node.Node
	// The exit code obtained when running the script last time
	lastStatusCode int
	// The monitor result will be reported only when it remains the same for max consecutive times(as specified by the configuration)
	// It is only effective when the operation fails
	consecutiveCount int
	// Mark whether the service has exited
	isExited bool
}

type NodeInfo struct {
	// Expected GPU count on each node
	ExpectedGpuCount int `json:"expectedGpuCount"`
	// The gpu count observed by the device plugin
	ObservedGpuCount int `json:"observedGpuCount"`
	// The name of the node
	NodeName string `json:"nodeName"`
}

func NewMonitor(config *MonitorConfig,
	queue *types.MonitorQueue, node *node.Node, scriptPath string) *Monitor {
	var err error
	// read file from the specified path
	fullPath := filepath.Join(scriptPath, config.Script)
	if !utils.IsFileExist(fullPath) {
		klog.ErrorS(err, "failed to load script")
		return nil
	}

	inst := &Monitor{
		config:         config,
		queue:          queue,
		scriptPath:     fullPath,
		tomb:           channel.NewTomb(),
		node:           node,
		lastStatusCode: types.StatusOk,
		isExited:       true,
	}
	key := commonfaults.GenerateTaintKey(config.Id)
	if node != nil && node.FindConditionByType(key) != nil {
		inst.lastStatusCode = types.StatusError
	}
	return inst
}

func (m *Monitor) Start() {
	if m == nil || !m.config.IsEnable() {
		return
	}
	go m.startCronJob()
	m.isExited = false
}

func (m *Monitor) Stop() {
	if !m.IsExited() && m.tomb != nil {
		m.tomb.Stop()
	}
	m.isExited = true
}

func (m *Monitor) startCronJob() {
	start := time.Now().UTC()
	defer func() {
		klog.Infof("stop cronjob %s, duration: %v", m.config.Id, time.Since(start))
	}()

	// Create a new Cron instance. If a job is still running,
	// subsequent triggers of the same job will be skipped to avoid overlapping executions.
	c := cron.New(cron.WithChain(
		cron.SkipIfStillRunning(cron.DiscardLogger),
	))

	schedule, _, err := timeutil.ParseCronStandard(m.config.Cronjob)
	if err != nil {
		klog.ErrorS(err, "failed to parse cronjob schedule")
		return
	}
	c.Schedule(schedule, m)
	c.Start()
	klog.Infof("start cronjob %s", m.config.Id)

	<-m.tomb.Stopping()
	// Note: Once stopped, it cannot be restarted. You can only create a new one.
	c.Stop()
	m.tomb.Done()
}

func (m *Monitor) Run() {
	args := []string{m.scriptPath}
	for _, arg := range m.config.Arguments {
		if arg = m.convertReservedWord(arg); arg != "" {
			args = append(args, arg)
		}
	}

	timeout := time.Second * time.Duration(m.config.TimeoutSecond)
	statusCode, output := utils.ExecuteScript(args, timeout)
	// If the result is unknown, ignore it
	if statusCode != types.StatusOk && statusCode != types.StatusError {
		return
	}

	msg := &types.MonitorMessage{
		Id:         m.config.Id,
		StatusCode: statusCode,
		Value:      output,
	}

	if statusCode == types.StatusOk {
		if statusCode != m.lastStatusCode {
			(*m.queue).Add(msg)
		}
		m.consecutiveCount = 0
	} else {
		m.consecutiveCount++
		if m.consecutiveCount == m.config.ConsecutiveCount {
			(*m.queue).Add(msg)
		}
	}
	m.lastStatusCode = statusCode
}

func (m *Monitor) convertReservedWord(arg string) string {
	switch arg {
	case "$Node":
		info := m.genNodeInfo()
		if info == nil {
			klog.Errorf("failed to build nodeInfo")
			return ""
		}
		arg = string(jsonutils.MarshalSilently(info))
	}
	return arg
}

func (m *Monitor) genNodeInfo() *NodeInfo {
	if m.node == nil || m.node.GetK8sNode() == nil {
		return nil
	}
	info := &NodeInfo{
		NodeName:         m.node.GetK8sNode().Name,
		ExpectedGpuCount: v1.GetNodeGpuCount(m.node.GetK8sNode()),
	}
	gpuQuantity := m.node.GetGpuQuantity()
	if !gpuQuantity.IsZero() {
		info.ObservedGpuCount = int(gpuQuantity.Value())
	}
	return info
}

func (m *Monitor) IsExited() bool {
	return m.isExited
}
