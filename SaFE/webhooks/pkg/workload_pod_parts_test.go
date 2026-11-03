/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package webhooks

import (
	"testing"

	"gotest.tools/assert"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
)

func podPartsWorkload() *v1.Workload {
	w := &v1.Workload{}
	w.Spec.GroupVersionKind = v1.GroupVersionKind{Version: common.DefaultVersion, Kind: common.SandboxKind}
	w.Spec.Secrets = []v1.SecretEntity{{Id: "key", Type: v1.SecretGeneral}, {Id: "pull", Type: v1.SecretImage}}
	w.Spec.Volumes = []v1.PodVolume{
		{Name: "bin", EmptyDir: &v1.PodEmptyDir{}},
		{Name: "key-src", Secret: "key"},
		{Name: "session", DownwardAPI: []v1.PodDownwardFile{{Path: "session-id", FieldPath: "metadata.name"}}},
	}
	w.Spec.InitContainers = []v1.PodInitContainer{{Name: "inject", Image: "img",
		VolumeMounts: []v1.PodVolumeMount{{Name: "bin", MountPath: "/shared/bin"}, {Name: "key-src", MountPath: "/etc/key"}}}}
	w.Spec.VolumeMounts = []v1.PodVolumeMount{{Name: "bin", MountPath: "/shared/bin"}}
	w.Spec.EnvRefs = []v1.PodEnvVar{{Name: "NODE_NAME", FieldPath: "spec.nodeName"},
		{Name: "KEY", ConfigMapKeyRef: &v1.PodConfigMapKey{Name: "cm", Key: "k"}}}
	return w
}

func TestValidatePodParts(t *testing.T) {
	assert.NilError(t, validatePodParts(podPartsWorkload()))
	cases := map[string]func(*v1.Workload){
		"other kind":         func(w *v1.Workload) { w.Spec.Kind = common.PytorchJobKind },
		"image secret":       func(w *v1.Workload) { w.Spec.Volumes[1].Secret = "pull" },
		"unlisted secret":    func(w *v1.Workload) { w.Spec.Volumes[1].Secret = "nope" },
		"host path source":   func(w *v1.Workload) { w.Spec.Volumes[0].EmptyDir = nil },
		"two sources":        func(w *v1.Workload) { w.Spec.Volumes[0].ConfigMap = "cm" },
		"disk medium":        func(w *v1.Workload) { w.Spec.Volumes[0].EmptyDir.Medium = "HugePages" },
		"escaping file":      func(w *v1.Workload) { w.Spec.Volumes[2].DownwardAPI[0].Path = "../x" },
		"duplicate volume":   func(w *v1.Workload) { w.Spec.Volumes[2].Name = "bin" },
		"unknown mount":      func(w *v1.Workload) { w.Spec.VolumeMounts[0].Name = "nope" },
		"relative mount":     func(w *v1.Workload) { w.Spec.VolumeMounts[0].MountPath = "shared" },
		"overlapping mounts": func(w *v1.Workload) { w.Spec.InitContainers[0].VolumeMounts[1].MountPath = "/shared/bin/key" },
		"no image":           func(w *v1.Workload) { w.Spec.InitContainers[0].Image = "" },
		"bad name":           func(w *v1.Workload) { w.Spec.InitContainers[0].Name = "Inject" },
		"two refs": func(w *v1.Workload) {
			w.Spec.EnvRefs[0].ConfigMapKeyRef = &v1.PodConfigMapKey{Name: "cm", Key: "k"}
		},
		"value and ref": func(w *v1.Workload) { w.Spec.EnvRefs[0].Value = "x" },
	}
	for name, mutate := range cases {
		w := podPartsWorkload()
		mutate(w)
		assert.Assert(t, validatePodParts(w) != nil, name)
	}
}

func TestPodPartsAreImmutable(t *testing.T) {
	old, changed := podPartsWorkload(), podPartsWorkload()
	assert.NilError(t, validatePodPartsImmutable(changed, old))
	changed.Spec.InitContainers[0].Image = "other"
	assert.Assert(t, validatePodPartsImmutable(changed, old) != nil)
}
