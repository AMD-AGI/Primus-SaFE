package tracelens

import (
	"testing"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
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
			expectedMemory: "2Gi",
			expectedCPU:    "1",
		},
		{
			name:           "medium profile",
			profile:        tlconst.ProfileMedium,
			expectedMemory: "4Gi",
			expectedCPU:    "2",
		},
		{
			name:           "large profile",
			profile:        tlconst.ProfileLarge,
			expectedMemory: "8Gi",
			expectedCPU:    "4",
		},
		{
			name:           "empty profile defaults to medium",
			profile:        "",
			expectedMemory: "4Gi",
			expectedCPU:    "2",
		},
		{
			name:           "unknown profile defaults to medium",
			profile:        "unknown",
			expectedMemory: "4Gi",
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

func TestBuildPodSpec(t *testing.T) {
	now := time.Now()
	session := &model.TracelensSessions{
		SessionID:       "test-session-123",
		WorkloadUID:     "workload-456",
		ProfilerFileID:  789,
		ResourceProfile: tlconst.ProfileMedium,
		PodNamespace:    "primus-lens",
		ExpiresAt:       now.Add(1 * time.Hour),
	}

	pod := buildPodSpec(session, "tracelens-test-session-123", "/path/to/profiler/file.json")

	// Verify pod metadata
	assert.Equal(t, "tracelens-test-session-123", pod.Name)
	assert.Equal(t, "primus-lens", pod.Namespace)
	assert.Equal(t, "tracelens", pod.Labels["app"])
	assert.Equal(t, "test-session-123", pod.Labels["tracelens.lens.primus/session"])
	assert.Equal(t, "workload-456", pod.Labels["tracelens.lens.primus/workload"])

	// Verify annotations
	assert.Contains(t, pod.Annotations["tracelens.lens.primus/profiler-file"], "/path/to/profiler/file.json")
	assert.NotEmpty(t, pod.Annotations["tracelens.lens.primus/expires-at"])

	// Verify pod spec
	assert.Equal(t, corev1.RestartPolicyNever, pod.Spec.RestartPolicy)
	assert.Len(t, pod.Spec.Containers, 1)

	container := pod.Spec.Containers[0]
	assert.Equal(t, "tracelens", container.Name)
	assert.Equal(t, tlconst.DefaultTraceLensImage, container.Image)
	assert.Len(t, container.Ports, 1)
	assert.Equal(t, int32(tlconst.DefaultPodPort), container.Ports[0].ContainerPort)

	// Verify environment variables
	envMap := make(map[string]string)
	for _, env := range container.Env {
		envMap[env.Name] = env.Value
	}
	assert.Equal(t, "test-session-123", envMap["SESSION_ID"])
	assert.Equal(t, "789", envMap["PROFILER_FILE_ID"])
	assert.NotEmpty(t, envMap["API_BASE_URL"])
	assert.NotEmpty(t, envMap["BASE_URL_PATH"])
	assert.Equal(t, "/path/to/profiler/file.json", envMap["TRACE_FILE_PATH"])

	// Verify resource limits
	memLimit := container.Resources.Limits.Memory()
	assert.Equal(t, "4Gi", memLimit.String())
	cpuLimit := container.Resources.Limits.Cpu()
	assert.Equal(t, "2", cpuLimit.String())

	// Verify probes
	assert.NotNil(t, container.ReadinessProbe)
	assert.NotNil(t, container.LivenessProbe)
	assert.NotNil(t, container.ReadinessProbe.HTTPGet)
	assert.NotNil(t, container.LivenessProbe.HTTPGet)
}

func TestBuildPodSpecResourceProfiles(t *testing.T) {
	profiles := []struct {
		profile        string
		expectedMemory string
		expectedCPU    string
	}{
		{tlconst.ProfileSmall, "2Gi", "1"},
		{tlconst.ProfileMedium, "4Gi", "2"},
		{tlconst.ProfileLarge, "8Gi", "4"},
	}

	for _, p := range profiles {
		t.Run(p.profile, func(t *testing.T) {
			session := &model.TracelensSessions{
				SessionID:       "test-session",
				WorkloadUID:     "workload",
				ProfilerFileID:  1,
				ResourceProfile: p.profile,
				PodNamespace:    "default",
				ExpiresAt:       time.Now().Add(1 * time.Hour),
			}

			pod := buildPodSpec(session, "test-pod", "/file.json")

			container := pod.Spec.Containers[0]
			memLimit := container.Resources.Limits.Memory()
			cpuLimit := container.Resources.Limits.Cpu()

			assert.Equal(t, p.expectedMemory, memLimit.String())
			assert.Equal(t, p.expectedCPU, cpuLimit.String())
		})
	}
}
