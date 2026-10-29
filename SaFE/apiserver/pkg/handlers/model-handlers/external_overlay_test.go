/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package model_handlers

import (
	"context"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	testifyassert "github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/extauth"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
)

const externalUserID = "ext-1"

// asExternal makes the request one the external authenticator chain accepted for ext-1, an
// identity whose groups the configured mapping turns into roles: the same overlay the
// apiserver's authentication step puts in the request context.
func asExternal(c *gin.Context, groups ...string) *gin.Context {
	mappings := []commonconfig.AuthGroupMapping{{Group: "platform-admins", Roles: []string{"system-admin"}}}
	overlay := extauth.MapGroups(externalUserID, groups, mappings, false)
	c.Request = c.Request.WithContext(extauth.WithOverlay(c.Request.Context(), overlay))
	return c
}

// withExternalUser adds ext-1 to the cluster of the handler: a user the identity provider
// created on first sight, so the record holds the default role and nothing else.
func withExternalUser(t *testing.T, h *Handler) *Handler {
	t.Helper()
	require.NoError(t, h.accessController.Client.Create(context.Background(), &v1.User{
		ObjectMeta: metav1.ObjectMeta{Name: externalUserID},
		Spec:       v1.UserSpec{Type: v1.SSOUserType, Roles: []v1.UserRole{v1.DefaultRole}},
	}))
	return h
}

// canViewModel and canAccessWorkspace read the user that readVisibilityUser fetches. In
// external mode that user must carry the roles the request's groups map to, or an
// administrator made so by a group is refused what a stored administrator may see.
func TestExternalAdminSeesWhatAnAdministratorSees(t *testing.T) {
	t.Run("getModel: canViewModel", func(t *testing.T) {
		h := withExternalUser(t, newReadRBACHandler(t)) // m-other: private, ws-2, owned by stranger-1
		params := gin.Params{{Key: "id", Value: "m-other"}}

		_, err := h.getModel(asExternal(readRBACCtx(externalUserID, "", params)))
		testifyassert.Error(t, err, "no group, no admin: the control that the fixture refuses")
		testifyassert.Contains(t, err.Error(), "not allowed")

		_, err = h.getModel(asExternal(readRBACCtx(externalUserID, "", params), "platform-admins"))
		testifyassert.NoError(t, err)
	})

	t.Run("listModels: canViewModel", func(t *testing.T) {
		h := withExternalUser(t, newReadRBACHandler(t))
		list := func(c *gin.Context) map[string]bool {
			res, err := h.listModels(c)
			require.NoError(t, err)
			ids := map[string]bool{}
			for _, it := range res.(*ListModelResponse).Items {
				ids[it.ID] = true
			}
			return ids
		}
		testifyassert.Equal(t, map[string]bool{"m-pub": true}, list(asExternal(readRBACCtx(externalUserID, "limit=100&offset=0", nil))))
		testifyassert.Equal(t, map[string]bool{"m-pub": true, "m-own": true, "m-wsonly": true, "m-other": true},
			list(asExternal(readRBACCtx(externalUserID, "limit=100&offset=0", nil), "platform-admins")))
	})

	t.Run("playground services: canAccessWorkspace", func(t *testing.T) {
		h := withExternalUser(t, newPlaygroundRBACHandler(t)) // wl-ws2 runs in ws-2, which ext-1 is not a member of
		list := func(c *gin.Context) map[string]bool {
			res, err := h.listPlaygroundServices(c)
			require.NoError(t, err)
			ids := map[string]bool{}
			for _, it := range res.(*ListPlaygroundServicesResponse).Items {
				ids[it.ID] = true
			}
			return ids
		}
		testifyassert.False(t, list(asExternal(readRBACCtx(externalUserID, "", nil)))["wl-ws2"])
		testifyassert.True(t, list(asExternal(readRBACCtx(externalUserID, "", nil), "platform-admins"))["wl-ws2"])
	})

	t.Run("chat with a workload: canAccessWorkspace", func(t *testing.T) {
		h := withExternalUser(t, newPlaygroundRBACHandler(t))
		chat := func(groups ...string) int {
			c, w := newChatCtx(externalUserID, `{"serviceId":"wl-ws2","messages":[{"role":"user","content":"hi"}]}`)
			h.Chat(asExternal(c, groups...))
			return w.Code
		}
		testifyassert.Equal(t, http.StatusForbidden, chat())
		testifyassert.NotEqual(t, http.StatusForbidden, chat("platform-admins"), "an admin by group passes the workspace gate")
	})
}
