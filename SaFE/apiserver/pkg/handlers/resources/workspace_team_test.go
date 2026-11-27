/*
 * Copyright (C) 2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resources

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/extauth"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/resources/view"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/identity"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
)

// GET /teams refuses a restricted user, whether the record or the request's identity restricts it.
func TestListTeamsRefusesRestrictedUsers(t *testing.T) {
	gin.SetMode(gin.TestMode)
	frozen := genUserFixture("frozen", []v1.UserRole{v1.DefaultRole}, nil, nil)
	frozen.Spec.RestrictedType = v1.UserFrozen
	h := newChartTeamsHandler(t, interceptor.Funcs{}, frozen,
		genUserFixture("u1", []v1.UserRole{v1.DefaultRole}, nil, nil))
	ctx := context.Background()

	rsp := callTeamsBody(ctx, h.ListTeams, "frozen", "", "", "")
	require.Equal(t, http.StatusForbidden, rsp.Code, rsp.Body.String())

	restricted := extauth.WithOverlay(ctx, extauth.MapGroups("u1", nil, nil, true))
	rsp = callTeamsBody(restricted, h.ListTeams, "u1", "", "", "")
	require.Equal(t, http.StatusForbidden, rsp.Code, rsp.Body.String())

	rsp = callTeamsBody(ctx, h.ListTeams, "u1", "", "", "")
	require.Equal(t, http.StatusOK, rsp.Code, rsp.Body.String())
}

// useExternalAuth switches the test to external mode with SCIM on and the group mappings, with the
// client of h for the team lookups, and restores the previous mode afterwards.
func useExternalAuth(t *testing.T, h *Handler, mappings []map[string]any) {
	t.Helper()
	t.Cleanup(func() { require.NoError(t, authority.InitExternalAuth(nil)) })
	setTeamsConfig(t, "crypto.enable", false)
	setTeamsConfig(t, "auth.mode", "external")
	setTeamsConfig(t, "auth.scim.enabled", true)
	setTeamsConfig(t, "auth.authenticators", []map[string]any{{"type": "builtin"}})
	setTeamsConfig(t, "auth.group_mappings", mappings)
	require.NoError(t, authority.InitExternalAuth(h.Client))
}

// GET /users/:name/workspaces judges the target by its recorded identity: a system-admin role that
// only the record stores does not count, and one granted by a recorded group does.
func TestGetUserWorkspacesJudgesTheEffectiveTarget(t *testing.T) {
	gin.SetMode(gin.TestMode)
	wsA := genWorkspaceFixture("ws-a", "A")
	wsA.Spec.Teams = []v1.WorkspaceTeam{{Name: "t1"}}
	stale := recordedTeamsUser("stale", []string{"t1"}, nil)
	stale.Spec.Roles = []v1.UserRole{v1.SystemAdminRole}
	byGroup := recordedTeamsUser("bygroup", nil, []string{"platform-admins"})
	byGroup.Spec.Resources = map[string][]string{common.UserWorkspaces: {"ws-a"}}
	h := newChartTeamsHandler(t, interceptor.Funcs{}, wsA, stale, byGroup,
		genUserFixture("admin", []v1.UserRole{v1.SystemAdminRole}, nil, nil))
	useExternalAuth(t, h, []map[string]any{{"group": "platform-admins", "roles": []string{"system-admin"}}})
	ctx := extauth.WithOverlay(context.Background(), extauth.MapGroups("admin", []string{"platform-admins"},
		[]commonconfig.AuthGroupMapping{{Group: "platform-admins", Roles: []string{"system-admin"}}}, false))

	rsp := callTeamsBody(ctx, h.GetUserWorkspaces, "admin", "stale", "", "")
	require.Equal(t, http.StatusOK, rsp.Code, rsp.Body.String())
	var got view.UserWorkspacesResponse
	require.NoError(t, json.Unmarshal(rsp.Body.Bytes(), &got))
	require.Equal(t, []view.WorkspaceEntry{{Id: "ws-a", Name: "A", Sources: []string{"team:t1"}}}, got.Workspaces)

	rsp = callTeamsBody(ctx, h.GetUserWorkspaces, "admin", "bygroup", "", "")
	require.Equal(t, http.StatusOK, rsp.Code, rsp.Body.String())
	got = view.UserWorkspacesResponse{}
	require.NoError(t, json.Unmarshal(rsp.Body.Bytes(), &got))
	require.Empty(t, got.Workspaces)
	require.Empty(t, got.ManagedWorkspaces)
}

// useIdentityServer switches the test to external mode with an identity service served by handler,
// signed in to as the introspection client svc-safe, and empties the team cache around the test.
func useIdentityServer(t *testing.T, handler http.HandlerFunc) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	secret := filepath.Join(t.TempDir(), "client-secret")
	require.NoError(t, os.WriteFile(secret, []byte("secret"), 0o600))
	identity.ResetTeamsCache()
	t.Cleanup(identity.ResetTeamsCache)
	setTeamsConfig(t, "auth.mode", "external")
	setTeamsConfig(t, "auth.scim.identity_url", srv.URL)
	setTeamsConfig(t, "auth.authenticators", []map[string]any{{
		"type": "introspection", "url": srv.URL + "/oauth/introspect",
		"client_id": "svc-safe", "client_secret_file": secret,
	}})
}

// The member endpoints answer an error, not an empty list, when the identity service refuses the
// call or fails: 502 for a refusal, 503 for a failure.
func TestMemberEndpointsReportIdentityFailures(t *testing.T) {
	gin.SetMode(gin.TestMode)
	wsA := genWorkspaceFixture("ws-a", "A")
	wsA.Spec.Teams = []v1.WorkspaceTeam{{Name: "t1"}}
	h := newChartTeamsHandler(t, interceptor.Funcs{}, wsA,
		genUserFixture("admin", []v1.UserRole{v1.SystemAdminRole}, nil, nil))
	for status, want := range map[int]int{
		http.StatusUnauthorized:        http.StatusBadGateway,
		http.StatusForbidden:           http.StatusBadGateway,
		http.StatusInternalServerError: http.StatusServiceUnavailable,
	} {
		useIdentityServer(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(status) })
		rsp := callTeamsBody(context.Background(), h.ListTeamMembers, "admin", "t1", "", "")
		require.Equal(t, want, rsp.Code, "%d: %s", status, rsp.Body.String())
		identity.ResetTeamsCache()
		rsp = callTeamsBody(context.Background(), h.ListWorkspaceMembers, "admin", "ws-a", "", "")
		require.Equal(t, want, rsp.Code, "%d: %s", status, rsp.Body.String())
	}
}

// PATCH /workspaces/:name answers 400 to invalid team bindings and access policies, and writes nothing.
func TestPatchWorkspaceValidatesTeams(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := newChartTeamsHandler(t, interceptor.Funcs{}, genWorkspaceFixture("ws-a", "A"),
		genUserFixture("admin", []v1.UserRole{v1.SystemAdminRole}, nil, nil))
	for _, body := range []string{
		`{"teams":[{"name":"Bad_Team"}]}`,
		`{"teams":[{"name":"t1","role":"owner"}]}`,
		`{"teams":[{"name":"t1"},{"name":"t1","role":"manager"}]}`,
		`{"accessPolicy":"open"}`,
	} {
		rsp := callTeamsBody(context.Background(), h.PatchWorkspace, "admin", "ws-a", body, "")
		require.Equal(t, http.StatusBadRequest, rsp.Code, body+": "+rsp.Body.String())
	}
	require.Empty(t, boundTeams(t, h, "ws-a"))
}

// failingTeamsHandler returns a handler whose write of ws-b fails. Before failing, it runs change on
// the stored ws-a, as another administrator would between the writes of the batch.
func failingTeamsHandler(t *testing.T, change func(*v1.Workspace), wsA *v1.Workspace) *Handler {
	t.Helper()
	funcs := interceptor.Funcs{
		Update: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.UpdateOption) error {
			if obj.GetName() == "ws-b" {
				stored := &v1.Workspace{}
				if err := c.Get(ctx, client.ObjectKey{Name: "ws-a"}, stored); err != nil {
					return err
				}
				change(stored)
				if err := c.Update(ctx, stored); err != nil {
					return err
				}
				return apierrors.NewInternalError(errors.New("write refused"))
			}
			return c.Update(ctx, obj, opts...)
		},
	}
	return newChartTeamsHandler(t, funcs, wsA, genWorkspaceFixture("ws-b", "B"),
		genUserFixture("admin", []v1.UserRole{v1.SystemAdminRole}, nil, nil))
}

// The undo leaves a binding alone that another request changed after the batch wrote it: a demoted
// manager binding that was then removed is not bound again.
func TestUpdateTeamWorkspacesUndoKeepsAConcurrentRemoval(t *testing.T) {
	gin.SetMode(gin.TestMode)
	wsA := genWorkspaceFixture("ws-a", "A")
	wsA.Spec.Teams = []v1.WorkspaceTeam{{Name: "t1", Role: v1.WorkspaceTeamManager}}
	h := failingTeamsHandler(t, func(ws *v1.Workspace) { ws.Spec.Teams = nil }, wsA)

	rsp := callTeamsBody(context.Background(), h.UpdateTeamWorkspaces, "admin", "t1",
		`[{"workspace":"ws-a","role":"member"},{"workspace":"ws-b"}]`, "")
	require.Equal(t, http.StatusInternalServerError, rsp.Code, rsp.Body.String())
	require.Empty(t, boundTeams(t, h, "ws-a"))
	require.Empty(t, boundTeams(t, h, "ws-b"))
}

// The undo touches only what the batch wrote: an entry that was already bound as asked, and was
// unbound by another request meanwhile, is not bound again.
func TestUpdateTeamWorkspacesUndoSkipsUnchangedEntries(t *testing.T) {
	gin.SetMode(gin.TestMode)
	wsA := genWorkspaceFixture("ws-a", "A")
	wsA.Spec.Teams = []v1.WorkspaceTeam{{Name: "t1", Role: v1.WorkspaceTeamMember}}
	h := failingTeamsHandler(t, func(ws *v1.Workspace) { ws.Spec.Teams = nil }, wsA)

	rsp := callTeamsBody(context.Background(), h.UpdateTeamWorkspaces, "admin", "t1",
		`[{"workspace":"ws-a"},{"workspace":"ws-b"}]`, "")
	require.Equal(t, http.StatusInternalServerError, rsp.Code, rsp.Body.String())
	require.Empty(t, boundTeams(t, h, "ws-a"))
}

// An undo that cannot restore a binding names the bindings that stayed in the error.
func TestUpdateTeamWorkspacesReportsBindingsThatStayed(t *testing.T) {
	gin.SetMode(gin.TestMode)
	funcs := interceptor.Funcs{
		Update: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.UpdateOption) error {
			ws := obj.(*v1.Workspace)
			if ws.Name == "ws-b" || ws.Name == "ws-a" && len(ws.Spec.Teams) == 0 {
				return apierrors.NewInternalError(errors.New("write refused"))
			}
			return c.Update(ctx, obj, opts...)
		},
	}
	h := newChartTeamsHandler(t, funcs, genWorkspaceFixture("ws-a", "A"), genWorkspaceFixture("ws-b", "B"),
		genUserFixture("admin", []v1.UserRole{v1.SystemAdminRole}, nil, nil))

	rsp := callTeamsBody(context.Background(), h.UpdateTeamWorkspaces, "admin", "t1",
		`[{"workspace":"ws-a","role":"manager"},{"workspace":"ws-b"}]`, "")
	require.Equal(t, http.StatusInternalServerError, rsp.Code, rsp.Body.String())
	require.Contains(t, rsp.Body.String(), "ws-a (manager)")
	require.Equal(t, []v1.WorkspaceTeam{{Name: "t1", Role: v1.WorkspaceTeamManager}}, boundTeams(t, h, "ws-a"))
}
