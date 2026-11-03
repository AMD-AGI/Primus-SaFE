/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package dispatcher

import (
	"context"
	"testing"

	"gotest.tools/assert"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	jobutils "github.com/AMD-AIG-AIMA/SAFE/job-manager/pkg/utils"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/stringutil"
)

// dispatchSandboxWithParts renders a Sandbox through the chart template after mutate sets its pod parts.
func dispatchSandboxWithParts(t *testing.T, mutate func(*v1.Workload)) (*unstructured.Unstructured, error) {
	t.Helper()
	template := renderChartSandboxTemplate(t)
	commonconfig.SetValue("net.rdma_name", "rdma/hca")
	t.Cleanup(func() { commonconfig.SetValue("net.rdma_name", "") })
	workspace := jobutils.TestWorkspaceData.DeepCopy()
	workload := jobutils.TestWorkloadData.DeepCopy()
	workload.Spec.Workspace = workspace.Name
	workload.Spec.GroupVersionKind = v1.GroupVersionKind{Version: common.DefaultVersion, Kind: common.SandboxKind}
	workload.Spec.JobPort = 0
	workload.Spec.EntryPoints = nil
	workload.Spec.Resources[0].RdmaResource = ""
	mutate(workload)
	configmap, err := parseConfigmap(template)
	assert.NilError(t, err)
	v1.SetAnnotation(workload, v1.MainContainerAnnotation, v1.GetMainContainer(configmap))
	scheme, err := genMockScheme()
	assert.NilError(t, err)
	adminClient := fake.NewClientBuilder().WithObjects(configmap,
		jobutils.TestSandboxResourceTemplate, workspace).WithScheme(scheme).Build()
	r := DispatcherReconciler{Client: adminClient}
	return r.generateK8sObject(context.Background(), workload, nil)
}

func samplePodParts(w *v1.Workload) {
	w.Spec.EntryPoints = []string{stringutil.Base64Encode("a; b && exec /shared/bin/envd")}
	w.Spec.Readiness = &v1.HealthCheck{Path: "/health", Port: 8080, PeriodSeconds: 2, FailureThreshold: 30}
	w.Spec.Secrets = []v1.SecretEntity{{Id: "sbx-key", Type: v1.SecretGeneral}, {Id: "other", Type: v1.SecretGeneral}}
	w.Spec.Volumes = []v1.PodVolume{
		{Name: "envd-bin", EmptyDir: &v1.PodEmptyDir{}},
		{Name: "signing-key", EmptyDir: &v1.PodEmptyDir{Medium: "Memory", SizeLimit: "1Mi"}},
		{Name: "key-src", Secret: "sbx-key"},
		{Name: "session", DownwardAPI: []v1.PodDownwardFile{{Path: "session-id", FieldPath: "metadata.annotations['a']"}}},
	}
	w.Spec.InitContainers = []v1.PodInitContainer{{
		Name: "envd-injector", Image: "injector@sha256:1", Command: []string{"sh", "-c", "true"},
		Env:          []v1.PodEnvVar{{Name: "SIGNING_KEY_REQUIRED", Value: "1"}},
		VolumeMounts: []v1.PodVolumeMount{{Name: "envd-bin", MountPath: "/shared/bin"}, {Name: "key-src", MountPath: "/etc/key", ReadOnly: true}},
	}}
	w.Spec.VolumeMounts = []v1.PodVolumeMount{
		{Name: "envd-bin", MountPath: "/shared/bin"},
		{Name: "session", MountPath: "/var/run/agent-sandbox/session", ReadOnly: true},
	}
	w.Spec.EnvRefs = []v1.PodEnvVar{
		{Name: "NODE_NAME", FieldPath: "spec.nodeName"},
		{Name: "ENVD_AUTH_PUBLIC_KEY", ConfigMapKeyRef: &v1.PodConfigMapKey{Name: "envd-public-key", Key: "public.pem"}},
	}
}

func TestSandboxRendersPodParts(t *testing.T) {
	obj, err := dispatchSandboxWithParts(t, samplePodParts)
	assert.NilError(t, err)
	podSpec := []string{"spec", "podTemplate", "spec"}

	inits, _, _ := unstructured.NestedSlice(obj.Object, append(podSpec, "initContainers")...)
	assert.Equal(t, len(inits), 1)
	init := inits[0].(map[string]interface{})
	assert.Equal(t, init["image"], "injector@sha256:1")
	assert.Equal(t, init["terminationMessagePolicy"], "FallbackToLogsOnError")
	initPaths := mountPathCounts(init["volumeMounts"])
	assert.DeepEqual(t, initPaths, map[string]int{"/shared/bin": 1, "/etc/key": 1})

	volumes, _, _ := unstructured.NestedSlice(obj.Object, append(podSpec, "volumes")...)
	assert.Assert(t, memoryVolume(t, volumes, "signing-key"))
	assert.Assert(t, hasNamedItem(volumes, "key-src"))
	assert.Assert(t, !hasNamedItem(volumes, "sbx-key"), "a secret mounted by a workload volume got a default volume")
	assert.Assert(t, hasNamedItem(volumes, "other"))

	main := mainContainer(t, obj)
	assert.DeepEqual(t, main["command"], []interface{}{"/bin/sh", "-c", "a; b && exec /shared/bin/envd"})
	probe, _ := main["readinessProbe"].(map[string]interface{})
	assert.DeepEqual(t, probe["httpGet"], map[string]interface{}{"path": "/health", "port": int64(8080)})
	mainPaths := mountPathCounts(main["volumeMounts"])
	assert.Equal(t, mainPaths["/shared/bin"], 1)
	assert.Equal(t, mainPaths["/var/run/agent-sandbox/session"], 1)
	assert.Equal(t, mainPaths[common.SecretPath+"/sbx-key"], 0)
	assert.Equal(t, mainPaths[common.SecretPath+"/other"], 1)
	byName := envByName(main["env"].([]interface{}))
	assert.DeepEqual(t, byName["NODE_NAME"]["valueFrom"], map[string]interface{}{
		"fieldRef": map[string]interface{}{"fieldPath": "spec.nodeName"}})
	assert.DeepEqual(t, byName["ENVD_AUTH_PUBLIC_KEY"]["valueFrom"], map[string]interface{}{
		"configMapKeyRef": map[string]interface{}{"name": "envd-public-key", "key": "public.pem"}})
}

func TestSandboxPodPartsRejectTakenPaths(t *testing.T) {
	_, err := dispatchSandboxWithParts(t, func(w *v1.Workload) {
		samplePodParts(w)
		w.Spec.VolumeMounts = append(w.Spec.VolumeMounts, v1.PodVolumeMount{Name: "envd-bin", MountPath: common.SecretPath + "/other/x"})
	})
	assert.ErrorContains(t, err, "overlaps")

	_, err = dispatchSandboxWithParts(t, func(w *v1.Workload) {
		samplePodParts(w)
		w.Spec.Volumes = append(w.Spec.Volumes, v1.PodVolume{Name: "other", EmptyDir: &v1.PodEmptyDir{}})
	})
	assert.ErrorContains(t, err, "already in the pod")
}
