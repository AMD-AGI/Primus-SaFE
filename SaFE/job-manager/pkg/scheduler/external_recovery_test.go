/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package scheduler

import (
	"context"
	"errors"
	"strings"
	"testing"

	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

func TestExternalRetryDelay(t *testing.T) {
	if d, ok := externalRetryDelay(ExternalCapacityReason, nil); !ok || d != externalWaitRetry {
		t.Fatalf("plain wait = %v %v", d, ok)
	}
	if d, ok := externalRetryDelay(ExternalImageReason, errors.New("boom")); !ok || d != externalExchangeRetry {
		t.Fatalf("error without Retry-After = %v %v", d, ok)
	}
	for _, reason := range []string{ExternalUnsupportedReason, ExternalConstraintReason} {
		if _, ok := externalRetryDelay(reason, nil); ok {
			t.Fatalf("%q must not be retried", reason)
		}
		if !isTerminalExternalReason(reason) {
			t.Fatalf("%q must be terminal", reason)
		}
	}
}

func TestPersistExternalTerminalFailure(t *testing.T) {
	w := gpuWorkload()
	w.Status.Phase = v1.WorkloadPending
	r := &SchedulerReconciler{Client: ctrlfake.NewClientBuilder().WithScheme(ttlScheme(t)).
		WithObjects(w).WithStatusSubresource(&v1.Workload{}).Build()}
	reason := ExternalImageResolveReason + " - tls: unknown authority"
	if err := r.persistExternalTerminalFailure(context.Background(), w, reason); err != nil {
		t.Fatal(err)
	}
	// Caller snapshot must mirror Failed so updateUnScheduled does not overwrite.
	if w.Status.Phase != v1.WorkloadFailed {
		t.Fatalf("caller phase=%s want Failed", w.Status.Phase)
	}
	stored := &v1.Workload{}
	if err := r.Get(context.Background(), client.ObjectKey{Name: w.Name}, stored); err != nil {
		t.Fatal(err)
	}
	if stored.Status.Phase != v1.WorkloadFailed {
		t.Fatalf("phase=%s", stored.Status.Phase)
	}
	if !strings.Contains(stored.Status.Message, "image cannot be resolved") {
		t.Fatalf("message=%q", stored.Status.Message)
	}
	if err := r.persistExternalTerminalFailure(context.Background(), stored, reason); err != nil {
		t.Fatal(err)
	}
}
