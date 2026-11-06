/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package workload

import (
	"path/filepath"
	"strings"

	corev1 "k8s.io/api/core/v1"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

// HostPathRoot is the node directory below which a hostpath volume of a workspace declares host
// paths for spec.hostpath.
type HostPathRoot struct {
	// Path is the volume's hostPath joined with its subPath: the directory the volume mounts.
	Path string
	// ReadOnly is set when the volume is mounted read-only (accessMode ReadOnlyMany).
	ReadOnly bool
	// UserDirs is set for a volume with enableUserDir. Such a volume declares neither Path itself
	// nor its user directories (Path/users and below), only the other directories directly under
	// Path, each a root of its own; see DeclaredHostPathRoot.
	UserDirs bool
}

// String names the root in a message: "/shared/teamA", or "/shared_nfs/* but /shared_nfs/users".
func (r HostPathRoot) String() string {
	if r.UserDirs {
		return filepath.Join(r.Path, "*") + " but " + filepath.Join(r.Path, v1.UserDirsPath)
	}
	return r.Path
}

// DeclaredHostPathRoots returns what the hostpath volumes of the workspace that are mounted into
// workloads of the given scope declare for spec.hostpath: for each volume, the directory it mounts
// (hostPath joined with subPath, not the rest of hostPath), read-only when the volume is.
//
// A volume with enableUserDir shows each workload its own user directory there, so it keeps the
// user directories out of what it declares (HostPathRoot.UserDirs). A volume whose subPath is
// absolute or has a ".." element does not name a directory under its hostPath and declares
// nothing.
func DeclaredHostPathRoots(workspace *v1.Workspace, scope v1.WorkspaceScope) []HostPathRoot {
	if workspace == nil {
		return nil
	}
	var roots []HostPathRoot
	for _, vol := range workspace.Spec.Volumes {
		if vol.Type != v1.HOSTPATH || !vol.AppliesToScope(scope) {
			continue
		}
		hostPath := strings.TrimSpace(vol.HostPath)
		if hostPath == "" || filepath.IsAbs(vol.SubPath) || hasDotDot(vol.SubPath) {
			continue
		}
		roots = append(roots, HostPathRoot{
			Path:     filepath.Join(hostPath, vol.SubPath),
			ReadOnly: vol.AccessMode == corev1.ReadOnlyMany,
			UserDirs: vol.EnableUserDir,
		})
	}
	return roots
}

// DeclaredHostPath is where a spec.hostpath entry lies among the host paths its workspace declares.
type DeclaredHostPath struct {
	// Root is the directory to mount for the entry, "" when the workspace does not declare it: the
	// longest root that contains the entry, where the root of a volume with user directories is the
	// directory directly under the volume's that the entry is in.
	Root string
	// Sub is the entry's path below Root, "" for Root itself.
	Sub string
	// ReadOnly is set when every volume that declares the entry is read-only, so that no volume of
	// the workspace shows the entry to the workload writable.
	ReadOnly bool
}

// DeclaredHostPathRoot returns where hostpath lies among what the workspace declares for workloads
// of the given scope (DeclaredHostPathRoots). The root is empty for a relative entry, an entry
// with a ".." element, an undeclared entry, or a nil workspace.
//
// The check is lexical: a symlink below the root can still point anywhere on the node. A caller
// that mounts the entry must mount the root and reach the entry through subPath, which the kubelet
// resolves inside the root and refuses when it escapes it, and must mount it read-only when
// ReadOnly is set. That is also why a volume with user directories gives each directory under it a
// root of its own: a symlink written into one cannot lead a later mount into the user directories.
func DeclaredHostPathRoot(workspace *v1.Workspace, scope v1.WorkspaceScope, hostpath string) DeclaredHostPath {
	if !filepath.IsAbs(hostpath) || hasDotDot(hostpath) {
		return DeclaredHostPath{}
	}
	cleaned := filepath.Clean(hostpath)
	var declared DeclaredHostPath
	writable := false
	for _, root := range DeclaredHostPathRoots(workspace, scope) {
		path, ok := rootFor(root, cleaned)
		if !ok {
			continue
		}
		writable = writable || !root.ReadOnly
		if len(path) > len(declared.Root) {
			declared.Root = path
		}
	}
	if declared.Root == "" {
		return DeclaredHostPath{}
	}
	declared.Sub = strings.TrimPrefix(strings.TrimPrefix(cleaned, declared.Root), "/")
	declared.ReadOnly = !writable
	return declared
}

// rootFor returns the directory of root to mount for the clean absolute path, and whether root
// declares the path at all.
func rootFor(root HostPathRoot, cleaned string) (string, bool) {
	sub, ok := below(root.Path, cleaned)
	if !ok {
		return "", false
	}
	if !root.UserDirs {
		return root.Path, true
	}
	first, _, _ := strings.Cut(sub, "/")
	if first == "" || first == v1.UserDirsPath {
		return "", false
	}
	return filepath.Join(root.Path, first), true
}

// below reports whether the clean absolute path is root or lies under it, and the path below it.
func below(root, cleaned string) (string, bool) {
	switch {
	case cleaned == root:
		return "", true
	case root == "/":
		return strings.TrimPrefix(cleaned, "/"), true
	case strings.HasPrefix(cleaned, root+"/"):
		return strings.TrimPrefix(cleaned, root+"/"), true
	}
	return "", false
}

// UndeclaredHostPaths returns the entries of hostpaths that the workspace does not declare for
// workloads of the given scope (DeclaredHostPathRoot). Mounting such an entry gives a workload a
// node path nobody declared for the workspace.
func UndeclaredHostPaths(workspace *v1.Workspace, scope v1.WorkspaceScope, hostpaths []string) []string {
	var undeclared []string
	for _, hostpath := range hostpaths {
		if DeclaredHostPathRoot(workspace, scope, hostpath).Root == "" {
			undeclared = append(undeclared, hostpath)
		}
	}
	return undeclared
}

// HostPathVolumeRoot returns the longest hostPath of a hostpath volume of the workspace mounted
// into workloads of the given scope that lexically contains hostpath, whatever part of it the
// volume declares, and the path below it ("" when hostpath is that hostPath itself). It returns an
// empty root for a relative entry, an entry with a ".." element, an entry under no such hostPath,
// or a nil workspace. An external Pod, which cannot use subPath, is held to it.
func HostPathVolumeRoot(workspace *v1.Workspace, scope v1.WorkspaceScope, hostpath string) (string, string) {
	if workspace == nil || !filepath.IsAbs(hostpath) || hasDotDot(hostpath) {
		return "", ""
	}
	cleaned := filepath.Clean(hostpath)
	best, bestSub := "", ""
	for _, vol := range workspace.Spec.Volumes {
		if vol.Type != v1.HOSTPATH || !vol.AppliesToScope(scope) {
			continue
		}
		root := strings.TrimSpace(vol.HostPath)
		if root == "" {
			continue
		}
		root = filepath.Clean(root)
		if sub, ok := below(root, cleaned); ok && len(root) > len(best) {
			best, bestSub = root, sub
		}
	}
	return best, bestSub
}

// hasDotDot reports whether a slash-separated path has a ".." element.
func hasDotDot(path string) bool {
	for _, elem := range strings.Split(path, "/") {
		if elem == ".." {
			return true
		}
	}
	return false
}
