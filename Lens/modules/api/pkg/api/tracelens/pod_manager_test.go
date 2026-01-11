// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package tracelens

import (
	"testing"

	tlconst "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/tracelens"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
)

func TestGeneratePodName(t *testing.T) {
	tests := []struct {
		name      string
		sessionID string
		expected  string
	}{
		{
			name:      "short session ID",
			sessionID: "abc123",
			expected:  "tracelens-abc123",
		},
		{
			name:      "typical session ID",
			sessionID: "tls-a1b2c3d4e5f6",
			expected:  "tracelens-tls-a1b2c3d4e5f6",
		},
		{
			name:      "long session ID truncated",
			sessionID: "tls-this-is-a-very-long-session-id-that-exceeds-the-kubernetes-pod-name-limit",
			expected:  "tracelens-tls-this-is-a-very-long-session-id-that-exceeds-the-k",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := generatePodName(tt.sessionID)
			assert.Equal(t, tt.expected, result)
			assert.LessOrEqual(t, len(result), 63, "Pod name should not exceed 63 characters")
		})
	}
}

func TestGeneratePodNameMaxLength(t *testing.T) {
	// Create a session ID that would result in a name longer than 63 characters
	longSessionID := "abcdefghijklmnopqrstuvwxyz0123456789abcdefghijklmnopqrstuvwxyz"
	result := generatePodName(longSessionID)

	assert.LessOrEqual(t, len(result), 63)
	assert.True(t, len(result) == 63 || len(result) == len("tracelens-")+len(longSessionID))
}

func TestGetResourceLimits(t *testing.T) {
	tests := []struct {
		name           string
		profile        string
		expectedMemory string
		expectedCPU    string
	}{
		{
			name:           "small profile",
			profile:        tlconst.ProfileSmall,
			expectedMemory: "8Gi",
			expectedCPU:    "1",
		},
		{
			name:           "medium profile",
			profile:        tlconst.ProfileMedium,
			expectedMemory: "16Gi",
			expectedCPU:    "2",
		},
		{
			name:           "large profile",
			profile:        tlconst.ProfileLarge,
			expectedMemory: "32Gi",
			expectedCPU:    "4",
		},
		{
			name:           "empty profile defaults to medium",
			profile:        "",
			expectedMemory: "16Gi",
			expectedCPU:    "2",
		},
		{
			name:           "unknown profile defaults to medium",
			profile:        "unknown",
			expectedMemory: "16Gi",
			expectedCPU:    "2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			memory, cpu := getResourceLimits(tt.profile)
			assert.Equal(t, tt.expectedMemory, memory)
			assert.Equal(t, tt.expectedCPU, cpu)
		})
	}
}

func TestGetPodFailureReason(t *testing.T) {
	tests := []struct {
		name     string
		pod      *corev1.Pod
		expected string
	}{
		{
			name: "pod with status message",
			pod: &corev1.Pod{
				Status: corev1.PodStatus{
					Message: "Pod failed due to resource constraints",
				},
			},
			expected: "Pod failed due to resource constraints",
		},
		{
			name: "pod with terminated container message",
			pod: &corev1.Pod{
				Status: corev1.PodStatus{
					ContainerStatuses: []corev1.ContainerStatus{
						{
							State: corev1.ContainerState{
								Terminated: &corev1.ContainerStateTerminated{
									Message: "Container terminated with error",
								},
							},
						},
					},
				},
			},
			expected: "Container terminated with error",
		},
		{
			name: "pod with waiting container message",
			pod: &corev1.Pod{
				Status: corev1.PodStatus{
					ContainerStatuses: []corev1.ContainerStatus{
						{
							State: corev1.ContainerState{
								Waiting: &corev1.ContainerStateWaiting{
									Message: "Image pull failed",
								},
							},
						},
					},
				},
			},
			expected: "Image pull failed",
		},
		{
			name: "pod with no message",
			pod: &corev1.Pod{
				Status: corev1.PodStatus{},
			},
			expected: "unknown reason",
		},
		{
			name: "pod with empty container statuses",
			pod: &corev1.Pod{
				Status: corev1.PodStatus{
					ContainerStatuses: []corev1.ContainerStatus{},
				},
			},
			expected: "unknown reason",
		},
		{
			name: "pod with running container (no error)",
			pod: &corev1.Pod{
				Status: corev1.PodStatus{
					ContainerStatuses: []corev1.ContainerStatus{
						{
							State: corev1.ContainerState{
								Running: &corev1.ContainerStateRunning{},
							},
						},
					},
				},
			},
			expected: "unknown reason",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getPodFailureReason(tt.pod)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// NOTE: TestBuildPodSpec and TestBuildPodSpecResourceProfiles are removed
// because buildPodSpec depends on cluster_manager which requires K8s cluster
// connection that is not available in unit test environment.

