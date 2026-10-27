/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package dispatcher

import (
	"context"
	"testing"

	"github.com/spf13/viper"
	"gotest.tools/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	k8sfake "k8s.io/client-go/kubernetes/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	commonclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/k8sclient"
	"github.com/AMD-AIG-AIMA/SAFE/job-manager/pkg/syncer"
	jobutils "github.com/AMD-AIG-AIMA/SAFE/job-manager/pkg/utils"
)

func TestApplyAgentSandboxPyTorchInjectsKeyAndSessionFile(t *testing.T) {
	viper.Set("sandbox.namespace", "agent-sandbox-system")
	viper.Set("sandbox.secret", "envd-router-identity")
	viper.Set("sandbox.envd_injector_image", "example/envd-injector:test")
	t.Cleanup(func() {
		viper.Set("sandbox.namespace", "")
		viper.Set("sandbox.secret", "")
		viper.Set("sandbox.envd_injector_image", "")
	})
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "envd-router-identity", Namespace: "agent-sandbox-system"},
		Data:       map[string][]byte{"public.pem": []byte("-----BEGIN PUBLIC KEY-----\nMIIB\n-----END PUBLIC KEY-----\n")},
	}
	sets := &syncer.ClusterClientSets{}
	sets.SetClientFactory(commonclient.NewClientFactoryWithOnlyClient(
		context.Background(), "crusoe", k8sfake.NewSimpleClientset(secret)))

	workload := &v1.Workload{}
	workload.Name = "asb-sbx-1"
	workload.Spec.GroupVersionKind.Kind = "PyTorchJob"
	v1.SetLabel(workload, grantIDLabel, "gr-01")
	v1.SetLabel(workload, sandboxIDLabel, "sbx-1")
	obj := pytorchObject()
	rt := pytorchTemplate()
	err := (&DispatcherReconciler{}).applyAgentSandboxPyTorch(context.Background(), sets, obj, workload, nil, rt)
	assert.NilError(t, err)
	assert.Assert(t, workload.GetEnv(sandboxAuthPublicKeyEnvName) != "")
	assert.Equal(t, v1.GetLabel(workload, sandboxIDLabel), "sbx-1")
	assert.Equal(t, workload.Annotations[sessionIDAnnotation], "sbx-1")
	assert.NilError(t, updateMetadata(workload, obj, rt.Spec.ResourceSpecs[0], 0))

	anno, found, err := jobutils.NestedMap(obj.Object, []string{
		"spec", "pytorchReplicaSpecs", "Master", "template", "metadata", "annotations"})
	assert.NilError(t, err)
	assert.Assert(t, found)
	assert.Equal(t, anno[sessionIDAnnotation], "sbx-1")

	volumes, found, err := jobutils.NestedSlice(obj.Object, []string{
		"spec", "pytorchReplicaSpecs", "Master", "template", "spec", "volumes"})
	assert.NilError(t, err)
	assert.Assert(t, found)
	for _, item := range volumes {
		raw := item.(map[string]interface{})
		_, host := raw["hostPath"]
		assert.Assert(t, !host)
		assert.Assert(t, raw["name"] != "varlog")
	}
	containers, found, err := jobutils.NestedSlice(obj.Object, []string{
		"spec", "pytorchReplicaSpecs", "Master", "template", "spec", "containers"})
	assert.NilError(t, err)
	assert.Assert(t, found)
	container := containers[0].(map[string]interface{})
	mounts := container["volumeMounts"].([]interface{})
	sawSession := false
	for _, item := range mounts {
		mount := item.(map[string]interface{})
		assert.Assert(t, mount["name"] != "varlog")
		if mount["mountPath"] == sessionIdentityMount {
			sawSession = true
		}
	}
	assert.Assert(t, sawSession)
	assert.Equal(t, container["args"].([]interface{})[0], "mkdir -p /app && exec /shared/bin/envd --port=8080 --workspace=/app")
	inits, found, err := jobutils.NestedSlice(obj.Object, []string{
		"spec", "pytorchReplicaSpecs", "Master", "template", "spec", "initContainers"})
	assert.NilError(t, err)
	assert.Assert(t, found)
	assert.Equal(t, inits[0].(map[string]interface{})["name"], "envd-injector")
	assert.Equal(t, inits[0].(map[string]interface{})["image"], "example/envd-injector:test")
	security := container["securityContext"].(map[string]interface{})
	caps := security["capabilities"].(map[string]interface{})
	for _, item := range caps["add"].([]interface{}) {
		assert.Assert(t, item != "SYS_PTRACE")
	}
}

func TestAgentSandboxPyTorchRequiresGrant(t *testing.T) {
	plain := &v1.Workload{}
	plain.Spec.GroupVersionKind.Kind = "PyTorchJob"
	assert.Assert(t, !agentSandboxPyTorch(plain))
	named := plain.DeepCopy()
	v1.SetLabel(named, sandboxIDLabel, "sbx-1")
	assert.Assert(t, !agentSandboxPyTorch(named))
	granted := plain.DeepCopy()
	v1.SetLabel(granted, grantIDLabel, "gr-01")
	assert.Assert(t, agentSandboxPyTorch(granted))
}

func TestApplyAgentSandboxPyTorchSkipsAbsentWorker(t *testing.T) {
	viper.Set("sandbox.namespace", "agent-sandbox-system")
	viper.Set("sandbox.secret", "envd-router-identity")
	viper.Set("sandbox.envd_injector_image", "example/envd-injector:test")
	t.Cleanup(func() {
		viper.Set("sandbox.namespace", "")
		viper.Set("sandbox.secret", "")
		viper.Set("sandbox.envd_injector_image", "")
	})
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "envd-router-identity", Namespace: "agent-sandbox-system"},
		Data:       map[string][]byte{"public.pem": []byte("-----BEGIN PUBLIC KEY-----\nMIIB\n-----END PUBLIC KEY-----\n")},
	}
	sets := &syncer.ClusterClientSets{}
	sets.SetClientFactory(commonclient.NewClientFactoryWithOnlyClient(
		context.Background(), "crusoe", k8sfake.NewSimpleClientset(secret)))

	workload := &v1.Workload{}
	workload.Name = "asb-sbx-1"
	workload.Spec.GroupVersionKind.Kind = "PyTorchJob"
	v1.SetLabel(workload, grantIDLabel, "gr-01")
	v1.SetLabel(workload, sandboxIDLabel, "sbx-1")
	rt := pytorchTemplate()
	rt.Spec.ResourceSpecs = append(rt.Spec.ResourceSpecs, v1.ResourceSpec{
		PrePaths:     []string{"spec", "pytorchReplicaSpecs", "Worker"},
		PodSpecPaths: []string{"template", "spec"},
	})
	err := (&DispatcherReconciler{}).applyAgentSandboxPyTorch(context.Background(), sets, pytorchObject(), workload, nil, rt)
	assert.NilError(t, err)
}

func pytorchTemplate() *v1.ResourceTemplate {
	return &v1.ResourceTemplate{Spec: v1.ResourceTemplateSpec{ResourceSpecs: []v1.ResourceSpec{{
		PrePaths:      []string{"spec", "pytorchReplicaSpecs", "Master"},
		TemplatePaths: []string{"template"},
		PodSpecPaths:  []string{"template", "spec"},
	}}}}
}

func pytorchObject() *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"spec": map[string]interface{}{
			"pytorchReplicaSpecs": map[string]interface{}{
				"Master": map[string]interface{}{
					"template": map[string]interface{}{
						"spec": map[string]interface{}{
							"containers": []interface{}{
								map[string]interface{}{
									"name": "pytorch",
									"securityContext": map[string]interface{}{
										"capabilities": map[string]interface{}{
											"add": []interface{}{"IPC_LOCK", "SYS_PTRACE", "SYS_RESOURCE"},
										},
									},
									"volumeMounts": []interface{}{
										map[string]interface{}{"name": "varlog", "mountPath": "/var/log"},
									},
								},
							},
							"volumes": []interface{}{
								map[string]interface{}{"name": "varlog", "hostPath": map[string]interface{}{"path": "/var/log"}},
								map[string]interface{}{"name": "shared-data", "emptyDir": map[string]interface{}{}},
							},
						},
					},
				},
			},
		},
	}}
}
