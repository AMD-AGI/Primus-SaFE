/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package webhooks

import (
	"path"
	"reflect"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/apimachinery/pkg/util/validation/field"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
)

// hasPodParts reports whether the workload sets init containers, volumes, mounts or env refs.
func hasPodParts(spec *v1.WorkloadSpec) bool {
	return len(spec.InitContainers) > 0 || len(spec.Volumes) > 0 ||
		len(spec.VolumeMounts) > 0 || len(spec.EnvRefs) > 0
}

// validatePodParts checks the init containers, volumes, mounts and env refs of a workload.
// They are accepted only on Sandbox. A secret volume must name a general secret of the workload.
func validatePodParts(workload *v1.Workload) error {
	spec := &workload.Spec
	if !hasPodParts(spec) {
		return nil
	}
	root := field.NewPath("spec")
	if workload.SpecKind() != common.SandboxKind {
		return field.Forbidden(root.Child("initContainers"), "init containers, volumes, volumeMounts and envRefs are only for "+common.SandboxKind)
	}
	generalSecrets := map[string]bool{}
	for _, s := range spec.Secrets {
		if s.Type == v1.SecretGeneral {
			generalSecrets[s.Id] = true
		}
	}
	volumes := map[string]bool{}
	for i, vol := range spec.Volumes {
		p := root.Child("volumes").Index(i)
		if err := validatePodPartName(p.Child("name"), vol.Name); err != nil {
			return err
		}
		if volumes[vol.Name] {
			return field.Duplicate(p.Child("name"), vol.Name)
		}
		volumes[vol.Name] = true
		if err := validatePodVolumeSource(p, vol, generalSecrets); err != nil {
			return err
		}
	}
	containers := map[string]bool{}
	for i, c := range spec.InitContainers {
		p := root.Child("initContainers").Index(i)
		if err := validatePodPartName(p.Child("name"), c.Name); err != nil {
			return err
		}
		if containers[c.Name] {
			return field.Duplicate(p.Child("name"), c.Name)
		}
		containers[c.Name] = true
		if c.Image == "" {
			return field.Required(p.Child("image"), "")
		}
		if err := validatePodEnv(p.Child("env"), c.Env); err != nil {
			return err
		}
		if err := validatePodMounts(p.Child("volumeMounts"), c.VolumeMounts, volumes); err != nil {
			return err
		}
	}
	if err := validatePodMounts(root.Child("volumeMounts"), spec.VolumeMounts, volumes); err != nil {
		return err
	}
	return validatePodEnv(root.Child("envRefs"), spec.EnvRefs)
}

// validatePodVolumeSource checks that a volume has exactly one valid source.
func validatePodVolumeSource(p *field.Path, vol v1.PodVolume, generalSecrets map[string]bool) error {
	sources := 0
	if vol.EmptyDir != nil {
		sources++
		if vol.EmptyDir.Medium != "" && vol.EmptyDir.Medium != string(corev1.StorageMediumMemory) {
			return field.NotSupported(p.Child("emptyDir", "medium"), vol.EmptyDir.Medium, []string{"", string(corev1.StorageMediumMemory)})
		}
		if vol.EmptyDir.SizeLimit != "" {
			if _, err := resource.ParseQuantity(vol.EmptyDir.SizeLimit); err != nil {
				return field.Invalid(p.Child("emptyDir", "sizeLimit"), vol.EmptyDir.SizeLimit, err.Error())
			}
		}
	}
	if len(vol.DownwardAPI) > 0 {
		sources++
		for j, f := range vol.DownwardAPI {
			fp := p.Child("downwardAPI").Index(j)
			if f.Path == "" || path.IsAbs(f.Path) || path.Clean(f.Path) != f.Path || f.Path == ".." || strings.HasPrefix(f.Path, "../") {
				return field.Invalid(fp.Child("path"), f.Path, "must be a clean relative path")
			}
			if f.FieldPath == "" {
				return field.Required(fp.Child("fieldPath"), "")
			}
		}
	}
	if vol.ConfigMap != "" {
		sources++
		if err := validateDNSSubdomain(p.Child("configMap"), vol.ConfigMap); err != nil {
			return err
		}
	}
	if vol.Secret != "" {
		sources++
		if !generalSecrets[vol.Secret] {
			return field.Invalid(p.Child("secret"), vol.Secret, "must be a general secret listed in spec.secrets")
		}
	}
	if sources != 1 {
		return field.Invalid(p, vol.Name, "exactly one of emptyDir, downwardAPI, configMap and secret must be set")
	}
	return nil
}

// validatePodMounts checks that mounts use declared volumes at distinct clean absolute paths.
func validatePodMounts(p *field.Path, mounts []v1.PodVolumeMount, volumes map[string]bool) error {
	var paths []string
	for i, m := range mounts {
		mp := p.Index(i)
		if !volumes[m.Name] {
			return field.NotFound(mp.Child("name"), m.Name)
		}
		if !path.IsAbs(m.MountPath) || path.Clean(m.MountPath) != m.MountPath || m.MountPath == "/" {
			return field.Invalid(mp.Child("mountPath"), m.MountPath, "must be a clean absolute path other than /")
		}
		for _, other := range paths {
			if pathsOverlap(m.MountPath, other) {
				return field.Invalid(mp.Child("mountPath"), m.MountPath, "overlaps "+other)
			}
		}
		paths = append(paths, m.MountPath)
	}
	return nil
}

// validatePodEnv checks that each variable is named and sets at most one reference.
func validatePodEnv(p *field.Path, env []v1.PodEnvVar) error {
	names := map[string]bool{}
	for i, e := range env {
		ep := p.Index(i)
		if e.Name == "" {
			return field.Required(ep.Child("name"), "")
		}
		if names[e.Name] {
			return field.Duplicate(ep.Child("name"), e.Name)
		}
		names[e.Name] = true
		refs := 0
		if e.FieldPath != "" {
			refs++
		}
		if e.ConfigMapKeyRef != nil {
			refs++
			if err := validateDNSSubdomain(ep.Child("configMapKeyRef", "name"), e.ConfigMapKeyRef.Name); err != nil {
				return err
			}
			if e.ConfigMapKeyRef.Key == "" {
				return field.Required(ep.Child("configMapKeyRef", "key"), "")
			}
		}
		if refs > 1 || refs == 1 && e.Value != "" {
			return field.Invalid(ep, e.Name, "set only one of value, fieldPath and configMapKeyRef")
		}
	}
	return nil
}

// validatePodPartsImmutable rejects changes to the pod parts after creation.
func validatePodPartsImmutable(newWorkload, oldWorkload *v1.Workload) error {
	n, o := &newWorkload.Spec, &oldWorkload.Spec
	root := field.NewPath("spec")
	if !reflect.DeepEqual(n.InitContainers, o.InitContainers) {
		return field.Forbidden(root.Child("initContainers"), "immutable")
	}
	if !reflect.DeepEqual(n.Volumes, o.Volumes) {
		return field.Forbidden(root.Child("volumes"), "immutable")
	}
	if !reflect.DeepEqual(n.VolumeMounts, o.VolumeMounts) {
		return field.Forbidden(root.Child("volumeMounts"), "immutable")
	}
	if !reflect.DeepEqual(n.EnvRefs, o.EnvRefs) {
		return field.Forbidden(root.Child("envRefs"), "immutable")
	}
	return nil
}

// validatePodPartName checks a DNS-1123 label.
func validatePodPartName(p *field.Path, name string) error {
	if errs := validation.IsDNS1123Label(name); len(errs) > 0 {
		return field.Invalid(p, name, errs[0])
	}
	return nil
}

// validateDNSSubdomain checks a DNS-1123 subdomain.
func validateDNSSubdomain(p *field.Path, name string) error {
	if errs := validation.IsDNS1123Subdomain(name); len(errs) > 0 {
		return field.Invalid(p, name, errs[0])
	}
	return nil
}

// pathsOverlap reports whether a equals b or one contains the other.
func pathsOverlap(a, b string) bool {
	return a == b || strings.HasPrefix(a, b+"/") || strings.HasPrefix(b, a+"/")
}
