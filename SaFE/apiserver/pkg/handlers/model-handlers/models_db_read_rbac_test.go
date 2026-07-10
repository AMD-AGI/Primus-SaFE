/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package model_handlers

import (
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	mock_client "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/mock"
)

// dbReadModels returns a fixed set of DB models with mixed visibility used to
// exercise read-path RBAC on the database code path.
func dbReadModels() []*dbclient.Model {
	return []*dbclient.Model{
		{ID: "d-pub", AccessMode: "local", DisplayName: "pub", UserId: "owner-1", Workspace: ""},
		{ID: "d-own", AccessMode: "local", DisplayName: "own", UserId: "owner-1", Workspace: "ws-1"},
		{ID: "d-wsonly", AccessMode: "local", DisplayName: "wsonly", UserId: "nobody", Workspace: "ws-1"},
		{ID: "d-other", AccessMode: "local", DisplayName: "other", UserId: "stranger-1", Workspace: "ws-2"},
	}
}

func newDBReadRBACHandler(t *testing.T, m dbclient.Interface) *Handler {
	t.Helper()
	return &Handler{
		dbClient:         m,
		k8sClient:        ctrlfake.NewClientBuilder().WithScheme(modelScheme(t)).Build(),
		accessController: newReadRBACAC(t),
	}
}

// TestListModelsDBReadRBAC verifies #9: read visibility filtering also applies
// on the database code path of listModels (not only the K8s fallback).
func TestListModelsDBReadRBAC(t *testing.T) {
	cases := []struct {
		name string
		user string
		want []string
	}{
		{"member sees public + ws-1", "member-1", []string{"d-pub", "d-own", "d-wsonly"}},
		{"owner sees public + owned", "owner-1", []string{"d-pub", "d-own"}},
		{"stranger sees public + owned", "stranger-1", []string{"d-pub", "d-other"}},
		{"admin sees all", "admin-1", []string{"d-pub", "d-own", "d-wsonly", "d-other"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			m := mock_client.NewMockInterface(ctrl)
			m.EXPECT().ListModels(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
				Return(dbReadModels(), nil)
			h := newDBReadRBACHandler(t, m)

			res, err := h.listModels(readRBACCtx(tc.user, "limit=100&offset=0", nil))
			assert.NoError(t, err)
			resp, ok := res.(*ListModelResponse)
			assert.True(t, ok)
			got := make(map[string]bool, len(resp.Items))
			for _, it := range resp.Items {
				got[it.ID] = true
			}
			assert.Equal(t, len(tc.want), len(got), "unexpected visible set: %v", got)
			for _, id := range tc.want {
				assert.True(t, got[id], "expected %s visible to %s", id, tc.user)
			}
		})
	}
}

// TestGetModelDBReadRBAC verifies #9: getModel enforces read authorization on
// the database code path and returns 403 for models the caller may not access.
func TestGetModelDBReadRBAC(t *testing.T) {
	cases := []struct {
		name   string
		user   string
		denied bool
	}{
		{"member denied other workspace", "member-1", true},
		{"owner allowed", "stranger-1", false},
		{"admin allowed", "admin-1", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			m := mock_client.NewMockInterface(ctrl)
			m.EXPECT().GetModelByID(gomock.Any(), "d-other").
				Return(&dbclient.Model{ID: "d-other", AccessMode: "local", UserId: "stranger-1", Workspace: "ws-2"}, nil)
			h := newDBReadRBACHandler(t, m)

			_, err := h.getModel(readRBACCtx(tc.user, "", gin.Params{{Key: "id", Value: "d-other"}}))
			if tc.denied {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "not allowed")
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
