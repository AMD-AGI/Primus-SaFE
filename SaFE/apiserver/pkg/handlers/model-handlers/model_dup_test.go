/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package model_handlers

import (
	"context"
	"errors"
	"testing"

	"k8s.io/apimachinery/pkg/runtime"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

// TestFindModelBySourceURLListError verifies S14: when the K8s List call fails,
// the duplicate check must surface the error instead of silently reporting
// "no duplicate", which could otherwise allow creating duplicate models.
func TestFindModelBySourceURLListError(t *testing.T) {
	s := runtime.NewScheme()
	if err := v1.AddToScheme(s); err != nil {
		t.Fatalf("add scheme: %v", err)
	}
	failing := ctrlfake.NewClientBuilder().WithScheme(s).WithInterceptorFuncs(interceptor.Funcs{
		List: func(_ context.Context, _ ctrlclient.WithWatch, _ ctrlclient.ObjectList, _ ...ctrlclient.ListOption) error {
			return errors.New("api server down")
		},
	}).Build()

	h := newMockModelHandler(failing)
	if _, err := h.findModelBySourceURL(context.Background(), "https://huggingface.co/x/y", "ws1"); err == nil {
		t.Fatal("expected error when List fails, got nil")
	}
}