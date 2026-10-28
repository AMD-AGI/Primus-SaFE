/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resources

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/extauth"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/resources/view"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
)

func genWorkspaceFixture(id, displayName string) *v1.Workspace {
	return &v1.Workspace{ObjectMeta: metav1.ObjectMeta{
		Name:   id,
		Labels: map[string]string{v1.DisplayNameLabel: displayName},
	}}
}

func genUserFixture(id string, roles []v1.UserRole, workspaces, managed []string) *v1.User {
	user := &v1.User{
		ObjectMeta: metav1.ObjectMeta{
			Name:        id,
			Labels:      map[string]string{v1.UserIdLabel: id},
			Annotations: map[string]string{v1.UserNameAnnotation: id},
		},
		Spec: v1.UserSpec{Type: v1.DefaultUserType, Roles: roles},
	}
	if len(workspaces) > 0 || len(managed) > 0 {
		user.Spec.Resources = map[string][]string{}
		if len(workspaces) > 0 {
			user.Spec.Resources[common.UserWorkspaces] = workspaces
		}
		if len(managed) > 0 {
			user.Spec.Resources[common.UserManagedWorkspaces] = managed
		}
	}
	return user
}

func newWorkspacesHandler(objs ...client.Object) *Handler {
	scheme := runtime.NewScheme()
	_ = v1.AddToScheme(scheme)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build()
	// The controller is built for this client, not taken from NewAccessController: that
	// one is a process-wide singleton that keeps the first client it was given, and a test
	// here must neither read another test's users nor leave its own behind.
	return &Handler{
		Client:           fakeClient,
		accessController: &authority.AccessController{Client: fakeClient},
	}
}

// getSelfWorkspaces runs the handler the way the router does: the caller is the user
// the authentication layer put in the context, and the overlay (if any) rides on the
// request context.
func getSelfWorkspaces(h *Handler, userId string, overlay *extauth.Overlay) *httptest.ResponseRecorder {
	rsp := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rsp)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	if overlay != nil {
		c.Request = c.Request.WithContext(extauth.WithOverlay(c.Request.Context(), overlay))
	}
	if userId != "" {
		c.Set(common.UserId, userId)
	}
	h.GetSelfWorkspaces(c)
	return rsp
}

func TestGetSelfWorkspaces(t *testing.T) {
	gin.SetMode(gin.TestMode)
	wsA := genWorkspaceFixture("ws-a", "Team A")
	wsB := genWorkspaceFixture("ws-b", "Team B")
	wsC := genWorkspaceFixture("ws-c", "Team C")

	tests := []struct {
		name        string
		user        *v1.User
		overlay     *extauth.Overlay
		wantCode    int
		wantWs      []view.WorkspaceEntry
		wantManaged []view.WorkspaceEntry
	}{
		{
			name:        "access and management are listed apart, with display names",
			user:        genUserFixture("u1", []v1.UserRole{v1.DefaultRole}, []string{"ws-a", "ws-b"}, []string{"ws-b"}),
			wantCode:    http.StatusOK,
			wantWs:      []view.WorkspaceEntry{{Id: "ws-a", Name: "Team A"}, {Id: "ws-b", Name: "Team B"}},
			wantManaged: []view.WorkspaceEntry{{Id: "ws-b", Name: "Team B"}},
		},
		{
			name:        "a workspace that no longer exists is left out",
			user:        genUserFixture("u1", []v1.UserRole{v1.DefaultRole}, []string{"ws-a", "ws-gone"}, []string{"ws-gone"}),
			wantCode:    http.StatusOK,
			wantWs:      []view.WorkspaceEntry{{Id: "ws-a", Name: "Team A"}},
			wantManaged: []view.WorkspaceEntry{},
		},
		{
			name:        "a user without any workspace gets empty lists, not null",
			user:        genUserFixture("u1", []v1.UserRole{v1.DefaultRole}, nil, nil),
			wantCode:    http.StatusOK,
			wantWs:      []view.WorkspaceEntry{},
			wantManaged: []view.WorkspaceEntry{},
		},
		{
			name:        "a system administrator is not listed in any workspace",
			user:        genUserFixture("u1", []v1.UserRole{v1.SystemAdminRole}, []string{"ws-a"}, []string{"ws-a"}),
			wantCode:    http.StatusOK,
			wantWs:      []view.WorkspaceEntry{},
			wantManaged: []view.WorkspaceEntry{},
		},
		{
			name:        "a read-only administrator keeps the workspaces it was granted",
			user:        genUserFixture("u1", []v1.UserRole{v1.SystemAdminReadonlyRole}, []string{"ws-a"}, nil),
			wantCode:    http.StatusOK,
			wantWs:      []view.WorkspaceEntry{{Id: "ws-a", Name: "Team A"}},
			wantManaged: []view.WorkspaceEntry{},
		},
		{
			// External mode: the record holds nothing, the groups of the request map to a
			// managed workspace. The lists must show it, or a workspace administrator that
			// external authentication named would see no management entry.
			name:        "the request's group mapping is part of the lists",
			user:        genUserFixture("u1", []v1.UserRole{v1.DefaultRole}, []string{"ws-a"}, nil),
			overlay:     &extauth.Overlay{UserId: "u1", Workspaces: []string{"ws-c"}, Managed: []string{"ws-c"}},
			wantCode:    http.StatusOK,
			wantWs:      []view.WorkspaceEntry{{Id: "ws-a", Name: "Team A"}, {Id: "ws-c", Name: "Team C"}},
			wantManaged: []view.WorkspaceEntry{{Id: "ws-c", Name: "Team C"}},
		},
		{
			name:        "an overlay of another user adds nothing",
			user:        genUserFixture("u1", []v1.UserRole{v1.DefaultRole}, nil, nil),
			overlay:     &extauth.Overlay{UserId: "someone-else", Workspaces: []string{"ws-c"}, Managed: []string{"ws-c"}},
			wantCode:    http.StatusOK,
			wantWs:      []view.WorkspaceEntry{},
			wantManaged: []view.WorkspaceEntry{},
		},
		{
			name:        "a group mapped to the administrator role empties the lists",
			user:        genUserFixture("u1", []v1.UserRole{v1.DefaultRole}, []string{"ws-a"}, nil),
			overlay:     &extauth.Overlay{UserId: "u1", Roles: []v1.UserRole{v1.SystemAdminRole}},
			wantCode:    http.StatusOK,
			wantWs:      []view.WorkspaceEntry{},
			wantManaged: []view.WorkspaceEntry{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newWorkspacesHandler(tt.user, wsA, wsB, wsC)
			rsp := getSelfWorkspaces(h, tt.user.Name, tt.overlay)
			require.Equal(t, tt.wantCode, rsp.Code, rsp.Body.String())

			var got view.UserWorkspacesResponse
			require.NoError(t, json.Unmarshal(rsp.Body.Bytes(), &got))
			require.Equal(t, tt.wantWs, got.Workspaces)
			require.Equal(t, tt.wantManaged, got.ManagedWorkspaces)
			// Both fields are always arrays on the wire.
			require.NotContains(t, rsp.Body.String(), "null")
		})
	}
}

// The overlay is applied to a copy: serving the lists never writes a group mapping
// into the stored user.
func TestGetSelfWorkspacesDoesNotStoreTheOverlay(t *testing.T) {
	gin.SetMode(gin.TestMode)
	user := genUserFixture("u1", []v1.UserRole{v1.DefaultRole}, nil, nil)
	h := newWorkspacesHandler(user, genWorkspaceFixture("ws-c", "Team C"))
	rsp := getSelfWorkspaces(h, "u1", &extauth.Overlay{UserId: "u1", Workspaces: []string{"ws-c"}, Managed: []string{"ws-c"}})
	require.Equal(t, http.StatusOK, rsp.Code, rsp.Body.String())

	stored := &v1.User{}
	require.NoError(t, h.Get(context.Background(), client.ObjectKey{Name: "u1"}, stored))
	require.Empty(t, stored.Spec.Resources)
}

func TestGetSelfWorkspacesRefusesAnUnknownCaller(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := newWorkspacesHandler(genUserFixture("u1", []v1.UserRole{v1.DefaultRole}, nil, nil))

	t.Run("no user in the request", func(t *testing.T) {
		rsp := getSelfWorkspaces(h, "", nil)
		require.Equal(t, http.StatusUnauthorized, rsp.Code, rsp.Body.String())
	})
	t.Run("a user that has no record", func(t *testing.T) {
		rsp := getSelfWorkspaces(h, "nobody", nil)
		require.NotEqual(t, http.StatusOK, rsp.Code, rsp.Body.String())
		require.NotContains(t, rsp.Body.String(), "workspaces")
	})
}

// The lists of the user record and of the new route come from one function, so a client
// that moves from the first to the second sees the same workspaces.
func TestSelfWorkspacesMatchTheUserRecord(t *testing.T) {
	gin.SetMode(gin.TestMode)
	user := genUserFixture("u1", []v1.UserRole{v1.DefaultRole}, []string{"ws-a", "ws-b"}, []string{"ws-b"})
	h := newWorkspacesHandler(user, genWorkspaceFixture("ws-a", "Team A"), genWorkspaceFixture("ws-b", "Team B"))

	item := h.cvtToUserResponseItem(context.Background(), user)
	require.Equal(t, []view.WorkspaceEntry{{Id: "ws-a", Name: "Team A"}, {Id: "ws-b", Name: "Team B"}}, item.Workspaces)
	require.Equal(t, []view.WorkspaceEntry{{Id: "ws-b", Name: "Team B"}}, item.ManagedWorkspaces)

	rsp := getSelfWorkspaces(h, "u1", nil)
	var got view.UserWorkspacesResponse
	require.NoError(t, json.Unmarshal(rsp.Body.Bytes(), &got))
	require.Equal(t, item.Workspaces, got.Workspaces)
	require.Equal(t, item.ManagedWorkspaces, got.ManagedWorkspaces)
}

// The user record keeps its shape for a user without workspaces: the fields stay null
// there, as they always were, so existing clients are not affected by the new route.
func TestUserRecordKeepsNullListsWithoutWorkspaces(t *testing.T) {
	user := genUserFixture("u1", []v1.UserRole{v1.DefaultRole}, nil, nil)
	h := newWorkspacesHandler(user)
	data, err := json.Marshal(h.cvtToUserResponseItem(context.Background(), user))
	require.NoError(t, err)
	require.Contains(t, string(data), `"workspaces":null`)
	require.Contains(t, string(data), `"managedWorkspaces":null`)
}

// The new route sits next to /users/:name and /users/:name/settings in the route table.
// The three must keep resolving to their own handlers, and the route serves the caller
// only: another user's id is not a path of it.
func TestSelfWorkspacesRoute(t *testing.T) {
	gin.SetMode(gin.TestMode)
	real := gin.New()
	InitCustomRouters(real, &Handler{})

	// Rebuild the users routes of the real table on a bare engine whose handlers say
	// which route pattern answered, so the check needs no authentication.
	root := "/" + common.PrimusRouterCustomRootPath + "/users"
	probe := gin.New()
	registered := 0
	for _, r := range real.Routes() {
		if r.Method != http.MethodGet || !strings.HasPrefix(r.Path, root) {
			continue
		}
		pattern := r.Path
		probe.GET(pattern, func(c *gin.Context) { c.String(http.StatusOK, pattern) })
		registered++
	}
	require.Positive(t, registered, "no users route found: the test would pass vacuously")

	for _, tt := range []struct {
		path     string
		wantCode int
		want     string
	}{
		{root + "/self", http.StatusOK, root + "/:name"},
		{root + "/u1", http.StatusOK, root + "/:name"},
		{root + "/self/settings", http.StatusOK, root + "/:name/settings"},
		{root + "/u1/settings", http.StatusOK, root + "/:name/settings"},
		{root + "/self/workspaces", http.StatusOK, root + "/self/workspaces"},
		{root + "/u1/workspaces", http.StatusNotFound, ""},
	} {
		rsp := httptest.NewRecorder()
		probe.ServeHTTP(rsp, httptest.NewRequest(http.MethodGet, tt.path, nil))
		require.Equal(t, tt.wantCode, rsp.Code, tt.path)
		if tt.wantCode == http.StatusOK {
			require.Equal(t, tt.want, strings.TrimSpace(rsp.Body.String()), tt.path)
		}

		// The real engine registered the routes in the router's own order: a path that
		// has a route is stopped by the authentication middleware, one without is a 404.
		rsp = httptest.NewRecorder()
		real.ServeHTTP(rsp, httptest.NewRequest(http.MethodGet, tt.path, nil))
		if tt.wantCode == http.StatusNotFound {
			require.Equal(t, http.StatusNotFound, rsp.Code, "real engine: "+tt.path)
		} else {
			require.NotEqual(t, http.StatusNotFound, rsp.Code, "real engine: "+tt.path)
		}
	}
}

// A delegated token acts as one user and may read that user's workspaces; the route
// table test in routes_delegated_test.go skips the "self" routes, so this pins that
// one: it is reachable with a delegated read-only scope and is not a write.
func TestSelfWorkspacesIsReadableByDelegatedTokens(t *testing.T) {
	path := "/" + common.PrimusRouterCustomRootPath + "/users/self/workspaces"
	_, forbidden := extauth.Forbidden(http.MethodGet, path)
	require.False(t, forbidden)

	// Only GET is registered for it, so a delegated write cannot reach anything here.
	gin.SetMode(gin.TestMode)
	e := gin.New()
	InitCustomRouters(e, &Handler{})
	var methods []string
	for _, r := range e.Routes() {
		if r.Path == path {
			methods = append(methods, r.Method)
		}
	}
	require.Equal(t, []string{http.MethodGet}, methods)
}
