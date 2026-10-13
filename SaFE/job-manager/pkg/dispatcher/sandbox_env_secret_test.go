/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package dispatcher

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"testing"

	"gotest.tools/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	jobutils "github.com/AMD-AIG-AIMA/SAFE/job-manager/pkg/utils"
)

const (
	primusSafeChart = "../../../charts/primus-safe"
	envSecretName   = "sandbox-env"
)

// renderChartSandboxTemplate renders the chart's own sandbox template ConfigMap, so the test
// dispatches through the template a deployment actually installs.
func renderChartSandboxTemplate(t *testing.T, values ...string) string {
	t.Helper()
	if _, err := exec.LookPath("helm"); err != nil {
		t.Skip("helm is not installed")
	}
	if _, err := os.Stat(primusSafeChart); err != nil {
		t.Skipf("chart not found at %s", primusSafeChart)
	}
	args := append([]string{"template", primusSafeChart, "--namespace", common.PrimusSafeNamespace,
		"--show-only", "templates/configmap/sandbox_template.yaml"}, values...)
	var stderr bytes.Buffer
	cmd := exec.Command("helm", args...)
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	assert.NilError(t, err, stderr.String())
	return string(out)
}

// dispatchSandbox renders a Sandbox workload through generateK8sObject with the
// given template and returns the object and its main container's env.
func dispatchSandbox(t *testing.T, template string, env map[string]string, extra ...*corev1.Secret) (*v1.Workload, *unstructured.Unstructured, []interface{}) {
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
	for k, v := range env {
		workload.Spec.Env[k] = v
	}

	configmap, err := parseConfigmap(template)
	assert.NilError(t, err)
	v1.SetAnnotation(workload, v1.MainContainerAnnotation, v1.GetMainContainer(configmap))
	scheme, err := genMockScheme()
	assert.NilError(t, err)
	builder := fake.NewClientBuilder().WithObjects(configmap,
		jobutils.TestSandboxResourceTemplate, workspace).WithScheme(scheme)
	for _, s := range extra {
		s = s.DeepCopy()
		s.Namespace = workspace.Name
		builder = builder.WithObjects(s)
	}
	adminClient := builder.Build()
	r := DispatcherReconciler{Client: adminClient}
	obj, err := r.generateK8sObject(context.Background(), workload, nil)
	assert.NilError(t, err)
	return workload, obj, getEnvs(t, obj, workload, &jobutils.TestSandboxResourceTemplate.Spec.ResourceSpecs[0])
}

func envByName(envs []interface{}) map[string]map[string]interface{} {
	result := map[string]map[string]interface{}{}
	for _, item := range envs {
		entry := item.(map[string]interface{})
		result[entry["name"].(string)] = entry
	}
	return result
}

func mainContainer(t *testing.T, obj *unstructured.Unstructured) map[string]interface{} {
	t.Helper()
	containers, _, err := unstructured.NestedSlice(obj.Object, "spec", "podTemplate", "spec", "containers")
	assert.NilError(t, err)
	for _, c := range containers {
		if c.(map[string]interface{})["name"] == "codeinterpreter" {
			return c.(map[string]interface{})
		}
	}
	t.Fatal("main container not found")
	return nil
}

// TestSandboxReceivesTheEnvSecretByReference dispatches a Sandbox through the chart's template
// with sandbox.env_secret set. The main container keeps the envFrom reference to the Secret in
// its own namespace; the Secret's values appear nowhere in the Workload or the dispatched
// object; and a variable the Workload sets stays an explicit env entry, which Kubernetes gives
// precedence over envFrom.
func TestSandboxReceivesTheEnvSecretByReference(t *testing.T) {
	template := renderChartSandboxTemplate(t, "--set", "sandbox.env_secret="+envSecretName)
	const sentinel = "value-that-must-stay-in-the-secret"
	secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: envSecretName},
		Data: map[string][]byte{"ANY_NEW_NAME": []byte(sentinel), "USER_SET_TOKEN": []byte(sentinel)}}
	workload, obj, envs := dispatchSandbox(t, template, map[string]string{"USER_SET_TOKEN": "user-value"}, secret)

	assert.Equal(t, obj.GetNamespace(), workload.Spec.Workspace)
	main := mainContainer(t, obj)
	assert.DeepEqual(t, main["envFrom"], []interface{}{
		map[string]interface{}{"secretRef": map[string]interface{}{"name": envSecretName, "optional": true}},
	})
	byName := envByName(envs)
	assert.DeepEqual(t, byName["USER_SET_TOKEN"], map[string]interface{}{"name": "USER_SET_TOKEN", "value": "user-value"})
	_, listed := byName["ANY_NEW_NAME"]
	assert.Equal(t, listed, false, "a Secret key became an explicit env entry")

	for what, v := range map[string]interface{}{"workload": workload, "dispatched object": obj.Object} {
		raw, err := json.Marshal(v)
		assert.NilError(t, err)
		assert.Assert(t, !strings.Contains(string(raw), sentinel), "the Secret's value is in the %s", what)
	}
}

// TestSandboxDefaultTemplateReferencesNoSecret: without sandbox.env_secret the dispatched
// Sandbox reads nothing from a Secret.
func TestSandboxDefaultTemplateReferencesNoSecret(t *testing.T) {
	template := renderChartSandboxTemplate(t)
	assert.Assert(t, !strings.Contains(template, "envFrom"))
	_, obj, envs := dispatchSandbox(t, template, nil)
	_, hasEnvFrom := mainContainer(t, obj)["envFrom"]
	assert.Equal(t, hasEnvFrom, false)
	for name, entry := range envByName(envs) {
		if from, ok := entry["valueFrom"].(map[string]interface{}); ok {
			_, isSecret := from["secretKeyRef"]
			assert.Equal(t, isSecret, false, "%s reads a Secret", name)
		}
	}
}
