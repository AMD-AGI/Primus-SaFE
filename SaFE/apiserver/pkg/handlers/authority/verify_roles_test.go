/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package authority

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/lib/pq"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	mock_client "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/mock"
)

// rawRoles returns what the response carries under "roles", as JSON: a list, or null, or absent.
func rawRoles(t *testing.T, body []byte) json.RawMessage {
	t.Helper()
	var resp struct {
		Data map[string]json.RawMessage `json:"data"`
	}
	require.NoError(t, json.Unmarshal(body, &resp))
	roles, present := resp.Data["roles"]
	require.True(t, present, "the response has no roles field: %s", body)
	return roles
}

// POST /auth/verify returns the roles of the user whatever the credential, so a service that serves only users who
// hold a certain role can ask: a user who holds only the minimal role is told apart from one who holds access. The
// field is always a list, empty when the user holds none.
func TestVerifyReturnsTheRolesOfTheUser(t *testing.T) {
	defer setupInternalAuth(t)()
	defer setupDefaultToken(t)()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockDB := mock_client.NewMockInterface(ctrl)
	testKey := syntheticAPIKey("verify-roles")
	mockDB.EXPECT().GetApiKeyByKey(gomock.Any(), HashApiKey(testKey, nil)).Return(&dbclient.ApiKey{
		Id: 7, UserId: "roles-user", UserName: "roles", Whitelist: "[]",
		ExpirationTime: pq.NullTime{Time: time.Now().UTC().Add(time.Hour), Valid: true},
	}, nil).AnyTimes()

	for _, tt := range []struct {
		name   string
		stored []v1.UserRole
		want   string
	}{
		{"the minimal role", []v1.UserRole{v1.SelfReadRole}, `["self-read"]`},
		{"the default role", []v1.UserRole{v1.DefaultRole}, `["default"]`},
		{"both", []v1.UserRole{v1.SelfReadRole, v1.DefaultRole}, `["self-read","default"]`},
		{"none is a list, not null", nil, `[]`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			scheme := runtime.NewScheme()
			require.NoError(t, v1.AddToScheme(scheme))
			cli := fake.NewClientBuilder().WithScheme(scheme).WithObjects(&v1.User{
				ObjectMeta: metav1.ObjectMeta{Name: "roles-user"},
				Spec:       v1.UserSpec{Type: v1.DefaultUserType, Roles: tt.stored},
			}).Build()
			oldDefault, oldKey := defaultTokenInstance, apiKeyTokenInstance
			defaultTokenInstance = &defaultToken{Client: cli}
			apiKeyTokenInstance = &ApiKeyToken{dbClient: mockDB, Client: cli}
			defer func() { defaultTokenInstance, apiKeyTokenInstance = oldDefault, oldKey }()

			token := generateTestToken(t, "roles-user", "roles")
			for name, req := range map[string]VerifyTokenRequest{
				"cookie":  {Cookie: "Token=" + token},
				"bearer":  {Authorization: "Bearer " + token},
				"api key": verifyRequestWithAuthKey(VerifyTokenRequest{ClientIP: "10.0.0.1"}, testKey),
			} {
				w := callVerifyToken(t, req, testInternalToken)
				require.Equal(t, http.StatusOK, w.Code, "%s: %s", name, w.Body.String())
				require.JSONEq(t, tt.want, string(rawRoles(t, w.Body.Bytes())), name)
			}
		})
	}
}

// In external mode the roles are those of the request as the apiserver sees it, which include the base roles:
// a new identity of a deployment whose base role is the minimal one is told apart by the roles the endpoint
// returns, with nothing stored on its record.
func TestVerifyReturnsTheBaseRolesInExternalMode(t *testing.T) {
	defer setupInternalAuth(t)()
	for _, tt := range []struct {
		name   string
		base   any
		groups []string
		want   string
	}{
		{"the default base role", nil, []string{"team-a"}, `["default"]`},
		{"the minimal base role", []string{"self-read"}, []string{"team-a"}, `["self-read"]`},
		{"an administrator the provider vouches for holds that role alone", []string{"self-read"}, []string{"platform-admins"}, `["system-admin"]`},
		{"two base roles", []string{"self-read", "viewer"}, nil, `["self-read","viewer"]`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			viper.Set("users.base_roles", tt.base)
			t.Cleanup(func() { viper.Set("users.base_roles", nil) })
			e := newExtEnv(t, adminMapping, false)

			w := callVerifyToken(t, VerifyTokenRequest{Authorization: "Bearer " + e.jwt(map[string]any{"groups": tt.groups})}, testInternalToken)
			require.Equal(t, http.StatusOK, w.Code, w.Body.String())
			require.JSONEq(t, tt.want, string(rawRoles(t, w.Body.Bytes())))

			u := e.user(extUser)
			require.Empty(t, u.Spec.Roles, "the base roles are never stored by the apiserver")
		})
	}
}
