/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package dispatcher

import (
	"testing"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	jobutils "github.com/AMD-AIG-AIMA/SAFE/job-manager/pkg/utils"
	"github.com/spf13/viper"
	"gotest.tools/v3/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// --- from external_gang_test.go ---

const workerDispatchImage = "docker.io/team/app@sha256:fedcba9876543210fedcba9876543210fedcba9876543210fedcba9876543210"
const pinnedDispatchImage = "docker.io/team/app@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func TestInferaUsesK8sDiscovery(t *testing.T) {
	infera := &v1.Workload{
		Spec: v1.WorkloadSpec{
			GroupVersionKind: v1.GroupVersionKind{Kind: common.InferaDeploymentKind, Version: "v1"},
		},
	}
	pytorch := &v1.Workload{
		Spec: v1.WorkloadSpec{
			GroupVersionKind: v1.GroupVersionKind{Kind: common.PytorchJobKind, Version: "v1"},
		},
	}
	cases := []struct {
		name string
		w    *v1.Workload
		obj  *unstructured.Unstructured
		want bool
	}{
		{name: "nil workload", want: false},
		{name: "non-infera", w: pytorch, want: false},
		{name: "infera default when field unset", w: infera, want: true},
		{
			name: "infera kubernetes",
			w:    infera,
			obj: &unstructured.Unstructured{Object: map[string]interface{}{
				"spec": map[string]interface{}{"discoveryBackend": "kubernetes"},
			}},
			want: true,
		},
		{
			name: "infera etcd",
			w:    infera,
			obj: &unstructured.Unstructured{Object: map[string]interface{}{
				"spec": map[string]interface{}{"discoveryBackend": "etcd"},
			}},
			want: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, inferaUsesK8sDiscovery(tc.w, tc.obj), tc.want)
		})
	}
}

// External Infera pods with kubernetes discovery must keep the projected SA
// token; other external pods keep automount disabled.
func TestExternalAutomountServiceAccountToken(t *testing.T) {
	viper.Set("global.domain", "primus-safe.amd.com")
	viper.Set("global.sub_domain", "global")
	t.Cleanup(func() {
		viper.Set("global.domain", "")
		viper.Set("global.sub_domain", "")
	})

	inferaSpec := v1.ResourceSpec{
		PrePaths:      []string{"spec", "services", "role0"},
		PodSpecPaths:  []string{"extraPodSpec"},
		TemplatePaths: []string{"extraPodSpec"},
	}
	inferaObj := &unstructured.Unstructured{Object: map[string]interface{}{
		"spec": map[string]interface{}{
			"discoveryBackend": "kubernetes",
			"services": map[string]interface{}{
				"role0": map[string]interface{}{
					"extraPodSpec": map[string]interface{}{
						"containers": []interface{}{
							map[string]interface{}{"name": "main"},
						},
					},
				},
			},
		},
	}}
	inferaWL := &v1.Workload{
		ObjectMeta: metav1.ObjectMeta{
			Name: "idep-ext",
			UID:  "22222222-2222-2222-2222-222222222222",
			Annotations: map[string]string{
				v1.MainContainerAnnotation:      "main",
				v1.InferaServiceRolesAnnotation: "frontend",
			},
		},
		Spec: v1.WorkloadSpec{
			GroupVersionKind: v1.GroupVersionKind{Kind: common.InferaDeploymentKind, Version: "v1"},
			Resources:        []v1.WorkloadResource{{Replica: 1, CPU: "1", Memory: "1Gi"}},
		},
		Status: v1.WorkloadStatus{ExternalExecution: &v1.WorkloadExternalExecution{
			ClaimId: "c-infera", DispatchGeneration: 1,
			Placements: []v1.WorkloadExternalPlacement{{
				UnitKey: v1.ExternalSingleUnitKey, NodeName: "vk-1", ImageRef: pinnedDispatchImage,
			}},
		}},
	}
	assert.NilError(t, initializeObject(inferaObj, inferaWL, nil, &inferaSpec, 0))
	automount, found, err := unstructured.NestedBool(inferaObj.Object,
		"spec", "services", "role0", "extraPodSpec", "automountServiceAccountToken")
	assert.NilError(t, err)
	assert.Assert(t, found)
	assert.Assert(t, automount, "Infera kubernetes discovery needs the SA token")

	etcdObj := inferaObj.DeepCopy()
	assert.NilError(t, unstructured.SetNestedField(etcdObj.Object, "etcd", "spec", "discoveryBackend"))
	// Re-apply only the automount decision against an etcd backend.
	path := podSpecPath(inferaWL, &inferaSpec, "automountServiceAccountToken")
	assert.NilError(t, jobutils.SetNestedField(etcdObj.Object, inferaUsesK8sDiscovery(inferaWL, etcdObj), path))
	etcdAutomount, etcdFound, err := unstructured.NestedBool(etcdObj.Object,
		"spec", "services", "role0", "extraPodSpec", "automountServiceAccountToken")
	assert.NilError(t, err)
	assert.Assert(t, etcdFound)
	assert.Assert(t, !etcdAutomount, "etcd discovery does not need the SA token")
}

func claimWorkload() *v1.Workload {
	return &v1.Workload{
		ObjectMeta: metav1.ObjectMeta{Name: "train-1", UID: "11111111-1111-1111-1111-111111111111"},
		Status: v1.WorkloadStatus{ExternalExecution: &v1.WorkloadExternalExecution{
			ClaimId: "c1", DispatchGeneration: 1,
			Placements: []v1.WorkloadExternalPlacement{{
				UnitKey: v1.ExternalSingleUnitKey, NodeName: "vk-1", ImageRef: pinnedDispatchImage,
			}},
		}},
	}
}

// externalGangWorkload returns an admitted RDMA PyTorchJob with one master and two workers.
func externalGangWorkload() *v1.Workload {
	w := claimWorkload()
	w.Spec.GroupVersionKind = v1.GroupVersionKind{Kind: common.PytorchJobKind, Version: "v1"}
	w.Spec.Resources = []v1.WorkloadResource{
		{Replica: 1, CPU: "8", Memory: "64Gi", GPU: "1", GPUName: "amd.com/gpu", RdmaResource: "1k"},
		{Replica: 2, CPU: "8", Memory: "64Gi", GPU: "1", GPUName: "amd.com/gpu", RdmaResource: "1k"},
	}
	w.Status.ExternalExecution.Placements = []v1.WorkloadExternalPlacement{
		{UnitKey: v1.ExternalSingleUnitKey, NodeName: "vk-a", ImageRef: pinnedDispatchImage, CPUMillis: 8000},
		{UnitKey: "worker/0", NodeName: "vk-b", ImageRef: pinnedDispatchImage, CPUMillis: 8000},
		{UnitKey: "worker/1", NodeName: "vk-c", ImageRef: pinnedDispatchImage, CPUMillis: 8000},
	}
	return w
}

// pinnedNodes returns the node named by each term's metadata.name pin.
func pinnedNodes(t *testing.T, obj *unstructured.Unstructured) []string {
	terms, _, err := unstructured.NestedSlice(obj.Object, "spec", "template", "spec", "affinity",
		"nodeAffinity", "requiredDuringSchedulingIgnoredDuringExecution", "nodeSelectorTerms")
	assert.NilError(t, err)
	var nodes []string
	for _, raw := range terms {
		for _, f := range raw.(map[string]interface{})["matchFields"].([]interface{}) {
			m := f.(map[string]interface{})
			if m["key"] != "metadata.name" || m["operator"] != "In" {
				continue
			}
			values := m["values"].([]interface{})
			assert.Equal(t, len(values), 1, "a node field selector takes exactly one name")
			nodes = append(nodes, values[0].(string))
		}
	}
	return nodes
}

// The API server accepts one name per metadata.name selector, so each role's approved
// nodes are pinned in separate terms.
func TestApplyExternalNodePinGangOneNodePerTerm(t *testing.T) {
	w := externalGangWorkload()
	master := &unstructured.Unstructured{Object: map[string]interface{}{}}
	assert.NilError(t, applyExternalNodePin(master, w, externalShapeSpec(), 0))
	assert.DeepEqual(t, pinnedNodes(t, master), []string{"vk-a"})

	worker := &unstructured.Unstructured{Object: map[string]interface{}{}}
	assert.NilError(t, applyExternalNodePin(worker, w, externalShapeSpec(), 1))
	assert.DeepEqual(t, pinnedNodes(t, worker), []string{"vk-b", "vk-c"})
}

// The gang is decided by the workload shape, not by how many units the claim returned.
func TestIsExternalGangFollowsWorkloadShape(t *testing.T) {
	short := externalGangWorkload()
	short.Status.ExternalExecution.Placements = short.Status.ExternalExecution.Placements[:1]
	assert.Assert(t, isExternalGang(short))

	single := claimWorkload()
	single.Status.ExternalExecution.Placements = append(single.Status.ExternalExecution.Placements,
		v1.WorkloadExternalPlacement{UnitKey: "worker/0", NodeName: "vk-2"})
	assert.Assert(t, !isExternalGang(single))
}

// A later host-network pass must not re-enable the host network on a single-unit external pod.
func TestUpdateHostNetworkKeepsExternalDecision(t *testing.T) {
	w := claimWorkload()
	w.Spec.Resources = []v1.WorkloadResource{{Replica: 1}}
	w.Annotations = map[string]string{v1.ForceHostNetworkAnnotation: v1.TrueStr}
	obj := &unstructured.Unstructured{Object: map[string]interface{}{}}
	assert.NilError(t, updateHostNetwork(w, obj, externalShapeSpec(), 0))
	enabled, _, _ := unstructured.NestedBool(obj.Object, "spec", "template", "spec", "hostNetwork")
	assert.Assert(t, !enabled)
}

// A worker template carries the worker unit's approval, not the master's.
func TestExternalRoleUnitKey(t *testing.T) {
	w := externalGangWorkload()
	w.Status.ExternalExecution.Placements[1].ImageRef = workerDispatchImage
	w.Status.ExternalExecution.Placements[1].CPUMillis = 4000
	assert.Equal(t, externalApprovedImage(w, externalRoleUnitKey(w, 0)), pinnedDispatchImage)
	assert.Equal(t, externalApprovedImage(w, externalRoleUnitKey(w, 1)), workerDispatchImage)
	resources, err := externalApprovedResourceMap(w, externalRoleUnitKey(w, 1))
	assert.NilError(t, err)
	assert.Equal(t, resources["cpu"], "4000m")
	assert.Equal(t, externalRoleUnitKey(claimWorkload(), 0), v1.ExternalSingleUnitKey)
}

func TestExternalGangProblem(t *testing.T) {
	tagged := externalGangWorkload()
	tagged.Status.ExternalExecution.Placements[2].ImageRef = "docker.io/team/app:v1"
	missing := externalGangWorkload()
	missing.Status.ExternalExecution.Placements = missing.Status.ExternalExecution.Placements[:2]
	shared := externalGangWorkload()
	shared.Status.ExternalExecution.Placements[2].NodeName = "vk-b"
	for name, w := range map[string]*v1.Workload{"tag image": tagged, "missing unit": missing, "shared node": shared} {
		if problem := externalGangProblem(w); problem == "" {
			t.Errorf("%s: expected a gang problem", name)
		}
	}
	if problem := externalGangProblem(externalGangWorkload()); problem != "" {
		t.Fatalf("good gang reported %q", problem)
	}
}

// externalEnvObject returns a pod object whose main container carries the given env names.
func externalEnvObject(names ...string) *unstructured.Unstructured {
	envs := make([]interface{}, 0, len(names))
	for _, n := range names {
		envs = append(envs, map[string]interface{}{"name": n, "value": "site"})
	}
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"spec": map[string]interface{}{"template": map[string]interface{}{"spec": map[string]interface{}{
			"containers": []interface{}{map[string]interface{}{"name": "main", "env": envs}},
		}}},
	}}
}

// envValues maps the main container env names to their values.
func envValues(t *testing.T, obj *unstructured.Unstructured) map[string]string {
	containers, _, err := unstructured.NestedSlice(obj.Object, "spec", "template", "spec", "containers")
	assert.NilError(t, err)
	result := map[string]string{}
	for _, raw := range containers[0].(map[string]interface{})["env"].([]interface{}) {
		m := raw.(map[string]interface{})
		result[m["name"].(string)], _ = m["value"].(string)
	}
	return result
}

// A gang leaves fabric settings to the node's site configuration and disables MSCCL.
func TestApplyExternalCommEnvGang(t *testing.T) {
	obj := externalEnvObject(append(append([]string{}, externalSiteCommEnvs...), "NCCL_DEBUG")...)
	assert.NilError(t, applyExternalCommEnv(obj, externalGangWorkload(), externalShapeSpec()))
	envs := envValues(t, obj)
	for _, name := range externalSiteCommEnvs {
		_, found := envs[name]
		assert.Assert(t, !found, "%s must be left to the site", name)
	}
	assert.Equal(t, envs["NCCL_DEBUG"], "site")
	assert.Equal(t, envs["GIT_CONFIG_COUNT"], "1")
	assert.Equal(t, envs["GIT_CONFIG_KEY_0"], "safe.directory")
	assert.Equal(t, envs["GIT_CONFIG_VALUE_0"], "*")
}

// A single-unit pod keeps its env; a value already set for MSCCL is kept.
func TestApplyExternalCommEnvSingleUnit(t *testing.T) {
	obj := externalEnvObject("NCCL_IB_HCA")
	assert.NilError(t, applyExternalCommEnv(obj, claimWorkload(), externalShapeSpec()))
	envs := envValues(t, obj)
	assert.Equal(t, envs["NCCL_IB_HCA"], "site")
	assert.Equal(t, envs["GIT_CONFIG_COUNT"], "1")
}

// A workload outside external capacity is left untouched.
func TestApplyExternalCommEnvSkipsNativeWorkload(t *testing.T) {
	native := externalGangWorkload()
	native.Status.ExternalExecution = nil
	obj := externalEnvObject("NCCL_IB_HCA")
	assert.NilError(t, applyExternalCommEnv(obj, native, externalShapeSpec()))
	assert.DeepEqual(t, envValues(t, obj), map[string]string{"NCCL_IB_HCA": "site"})
}

// --- from external_pod_test.go ---

func externalAuthoringPod() (*unstructured.Unstructured, *v1.Workload, v1.ResourceSpec) {
	obj := &unstructured.Unstructured{Object: map[string]interface{}{
		"spec": map[string]interface{}{
			"pytorchReplicaSpecs": map[string]interface{}{
				"Master": map[string]interface{}{
					"template": map[string]interface{}{
						"spec": map[string]interface{}{
							"hostNetwork": true,
							"containers": []interface{}{
								map[string]interface{}{
									"name": "pytorch",
									"env": []interface{}{
										map[string]interface{}{
											"name":  "WORKLOAD_MANAGER_URL",
											"value": "http://workloadmanager.agent-sandbox-system.svc.cluster.local:8080",
										},
										map[string]interface{}{
											"name":  "OTHER_SVC",
											"value": "http://apiserver.primus-safe.svc.cluster.local:8080/api/v1",
										},
									},
									"securityContext": map[string]interface{}{
										"privileged": true,
										"runAsUser":  int64(0),
										"runAsGroup": int64(0),
										"capabilities": map[string]interface{}{
											"add": []interface{}{"IPC_LOCK", "SYS_PTRACE"},
										},
									},
									"volumeMounts": []interface{}{
										map[string]interface{}{
											"name":      "varlog",
											"mountPath": "/var/log",
										},
										map[string]interface{}{
											"name":      "shared",
											"mountPath": "/shared_nfs/users/u1",
											"subPath":   "users/u1",
										},
									},
								},
							},
							"initContainers": []interface{}{
								map[string]interface{}{
									"name": "preprocess",
									"securityContext": map[string]interface{}{
										"capabilities": map[string]interface{}{
											"add": []interface{}{"IPC_LOCK"},
										},
									},
								},
							},
							"volumes": []interface{}{
								map[string]interface{}{
									"name": "varlog",
									"hostPath": map[string]interface{}{
										"path": "/var/log",
										"type": "Directory",
									},
								},
								map[string]interface{}{
									"name": "shared",
									"hostPath": map[string]interface{}{
										"path": "/shared_nfs",
										"type": "Directory",
									},
								},
							},
						},
					},
				},
			},
		},
	}}
	workload := &v1.Workload{
		ObjectMeta: metav1.ObjectMeta{
			Name: "ext-1",
			Labels: map[string]string{
				v1.UserIdLabel: "a1b2c3d4e5f6789012345678abcdef01",
			},
			Annotations: map[string]string{
				v1.UserAccountAnnotation: "jdoe",
			},
		},
		Spec: v1.WorkloadSpec{
			Workspace:        "ws-ext",
			GroupVersionKind: v1.GroupVersionKind{Kind: common.AuthoringKind, Version: "v1"},
		},
		Status: v1.WorkloadStatus{
			ExternalExecution: &v1.WorkloadExternalExecution{ClaimId: "claim-1"},
		},
	}
	spec := v1.ResourceSpec{
		PrePaths: []string{"spec", "pytorchReplicaSpecs", "Master", "template"},
	}
	return obj, workload, spec
}

func TestApplyExternalPodPolicy(t *testing.T) {
	viper.Set("global.domain", "primus-safe.amd.com")
	viper.Set("global.sub_domain", "global")
	t.Cleanup(func() {
		viper.Set("global.domain", "")
		viper.Set("global.sub_domain", "")
	})

	obj, workload, spec := externalAuthoringPod()
	assert.Assert(t, isExternalWorkload(workload))

	assert.NilError(t, applyExternalVirtualKubeletToleration(obj, workload, spec))
	assert.NilError(t, applyExternalContainerSecurity(obj, workload, spec))
	assert.NilError(t, applyExternalVolumePolicy(obj, workload, spec))
	assert.NilError(t, applyExternalEnvRewrite(obj, workload, spec))
	assert.NilError(t, validateExternalPodShape(obj, workload, spec))

	podSpec, _, err := unstructured.NestedMap(obj.Object,
		"spec", "pytorchReplicaSpecs", "Master", "template", "spec")
	assert.NilError(t, err)

	_, hasPodSC := podSpec["securityContext"]
	if hasPodSC {
		podSC := podSpec["securityContext"].(map[string]interface{})
		_, hasPodUID := podSC["runAsUser"]
		_, hasPodGID := podSC["runAsGroup"]
		assert.Assert(t, !hasPodUID && !hasPodGID, "pod-level runAs must not be set")
	}

	tolerations, _ := podSpec["tolerations"].([]interface{})
	found := map[string]bool{}
	for _, raw := range tolerations {
		tMap := raw.(map[string]interface{})
		key, _ := tMap["key"].(string)
		for _, want := range v1.ExternalVirtualKubeletTaintKeys() {
			if key == want {
				found[key] = true
				assert.Equal(t, tMap["operator"], "Exists")
				assert.Equal(t, tMap["effect"], "NoSchedule")
			}
		}
	}
	for _, want := range v1.ExternalVirtualKubeletTaintKeys() {
		assert.Assert(t, found[want], "missing VK toleration %s", want)
	}

	containers, _ := podSpec["containers"].([]interface{})
	main := containers[0].(map[string]interface{})
	sc := main["securityContext"].(map[string]interface{})
	assert.Equal(t, sc["privileged"], false)
	_, hasCaps := sc["capabilities"]
	assert.Assert(t, !hasCaps, "main capabilities.add should be cleared")
	_, hasRunAsUser := sc["runAsUser"]
	_, hasRunAsGroup := sc["runAsGroup"]
	assert.Assert(t, !hasRunAsUser && !hasRunAsGroup,
		"dispatcher must not set runAs; provider fills identity from account annotation")

	inits, _ := podSpec["initContainers"].([]interface{})
	initSC := inits[0].(map[string]interface{})["securityContext"].(map[string]interface{})
	_, hasInitCaps := initSC["capabilities"]
	assert.Assert(t, !hasInitCaps, "init capabilities.add should be cleared on the external path")
	assert.Equal(t, initSC["privileged"], false)
	assert.Equal(t, initSC["allowPrivilegeEscalation"], false)
	_, hasInitUID := initSC["runAsUser"]
	_, hasInitGID := initSC["runAsGroup"]
	assert.Assert(t, !hasInitUID && !hasInitGID, "init must keep no runAs")

	volumes, _ := podSpec["volumes"].([]interface{})
	varlog := volumes[0].(map[string]interface{})
	_, hasHostPath := varlog["hostPath"]
	_, hasEmptyDir := varlog["emptyDir"]
	assert.Assert(t, !hasHostPath && hasEmptyDir, "/var/log should become emptyDir")

	mounts, _ := main["volumeMounts"].([]interface{})
	sharedMount := mounts[1].(map[string]interface{})
	_, hasSubPath := sharedMount["subPath"]
	assert.Assert(t, !hasSubPath)
	assert.Equal(t, sharedMount["mountPath"], "/shared_nfs")

	envs, _ := main["env"].([]interface{})
	assert.Equal(t, envs[0].(map[string]interface{})["value"],
		"http://workloadmanager.agent-sandbox-system.svc.cluster.local:8080",
		"WORKLOAD_MANAGER_URL must stay on the execution-cluster Service")
	assert.Equal(t, envs[1].(map[string]interface{})["value"],
		"https://global.primus-safe.amd.com/api/v1")
}

func TestBuildObjectAnnotationsIncludesUserAccount(t *testing.T) {
	_, workload, _ := externalAuthoringPod()
	annos := buildObjectAnnotations(workload)
	assert.Equal(t, annos[v1.UserAccountAnnotation], "jdoe")
}

// --- from external_shape_test.go ---

func externalShapeWorkload() *v1.Workload {
	return &v1.Workload{
		ObjectMeta: metav1.ObjectMeta{Name: "w"},
		Status: v1.WorkloadStatus{ExternalExecution: &v1.WorkloadExternalExecution{
			ClaimId: "claim-1",
			Placements: []v1.WorkloadExternalPlacement{{
				UnitKey: v1.ExternalSingleUnitKey, NodeName: "vk-approved",
			}},
		}},
	}
}

func externalShapeSpec() v1.ResourceSpec {
	return v1.ResourceSpec{PrePaths: []string{"spec"}, PodSpecPaths: []string{"template", "spec"}}
}

// The claim approves one vector; a second container would request it again.
func TestValidateExternalPodShapeRejectsSidecars(t *testing.T) {
	obj := &unstructured.Unstructured{Object: map[string]interface{}{
		"spec": map[string]interface{}{"template": map[string]interface{}{"spec": map[string]interface{}{
			"containers": []interface{}{
				map[string]interface{}{"name": "main"},
				map[string]interface{}{"name": "sidecar"},
			},
		}}},
	}}
	assert.ErrorContains(t, validateExternalPodShape(obj, externalShapeWorkload(), externalShapeSpec()),
		"exactly one container")
}

// Workspace affinity is ANDed into every existing term so user constraints cannot be
// bypassed by an OR-appended workspace-only term.
func TestApplyExternalSchedulerAffinityAndsIntoEveryTerm(t *testing.T) {
	w := claimWorkload()
	w.Spec.Workspace = "crusoe-spur-vk"
	w.Status.ExternalExecution.PlacementMode = v1.ExternalPlacementKubeScheduler
	obj := &unstructured.Unstructured{Object: map[string]interface{}{
		"spec": map[string]interface{}{"template": map[string]interface{}{"spec": map[string]interface{}{
			"affinity": map[string]interface{}{"nodeAffinity": map[string]interface{}{
				"requiredDuringSchedulingIgnoredDuringExecution": map[string]interface{}{
					"nodeSelectorTerms": []interface{}{
						map[string]interface{}{"matchExpressions": []interface{}{
							map[string]interface{}{"key": "gpu", "operator": "Exists"},
						}},
					},
				},
			}},
		}}},
	}}
	assert.NilError(t, applyExternalSchedulerAffinity(obj, w, externalShapeSpec()))
	terms, _, err := unstructured.NestedSlice(obj.Object, "spec", "template", "spec", "affinity",
		"nodeAffinity", "requiredDuringSchedulingIgnoredDuringExecution", "nodeSelectorTerms")
	assert.NilError(t, err)
	assert.Equal(t, len(terms), 2, "current+legacy workspace keys expand each user term")
	for i, raw := range terms {
		exprs := raw.(map[string]interface{})["matchExpressions"].([]interface{})
		keys := map[string]bool{}
		for _, e := range exprs {
			keys[e.(map[string]interface{})["key"].(string)] = true
		}
		assert.Assert(t, keys["gpu"], "term %d dropped the user constraint", i)
		assert.Assert(t, keys[v1.ExternalWorkspaceLabel] || keys[v1.ExternalWorkspaceLabelLegacy],
			"term %d missing workspace constraint", i)
	}
}

// Node selector terms are ORed, so every term has to carry the pin, and a term's own
// matchFields must be kept rather than overwritten.
func TestApplyExternalNodePinCoversEveryTerm(t *testing.T) {
	userField := map[string]interface{}{"key": "metadata.name", "operator": "NotIn", "values": []interface{}{"x"}}
	obj := &unstructured.Unstructured{Object: map[string]interface{}{
		"spec": map[string]interface{}{"template": map[string]interface{}{"spec": map[string]interface{}{
			"affinity": map[string]interface{}{"nodeAffinity": map[string]interface{}{
				"requiredDuringSchedulingIgnoredDuringExecution": map[string]interface{}{
					"nodeSelectorTerms": []interface{}{
						map[string]interface{}{"matchFields": []interface{}{userField}},
						map[string]interface{}{"matchExpressions": []interface{}{
							map[string]interface{}{"key": "gpu", "operator": "Exists"},
						}},
					},
				},
			}},
		}}},
	}}
	assert.NilError(t, applyExternalNodePin(obj, externalShapeWorkload(), externalShapeSpec(), 0))
	terms, _, err := unstructured.NestedSlice(obj.Object, "spec", "template", "spec", "affinity",
		"nodeAffinity", "requiredDuringSchedulingIgnoredDuringExecution", "nodeSelectorTerms")
	assert.NilError(t, err)
	assert.Equal(t, len(terms), 2)
	for i, raw := range terms {
		fields := raw.(map[string]interface{})["matchFields"].([]interface{})
		pinned := false
		for _, f := range fields {
			m := f.(map[string]interface{})
			if m["operator"] == "In" && m["key"] == "metadata.name" {
				pinned = true
				assert.DeepEqual(t, m["values"], []interface{}{"vk-approved"})
			}
		}
		assert.Assert(t, pinned, "term %d is not pinned", i)
	}
	first := terms[0].(map[string]interface{})["matchFields"].([]interface{})
	assert.Equal(t, len(first), 2, "user matchFields must be kept")
}
