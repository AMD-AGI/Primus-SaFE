/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package workload

import (
	"path/filepath"
	"strings"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

// DeclaredHostPathRoot returns the longest hostpath volume root of the workspace that is mounted
// into workloads of the given scope and lexically contains hostpath, and the path below that root
// ("" when hostpath is the root itself). It returns an empty root for a relative entry, an entry
// with a ".." element, an entry under no such root, or a nil workspace.
//
// The check is lexical: a symlink below the root can still point anywhere on the node. A caller
// that mounts the entry must mount the root and reach the entry through subPath, which the kubelet
// resolves inside the volume and refuses when it escapes it.
func DeclaredHostPathRoot(workspace *v1.Workspace, scope v1.WorkspaceScope, hostpath string) (string, string) {
	if workspace == nil || !filepath.IsAbs(hostpath) {
		return "", ""
	}
	for _, elem := range strings.Split(hostpath, "/") {
		if elem == ".." {
			return "", ""
		}
	}
	cleaned := filepath.Clean(hostpath)
	best := ""
	for _, vol := range workspace.Spec.Volumes {
		if vol.Type != v1.HOSTPATH || !vol.AppliesToScope(scope) {
			continue
		}
		root := strings.TrimSpace(vol.HostPath)
		if root == "" {
			continue
		}
		root = filepath.Clean(root)
		if cleaned != root && root != "/" && !strings.HasPrefix(cleaned, root+"/") {
			continue
		}
		if len(root) > len(best) {
			best = root
		}
	}
	if best == "" {
		return "", ""
	}
	return best, strings.TrimPrefix(strings.TrimPrefix(cleaned, best), "/")
}

// UndeclaredHostPaths returns the entries of hostpaths that are neither a hostpath volume the
// workspace mounts into workloads of the given scope nor a directory under one. Mounting such an
// entry gives a workload a node path nobody declared for the workspace.
func UndeclaredHostPaths(workspace *v1.Workspace, scope v1.WorkspaceScope, hostpaths []string) []string {
	var undeclared []string
	for _, hostpath := range hostpaths {
		if root, _ := DeclaredHostPathRoot(workspace, scope, hostpath); root == "" {
			undeclared = append(undeclared, hostpath)
		}
	}
	return undeclared
}
