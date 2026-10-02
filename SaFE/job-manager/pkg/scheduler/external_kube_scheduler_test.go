/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package scheduler

import (
	"context"
	"strings"
	"testing"

	"github.com/google/go-containerregistry/pkg/authn"
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
	"github.com/AMD-AIG-AIMA/SAFE/job-manager/pkg/imagedigest"
)

func stubImageResolve(t *testing.T) {
	t.Helper()
	prev := imagedigest.ResolveFunc
	imagedigest.ResolveFunc = func(_ context.Context, image string, _ authn.Keychain) (string, error) {
		if imagedigest.IsPinned(image) {
			return image, nil
		}
		return image + "@sha256:" + strings.Repeat("b", 64), nil
	}
	t.Cleanup(func() { imagedigest.ResolveFunc = prev })
}

func TestAdmitExternalViaSchedulerSingle(t *testing.T) {
	stubImageResolve(t)
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
	if len(stored.Status.ExternalExecution.ResolvedImages) != 1 ||
		!imagedigest.IsPinned(stored.Status.ExternalExecution.ResolvedImages[0]) {
		t.Fatalf("resolved images: %+v", stored.Status.ExternalExecution.ResolvedImages)
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
	stubImageResolve(t)
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

func TestBuildGangPodTemplate(t *testing.T) {
	w := gangWorkload()
	w.UID = types.UID("abcd1234")
	w.Spec.Workspace = "ws-ext"
	w.Spec.Images = []string{"harbor.example/app:v1"}
	w.Spec.Priority = common.HighPriorityInt
	w.Spec.Resources[0].CPU = "12"
	w.Spec.Resources[0].Memory = "64Gi"
	w.Spec.Resources[0].GPU = "8"
	pt, err := buildGangPodTemplate(w, "ws-ext", "pt-1", "pr-1")
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	spec, _, _ := unstructured.NestedMap(pt.Object, "template", "spec")
	if spec["priorityClassName"] != v1.ExternalPriorityClassHigh {
		t.Fatalf("priorityClassName=%v", spec["priorityClassName"])
	}
	tols, _, _ := unstructured.NestedSlice(spec, "tolerations")
	if len(tols) < 2 {
		t.Fatalf("want VK + booking tolerations, got %d", len(tols))
	}
}
