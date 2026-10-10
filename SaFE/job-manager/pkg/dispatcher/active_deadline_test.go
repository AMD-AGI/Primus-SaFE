/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package dispatcher

import (
	"context"
	"testing"
	"time"

	"gotest.tools/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	jobutils "github.com/AMD-AIG-AIMA/SAFE/job-manager/pkg/utils"
)

// activeDeadlineCase renders a workload of one kind through generateK8sObject
// and returns the pod spec activeDeadlineSeconds of its first resource spec.
type activeDeadlineCase struct {
	kind     string
	template string
	rt       *v1.ResourceTemplate
}

var activeDeadlineCases = map[string]activeDeadlineCase{
	common.PytorchJobKind: {common.PytorchJobKind, TestPytorchJobTemplateConfig, jobutils.TestPytorchResourceTemplate},
	common.JobKind:        {common.JobKind, TestJobTemplateConfig, jobutils.TestJobResourceTemplate},
	common.SandboxKind:    {common.SandboxKind, TestSandboxConfig, jobutils.TestSandboxResourceTemplate},
	common.DeploymentKind: {common.DeploymentKind, TestDeploymentTemplateConfig, jobutils.TestDeploymentResourceTemplate},
}

func renderActiveDeadline(t *testing.T, c activeDeadlineCase, mutate func(*v1.Workload)) (int64, bool) {
	t.Helper()
	commonconfig.SetValue("net.rdma_name", "rdma/hca")
	defer commonconfig.SetValue("net.rdma_name", "")
	workspace := jobutils.TestWorkspaceData.DeepCopy()
	workload := jobutils.TestWorkloadData.DeepCopy()
	workload.Spec.Workspace = workspace.Name
	workload.Spec.GroupVersionKind = v1.GroupVersionKind{Version: "v1", Kind: c.kind}
	if c.kind == common.DeploymentKind {
		workload.Spec.GroupVersionKind.Group = "apps"
	}
	if c.kind == common.SandboxKind {
		workload.Spec.GroupVersionKind.Version = common.DefaultVersion
		workload.Spec.JobPort = 0
		workload.Spec.EntryPoints = nil
		workload.Spec.Resources[0].RdmaResource = ""
		workload.Spec.Env[sandboxAuthPublicKeyEnvName] = "test-public-key"
	}
	if mutate != nil {
		mutate(workload)
	}
	configmap, err := parseConfigmap(c.template)
	assert.NilError(t, err)
	metav1.SetMetaDataAnnotation(&workload.ObjectMeta, v1.MainContainerAnnotation, v1.GetMainContainer(configmap))
	scheme, err := genMockScheme()
	assert.NilError(t, err)
	adminClient := fake.NewClientBuilder().WithObjects(configmap, c.rt, workspace).WithScheme(scheme).Build()
	r := DispatcherReconciler{Client: adminClient}
	obj, err := r.generateK8sObject(context.Background(), workload, nil)
	assert.NilError(t, err)
	path := podSpecPath(workload, &c.rt.Spec.ResourceSpecs[0], "activeDeadlineSeconds")
	v, found, err := jobutils.NestedInt64(obj.Object, path)
	assert.NilError(t, err)
	return v, found
}

func TestActiveDeadlineFromTimeoutPerKind(t *testing.T) {
	tests := []struct {
		kind string
		want bool
	}{
		{common.PytorchJobKind, true},
		{common.JobKind, true},
		{common.SandboxKind, true},
		// A Deployment's pods are a long-running service, and the API server
		// refuses activeDeadlineSeconds in its pod template.
		{common.DeploymentKind, false},
	}
	for _, tt := range tests {
		t.Run(tt.kind, func(t *testing.T) {
			v, found := renderActiveDeadline(t, activeDeadlineCases[tt.kind], func(w *v1.Workload) {
				w.Spec.Timeout = pointer.Int(180000)
			})
			assert.Equal(t, found, tt.want)
			if tt.want {
				assert.Equal(t, v, int64(180000))
			}
		})
	}
}

func TestActiveDeadlineAbsentWithoutTimeout(t *testing.T) {
	for _, kind := range []string{common.PytorchJobKind, common.JobKind, common.SandboxKind} {
		t.Run(kind, func(t *testing.T) {
			for _, timeout := range []*int{nil, pointer.Int(0)} {
				_, found := renderActiveDeadline(t, activeDeadlineCases[kind], func(w *v1.Workload) {
					w.Spec.Timeout = timeout
				})
				assert.Equal(t, found, false)
			}
		})
	}
}

// A redispatched workload keeps the timeout as written: the workload timeout is
// fixed and counted from the first start, so the pod value stays an upper bound.
func TestActiveDeadlineOnFailoverKeepsTheTimeout(t *testing.T) {
	v, found := renderActiveDeadline(t, activeDeadlineCases[common.PytorchJobKind], func(w *v1.Workload) {
		w.Spec.Timeout = pointer.Int(86400)
		v1.SetLabel(w, v1.WorkloadDispatchCntLabel, "3")
		w.Status.StartTime = &metav1.Time{Time: time.Now().UTC().Add(-10 * time.Hour)}
	})
	assert.Equal(t, found, true)
	assert.Equal(t, v, int64(86400))
}

func TestModifyActiveDeadlineKinds(t *testing.T) {
	path := []string{"spec", "template", "spec", "activeDeadlineSeconds"}
	tests := []struct {
		kind string
		want bool
	}{
		{common.AuthoringKind, true},
		{common.PytorchJobKind, true},
		{common.UnifiedJobKind, true},
		{common.TorchFTKind, true},
		{common.JobKind, true},
		{common.MonarchClient, true},
		{common.SandboxKind, true},
		{common.DeploymentKind, false},
		{common.StatefulSetKind, false},
		{common.CICDGithubRunnerKind, false},
		{common.CICDScaleRunnerSetKind, false},
		{common.CICDEphemeralRunnerKind, false},
		{common.DynamoDeploymentKind, false},
		{common.InferaDeploymentKind, false},
		{common.MonarchMesh, false},
		{common.RayJobKind, false},
	}
	for _, tt := range tests {
		t.Run(tt.kind, func(t *testing.T) {
			w := &v1.Workload{}
			w.Spec.GroupVersionKind = v1.GroupVersionKind{Version: "v1", Kind: tt.kind}
			w.Spec.Timeout = pointer.Int(3600)
			obj := &unstructured.Unstructured{Object: map[string]interface{}{}}
			assert.NilError(t, modifyActiveDeadline(obj, w, path))
			v, found, err := jobutils.NestedInt64(obj.Object, path)
			assert.NilError(t, err)
			assert.Equal(t, found, tt.want)
			if tt.want {
				assert.Equal(t, v, int64(3600))
			}
		})
	}
}

func TestModifyActiveDeadlineExistingValue(t *testing.T) {
	path := []string{"spec", "template", "spec", "activeDeadlineSeconds"}
	tests := []struct {
		name     string
		existing interface{}
		want     int64
	}{
		{"smaller int64 is kept", int64(600), 600},
		{"smaller float from yaml is kept", float64(600), 600},
		{"equal is kept", int64(3600), 3600},
		{"larger is lowered to the timeout", int64(7200), 3600},
		{"larger float from yaml is lowered", float64(7200), 3600},
		{"zero is replaced", int64(0), 3600},
		{"non-number is replaced", "600", 3600},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := &v1.Workload{}
			w.Spec.GroupVersionKind = v1.GroupVersionKind{Version: "v1", Kind: common.JobKind}
			w.Spec.Timeout = pointer.Int(3600)
			obj := &unstructured.Unstructured{Object: map[string]interface{}{}}
			assert.NilError(t, jobutils.SetNestedField(obj.Object, tt.existing, path))
			assert.NilError(t, modifyActiveDeadline(obj, w, path))
			got, _, _ := jobutils.NestedField(obj.Object, path)
			n, ok := toPositiveInt64(got)
			assert.Equal(t, ok, true)
			assert.Equal(t, n, tt.want)
		})
	}
}

// Without a timeout a value set by the template is left alone.
func TestModifyActiveDeadlineNoTimeoutKeepsTemplateValue(t *testing.T) {
	path := []string{"spec", "template", "spec", "activeDeadlineSeconds"}
	w := &v1.Workload{}
	w.Spec.GroupVersionKind = v1.GroupVersionKind{Version: "v1", Kind: common.JobKind}
	obj := &unstructured.Unstructured{Object: map[string]interface{}{}}
	assert.NilError(t, jobutils.SetNestedField(obj.Object, int64(600), path))
	assert.NilError(t, modifyActiveDeadline(obj, w, path))
	v, _, _ := jobutils.NestedInt64(obj.Object, path)
	assert.Equal(t, v, int64(600))
}
