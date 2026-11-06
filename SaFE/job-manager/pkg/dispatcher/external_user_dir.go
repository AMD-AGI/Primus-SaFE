/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package dispatcher

import (
	"fmt"
	"path/filepath"
	"strings"

	corev1 "k8s.io/api/core/v1"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
)

// userDirMount is one hostPath volume, and the mount of it, that an external Pod gets in place of a
// whole workspace volume when user directories are mounted for external Pods.
type userDirMount struct {
	name      string
	hostPath  string
	hostType  string
	mountPath string
	readOnly  bool
	// userDir marks the Pod's own user directory.
	userDir bool
}

// externalUserDirMounts returns what replaces the whole-volume mount of vol in an external Pod, and
// whether vol is replaced at all. With external_execution.user_dir_mounts on, a hostpath volume with
// enableUserDir or sharedPaths is mounted as the Pod's own user directory (enableUserDir) and the
// volume's shared paths, each as its own hostPath below the volume root, at the path it has below
// mountPath. The user directory is mounted where USER_DATA_PATH points, as before.
//
// The user directory keeps the volume's name and access mode, and is DirectoryOrCreate so that a
// user's first Pod can have it made. A shared path must exist, and is read-only when it says so or
// when the volume is. Without the flag, for Pods on the cluster's own nodes, and for other volumes,
// nothing is replaced and the volume is mounted as before.
func externalUserDirMounts(workload *v1.Workload, vol v1.WorkspaceVolume) ([]userDirMount, bool, error) {
	if !commonconfig.IsExternalUserDirMountsEnable() || !isExternalWorkload(workload) ||
		vol.Type != v1.HOSTPATH || (!vol.EnableUserDir && len(vol.SharedPaths) == 0) {
		return nil, false, nil
	}
	root := filepath.Join(vol.HostPath, vol.SubPath)
	volumeReadOnly := vol.AccessMode == corev1.ReadOnlyMany
	var mounts []userDirMount
	if vol.EnableUserDir {
		userId := v1.GetUserId(workload)
		if userId == "" || strings.Contains(userId, "/") || userId == "." || userId == ".." {
			return nil, true, fmt.Errorf("workload %s has no user id that names a user directory: %q",
				workload.Name, userId)
		}
		userDir := generateUserDir(userId)
		mounts = append(mounts, userDirMount{
			name:      vol.GenFullVolumeId(),
			hostPath:  filepath.Join(root, userDir),
			hostType:  string(corev1.HostPathDirectoryOrCreate),
			mountPath: vol.MountPath + "/" + userDir,
			readOnly:  volumeReadOnly,
			userDir:   true,
		})
	}
	for i, shared := range vol.SharedPaths {
		if err := vol.ValidateSharedPath(shared.Path); err != nil {
			return nil, true, err
		}
		mounts = append(mounts, userDirMount{
			name:      fmt.Sprintf("%s-shared-%d", vol.GenFullVolumeId(), i),
			hostPath:  filepath.Join(root, shared.Path),
			hostType:  string(corev1.HostPathDirectory),
			mountPath: vol.MountPath + "/" + shared.Path,
			readOnly:  shared.ReadOnly || volumeReadOnly,
		})
	}
	return mounts, true, nil
}

// checkExternalWorkspaceVolume refuses, while user directories are mounted for external Pods, a
// hostpath workspace volume that an external workload would mount whole, or a shared path, whose
// source on the node overlaps the user directories of a volume with enableUserDir: a second volume
// over the same tree, or a shared path of another volume, would otherwise mount other users'
// directories beside the Pod's own. mounts and replaced are what externalUserDirMounts returned.
func checkExternalWorkspaceVolume(workload *v1.Workload, workspace *v1.Workspace, vol v1.WorkspaceVolume,
	mounts []userDirMount, replaced bool) error {
	if !commonconfig.IsExternalUserDirMountsEnable() || !isExternalWorkload(workload) || vol.Type != v1.HOSTPATH {
		return nil
	}
	sources := []string{filepath.Join(vol.HostPath, vol.SubPath)}
	if replaced {
		sources = sources[:0]
		for _, m := range mounts {
			if !m.userDir {
				sources = append(sources, m.hostPath)
			}
		}
	}
	for _, source := range sources {
		if users := userDirsOverlapping(workspace, source); users != "" {
			return fmt.Errorf("volume(%s) of workspace %s would mount %q into an external Pod, which "+
				"overlaps the user directories under %q", vol.GenFullVolumeId(), workspace.Name, source, users)
		}
	}
	return nil
}

// userDirsOverlapping returns the user directories ({hostPath}[/{subPath}]/users, v1.UserDirsPath)
// of a hostpath volume with enableUserDir in the workspace that path equals, lies under or contains,
// or "" when there is none. "/" contains every one.
func userDirsOverlapping(workspace *v1.Workspace, path string) string {
	if workspace == nil {
		return ""
	}
	cleaned := filepath.Clean(path)
	for _, vol := range workspace.Spec.Volumes {
		if vol.Type != v1.HOSTPATH || !vol.EnableUserDir || strings.TrimSpace(vol.HostPath) == "" {
			continue
		}
		users := filepath.Join(vol.HostPath, vol.SubPath, v1.UserDirsPath)
		if cleaned == "/" || pathsOverlap(cleaned, users) {
			return users
		}
	}
	return ""
}

// checkExternalUserDirHostPath refuses, while user directories are mounted for external Pods, a
// hostpath entry of an external workload that equals, lies under or contains the user directories
// ({hostPath}/users, v1.UserDirsPath) of a hostpath volume with enableUserDir, or that is relative or has "..". Such
// an entry would mount other users' directories, or reach them through "..", beside the Pod's own.
func checkExternalUserDirHostPath(workload *v1.Workload, workspace *v1.Workspace, hostpath string) error {
	if !commonconfig.IsExternalUserDirMountsEnable() || !isExternalWorkload(workload) {
		return nil
	}
	if !filepath.IsAbs(hostpath) {
		return fmt.Errorf("hostpath %q of an external workload is not absolute", hostpath)
	}
	for _, elem := range strings.Split(hostpath, "/") {
		if elem == ".." {
			return fmt.Errorf("hostpath %q of an external workload has a \"..\" element", hostpath)
		}
	}
	if users := userDirsOverlapping(workspace, hostpath); users != "" {
		return fmt.Errorf("hostpath %q of an external workload overlaps the user directories under %q; "+
			"a Pod gets its own user directory from the workspace volume", hostpath, users)
	}
	return nil
}
