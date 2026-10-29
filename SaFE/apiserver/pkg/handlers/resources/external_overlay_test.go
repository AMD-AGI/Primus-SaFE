/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resources

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	testifyassert "github.com/stretchr/testify/assert"
	testifyrequire "github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/extauth"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/resources/view"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	jsonutils "github.com/AMD-AIG-AIMA/SAFE/utils/pkg/json"
)

const externalUserID = "ext-1"

// asExternal makes the request one the external authenticator chain accepted for ext-1, an
// identity whose groups the configured mapping turns into roles and workspaces: the same
// overlay the apiserver's authentication step puts in the request context.
func asExternal(c *gin.Context, groups ...string) *gin.Context {
	mappings := []commonconfig.AuthGroupMapping{
		{Group: "platform-admins", Roles: []string{"system-admin"}},
		{Group: "team-b", Roles: []string{"default"}, Workspaces: []string{"ws-b"}},
	}
	c.Set(common.UserId, externalUserID)
	c.Request = c.Request.WithContext(extauth.WithOverlay(c.Request.Context(),
		extauth.MapGroups(externalUserID, groups, mappings, false)))
	return c
}

// externalUser is what the first sight of an identity leaves in the cluster: the default role
// and, here, membership of ws-a; no administrator role.
func externalUser() *v1.User {
	return &v1.User{
		ObjectMeta: metav1.ObjectMeta{Name: externalUserID, Labels: map[string]string{v1.UserIdLabel: externalUserID}},
		Spec: v1.UserSpec{Type: v1.SSOUserType, Roles: []v1.UserRole{v1.DefaultRole},
			Resources: map[string][]string{common.UserWorkspaces: {"ws-a"}}},
	}
}

func (h *Handler) storedUser(t *testing.T, name string) *v1.User {
	t.Helper()
	u := &v1.User{}
	testifyrequire.NoError(t, h.Get(context.Background(), client.ObjectKey{Name: name}, u))
	return u
}

// getAndSetUsername is how every handler resolves the requester; in external mode that user
// must be the one the request's groups make, or the checks that read its roles directly
// refuse an administrator made so by a group.
func TestGetAndSetUsernameCarriesTheOverlay(t *testing.T) {
	h, _ := newSandboxEnvHandler(t)
	testifyrequire.NoError(t, h.Create(context.Background(), externalUser()))

	user, err := h.getAndSetUsername(asExternal(newPostCtx(nil), "platform-admins", "team-b"))
	testifyrequire.NoError(t, err)
	testifyassert.True(t, user.IsSystemAdmin())
	testifyassert.ElementsMatch(t, []string{"ws-a", "ws-b"}, user.Spec.Resources[common.UserWorkspaces])

	user, err = h.getAndSetUsername(asExternal(newPostCtx(nil)))
	testifyrequire.NoError(t, err)
	testifyassert.False(t, user.IsSystemAdmin(), "no group, no role")
	testifyassert.Equal(t, []string{"ws-a"}, user.Spec.Resources[common.UserWorkspaces])
}

// authorizeSandboxEnvSecret refuses all but system admins and the admins of every workspace
// the Secret is bound to. An administrator by group is one, through the real handler.
func TestExternalAdminCanCreateTheSandboxEnvSecret(t *testing.T) {
	create := func(h *Handler, groups ...string) error {
		c := asExternal(newPostCtx(jsonutils.MarshalSilently(sandboxEnvCreate(sandboxEnvSecret, "ws-a"))), groups...)
		_, err := h.createSecret(c)
		return err
	}
	h, _ := newSandboxEnvHandler(t)
	testifyrequire.NoError(t, h.Create(context.Background(), externalUser()))

	// The control: the same request without the group is refused by the guard itself.
	assertGuardRefused(t, create(h))

	testifyrequire.NoError(t, create(h, "platform-admins"))
	_, err := h.clientSet.CoreV1().Secrets(common.PrimusSafeNamespace).Get(context.Background(), sandboxEnvSecret, metav1.GetOptions{})
	testifyrequire.NoError(t, err, "the Secret exists")
}

// The same guard on the delete path, which reads the Secret's workspace bindings too.
func TestExternalAdminCanDeleteTheSandboxEnvSecret(t *testing.T) {
	// Owned by the requester, so the ordinary role check passes and only the guard can refuse.
	h, _ := newSandboxEnvHandler(t, ownedSecret(sandboxEnvSecret, externalUserID, "ws-a"))
	testifyrequire.NoError(t, h.Create(context.Background(), externalUser()))
	del := func(groups ...string) error {
		c := asExternal(newPostCtx(nil), groups...)
		c.Set(common.Name, sandboxEnvSecret)
		_, err := h.deleteSecret(c)
		return err
	}
	assertGuardRefused(t, del())
	testifyrequire.NoError(t, del("platform-admins"))
}

// generateWorkspace lets only a system admin set reserved labels. An administrator by group
// creating a workspace through the real handler gets them stamped.
func TestExternalAdminCanStampReservedWorkspaceLabels(t *testing.T) {
	// Not newAdminHandlerWithObjects: its access controller is the process-wide singleton, bound
	// to whichever client of the test binary asked first.
	scheme := runtime.NewScheme()
	testifyrequire.NoError(t, v1.AddToScheme(scheme))
	cli := fake.NewClientBuilder().WithScheme(scheme).WithObjects(genMockRole(), externalUser()).Build()
	h := &Handler{Client: cli, accessController: &authority.AccessController{Client: cli}}
	body := jsonutils.MarshalSilently(view.CreateWorkspaceRequest{
		Name: "ext-ws", ClusterId: "c1", Labels: map[string]string{v1.WorkspaceExternalLabel: v1.TrueStr},
	})
	rsp := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rsp)
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	h.CreateWorkspace(asExternal(c, "platform-admins"))
	testifyrequire.Equal(t, http.StatusOK, rsp.Code, rsp.Body.String())

	list := &v1.WorkspaceList{}
	testifyrequire.NoError(t, h.List(context.Background(), list))
	testifyrequire.Len(t, list.Items, 1)
	testifyassert.Equal(t, v1.TrueStr, list.Items[0].Labels[v1.WorkspaceExternalLabel],
		"a reserved label an administrator set must be kept")
	testifyassert.Equal(t, externalUserID, list.Items[0].Labels[v1.UserIdLabel])
}

// requestUser is a copy for the request: updating a user from it would store the roles and
// workspaces the identity only lent, and a role revoked upstream would then outlive the
// revocation in the record. The handler that writes the requester's own record reads it first.
func TestUpdateOwnSettingsDoesNotStoreWhatTheIdentityLent(t *testing.T) {
	h, _ := newSandboxEnvHandler(t)
	testifyrequire.NoError(t, h.Create(context.Background(), externalUser()))

	c := asExternal(newPostCtx([]byte(`{"enableNotification": true}`)), "platform-admins", "team-b")
	c.Set(common.Name, common.UserSelf)
	_, err := h.updateUserSettings(c)
	testifyrequire.NoError(t, err)

	stored := h.storedUser(t, externalUserID)
	testifyassert.Equal(t, v1.TrueStr, v1.GetAnnotation(stored, v1.UserEnableNotificationAnnotation), "the setting itself was saved")
	testifyassert.Equal(t, []v1.UserRole{v1.DefaultRole}, stored.Spec.Roles, "the role the group lent must not be stored")
	testifyassert.Equal(t, []string{"ws-a"}, stored.Spec.Resources[common.UserWorkspaces], "nor the workspace")
	testifyassert.False(t, stored.IsRestricted())
}
