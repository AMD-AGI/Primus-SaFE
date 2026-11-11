/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resources

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/resources/view"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
)

// statusContext returns a gin context for a request authenticated as userType with a session cookie.
func statusContext(userType string) *gin.Context {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPatch, "/api/v1/users/bob", nil)
	c.Request.AddCookie(&http.Cookie{Name: authority.CookieToken, Value: "safe-session"})
	c.Set(common.UserType, userType)
	return c
}

func TestForwardIdentityStatusNeedsAnIdentityCredential(t *testing.T) {
	t.Cleanup(func() { viper.Set("auth.mode", nil) })
	frozen := v1.UserFrozen
	req := &view.PatchUserRequest{RestrictedType: &frozen}
	user := &v1.User{}
	user.Name = "bob"

	// A SaFE session is not an identity credential: the change is refused before identity is called.
	viper.Set("auth.mode", "external")
	err := forwardIdentityStatus(statusContext(string(v1.SSOUserType)), user, req)
	require.Error(t, err)
	require.True(t, apierrors.IsConflict(err), "got %v", err)

	// A credential the external chain accepted is forwarded to identity.
	err = forwardIdentityStatus(statusContext(authority.UserTypeExternal), user, req)
	require.Error(t, err)
	require.False(t, apierrors.IsConflict(err), "got %v", err)
	require.Contains(t, err.Error(), "identity")

	// Builtin mode keeps the status local.
	viper.Set("auth.mode", "builtin")
	require.NoError(t, forwardIdentityStatus(statusContext(string(v1.SSOUserType)), user, req))
}
