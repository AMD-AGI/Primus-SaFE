package reconciler

import (
	"context"
	"testing"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/config"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	primusSafeV1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// MockWorkloadFacade is a mock implementation for testing
type MockWorkloadFacade struct {
	reusedDetection *model.FrameworkDetection
	reportedSources []string
}

func (m *MockWorkloadFacade) GetAiWorkloadMetadata(ctx context.Context, workloadUID string) (*model.AiWorkloadMetadata, error) {
	return nil, nil
}

func (m *MockWorkloadFacade) CreateAiWorkloadMetadata(ctx context.Context, metadata *model.AiWorkloadMetadata) error {
	return nil
}

func (m *MockWorkloadFacade) UpdateAiWorkloadMetadata(ctx context.Context, metadata *model.AiWorkloadMetadata) error {
	return nil
}

func (m *MockWorkloadFacade) FindCandidateWorkloads(ctx context.Context, imagePrefix string, startTime, minConfidence float64, limit int) ([]*model.CandidateWorkload, error) {
	return nil, nil
}

// TestConvertToInternalWorkload tests workload conversion
func TestConvertToInternalWorkload(t *testing.T) {
	// Create a test Primus SAFE workload
	workload := &primusSafeV1.Workload{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-workload",
			Namespace: "default",
			UID:       types.UID("test-uid-123"),
			Labels: map[string]string{
				"framework": "primus",
			},
		},
		Spec: primusSafeV1.WorkloadSpec{
			Template: primusSafeV1.Template{
				Containers: []corev1.Container{
					{
						Name:    "training",
						Image:   "registry.example.com/primus:v1.2.3",
						Command: []string{"python", "train.py"},
						Args:    []string{"--config", "config.yaml"},
						Env: []corev1.EnvVar{
							{Name: "PRIMUS_CONFIG", Value: "/config/primus.yaml"},
							{Name: "WORLD_SIZE", Value: "8"},
						},
					},
				},
			},
		},
	}

	// Convert to internal workload
	internal := convertToInternalWorkload(workload)

	// Verify conversion
	assert.Equal(t, "test-uid-123", internal.UID)
	assert.Equal(t, "test-workload", internal.Name)
	assert.Equal(t, "default", internal.Namespace)
	assert.Equal(t, "registry.example.com/primus:v1.2.3", internal.Image)
	assert.Equal(t, []string{"python", "train.py"}, internal.Command)
	assert.Equal(t, []string{"--config", "config.yaml"}, internal.Args)
	assert.Equal(t, "/config/primus.yaml", internal.Env["PRIMUS_CONFIG"])
	assert.Equal(t, "8", internal.Env["WORLD_SIZE"])
	assert.Equal(t, "primus", internal.Labels["framework"])
}

// TestExtractFunctions tests extraction functions
func TestExtractFunctions(t *testing.T) {
	workload := &primusSafeV1.Workload{
		Spec: primusSafeV1.WorkloadSpec{
			Template: primusSafeV1.Template{
				Containers: []corev1.Container{
					{
						Image:   "test-image:v1",
						Command: []string{"python"},
						Args:    []string{"--test"},
					},
				},
			},
		},
	}

	assert.Equal(t, "test-image:v1", extractImage(workload))
	assert.Equal(t, []string{"python"}, extractCommand(workload))
	assert.Equal(t, []string{"--test"}, extractArgs(workload))
}

// TestExtractFunctionsEmptyContainer tests extraction with no containers
func TestExtractFunctionsEmptyContainer(t *testing.T) {
	workload := &primusSafeV1.Workload{
		Spec: primusSafeV1.WorkloadSpec{
			Template: primusSafeV1.Template{
				Containers: []corev1.Container{},
			},
		},
	}

	assert.Equal(t, "", extractImage(workload))
	assert.Equal(t, []string{}, extractCommand(workload))
	assert.Equal(t, []string{}, extractArgs(workload))
}

