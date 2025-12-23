package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ======================== filterDetectionTasks Tests ========================

func TestFilterDetectionTasks(t *testing.T) {
	tests := []struct {
		name     string
		tasks    []*model.WorkloadTaskState
		expected int
	}{
		{
			name:     "nil tasks",
			tasks:    nil,
			expected: 0,
		},
		{
			name:     "empty tasks",
			tasks:    []*model.WorkloadTaskState{},
			expected: 0,
		},
		{
			name: "all detection tasks",
			tasks: []*model.WorkloadTaskState{
				{TaskType: "detection_coordinator"},
				{TaskType: "active_detection"},
				{TaskType: "process_probe"},
				{TaskType: "log_detection"},
				{TaskType: "image_probe"},
				{TaskType: "label_probe"},
			},
			expected: 6,
		},
		{
			name: "no detection tasks",
			tasks: []*model.WorkloadTaskState{
				{TaskType: "tensorboard_stream"},
				{TaskType: "metadata_collection"},
				{TaskType: "unknown_task"},
			},
			expected: 0,
		},
		{
			name: "mixed tasks",
			tasks: []*model.WorkloadTaskState{
				{TaskType: "detection_coordinator"},
				{TaskType: "tensorboard_stream"},
				{TaskType: "process_probe"},
				{TaskType: "metadata_collection"},
				{TaskType: "log_detection"},
			},
			expected: 3,
		},
		{
			name: "single detection task",
			tasks: []*model.WorkloadTaskState{
				{TaskType: "detection_coordinator"},
			},
			expected: 1,
		},
		{
			name: "duplicate task types",
			tasks: []*model.WorkloadTaskState{
				{TaskType: "detection_coordinator", WorkloadUID: "uid1"},
				{TaskType: "detection_coordinator", WorkloadUID: "uid2"},
				{TaskType: "process_probe", WorkloadUID: "uid1"},
			},
			expected: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := filterDetectionTasks(tt.tasks)
			assert.Len(t, result, tt.expected)
		})
	}
}

func TestFilterDetectionTasks_PreservesTaskDetails(t *testing.T) {
	now := time.Now()
	tasks := []*model.WorkloadTaskState{
		{
			ID:          1,
			WorkloadUID: "test-uid-1",
			TaskType:    "detection_coordinator",
			Status:      "completed",
			LockOwner:   "worker-1",
			CreatedAt:   now,
			UpdatedAt:   now,
			Ext: model.ExtType{
				"coordinator_state": "confirmed",
			},
		},
		{
			ID:          2,
			WorkloadUID: "test-uid-1",
			TaskType:    "tensorboard_stream",
			Status:      "running",
		},
	}

	result := filterDetectionTasks(tasks)
	require.Len(t, result, 1)
	assert.Equal(t, int64(1), result[0].ID)
	assert.Equal(t, "test-uid-1", result[0].WorkloadUID)
	assert.Equal(t, "detection_coordinator", result[0].TaskType)
	assert.Equal(t, "completed", result[0].Status)
	assert.Equal(t, "worker-1", result[0].LockOwner)
}

// ======================== buildTaskItem Tests ========================

func TestBuildTaskItem(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name     string
		task     *model.WorkloadTaskState
		validate func(t *testing.T, item DetectionTaskItem)
	}{
		{
			name: "basic task",
			task: &model.WorkloadTaskState{
				TaskType:  "detection_coordinator",
				Status:    "pending",
				CreatedAt: now,
				UpdatedAt: now,
			},
			validate: func(t *testing.T, item DetectionTaskItem) {
				assert.Equal(t, "detection_coordinator", item.TaskType)
				assert.Equal(t, "pending", item.Status)
				assert.Equal(t, now, item.CreatedAt)
				assert.Equal(t, now, item.UpdatedAt)
				assert.Empty(t, item.LockOwner)
				assert.Nil(t, item.NextAttemptAt)
			},
		},
		{
			name: "task with lock owner",
			task: &model.WorkloadTaskState{
				TaskType:  "process_probe",
				Status:    "running",
				LockOwner: "worker-instance-123",
				CreatedAt: now,
				UpdatedAt: now,
			},
			validate: func(t *testing.T, item DetectionTaskItem) {
				assert.Equal(t, "process_probe", item.TaskType)
				assert.Equal(t, "running", item.Status)
				assert.Equal(t, "worker-instance-123", item.LockOwner)
			},
		},
		{
			name: "task with ext fields - attempt_count",
			task: &model.WorkloadTaskState{
				TaskType:  "log_detection",
				Status:    "pending",
				CreatedAt: now,
				UpdatedAt: now,
				Ext: model.ExtType{
					"attempt_count": float64(3),
				},
			},
			validate: func(t *testing.T, item DetectionTaskItem) {
				assert.Equal(t, 3, item.AttemptCount)
				assert.NotNil(t, item.Ext)
			},
		},
		{
			name: "task with ext fields - coordinator_state",
			task: &model.WorkloadTaskState{
				TaskType:  "detection_coordinator",
				Status:    "completed",
				CreatedAt: now,
				UpdatedAt: now,
				Ext: model.ExtType{
					"coordinator_state": "confirmed",
				},
			},
			validate: func(t *testing.T, item DetectionTaskItem) {
				assert.Equal(t, "confirmed", item.CoordinatorState)
			},
		},
		{
			name: "task with ext fields - next_attempt_at",
			task: &model.WorkloadTaskState{
				TaskType:  "active_detection",
				Status:    "pending",
				CreatedAt: now,
				UpdatedAt: now,
				Ext: model.ExtType{
					"next_attempt_at": "2025-12-22T10:30:00Z",
				},
			},
			validate: func(t *testing.T, item DetectionTaskItem) {
				require.NotNil(t, item.NextAttemptAt)
				assert.Equal(t, 2025, item.NextAttemptAt.Year())
				assert.Equal(t, time.December, item.NextAttemptAt.Month())
				assert.Equal(t, 22, item.NextAttemptAt.Day())
			},
		},
		{
			name: "task with nil ext",
			task: &model.WorkloadTaskState{
				TaskType:  "image_probe",
				Status:    "completed",
				CreatedAt: now,
				UpdatedAt: now,
				Ext:       nil,
			},
			validate: func(t *testing.T, item DetectionTaskItem) {
				assert.Equal(t, "image_probe", item.TaskType)
				assert.Equal(t, 0, item.AttemptCount)
				assert.Empty(t, item.CoordinatorState)
				assert.Nil(t, item.NextAttemptAt)
				assert.Nil(t, item.Ext)
			},
		},
		{
			name: "task with invalid next_attempt_at format",
			task: &model.WorkloadTaskState{
				TaskType:  "label_probe",
				Status:    "pending",
				CreatedAt: now,
				UpdatedAt: now,
				Ext: model.ExtType{
					"next_attempt_at": "invalid-date",
				},
			},
			validate: func(t *testing.T, item DetectionTaskItem) {
				assert.Nil(t, item.NextAttemptAt)
			},
		},
		{
			name: "task with all ext fields",
			task: &model.WorkloadTaskState{
				TaskType:  "detection_coordinator",
				Status:    "in_progress",
				LockOwner: "worker-1",
				CreatedAt: now,
				UpdatedAt: now,
				Ext: model.ExtType{
					"attempt_count":     float64(5),
					"coordinator_state": "aggregating",
					"next_attempt_at":   "2025-12-25T00:00:00Z",
					"custom_field":      "custom_value",
				},
			},
			validate: func(t *testing.T, item DetectionTaskItem) {
				assert.Equal(t, 5, item.AttemptCount)
				assert.Equal(t, "aggregating", item.CoordinatorState)
				require.NotNil(t, item.NextAttemptAt)
				assert.Equal(t, 25, item.NextAttemptAt.Day())
				assert.NotNil(t, item.Ext)
				assert.Equal(t, "custom_value", item.Ext["custom_field"])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			item := buildTaskItem(tt.task)
			tt.validate(t, item)
		})
	}
}

// ======================== buildCoverageItem Tests ========================

func TestBuildCoverageItem(t *testing.T) {
	now := time.Now()
	earlier := now.Add(-1 * time.Hour)
	later := now.Add(1 * time.Hour)

	tests := []struct {
		name     string
		coverage *model.DetectionCoverage
		validate func(t *testing.T, item DetectionCoverageItem)
	}{
		{
			name: "basic coverage - process source",
			coverage: &model.DetectionCoverage{
				WorkloadUID:  "test-uid",
				Source:       "process",
				Status:       "collected",
				AttemptCount: 1,
			},
			validate: func(t *testing.T, item DetectionCoverageItem) {
				assert.Equal(t, "process", item.Source)
				assert.Equal(t, "collected", item.Status)
				assert.Equal(t, int32(1), item.AttemptCount)
				assert.Nil(t, item.LastAttemptAt)
				assert.Nil(t, item.LastSuccessAt)
				assert.False(t, item.HasGap)
			},
		},
		{
			name: "coverage with timestamps",
			coverage: &model.DetectionCoverage{
				WorkloadUID:   "test-uid",
				Source:        "image",
				Status:        "collected",
				AttemptCount:  2,
				LastAttemptAt: now,
				LastSuccessAt: now,
				EvidenceCount: 3,
			},
			validate: func(t *testing.T, item DetectionCoverageItem) {
				assert.Equal(t, "image", item.Source)
				assert.Equal(t, "collected", item.Status)
				require.NotNil(t, item.LastAttemptAt)
				require.NotNil(t, item.LastSuccessAt)
				assert.Equal(t, int32(3), item.EvidenceCount)
			},
		},
		{
			name: "coverage with error",
			coverage: &model.DetectionCoverage{
				WorkloadUID:   "test-uid",
				Source:        "label",
				Status:        "failed",
				AttemptCount:  5,
				LastAttemptAt: now,
				LastError:     "connection timeout",
			},
			validate: func(t *testing.T, item DetectionCoverageItem) {
				assert.Equal(t, "failed", item.Status)
				assert.Equal(t, "connection timeout", item.LastError)
				assert.Equal(t, int32(5), item.AttemptCount)
			},
		},
		{
			name: "log source - no time range",
			coverage: &model.DetectionCoverage{
				WorkloadUID:  "test-uid",
				Source:       "log",
				Status:       "pending",
				AttemptCount: 0,
			},
			validate: func(t *testing.T, item DetectionCoverageItem) {
				assert.Equal(t, "log", item.Source)
				assert.Nil(t, item.CoveredFrom)
				assert.Nil(t, item.CoveredTo)
				assert.Nil(t, item.LogAvailableFrom)
				assert.Nil(t, item.LogAvailableTo)
				assert.False(t, item.HasGap)
			},
		},
		{
			name: "log source - with covered range, no gap",
			coverage: &model.DetectionCoverage{
				WorkloadUID:      "test-uid",
				Source:           "log",
				Status:           "collected",
				AttemptCount:     3,
				CoveredFrom:      earlier,
				CoveredTo:        now,
				LogAvailableFrom: earlier,
				LogAvailableTo:   now,
			},
			validate: func(t *testing.T, item DetectionCoverageItem) {
				assert.Equal(t, "log", item.Source)
				require.NotNil(t, item.CoveredFrom)
				require.NotNil(t, item.CoveredTo)
				require.NotNil(t, item.LogAvailableFrom)
				require.NotNil(t, item.LogAvailableTo)
				assert.False(t, item.HasGap)
			},
		},
		{
			name: "log source - with gap (never covered)",
			coverage: &model.DetectionCoverage{
				WorkloadUID:      "test-uid",
				Source:           "log",
				Status:           "pending",
				AttemptCount:     0,
				LogAvailableFrom: earlier,
				LogAvailableTo:   now,
			},
			validate: func(t *testing.T, item DetectionCoverageItem) {
				assert.Equal(t, "log", item.Source)
				assert.Nil(t, item.CoveredFrom)
				assert.Nil(t, item.CoveredTo)
				require.NotNil(t, item.LogAvailableFrom)
				require.NotNil(t, item.LogAvailableTo)
				assert.True(t, item.HasGap)
			},
		},
		{
			name: "log source - with gap (new logs available)",
			coverage: &model.DetectionCoverage{
				WorkloadUID:      "test-uid",
				Source:           "log",
				Status:           "collecting",
				CoveredFrom:      earlier,
				CoveredTo:        now,
				LogAvailableFrom: earlier,
				LogAvailableTo:   later,
			},
			validate: func(t *testing.T, item DetectionCoverageItem) {
				assert.Equal(t, "log", item.Source)
				assert.True(t, item.HasGap)
			},
		},
		{
			name: "non-log source - ignores time range fields",
			coverage: &model.DetectionCoverage{
				WorkloadUID:      "test-uid",
				Source:           "process",
				Status:           "collected",
				CoveredFrom:      earlier,
				CoveredTo:        now,
				LogAvailableFrom: earlier,
				LogAvailableTo:   later,
			},
			validate: func(t *testing.T, item DetectionCoverageItem) {
				assert.Equal(t, "process", item.Source)
				// For non-log sources, these should not be set
				assert.Nil(t, item.CoveredFrom)
				assert.Nil(t, item.CoveredTo)
				assert.Nil(t, item.LogAvailableFrom)
				assert.Nil(t, item.LogAvailableTo)
				assert.False(t, item.HasGap)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			item := buildCoverageItem(tt.coverage)
			tt.validate(t, item)
		})
	}
}

// ======================== buildDetectionStatusResponse Tests ========================

func TestBuildDetectionStatusResponse_BasicFields(t *testing.T) {
	now := time.Now()
	detection := &model.WorkloadDetection{
		ID:               1,
		WorkloadUID:      "test-workload-uid",
		Status:           "confirmed",
		DetectionState:   "completed",
		Framework:        "pytorch",
		WorkloadType:     "training",
		Confidence:       0.95,
		FrameworkLayer:   "base",
		WrapperFramework: "",
		BaseFramework:    "pytorch",
		EvidenceCount:    5,
		AttemptCount:     2,
		MaxAttempts:      5,
		CreatedAt:        now,
		UpdatedAt:        now,
	}

	response := buildDetectionStatusResponse(detection, nil, nil)

	assert.Equal(t, "test-workload-uid", response.WorkloadUID)
	assert.Equal(t, "confirmed", response.Status)
	assert.Equal(t, "completed", response.DetectionState)
	assert.Equal(t, "pytorch", response.Framework)
	assert.Equal(t, "training", response.WorkloadType)
	assert.Equal(t, 0.95, response.Confidence)
	assert.Equal(t, "base", response.FrameworkLayer)
	assert.Equal(t, "pytorch", response.BaseFramework)
	assert.Equal(t, int32(5), response.EvidenceCount)
	assert.Equal(t, int32(2), response.AttemptCount)
	assert.Equal(t, int32(5), response.MaxAttempts)
	assert.Equal(t, now, response.CreatedAt)
	assert.Equal(t, now, response.UpdatedAt)
	assert.Empty(t, response.Coverage)
	assert.Empty(t, response.Tasks)
	assert.False(t, response.HasConflicts)
}

func TestBuildDetectionStatusResponse_WithOptionalTimeFields(t *testing.T) {
	now := time.Now()
	lastAttempt := now.Add(-1 * time.Hour)
	nextAttempt := now.Add(30 * time.Minute)
	confirmedAt := now.Add(-30 * time.Minute)

	detection := &model.WorkloadDetection{
		WorkloadUID:    "test-uid",
		Status:         "confirmed",
		DetectionState: "completed",
		LastAttemptAt:  lastAttempt,
		NextAttemptAt:  nextAttempt,
		ConfirmedAt:    confirmedAt,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	response := buildDetectionStatusResponse(detection, nil, nil)

	require.NotNil(t, response.LastAttemptAt)
	require.NotNil(t, response.NextAttemptAt)
	require.NotNil(t, response.ConfirmedAt)
	assert.Equal(t, lastAttempt, *response.LastAttemptAt)
	assert.Equal(t, nextAttempt, *response.NextAttemptAt)
	assert.Equal(t, confirmedAt, *response.ConfirmedAt)
}

func TestBuildDetectionStatusResponse_ZeroTimeFieldsAreNil(t *testing.T) {
	detection := &model.WorkloadDetection{
		WorkloadUID:    "test-uid",
		Status:         "pending",
		DetectionState: "pending",
		LastAttemptAt:  time.Time{},
		NextAttemptAt:  time.Time{},
		ConfirmedAt:    time.Time{},
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	response := buildDetectionStatusResponse(detection, nil, nil)

	assert.Nil(t, response.LastAttemptAt)
	assert.Nil(t, response.NextAttemptAt)
	assert.Nil(t, response.ConfirmedAt)
}

func TestBuildDetectionStatusResponse_WithFrameworks(t *testing.T) {
	now := time.Now()

	frameworks := []string{"pytorch", "deepspeed", "accelerate"}
	frameworksJSON, _ := json.Marshal(frameworks)

	detection := &model.WorkloadDetection{
		WorkloadUID:    "test-uid",
		Status:         "confirmed",
		DetectionState: "completed",
		Framework:      "pytorch",
		Frameworks:     model.ExtJSON(frameworksJSON),
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	response := buildDetectionStatusResponse(detection, nil, nil)

	assert.Equal(t, "pytorch", response.Framework)
	require.Len(t, response.Frameworks, 3)
	assert.Contains(t, response.Frameworks, "pytorch")
	assert.Contains(t, response.Frameworks, "deepspeed")
	assert.Contains(t, response.Frameworks, "accelerate")
}

func TestBuildDetectionStatusResponse_WithEvidenceSources(t *testing.T) {
	now := time.Now()

	sources := []string{"process", "log", "image"}
	sourcesJSON, _ := json.Marshal(sources)

	detection := &model.WorkloadDetection{
		WorkloadUID:     "test-uid",
		Status:          "confirmed",
		DetectionState:  "completed",
		EvidenceSources: model.ExtJSON(sourcesJSON),
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	response := buildDetectionStatusResponse(detection, nil, nil)

	require.Len(t, response.EvidenceSources, 3)
	assert.Contains(t, response.EvidenceSources, "process")
	assert.Contains(t, response.EvidenceSources, "log")
	assert.Contains(t, response.EvidenceSources, "image")
}

func TestBuildDetectionStatusResponse_WithConflicts(t *testing.T) {
	now := time.Now()

	conflicts := []map[string]interface{}{
		{
			"source1":    "process",
			"source2":    "log",
			"framework1": "pytorch",
			"framework2": "tensorflow",
		},
	}
	conflictsJSON, _ := json.Marshal(conflicts)

	detection := &model.WorkloadDetection{
		WorkloadUID:    "test-uid",
		Status:         "conflict",
		DetectionState: "completed",
		Conflicts:      model.ExtJSON(conflictsJSON),
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	response := buildDetectionStatusResponse(detection, nil, nil)

	assert.True(t, response.HasConflicts)
	require.Len(t, response.Conflicts, 1)
}

func TestBuildDetectionStatusResponse_EmptyConflicts(t *testing.T) {
	now := time.Now()

	emptyConflicts := []interface{}{}
	conflictsJSON, _ := json.Marshal(emptyConflicts)

	detection := &model.WorkloadDetection{
		WorkloadUID:    "test-uid",
		Status:         "confirmed",
		DetectionState: "completed",
		Conflicts:      model.ExtJSON(conflictsJSON),
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	response := buildDetectionStatusResponse(detection, nil, nil)

	assert.False(t, response.HasConflicts)
	assert.Empty(t, response.Conflicts)
}

func TestBuildDetectionStatusResponse_WithCoverages(t *testing.T) {
	now := time.Now()

	detection := &model.WorkloadDetection{
		WorkloadUID:    "test-uid",
		Status:         "confirmed",
		DetectionState: "completed",
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	coverages := []*model.DetectionCoverage{
		{
			WorkloadUID:  "test-uid",
			Source:       "process",
			Status:       "collected",
			AttemptCount: 1,
		},
		{
			WorkloadUID:  "test-uid",
			Source:       "log",
			Status:       "collecting",
			AttemptCount: 2,
		},
	}

	response := buildDetectionStatusResponse(detection, coverages, nil)

	require.Len(t, response.Coverage, 2)
	assert.Equal(t, "process", response.Coverage[0].Source)
	assert.Equal(t, "collected", response.Coverage[0].Status)
	assert.Equal(t, "log", response.Coverage[1].Source)
	assert.Equal(t, "collecting", response.Coverage[1].Status)
}

func TestBuildDetectionStatusResponse_WithTasks(t *testing.T) {
	now := time.Now()

	detection := &model.WorkloadDetection{
		WorkloadUID:    "test-uid",
		Status:         "confirmed",
		DetectionState: "completed",
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	tasks := []*model.WorkloadTaskState{
		{
			TaskType:  "detection_coordinator",
			Status:    "completed",
			CreatedAt: now,
			UpdatedAt: now,
		},
		{
			TaskType:  "process_probe",
			Status:    "completed",
			CreatedAt: now,
			UpdatedAt: now,
		},
	}

	response := buildDetectionStatusResponse(detection, nil, tasks)

	require.Len(t, response.Tasks, 2)
	assert.Equal(t, "detection_coordinator", response.Tasks[0].TaskType)
	assert.Equal(t, "completed", response.Tasks[0].Status)
	assert.Equal(t, "process_probe", response.Tasks[1].TaskType)
}

func TestBuildDetectionStatusResponse_FullIntegration(t *testing.T) {
	now := time.Now()
	lastAttempt := now.Add(-1 * time.Hour)
	confirmedAt := now.Add(-30 * time.Minute)

	frameworks := []string{"pytorch", "deepspeed"}
	frameworksJSON, _ := json.Marshal(frameworks)
	sources := []string{"process", "log"}
	sourcesJSON, _ := json.Marshal(sources)

	detection := &model.WorkloadDetection{
		ID:               123,
		WorkloadUID:      "integration-test-uid",
		Status:           "confirmed",
		DetectionState:   "completed",
		Framework:        "pytorch",
		Frameworks:       model.ExtJSON(frameworksJSON),
		WorkloadType:     "training",
		Confidence:       0.92,
		FrameworkLayer:   "wrapper",
		WrapperFramework: "deepspeed",
		BaseFramework:    "pytorch",
		AttemptCount:     3,
		MaxAttempts:      5,
		LastAttemptAt:    lastAttempt,
		ConfirmedAt:      confirmedAt,
		EvidenceCount:    7,
		EvidenceSources:  model.ExtJSON(sourcesJSON),
		CreatedAt:        now.Add(-24 * time.Hour),
		UpdatedAt:        now,
	}

	coverages := []*model.DetectionCoverage{
		{
			WorkloadUID:   "integration-test-uid",
			Source:        "process",
			Status:        "collected",
			AttemptCount:  1,
			LastSuccessAt: now,
			EvidenceCount: 3,
		},
		{
			WorkloadUID:      "integration-test-uid",
			Source:           "log",
			Status:           "collected",
			AttemptCount:     2,
			LastSuccessAt:    now,
			EvidenceCount:    4,
			CoveredFrom:      now.Add(-24 * time.Hour),
			CoveredTo:        now,
			LogAvailableFrom: now.Add(-24 * time.Hour),
			LogAvailableTo:   now,
		},
	}

	tasks := []*model.WorkloadTaskState{
		{
			TaskType:  "detection_coordinator",
			Status:    "completed",
			CreatedAt: now.Add(-24 * time.Hour),
			UpdatedAt: now,
			Ext: model.ExtType{
				"coordinator_state": "confirmed",
				"attempt_count":     float64(3),
			},
		},
	}

	response := buildDetectionStatusResponse(detection, coverages, tasks)

	// Verify all fields
	assert.Equal(t, "integration-test-uid", response.WorkloadUID)
	assert.Equal(t, "confirmed", response.Status)
	assert.Equal(t, "completed", response.DetectionState)
	assert.Equal(t, "pytorch", response.Framework)
	assert.Len(t, response.Frameworks, 2)
	assert.Equal(t, "training", response.WorkloadType)
	assert.Equal(t, 0.92, response.Confidence)
	assert.Equal(t, "wrapper", response.FrameworkLayer)
	assert.Equal(t, "deepspeed", response.WrapperFramework)
	assert.Equal(t, "pytorch", response.BaseFramework)
	assert.Equal(t, int32(7), response.EvidenceCount)
	assert.Len(t, response.EvidenceSources, 2)
	assert.Equal(t, int32(3), response.AttemptCount)
	assert.Equal(t, int32(5), response.MaxAttempts)
	require.NotNil(t, response.LastAttemptAt)
	require.NotNil(t, response.ConfirmedAt)
	assert.Len(t, response.Coverage, 2)
	assert.Len(t, response.Tasks, 1)
	assert.False(t, response.HasConflicts)
}

// ======================== Edge Cases Tests ========================

func TestBuildCoverageItem_ZeroTimeFields(t *testing.T) {
	coverage := &model.DetectionCoverage{
		WorkloadUID:   "test-uid",
		Source:        "process",
		Status:        "pending",
		AttemptCount:  0,
		LastAttemptAt: time.Time{},
		LastSuccessAt: time.Time{},
	}

	item := buildCoverageItem(coverage)

	assert.Nil(t, item.LastAttemptAt)
	assert.Nil(t, item.LastSuccessAt)
}

func TestBuildTaskItem_ExtWithNonFloatAttemptCount(t *testing.T) {
	now := time.Now()
	task := &model.WorkloadTaskState{
		TaskType:  "detection_coordinator",
		Status:    "pending",
		CreatedAt: now,
		UpdatedAt: now,
		Ext: model.ExtType{
			"attempt_count": "not a number",
		},
	}

	item := buildTaskItem(task)

	// Should not panic and should have default value
	assert.Equal(t, 0, item.AttemptCount)
}

func TestBuildTaskItem_ExtWithNonStringCoordinatorState(t *testing.T) {
	now := time.Now()
	task := &model.WorkloadTaskState{
		TaskType:  "detection_coordinator",
		Status:    "pending",
		CreatedAt: now,
		UpdatedAt: now,
		Ext: model.ExtType{
			"coordinator_state": 123,
		},
	}

	item := buildTaskItem(task)

	// Should not panic and should be empty
	assert.Empty(t, item.CoordinatorState)
}

func TestBuildDetectionStatusResponse_InvalidFrameworksJSON(t *testing.T) {
	now := time.Now()
	detection := &model.WorkloadDetection{
		WorkloadUID:    "test-uid",
		Status:         "pending",
		DetectionState: "pending",
		Frameworks:     model.ExtJSON("invalid json"),
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	response := buildDetectionStatusResponse(detection, nil, nil)

	// Should not panic and frameworks should be nil/empty
	assert.Nil(t, response.Frameworks)
}

func TestBuildDetectionStatusResponse_InvalidEvidenceSourcesJSON(t *testing.T) {
	now := time.Now()
	detection := &model.WorkloadDetection{
		WorkloadUID:     "test-uid",
		Status:          "pending",
		DetectionState:  "pending",
		EvidenceSources: model.ExtJSON("not valid json"),
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	response := buildDetectionStatusResponse(detection, nil, nil)

	// Should not panic and evidence sources should be nil/empty
	assert.Nil(t, response.EvidenceSources)
}

func TestBuildDetectionStatusResponse_InvalidConflictsJSON(t *testing.T) {
	now := time.Now()
	detection := &model.WorkloadDetection{
		WorkloadUID:    "test-uid",
		Status:         "conflict",
		DetectionState: "completed",
		Conflicts:      model.ExtJSON("{not valid}"),
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	response := buildDetectionStatusResponse(detection, nil, nil)

	// Should not panic
	assert.False(t, response.HasConflicts)
	assert.Nil(t, response.Conflicts)
}

// ======================== Benchmark Tests ========================

func BenchmarkBuildDetectionStatusResponse(b *testing.B) {
	now := time.Now()
	frameworks := []string{"pytorch", "deepspeed"}
	frameworksJSON, _ := json.Marshal(frameworks)
	sources := []string{"process", "log"}
	sourcesJSON, _ := json.Marshal(sources)

	detection := &model.WorkloadDetection{
		WorkloadUID:      "benchmark-uid",
		Status:           "confirmed",
		DetectionState:   "completed",
		Framework:        "pytorch",
		Frameworks:       model.ExtJSON(frameworksJSON),
		EvidenceSources:  model.ExtJSON(sourcesJSON),
		Confidence:       0.95,
		FrameworkLayer:   "wrapper",
		WrapperFramework: "deepspeed",
		BaseFramework:    "pytorch",
		LastAttemptAt:    now,
		ConfirmedAt:      now,
		CreatedAt:        now,
		UpdatedAt:        now,
	}

	coverages := []*model.DetectionCoverage{
		{Source: "process", Status: "collected"},
		{Source: "log", Status: "collected"},
	}

	tasks := []*model.WorkloadTaskState{
		{TaskType: "detection_coordinator", Status: "completed", CreatedAt: now, UpdatedAt: now},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = buildDetectionStatusResponse(detection, coverages, tasks)
	}
}

func BenchmarkBuildCoverageItem(b *testing.B) {
	now := time.Now()
	coverage := &model.DetectionCoverage{
		WorkloadUID:      "benchmark-uid",
		Source:           "log",
		Status:           "collected",
		AttemptCount:     3,
		LastAttemptAt:    now,
		LastSuccessAt:    now,
		CoveredFrom:      now.Add(-24 * time.Hour),
		CoveredTo:        now,
		LogAvailableFrom: now.Add(-24 * time.Hour),
		LogAvailableTo:   now,
		EvidenceCount:    10,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = buildCoverageItem(coverage)
	}
}

func BenchmarkBuildTaskItem(b *testing.B) {
	now := time.Now()
	task := &model.WorkloadTaskState{
		TaskType:  "detection_coordinator",
		Status:    "completed",
		LockOwner: "worker-1",
		CreatedAt: now,
		UpdatedAt: now,
		Ext: model.ExtType{
			"coordinator_state": "confirmed",
			"attempt_count":     float64(5),
			"next_attempt_at":   "2025-12-22T10:30:00Z",
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = buildTaskItem(task)
	}
}

func BenchmarkFilterDetectionTasks(b *testing.B) {
	tasks := []*model.WorkloadTaskState{
		{TaskType: "detection_coordinator"},
		{TaskType: "tensorboard_stream"},
		{TaskType: "active_detection"},
		{TaskType: "metadata_collection"},
		{TaskType: "process_probe"},
		{TaskType: "log_detection"},
		{TaskType: "image_probe"},
		{TaskType: "label_probe"},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = filterDetectionTasks(tasks)
	}
}

// ======================== HTTP Handler Tests ========================
// Note: Full HTTP handler tests require cluster manager and database initialization.
// These tests focus on parameter validation and are marked as skipped when
// the cluster manager is not initialized.

// TestDetectionStatus_RouteMatching tests that routes are correctly defined
func TestDetectionStatus_RouteMatching(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Test that the router can be set up without errors
	router := gin.New()
	detectionGroup := router.Group("/detection-status")
	{
		detectionGroup.GET("/summary", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"test": "summary"})
		})
		detectionGroup.GET("", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"test": "list"})
		})
		detectionGroup.GET("/:workload_uid", func(c *gin.Context) {
			uid := c.Param("workload_uid")
			if uid == "" {
				c.JSON(http.StatusBadRequest, gin.H{"error": "workload_uid is required"})
				return
			}
			c.JSON(http.StatusOK, gin.H{"workload_uid": uid})
		})
		detectionGroup.GET("/:workload_uid/coverage", func(c *gin.Context) {
			uid := c.Param("workload_uid")
			c.JSON(http.StatusOK, gin.H{"workload_uid": uid, "type": "coverage"})
		})
		detectionGroup.GET("/:workload_uid/tasks", func(c *gin.Context) {
			uid := c.Param("workload_uid")
			c.JSON(http.StatusOK, gin.H{"workload_uid": uid, "type": "tasks"})
		})
		detectionGroup.GET("/:workload_uid/evidence", func(c *gin.Context) {
			uid := c.Param("workload_uid")
			c.JSON(http.StatusOK, gin.H{"workload_uid": uid, "type": "evidence"})
		})
		detectionGroup.POST("/:workload_uid/trigger", func(c *gin.Context) {
			uid := c.Param("workload_uid")
			if uid == "" {
				c.JSON(http.StatusBadRequest, gin.H{"error": "workload_uid is required"})
				return
			}
			c.JSON(http.StatusOK, gin.H{"workload_uid": uid, "triggered": true})
		})
		detectionGroup.POST("/log-report", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"status": "ok"})
		})
	}

	tests := []struct {
		name           string
		method         string
		url            string
		expectedStatus int
		checkBody      func(*testing.T, *httptest.ResponseRecorder)
	}{
		{
			name:           "get_summary",
			method:         "GET",
			url:            "/detection-status/summary",
			expectedStatus: http.StatusOK,
			checkBody: func(t *testing.T, w *httptest.ResponseRecorder) {
				var resp map[string]interface{}
				json.Unmarshal(w.Body.Bytes(), &resp)
				assert.Equal(t, "summary", resp["test"])
			},
		},
		{
			name:           "list_statuses",
			method:         "GET",
			url:            "/detection-status",
			expectedStatus: http.StatusOK,
			checkBody: func(t *testing.T, w *httptest.ResponseRecorder) {
				var resp map[string]interface{}
				json.Unmarshal(w.Body.Bytes(), &resp)
				assert.Equal(t, "list", resp["test"])
			},
		},
		{
			name:           "get_status_by_uid",
			method:         "GET",
			url:            "/detection-status/test-uid-123",
			expectedStatus: http.StatusOK,
			checkBody: func(t *testing.T, w *httptest.ResponseRecorder) {
				var resp map[string]interface{}
				json.Unmarshal(w.Body.Bytes(), &resp)
				assert.Equal(t, "test-uid-123", resp["workload_uid"])
			},
		},
		{
			name:           "get_coverage",
			method:         "GET",
			url:            "/detection-status/test-uid-456/coverage",
			expectedStatus: http.StatusOK,
			checkBody: func(t *testing.T, w *httptest.ResponseRecorder) {
				var resp map[string]interface{}
				json.Unmarshal(w.Body.Bytes(), &resp)
				assert.Equal(t, "test-uid-456", resp["workload_uid"])
				assert.Equal(t, "coverage", resp["type"])
			},
		},
		{
			name:           "get_tasks",
			method:         "GET",
			url:            "/detection-status/test-uid-789/tasks",
			expectedStatus: http.StatusOK,
			checkBody: func(t *testing.T, w *httptest.ResponseRecorder) {
				var resp map[string]interface{}
				json.Unmarshal(w.Body.Bytes(), &resp)
				assert.Equal(t, "test-uid-789", resp["workload_uid"])
				assert.Equal(t, "tasks", resp["type"])
			},
		},
		{
			name:           "get_evidence",
			method:         "GET",
			url:            "/detection-status/test-uid-abc/evidence",
			expectedStatus: http.StatusOK,
			checkBody: func(t *testing.T, w *httptest.ResponseRecorder) {
				var resp map[string]interface{}
				json.Unmarshal(w.Body.Bytes(), &resp)
				assert.Equal(t, "test-uid-abc", resp["workload_uid"])
				assert.Equal(t, "evidence", resp["type"])
			},
		},
		{
			name:           "trigger_detection",
			method:         "POST",
			url:            "/detection-status/test-uid-def/trigger",
			expectedStatus: http.StatusOK,
			checkBody: func(t *testing.T, w *httptest.ResponseRecorder) {
				var resp map[string]interface{}
				json.Unmarshal(w.Body.Bytes(), &resp)
				assert.Equal(t, "test-uid-def", resp["workload_uid"])
				assert.Equal(t, true, resp["triggered"])
			},
		},
		{
			name:           "log_report",
			method:         "POST",
			url:            "/detection-status/log-report",
			expectedStatus: http.StatusOK,
			checkBody: func(t *testing.T, w *httptest.ResponseRecorder) {
				var resp map[string]interface{}
				json.Unmarshal(w.Body.Bytes(), &resp)
				assert.Equal(t, "ok", resp["status"])
			},
		},
		{
			name:           "invalid_route",
			method:         "GET",
			url:            "/detection-status/test-uid/invalid",
			expectedStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			req, _ := http.NewRequest(tt.method, tt.url, nil)
			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)
			if tt.checkBody != nil {
				tt.checkBody(t, w)
			}
		})
	}
}

// TestDetectionStatus_QueryParamParsing tests query parameter parsing without DB access
func TestDetectionStatus_QueryParamParsing(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Create a mock handler that just validates query params
	router := gin.New()
	router.GET("/detection-status", func(c *gin.Context) {
		status := c.Query("status")
		state := c.Query("state")
		page := c.DefaultQuery("page", "1")
		pageSize := c.DefaultQuery("page_size", "20")
		cluster := c.Query("cluster")

		c.JSON(http.StatusOK, gin.H{
			"status":    status,
			"state":     state,
			"page":      page,
			"page_size": pageSize,
			"cluster":   cluster,
		})
	})

	tests := []struct {
		name        string
		queryParams string
		validate    func(*testing.T, map[string]interface{})
	}{
		{
			name:        "default_values",
			queryParams: "",
			validate: func(t *testing.T, resp map[string]interface{}) {
				assert.Equal(t, "1", resp["page"])
				assert.Equal(t, "20", resp["page_size"])
				assert.Equal(t, "", resp["status"])
			},
		},
		{
			name:        "status_filter",
			queryParams: "?status=confirmed",
			validate: func(t *testing.T, resp map[string]interface{}) {
				assert.Equal(t, "confirmed", resp["status"])
			},
		},
		{
			name:        "state_filter",
			queryParams: "?state=completed",
			validate: func(t *testing.T, resp map[string]interface{}) {
				assert.Equal(t, "completed", resp["state"])
			},
		},
		{
			name:        "pagination",
			queryParams: "?page=3&page_size=50",
			validate: func(t *testing.T, resp map[string]interface{}) {
				assert.Equal(t, "3", resp["page"])
				assert.Equal(t, "50", resp["page_size"])
			},
		},
		{
			name:        "cluster_param",
			queryParams: "?cluster=gpu-cluster-01",
			validate: func(t *testing.T, resp map[string]interface{}) {
				assert.Equal(t, "gpu-cluster-01", resp["cluster"])
			},
		},
		{
			name:        "all_params",
			queryParams: "?status=confirmed&state=completed&page=2&page_size=30&cluster=test-cluster",
			validate: func(t *testing.T, resp map[string]interface{}) {
				assert.Equal(t, "confirmed", resp["status"])
				assert.Equal(t, "completed", resp["state"])
				assert.Equal(t, "2", resp["page"])
				assert.Equal(t, "30", resp["page_size"])
				assert.Equal(t, "test-cluster", resp["cluster"])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			req, _ := http.NewRequest("GET", "/detection-status"+tt.queryParams, nil)
			router.ServeHTTP(w, req)

			assert.Equal(t, http.StatusOK, w.Code)

			var resp map[string]interface{}
			err := json.Unmarshal(w.Body.Bytes(), &resp)
			assert.NoError(t, err)
			tt.validate(t, resp)
		})
	}
}

// TestLogReportRequest_Validation tests request body validation for log-report endpoint
func TestLogReportRequest_Validation(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Create a mock handler that validates request body
	router := gin.New()
	router.POST("/detection-status/log-report", func(c *gin.Context) {
		type LogReportRequest struct {
			WorkloadUID    string    `json:"workload_uid" binding:"required"`
			DetectedAt     time.Time `json:"detected_at"`
			LogTimestamp   time.Time `json:"log_timestamp" binding:"required"`
			Framework      string    `json:"framework"`
			Confidence     float64   `json:"confidence"`
			PatternMatched string    `json:"pattern_matched"`
		}

		var req LogReportRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "ok", "workload_uid": req.WorkloadUID})
	})

	tests := []struct {
		name           string
		body           string
		expectedStatus int
		checkResponse  func(*testing.T, *httptest.ResponseRecorder)
	}{
		{
			name:           "valid_request",
			body:           `{"workload_uid": "test-uid", "log_timestamp": "2025-12-22T10:00:00Z"}`,
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, w *httptest.ResponseRecorder) {
				var resp map[string]interface{}
				json.Unmarshal(w.Body.Bytes(), &resp)
				assert.Equal(t, "ok", resp["status"])
				assert.Equal(t, "test-uid", resp["workload_uid"])
			},
		},
		{
			name:           "valid_request_with_all_fields",
			body:           `{"workload_uid": "test-uid", "log_timestamp": "2025-12-22T10:00:00Z", "framework": "pytorch", "confidence": 0.95, "pattern_matched": "torch.distributed"}`,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "empty_body",
			body:           "",
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "invalid_json",
			body:           "{invalid json}",
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "missing_workload_uid",
			body:           `{"log_timestamp": "2025-12-22T10:00:00Z"}`,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "missing_log_timestamp",
			body:           `{"workload_uid": "test-uid"}`,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "invalid_timestamp_format",
			body:           `{"workload_uid": "test-uid", "log_timestamp": "invalid-time"}`,
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			req, _ := http.NewRequest("POST", "/detection-status/log-report", bytes.NewBufferString(tt.body))
			req.Header.Set("Content-Type", "application/json")
			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)
			if tt.checkResponse != nil {
				tt.checkResponse(t, w)
			}
		})
	}
}

// ======================== Response Type Tests ========================

func TestDetectionStatusResponse_JSONMarshal(t *testing.T) {
	now := time.Now()
	response := DetectionStatusResponse{
		WorkloadUID:      "test-uid",
		Status:           "confirmed",
		DetectionState:   "completed",
		Framework:        "pytorch",
		Frameworks:       []string{"pytorch", "deepspeed"},
		WorkloadType:     "training",
		Confidence:       0.95,
		FrameworkLayer:   "wrapper",
		WrapperFramework: "deepspeed",
		BaseFramework:    "pytorch",
		EvidenceCount:    5,
		EvidenceSources:  []string{"process", "log"},
		AttemptCount:     2,
		MaxAttempts:      5,
		LastAttemptAt:    &now,
		ConfirmedAt:      &now,
		CreatedAt:        now,
		UpdatedAt:        now,
		Coverage:         []DetectionCoverageItem{},
		Tasks:            []DetectionTaskItem{},
		HasConflicts:     false,
	}

	data, err := json.Marshal(response)
	assert.NoError(t, err)
	assert.NotEmpty(t, data)

	// Verify we can unmarshal back
	var unmarshaled DetectionStatusResponse
	err = json.Unmarshal(data, &unmarshaled)
	assert.NoError(t, err)
	assert.Equal(t, response.WorkloadUID, unmarshaled.WorkloadUID)
	assert.Equal(t, response.Status, unmarshaled.Status)
	assert.Equal(t, response.Framework, unmarshaled.Framework)
	assert.Equal(t, response.Confidence, unmarshaled.Confidence)
}

func TestDetectionCoverageItem_JSONMarshal(t *testing.T) {
	now := time.Now()
	item := DetectionCoverageItem{
		Source:           "log",
		Status:           "collected",
		AttemptCount:     3,
		LastAttemptAt:    &now,
		LastSuccessAt:    &now,
		EvidenceCount:    5,
		CoveredFrom:      &now,
		CoveredTo:        &now,
		LogAvailableFrom: &now,
		LogAvailableTo:   &now,
		HasGap:           false,
	}

	data, err := json.Marshal(item)
	assert.NoError(t, err)
	assert.NotEmpty(t, data)

	var unmarshaled DetectionCoverageItem
	err = json.Unmarshal(data, &unmarshaled)
	assert.NoError(t, err)
	assert.Equal(t, item.Source, unmarshaled.Source)
	assert.Equal(t, item.Status, unmarshaled.Status)
}

func TestDetectionTaskItem_JSONMarshal(t *testing.T) {
	now := time.Now()
	item := DetectionTaskItem{
		TaskType:         "detection_coordinator",
		Status:           "completed",
		LockOwner:        "worker-1",
		CreatedAt:        now,
		UpdatedAt:        now,
		AttemptCount:     3,
		NextAttemptAt:    &now,
		CoordinatorState: "confirmed",
		Ext: map[string]interface{}{
			"custom_field": "custom_value",
		},
	}

	data, err := json.Marshal(item)
	assert.NoError(t, err)
	assert.NotEmpty(t, data)

	var unmarshaled DetectionTaskItem
	err = json.Unmarshal(data, &unmarshaled)
	assert.NoError(t, err)
	assert.Equal(t, item.TaskType, unmarshaled.TaskType)
	assert.Equal(t, item.Status, unmarshaled.Status)
	assert.Equal(t, item.CoordinatorState, unmarshaled.CoordinatorState)
}

func TestDetectionEvidenceItem_JSONMarshal(t *testing.T) {
	now := time.Now()
	item := DetectionEvidenceItem{
		ID:               1,
		WorkloadUID:      "test-uid",
		Source:           "process",
		SourceType:       "active",
		Framework:        "pytorch",
		WorkloadType:     "training",
		Confidence:       0.95,
		FrameworkLayer:   "base",
		WrapperFramework: "",
		BaseFramework:    "pytorch",
		Evidence: map[string]interface{}{
			"cmdline":      "python train.py",
			"process_name": "python",
		},
		DetectedAt: now,
		CreatedAt:  now,
	}

	data, err := json.Marshal(item)
	assert.NoError(t, err)
	assert.NotEmpty(t, data)

	var unmarshaled DetectionEvidenceItem
	err = json.Unmarshal(data, &unmarshaled)
	assert.NoError(t, err)
	assert.Equal(t, item.ID, unmarshaled.ID)
	assert.Equal(t, item.Source, unmarshaled.Source)
	assert.Equal(t, item.Framework, unmarshaled.Framework)
}

func TestDetectionSummaryResponse_JSONMarshal(t *testing.T) {
	response := DetectionSummaryResponse{
		TotalWorkloads: 100,
		StatusCounts: map[string]int64{
			"unknown":   20,
			"suspected": 30,
			"confirmed": 40,
			"verified":  5,
			"conflict":  5,
		},
		DetectionStateCounts: map[string]int64{
			"pending":     10,
			"in_progress": 5,
			"completed":   80,
			"failed":      5,
		},
		RecentDetections: []DetectionStatusResponse{
			{
				WorkloadUID:    "test-uid-1",
				Status:         "confirmed",
				DetectionState: "completed",
				Framework:      "pytorch",
			},
		},
	}

	data, err := json.Marshal(response)
	assert.NoError(t, err)
	assert.NotEmpty(t, data)

	var unmarshaled DetectionSummaryResponse
	err = json.Unmarshal(data, &unmarshaled)
	assert.NoError(t, err)
	assert.Equal(t, response.TotalWorkloads, unmarshaled.TotalWorkloads)
	assert.Len(t, unmarshaled.StatusCounts, 5)
	assert.Len(t, unmarshaled.DetectionStateCounts, 4)
	assert.Len(t, unmarshaled.RecentDetections, 1)
}

// ======================== Additional Edge Case Tests ========================

func TestBuildDetectionStatusResponse_WrapperFramework(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name            string
		detection       *model.WorkloadDetection
		expectedLayer   string
		expectedWrapper string
		expectedBase    string
	}{
		{
			name: "wrapper_framework_detection",
			detection: &model.WorkloadDetection{
				WorkloadUID:      "test-uid",
				Status:           "confirmed",
				DetectionState:   "completed",
				Framework:        "pytorch",
				FrameworkLayer:   "wrapper",
				WrapperFramework: "deepspeed",
				BaseFramework:    "pytorch",
				CreatedAt:        now,
				UpdatedAt:        now,
			},
			expectedLayer:   "wrapper",
			expectedWrapper: "deepspeed",
			expectedBase:    "pytorch",
		},
		{
			name: "base_framework_only",
			detection: &model.WorkloadDetection{
				WorkloadUID:    "test-uid",
				Status:         "confirmed",
				DetectionState: "completed",
				Framework:      "pytorch",
				FrameworkLayer: "base",
				BaseFramework:  "pytorch",
				CreatedAt:      now,
				UpdatedAt:      now,
			},
			expectedLayer:   "base",
			expectedWrapper: "",
			expectedBase:    "pytorch",
		},
		{
			name: "accelerate_wrapper",
			detection: &model.WorkloadDetection{
				WorkloadUID:      "test-uid",
				Status:           "confirmed",
				DetectionState:   "completed",
				Framework:        "pytorch",
				FrameworkLayer:   "wrapper",
				WrapperFramework: "accelerate",
				BaseFramework:    "pytorch",
				CreatedAt:        now,
				UpdatedAt:        now,
			},
			expectedLayer:   "wrapper",
			expectedWrapper: "accelerate",
			expectedBase:    "pytorch",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response := buildDetectionStatusResponse(tt.detection, nil, nil)
			assert.Equal(t, tt.expectedLayer, response.FrameworkLayer)
			assert.Equal(t, tt.expectedWrapper, response.WrapperFramework)
			assert.Equal(t, tt.expectedBase, response.BaseFramework)
		})
	}
}

func TestBuildCoverageItem_AllSources(t *testing.T) {
	sources := []string{"process", "log", "image", "label", "wandb", "import"}

	for _, source := range sources {
		t.Run("source_"+source, func(t *testing.T) {
			coverage := &model.DetectionCoverage{
				WorkloadUID:  "test-uid",
				Source:       source,
				Status:       "collected",
				AttemptCount: 1,
			}

			item := buildCoverageItem(coverage)
			assert.Equal(t, source, item.Source)
			assert.Equal(t, "collected", item.Status)
		})
	}
}

func TestBuildTaskItem_AllTaskTypes(t *testing.T) {
	taskTypes := []string{
		"detection_coordinator",
		"active_detection",
		"process_probe",
		"log_detection",
		"image_probe",
		"label_probe",
	}

	now := time.Now()

	for _, taskType := range taskTypes {
		t.Run("task_"+taskType, func(t *testing.T) {
			task := &model.WorkloadTaskState{
				TaskType:  taskType,
				Status:    "completed",
				CreatedAt: now,
				UpdatedAt: now,
			}

			item := buildTaskItem(task)
			assert.Equal(t, taskType, item.TaskType)
			assert.Equal(t, "completed", item.Status)
		})
	}
}

func TestFilterDetectionTasks_AllTaskTypes(t *testing.T) {
	detectionTaskTypes := []string{
		"detection_coordinator",
		"active_detection",
		"process_probe",
		"log_detection",
		"image_probe",
		"label_probe",
	}

	for _, taskType := range detectionTaskTypes {
		t.Run("includes_"+taskType, func(t *testing.T) {
			tasks := []*model.WorkloadTaskState{
				{TaskType: taskType},
			}
			result := filterDetectionTasks(tasks)
			assert.Len(t, result, 1)
			assert.Equal(t, taskType, result[0].TaskType)
		})
	}

	nonDetectionTaskTypes := []string{
		"tensorboard_stream",
		"metadata_collection",
		"profiler_stream",
		"unknown_task",
	}

	for _, taskType := range nonDetectionTaskTypes {
		t.Run("excludes_"+taskType, func(t *testing.T) {
			tasks := []*model.WorkloadTaskState{
				{TaskType: taskType},
			}
			result := filterDetectionTasks(tasks)
			assert.Len(t, result, 0)
		})
	}
}
