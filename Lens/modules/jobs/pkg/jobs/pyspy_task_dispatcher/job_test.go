// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package pyspy_task_dispatcher

import (
	"strings"
	"testing"
	"time"
)

// TestJobConstants tests job constants
func TestJobConstants(t *testing.T) {
	tests := []struct {
		name     string
		constant interface{}
		expected interface{}
	}{
		{
			name:     "LockDuration",
			constant: LockDuration,
			expected: 10 * time.Minute,
		},
		{
			name:     "JobSchedule",
			constant: JobSchedule,
			expected: "@every 5s",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.constant != tt.expected {
				t.Errorf("Expected %v, got %v", tt.expected, tt.constant)
			}
		})
	}
}

// TestGenerateInstanceID tests instance ID generation
func TestGenerateInstanceID(t *testing.T) {
	// Generate multiple IDs and verify uniqueness
	ids := make(map[string]bool)
	for i := 0; i < 100; i++ {
		id := generateInstanceID()
		
		// Check format
		if !strings.HasPrefix(id, "pyspy-dispatcher-") {
			t.Errorf("Expected prefix 'pyspy-dispatcher-', got %s", id)
		}
		
		// Check length (prefix + 8 char UUID)
		expectedLen := len("pyspy-dispatcher-") + 8
		if len(id) != expectedLen {
			t.Errorf("Expected length %d, got %d for id %s", expectedLen, len(id), id)
		}
		
		// Check uniqueness
		if ids[id] {
			t.Errorf("Duplicate ID generated: %s", id)
		}
		ids[id] = true
	}
}

// TestPySpyTaskDispatcherJob_Schedule tests the Schedule method
func TestPySpyTaskDispatcherJob_Schedule(t *testing.T) {
	job := &PySpyTaskDispatcherJob{}
	schedule := job.Schedule()
	
	if schedule != JobSchedule {
		t.Errorf("Expected schedule %s, got %s", JobSchedule, schedule)
	}
	if schedule != "@every 5s" {
		t.Errorf("Expected '@every 5s', got %s", schedule)
	}
}

// TestPySpyTaskDispatcherJob_Fields tests job struct fields
func TestPySpyTaskDispatcherJob_Fields(t *testing.T) {
	job := &PySpyTaskDispatcherJob{
		instanceID: "test-instance-123",
	}

	if job.instanceID != "test-instance-123" {
		t.Errorf("Expected instanceID 'test-instance-123', got %s", job.instanceID)
	}
	if job.facade != nil {
		// facade is nil when not initialized
	}
	if job.storageBackend != nil {
		// storageBackend is nil when not initialized
	}
}

// TestInstanceIDFormat tests instance ID format validation
func TestInstanceIDFormat(t *testing.T) {
	// Generate multiple IDs
	for i := 0; i < 10; i++ {
		id := generateInstanceID()
		
		// Should have format: pyspy-dispatcher-xxxxxxxx
		parts := strings.Split(id, "-")
		if len(parts) != 3 {
			t.Errorf("Expected 3 parts separated by -, got %d in %s", len(parts), id)
		}
		
		if parts[0] != "pyspy" {
			t.Errorf("Expected first part 'pyspy', got %s", parts[0])
		}
		if parts[1] != "dispatcher" {
			t.Errorf("Expected second part 'dispatcher', got %s", parts[1])
		}
		
		// Third part should be 8 character hex
		if len(parts[2]) != 8 {
			t.Errorf("Expected 8 character UUID suffix, got %d chars: %s", len(parts[2]), parts[2])
		}
	}
}

// TestLockDurationValue tests lock duration is reasonable
func TestLockDurationValue(t *testing.T) {
	// Lock duration should be greater than typical py-spy execution time
	// (which could be up to a few minutes)
	if LockDuration < 5*time.Minute {
		t.Error("Lock duration too short for typical py-spy execution")
	}
	
	// Lock duration shouldn't be excessively long
	if LockDuration > 30*time.Minute {
		t.Error("Lock duration seems excessively long")
	}
}

// TestJobScheduleFormat tests job schedule format
func TestJobScheduleFormat(t *testing.T) {
	schedule := JobSchedule
	
	// Should be a cron expression or @every format
	if !strings.HasPrefix(schedule, "@every ") && !strings.Contains(schedule, " ") {
		t.Error("Schedule should be a valid cron expression or @every format")
	}
	
	// If @every format, validate duration
	if strings.HasPrefix(schedule, "@every ") {
		duration := strings.TrimPrefix(schedule, "@every ")
		_, err := time.ParseDuration(duration)
		if err != nil {
			t.Errorf("Invalid duration in schedule: %s", duration)
		}
	}
}

// BenchmarkGenerateInstanceID benchmarks instance ID generation
func BenchmarkGenerateInstanceID(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = generateInstanceID()
	}
}

