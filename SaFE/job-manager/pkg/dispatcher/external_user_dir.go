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
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	jobutils "github.com/AMD-AIG-AIMA/SAFE/job-manager/pkg/utils"
)

// userDirMount is one hostPath volume, and the mount of it, that an external Pod gets in place of a
// whole workspace volume when user directories are mounted for external Pods.
type userDirMount struct {
	name      string
	hostPath  string
	hostType  string
	mountPath string
	readOnly  bool
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
// nothing is replaced and the volume is mounted as before. checkExternalPodHostPaths then checks
// every hostPath the Pod ends up with.
func externalUserDirMounts(workload *v1.Workload, vol v1.WorkspaceVolume) ([]userDirMount, bool, error) {
	if !commonconfig.IsExternalUserDirMountsEnable() || !isExternalWorkload(workload) ||
		vol.Type != v1.HOSTPATH || (!vol.EnableUserDir && len(vol.SharedPaths) == 0) {
		return nil, false, nil
	}
	if filepath.IsAbs(vol.SubPath) || hasDotDot(vol.SubPath) {
		return nil, true, fmt.Errorf("volume(%s) has subPath %q, which is not a path below its hostPath",
			vol.GenFullVolumeId(), vol.SubPath)
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

// checkExternalPodHostPaths is the last word on what an external Pod mounts while user directories
// are mounted for external Pods. Every hostPath volume of the rendered Pod, wherever it came from (a
// workspace volume, a shared path, spec.hostpath, the resource template), must be an absolute path
// without "..", and must not equal, lie under or contain the user directories of a hostpath volume
// of the workspace with enableUserDir, unless it is exactly the Pod's own user directory there.
// Otherwise another volume over the same tree, a volume or user directory nested in it, or a
// hostpath entry would mount other users' directories beside the Pod's own.
func checkExternalPodHostPaths(obj *unstructured.Unstructured, workload *v1.Workload, workspace *v1.Workspace,
	resourceSpec v1.ResourceSpec) error {
	if !commonconfig.IsExternalUserDirMountsEnable() || !isExternalWorkload(workload) {
		return nil
	}
	volumes, _, err := jobutils.NestedSlice(obj.Object, podSpecPath(workload, &resourceSpec, "volumes"))
	if err != nil {
		return err
	}
	userId := v1.GetUserId(workload)
	for _, raw := range volumes {
		vol, _ := raw.(map[string]interface{})
		hostPath, _ := vol["hostPath"].(map[string]interface{})
		if hostPath == nil {
			continue
		}
		name, _ := vol["name"].(string)
		path, _ := hostPath["path"].(string)
		switch {
		case !filepath.IsAbs(path):
			return fmt.Errorf("volume %q of an external Pod mounts %q, which is not absolute", name, path)
		case hasDotDot(path):
			return fmt.Errorf("volume %q of an external Pod mounts %q, which has a \"..\" element", name, path)
		}
		if users := userDirsOverlapping(workspace, workload, path, userId); users != "" {
			return fmt.Errorf("volume %q of an external Pod mounts %q, which overlaps the user directories "+
				"under %q; a Pod gets only its own user directory, from the workspace volume", name, path, users)
		}
	}
	return nil
}

// userDirsOverlapping returns the user directories ({hostPath}[/{subPath}]/users, v1.UserDirsPath)
// of a hostpath volume with enableUserDir mounted into the workload's scope that path equals, lies
// under or contains, or "" when there is none. "/" contains every one. Exactly {users}/{ownUserId} is the Pod's own
// user directory and does not count.
func userDirsOverlapping(workspace *v1.Workspace, workload *v1.Workload, path, ownUserId string) string {
	if workspace == nil {
		return ""
	}
	cleaned := filepath.Clean(path)
	for _, vol := range workspace.Spec.Volumes {
		if vol.Type != v1.HOSTPATH || !vol.EnableUserDir || strings.TrimSpace(vol.HostPath) == "" {
			continue
		}
		// A volume whose scopes leave out the workload's scope is not mounted into it, so its user
		// directories say nothing about this Pod: a workspace can give, say, Authoring the whole tree
		// through a volume of its own while every other scope gets only its own user directory.
		if !workspaceVolumeAppliesTo(vol, workload) {
			continue
		}
		users := filepath.Join(vol.HostPath, vol.SubPath, v1.UserDirsPath)
		if ownUserId != "" && cleaned == filepath.Join(users, ownUserId) {
			continue
		}
		if cleaned == "/" || pathsOverlap(cleaned, users) {
			return users
		}
	}
	return ""
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
