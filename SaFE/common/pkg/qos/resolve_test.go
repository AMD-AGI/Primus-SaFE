/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package qos

import (
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
)

func TestResolveFallsThroughEachField(t *testing.T) {
	user := &v1.User{Spec: v1.UserSpec{Qos: &v1.UserQos{
		MaxResources: &v1.ResourceCap{CPU: "32"},
		MaxRuntime:   map[string]int{string(v1.AuthoringScope): 86400},
		Clusters: map[string]v1.UserQosScope{
			"c1": {
				MaxResources: &v1.ResourceCap{GPU: "2", CPU: "32"},
				MaxRuntime:   map[string]int{string(v1.AuthoringScope): 3600},
			},
		},
	}}}
	cluster := &v1.Qos{Spec: v1.QosSpec{
		MaxResources: &v1.ResourceCap{GPU: "8", Memory: "128Gi"},
		MaxRuntime:   map[string]int{string(v1.TrainScope): 7200},
	}}
	global := &v1.Qos{Spec: v1.QosSpec{
		MaxResources: &v1.ResourceCap{CPU: "4", Memory: "64Gi", GPU: "1"},
		MaxRuntime:   map[string]int{string(v1.AuthoringScope): 600, string(v1.TrainScope): 600, string(v1.RayScope): 900},
	}}

	got := Resolve(user, cluster, global, "c1", string(v1.AuthoringScope))
	if got.GPU != "2" || got.CPU != "32" || got.Memory != "128Gi" || got.RuntimeSeconds != 3600 {
		t.Fatalf("c1 authoring: %+v", got)
	}
	got = Resolve(user, nil, global, "c2", string(v1.RayScope))
	if got.GPU != "1" || got.CPU != "4" || got.Memory != "64Gi" || got.RuntimeSeconds != 900 {
		t.Fatalf("c2 ray: %+v", got)
	}
	got = Resolve(user, nil, global, "c2", string(v1.AuthoringScope))
	if got.RuntimeSeconds != 600 {
		t.Fatalf("a user-wide timeout does not apply: %+v", got)
	}
	got = Resolve(user, cluster, global, "c1", "PyTorchJob")
	if got.RuntimeSeconds != 0 {
		t.Fatalf("a workload kind is not a timeout key: %+v", got)
	}
}

func TestQuotaReasonNamesTheExceededDimension(t *testing.T) {
	used := corev1.ResourceList{common.AmdGpu: resource.MustParse("6")}
	ask := corev1.ResourceList{common.AmdGpu: resource.MustParse("4")}
	reason, err := QuotaReason("alice", "c1", used, ask, Limits{GPU: "8"})
	if err != nil {
		t.Fatal(err)
	}
	if reason == "" || !strings.Contains(reason, "gpu") || !strings.Contains(reason, "alice") {
		t.Fatalf("reason: %q", reason)
	}
	reason, err = QuotaReason("alice", "c1", nil, corev1.ResourceList{common.AmdGpu: resource.MustParse("16")}, Limits{GPU: "8"})
	if err != nil || reason == "" {
		t.Fatalf("a request above the ceiling must pending, reason %q err %v", reason, err)
	}
	reason, err = QuotaReason("alice", "c1", used, ask, Limits{GPU: "16"})
	if err != nil || reason != "" {
		t.Fatalf("within the ceiling: %q %v", reason, err)
	}
}
