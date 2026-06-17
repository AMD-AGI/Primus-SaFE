/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package dispatcher

import (
	"testing"

	"gotest.tools/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

func TestGenerateRandomPort(t *testing.T) {
	ports := map[int]struct{}{}
	p := generateRandomPort(ports)
	assert.Assert(t, p >= 20000 && p < 30000)
	// The chosen port is recorded to avoid reuse.
	_, ok := ports[p]
	assert.Equal(t, ok, true)

	// A second call yields a port also recorded in the set.
	p2 := generateRandomPort(ports)
	assert.Assert(t, p2 >= 20000 && p2 < 30000)
}

func TestGenerateMeshNamePrefix(t *testing.T) {
	assert.Equal(t, generateMeshNamePrefix("my-job_name"), "myjobnamemesh")
	assert.Equal(t, generateMeshNamePrefix("abc"), "abcmesh")
}

func TestGenerateServicePorts(t *testing.T) {
	svc := &v1.Service{Protocol: corev1.ProtocolTCP, Port: 8080, TargetPort: 9090}
	ports := generateServicePorts(svc)
	assert.Equal(t, len(ports), 1)
	assert.Equal(t, ports[0].Port, int32(8080))
	assert.Equal(t, ports[0].TargetPort.IntVal, int32(9090))
	assert.Equal(t, string(ports[0].Protocol), string(corev1.ProtocolTCP))
}

func TestShouldDispatch(t *testing.T) {
	// Scheduled but not dispatched -> true.
	w := &v1.Workload{ObjectMeta: metav1.ObjectMeta{
		Annotations: map[string]string{v1.WorkloadScheduledAnnotation: "true"},
	}}
	assert.Equal(t, shouldDispatch(w), true)

	// Already dispatched -> false.
	w.Annotations[v1.WorkloadDispatchedAnnotation] = "true"
	assert.Equal(t, shouldDispatch(w), false)

	// Not scheduled -> false.
	w2 := &v1.Workload{}
	assert.Equal(t, shouldDispatch(w2), false)
}

func TestBuildServiceSelectorDefault(t *testing.T) {
	w := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "wl-1"}}
	svc := &v1.Service{ExtraSelectors: map[string]string{
		"role":              "head",
		v1.K8sObjectIdLabel: "should-be-overridden",
	}}
	sel := buildServiceSelector(w, svc)
	// SaFE-managed key wins and equals the workload name.
	assert.Equal(t, sel[v1.K8sObjectIdLabel], "wl-1")
	// User-supplied non-colliding key is preserved.
	assert.Equal(t, sel["role"], "head")
}
