/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package exporters

import (
	"fmt"
	"testing"
	"time"

	"gotest.tools/assert"
	"k8s.io/client-go/util/workqueue"

	"github.com/AMD-AIG-AIMA/SAFE/node-agent/pkg/types"
)

type stubExporter struct {
	err error
}

func (s *stubExporter) Handle(*types.MonitorMessage) error { return s.err }
func (s *stubExporter) Name() string                       { return "stub" }

// TestExporterManagerRegister appends custom exporters.
func TestExporterManagerRegister(t *testing.T) {
	var queue types.MonitorQueue
	queue = workqueue.NewTypedRateLimitingQueueWithConfig(
		workqueue.DefaultTypedControllerRateLimiter[*types.MonitorMessage](),
		workqueue.TypedRateLimitingQueueConfig[*types.MonitorMessage]{Name: "register"})
	m := &ExporterManager{queue: &queue}
	m.Register(&stubExporter{})
	assert.Equal(t, len(m.exporters), 1)
}

// TestExporterManagerDispatchShutdown returns true when queue is shut down.
func TestExporterManagerDispatchShutdown(t *testing.T) {
	manager, _ := newExporterManager(t)
	(*manager.queue).ShutDown()
	assert.Equal(t, manager.Dispatch(), true)
}

// TestExporterManagerDispatchRetry requeues messages when an exporter fails.
func TestExporterManagerDispatchRetry(t *testing.T) {
	var queue types.MonitorQueue
	queue = workqueue.NewTypedRateLimitingQueueWithConfig(
		workqueue.DefaultTypedControllerRateLimiter[*types.MonitorMessage](),
		workqueue.TypedRateLimitingQueueConfig[*types.MonitorMessage]{Name: "retry"})
	m := &ExporterManager{queue: &queue, exporters: []Exporter{&stubExporter{err: fmt.Errorf("fail")}}}
	msg := &types.MonitorMessage{Id: "safe.retry", StatusCode: types.StatusError}
	queue.Add(msg)
	assert.Equal(t, m.Dispatch(), false)
	assert.Assert(t, queue.NumRequeues(msg) > 0)
	queue.ShutDown()
}

// TestExporterManagerDispatchMaxRetries drops messages after exceeding retry limit.
func TestExporterManagerDispatchMaxRetries(t *testing.T) {
	var queue types.MonitorQueue
	queue = workqueue.NewTypedRateLimitingQueueWithConfig(
		workqueue.DefaultTypedControllerRateLimiter[*types.MonitorMessage](),
		workqueue.TypedRateLimitingQueueConfig[*types.MonitorMessage]{Name: "max-retry"})
	m := &ExporterManager{queue: &queue, exporters: []Exporter{&stubExporter{err: fmt.Errorf("fail")}}}
	msg := &types.MonitorMessage{Id: "safe.drop", StatusCode: types.StatusError}
	queue.Add(msg)
	got, shutdown := queue.Get()
	assert.Equal(t, shutdown, false)
	queue.Done(got)
	for i := 0; i <= maxRetries; i++ {
		queue.AddRateLimited(got)
	}
	queue.Add(got)
	assert.Equal(t, m.Dispatch(), false)
	queue.ShutDown()
}

// TestExporterManagerStartAndStop exits the dispatch loop when the queue shuts down.
func TestExporterManagerStartAndStop(t *testing.T) {
	manager, _ := newExporterManager(t)
	go func() {
		time.Sleep(20 * time.Millisecond)
		(*manager.queue).ShutDown()
	}()
	manager.Start()
	time.Sleep(100 * time.Millisecond)
	manager.Stop()
	assert.Equal(t, manager.IsExited(), true)
}

// TestExporterManagerStopIdempotent ignores repeated stop calls.
func TestExporterManagerStopIdempotent(t *testing.T) {
	manager, _ := newExporterManager(t)
	manager.isExited = true
	manager.Stop()
	assert.Equal(t, manager.IsExited(), true)
}
