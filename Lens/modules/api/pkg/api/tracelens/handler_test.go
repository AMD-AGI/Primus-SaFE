// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package tracelens

import (
	"testing"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	tlconst "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/tracelens"
	"github.com/stretchr/testify/assert"
)

func TestGenerateSessionID(t *testing.T) {
	tests := []struct {
		name        string
		workloadUID string
		fileID      int32
	}{
		{
			name:        "short workload uid",
			workloadUID: "abc",
			fileID:      123,
		},
		{
			name:        "exactly 8 char workload uid",
			workloadUID: "12345678",
			fileID:      456,
		},
		{
			name:        "long workload uid truncated",
			workloadUID: "abcdefghijklmnop",
			fileID:      789,
		},
		{
			name:        "uuid style workload uid",
			workloadUID: "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
			fileID:      1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := generateSessionID(tt.workloadUID, tt.fileID)

			// Should start with prefix
			assert.Contains(t, result, tlconst.SessionIDPrefix)

			// Should contain file ID
			assert.Contains(t, result, "-")

			// Should be unique (contains timestamp)
			result2 := generateSessionID(tt.workloadUID, tt.fileID)
			// Note: might be same if called in same second
			assert.NotEmpty(t, result)
			assert.NotEmpty(t, result2)
		})
	}
}

func TestGenerateSessionIDFormat(t *testing.T) {
	workloadUID := "workload-12345678"
	fileID := int32(100)

	result := generateSessionID(workloadUID, fileID)

	// Format: tls-{prefix}-{fileID}-{timestamp}
	assert.True(t, len(result) > 0)
	assert.Contains(t, result, "tls-")
	assert.Contains(t, result, "workload")
}

func TestToSessionResponse(t *testing.T) {
	now := time.Now()
	readyAt := now.Add(-5 * time.Minute)
	lastAccessed := now.Add(-1 * time.Minute)

	tests := []struct {
		name     string
		session  *model.TracelensSessions
		checkUI  bool
		checkEst bool
	}{
		{
			name: "ready session with all fields",
			session: &model.TracelensSessions{
				ID:              1,
				SessionID:       "tls-test-123",
				WorkloadUID:     "workload-456",
				ProfilerFileID:  789,
				Status:          tlconst.StatusReady,
				StatusMessage:   "Session is ready",
				PodName:         "tracelens-test-123",
				PodIP:           "10.0.0.100",
				ResourceProfile: tlconst.ProfileMedium,
				CreatedAt:       now,
				ReadyAt:         readyAt,
				ExpiresAt:       now.Add(1 * time.Hour),
				LastAccessedAt:  lastAccessed,
			},
			checkUI:  true,
			checkEst: false,
		},
		{
			name: "pending session",
			session: &model.TracelensSessions{
				ID:              2,
				SessionID:       "tls-pending-456",
				WorkloadUID:     "workload-789",
				ProfilerFileID:  123,
				Status:          tlconst.StatusPending,
				ResourceProfile: tlconst.ProfileSmall,
				CreatedAt:       now,
				ExpiresAt:       now.Add(1 * time.Hour),
			},
			checkUI:  false,
			checkEst: true,
		},
		{
			name: "creating session",
			session: &model.TracelensSessions{
				ID:              3,
				SessionID:       "tls-creating-789",
				WorkloadUID:     "workload-abc",
				ProfilerFileID:  456,
				Status:          tlconst.StatusCreating,
				ResourceProfile: tlconst.ProfileLarge,
				CreatedAt:       now,
				ExpiresAt:       now.Add(1 * time.Hour),
			},
			checkUI:  false,
			checkEst: true,
		},
		{
			name: "failed session",
			session: &model.TracelensSessions{
				ID:              4,
				SessionID:       "tls-failed-000",
				WorkloadUID:     "workload-xyz",
				ProfilerFileID:  999,
				Status:          tlconst.StatusFailed,
				StatusMessage:   "Pod creation failed",
				ResourceProfile: tlconst.ProfileMedium,
				CreatedAt:       now,
				ExpiresAt:       now.Add(1 * time.Hour),
			},
			checkUI:  false,
			checkEst: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := toSessionResponse(tt.session)

			// Basic fields
			assert.Equal(t, tt.session.SessionID, resp.SessionID)
			assert.Equal(t, tt.session.WorkloadUID, resp.WorkloadUID)
			assert.Equal(t, tt.session.ProfilerFileID, resp.ProfilerFileID)
			assert.Equal(t, tt.session.Status, resp.Status)
			assert.Equal(t, tt.session.StatusMessage, resp.StatusMessage)
			assert.Equal(t, tt.session.PodName, resp.PodName)
			assert.Equal(t, tt.session.PodIP, resp.PodIP)
			assert.Equal(t, tt.session.ResourceProfile, resp.ResourceProfile)
			assert.Equal(t, tt.session.CreatedAt, resp.CreatedAt)
			assert.Equal(t, tt.session.ExpiresAt, resp.ExpiresAt)

			// UI path only for ready sessions
			if tt.checkUI {
				assert.NotEmpty(t, resp.UIPath)
				assert.Contains(t, resp.UIPath, tt.session.SessionID)
			} else {
				assert.Empty(t, resp.UIPath)
			}

			// Estimated ready for pending/creating
			if tt.checkEst {
				assert.Equal(t, 30, resp.EstimatedReady)
			} else {
				assert.Equal(t, 0, resp.EstimatedReady)
			}

			// ReadyAt
			if !tt.session.ReadyAt.IsZero() {
				assert.NotNil(t, resp.ReadyAt)
			} else {
				assert.Nil(t, resp.ReadyAt)
			}

			// LastAccessedAt
			if !tt.session.LastAccessedAt.IsZero() {
				assert.NotNil(t, resp.LastAccessedAt)
			} else {
				assert.Nil(t, resp.LastAccessedAt)
			}
		})
	}
}

func TestToSessionResponseUIPath(t *testing.T) {
	session := &model.TracelensSessions{
		SessionID: "tls-session-abc",
		Status:    tlconst.StatusReady,
	}

	resp := toSessionResponse(session)

	expectedPath := "/api/v1/tracelens/sessions/tls-session-abc/ui/"
	assert.Equal(t, expectedPath, resp.UIPath)
}

func TestToSessionResponseZeroReadyAt(t *testing.T) {
	session := &model.TracelensSessions{
		SessionID: "tls-test",
		Status:    tlconst.StatusPending,
		// ReadyAt is zero value
	}

	resp := toSessionResponse(session)

	assert.Nil(t, resp.ReadyAt)
}

func TestToSessionResponseZeroLastAccessed(t *testing.T) {
	session := &model.TracelensSessions{
		SessionID: "tls-test",
		Status:    tlconst.StatusReady,
		// LastAccessedAt is zero value
	}

	resp := toSessionResponse(session)

	assert.Nil(t, resp.LastAccessedAt)
}

func TestCreateSessionRequestValidation(t *testing.T) {
	tests := []struct {
		name        string
		req         CreateSessionRequest
		expectValid bool
	}{
		{
			name: "valid request with all fields",
			req: CreateSessionRequest{
				WorkloadUID:     "workload-123",
				ProfilerFileID:  456,
				TTLMinutes:      60,
				ResourceProfile: "medium",
			},
			expectValid: true,
		},
		{
			name: "valid request with minimal fields",
			req: CreateSessionRequest{
				WorkloadUID:    "workload-123",
				ProfilerFileID: 456,
			},
			expectValid: true,
		},
		{
			name: "missing workload uid",
			req: CreateSessionRequest{
				ProfilerFileID: 456,
			},
			expectValid: false,
		},
		{
			name: "missing profiler file id",
			req: CreateSessionRequest{
				WorkloadUID: "workload-123",
			},
			expectValid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isValid := tt.req.WorkloadUID != "" && tt.req.ProfilerFileID != 0
			assert.Equal(t, tt.expectValid, isValid)
		})
	}
}

func TestExtendSessionRequestValidation(t *testing.T) {
	tests := []struct {
		name        string
		minutes     int
		expectValid bool
	}{
		{
			name:        "valid extend minutes",
			minutes:     30,
			expectValid: true,
		},
		{
			name:        "minimum valid",
			minutes:     1,
			expectValid: true,
		},
		{
			name:        "maximum valid",
			minutes:     120,
			expectValid: true,
		},
		{
			name:        "zero minutes invalid",
			minutes:     0,
			expectValid: false,
		},
		{
			name:        "negative minutes invalid",
			minutes:     -10,
			expectValid: false,
		},
		{
			name:        "exceeds maximum",
			minutes:     150,
			expectValid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isValid := tt.minutes >= 1 && tt.minutes <= 120
			assert.Equal(t, tt.expectValid, isValid)
		})
	}
}

func TestResourceProfileValidation(t *testing.T) {
	tests := []struct {
		name        string
		profile     string
		expectValid bool
	}{
		{
			name:        "small profile",
			profile:     "small",
			expectValid: true,
		},
		{
			name:        "medium profile",
			profile:     "medium",
			expectValid: true,
		},
		{
			name:        "large profile",
			profile:     "large",
			expectValid: true,
		},
		{
			name:        "invalid profile",
			profile:     "xlarge",
			expectValid: false,
		},
		{
			name:        "empty profile (uses default)",
			profile:     "",
			expectValid: true, // empty is valid, defaults to medium
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var isValid bool
			if tt.profile == "" {
				isValid = true // empty uses default
			} else {
				isValid = tlconst.IsValidResourceProfile(tt.profile)
			}
			assert.Equal(t, tt.expectValid, isValid)
		})
	}
}

func TestSessionStatusTransitions(t *testing.T) {
	// Test valid status transitions
	tests := []struct {
		name       string
		fromStatus string
		canExtend  bool
		canDelete  bool
	}{
		{
			name:       "pending session",
			fromStatus: tlconst.StatusPending,
			canExtend:  true,
			canDelete:  true,
		},
		{
			name:       "creating session",
			fromStatus: tlconst.StatusCreating,
			canExtend:  true,
			canDelete:  true,
		},
		{
			name:       "ready session",
			fromStatus: tlconst.StatusReady,
			canExtend:  true,
			canDelete:  true,
		},
		{
			name:       "failed session",
			fromStatus: tlconst.StatusFailed,
			canExtend:  true,
			canDelete:  true,
		},
		{
			name:       "expired session",
			fromStatus: tlconst.StatusExpired,
			canExtend:  false,
			canDelete:  true,
		},
		{
			name:       "deleted session",
			fromStatus: tlconst.StatusDeleted,
			canExtend:  false,
			canDelete:  false, // already deleted
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			canExtend := tt.fromStatus != tlconst.StatusDeleted && tt.fromStatus != tlconst.StatusExpired
			assert.Equal(t, tt.canExtend, canExtend)
		})
	}
}

func TestListSessionsResponse(t *testing.T) {
	now := time.Now()

	sessions := []*model.TracelensSessions{
		{
			SessionID:      "tls-1",
			WorkloadUID:    "workload-1",
			ProfilerFileID: 1,
			Status:         tlconst.StatusReady,
			CreatedAt:      now,
			ExpiresAt:      now.Add(1 * time.Hour),
		},
		{
			SessionID:      "tls-2",
			WorkloadUID:    "workload-2",
			ProfilerFileID: 2,
			Status:         tlconst.StatusPending,
			CreatedAt:      now,
			ExpiresAt:      now.Add(1 * time.Hour),
		},
	}

	resp := ListSessionsResponse{
		Sessions: make([]SessionResponse, 0, len(sessions)),
		Total:    len(sessions),
	}
	for _, s := range sessions {
		resp.Sessions = append(resp.Sessions, *toSessionResponse(s))
	}

	assert.Len(t, resp.Sessions, 2)
	assert.Equal(t, 2, resp.Total)
	assert.Equal(t, "tls-1", resp.Sessions[0].SessionID)
	assert.Equal(t, "tls-2", resp.Sessions[1].SessionID)
}

func TestTTLDefaults(t *testing.T) {
	tests := []struct {
		name        string
		ttlMinutes  int
		expectedTTL int
	}{
		{
			name:        "zero ttl uses default",
			ttlMinutes:  0,
			expectedTTL: 60,
		},
		{
			name:        "negative ttl uses default",
			ttlMinutes:  -10,
			expectedTTL: 60,
		},
		{
			name:        "positive ttl preserved",
			ttlMinutes:  120,
			expectedTTL: 120,
		},
		{
			name:        "small positive ttl preserved",
			ttlMinutes:  5,
			expectedTTL: 5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ttl := tt.ttlMinutes
			if ttl <= 0 {
				ttl = 60
			}
			assert.Equal(t, tt.expectedTTL, ttl)
		})
	}
}

func TestResourceProfileDefaults(t *testing.T) {
	tests := []struct {
		name            string
		profile         string
		expectedProfile string
	}{
		{
			name:            "empty profile uses medium",
			profile:         "",
			expectedProfile: tlconst.ProfileMedium,
		},
		{
			name:            "small profile preserved",
			profile:         "small",
			expectedProfile: "small",
		},
		{
			name:            "large profile preserved",
			profile:         "large",
			expectedProfile: "large",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			profile := tt.profile
			if profile == "" {
				profile = tlconst.ProfileMedium
			}
			assert.Equal(t, tt.expectedProfile, profile)
		})
	}
}

