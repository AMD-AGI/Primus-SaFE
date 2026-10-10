/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package scheduler

import (
	"context"
	"fmt"
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
)

func gangWorkload() *v1.Workload {
	w := gpuWorkload()
	w.Spec.GroupVersionKind = v1.GroupVersionKind{Kind: common.PytorchJobKind, Version: "v1"}
	w.Spec.Resources[0].RdmaResource = "1k"
	worker := w.Spec.Resources[0]
	worker.Replica = 2
	w.Spec.Resources = append(w.Spec.Resources, worker)
	w.Spec.JobPort = 23456
	return w
}

func TestAdmitExternalViaSchedulerSingle(t *testing.T) {
	sch := runtime.NewScheme()
	_ = v1.AddToScheme(sch)

	w := &v1.Workload{
		ObjectMeta: metav1.ObjectMeta{Name: "w1", Namespace: "default", UID: types.UID("abcd1234-uuid")},
		Spec: v1.WorkloadSpec{
			Workspace: "ws-ext",
			Images:    []string{"harbor.example/app:v1"},
			Resources: []v1.WorkloadResource{{Replica: 1}},
		},
	}
	w.Spec.GroupVersionKind.Kind = common.AuthoringKind
	ws := &v1.Workspace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "ws-ext",
			Labels: map[string]string{
				v1.WorkspaceExternalLabel:      "true",
				v1.WorkspaceKubeSchedulerLabel: "true",
			},
		},
	}
	cli := fake.NewClientBuilder().WithScheme(sch).WithStatusSubresource(w).WithObjects(w).Build()
	r := &SchedulerReconciler{Client: cli}

	ok, reason, err := r.admitExternalViaScheduler(context.Background(), w, ws)
	if err != nil {
		t.Fatalf("admit: %v", err)
	}
	if !ok || reason != "" {
		t.Fatalf("want admitted, got ok=%v reason=%q", ok, reason)
	}
	stored := &v1.Workload{}
	if err := cli.Get(context.Background(), client.ObjectKeyFromObject(w), stored); err != nil {
		t.Fatalf("get: %v", err)
	}
	if stored.Status.ExternalExecution == nil ||
		stored.Status.ExternalExecution.PlacementMode != v1.ExternalPlacementKubeScheduler {
		t.Fatalf("placement mode not set: %+v", stored.Status.ExternalExecution)
	}
}

func TestInterpretProvisioningRequest(t *testing.T) {
	pr := &unstructured.Unstructured{Object: map[string]interface{}{
		"status": map[string]interface{}{
			"conditions": []interface{}{
				map[string]interface{}{
					"type": "Provisioned", "status": "True", "reason": "CapacityIsProvisioned",
				},
			},
		},
	}}
	if got := interpretProvisioningRequest(pr); got.action != prAdmit {
		t.Fatalf("want admit, got %+v", got)
	}

	pr = &unstructured.Unstructured{Object: map[string]interface{}{
		"status": map[string]interface{}{
			"conditions": []interface{}{
				map[string]interface{}{
					"type": "Failed", "status": "True", "reason": "CapacityExceedsLimit", "message": "too big",
				},
			},
		},
	}}
	if got := interpretProvisioningRequest(pr); got.action != prFail ||
		!strings.HasPrefix(got.reason, ExternalPRFailedReason) {
		t.Fatalf("want fail, got %+v", got)
	}

	pr = &unstructured.Unstructured{Object: map[string]interface{}{
		"status": map[string]interface{}{
			"conditions": []interface{}{
				map[string]interface{}{
					"type": "BookingExpired", "status": "True", "reason": "CapacityReservationTimeExpired",
				},
			},
		},
	}}
	if got := interpretProvisioningRequest(pr); got.action != prRebuild {
		t.Fatalf("want rebuild, got %+v", got)
	}

	pr = &unstructured.Unstructured{Object: map[string]interface{}{
		"status": map[string]interface{}{
			"conditions": []interface{}{
				map[string]interface{}{
					"type": "Provisioned", "status": "False", "reason": "CapacityUnavailable", "message": "busy",
				},
			},
		},
	}}
	if got := interpretProvisioningRequest(pr); got.action != prWait ||
		!strings.HasPrefix(got.reason, ExternalCapacityUnavailableReason) {
		t.Fatalf("want capacity-unavailable wait, got %+v", got)
	}

	if got := interpretProvisioningRequest(&unstructured.Unstructured{Object: map[string]interface{}{}}); got.action != prWait ||
		got.reason != ExternalWaitingPRAcceptReason {
		t.Fatalf("want accept wait, got %+v", got)
	}
}

func TestEnsureExternalProvisioningProvisioned(t *testing.T) {
	sch := runtime.NewScheme()
	_ = v1.AddToScheme(sch)

	w := gangWorkload()
	w.UID = types.UID("ganguid1-uuid")
	w.Spec.Workspace = "ws-ext"
	w.Spec.Images = []string{"harbor.example/app:v1"}
	w.Spec.Resources[0].CPU = "12"
	w.Spec.Resources[0].Memory = "64Gi"
	w.Spec.Resources[0].GPU = "8"
	w.Spec.Resources[1].CPU = "12"
	w.Spec.Resources[1].Memory = "64Gi"
	w.Spec.Resources[1].GPU = "8"
	v1.SetLabel(w, v1.ClusterIdLabel, "crusoe")

	cli := fake.NewClientBuilder().WithScheme(sch).WithStatusSubresource(w).WithObjects(w).Build()
	listKinds := map[schema.GroupVersionResource]string{
		podTemplateGVR:         "PodTemplateList",
		provisioningRequestGVR: "ProvisioningRequestList",
	}
	dyn := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), listKinds)
	r := &SchedulerReconciler{Client: cli, dataPlaneDynamicOverride: dyn}
	ws := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws-ext"}}

	ok, reason, err := r.admitExternalViaScheduler(context.Background(), w, ws)
	if err != nil {
		t.Fatalf("first admit: %v", err)
	}
	if ok {
		t.Fatalf("want wait before Provisioned, got admitted reason=%q", reason)
	}

	stored := &v1.Workload{}
	if err := cli.Get(context.Background(), client.ObjectKeyFromObject(w), stored); err != nil {
		t.Fatalf("get: %v", err)
	}
	prName := stored.Status.ExternalExecution.ProvisioningRequest
	if prName == "" {
		t.Fatal("missing PR name")
	}
	got, err := dyn.Resource(provisioningRequestGVR).Namespace("ws-ext").Get(
		context.Background(), prName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get pr: %v", err)
	}
	_ = unstructured.SetNestedSlice(got.Object, []interface{}{
		map[string]interface{}{"type": "Provisioned", "status": "True", "reason": "CapacityIsProvisioned"},
	}, "status", "conditions")
	if _, err = dyn.Resource(provisioningRequestGVR).Namespace("ws-ext").Update(
		context.Background(), got, metav1.UpdateOptions{}); err != nil {
		t.Fatalf("update pr: %v", err)
	}

	ok, reason, err = r.admitExternalViaScheduler(context.Background(), stored, ws)
	if err != nil {
		t.Fatalf("second admit: %v", err)
	}
	if !ok || reason != "" {
		t.Fatalf("want admitted after Provisioned, got ok=%v reason=%q", ok, reason)
	}
}

func TestValidateExternalShapeForScheduler(t *testing.T) {
	single := &v1.Workload{Spec: v1.WorkloadSpec{Resources: []v1.WorkloadResource{{Replica: 1}}}}
	single.Spec.GroupVersionKind.Kind = common.AuthoringKind
	if err := validateExternalShapeForScheduler(single); err != nil {
		t.Fatalf("single replica: %v", err)
	}

	gang := gangWorkload()
	if err := validateExternalShapeForScheduler(gang); err != nil {
		t.Fatalf("rdma gang: %v", err)
	}

	infera := &v1.Workload{Spec: v1.WorkloadSpec{
		Resources: []v1.WorkloadResource{{Replica: 1}, {Replica: 1, GPU: "8"}, {Replica: 1, GPU: "8"}},
	}}
	infera.Spec.GroupVersionKind.Kind = common.InferaDeploymentKind
	if err := validateExternalShapeForScheduler(infera); err != nil {
		t.Fatalf("infera 1p1d: %v", err)
	}

	multi := &v1.Workload{Spec: v1.WorkloadSpec{
		Resources: []v1.WorkloadResource{{Replica: 1}, {Replica: 1}},
	}}
	multi.Spec.GroupVersionKind.Kind = common.PytorchJobKind
	if err := validateExternalShapeForScheduler(multi); err == nil {
		t.Fatal("want error for non-rdma multi replica pytorch")
	}
}

func TestBuildGangPodTemplate(t *testing.T) {
	w := gangWorkload()
	w.UID = types.UID("abcd1234")
	w.Spec.Workspace = "ws-ext"
	w.Spec.Images = []string{"harbor.example/app:v1"}
	w.Spec.Priority = common.HighPriorityInt
	timeout := 3600
	w.Spec.Timeout = &timeout
	w.Spec.CustomerLabels = map[string]string{common.SpecifiedNodes: "node-a node-b"}
	w.Spec.Resources[0].CPU = "4"
	w.Spec.Resources[0].Memory = "32Gi"
	w.Spec.Resources[0].GPU = "4"
	w.Spec.Resources[1].CPU = "12"
	w.Spec.Resources[1].Memory = "64Gi"
	w.Spec.Resources[1].GPU = "8"
	pt, err := buildGangPodTemplate(w, "ws-ext", "pt-1", "pr-1")
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	spec, _, _ := unstructured.NestedMap(pt.Object, "template", "spec")
	if spec["priorityClassName"] != v1.ExternalPriorityClassHigh {
		t.Fatalf("priorityClassName=%v", spec["priorityClassName"])
	}
	if spec["activeDeadlineSeconds"] != int64(3600) {
		t.Fatalf("activeDeadlineSeconds=%v want 3600", spec["activeDeadlineSeconds"])
	}
	tols, _, _ := unstructured.NestedSlice(spec, "tolerations")
	if len(tols) < 2 {
		t.Fatalf("want VK + booking tolerations, got %d", len(tols))
	}
	containers, _, _ := unstructured.NestedSlice(spec, "containers")
	c0, _ := containers[0].(map[string]interface{})
	reqs, _, _ := unstructured.NestedMap(c0, "resources", "requests")
	if fmt.Sprint(reqs["cpu"]) != "12" || fmt.Sprint(reqs["memory"]) != "64Gi" {
		t.Fatalf("want max of roles in booking template, got %v", reqs)
	}
	terms, _, _ := unstructured.NestedSlice(spec, "affinity", "nodeAffinity",
		"requiredDuringSchedulingIgnoredDuringExecution", "nodeSelectorTerms")
	if len(terms) != 2 {
		t.Fatalf("want current+legacy workspace terms, got %d", len(terms))
	}
	for i, raw := range terms {
		exprs := raw.(map[string]interface{})["matchExpressions"].([]interface{})
		keys := map[string]bool{}
		for _, e := range exprs {
			keys[e.(map[string]interface{})["key"].(string)] = true
		}
		if keys[v1.ExternalLeaseEndLabel] || keys[v1.ExternalLeaseEndLabelLegacy] {
			t.Fatalf("term %d must not carry lease-end", i)
		}
		if !keys[v1.K8sHostName] {
			t.Fatalf("term %d missing specified_nodes hostname", i)
		}
		if !(keys[v1.ExternalWorkspaceLabel] || keys[v1.ExternalWorkspaceLabelLegacy]) {
			t.Fatalf("term %d missing workspace", i)
		}
	}
}

func TestExternalObjectKeyStableAndDistinct(t *testing.T) {
	a := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "w1", UID: "11111111-1111-1111-1111-111111111111"}}
	b := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "w2", UID: "11111111-1111-1111-1111-111111111111"}}
	if externalObjectKey(a) == externalObjectKey(b) {
		t.Fatal("same UID different name must not collide")
	}
	if externalObjectKey(a) != externalObjectKey(a) {
		t.Fatal("key must be stable")
	}
}
