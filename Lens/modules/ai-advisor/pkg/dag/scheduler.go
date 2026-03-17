// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package dag

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
)

const (
	tickInterval = 5 * time.Second
)

// TaskExecutor is the interface that each sub-task executor must implement.
type TaskExecutor interface {
	Execute(ctx context.Context, master *MasterTask, sub *SubTask) error
}

// DAGScheduler manages the in-memory DAG execution for intent analysis tasks.
type DAGScheduler struct {
	clusterID   string
	executors   map[TaskType]TaskExecutor
	activeTasks map[string]*MasterTask // keyed by workloadUID
	mu          sync.Mutex
	idCounter   atomic.Int64
}

// NewDAGScheduler creates a scheduler for the given cluster.
func NewDAGScheduler(clusterID string) *DAGScheduler {
	return &DAGScheduler{
		clusterID:   clusterID,
		executors:   make(map[TaskType]TaskExecutor),
		activeTasks: make(map[string]*MasterTask),
	}
}

// RegisterExecutor binds an executor to a task type.
func (s *DAGScheduler) RegisterExecutor(taskType TaskType, executor TaskExecutor) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.executors[taskType] = executor
}

// Start launches the background tick loop that drives DAG execution.
func (s *DAGScheduler) Start(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(tickInterval)
		defer ticker.Stop()

		log.Infof("DAGScheduler[%s]: started with tick interval %v", s.clusterID, tickInterval)
		for {
			select {
			case <-ctx.Done():
				log.Infof("DAGScheduler[%s]: shutting down", s.clusterID)
				return
			case <-ticker.C:
				s.tick(ctx)
			}
		}
	}()
}

// OnWorkloadDetected creates a new master task with the full DAG template.
func (s *DAGScheduler) OnWorkloadDetected(ctx context.Context, workloadUID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.activeTasks[workloadUID]; exists {
		log.Debugf("DAGScheduler: master task already exists for workload %s", workloadUID)
		return
	}

	masterID := s.idCounter.Add(1)
	subTasks := BuildDAGTemplate()
	for i, st := range subTasks {
		st.ID = masterID*100 + int64(i+1)
		st.MasterTaskID = masterID
	}

	master := &MasterTask{
		ID:          masterID,
		ClusterID:   s.clusterID,
		WorkloadUID: workloadUID,
		Status:      MasterStatusRunning,
		SubTasks:    subTasks,
		CreatedAt:   time.Now(),
	}

	s.activeTasks[workloadUID] = master
	log.Infof("DAGScheduler: created master task %d for workload %s with %d sub-tasks",
		masterID, workloadUID, len(subTasks))
}

// tick processes all active master tasks, finding and executing ready sub-tasks.
func (s *DAGScheduler) tick(ctx context.Context) {
	s.mu.Lock()
	tasks := make([]*MasterTask, 0, len(s.activeTasks))
	for _, mt := range s.activeTasks {
		if mt.Status == MasterStatusRunning {
			tasks = append(tasks, mt)
		}
	}
	s.mu.Unlock()

	for _, master := range tasks {
		taskMap := make(map[TaskType]*SubTask)
		for _, st := range master.SubTasks {
			taskMap[st.TaskType] = st
		}

		// Find ready sub-tasks
		for _, st := range master.SubTasks {
			if st.Status != SubStatusPending {
				continue
			}
			if !st.AllDependenciesMet(taskMap) {
				continue
			}
			s.executeSubTask(ctx, master, st)
		}

		// Check for master completion
		s.checkMasterCompletion(master)
	}
}

func (s *DAGScheduler) executeSubTask(ctx context.Context, master *MasterTask, sub *SubTask) {
	executor, ok := s.executors[sub.TaskType]
	if !ok {
		log.Warnf("DAGScheduler: no executor registered for task type %s", sub.TaskType)
		sub.Status = SubStatusFailed
		sub.Error = "no executor registered"
		return
	}

	now := time.Now()
	sub.Status = SubStatusRunning
	sub.StartedAt = &now

	err := executor.Execute(ctx, master, sub)

	completedAt := time.Now()
	if err != nil {
		sub.Status = SubStatusPending // retry on next tick
		sub.Error = err.Error()
		sub.StartedAt = nil
		log.Debugf("DAGScheduler: sub-task %s for workload %s returned error (will retry): %v",
			sub.TaskType, master.WorkloadUID, err)
		return
	}

	sub.Status = SubStatusCompleted
	sub.CompletedAt = &completedAt
	log.Infof("DAGScheduler: sub-task %s completed for workload %s", sub.TaskType, master.WorkloadUID)
}

func (s *DAGScheduler) checkMasterCompletion(master *MasterTask) {
	allDone := true
	anyFailed := false
	for _, st := range master.SubTasks {
		if !st.IsTerminal() {
			allDone = false
		}
		if st.Status == SubStatusFailed {
			anyFailed = true
		}
	}

	if !allDone {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if anyFailed {
		master.Status = MasterStatusFailed
	} else {
		master.Status = MasterStatusCompleted
	}

	log.Infof("DAGScheduler: master task %d for workload %s finished with status %s",
		master.ID, master.WorkloadUID, master.Status)
	delete(s.activeTasks, master.WorkloadUID)
}

// GetActiveTask returns the current master task for a workload, if any.
func (s *DAGScheduler) GetActiveTask(workloadUID string) *MasterTask {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.activeTasks[workloadUID]
}

// ActiveTaskCount returns the number of currently active master tasks.
func (s *DAGScheduler) ActiveTaskCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.activeTasks)
}
