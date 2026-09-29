/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package dispatcher

import (
	"testing"

	"github.com/spf13/viper"
	"gotest.tools/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
)

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
	foundTaint := false
	for _, raw := range tolerations {
		tMap := raw.(map[string]interface{})
		if tMap["key"] == v1.ExternalVirtualKubeletTaint {
			foundTaint = true
			assert.Equal(t, tMap["operator"], "Exists")
			assert.Equal(t, tMap["effect"], "NoSchedule")
		}
	}
	assert.Assert(t, foundTaint, "missing virtual-kubelet toleration")

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
