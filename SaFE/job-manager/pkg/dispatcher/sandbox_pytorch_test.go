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
	t.Cleanup(func() {
		viper.Set("sandbox.namespace", "")
		viper.Set("sandbox.secret", "")
	})
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "envd-router-identity", Namespace: "agent-sandbox-system"},
		Data:       map[string][]byte{"public.pem": []byte("-----BEGIN PUBLIC KEY-----\nMIIB\n-----END PUBLIC KEY-----\n")},
	}
	sets := &syncer.ClusterClientSets{}
	sets.SetClientFactory(commonclient.NewClientFactoryWithOnlyClient(
		context.Background(), "crusoe", k8sfake.NewSimpleClientset(secret)))

	workload := &v1.Workload{}
	workload.Name = "sbx-1"
	workload.Spec.GroupVersionKind.Kind = "PyTorchJob"
	v1.SetLabel(workload, sandboxNameLabel, "sbx-1")
	obj := &unstructured.Unstructured{Object: map[string]interface{}{
		"spec": map[string]interface{}{
			"pytorchReplicaSpecs": map[string]interface{}{
				"Master": map[string]interface{}{
					"template": map[string]interface{}{
						"spec": map[string]interface{}{
							"containers": []interface{}{
								map[string]interface{}{"name": "pytorch"},
							},
						},
					},
				},
			},
		},
	}}
	rt := &v1.ResourceTemplate{Spec: v1.ResourceTemplateSpec{ResourceSpecs: []v1.ResourceSpec{{
		PrePaths:     []string{"spec", "pytorchReplicaSpecs", "Master"},
		PodSpecPaths: []string{"template", "spec"},
	}}}}
	err := (&DispatcherReconciler{}).applyAgentSandboxPyTorch(context.Background(), sets, obj, workload, nil, rt)
	assert.NilError(t, err)
	assert.Assert(t, workload.GetEnv(sandboxAuthPublicKeyEnvName) != "")
	assert.Equal(t, workload.Annotations[sessionIDAnnotation], "sbx-1")

	volumes, found, err := jobutils.NestedSlice(obj.Object, []string{
		"spec", "pytorchReplicaSpecs", "Master", "template", "spec", "volumes"})
	assert.NilError(t, err)
	assert.Assert(t, found)
	assert.Equal(t, volumes[0].(map[string]interface{})["name"], sessionIdentityVolume)
	containers, found, err := jobutils.NestedSlice(obj.Object, []string{
		"spec", "pytorchReplicaSpecs", "Master", "template", "spec", "containers"})
	assert.NilError(t, err)
	assert.Assert(t, found)
	mounts := containers[0].(map[string]interface{})["volumeMounts"].([]interface{})
	assert.Equal(t, mounts[0].(map[string]interface{})["mountPath"], sessionIdentityMount)
}

func TestAgentSandboxPyTorchRequiresLabel(t *testing.T) {
	plain := &v1.Workload{}
	plain.Spec.GroupVersionKind.Kind = "PyTorchJob"
	assert.Assert(t, !agentSandboxPyTorch(plain))
	labeled := plain.DeepCopy()
	v1.SetLabel(labeled, sandboxNameLabel, "sbx-1")
	assert.Assert(t, agentSandboxPyTorch(labeled))
}
