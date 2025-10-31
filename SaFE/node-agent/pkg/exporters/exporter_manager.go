/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package exporters

import (
	"k8s.io/klog/v2"

	"github.com/AMD-AIG-AIMA/SAFE/node-agent/pkg/node"
	"github.com/AMD-AIG-AIMA/SAFE/node-agent/pkg/types"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/channel"
)

const (
	maxRetries = 100
)

// Exporter defines the interface for handling monitor messages
type Exporter interface {
	// Handle processes a monitor message and returns any error encountered
	Handle(message *types.MonitorMessage) error
	// Name returns the name of the exporter for logging purposes
	Name() string
}

// ExporterManager manages multiple exporters and dispatches monitor messages to them
type ExporterManager struct {
	// Queue for receiving monitor messages
	queue *types.MonitorQueue
	// Used to control whether to exit the exporter manager
	tomb *channel.Tomb
	// List of registered exporters
	exporters []Exporter
	// Flag indicating if manager has exited
	isExited bool
}

// NewExporterManager creates a new ExporterManager instance and registers default exporters.
func NewExporterManager(queue *types.MonitorQueue, node *node.Node) *ExporterManager {
	m := &ExporterManager{
		queue: queue,
		tomb:  channel.NewTomb(),
	}
	m.Register(&K8sExporter{node: node})
	return m
}

// Register adds a new exporter to the manager.
func (m *ExporterManager) Register(e Exporter) {
	m.exporters = append(m.exporters, e)
}

// Start begins the message dispatching process in a separate goroutine.
func (m *ExporterManager) Start() {
	go func() {
		m.isExited = false
		defer func() {
			m.tomb.Done()
		}()
		for {
			select {
			case <-m.tomb.Stopping():
				return
			default:
				if shutdown := m.Dispatch(); shutdown {
					break
				}
			}
		}
	}()
}

// Stop terminates the exporter manager and all registered exporters.
func (m *ExporterManager) Stop() {
	if m.isExited {
		return
	}
	m.tomb.Stop()
	m.isExited = true
}

// IsExited returns whether the exporter manager has been stopped.
func (m *ExporterManager) IsExited() bool {
	return m.isExited
}

// Dispatch retrieves a message from the queue and sends it to all registered exporters.
func (m *ExporterManager) Dispatch() bool {
	message, shutdown := (*m.queue).Get()
	if shutdown {
		return true
	}
	defer (*m.queue).Done(message)

	if (*m.queue).NumRequeues(message) > maxRetries {
		klog.ErrorS(nil, "message exceeded max retries, dropping", "message", message)
		(*m.queue).Forget(message)
		return false
	}

	for i := range m.exporters {
		if err := m.exporters[i].Handle(message); err != nil {
			klog.ErrorS(err, "failed to handle message",
				"exporter", m.exporters[i].Name(), "message", message)
			(*m.queue).AddRateLimited(message)
			return false
		}
	}
	(*m.queue).Forget(message)
	return false
}
