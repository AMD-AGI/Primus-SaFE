/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package dispatcher

import (
	"context"
	"strings"
	"testing"

	"gotest.tools/assert"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonworkload "github.com/AMD-AIG-AIMA/SAFE/common/pkg/workload"
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
	assertSandboxPodPosture(t, obj, false)
}

// assertVirtualKubeletSandboxPodSecurity checks the posture of a Sandbox Pod bound for
// an Autopilot virtual-kubelet node: everything assertSandboxPodSecurity checks except
// that no seccomp profile and no capabilities are set anywhere, since that backend
// refuses a manifest with either.
func assertVirtualKubeletSandboxPodSecurity(t *testing.T, obj *unstructured.Unstructured) {
	t.Helper()
	assertSandboxPodPosture(t, obj, true)
}

// assertVirtualKubeletContainerSecurity is assertSandboxContainerSecurity for a
// virtual-kubelet node: no privilege, no privilege escalation, and neither a
// capabilities field nor a seccomp profile.
func assertVirtualKubeletContainerSecurity(t *testing.T, name string, raw interface{}) {
	t.Helper()
	container := raw.(map[string]interface{})
	sc, ok := container["securityContext"].(map[string]interface{})
	assert.Assert(t, ok, "%s has no securityContext", name)
	assert.Equal(t, sc["privileged"], false, "%s privileged", name)
	assert.Equal(t, sc["allowPrivilegeEscalation"], false, "%s allowPrivilegeEscalation", name)
	_, hasProfile := sc["seccompProfile"]
	assert.Assert(t, !hasProfile, "%s has a seccomp profile the virtual-kubelet backend refuses", name)
	_, hasCaps := sc["capabilities"]
	assert.Assert(t, !hasCaps, "%s has capabilities the virtual-kubelet backend refuses", name)
}

func assertSandboxPodPosture(t *testing.T, obj *unstructured.Unstructured, onVirtualKubelet bool) {
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
	_, hasProfile, _ := unstructured.NestedFieldNoCopy(spec, "securityContext", "seccompProfile")
	if onVirtualKubelet {
		assert.Assert(t, !hasProfile, "the Pod has a seccomp profile the virtual-kubelet backend refuses")
	} else {
		seccomp, _, _ := unstructured.NestedString(spec, "securityContext", "seccompProfile", "type")
		assert.Equal(t, seccomp, "RuntimeDefault")
	}
	containers := spec["containers"].([]interface{})
	inits, _ := spec["initContainers"].([]interface{})
	assert.Assert(t, len(containers) > 0)
	assert.Assert(t, len(inits) > 0, "the envd injector should be rendered")
	for _, c := range append(append([]interface{}{}, containers...), inits...) {
		name := c.(map[string]interface{})["name"].(string)
		if onVirtualKubelet {
			assertVirtualKubeletContainerSecurity(t, name, c)
		} else {
			assertSandboxContainerSecurity(t, name, c)
		}
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

// A Sandbox whose template has no init containers, or a container entry that is not
// an object, is still rendered; the bad entry is left alone.
func TestSandboxPodSecurityToleratesMissingAndOddEntries(t *testing.T) {
	w := &v1.Workload{}
	w.Spec.GroupVersionKind = v1.GroupVersionKind{Version: common.DefaultVersion, Kind: common.SandboxKind}
	obj, spec := sandboxPodObject(t, w, map[string]interface{}{
		"containers": []interface{}{"not-a-container", map[string]interface{}{"name": "main"}},
	})
	assert.NilError(t, applySandboxPodSecurity(obj, w, spec))
	containers, _, err := jobutils.NestedSlice(obj.Object, podSpecPath(w, spec, "containers"))
	assert.NilError(t, err)
	assert.Equal(t, containers[0], "not-a-container")
	assertSandboxContainerSecurity(t, "main", containers[1])
	_, found, _ := jobutils.NestedSlice(obj.Object, podSpecPath(w, spec, "initContainers"))
	assert.Assert(t, !found)
}

// A template whose pod fields have the wrong shape makes applySandboxPodSecurity
// fail, which fails the dispatch instead of producing a Sandbox without the posture.
// A template with nothing at the resource spec's PrePaths is covered by
// TestSandboxPodSecurityRefusesMissingPod.
func TestSandboxPodSecurityRefusesMalformedTemplate(t *testing.T) {
	w := &v1.Workload{}
	w.Spec.GroupVersionKind = v1.GroupVersionKind{Version: common.DefaultVersion, Kind: common.SandboxKind}
	for name, podSpec := range map[string]map[string]interface{}{
		"securityContext is not an object": {"securityContext": "x"},
		"containers is not a list":         {"containers": "x"},
		"initContainers is not a list":     {"initContainers": "x"},
	} {
		t.Run(name, func(t *testing.T) {
			obj, spec := sandboxPodObject(t, w, podSpec)
			assert.Assert(t, applySandboxPodSecurity(obj, w, spec) != nil)
		})
	}
	spec := jobutils.TestSandboxResourceTemplate.Spec.ResourceSpecs[0]
	path := podSpecPath(w, &spec)
	obj := &unstructured.Unstructured{Object: map[string]interface{}{}}
	assert.NilError(t, jobutils.SetNestedField(obj.Object, "not-a-pod-spec", path))
	assert.Assert(t, applySandboxPodSecurity(obj, w, &spec) != nil, "a pod spec that is not an object")
}

// A Sandbox object with nothing at its PrePaths is refused on create and on sync. For
// other kinds a missing PrePaths still means an absent optional role and is skipped.
func TestSandboxPodSecurityRefusesMissingPod(t *testing.T) {
	sandbox := &v1.Workload{}
	sandbox.Spec.GroupVersionKind = v1.GroupVersionKind{Version: common.DefaultVersion, Kind: common.SandboxKind}
	sandbox.Spec.Resources = []v1.WorkloadResource{{Replica: 1}}
	rt := jobutils.TestSandboxResourceTemplate
	for name, object := range map[string]map[string]interface{}{
		"no spec":                      {},
		"podTemplate is not an object": {"spec": map[string]interface{}{"podTemplate": "x"}},
	} {
		t.Run(name, func(t *testing.T) {
			obj := &unstructured.Unstructured{Object: object}
			if _, ok := object["spec"]; !ok {
				err := initializeObject(obj, sandbox, nil, &rt.Spec.ResourceSpecs[0], 0)
				assert.ErrorContains(t, err, "sandbox template has no object at [spec]")
				assert.ErrorContains(t, enforceSandboxPodSecurity(obj, sandbox, rt),
					"sandbox template has no object at [spec]")
				return
			}
			assert.Assert(t, enforceSandboxPodSecurity(obj, sandbox, rt) != nil)
		})
	}
	other := sandbox.DeepCopy()
	other.Spec.GroupVersionKind.Kind = common.PytorchJobKind
	assert.NilError(t, initializeObject(&unstructured.Unstructured{Object: map[string]interface{}{}},
		other, nil, &rt.Spec.ResourceSpecs[0], 0))
}

// A Sandbox created with forceHostNetwork or an RDMA resource keeps the posture after
// a spec sync changes its image, env, resources or entrypoint: syncWorkloadToObject
// runs updateHostNetwork and updateContainers, and the object it writes back must
// still have hostNetwork=false and the hardened containers.
func TestSandboxPodSecuritySurvivesSync(t *testing.T) {
	hostNetwork := map[string]func(*v1.Workload){
		"forceHostNetwork": func(w *v1.Workload) { v1.SetAnnotation(w, v1.ForceHostNetworkAnnotation, v1.TrueStr) },
		"rdma":             func(w *v1.Workload) { w.Spec.Resources[0].RdmaResource = "1" },
	}
	changes := map[string]struct {
		change func(*v1.Workload)
		landed func(t *testing.T, main map[string]interface{})
	}{
		"image": {
			change: func(w *v1.Workload) { w.Spec.Images[0] = "registry.example/sandbox:v2" },
			landed: func(t *testing.T, main map[string]interface{}) {
				assert.Equal(t, main["image"], "registry.example/sandbox:v2")
			},
		},
		"env": {
			change: func(w *v1.Workload) { w.Spec.Env["SYNC_TEST"] = "changed" },
			landed: func(t *testing.T, main map[string]interface{}) {
				assert.Equal(t, envByName(main["env"].([]interface{}))["SYNC_TEST"]["value"], "changed")
			},
		},
		"resource": {
			change: func(w *v1.Workload) { w.Spec.Resources[0].CPU = "7" },
			landed: func(t *testing.T, main map[string]interface{}) {
				cpu, _, _ := unstructured.NestedFieldNoCopy(main, "resources", "limits", "cpu")
				assert.Equal(t, cpu, "7")
			},
		},
		"entrypoint": {
			change: func(w *v1.Workload) { w.Spec.EntryPoints = []string{"echo synced"} },
			landed: func(t *testing.T, main map[string]interface{}) {
				command := main["command"].([]interface{})
				assert.Assert(t, strings.HasSuffix(command[len(command)-1].(string), "echo synced"))
			},
		},
	}
	for hostName, enableHost := range hostNetwork {
		for changeName, tc := range changes {
			t.Run(hostName+"/"+changeName, func(t *testing.T) {
				r, w := agentSandboxFixture(t, enableHost)
				assert.Assert(t, commonworkload.IsEnabledHostNetwork(w, 0), "the fixture must ask for hostNetwork")
				created, err := r.generateK8sObject(context.Background(), w, nil)
				assert.NilError(t, err)
				assertSandboxPodSecurity(t, created)

				tc.change(w)
				cs, read := proxyDynamicClient(t, created)
				current, _ := read()
				assert.NilError(t, r.syncWorkloadToObject(context.Background(), w, cs, current))
				synced, updates := read()
				assert.Equal(t, updates, 1, "the sync must write the object")
				assertSandboxPodSecurity(t, synced)
				tc.landed(t, podItems(t, synced, "containers")[v1.GetMainContainer(w)])
			})
		}
	}
}

// externalSandbox marks a Sandbox workload as admitted against external capacity, which
// sends its Pod to an Autopilot virtual-kubelet node.
func externalSandbox(w *v1.Workload) {
	w.Spec.Resources[0].CPU = "4"
	w.Spec.Resources[0].Memory = "8Gi"
	w.Status.ExternalExecution = &v1.WorkloadExternalExecution{
		ClaimId: "claim-1",
		Placements: []v1.WorkloadExternalPlacement{{UnitKey: v1.ExternalSingleUnitKey, NodeName: "vk-1",
			ImageRef: w.Spec.Images[0], CPUMillis: 4000, MemoryBytes: 8 << 30}},
	}
}

// On the external path the Pod goes to an Autopilot virtual-kubelet node, whose backend
// refuses any seccomp profile and any capability added or dropped. The create path
// (applyWorkloadSpecToObject, then initializeObject with applyExternalContainerSecurity
// and applySandboxPodSecurity last) yields no seccomp profile and no capabilities
// anywhere, no container runAsUser, and the rest of the posture: hostNetwork false even
// though externalHostNetworkEnabled turned it on for forceHostNetwork, no token, no
// privilege and no privilege escalation.
func TestSandboxPodSecurityOnExternalPath(t *testing.T) {
	for name, placement := range map[string]func(*v1.Workload){
		"claim": func(*v1.Workload) {},
		"kube-scheduler": func(w *v1.Workload) {
			w.Status.ExternalExecution.ClaimId = ""
			w.Status.ExternalExecution.PlacementMode = v1.ExternalPlacementKubeScheduler
		},
	} {
		t.Run(name, func(t *testing.T) {
			obj, err := renderAgentSandbox(t, func(w *v1.Workload) {
				v1.SetAnnotation(w, v1.ForceHostNetworkAnnotation, v1.TrueStr)
				externalSandbox(w)
				placement(w)
			})
			assert.NilError(t, err)
			assertVirtualKubeletSandboxPodSecurity(t, obj)
			for name, c := range podItems(t, obj, "containers") {
				sc := c["securityContext"].(map[string]interface{})
				_, hasUser := sc["runAsUser"]
				_, hasGroup := sc["runAsGroup"]
				assert.Assert(t, !hasUser && !hasGroup, "%s keeps the external path's cleared identity", name)
			}
		})
	}
}

// A template that brings its own seccomp profile and capabilities, on the Pod and on
// the containers, loses all of them when the Sandbox goes to a virtual-kubelet node,
// and the same object for a Sandbox elsewhere gets the full posture. The Pod's other
// securityContext fields are kept on both.
func TestSandboxPodSecurityRemovesSeccompAndCapabilitiesForVirtualKubelet(t *testing.T) {
	template := func() map[string]interface{} {
		return map[string]interface{}{
			"hostNetwork":                  true,
			"hostPID":                      true,
			"automountServiceAccountToken": true,
			"securityContext": map[string]interface{}{
				"runAsUser":      int64(0),
				"seccompProfile": map[string]interface{}{"type": "Localhost", "localhostProfile": "x.json"},
			},
			"containers": []interface{}{map[string]interface{}{
				"name": "main",
				"securityContext": map[string]interface{}{
					"privileged":               true,
					"allowPrivilegeEscalation": true,
					"capabilities": map[string]interface{}{
						"add": []interface{}{"SYS_ADMIN"}, "drop": []interface{}{"NET_RAW"}},
					"seccompProfile": map[string]interface{}{"type": "Unconfined"},
				},
			}},
			"initContainers": []interface{}{map[string]interface{}{
				"name": "init",
				"securityContext": map[string]interface{}{
					"capabilities": map[string]interface{}{"drop": []interface{}{"ALL"}},
				},
			}},
		}
	}
	for name, tc := range map[string]struct {
		mutate func(*v1.Workload)
		assert func(*testing.T, *unstructured.Unstructured)
	}{
		"virtual-kubelet": {mutate: externalSandbox, assert: assertVirtualKubeletSandboxPodSecurity},
		"elsewhere":       {mutate: func(*v1.Workload) {}, assert: assertSandboxPodSecurity},
	} {
		t.Run(name, func(t *testing.T) {
			w := jobutils.TestWorkloadData.DeepCopy()
			w.Spec.GroupVersionKind = v1.GroupVersionKind{Version: common.DefaultVersion, Kind: common.SandboxKind}
			tc.mutate(w)
			obj, spec := sandboxPodObject(t, w, template())
			assert.NilError(t, applySandboxPodSecurity(obj, w, spec))
			tc.assert(t, obj)
			runAsUser, found, _ := unstructured.NestedInt64(obj.Object,
				append(podSpecPath(w, spec), "securityContext", "runAsUser")...)
			assert.Assert(t, found)
			assert.Equal(t, runAsUser, int64(0), "the Pod's other securityContext fields are kept")
		})
	}
}

// A Pod-level securityContext that is not an object is refused for a virtual-kubelet
// Sandbox as it is for any other, instead of being left with whatever it holds.
func TestSandboxPodSecurityRefusesMalformedSecurityContextForVirtualKubelet(t *testing.T) {
	w := jobutils.TestWorkloadData.DeepCopy()
	w.Spec.GroupVersionKind = v1.GroupVersionKind{Version: common.DefaultVersion, Kind: common.SandboxKind}
	externalSandbox(w)
	obj, spec := sandboxPodObject(t, w, map[string]interface{}{"securityContext": "x"})
	assert.Assert(t, applySandboxPodSecurity(obj, w, spec) != nil)
}

// The sync-path entry point, enforceSandboxPodSecurity, gives an object that already
// has the full posture (the create path's first pass, or a Sandbox rendered before it
// was admitted externally) the virtual-kubelet posture once the workload is external,
// and gives it back the full posture when it is not.
func TestSandboxPodSecurityEnforceFollowsBackend(t *testing.T) {
	created, err := renderAgentSandbox(t, func(*v1.Workload) {})
	assert.NilError(t, err)
	assertSandboxPodSecurity(t, created)

	_, w := agentSandboxFixture(t, externalSandbox)
	rt := jobutils.TestSandboxResourceTemplate
	obj := created.DeepCopy()
	assert.NilError(t, enforceSandboxPodSecurity(obj, w, rt))
	assertVirtualKubeletSandboxPodSecurity(t, obj)

	w.Status.ExternalExecution = nil
	assert.NilError(t, enforceSandboxPodSecurity(obj, w, rt))
	assertSandboxPodSecurity(t, obj)
}

// A spec sync does not rewrite an external Sandbox (its claim fixed the image and
// vector), so the virtual-kubelet posture it was created with stays as it is.
func TestSandboxPodSecurityExternalSyncKeepsVirtualKubeletPosture(t *testing.T) {
	r, w := agentSandboxFixture(t, func(w *v1.Workload) {
		v1.SetAnnotation(w, v1.ForceHostNetworkAnnotation, v1.TrueStr)
		externalSandbox(w)
	})
	created, err := r.generateK8sObject(context.Background(), w, nil)
	assert.NilError(t, err)
	assertVirtualKubeletSandboxPodSecurity(t, created)

	w.Spec.Images[0] = "registry.example/sandbox:v2"
	cs, read := proxyDynamicClient(t, created)
	current, _ := read()
	assert.NilError(t, r.syncWorkloadToObject(context.Background(), w, cs, current))
	synced, updates := read()
	assert.Equal(t, updates, 0, "an external object is not synced in place")
	assertVirtualKubeletSandboxPodSecurity(t, synced)
}
