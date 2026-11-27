/*
 * Copyright (C) 2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package scim

import (
	"context"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/resources"
	commonworkspace "github.com/AMD-AIG-AIMA/SAFE/common/pkg/workspace"
)

// teamWorkspace returns a workspace with the team bindings.
func teamWorkspace(name string, teams ...v1.WorkspaceTeam) *v1.Workspace {
	ws := &v1.Workspace{}
	ws.Name = name
	ws.Spec.Teams = teams
	return ws
}

// A deleted team is unbound from every workspace; other bindings stay, and an unknown team is 204.
func TestSCIMDeleteTeamUnbindsWorkspaces(t *testing.T) {
	gin.SetMode(gin.TestMode)
	scheme := runtime.NewScheme()
	require.NoError(t, v1.AddToScheme(scheme))
	cli := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(
			teamWorkspace("ws-a", v1.WorkspaceTeam{Name: "t1"}, v1.WorkspaceTeam{Name: "t2", Role: v1.WorkspaceTeamManager}),
			teamWorkspace("ws-b", v1.WorkspaceTeam{Name: "t1", Role: v1.WorkspaceTeamManager}),
			teamWorkspace("ws-c", v1.WorkspaceTeam{Name: "t3"}),
		).
		WithIndex(&v1.Workspace{}, commonworkspace.TeamIndex, commonworkspace.TeamIndexFunc).Build()
	engine := gin.New()
	Mount(engine, &resources.Handler{Client: cli}, NewToken("secret"))

	require.Equal(t, http.StatusUnauthorized, scimDo(engine, http.MethodDelete, "/scim/v2/Teams/t1", "", "").Code)
	require.Equal(t, http.StatusBadRequest, scimDo(engine, http.MethodDelete, "/scim/v2/Teams/Bad_Team", "", "secret").Code)

	rsp := scimDo(engine, http.MethodDelete, "/scim/v2/Teams/t1", "", "secret")
	require.Equal(t, http.StatusNoContent, rsp.Code, rsp.Body.String())

	teams := func(name string) []v1.WorkspaceTeam {
		ws := &v1.Workspace{}
		require.NoError(t, cli.Get(context.Background(), client.ObjectKey{Name: name}, ws))
		return ws.Spec.Teams
	}
	require.Equal(t, []v1.WorkspaceTeam{{Name: "t2", Role: v1.WorkspaceTeamManager}}, teams("ws-a"))
	require.Empty(t, teams("ws-b"))
	require.Equal(t, []v1.WorkspaceTeam{{Name: "t3"}}, teams("ws-c"))

	require.Equal(t, http.StatusNoContent, scimDo(engine, http.MethodDelete, "/scim/v2/Teams/t1", "", "secret").Code)
	require.Equal(t, http.StatusNoContent, scimDo(engine, http.MethodDelete, "/scim/v2/Teams/unknown", "", "secret").Code)
}
