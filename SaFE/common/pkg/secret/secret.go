/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package secret

import (
	"encoding/json"

	corev1 "k8s.io/api/core/v1"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

// GetSecretWorkspaces extracts workspace IDs from a secret's annotations.
// Returns nil if no workspace IDs are found or if unmarshaling fails.
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
