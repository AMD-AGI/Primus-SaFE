package stage

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/AMD-AGI/Primus-SaFE/Lens/bootstrap/installer/pkg/types"
)

func TestNewWaitStage(t *testing.T) {
	conditions := []WaitCondition{
		{
			Kind:      "Deployment",
			Name:      "test-deployment",
			Condition: "Available",
		},
	}

	s := NewWaitStage("wait-test", conditions)

	assert.Equal(t, "wait-test", s.Name())
	assert.Equal(t, 10*time.Minute, s.timeout)
	assert.Equal(t, 5*time.Second, s.interval)
	assert.Len(t, s.conditions, 1)
}

func TestWaitStageWithOptions(t *testing.T) {
	conditions := []WaitCondition{
		{
			Kind:      "Pod",
			Name:      "test-pod",
			Condition: "Ready",
		},
	}

	s := NewWaitStage(
		"wait-test",
		conditions,
		WithWaitTimeout(15*time.Minute),
		WithPollInterval(10*time.Second),
	)

	assert.Equal(t, 15*time.Minute, s.timeout)
	assert.Equal(t, 10*time.Second, s.interval)
}

func TestWaitStageRunDryRun(t *testing.T) {
	conditions := []WaitCondition{
		{
			Kind:      "Deployment",
			Name:      "test-deployment",
			Condition: "Available",
		},
		{
			Kind:          "Pod",
			LabelSelector: "app=test",
			Condition:     "Ready",
		},
	}

	s := NewWaitStage("wait-test", conditions)

	opts := types.RunOptions{
		DryRun:    true,
		Namespace: "test-ns",
	}

	ctx := context.Background()
	err := s.Run(ctx, opts)

	// Should succeed in dry-run mode
	assert.NoError(t, err)
}

func TestWaitStageRollback(t *testing.T) {
	s := NewWaitStage("wait-test", nil)

	opts := types.RunOptions{
		Namespace: "test-ns",
	}

	ctx := context.Background()
	err := s.Rollback(ctx, opts)

	// Rollback should be a no-op
	assert.NoError(t, err)
}

func TestWaitConditionDefaults(t *testing.T) {
	cond := WaitCondition{
		Kind:      "Deployment",
		Name:      "test",
		Condition: "Available",
	}

	assert.Empty(t, cond.Namespace)
	assert.Empty(t, cond.LabelSelector)
	assert.Empty(t, cond.JSONPath)
	assert.Zero(t, cond.Timeout)
}

func TestWaitConditionWithJSONPath(t *testing.T) {
	cond := WaitCondition{
		Kind:          "PostgresCluster",
		Name:          "primus-lens",
		JSONPath:      "{.status.conditions[0].status}",
		ExpectedValue: "True",
		Timeout:       10 * time.Minute,
	}

	assert.Equal(t, "PostgresCluster", cond.Kind)
	assert.Equal(t, "{.status.conditions[0].status}", cond.JSONPath)
	assert.Equal(t, "True", cond.ExpectedValue)
}

func TestWaitStageName(t *testing.T) {
	tests := []struct {
		name      string
		stageName string
		want      string
	}{
		{
			name:      "simple name",
			stageName: "wait-operators",
			want:      "wait-operators",
		},
		{
			name:      "complex name",
			stageName: "wait-infrastructure-ready",
			want:      "wait-infrastructure-ready",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewWaitStage(tt.stageName, nil)
			assert.Equal(t, tt.want, s.Name())
		})
	}
}
