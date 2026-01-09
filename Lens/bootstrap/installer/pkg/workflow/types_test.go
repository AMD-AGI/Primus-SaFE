// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package workflow

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/AMD-AGI/Primus-SaFE/Lens/bootstrap/installer/pkg/config"
)

func TestRegisterAndCreateWorkflow(t *testing.T) {
	// Test that dataplane workflow is registered
	workflows := ListWorkflows()
	assert.Contains(t, workflows, "dataplane")
	assert.Contains(t, workflows, "controlplane")
	assert.Contains(t, workflows, "standalone")
}

func TestNewWorkflowDataplane(t *testing.T) {
	cfg := config.DefaultConfig()

	wf, err := NewWorkflow("dataplane", cfg)
	assert.NoError(t, err)
	assert.NotNil(t, wf)
	assert.Equal(t, "dataplane", wf.Name())
}

func TestNewWorkflowControlplane(t *testing.T) {
	cfg := config.DefaultConfig()

	wf, err := NewWorkflow("controlplane", cfg)
	assert.NoError(t, err)
	assert.NotNil(t, wf)
	assert.Equal(t, "controlplane", wf.Name())
}

func TestNewWorkflowStandalone(t *testing.T) {
	cfg := config.DefaultConfig()

	wf, err := NewWorkflow("standalone", cfg)
	assert.NoError(t, err)
	assert.NotNil(t, wf)
	assert.Equal(t, "standalone", wf.Name())
}

func TestNewWorkflowUnknown(t *testing.T) {
	cfg := config.DefaultConfig()

	wf, err := NewWorkflow("unknown", cfg)
	assert.Error(t, err)
	assert.Nil(t, wf)

	var unknownErr *ErrUnknownWorkflow
	assert.ErrorAs(t, err, &unknownErr)
	assert.Equal(t, "unknown", unknownErr.Name)
}

func TestErrUnknownWorkflow(t *testing.T) {
	err := &ErrUnknownWorkflow{Name: "test"}
	assert.Equal(t, "unknown workflow: test", err.Error())
}

func TestStateConstants(t *testing.T) {
	assert.Equal(t, State("Unknown"), StateUnknown)
	assert.Equal(t, State("Pending"), StatePending)
	assert.Equal(t, State("InProgress"), StateInProgress)
	assert.Equal(t, State("Ready"), StateReady)
	assert.Equal(t, State("Failed"), StateFailed)
}

