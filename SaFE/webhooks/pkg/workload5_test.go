/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package webhooks

import (
	"context"
	"testing"

	"gotest.tools/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

// TestWorkloadValidateCommonStepErrors covers each downstream validation return-error branch.
func TestWorkloadValidateCommonStepErrors(t *testing.T) {
	ctx := context.Background()
	c := fullWorkloadEnvClient(t)
	v := &WorkloadValidator{Client: c}

	// service error
	svc := fullValidWorkload()
	svc.Spec.Service = &v1.Service{Port: 0, TargetPort: 80, Protocol: corev1.ProtocolTCP, ServiceType: corev1.ServiceTypeClusterIP}
	assert.Assert(t, v.validateCommon(ctx, svc, nil) != nil)

	// health check error
	hc := fullValidWorkload()
	hc.Spec.Liveness = &v1.HealthCheck{Port: 80}
	assert.Assert(t, v.validateCommon(ctx, hc, nil) != nil)

	// resource exceeds flavor
	res := fullValidWorkload()
	res.Spec.Resources = []v1.WorkloadResource{{Replica: 1, CPU: "100", Memory: "2Gi", EphemeralStorage: "3Gi"}}
	assert.Assert(t, v.validateCommon(ctx, res, nil) != nil)

	// labels error
	lbl := fullValidWorkload()
	lbl.Spec.CustomerLabels = map[string]string{"Bad Key": "v"}
	assert.Assert(t, v.validateCommon(ctx, lbl, nil) != nil)

	// owner workload missing
	owner := fullValidWorkload()
	v1.SetLabel(owner, v1.OwnerLabel, "missing")
	assert.Assert(t, v.validateCommon(ctx, owner, nil) != nil)
}

// TestWorkloadValidateCommonTemplateMissing covers the template-not-found branch.
func TestWorkloadValidateCommonTemplateMissing(t *testing.T) {
	scheme := newScheme(t)
	ws := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}, Spec: v1.WorkspaceSpec{Replica: 100}}
	flavor := gpuFlavor("flavor1")
	flavor.Spec.Gpu = nil
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ws, flavor).Build()
	v := &WorkloadValidator{Client: c}
	w := fullValidWorkload()
	assert.Assert(t, v.validateCommon(context.Background(), w, nil) != nil)
}

// TestWorkloadValidateOnCreationCronJobs covers the cron job validation branch.
func TestWorkloadValidateOnCreationCronJobs(t *testing.T) {
	c := fullWorkloadEnvClient(t)
	v := &WorkloadValidator{Client: c}
	w := fullValidWorkload()
	w.Spec.CronJobs = []v1.CronJob{{Schedule: "", Action: v1.CronStart}}
	assert.Assert(t, v.validateOnCreation(context.Background(), w) != nil)
}

// TestWorkloadValidateOnUpdateBranches covers immutable and spec-change update branches.
func TestWorkloadValidateOnUpdateBranches(t *testing.T) {
	c := fullWorkloadEnvClient(t)
	v := &WorkloadValidator{Client: c}

	// immutable workspace change
	oldW := fullValidWorkload()
	newW := fullValidWorkload()
	newW.Spec.Workspace = "other"
	assert.Assert(t, v.validateOnUpdate(context.Background(), newW, oldW) != nil)

	// spec changed on dispatched workload
	oldD := fullValidWorkload()
	v1.SetAnnotation(oldD, v1.WorkloadDispatchedAnnotation, v1.TrueStr)
	newD := fullValidWorkload()
	v1.SetAnnotation(newD, v1.WorkloadDispatchedAnnotation, v1.TrueStr)
	newD.Spec.Resources = []v1.WorkloadResource{{Replica: 2, CPU: "2", Memory: "4Gi", EphemeralStorage: "5Gi"}}
	assert.Assert(t, v.validateOnUpdate(context.Background(), newD, oldD) != nil)
}

// TestWorkloadValidateOnUpdateCronJobs covers cron change validation on update.
func TestWorkloadValidateOnUpdateCronJobs(t *testing.T) {
	c := fullWorkloadEnvClient(t)
	v := &WorkloadValidator{Client: c}
	oldW := fullValidWorkload()
	newW := fullValidWorkload()
	newW.Spec.CronJobs = []v1.CronJob{{Schedule: "", Action: v1.CronStart}}
	assert.Assert(t, v.validateOnUpdate(context.Background(), newW, oldW) != nil)
}

// TestWorkloadValidateWorkspaceQuotaOk covers the quota sub-resource success branch.
func TestWorkloadValidateWorkspaceQuotaOk(t *testing.T) {
	scheme := newScheme(t)
	ws := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}, Spec: v1.WorkspaceSpec{Replica: 0}}
	ws.Status.TotalResources = corev1.ResourceList{
		corev1.ResourceCPU:              resource.MustParse("100"),
		corev1.ResourceMemory:           resource.MustParse("1000Gi"),
		corev1.ResourceEphemeralStorage: resource.MustParse("1000Gi"),
	}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ws).Build()
	v := &WorkloadValidator{Client: c}
	w := validWorkload()
	w.Spec.Resources = []v1.WorkloadResource{{Replica: 1, CPU: "1", Memory: "2Gi", EphemeralStorage: "3Gi"}}
	assert.NilError(t, v.validateWorkspace(context.Background(), w))
}
