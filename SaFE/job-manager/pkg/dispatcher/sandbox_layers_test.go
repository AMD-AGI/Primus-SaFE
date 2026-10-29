/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package dispatcher

import (
	"context"
	"os/exec"
	"strings"
	"testing"

	"gotest.tools/assert"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	jobutils "github.com/AMD-AIG-AIMA/SAFE/job-manager/pkg/utils"
)

const testLayerImage = "registry.example/layer@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

// renderAgentSandbox dispatches a grant-labelled Sandbox through the chart template.
func renderAgentSandbox(t *testing.T, mutate func(*v1.Workload)) (*unstructured.Unstructured, error) {
	t.Helper()
	commonconfig.SetValue("net.rdma_name", "rdma/hca")
	t.Cleanup(func() { commonconfig.SetValue("net.rdma_name", "") })
	workspace := jobutils.TestWorkspaceData.DeepCopy()
	workload := jobutils.TestWorkloadData.DeepCopy()
	workload.Spec.Workspace = workspace.Name
	workload.Spec.GroupVersionKind = v1.GroupVersionKind{Version: common.DefaultVersion, Kind: common.SandboxKind}
	workload.Spec.JobPort = 0
	workload.Spec.EntryPoints = nil
	workload.Spec.Resources[0].RdmaResource = ""
	workload.Spec.Env[sandboxAuthPublicKeyEnvName] = "test-public-key"
	v1.SetLabel(workload, grantIDLabel, "gr-01")
	mutate(workload)

	configmap, err := parseConfigmap(renderChartSandboxTemplate(t))
	assert.NilError(t, err)
	v1.SetAnnotation(workload, v1.MainContainerAnnotation, v1.GetMainContainer(configmap))
	scheme, err := genMockScheme()
	assert.NilError(t, err)
	adminClient := fake.NewClientBuilder().WithObjects(configmap,
		jobutils.TestSandboxResourceTemplate, workspace).WithScheme(scheme).Build()
	r := DispatcherReconciler{Client: adminClient}
	return r.generateK8sObject(context.Background(), workload, nil)
}

func podItems(t *testing.T, obj *unstructured.Unstructured, field string) map[string]map[string]interface{} {
	t.Helper()
	items, _, err := unstructured.NestedSlice(obj.Object, "spec", "podTemplate", "spec", field)
	assert.NilError(t, err)
	out := map[string]map[string]interface{}{}
	for _, item := range items {
		raw := item.(map[string]interface{})
		out[raw["name"].(string)] = raw
	}
	return out
}

func TestSandboxLayersRenderedFromAnnotation(t *testing.T) {
	obj, err := renderAgentSandbox(t, func(w *v1.Workload) {
		v1.SetAnnotation(w, sandboxLayersAnnotation,
			`[{"image":"`+testLayerImage+`","from":"/layer","mount":"/opt/host","readOnly":true}]`)
		w.Spec.Env[signingKeyEnv] = "/etc/secrets/" + w.Name + "/signing-key.pem"
		w.Spec.Secrets = []v1.SecretEntity{{Id: w.Name, Type: v1.SecretGeneral}}
	})
	assert.NilError(t, err)

	inits, _, _ := unstructured.NestedSlice(obj.Object, "spec", "podTemplate", "spec", "initContainers")
	assert.Equal(t, len(inits), 2)
	assert.Equal(t, inits[0].(map[string]interface{})["name"], envdInjectorName)
	layer := inits[1].(map[string]interface{})
	assert.Equal(t, layer["name"], "as-layer-0")
	assert.Equal(t, layer["image"], testLayerImage)
	assert.DeepEqual(t, layer["command"], []interface{}{"sh", "-c", "set -e; cp -a '/layer/.' '/opt/host/'"})
	assert.DeepEqual(t, layer["volumeMounts"], []interface{}{map[string]interface{}{"name": "as-layer-0", "mountPath": "/opt/host"}})
	assert.DeepEqual(t, layer["securityContext"], inits[0].(map[string]interface{})["securityContext"])

	injector := envByName(inits[0].(map[string]interface{})["env"].([]interface{}))
	assert.Equal(t, injector[signingKeyRequiredEnv]["value"], "1")

	_, ok := podItems(t, obj, "volumes")["as-layer-0"]["emptyDir"]
	assert.Assert(t, ok)
	found := false
	for _, m := range mainContainer(t, obj)["volumeMounts"].([]interface{}) {
		raw := m.(map[string]interface{})
		if raw["name"] == "as-layer-0" {
			found = raw["mountPath"] == "/opt/host" && raw["readOnly"] == true
		}
	}
	assert.Assert(t, found)
}

func TestSandboxLayersBadAnnotationFailsWorkload(t *testing.T) {
	for name, value := range map[string]string{
		"not json": `{`,
		"tag":      `[{"image":"registry.example/layer:v1","from":"/layer","mount":"/opt/host"}]`,
		"reserved": `[{"image":"` + testLayerImage + `","from":"/layer","mount":"/run/signing-key"}]`,
		"shell":    `[{"image":"` + testLayerImage + `","from":"/layer'; id; '","mount":"/opt/host"}]`,
	} {
		t.Run(name, func(t *testing.T) {
			_, err := renderAgentSandbox(t, func(w *v1.Workload) {
				v1.SetAnnotation(w, sandboxLayersAnnotation, value)
			})
			assert.Assert(t, jobutils.IsUnrecoverableError(err), "err = %v", err)
		})
	}
}

func TestSandboxWithoutSigningKeyEnvDoesNotRequireKey(t *testing.T) {
	obj, err := renderAgentSandbox(t, func(*v1.Workload) {})
	assert.NilError(t, err)
	env, _ := podItems(t, obj, "initContainers")[envdInjectorName]["env"].([]interface{})
	_, required := envByName(env)[signingKeyRequiredEnv]
	assert.Assert(t, !required)
}

func TestEnvdInjectorFailsWhenRequiredKeyIsMissing(t *testing.T) {
	i := strings.Index(envdInjectorScript, `if [ -n "${SIGNING_KEY_REQUIRED-}" ]`)
	assert.Assert(t, i > 0)
	check := envdInjectorScript[i:]
	cmd := exec.Command("sh", "-c", check)
	cmd.Env = []string{"SIGNING_KEY_REQUIRED=1"}
	out, err := cmd.CombinedOutput()
	assert.Assert(t, err != nil)
	assert.Assert(t, strings.Contains(string(out), "signing key secret is not mounted"))

	cmd = exec.Command("sh", "-c", check)
	cmd.Env = []string{}
	assert.NilError(t, cmd.Run())
}
