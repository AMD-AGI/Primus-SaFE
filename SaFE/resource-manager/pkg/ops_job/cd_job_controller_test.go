/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package ops_job

import (
	"testing"

	"github.com/stretchr/testify/assert"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

func TestGetParameterValue(t *testing.T) {
	tests := []struct {
		name         string
		job          *v1.OpsJob
		paramName    string
		defaultValue string
		expected     string
	}{
		{
			name: "get existing parameter",
			job: &v1.OpsJob{
				Spec: v1.OpsJobSpec{
					Inputs: []v1.Parameter{
						{Name: "test_param", Value: "test_value"},
					},
				},
			},
			paramName:    "test_param",
			defaultValue: "default",
			expected:     "test_value",
		},
		{
			name: "get non-existing parameter returns default",
			job: &v1.OpsJob{
				Spec: v1.OpsJobSpec{
					Inputs: []v1.Parameter{
						{Name: "other_param", Value: "other_value"},
					},
				},
			},
			paramName:    "test_param",
			defaultValue: "default",
			expected:     "default",
		},
		{
			name: "empty inputs returns default",
			job: &v1.OpsJob{
				Spec: v1.OpsJobSpec{
					Inputs: []v1.Parameter{},
				},
			},
			paramName:    "test_param",
			defaultValue: "default",
			expected:     "default",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getParameterValue(tt.job, tt.paramName, tt.defaultValue)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestBuildDeployScript(t *testing.T) {
	r := &CDJobReconciler{}

	t.Run("build script with all components", func(t *testing.T) {
		script := r.buildDeployScript(
			"apiserver.image=v1.0;", // componentTags
			"image=v1.0;",           // nodeAgentTags
			"env_key=value",         // envFileConfig
			"main",                  // deployBranch
			true,                    // hasNodeAgent
			true,                    // hasCICD
			"node-agent:v1.0",       // nodeAgentImage
			"cicd-runner:v1.0",      // cicdRunnerImage
			"cicd-unified:v1.0",     // cicdUnifiedImage
		)

		// Verify script contains key elements
		assert.Contains(t, script, "HAS_NODE_AGENT=true")
		assert.Contains(t, script, "HAS_CICD=true")
		assert.Contains(t, script, "NODE_AGENT_IMAGE=\"node-agent:v1.0\"")
		assert.Contains(t, script, "CICD_RUNNER_IMAGE=\"cicd-runner:v1.0\"")
		assert.Contains(t, script, "DEPLOY_BRANCH=\"main\"")
		assert.Contains(t, script, "Step 1: Preparing repository")
		assert.Contains(t, script, "Step 3: Running local upgrade script")
		assert.Contains(t, script, "Step 4: Verifying local deployments")
		assert.Contains(t, script, "Step 5: Remote cluster updates")
	})

	t.Run("build script without remote updates", func(t *testing.T) {
		script := r.buildDeployScript(
			"apiserver.image=v1.0;", // componentTags
			"",                      // nodeAgentTags (empty)
			"env_key=value",         // envFileConfig
			"main",                  // deployBranch
			false,                   // hasNodeAgent
			false,                   // hasCICD
			"",                      // nodeAgentImage (empty)
			"",                      // cicdRunnerImage (empty)
			"",                      // cicdUnifiedImage (empty)
		)

		// Verify script contains local deployment steps
		assert.Contains(t, script, "HAS_NODE_AGENT=false")
		assert.Contains(t, script, "HAS_CICD=false")
		assert.Contains(t, script, "Step 1: Preparing repository")
		assert.Contains(t, script, "Step 3: Running local upgrade script")
		assert.Contains(t, script, "Step 4: Verifying local deployments")
		// Remote cluster update section is conditional
	})

	t.Run("build script with empty deploy branch", func(t *testing.T) {
		script := r.buildDeployScript(
			"apiserver.image=v1.0;",
			"",
			"env_key=value",
			"", // empty deploy branch
			false,
			false,
			"",
			"",
			"",
		)

		assert.Contains(t, script, "DEPLOY_BRANCH=\"\"")
	})
}

func TestWaitDeploymentReadyScript(t *testing.T) {
	r := &CDJobReconciler{}

	script := r.buildDeployScript(
		"apiserver.image=v1.0;",
		"",
		"env_key=value",
		"main",
		false,
		false,
		"",
		"",
		"",
	)

	// Verify deployment verification functions are present
	assert.Contains(t, script, "wait_deployment_ready()")
	assert.Contains(t, script, "wait_daemonset_ready()")
	assert.Contains(t, script, "primus-safe-apiserver")
	assert.Contains(t, script, "primus-safe-resource-manager")
	assert.Contains(t, script, "primus-safe-job-manager")
	assert.Contains(t, script, "primus-safe-webhooks")
	assert.Contains(t, script, "primus-safe-web")
	assert.Contains(t, script, "primus-safe-node-agent")

	// Verify error detection logic
	assert.Contains(t, script, "ErrImagePull")
	assert.Contains(t, script, "ImagePullBackOff")
	assert.Contains(t, script, "CrashLoopBackOff")
}
