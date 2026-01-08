// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package logs

import (
	"testing"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
)

func TestCheckpointEventTracker(t *testing.T) {
	tracker := NewCheckpointEventTracker()

	if tracker == nil {
		t.Fatal("NewCheckpointEventTracker returned nil")
	}

	if tracker.pendingEvents == nil {
		t.Error("pendingEvents map not initialized")
	}
}

func TestCheckpointEventTracker_StorePendingEvent(t *testing.T) {
	tracker := NewCheckpointEventTracker()
	
	event := &model.CheckpointEvent{
		WorkloadUID: "test-workload",
		Iteration:   100,
		StartTime:   time.Now(),
	}

	tracker.storePendingEvent("test-workload", 100, event)

	retrieved := tracker.getPendingEvent("test-workload", 100)
	if retrieved == nil {
		t.Error("Failed to retrieve pending event")
	}

	if retrieved.WorkloadUID != "test-workload" {
		t.Errorf("WorkloadUID = %v, want %v", retrieved.WorkloadUID, "test-workload")
	}

	if retrieved.Iteration != 100 {
		t.Errorf("Iteration = %v, want %v", retrieved.Iteration, 100)
	}
}

func TestCheckpointEventTracker_GetPendingEvent_NotFound(t *testing.T) {
	tracker := NewCheckpointEventTracker()

	retrieved := tracker.getPendingEvent("non-existent", 999)
	if retrieved != nil {
		t.Error("Expected nil for non-existent event")
	}
}

func TestCheckpointEventTracker_ClearPendingEvent(t *testing.T) {
	tracker := NewCheckpointEventTracker()
	
	event := &model.CheckpointEvent{
		WorkloadUID: "test-workload",
		Iteration:   100,
	}

	tracker.storePendingEvent("test-workload", 100, event)
	
	// Verify it exists
	if tracker.getPendingEvent("test-workload", 100) == nil {
		t.Error("Event not stored")
	}

	// Clear it
	tracker.clearPendingEvent("test-workload", 100)

	// Verify it's gone
	if tracker.getPendingEvent("test-workload", 100) != nil {
		t.Error("Event not cleared")
	}
}

func TestCheckpointEventTracker_MakeKey(t *testing.T) {
	tracker := NewCheckpointEventTracker()

	tests := []struct {
		workloadUID string
		iteration   int
		expectedKey string
	}{
		{"workload-1", 100, "workload-1:100"},
		{"workload-2", 0, "workload-2:0"},
		{"workload-3", 999, "workload-3:999"},
	}

	for _, tt := range tests {
		t.Run(tt.expectedKey, func(t *testing.T) {
			key := tracker.makeKey(tt.workloadUID, tt.iteration)
			if key != tt.expectedKey {
				t.Errorf("makeKey() = %v, want %v", key, tt.expectedKey)
			}
		})
	}
}

func TestCheckpointEventTracker_MultipleWorkloads(t *testing.T) {
	tracker := NewCheckpointEventTracker()

	// Store events for different workloads
	event1 := &model.CheckpointEvent{WorkloadUID: "workload-1", Iteration: 100}
	event2 := &model.CheckpointEvent{WorkloadUID: "workload-2", Iteration: 100}
	event3 := &model.CheckpointEvent{WorkloadUID: "workload-1", Iteration: 200}

	tracker.storePendingEvent("workload-1", 100, event1)
	tracker.storePendingEvent("workload-2", 100, event2)
	tracker.storePendingEvent("workload-1", 200, event3)

	// Verify all events are stored independently
	retrieved1 := tracker.getPendingEvent("workload-1", 100)
	retrieved2 := tracker.getPendingEvent("workload-2", 100)
	retrieved3 := tracker.getPendingEvent("workload-1", 200)

	if retrieved1 == nil || retrieved1.WorkloadUID != "workload-1" || retrieved1.Iteration != 100 {
		t.Error("Event 1 not correctly stored/retrieved")
	}

	if retrieved2 == nil || retrieved2.WorkloadUID != "workload-2" || retrieved2.Iteration != 100 {
		t.Error("Event 2 not correctly stored/retrieved")
	}

	if retrieved3 == nil || retrieved3.WorkloadUID != "workload-1" || retrieved3.Iteration != 200 {
		t.Error("Event 3 not correctly stored/retrieved")
	}
}

func TestGetCheckpointTracker_Singleton(t *testing.T) {
	// Reset global tracker
	globalCheckpointTracker = nil

	tracker1 := getCheckpointTracker()
	tracker2 := getCheckpointTracker()

	if tracker1 != tracker2 {
		t.Error("getCheckpointTracker() should return the same instance")
	}

	if tracker1 == nil {
		t.Error("getCheckpointTracker() returned nil")
	}
}

func BenchmarkCheckpointEventTracker_StorePendingEvent(b *testing.B) {
	tracker := NewCheckpointEventTracker()
	event := &model.CheckpointEvent{
		WorkloadUID: "test-workload",
		Iteration:   100,
		StartTime:   time.Now(),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tracker.storePendingEvent("test-workload", i, event)
	}
}

func BenchmarkCheckpointEventTracker_GetPendingEvent(b *testing.B) {
	tracker := NewCheckpointEventTracker()
	event := &model.CheckpointEvent{
		WorkloadUID: "test-workload",
		Iteration:   100,
		StartTime:   time.Now(),
	}
	tracker.storePendingEvent("test-workload", 100, event)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = tracker.getPendingEvent("test-workload", 100)
	}
}

