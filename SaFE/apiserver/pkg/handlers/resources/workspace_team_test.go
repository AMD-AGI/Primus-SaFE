/*
 * Copyright (C) 2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resources

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/extauth"
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
