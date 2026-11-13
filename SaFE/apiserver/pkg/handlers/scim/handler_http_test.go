/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package scim

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/resources"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/identity"
)

func scimEngine(t *testing.T, objs ...client.Object) (*gin.Engine, client.Client) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	scheme := runtime.NewScheme()
	require.NoError(t, v1.AddToScheme(scheme))
	cli := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build()
	engine := gin.New()
	Mount(engine, &resources.Handler{Client: cli}, NewToken("secret"))
	return engine, cli
}

func scimDo(engine *gin.Engine, method, path, body, token string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rsp := httptest.NewRecorder()
	engine.ServeHTTP(rsp, req)
	return rsp
}

func newSCIMRecord(name, userName string) *v1.User {
	user := &v1.User{}
	user.Name = name
	if userName != "" {
		v1.SetAnnotation(user, v1.UserNameAnnotation, userName)
	}
	user.Spec.Roles = []v1.UserRole{v1.DefaultRole}
	return user
}

// The Users routes reject a bad bearer, list and read records, and refuse a bad query.
func TestSCIMUserRoutes(t *testing.T) {
	engine, _ := scimEngine(t, newSCIMRecord("u-2", "bob"), newSCIMRecord("u-1", ""))
	require.Equal(t, http.StatusUnauthorized, scimDo(engine, http.MethodGet, "/scim/v2/Users", "", "").Code)

	list := scimDo(engine, http.MethodGet, "/scim/v2/Users?startIndex=1&count=1&filter=userName%20eq%20%22bob%22", "", "secret")
	require.Equal(t, http.StatusOK, list.Code)
	require.Contains(t, list.Body.String(), `"totalResults":1`)
	require.Contains(t, list.Body.String(), `"id":"u-2"`)

	past := scimDo(engine, http.MethodGet, "/scim/v2/Users?startIndex=9&count=-1", "", "secret")
	require.Equal(t, http.StatusOK, past.Code)
	require.Contains(t, past.Body.String(), `"itemsPerPage":0`)

	require.Equal(t, http.StatusBadRequest, scimDo(engine, http.MethodGet, "/scim/v2/Users?startIndex=no", "", "secret").Code)
	require.Equal(t, http.StatusBadRequest, scimDo(engine, http.MethodGet, "/scim/v2/Users?count=no", "", "secret").Code)
	require.Equal(t, http.StatusBadRequest, scimDo(engine, http.MethodGet, "/scim/v2/Users?filter=name%20sw%20%22a%22", "", "secret").Code)

	one := scimDo(engine, http.MethodGet, "/scim/v2/Users/u-1", "", "secret")
	require.Equal(t, http.StatusOK, one.Code)
	require.Contains(t, one.Body.String(), `"userName":"u-1"`)
	require.Equal(t, http.StatusNotFound, scimDo(engine, http.MethodGet, "/scim/v2/Users/missing", "", "secret").Code)

	_, err := (&server{}).load(context.Background(), "")
	require.True(t, apierrors.IsBadRequest(err))
}

// A patch freezes the user and rewrites roles from group mappings. A delete freezes the user and keeps
// the record, a patch of active=true restores it, and an unknown id is 204.
func TestSCIMPatchAndDelete(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	viper.Set("auth.group_mappings", []map[string]any{{"group": "admins", "roles": []string{"system-admin"}}})
	user := newSCIMRecord("u-1", "alice")
	engine, cli := scimEngine(t, user)

	same := `{"schemas":["urn:ietf:params:scim:api:messages:2.0:PatchOp"],"Operations":[{"op":"replace","path":"active","value":true}]}`
	require.Equal(t, http.StatusOK, scimDo(engine, http.MethodPatch, "/scim/v2/Users/u-1", same, "secret").Code)

	body := `{"op":"replace","path":"groups","value":["admins"]}`
	rsp := scimDo(engine, http.MethodPatch, "/scim/v2/Users/u-1", body, "secret")
	require.Equal(t, http.StatusOK, rsp.Code)
	got := &v1.User{}
	require.NoError(t, cli.Get(context.Background(), client.ObjectKey{Name: "u-1"}, got))
	require.Contains(t, got.Spec.Roles, v1.SystemAdminRole)

	off := `{"Operations":[{"op":"replace","path":"active","value":false}]}`
	require.Equal(t, http.StatusOK, scimDo(engine, http.MethodPatch, "/scim/v2/Users/u-1", off, "secret").Code)
	require.NoError(t, cli.Get(context.Background(), client.ObjectKey{Name: "u-1"}, got))
	require.Equal(t, v1.UserFrozen, got.Spec.RestrictedType)
	require.Equal(t, v1.TrueStr, v1.GetAnnotation(got, identity.DeactivatedAnnotation))

	on := `{"Operations":[{"op":"replace","value":{"active":true}}]}`
	require.Equal(t, http.StatusOK, scimDo(engine, http.MethodPatch, "/scim/v2/Users/u-1", on, "secret").Code)

	require.Equal(t, http.StatusBadRequest, scimDo(engine, http.MethodPatch, "/scim/v2/Users/u-1", "{", "secret").Code)
	require.Equal(t, http.StatusBadRequest, scimDo(engine, http.MethodPatch, "/scim/v2/Users/u-1", `{"Operations":[]}`, "secret").Code)
	require.Equal(t, http.StatusBadRequest, scimDo(engine, http.MethodPatch, "/scim/v2/Users/u-1", `{"Operations":[{"op":"replace","path":"displayName","value":"x"}]}`, "secret").Code)
	require.Equal(t, http.StatusNotFound, scimDo(engine, http.MethodPatch, "/scim/v2/Users/missing", off, "secret").Code)

	require.Equal(t, http.StatusNoContent, scimDo(engine, http.MethodDelete, "/scim/v2/Users/missing", "", "secret").Code)
	require.Equal(t, http.StatusNoContent, scimDo(engine, http.MethodDelete, "/scim/v2/Users/u-1", "", "secret").Code)
	require.NoError(t, cli.Get(context.Background(), client.ObjectKey{Name: "u-1"}, got))
	require.Equal(t, v1.UserFrozen, got.Spec.RestrictedType)
	require.Equal(t, v1.TrueStr, v1.GetAnnotation(got, identity.DeactivatedAnnotation))
	require.Equal(t, http.StatusNoContent, scimDo(engine, http.MethodDelete, "/scim/v2/Users/u-1", "", "secret").Code)
	require.Equal(t, http.StatusOK, scimDo(engine, http.MethodPatch, "/scim/v2/Users/u-1", on, "secret").Code)
	require.NoError(t, cli.Get(context.Background(), client.ObjectKey{Name: "u-1"}, got))
	require.Equal(t, v1.UserNormal, got.Spec.RestrictedType)
}

// A list or delete that the API server refuses is a 500.
func TestSCIMRoutesReportAPIErrors(t *testing.T) {
	gin.SetMode(gin.TestMode)
	scheme := runtime.NewScheme()
	require.NoError(t, v1.AddToScheme(scheme))
	user := newSCIMRecord("u-1", "alice")
	base := fake.NewClientBuilder().WithScheme(scheme).WithObjects(user).Build()
	cli := interceptor.NewClient(base, interceptor.Funcs{
		List: func(context.Context, client.WithWatch, client.ObjectList, ...client.ListOption) error {
			return apierrors.NewInternalError(context.Canceled)
		},
		Update: func(context.Context, client.WithWatch, client.Object, ...client.UpdateOption) error {
			return apierrors.NewInternalError(context.Canceled)
		},
	})
	engine := gin.New()
	Mount(engine, &resources.Handler{Client: cli}, NewToken("secret"))
	require.Equal(t, http.StatusInternalServerError, scimDo(engine, http.MethodGet, "/scim/v2/Users", "", "secret").Code)
	require.Equal(t, http.StatusInternalServerError, scimDo(engine, http.MethodDelete, "/scim/v2/Users/u-1", "", "secret").Code)
}
