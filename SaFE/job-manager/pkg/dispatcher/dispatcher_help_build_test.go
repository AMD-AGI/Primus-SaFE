/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package dispatcher

import (
	"testing"

	"gotest.tools/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

func TestEntrypointsEqual(t *testing.T) {
	assert.Equal(t, entrypointsEqual("same", "same"), true)
	assert.Equal(t, entrypointsEqual("a", "b"), false)
}

func TestBuildObjectLabels(t *testing.T) {
	w := &v1.Workload{ObjectMeta: metav1.ObjectMeta{
		Name:   "w",
		Labels: map[string]string{"team": "x", v1.PrimusSafePrefix + "internal": "skip"},
	}}
	labels := buildObjectLabels(w)
	// User labels are carried, SaFE-internal labels are filtered out.
	assert.Equal(t, labels["team"], "x")
	_, hasInternal := labels[v1.PrimusSafePrefix+"internal"]
	assert.Equal(t, hasInternal, false)
}

func TestBuildObjectAnnotations(t *testing.T) {
	w := &v1.Workload{ObjectMeta: metav1.ObjectMeta{
		Name:        "w",
		Annotations: map[string]string{"note": "hello", v1.PrimusSafePrefix + "x": "skip"},
	}}
	ann := buildObjectAnnotations(w)
	assert.Equal(t, ann["note"], "hello")
	_, hasInternal := ann[v1.PrimusSafePrefix+"x"]
	assert.Equal(t, hasInternal, false)
}

func TestBuildPodLabels(t *testing.T) {
	w := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "w"}}
	labels := buildPodLabels(w)
	assert.Equal(t, labels[v1.K8sObjectIdLabel], "w")
}

func TestBuildPodAnnotations(t *testing.T) {
	w := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "w"}}
	ann := buildPodAnnotations(w, 0)
	assert.Equal(t, ann[v1.ResourceIdAnnotation], "0")
}

func TestBuildEnvironmentBasic(t *testing.T) {
	w := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "w"}}
	w.Spec.Workspace = "ws"
	envs := buildEnvironment(w, nil, -1)
	// Core env vars are always present (WORKLOAD_ID, WORKSPACE, etc).
	assert.Assert(t, len(envs) > 0)
}

func TestBuildEnvironmentGpuAndSupervised(t *testing.T) {
	w := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "w"}}
	w.Spec.Workspace = "ws"
	w.Spec.IsSupervised = true
	w.Spec.Resources = []v1.WorkloadResource{{Replica: 1, GPU: "8", GPUName: "amd.com/gpu"}}
	envs := buildEnvironment(w, nil, 0)
	// GPU + supervised paths add more env vars than the basic case.
	assert.Assert(t, len(envs) > 5)
}
