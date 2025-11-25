package database

import (
	"context"
	"testing"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
)

func TestCheckpointEventFacade_CreateAndGet(t *testing.T) {
	// This is an integration test that requires a database connection
	// Skip if database is not available
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	facade := NewCheckpointEventFacade()
	ctx := context.Background()

	// Create a test checkpoint event
	event := &model.CheckpointEvent{
		WorkloadUID:    "test-workload-" + time.Now().Format("20060102150405"),
		PodUUID:        "test-pod-uuid",
		Iteration:      100,
		CheckpointPath: "/tmp/checkpoint/iter_100",
		EventType:      "start_saving",
		StartTime:      time.Now(),
		Status:         "in_progress",
		Metadata:       make(model.ExtType),
	}
	event.Metadata["test_key"] = "test_value"

	// Create the event
	err := facade.CreateCheckpointEvent(ctx, event)
	if err != nil {
		t.Fatalf("Failed to create checkpoint event: %v", err)
	}

	if event.ID == 0 {
		t.Error("Expected event ID to be set after creation")
	}

	// Retrieve the event
	retrieved, err := facade.GetCheckpointEventByWorkloadAndIteration(ctx, event.WorkloadUID, int(event.Iteration))
	if err != nil {
		t.Fatalf("Failed to get checkpoint event: %v", err)
	}

	if retrieved == nil {
		t.Fatal("Expected to retrieve checkpoint event, got nil")
	}

	// Verify fields
	if retrieved.WorkloadUID != event.WorkloadUID {
		t.Errorf("WorkloadUID mismatch: got %v, want %v", retrieved.WorkloadUID, event.WorkloadUID)
	}

	if retrieved.Iteration != event.Iteration {
		t.Errorf("Iteration mismatch: got %v, want %v", retrieved.Iteration, event.Iteration)
	}

	if retrieved.EventType != event.EventType {
		t.Errorf("EventType mismatch: got %v, want %v", retrieved.EventType, event.EventType)
	}
}

func TestCheckpointEventFacade_ListByWorkload(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	facade := NewCheckpointEventFacade()
	ctx := context.Background()

	workloadUID := "test-workload-list-" + time.Now().Format("20060102150405")

	// Create multiple events
	events := []*model.CheckpointEvent{
		{
			WorkloadUID:    workloadUID,
			PodUUID:        "pod-1",
			Iteration:      100,
			CheckpointPath: "/tmp/checkpoint/iter_100",
			EventType:      "start_saving",
			StartTime:      time.Now(),
			Status:         "in_progress",
			Metadata:       make(model.ExtType),
		},
		{
			WorkloadUID:    workloadUID,
			PodUUID:        "pod-1",
			Iteration:      100,
			CheckpointPath: "/tmp/checkpoint/iter_100",
			EventType:      "end_saving",
			EndTime:        time.Now(),
			DurationMs:     5000,
			Status:         "success",
			Metadata:       make(model.ExtType),
		},
		{
			WorkloadUID:    workloadUID,
			PodUUID:        "pod-1",
			Iteration:      200,
			CheckpointPath: "/tmp/checkpoint/iter_200",
			EventType:      "start_saving",
			StartTime:      time.Now(),
			Status:         "in_progress",
			Metadata:       make(model.ExtType),
		},
	}

	for _, event := range events {
		if err := facade.CreateCheckpointEvent(ctx, event); err != nil {
			t.Fatalf("Failed to create checkpoint event: %v", err)
		}
	}

	// List all events for the workload
	retrieved, err := facade.ListCheckpointEventsByWorkload(ctx, workloadUID)
	if err != nil {
		t.Fatalf("Failed to list checkpoint events: %v", err)
	}

	if len(retrieved) != len(events) {
		t.Errorf("Expected %d events, got %d", len(events), len(retrieved))
	}
}

func TestCheckpointEventFacade_ListByType(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	facade := NewCheckpointEventFacade()
	ctx := context.Background()

	workloadUID := "test-workload-type-" + time.Now().Format("20060102150405")

	// Create events with different types
	events := []*model.CheckpointEvent{
		{
			WorkloadUID:    workloadUID,
			PodUUID:        "pod-1",
			Iteration:      100,
			CheckpointPath: "/tmp/checkpoint/iter_100",
			EventType:      "start_saving",
			StartTime:      time.Now(),
			Status:         "in_progress",
			Metadata:       make(model.ExtType),
		},
		{
			WorkloadUID:    workloadUID,
			PodUUID:        "pod-1",
			Iteration:      100,
			CheckpointPath: "/tmp/checkpoint/iter_100",
			EventType:      "end_saving",
			EndTime:        time.Now(),
			Status:         "success",
			Metadata:       make(model.ExtType),
		},
		{
			WorkloadUID:    workloadUID,
			PodUUID:        "pod-1",
			Iteration:      0,
			CheckpointPath: "/tmp/checkpoint/iter_0",
			EventType:      "loading",
			StartTime:      time.Now(),
			Status:         "success",
			Metadata:       make(model.ExtType),
		},
	}

	for _, event := range events {
		if err := facade.CreateCheckpointEvent(ctx, event); err != nil {
			t.Fatalf("Failed to create checkpoint event: %v", err)
		}
	}

	// List only start_saving events
	startEvents, err := facade.ListCheckpointEventsByType(ctx, workloadUID, "start_saving")
	if err != nil {
		t.Fatalf("Failed to list start_saving events: %v", err)
	}

	if len(startEvents) != 1 {
		t.Errorf("Expected 1 start_saving event, got %d", len(startEvents))
	}

	// List only end_saving events
	endEvents, err := facade.ListCheckpointEventsByType(ctx, workloadUID, "end_saving")
	if err != nil {
		t.Fatalf("Failed to list end_saving events: %v", err)
	}

	if len(endEvents) != 1 {
		t.Errorf("Expected 1 end_saving event, got %d", len(endEvents))
	}
}

func TestCheckpointEventFacade_Update(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	facade := NewCheckpointEventFacade()
	ctx := context.Background()

	// Create a test event
	event := &model.CheckpointEvent{
		WorkloadUID:    "test-workload-update-" + time.Now().Format("20060102150405"),
		PodUUID:        "pod-1",
		Iteration:      100,
		CheckpointPath: "/tmp/checkpoint/iter_100",
		EventType:      "start_saving",
		StartTime:      time.Now(),
		Status:         "in_progress",
		Metadata:       make(model.ExtType),
	}

	if err := facade.CreateCheckpointEvent(ctx, event); err != nil {
		t.Fatalf("Failed to create checkpoint event: %v", err)
	}

	// Update the event
	event.Status = "success"
	event.EndTime = time.Now()
	event.DurationMs = 5000

	if err := facade.UpdateCheckpointEvent(ctx, event); err != nil {
		t.Fatalf("Failed to update checkpoint event: %v", err)
	}

	// Retrieve and verify
	retrieved, err := facade.GetCheckpointEventByWorkloadAndIteration(ctx, event.WorkloadUID, int(event.Iteration))
	if err != nil {
		t.Fatalf("Failed to get updated event: %v", err)
	}

	if retrieved.Status != "success" {
		t.Errorf("Status not updated: got %v, want success", retrieved.Status)
	}

	if retrieved.DurationMs != 5000 {
		t.Errorf("DurationMs not updated: got %v, want 5000", retrieved.DurationMs)
	}
}

func TestCheckpointEventFacade_GetLatest(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	facade := NewCheckpointEventFacade()
	ctx := context.Background()

	workloadUID := "test-workload-latest-" + time.Now().Format("20060102150405")

	// Create events with delays
	events := []*model.CheckpointEvent{
		{
			WorkloadUID:    workloadUID,
			PodUUID:        "pod-1",
			Iteration:      100,
			CheckpointPath: "/tmp/checkpoint/iter_100",
			EventType:      "start_saving",
			StartTime:      time.Now(),
			Status:         "success",
			Metadata:       make(model.ExtType),
		},
	}

	if err := facade.CreateCheckpointEvent(ctx, events[0]); err != nil {
		t.Fatalf("Failed to create first event: %v", err)
	}

	time.Sleep(10 * time.Millisecond)

	events = append(events, &model.CheckpointEvent{
		WorkloadUID:    workloadUID,
		PodUUID:        "pod-1",
		Iteration:      200,
		CheckpointPath: "/tmp/checkpoint/iter_200",
		EventType:      "start_saving",
		StartTime:      time.Now(),
		Status:         "success",
		Metadata:       make(model.ExtType),
	})

	if err := facade.CreateCheckpointEvent(ctx, events[1]); err != nil {
		t.Fatalf("Failed to create second event: %v", err)
	}

	// Get latest event
	latest, err := facade.GetLatestCheckpointEvent(ctx, workloadUID)
	if err != nil {
		t.Fatalf("Failed to get latest event: %v", err)
	}

	if latest == nil {
		t.Fatal("Expected latest event, got nil")
	}

	// Should be the second event (iteration 200)
	if latest.Iteration != 200 {
		t.Errorf("Expected latest iteration 200, got %d", latest.Iteration)
	}
}

func TestCheckpointEventFacade_GetNonExistent(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	facade := NewCheckpointEventFacade()
	ctx := context.Background()

	// Try to get non-existent event
	event, err := facade.GetCheckpointEventByWorkloadAndIteration(ctx, "non-existent-workload", 999)
	if err != nil {
		t.Fatalf("Expected no error for non-existent event, got: %v", err)
	}

	if event != nil {
		t.Error("Expected nil for non-existent event, got non-nil")
	}
}

