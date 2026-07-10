/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package model_handlers

import (
	"context"
	"errors"
	"net/http"
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

// TestCreateModelFromS3SyncListError verifies S14 also covers the s3_sync path:
// when the duplicate-check List fails, s3_sync creation must surface the error
// instead of silently skipping dedup (which could create duplicate models).
func TestCreateModelFromS3SyncListError(t *testing.T) {
	s := runtime.NewScheme()
	if err := v1.AddToScheme(s); err != nil {
		t.Fatalf("add scheme: %v", err)
	}
	failing := ctrlfake.NewClientBuilder().WithScheme(s).WithInterceptorFuncs(interceptor.Funcs{
		List: func(_ context.Context, _ ctrlclient.WithWatch, _ ctrlclient.ObjectList, _ ...ctrlclient.ListOption) error {
			return errors.New("api server down")
		},
	}).Build()

	h := &Handler{k8sClient: failing, accessController: adminModelAC()}
	c := sessCtx(t, http.MethodPost, `{"displayName":"S3","source":{"accessMode":"s3_sync"},"s3Source":{"uri":"s3://b/p"}}`, adminModelUserID, nil)
	if _, err := h.createModel(c); err == nil {
		t.Fatal("expected error when List fails during s3_sync duplicate check, got nil")
	}
}
