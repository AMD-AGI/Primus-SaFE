/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package workspace

import (
	"context"
	"strings"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
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

// GetVolumeMountPathByPreference returns the mountPath of the volume in the workspace whose
// MountPath (or HostPath if MountPath is empty) equals `prefer`. Returns "" when not found.
func GetVolumeMountPathByPreference(workspace *v1.Workspace, prefer string) string {
	if prefer == "" {
		return ""
	}
	prefer = strings.TrimSpace(prefer)
	for _, vol := range workspace.Spec.Volumes {
		mp := strings.TrimSpace(vol.MountPath)
		if mp == "" {
			mp = strings.TrimSpace(vol.HostPath)
		}
		if mp == prefer {
			if vol.MountPath != "" {
				return vol.MountPath
			}
			return vol.HostPath
		}
	}
	return ""
}

// ResolveDownloadRoot returns the PFS root path the model downloader should write to:
// if `prefer` matches a workspace volume, use it; otherwise fall back to the default selection.
func ResolveDownloadRoot(workspace *v1.Workspace, prefer string) string {
	if mp := GetVolumeMountPathByPreference(workspace, prefer); mp != "" {
		return mp
	}
	return GetNfsPathFromWorkspace(workspace)
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

// GetWorkspacesWithSamePath returns all workspace names that share the same storage base path.
// Used for download failover: when a download fails in one workspace, find alternative workspaces.
func GetWorkspacesWithSamePath(k8sClient client.Client, basePath string) ([]string, error) {
	workspaceList := &v1.WorkspaceList{}
	if err := k8sClient.List(context.Background(), workspaceList); err != nil {
		return nil, err
	}

	var result []string
	for _, ws := range workspaceList.Items {
		wsPath := GetNfsPathFromWorkspace(&ws)
		if wsPath != "" && wsPath == basePath {
			result = append(result, ws.Name)
		}
	}
	return result, nil
}

// IsPathAccessibleFromWorkspace checks if a file path is accessible from the specified workspace.
// This supports shared storage scenarios: even if LocalPaths records workspace B,
// workspace A can still access the file if they share the same storage base path.
// It checks ALL volumes mounted in the workspace, not just the primary PFS volume.
func IsPathAccessibleFromWorkspace(k8sClient client.Client, filePath, workspace string) (bool, error) {
	ws := &v1.Workspace{}
	if err := k8sClient.Get(context.Background(), client.ObjectKey{Name: workspace}, ws); err != nil {
		return false, err
	}

	for _, vol := range ws.Spec.Volumes {
		mountPath := strings.TrimSpace(vol.MountPath)
		if mountPath == "" {
			mountPath = strings.TrimSpace(vol.HostPath)
		}
		if mountPath != "" && strings.HasPrefix(filePath, mountPath) {
			return true, nil
		}
	}
	return false, nil
}
