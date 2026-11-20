/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package view

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

func TestValidateUserQosAcceptsWorkspacesOnly(t *testing.T) {
	workspaces := map[string]v1.UserQosScope{"ws1": {MaxResources: &v1.ResourceCap{GPU: "8"}}}
	if err := ValidateUserQos(&v1.UserQos{Workspaces: workspaces}); err != nil {
		t.Fatalf("per-workspace values are accepted: %v", err)
	}
	if err := ValidateUserQos(&v1.UserQos{MaxResources: &v1.ResourceCap{GPU: "8"}, Workspaces: workspaces}); err == nil {
		t.Fatal("top-level maxResources must be rejected")
	}
	if err := ValidateUserQos(&v1.UserQos{MaxRuntime: map[string]int{string(v1.TrainScope): 3600}}); err == nil {
		t.Fatal("top-level maxRuntime must be rejected")
	}
	if err := ValidateUserQos(&v1.UserQos{Workspaces: map[string]v1.UserQosScope{"": {}}}); err == nil {
		t.Fatal("an empty workspace id must be rejected")
	}
	if err := ValidateUserQos(&v1.UserQos{Workspaces: map[string]v1.UserQosScope{
		"ws1": {MaxRuntime: map[string]int{"Nope": 1}},
	}}); err == nil {
		t.Fatal("an unknown scope must be rejected")
	}
	if err := ValidateUserQos(nil); err != nil {
		t.Fatal(err)
	}
}

func TestQosResponseAndNormalization(t *testing.T) {
	empty := NewQosResponse(nil)
	if empty.Name != "" || empty.MaxResources != nil || empty.MaxRuntime != nil {
		t.Fatal("a nil qos has an empty response")
	}
	qos := &v1.Qos{
		ObjectMeta: metav1.ObjectMeta{Name: "c1"},
		Spec:       v1.QosSpec{MaxResources: &v1.ResourceCap{CPU: " 4 ", Memory: "512", GPU: " 1 "}},
	}
	got := NewQosResponse(qos)
	if got.Name != "c1" || got.MaxResources.CPU != " 4 " {
		t.Fatalf("response: %+v", got)
	}
	normalized := NormalizeQosResources(qos.Spec.MaxResources)
	if normalized == nil || normalized.CPU != "4" || normalized.Memory != "512Gi" || normalized.GPU != "1" {
		t.Fatalf("normalized: %+v", normalized)
	}
	if NormalizeQosResources(nil) != nil {
		t.Fatal("nil stays nil")
	}
	if NormalizeQosResources(&v1.ResourceCap{}) != nil {
		t.Fatal("an empty cap is cleared")
	}
	kept := NormalizeQosResources(&v1.ResourceCap{Memory: "512Gi"})
	if kept == nil || kept.Memory != "512Gi" {
		t.Fatalf("a unit is kept: %+v", kept)
	}
	if err := ValidateQosLimits(&v1.ResourceCap{CPU: "nope"}, nil); err == nil {
		t.Fatal("a bad quantity must be rejected")
	}
}
