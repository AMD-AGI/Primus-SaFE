/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package workspace

import (
	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

// DownloadTarget represents a target for downloading files
type DownloadTarget struct {
	Workspace string
	Path      string
}

// GetNfsPathFromWorkspace retrieves the NFS path from the workspace's volumes.
// It prioritizes PFS type volumes, otherwise falls back to the first available volume's mount path.
func GetNfsPathFromWorkspace(workspace *v1.Workspace) string {
	result := ""
	for _, vol := range workspace.Spec.Volumes {
		if vol.Type == v1.PFS {
			result = vol.MountPath
			break
		}
	}
	if result == "" && len(workspace.Spec.Volumes) > 0 {
		result = workspace.Spec.Volumes[0].MountPath
	}
	return result
}

// GetUniqueDownloadPaths extracts unique download paths from workspaces.
// It deduplicates by path - same path only creates one entry.
func GetUniqueDownloadPaths(workspaces []v1.Workspace) []DownloadTarget {
	pathMap := make(map[string]DownloadTarget) // key: actual storage path

	for _, ws := range workspaces {
		path := GetNfsPathFromWorkspace(&ws)
		if path == "" {
			continue
		}

		// Deduplicate: same path only creates one entry
		if _, exists := pathMap[path]; !exists {
			pathMap[path] = DownloadTarget{
				Workspace: ws.Name,
				Path:      path,
			}
		}
	}

	targets := make([]DownloadTarget, 0, len(pathMap))
	for _, target := range pathMap {
		targets = append(targets, target)
	}
	return targets
}
