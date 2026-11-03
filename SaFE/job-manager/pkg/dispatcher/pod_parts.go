/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package dispatcher

import (
	"fmt"
	"path"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	commonworkload "github.com/AMD-AIG-AIMA/SAFE/common/pkg/workload"
	jobutils "github.com/AMD-AIG-AIMA/SAFE/job-manager/pkg/utils"
)

// applyPodParts adds the workload's volumes, init containers, main-container mounts and env refs.
// The init containers see only their own mounts. A main-container mount may not overlap
// a mount the container already has, and a name may not repeat one already in the pod.
func applyPodParts(obj *unstructured.Unstructured, workload *v1.Workload, resourceSpec *v1.ResourceSpec) error {
	spec := &workload.Spec
	if len(spec.InitContainers) == 0 && len(spec.Volumes) == 0 && len(spec.VolumeMounts) == 0 && len(spec.EnvRefs) == 0 {
		return nil
	}
	volumesPath := podSpecPath(workload, resourceSpec, "volumes")
	volumes, _, _ := jobutils.NestedSlice(obj.Object, volumesPath)
	for _, vol := range spec.Volumes {
		if hasNamedItem(volumes, vol.Name) {
			return commonerrors.NewBadRequest(fmt.Sprintf("volume %s is already in the pod", vol.Name))
		}
		volumes = append(volumes, buildPodPartVolume(vol))
	}
	if err := jobutils.SetNestedField(obj.Object, volumes, volumesPath); err != nil {
		return err
	}

	initPath := podSpecPath(workload, resourceSpec, "initContainers")
	inits, _, _ := jobutils.NestedSlice(obj.Object, initPath)
	for _, c := range spec.InitContainers {
		if hasNamedItem(inits, c.Name) {
			return commonerrors.NewBadRequest(fmt.Sprintf("init container %s is already in the pod", c.Name))
		}
		init := map[string]interface{}{
			"name":                     c.Name,
			"image":                    c.Image,
			"imagePullPolicy":          "IfNotPresent",
			"terminationMessagePolicy": "FallbackToLogsOnError",
		}
		if len(c.Command) > 0 {
			init["command"] = stringsToInterfaces(c.Command)
		}
		if len(c.Env) > 0 {
			env := make([]interface{}, 0, len(c.Env))
			for _, e := range c.Env {
				env = append(env, buildPodPartEnv(e))
			}
			init["env"] = env
		}
		if len(c.VolumeMounts) > 0 {
			init["volumeMounts"] = buildPodPartMounts(c.VolumeMounts)
		}
		inits = append(inits, init)
	}
	if len(inits) > 0 {
		if err := jobutils.SetNestedField(obj.Object, inits, initPath); err != nil {
			return err
		}
	}

	containers, containersPath, err := getContainers(workload, obj, *resourceSpec)
	if err != nil {
		return err
	}
	mainName := commonworkload.GetMainContainer(workload, workload.SpecKind(), 0)
	for i := range containers {
		container, ok := containers[i].(map[string]interface{})
		if !ok || container["name"] != mainName {
			continue
		}
		taken := containerMountPaths([]interface{}{container})
		for _, m := range spec.VolumeMounts {
			for _, p := range taken {
				if pathsOverlap(m.MountPath, p) {
					return commonerrors.NewBadRequest(fmt.Sprintf("volumeMount %s overlaps %s", m.MountPath, p))
				}
			}
		}
		mounts, _ := container["volumeMounts"].([]interface{})
		container["volumeMounts"] = append(mounts, buildPodPartMounts(spec.VolumeMounts)...)
		for _, e := range spec.EnvRefs {
			setEnvEntry(container, buildPodPartEnv(e))
		}
		containers[i] = container
	}
	return jobutils.SetNestedField(obj.Object, containers, containersPath)
}

// podPartSecrets returns the secret ids that the workload mounts through its own volumes.
func podPartSecrets(workload *v1.Workload) map[string]bool {
	result := map[string]bool{}
	if workload == nil {
		return result
	}
	for _, vol := range workload.Spec.Volumes {
		if vol.Secret != "" {
			result[vol.Secret] = true
		}
	}
	return result
}

// buildPodPartVolume renders one workload volume.
func buildPodPartVolume(vol v1.PodVolume) map[string]interface{} {
	out := map[string]interface{}{"name": vol.Name}
	switch {
	case vol.EmptyDir != nil:
		emptyDir := map[string]interface{}{}
		if vol.EmptyDir.Medium != "" {
			emptyDir["medium"] = vol.EmptyDir.Medium
		}
		if vol.EmptyDir.SizeLimit != "" {
			emptyDir["sizeLimit"] = vol.EmptyDir.SizeLimit
		}
		out["emptyDir"] = emptyDir
	case len(vol.DownwardAPI) > 0:
		items := make([]interface{}, 0, len(vol.DownwardAPI))
		for _, f := range vol.DownwardAPI {
			items = append(items, map[string]interface{}{
				"path":     f.Path,
				"fieldRef": map[string]interface{}{"fieldPath": f.FieldPath},
			})
		}
		out["downwardAPI"] = map[string]interface{}{"items": items}
	case vol.ConfigMap != "":
		out["configMap"] = map[string]interface{}{"name": vol.ConfigMap}
	case vol.Secret != "":
		out["secret"] = map[string]interface{}{"secretName": vol.Secret}
	}
	return out
}

// buildPodPartMounts renders workload volume mounts.
func buildPodPartMounts(mounts []v1.PodVolumeMount) []interface{} {
	out := make([]interface{}, 0, len(mounts))
	for _, m := range mounts {
		mount := map[string]interface{}{"name": m.Name, "mountPath": m.MountPath}
		if m.ReadOnly {
			mount["readOnly"] = true
		}
		out = append(out, mount)
	}
	return out
}

// buildPodPartEnv renders one environment variable.
func buildPodPartEnv(e v1.PodEnvVar) map[string]interface{} {
	switch {
	case e.FieldPath != "":
		return map[string]interface{}{"name": e.Name, "valueFrom": map[string]interface{}{
			"fieldRef": map[string]interface{}{"fieldPath": e.FieldPath},
		}}
	case e.ConfigMapKeyRef != nil:
		return map[string]interface{}{"name": e.Name, "valueFrom": map[string]interface{}{
			"configMapKeyRef": map[string]interface{}{"name": e.ConfigMapKeyRef.Name, "key": e.ConfigMapKeyRef.Key},
		}}
	default:
		return map[string]interface{}{"name": e.Name, "value": e.Value}
	}
}

// setEnvEntry sets one container env entry, replacing an entry with the same name.
func setEnvEntry(container map[string]interface{}, entry map[string]interface{}) {
	envs, _ := container["env"].([]interface{})
	for i, item := range envs {
		if env, ok := item.(map[string]interface{}); ok && env["name"] == entry["name"] {
			envs[i] = entry
			container["env"] = envs
			return
		}
	}
	container["env"] = append(envs, entry)
}

// stringsToInterfaces converts a string slice for an unstructured object.
func stringsToInterfaces(in []string) []interface{} {
	out := make([]interface{}, 0, len(in))
	for _, s := range in {
		out = append(out, s)
	}
	return out
}

// containerMountPaths lists the mount paths already present on the containers.
func containerMountPaths(containers []interface{}) []string {
	var out []string
	for _, item := range containers {
		container, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		mounts, _ := container["volumeMounts"].([]interface{})
		for _, m := range mounts {
			if raw, ok := m.(map[string]interface{}); ok {
				if p, _ := raw["mountPath"].(string); p != "" {
					out = append(out, path.Clean(p))
				}
			}
		}
	}
	return out
}

// pathsOverlap reports whether a equals b or one contains the other.
func pathsOverlap(a, b string) bool {
	return a == b || strings.HasPrefix(a, b+"/") || strings.HasPrefix(b, a+"/")
}
