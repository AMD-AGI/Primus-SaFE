/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package secret

import (
	"encoding/json"

	corev1 "k8s.io/api/core/v1"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

// SandboxEnvSecretName is the Secret whose every key a sandbox's main container receives as an
// environment variable when the chart's sandbox.env_secret is set. The name is fixed, and is
// reserved whether or not the feature is on, so that no Secret of this name can be written by a
// non-admin, or taken over by the mirror, before or while the sandbox template starts reading it.
const SandboxEnvSecretName = "sandbox-env"

// GetSecretWorkspaces extracts workspace IDs from a secret's annotations.
// Returns nil if no workspace IDs are found or if Unmarshal fails.
func GetSecretWorkspaces(secret *corev1.Secret) []string {
	workspaceIdsStr := v1.GetAnnotation(secret, v1.WorkspaceIdsAnnotation)
	if workspaceIdsStr == "" {
		return nil
	}
	var workspaceIds []string
	if json.Unmarshal([]byte(workspaceIdsStr), &workspaceIds) != nil {
		return nil
	}
	return workspaceIds
}
