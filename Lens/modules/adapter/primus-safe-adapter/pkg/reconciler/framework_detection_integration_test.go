package reconciler

import (
	"context"
	"testing"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	coreModel "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model"
	primusSafeV1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// MockWorkloadFacade is a mock implementation for testing
type MockWorkloadFacade struct {
	reusedDetection *coreModel.FrameworkDetection
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

func (m *MockWorkloadFacade) FindCandidateWorkloads(ctx context.Context, imagePrefix string, startTime int64, minConfidence float64, limit int) ([]*model.AiWorkloadMetadata, error) {
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
			Image:      "registry.example.com/primus:v1.2.3",
			EntryPoint: "python train.py --config config.yaml",
			Env: map[string]string{
				"PRIMUS_CONFIG": "/config/primus.yaml",
				"WORLD_SIZE":    "8",
			},
		},
	}

	// Convert to internal workload
	internal := convertToInternalWorkload(workload)

	// Verify conversion
	assert.Equal(t, "test-uid-123", internal.UID)
	assert.Equal(t, "default", internal.Namespace)
	assert.Equal(t, "registry.example.com/primus:v1.2.3", internal.Image)
	assert.Equal(t, []string{"sh", "-c"}, internal.Command)
	assert.Equal(t, []string{"python train.py --config config.yaml"}, internal.Args)
	assert.Equal(t, "/config/primus.yaml", internal.Env["PRIMUS_CONFIG"])
	assert.Equal(t, "8", internal.Env["WORLD_SIZE"])
	assert.Equal(t, "primus", internal.Labels["framework"])
}

// TestExtractFunctions tests extraction functions
func TestExtractFunctions(t *testing.T) {
	workload := &primusSafeV1.Workload{
		Spec: primusSafeV1.WorkloadSpec{
			Image:      "test-image:v1",
			EntryPoint: "python --test",
		},
	}

	assert.Equal(t, "test-image:v1", extractImage(workload))
	assert.Equal(t, []string{"sh", "-c"}, extractCommand(workload))
	assert.Equal(t, []string{"python --test"}, extractArgs(workload))
}

// TestExtractFunctionsEmptyContainer tests extraction with no entry point
func TestExtractFunctionsEmptyContainer(t *testing.T) {
	workload := &primusSafeV1.Workload{
		Spec: primusSafeV1.WorkloadSpec{
			Image: "",
		},
	}

	assert.Equal(t, "", extractImage(workload))
	assert.Equal(t, []string{}, extractCommand(workload))
	assert.Equal(t, []string{}, extractArgs(workload))
}
