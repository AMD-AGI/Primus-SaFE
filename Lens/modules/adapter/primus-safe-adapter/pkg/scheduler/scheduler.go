/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package scheduler

import (
	"context"
	"sync"
	"time"

	"k8s.io/klog/v2"
)

// Task defines the scheduled task interface
type Task interface {
	// Name returns the task name
	Name() string
	// Run executes the task
	Run(ctx context.Context) error
}

// Scheduler is the scheduled task scheduler
type Scheduler struct {
	tasks    map[string]*scheduledTask
	mu       sync.RWMutex
	stopChan chan struct{}
	wg       sync.WaitGroup
}

type scheduledTask struct {
	task     Task
	interval time.Duration
	stopChan chan struct{}
}

// NewScheduler creates a new scheduler
func NewScheduler() *Scheduler {
	return &Scheduler{
		tasks:    make(map[string]*scheduledTask),
		stopChan: make(chan struct{}),
	}
}

// AddTask adds a scheduled task
func (s *Scheduler) AddTask(task Task, interval time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()

	name := task.Name()
	if _, exists := s.tasks[name]; exists {
		klog.Warningf("Task %s already exists, will be replaced", name)
		s.removeTaskLocked(name)
	}

	st := &scheduledTask{
		task:     task,
		interval: interval,
		stopChan: make(chan struct{}),
	}
	s.tasks[name] = st

	klog.Infof("Task %s added with interval %v", name, interval)
}

// Start starts the scheduler
func (s *Scheduler) Start(ctx context.Context) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for name, st := range s.tasks {
		s.wg.Add(1)
		go s.runTask(ctx, name, st)
	}

	klog.Info("Scheduler started")
}

// Stop stops the scheduler
func (s *Scheduler) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()

	close(s.stopChan)
	for name := range s.tasks {
		s.removeTaskLocked(name)
	}

	s.wg.Wait()
	klog.Info("Scheduler stopped")
}

// removeTaskLocked removes a task (lock must be acquired first)
func (s *Scheduler) removeTaskLocked(name string) {
	if st, exists := s.tasks[name]; exists {
		close(st.stopChan)
		delete(s.tasks, name)
		klog.Infof("Task %s removed", name)
	}
}

// runTask runs a single task
func (s *Scheduler) runTask(ctx context.Context, name string, st *scheduledTask) {
	defer s.wg.Done()

	ticker := time.NewTicker(st.interval)
	defer ticker.Stop()

	klog.Infof("Task %s started with interval %v", name, st.interval)

	// Execute immediately once
	if err := st.task.Run(ctx); err != nil {
		klog.Errorf("Task %s execution failed: %v", name, err)
	}

	for {
		select {
		case <-ctx.Done():
			klog.Infof("Task %s stopped due to context cancellation", name)
			return
		case <-s.stopChan:
			klog.Infof("Task %s stopped due to scheduler shutdown", name)
			return
		case <-st.stopChan:
			klog.Infof("Task %s stopped", name)
			return
		case <-ticker.C:
			if err := st.task.Run(ctx); err != nil {
				klog.Errorf("Task %s execution failed: %v", name, err)
			}
		}
	}
}
