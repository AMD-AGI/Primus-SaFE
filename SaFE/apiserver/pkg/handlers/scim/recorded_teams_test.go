/*
 * Copyright (C) 2026, Advanced Micro Devices, Inc. All rights reserved.
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
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/resources"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/identity"
)

// parseTeams runs parsePatch on the body and returns the teams it carries.
func parseTeams(t *testing.T, body string) (*[]string, error) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPatch, "/scim/v2/Users/x", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/scim+json")
	plan, err := parsePatch(c)
	return plan.teams, err
}

// A teams replace keeps the valid team ids, sorted and without duplicates; null and non-arrays are refused.
func TestParsePatchTeams(t *testing.T) {
	for _, tc := range []struct {
		name, value string
		want        []string
	}{
		{"sorted and deduplicated", `["t2","t1","t2"]`, []string{"t1", "t2"}},
		{"empty and invalid ids dropped", `["","t1","Bad_Team","a:b","team:t3","-x","t1"]`, []string{"t1"}},
		{"empty list", `[]`, []string{}},
		{"nothing valid", `["", "UPPER"]`, []string{}},
	} {
		body := `{"Operations":[{"op":"Replace","path":"teams","value":` + tc.value + `}]}`
		got, err := parseTeams(t, body)
		require.NoError(t, err, tc.name)
		require.NotNil(t, got, tc.name)
		require.Equal(t, tc.want, *got, tc.name)
	}
	for _, value := range []string{`null`, `"t1"`, `{"t1":true}`} {
		_, err := parseTeams(t, `{"op":"replace","path":"teams","value":`+value+`}`)
		require.Error(t, err, value)
	}
}

// A teams push is recorded on the user, filtered and sorted, and reported back.
func TestSCIMPatchRecordsTheTeams(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	engine, cli := scimEngine(t, newSCIMRecord("u-1", "alice"))
	set := `{"op":"replace","path":"teams","value":["t2","","Bad","t1"]}`
	rsp := scimDo(engine, http.MethodPatch, "/scim/v2/Users/u-1", set, "secret")
	require.Equal(t, http.StatusOK, rsp.Code)
	require.Contains(t, rsp.Body.String(), `"teams":["t1","t2"]`)
	got := &v1.User{}
	require.NoError(t, cli.Get(context.Background(), client.ObjectKey{Name: "u-1"}, got))
	require.Equal(t, []string{"t1", "t2"}, identity.RecordedTeams(got))
}

// A push of teams or groups to a deactivated user records nothing and writes nothing.
func TestSCIMPatchOfADeactivatedUserWritesNothing(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	gin.SetMode(gin.TestMode)
	user := newSCIMRecord("u-1", "alice")
	user.Spec.RestrictedType = v1.UserFrozen
	v1.SetAnnotation(user, identity.DeactivatedAnnotation, v1.TrueStr)
	scheme := runtime.NewScheme()
	require.NoError(t, v1.AddToScheme(scheme))
	writes := 0
	cli := interceptor.NewClient(fake.NewClientBuilder().WithScheme(scheme).WithObjects(user).Build(), interceptor.Funcs{
		Update: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.UpdateOption) error {
			writes++
			return c.Update(ctx, obj, opts...)
		},
	})
	engine := gin.New()
	Mount(engine, &resources.Handler{Client: cli}, NewToken("secret"))
	for _, body := range []string{
		`{"op":"replace","path":"teams","value":["t1"]}`,
		`{"op":"replace","path":"groups","value":["g1"]}`,
	} {
		rsp := scimDo(engine, http.MethodPatch, "/scim/v2/Users/u-1", body, "secret")
		require.Equal(t, http.StatusOK, rsp.Code, body)
		require.Contains(t, rsp.Body.String(), `"teams":[]`, body)
	}
	require.Zero(t, writes)
}
