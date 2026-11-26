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
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority/authtest"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/resources/view"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/identity"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	commonworkspace "github.com/AMD-AIG-AIMA/SAFE/common/pkg/workspace"
)

// newTeamsHandler builds a handler over a fake client with the team index and a default role
// that lets workspace users get their workspaces.
func newTeamsHandler(objs ...runtime.Object) *Handler {
	scheme := runtime.NewScheme()
	_ = v1.AddToScheme(scheme)
	role := &v1.Role{
		ObjectMeta: metav1.ObjectMeta{Name: string(v1.DefaultRole)},
		Rules: []v1.PolicyRule{{
			Resources:    []string{"workspace"},
			Verbs:        []v1.RoleVerb{v1.GetVerb, v1.ListVerb},
			GrantedUsers: []string{authority.GrantedWorkspaceUser},
		}},
	}
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(objs...).WithObjects(role).
		WithIndex(&v1.Workspace{}, commonworkspace.TeamIndex, commonworkspace.TeamIndexFunc).Build()
	return &Handler{
		Client:           fakeClient,
		accessController: &authority.AccessController{Client: fakeClient},
	}
}

// callTeamsHandler runs a handler as the user with the team name as the route parameter.
func callTeamsHandler(fn gin.HandlerFunc, userId, team string) *httptest.ResponseRecorder {
	rsp := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rsp)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c.Set(common.UserId, userId)
	c.Set(common.Name, team)
	fn(c)
	return rsp
}

func TestListTeamsIsEmptyWithoutIdentity(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := newTeamsHandler(genUserFixture("u1", []v1.UserRole{v1.DefaultRole}, nil, nil))
	rsp := callTeamsHandler(h.ListTeams, "u1", "")
	require.Equal(t, http.StatusOK, rsp.Code, rsp.Body.String())
	var got view.ListTeamResponse
	require.NoError(t, json.Unmarshal(rsp.Body.Bytes(), &got))
	require.Equal(t, 0, got.TotalCount)
	require.Contains(t, rsp.Body.String(), `"items":[]`)
}

func TestListTeamMembersAuthorization(t *testing.T) {
	gin.SetMode(gin.TestMode)
	bound := genWorkspaceFixture("ws-a", "A")
	bound.Spec.Teams = []v1.WorkspaceTeam{{Name: "t1"}}
	other := genWorkspaceFixture("ws-b", "B")
	h := newTeamsHandler(bound, other,
		genUserFixture("admin", []v1.UserRole{v1.SystemAdminRole}, nil, nil),
		genUserFixture("member", []v1.UserRole{v1.DefaultRole}, []string{"ws-a"}, nil),
		genUserFixture("outsider", []v1.UserRole{v1.DefaultRole}, []string{"ws-b"}, nil))

	for _, user := range []string{"admin", "member"} {
		rsp := callTeamsHandler(h.ListTeamMembers, user, "t1")
		require.Equal(t, http.StatusOK, rsp.Code, user+": "+rsp.Body.String())
		var got view.TeamMembersResponse
		require.NoError(t, json.Unmarshal(rsp.Body.Bytes(), &got))
		require.Equal(t, "t1", got.TeamId)
		require.Contains(t, rsp.Body.String(), `"items":[]`)
	}
	require.Equal(t, http.StatusForbidden, callTeamsHandler(h.ListTeamMembers, "outsider", "t1").Code)
	require.Equal(t, http.StatusForbidden, callTeamsHandler(h.ListTeamMembers, "outsider", "unbound").Code)
	require.Equal(t, http.StatusBadRequest, callTeamsHandler(h.ListTeamMembers, "admin", "Bad_Team").Code)
}

func TestListTeamWorkspacesShowsOnlyListableWorkspaces(t *testing.T) {
	gin.SetMode(gin.TestMode)
	wsA := genWorkspaceFixture("ws-a", "A")
	wsA.Spec.Teams = []v1.WorkspaceTeam{{Name: "t1"}}
	wsB := genWorkspaceFixture("ws-b", "B")
	wsB.Spec.Teams = []v1.WorkspaceTeam{{Name: "t1", Role: v1.WorkspaceTeamManager}}
	adminRole := &v1.Role{
		ObjectMeta: metav1.ObjectMeta{Name: string(v1.SystemAdminRole)},
		Rules: []v1.PolicyRule{{
			Resources:    []string{"workspace"},
			Verbs:        []v1.RoleVerb{v1.ListVerb},
			GrantedUsers: []string{authority.GrantedAllUser},
		}},
	}
	h := newTeamsHandler(wsA, wsB, adminRole,
		genUserFixture("admin", []v1.UserRole{v1.SystemAdminRole}, nil, nil),
		genUserFixture("member", []v1.UserRole{v1.DefaultRole}, []string{"ws-a"}, nil),
		genUserFixture("outsider", []v1.UserRole{v1.DefaultRole}, nil, nil))

	cases := map[string][]string{"admin": {"ws-a", "ws-b"}, "member": {"ws-a"}, "outsider": {}}
	for user, want := range cases {
		rsp := callTeamsHandler(h.ListTeamWorkspaces, user, "t1")
		require.Equal(t, http.StatusOK, rsp.Code, user+": "+rsp.Body.String())
		var got view.TeamWorkspacesResponse
		require.NoError(t, json.Unmarshal(rsp.Body.Bytes(), &got))
		ids := []string{}
		for _, item := range got.Items {
			ids = append(ids, item.WorkspaceId)
		}
		require.Equal(t, want, ids, user)
		require.Equal(t, len(want), got.TotalCount, user)
	}
}

func TestGenerateWorkspaceTeams(t *testing.T) {
	h := newTeamsHandler()
	admin := genUserFixture("admin", []v1.UserRole{v1.SystemAdminRole}, nil, nil)
	user := genUserFixture("user", []v1.UserRole{v1.DefaultRole}, nil, nil)
	newReq := func() *view.CreateWorkspaceRequest {
		return &view.CreateWorkspaceRequest{Name: "ws", ClusterId: "c1"}
	}
	ctx := context.Background()

	req := newReq()
	req.Teams = []v1.WorkspaceTeam{{Name: "t1"}, {Name: "t2", Role: v1.WorkspaceTeamMember}}
	ws, err := h.generateWorkspace(ctx, user, req)
	require.NoError(t, err)
	require.Equal(t, req.Teams, ws.Spec.Teams)

	req = newReq()
	req.Teams = []v1.WorkspaceTeam{{Name: "t1", Role: v1.WorkspaceTeamManager}}
	req.AccessPolicy = v1.WorkspaceAccessApproval
	_, err = h.generateWorkspace(ctx, user, req)
	require.True(t, commonerrors.IsForbidden(err), "%v", err)
	ws, err = h.generateWorkspace(ctx, admin, req)
	require.NoError(t, err)
	require.Equal(t, req.Teams, ws.Spec.Teams)
	require.Equal(t, v1.WorkspaceAccessApproval, ws.Spec.AccessPolicy)

	req = newReq()
	req.AccessPolicy = v1.WorkspaceAccessApproval
	_, err = h.generateWorkspace(ctx, user, req)
	require.True(t, commonerrors.IsForbidden(err), "%v", err)
	req.AccessPolicy = v1.WorkspaceAccessClosed
	_, err = h.generateWorkspace(ctx, user, req)
	require.NoError(t, err)

	for _, bad := range []*view.CreateWorkspaceRequest{
		{Name: "ws", ClusterId: "c1", Teams: []v1.WorkspaceTeam{{Name: "Bad_Team"}}},
		{Name: "ws", ClusterId: "c1", Teams: []v1.WorkspaceTeam{{Name: "t1", Role: "owner"}}},
		{Name: "ws", ClusterId: "c1", Teams: []v1.WorkspaceTeam{{Name: "t1"}, {Name: "t1"}}},
		{Name: "ws", ClusterId: "c1", AccessPolicy: "open"},
	} {
		_, err = h.generateWorkspace(ctx, admin, bad)
		require.True(t, commonerrors.IsBadRequest(err), "%v", err)
	}
}

// setTeamsConfig sets a configuration key for the test and restores its previous value afterwards.
func setTeamsConfig(t *testing.T, key string, value any) {
	t.Helper()
	old := viper.Get(key)
	viper.Set(key, value)
	t.Cleanup(func() { viper.Set(key, old) })
}

// useRecordedIdentity sets the auth mode and the SCIM switch for the test.
func useRecordedIdentity(t *testing.T, mode string, scim bool) {
	t.Helper()
	setTeamsConfig(t, "auth.mode", mode)
	setTeamsConfig(t, "auth.scim.enabled", scim)
}

// newChartTeamsHandler builds a handler over a fake client with the chart roles, the team index and
// the interceptor funcs.
func newChartTeamsHandler(t *testing.T, funcs interceptor.Funcs, objs ...client.Object) *Handler {
	t.Helper()
	scheme := runtime.NewScheme()
	require.NoError(t, v1.AddToScheme(scheme))
	base := fake.NewClientBuilder().WithScheme(scheme).WithObjects(append(authtest.ChartRoles(t), objs...)...).
		WithIndex(&v1.Workspace{}, commonworkspace.TeamIndex, commonworkspace.TeamIndexFunc).Build()
	cli := interceptor.NewClient(base, funcs)
	return &Handler{Client: cli, accessController: &authority.AccessController{Client: cli}}
}

// callTeamsBody runs a handler on ctx as the user, with the route name and the JSON body. A non-empty
// actor marks the request as made with a delegated credential of that client.
func callTeamsBody(ctx context.Context, fn gin.HandlerFunc, userId, name, body, actor string) *httptest.ResponseRecorder {
	rsp := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rsp)
	c.Request = httptest.NewRequest(http.MethodPut, "/", strings.NewReader(body)).WithContext(ctx)
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(common.UserId, userId)
	c.Set(common.Name, name)
	if actor != "" {
		c.Set(common.UserType, authority.UserTypeDelegated)
		c.Set(common.ActorClientId, actor)
	}
	fn(c)
	return rsp
}

// boundTeams returns the team bindings stored on the workspace.
func boundTeams(t *testing.T, h *Handler, name string) []v1.WorkspaceTeam {
	t.Helper()
	ws := &v1.Workspace{}
	require.NoError(t, h.Get(context.Background(), client.ObjectKey{Name: name}, ws))
	return ws.Spec.Teams
}

func TestManagerBindingChanged(t *testing.T) {
	member := v1.WorkspaceTeam{Name: "t1"}
	explicitMember := v1.WorkspaceTeam{Name: "t1", Role: v1.WorkspaceTeamMember}
	manager := v1.WorkspaceTeam{Name: "t1", Role: v1.WorkspaceTeamManager}
	other := v1.WorkspaceTeam{Name: "t2", Role: v1.WorkspaceTeamManager}
	for _, tc := range []struct {
		name          string
		current, next []v1.WorkspaceTeam
		want          bool
	}{
		{"nothing", nil, nil, false},
		{"add member", nil, []v1.WorkspaceTeam{member}, false},
		{"remove member", []v1.WorkspaceTeam{member}, nil, false},
		{"empty role equals member", []v1.WorkspaceTeam{member}, []v1.WorkspaceTeam{explicitMember}, false},
		{"add manager", nil, []v1.WorkspaceTeam{manager}, true},
		{"remove manager", []v1.WorkspaceTeam{manager}, nil, true},
		{"promote", []v1.WorkspaceTeam{member}, []v1.WorkspaceTeam{manager}, true},
		{"demote", []v1.WorkspaceTeam{manager}, []v1.WorkspaceTeam{member}, true},
		{"manager kept, member added", []v1.WorkspaceTeam{other}, []v1.WorkspaceTeam{other, member}, false},
		{"duplicate keeps manager", []v1.WorkspaceTeam{manager}, []v1.WorkspaceTeam{member, manager}, false},
	} {
		require.Equal(t, tc.want, managerBindingChanged(tc.current, tc.next), tc.name)
	}
}

// A batch that names one manager binding is refused whole for a workspace manager, with no write.
func TestUpdateTeamWorkspacesPreflightIsAllOrNothing(t *testing.T) {
	gin.SetMode(gin.TestMode)
	writes := 0
	funcs := interceptor.Funcs{
		Update: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.UpdateOption) error {
			writes++
			return c.Update(ctx, obj, opts...)
		},
	}
	h := newChartTeamsHandler(t, funcs, genWorkspaceFixture("ws-a", "A"), genWorkspaceFixture("ws-b", "B"),
		genUserFixture("mgr", []v1.UserRole{v1.DefaultRole}, []string{"ws-a", "ws-b"}, []string{"ws-a", "ws-b"}))
	ctx := context.Background()

	rsp := callTeamsBody(ctx, h.UpdateTeamWorkspaces, "mgr", "t1",
		`[{"workspace":"ws-a"},{"workspace":"ws-b","role":"manager"}]`, "")
	require.Equal(t, http.StatusForbidden, rsp.Code, rsp.Body.String())
	require.Zero(t, writes)
	require.Empty(t, boundTeams(t, h, "ws-a"))

	rsp = callTeamsBody(ctx, h.UpdateTeamWorkspaces, "mgr", "t1", `[{"workspace":"ws-a"},{"workspace":"ws-b"}]`, "")
	require.Equal(t, http.StatusOK, rsp.Code, rsp.Body.String())
	require.Equal(t, 2, writes)
}

// A write that fails part way undoes the bindings already written, also when the request context is
// cancelled by then, and an undone binding gets its previous role back.
func TestUpdateTeamWorkspacesUndoesOnACancelledRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	funcs := interceptor.Funcs{
		Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
			if err := ctx.Err(); err != nil {
				return err
			}
			return c.Get(ctx, key, obj, opts...)
		},
		Update: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.UpdateOption) error {
			if err := ctx.Err(); err != nil {
				return err
			}
			if obj.GetName() == "ws-b" {
				cancel()
				return apierrors.NewInternalError(errors.New("write refused"))
			}
			return c.Update(ctx, obj, opts...)
		},
	}
	wsA := genWorkspaceFixture("ws-a", "A")
	wsA.Spec.Teams = []v1.WorkspaceTeam{{Name: "t1", Role: v1.WorkspaceTeamMember}}
	h := newChartTeamsHandler(t, funcs, wsA, genWorkspaceFixture("ws-b", "B"),
		genUserFixture("admin", []v1.UserRole{v1.SystemAdminRole}, nil, nil))

	rsp := callTeamsBody(ctx, h.UpdateTeamWorkspaces, "admin", "t1",
		`[{"workspace":"ws-a","role":"manager"},{"workspace":"ws-b"}]`, "")
	require.Equal(t, http.StatusInternalServerError, rsp.Code, rsp.Body.String())
	require.Equal(t, []v1.WorkspaceTeam{{Name: "t1", Role: v1.WorkspaceTeamMember}}, boundTeams(t, h, "ws-a"))
	require.Empty(t, boundTeams(t, h, "ws-b"))
}

// A delegated credential may not set the teams or the access policy of a workspace.
func TestDelegatedTokenCannotChangeWorkspaceTeams(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := newChartTeamsHandler(t, interceptor.Funcs{}, genWorkspaceFixture("ws-a", "A"),
		genUserFixture("admin", []v1.UserRole{v1.SystemAdminRole}, nil, nil))
	ctx := context.Background()
	for _, body := range []string{`{"teams":[{"name":"t1"}]}`, `{"accessPolicy":"approval"}`} {
		rsp := callTeamsBody(ctx, h.PatchWorkspace, "admin", "ws-a", body, "agent")
		require.Equal(t, http.StatusForbidden, rsp.Code, body+": "+rsp.Body.String())
	}
	require.Empty(t, boundTeams(t, h, "ws-a"))
	rsp := callTeamsBody(ctx, h.PatchWorkspace, "admin", "ws-a", `{"description":"d"}`, "agent")
	require.Equal(t, http.StatusOK, rsp.Code, rsp.Body.String())
	rsp = callTeamsBody(ctx, h.PatchWorkspace, "admin", "ws-a", `{"teams":[{"name":"t1"}]}`, "")
	require.Equal(t, http.StatusOK, rsp.Code, rsp.Body.String())

	rsp = callTeamsBody(ctx, h.CreateWorkspace, "admin", "",
		`{"name":"ws-new","clusterId":"c1","teams":[{"name":"t1"}]}`, "agent")
	require.Equal(t, http.StatusForbidden, rsp.Code, rsp.Body.String())
}

// recordedTeamsUser returns a user whose record holds the teams and groups SCIM pushed.
func recordedTeamsUser(name string, teams, groups []string) *v1.User {
	user := genUserFixture(name, []v1.UserRole{v1.DefaultRole}, nil, nil)
	identity.RecordTeams(user, teams)
	identity.RecordGroups(user, groups)
	return user
}

// The overlay built from a user record holds the recorded teams and groups only in external mode with SCIM on.
func TestRecordedOverlayNeedsExternalModeAndSCIM(t *testing.T) {
	wsA := genWorkspaceFixture("ws-a", "A")
	wsA.Spec.Teams = []v1.WorkspaceTeam{{Name: "t1"}}
	user := recordedTeamsUser("u1", []string{"t1"}, []string{"g1"})
	h := newChartTeamsHandler(t, interceptor.Funcs{}, wsA, genWorkspaceFixture("ws-g", "G"), user)
	setTeamsConfig(t, "auth.group_mappings", []map[string]any{{"group": "g1", "workspaces": []string{"ws-g"}}})
	for _, tc := range []struct {
		mode string
		scim bool
		want []string
	}{
		{"builtin", true, nil},
		{"external", false, nil},
		{"external", true, []string{"ws-a", "ws-g"}},
	} {
		useRecordedIdentity(t, tc.mode, tc.scim)
		got := h.recordedOverlay(context.Background(), user)
		require.ElementsMatch(t, tc.want, got.Workspaces, "%s scim=%v", tc.mode, tc.scim)
	}
}

// listUserSources lists the users of the workspace as the administrator and returns the sources by user id.
func listUserSources(t *testing.T, h *Handler, workspace string) map[string][]string {
	t.Helper()
	rsp := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rsp)
	c.Request = httptest.NewRequest(http.MethodGet, "/?workspaceId="+workspace, nil)
	c.Set(common.UserId, "admin")
	h.ListUser(c)
	require.Equal(t, http.StatusOK, rsp.Code, rsp.Body.String())
	var got view.ListUserResponse
	require.NoError(t, json.Unmarshal(rsp.Body.Bytes(), &got))
	result := map[string][]string{}
	for _, item := range got.Items {
		result[item.Id] = item.Sources
	}
	return result
}

// The workspace user list shows team members and team sources only in external mode with SCIM on.
func TestListUserTeamSourcesNeedExternalModeAndSCIM(t *testing.T) {
	gin.SetMode(gin.TestMode)
	wsA := genWorkspaceFixture("ws-a", "A")
	wsA.Spec.Teams = []v1.WorkspaceTeam{{Name: "t1"}}
	both := recordedTeamsUser("both", []string{"t1"}, nil)
	both.Spec.Resources = map[string][]string{common.UserWorkspaces: {"ws-a"}}
	h := newChartTeamsHandler(t, interceptor.Funcs{}, wsA, both, recordedTeamsUser("teamonly", []string{"t1"}, nil),
		genUserFixture("admin", []v1.UserRole{v1.SystemAdminRole}, nil, nil))

	useRecordedIdentity(t, "builtin", true)
	require.Equal(t, map[string][]string{"both": {"direct"}}, listUserSources(t, h, "ws-a"))
	useRecordedIdentity(t, "external", false)
	require.Equal(t, map[string][]string{"both": {"direct"}}, listUserSources(t, h, "ws-a"))
	useRecordedIdentity(t, "external", true)
	require.Equal(t, map[string][]string{"both": {"direct", "team:t1"}, "teamonly": {"team:t1"}},
		listUserSources(t, h, "ws-a"))
}

// The platform groups of the member view come from the group mappings, which apply in external mode only.
func TestGroupWorkspaceMembersNeedExternalMode(t *testing.T) {
	setTeamsConfig(t, "auth.group_mappings", []map[string]any{
		{"group": "g1", "workspaces": []string{"ws-a"}},
		{"group": "g2", "workspaces": []string{"ws-a"}, "roles": []string{"workspace-admin"}},
	})
	setTeamsConfig(t, "auth.mode", "builtin")
	require.Empty(t, groupWorkspaceMembers("ws-a"))
	setTeamsConfig(t, "auth.mode", "external")
	require.Equal(t, []view.WorkspaceMemberGroup{
		{Name: "g1", Role: v1.WorkspaceTeamMember, Source: "group:g1"},
		{Name: "g2", Role: v1.WorkspaceTeamManager, Source: "group:g2"},
	}, groupWorkspaceMembers("ws-a"))
}
