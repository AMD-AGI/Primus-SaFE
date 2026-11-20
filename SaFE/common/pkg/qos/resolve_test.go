/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package qos

import (
	"context"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

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

	got := Resolve(user, cluster, "c1", string(v1.AuthoringScope))
	if got.GPU != "2" || got.CPU != "32" || got.Memory != "128Gi" || got.RuntimeSeconds != 3600 {
		t.Fatalf("c1 authoring: %+v", got)
	}
	got = Resolve(user, cluster, "c1", string(v1.TrainScope))
	if got.RuntimeSeconds != 7200 {
		t.Fatalf("c1 train falls back to the cluster: %+v", got)
	}
	got = Resolve(user, nil, "c2", string(v1.AuthoringScope))
	if got != (Limits{}) {
		t.Fatalf("a user-wide value does not apply: %+v", got)
	}
	got = Resolve(user, cluster, "c1", "PyTorchJob")
	if got.RuntimeSeconds != 0 {
		t.Fatalf("a workload kind is not a timeout key: %+v", got)
	}
}

func TestCeilingUsesOnlyTheClusterQos(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := v1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	stale := &v1.Qos{
		ObjectMeta: metav1.ObjectMeta{Name: "default"},
		Spec: v1.QosSpec{
			MaxResources: &v1.ResourceCap{GPU: "1"},
			MaxRuntime:   map[string]int{string(v1.TrainScope): 600},
		},
	}
	cluster := &v1.Qos{
		ObjectMeta: metav1.ObjectMeta{Name: "c1"},
		Spec:       v1.QosSpec{MaxResources: &v1.ResourceCap{GPU: "8"}},
	}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(stale, cluster).Build()

	got, err := Ceiling(context.Background(), c, "", "c2", string(v1.TrainScope))
	if err != nil || got != (Limits{}) {
		t.Fatalf("a cluster without qos has no ceiling: %+v %v", got, err)
	}
	got, err = Ceiling(context.Background(), c, "", "c1", string(v1.TrainScope))
	if err != nil || got != (Limits{GPU: "8"}) {
		t.Fatalf("c1 uses its own qos only: %+v %v", got, err)
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

func TestIsUserQuotaReasonAndResourceGrew(t *testing.T) {
	if !IsUserQuotaReason(UserQuotaReasonPrefix+": gpu") || !IsUserQuotaReason(UserQuotaCheckFailedPrefix+": bad") {
		t.Fatal("quota reasons must be recognized")
	}
	if IsUserQuotaReason("insufficient resources") {
		t.Fatal("a capacity reason is not a user quota reason")
	}
	prev := corev1.ResourceList{
		corev1.ResourceCPU:    resource.MustParse("1"),
		corev1.ResourceMemory: resource.MustParse("1Gi"),
		common.AmdGpu:         resource.MustParse("1"),
	}
	cpu, memory, gpu := ResourceGrew(nil, prev)
	if !cpu || !memory || !gpu {
		t.Fatalf("growing from empty: %v %v %v", cpu, memory, gpu)
	}
	same := prev.DeepCopy()
	cpu, memory, gpu = ResourceGrew(prev, same)
	if cpu || memory || gpu {
		t.Fatalf("an unchanged list does not grow: %v %v %v", cpu, memory, gpu)
	}
	_, err := Exceeded("alice", "c1", nil, nil, Limits{CPU: "nope"}, true, false, false)
	if err == nil {
		t.Fatal("a limit that is not a quantity must fail")
	}
}

func TestCeilingIgnoresAMissingUser(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := v1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	cluster := &v1.Qos{
		ObjectMeta: metav1.ObjectMeta{Name: "c1"},
		Spec:       v1.QosSpec{MaxResources: &v1.ResourceCap{CPU: "4"}},
	}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cluster).Build()
	got, err := Ceiling(context.Background(), c, "missing-user", "c1", string(v1.TrainScope))
	if err != nil || got.CPU != "4" {
		t.Fatalf("a missing user leaves the cluster ceiling: %+v %v", got, err)
	}
	got, err = Ceiling(context.Background(), c, "", "", string(v1.TrainScope))
	if err != nil || got != (Limits{}) {
		t.Fatalf("no cluster has no ceiling: %+v %v", got, err)
	}
}
