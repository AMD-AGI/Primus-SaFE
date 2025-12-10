package logs

import (
	"context"
	"strconv"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
)

// CheckpointEventTracker tracks checkpoint events and matches start/end pairs
type CheckpointEventTracker struct {
	pendingEvents map[string]*model.CheckpointEvent // key: workload_uid:iteration
}

// NewCheckpointEventTracker creates a new checkpoint event tracker
func NewCheckpointEventTracker() *CheckpointEventTracker {
	return &CheckpointEventTracker{
		pendingEvents: make(map[string]*model.CheckpointEvent),
	}
}

// handleCheckpointEvent handles checkpoint-related events
func handleCheckpointEvent(
	ctx context.Context,
	workloadUID, podUUID, eventType string,
	groups map[string]string,
	logTime time.Time,
) error {
	// Extract iteration number
	iterationStr, ok := groups["Iteration"]
	if !ok {
		logrus.Warn("No iteration found in checkpoint event")
		return nil
	}

	iteration, err := strconv.Atoi(iterationStr)
	if err != nil {
		logrus.Warnf("Invalid iteration number: %s", iterationStr)
		return nil
	}

	// Extract checkpoint path
	checkpointPath := groups["Path"]

	event := &model.CheckpointEvent{
		WorkloadUID:    workloadUID,
		PodUUID:        podUUID,
		Iteration:      int32(iteration),
		CheckpointPath: checkpointPath,
		EventType:      eventType,
		CreatedAt:      logTime,
		Status:         "in_progress",
		Metadata:       make(model.ExtType),
	}

	switch eventType {
	case "start_saving":
		event.StartTime = logTime
		event.Status = "in_progress"

	case "end_saving":
		event.EndTime = logTime
		event.Status = "success"

		// Extract duration if available
		if durationStr, ok := groups["DurationMs"]; ok {
			if duration, err := strconv.ParseInt(durationStr, 10, 64); err == nil {
				event.DurationMs = duration
			}
		}

		// Calculate duration from start time if available
		if startEvent := getCheckpointTracker().getPendingEvent(workloadUID, iteration); startEvent != nil {
			event.StartTime = startEvent.StartTime
			if event.DurationMs == 0 {
				event.DurationMs = logTime.Sub(startEvent.StartTime).Milliseconds()
			}
			// Clear pending event
			getCheckpointTracker().clearPendingEvent(workloadUID, iteration)
		}

	case "loading":
		event.StartTime = logTime
		event.EndTime = logTime
		event.Status = "success"
	}

	// Check if it's a fast checkpoint
	if _, ok := groups["FastCkpt"]; ok {
		event.IsFastCkpt = true
		event.Metadata["is_fast_ckpt"] = true
	}

	// Store additional metadata
	for k, v := range groups {
		if k != "Iteration" && k != "Path" && k != "DurationMs" && k != "FastCkpt" {
			event.Metadata[k] = v
		}
	}

	// Save to database or update pending
	if eventType == "start_saving" {
		// Store as pending
		getCheckpointTracker().storePendingEvent(workloadUID, iteration, event)
	}

	// Always save the event using facade
	if err := database.GetFacade().GetCheckpointEvent().CreateCheckpointEvent(ctx, event); err != nil {
		logrus.Errorf("Failed to save checkpoint event: %v", err)
		return err
	}

	logrus.Infof("Checkpoint event saved: workload=%s, type=%s, iteration=%d, duration=%dms",
		workloadUID, eventType, iteration, event.DurationMs)

	return nil
}

// Global checkpoint tracker
var globalCheckpointTracker *CheckpointEventTracker

// getCheckpointTracker returns the global checkpoint tracker
func getCheckpointTracker() *CheckpointEventTracker {
	if globalCheckpointTracker == nil {
		globalCheckpointTracker = NewCheckpointEventTracker()
	}
	return globalCheckpointTracker
}

// storePendingEvent stores a pending checkpoint event
func (t *CheckpointEventTracker) storePendingEvent(workloadUID string, iteration int, event *model.CheckpointEvent) {
	key := t.makeKey(workloadUID, iteration)
	t.pendingEvents[key] = event
}

// getPendingEvent retrieves a pending checkpoint event
func (t *CheckpointEventTracker) getPendingEvent(workloadUID string, iteration int) *model.CheckpointEvent {
	key := t.makeKey(workloadUID, iteration)
	return t.pendingEvents[key]
}

// clearPendingEvent clears a pending checkpoint event
func (t *CheckpointEventTracker) clearPendingEvent(workloadUID string, iteration int) {
	key := t.makeKey(workloadUID, iteration)
	delete(t.pendingEvents, key)
}

// makeKey creates a unique key for workload and iteration
func (t *CheckpointEventTracker) makeKey(workloadUID string, iteration int) string {
	return workloadUID + ":" + strconv.Itoa(iteration)
}
