package stage

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/AMD-AGI/Primus-SaFE/Lens/bootstrap/installer/pkg/types"
)

func TestNewHelmStage(t *testing.T) {
	s := NewHelmStage("test-stage", "test-release", "./chart")

	assert.Equal(t, "test-stage", s.Name())
	assert.Equal(t, "test-release", s.releaseName)
	assert.Equal(t, "./chart", s.chartPath)
	assert.Equal(t, 10*time.Minute, s.timeout)
	assert.False(t, s.wait)
}

func TestHelmStageWithOptions(t *testing.T) {
	s := NewHelmStage(
		"test-stage",
		"test-release",
		"./chart",
		WithValuesFile("values.yaml"),
		WithTimeout(5*time.Minute),
		WithWait(true),
		WithNamespace("custom-ns"),
		WithSetValue("key1", "value1"),
		WithSetValue("key2", "value2"),
		WithUpdateDeps(true),
	)

	assert.Equal(t, "values.yaml", s.valuesFile)
	assert.Equal(t, 5*time.Minute, s.timeout)
	assert.True(t, s.wait)
	assert.Equal(t, "custom-ns", s.namespace)
	assert.Equal(t, "value1", s.setValues["key1"])
	assert.Equal(t, "value2", s.setValues["key2"])
	assert.True(t, s.updateDeps)
}

func TestHelmStageRunDryRun(t *testing.T) {
	s := NewHelmStage("test-stage", "test-release", "./chart")

	opts := types.RunOptions{
		DryRun:    true,
		Namespace: "test-ns",
	}

	ctx := context.Background()
	err := s.Run(ctx, opts)

	// Should succeed in dry-run mode without actually running helm
	assert.NoError(t, err)
}

func TestHelmStageName(t *testing.T) {
	tests := []struct {
		name        string
		stageName   string
		releaseName string
		chartPath   string
		want        string
	}{
		{
			name:        "simple name",
			stageName:   "operators",
			releaseName: "primus-lens",
			chartPath:   "./chart",
			want:        "operators",
		},
		{
			name:        "complex name",
			stageName:   "infrastructure-setup",
			releaseName: "infra",
			chartPath:   "./charts/infra",
			want:        "infrastructure-setup",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewHelmStage(tt.stageName, tt.releaseName, tt.chartPath)
			assert.Equal(t, tt.want, s.Name())
		})
	}
}

func TestHelmStageRollbackDryRun(t *testing.T) {
	s := NewHelmStage("test-stage", "test-release", "./chart")

	opts := types.RunOptions{
		DryRun:    true,
		Namespace: "test-ns",
	}

	ctx := context.Background()
	err := s.Rollback(ctx, opts)

	assert.NoError(t, err)
}
