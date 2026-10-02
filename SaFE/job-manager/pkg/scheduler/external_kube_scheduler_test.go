/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package scheduler

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
)

func TestAdmitExternalViaSchedulerSingle(t *testing.T) {
	sch := runtime.NewScheme()
	_ = v1.AddToScheme(sch)

	w := &v1.Workload{
		ObjectMeta: metav1.ObjectMeta{Name: "w1", Namespace: "default", UID: types.UID("abcd1234-uuid")},
		Spec: v1.WorkloadSpec{
			Workspace: "ws-ext",
			Resources: []v1.WorkloadResource{{Replica: 1}},
		},
	}
	w.Spec.GroupVersionKind.Kind = common.AuthoringKind
	ws := &v1.Workspace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "ws-ext",
			Labels: map[string]string{
				v1.WorkspaceExternalLabel:       "true",
				v1.WorkspaceKubeSchedulerLabel:  "true",
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

func TestAdmitExternalViaSchedulerGangWaits(t *testing.T) {
	sch := runtime.NewScheme()
	_ = v1.AddToScheme(sch)

	w := &v1.Workload{
		ObjectMeta: metav1.ObjectMeta{Name: "gang", Namespace: "default", UID: types.UID("ganguid1-uuid")},
		Spec: v1.WorkloadSpec{
			Workspace: "ws-ext",
			Resources: []v1.WorkloadResource{
				{Replica: 1, GPU: "8", RdmaResource: "1"},
				{Replica: 1, GPU: "8", RdmaResource: "1"},
			},
			JobPort: 29400,
		},
	}
	w.Spec.GroupVersionKind.Kind = common.PytorchJobKind
	cli := fake.NewClientBuilder().WithScheme(sch).WithStatusSubresource(w).WithObjects(w).Build()
	r := &SchedulerReconciler{Client: cli}
	ws := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws-ext"}}

	ok, reason, err := r.admitExternalViaScheduler(context.Background(), w, ws)
	if err != nil {
		t.Fatalf("admit: %v", err)
	}
	if ok {
		t.Fatalf("gang should wait for PR")
	}
	if reason != ExternalWaitingPRReason {
		t.Fatalf("reason=%q", reason)
	}
}
