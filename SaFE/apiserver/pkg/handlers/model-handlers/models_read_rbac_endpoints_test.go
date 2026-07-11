/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package model_handlers

import (
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

// TestGetModelWorkloadsReadRBAC verifies that getModelWorkloads enforces the
// same read visibility as getModel: users who cannot see a private model must
// not be able to enumerate its associated workloads.
func TestGetModelWorkloadsReadRBAC(t *testing.T) {
	h := newReadRBACHandler(t) // m-other lives in ws-2, owned by stranger-1
	cases := []struct {
		name    string
		user    string
		modelID string
		denied  bool
	}{
		{"member denied other workspace", "member-1", "m-other", true},
		{"owner allowed", "stranger-1", "m-other", false},
		{"admin allowed", "admin-1", "m-other", false},
		{"public visible to member", "member-1", "m-pub", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := h.getModelWorkloads(readRBACCtx(tc.user, "", gin.Params{{Key: "id", Value: tc.modelID}}))
			if tc.denied {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "not allowed")
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// newLocalReadModel builds a Ready local model (deployable) owned by owner in
// the given workspace, with a ready local path so getWorkloadConfig can resolve
// a path once the visibility gate passes.
func newLocalReadModel(name, owner, workspace string) *v1.Model {
	m := &v1.Model{ObjectMeta: metav1.ObjectMeta{Name: name}}
	m.Labels = map[string]string{v1.UserIdLabel: owner}
	m.Spec.Workspace = workspace
	m.Spec.DisplayName = name
	m.Spec.Source.AccessMode = v1.AccessModeLocal
	m.Status.Phase = v1.ModelPhaseReady
	m.Status.LocalPaths = []v1.ModelLocalPath{{
		Workspace: workspace,
		Path:      "/apps/models/" + name,
		Status:    v1.LocalPathStatusReady,
	}}
	return m
}

// TestGetWorkloadConfigReadRBAC verifies getWorkloadConfig gates on read
// visibility before returning the on-disk model path / launch command.
func TestGetWorkloadConfigReadRBAC(t *testing.T) {
	// A private local model in ws-2 owned by stranger-1.
	k8s := ctrlfake.NewClientBuilder().WithScheme(modelScheme(t)).
		WithObjects(newLocalReadModel("lm-other", "stranger-1", "ws-2")).Build()
	h := &Handler{k8sClient: k8s, accessController: newReadRBACAC(t)}

	// member-1 (member of ws-1, not ws-2, not owner) must be denied before any
	// path is exposed.
	_, err := h.getWorkloadConfig(readRBACCtx("member-1", "workspace=ws-2", gin.Params{{Key: "id", Value: "lm-other"}}))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not allowed")

	// admin passes the visibility gate (it must not fail with a 403).
	_, err = h.getWorkloadConfig(readRBACCtx("admin-1", "workspace=ws-2", gin.Params{{Key: "id", Value: "lm-other"}}))
	if err != nil {
		assert.NotContains(t, err.Error(), "not allowed")
	}
}
