/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package dispatcher

import (
	"testing"

	"gotest.tools/assert"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	jobutils "github.com/AMD-AIG-AIMA/SAFE/job-manager/pkg/utils"
)

// assertSandboxContainerSecurity checks the container posture the sandbox-pod-guardrails
// admission policy admits.
func assertSandboxContainerSecurity(t *testing.T, name string, raw interface{}) {
	t.Helper()
	container := raw.(map[string]interface{})
	sc, ok := container["securityContext"].(map[string]interface{})
	assert.Assert(t, ok, "%s has no securityContext", name)
	assert.Equal(t, sc["privileged"], false, "%s privileged", name)
	assert.Equal(t, sc["allowPrivilegeEscalation"], false, "%s allowPrivilegeEscalation", name)
	_, hasProfile := sc["seccompProfile"]
	assert.Assert(t, !hasProfile, "%s overrides the Pod's seccomp profile", name)
	caps := sc["capabilities"].(map[string]interface{})
	assert.DeepEqual(t, caps["drop"], []interface{}{"ALL"})
	assert.DeepEqual(t, caps["add"], []interface{}{
		"AUDIT_WRITE", "CHOWN", "DAC_OVERRIDE", "FOWNER", "FSETID", "KILL",
		"NET_BIND_SERVICE", "SETGID", "SETUID",
	})
}

// sandboxPodObject puts podSpec where the Sandbox resource template keeps the pod spec.
func sandboxPodObject(t *testing.T, w *v1.Workload, podSpec map[string]interface{}) (*unstructured.Unstructured, *v1.ResourceSpec) {
	t.Helper()
	spec := jobutils.TestSandboxResourceTemplate.Spec.ResourceSpecs[0]
	obj := &unstructured.Unstructured{Object: map[string]interface{}{}}
	assert.NilError(t, jobutils.SetNestedField(obj.Object, podSpec, podSpecPath(w, &spec)))
	return obj, &spec
}

func assertSandboxPodSecurity(t *testing.T, obj *unstructured.Unstructured) {
	t.Helper()
	sandbox := &v1.Workload{}
	sandbox.Spec.GroupVersionKind = v1.GroupVersionKind{Version: common.DefaultVersion, Kind: common.SandboxKind}
	spec, found, err := jobutils.NestedMap(obj.Object,
		podSpecPath(sandbox, &jobutils.TestSandboxResourceTemplate.Spec.ResourceSpecs[0]))
	assert.NilError(t, err)
	assert.Assert(t, found)
	assert.Equal(t, spec["automountServiceAccountToken"], false)
	assert.Equal(t, spec["hostNetwork"], false)
	assert.Equal(t, spec["hostPID"], false)
	assert.Equal(t, spec["hostIPC"], false)
	seccomp, _, _ := unstructured.NestedString(spec, "securityContext", "seccompProfile", "type")
	assert.Equal(t, seccomp, "RuntimeDefault")
	containers := spec["containers"].([]interface{})
	inits, _ := spec["initContainers"].([]interface{})
	assert.Assert(t, len(containers) > 0)
	assert.Assert(t, len(inits) > 0, "the envd injector should be rendered")
	for _, c := range append(append([]interface{}{}, containers...), inits...) {
		assertSandboxContainerSecurity(t, c.(map[string]interface{})["name"].(string), c)
	}
}

// A Sandbox rendered from the chart template, with the envd injector and a template
// layer, carries the posture on the Pod, the main container and every init container.
func TestSandboxPodSecurityDefaults(t *testing.T) {
	obj, err := renderAgentSandbox(t, func(w *v1.Workload) {
		v1.SetAnnotation(w, sandboxLayersAnnotation,
			`[{"image":"`+testLayerImage+`","from":"/layer","mount":"/opt/host","readOnly":true}]`)
	})
	assert.NilError(t, err)
	assertSandboxPodSecurity(t, obj)
	inits := podItems(t, obj, "initContainers")
	_, hasLayer := inits["as-layer-0"]
	assert.Assert(t, hasLayer)
}

// The privileged annotation, which makes other kinds run privileged as root, does not
// reach a Sandbox Pod.
func TestSandboxPodSecurityIgnoresPrivilegedAnnotation(t *testing.T) {
	obj, err := renderAgentSandbox(t, func(w *v1.Workload) {
		v1.SetAnnotation(w, v1.WorkloadPrivilegedAnnotation, v1.TrueStr)
	})
	assert.NilError(t, err)
	assertSandboxPodSecurity(t, obj)
}

// A container-level securityContext a template brought along (capabilities it adds, a
// seccomp profile of its own) is replaced, not merged.
func TestSandboxPodSecurityReplacesTemplateCapabilities(t *testing.T) {
	w := &v1.Workload{}
	w.Spec.GroupVersionKind = v1.GroupVersionKind{Version: common.DefaultVersion, Kind: common.SandboxKind}
	obj, spec := sandboxPodObject(t, w, map[string]interface{}{
		"hostNetwork":                  true,
		"automountServiceAccountToken": true,
		"securityContext":              map[string]interface{}{"runAsUser": int64(0)},
		"containers": []interface{}{map[string]interface{}{
			"name": "main",
			"securityContext": map[string]interface{}{
				"privileged":     true,
				"capabilities":   map[string]interface{}{"add": []interface{}{"SYS_ADMIN", "NET_RAW"}},
				"seccompProfile": map[string]interface{}{"type": "Unconfined"},
			},
		}},
		"initContainers": []interface{}{map[string]interface{}{"name": "init"}},
	})
	assert.NilError(t, applySandboxPodSecurity(obj, w, spec))
	assertSandboxPodSecurity(t, obj)
	runAsUser, _, _ := unstructured.NestedInt64(obj.Object,
		append(podSpecPath(w, spec), "securityContext", "runAsUser")...)
	assert.Equal(t, runAsUser, int64(0), "the Pod's other securityContext fields are kept")
}

// Other kinds keep what they had.
func TestSandboxPodSecuritySkipsOtherKinds(t *testing.T) {
	w := &v1.Workload{}
	w.Spec.GroupVersionKind = v1.GroupVersionKind{Version: common.DefaultVersion, Kind: common.PytorchJobKind}
	container := map[string]interface{}{"name": "main", "securityContext": map[string]interface{}{"privileged": true}}
	obj, spec := sandboxPodObject(t, w, map[string]interface{}{"containers": []interface{}{container}})
	assert.NilError(t, applySandboxPodSecurity(obj, w, spec))
	_, found, _ := unstructured.NestedFieldNoCopy(obj.Object, append(podSpecPath(w, spec), "securityContext")...)
	assert.Assert(t, !found)
	assert.Equal(t, container["securityContext"].(map[string]interface{})["privileged"], true)
}
