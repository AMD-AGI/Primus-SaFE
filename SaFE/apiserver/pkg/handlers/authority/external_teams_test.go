/*
 * Copyright (C) 2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package authority

import (
	"context"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/extauth"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/identity"
	commonworkspace "github.com/AMD-AIG-AIMA/SAFE/common/pkg/workspace"
)

// teamsCluster returns a fake cluster with the team index holding the objects.
func teamsCluster(t *testing.T, objs ...client.Object) client.Client {
	t.Helper()
	scheme := runtime.NewScheme()
	require.NoError(t, v1.AddToScheme(scheme))
	return fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).
		WithIndex(&v1.Workspace{}, commonworkspace.TeamIndex, commonworkspace.TeamIndexFunc).Build()
}

// A group mapping whose group carries the team prefix is refused when external authentication starts.
func TestInitExternalAuthRefusesTeamPrefixedGroupMappings(t *testing.T) {
	cli := teamsCluster(t)
	useBuiltinChain(t, cli)
	viper.Set("auth.group_mappings", []map[string]any{{"group": "team:t1", "workspaces": []string{"ws-a"}}})
	require.ErrorContains(t, InitExternalAuth(cli), "team:")
	viper.Set("auth.group_mappings", []map[string]any{{"group": "teams", "workspaces": []string{"ws-a"}}})
	require.NoError(t, InitExternalAuth(cli))
}

// The recorded teams of a user grant the workspaces bound to them, with the default role, in external
// mode with SCIM on; with SCIM off and in builtin mode they grant nothing.
func TestWithRecordedGroupsAddsTheRecordedTeams(t *testing.T) {
	ws := &v1.Workspace{}
	ws.Name = "ws-a"
	ws.Spec.Teams = []v1.WorkspaceTeam{{Name: "t1", Role: v1.WorkspaceTeamManager}}
	user := &v1.User{}
	user.Name = "alice-id"
	identity.RecordTeams(user, []string{"t1"})
	cli := teamsCluster(t, ws)

	clearAuthState()
	t.Cleanup(clearAuthState)
	require.Nil(t, extauth.RequestOverlay(WithRecordedGroups(context.Background(), user), user), "builtin mode")

	useBuiltinChain(t, cli)
	t.Cleanup(func() { viper.Set("auth.scim.enabled", nil) })
	viper.Set("auth.scim.enabled", true)
	o := extauth.RequestOverlay(WithRecordedGroups(context.Background(), user), user)
	require.NotNil(t, o)
	require.Equal(t, []string{"ws-a"}, o.Workspaces)
	require.Equal(t, []string{"ws-a"}, o.Managed)
	require.Equal(t, []v1.UserRole{v1.DefaultRole}, o.Roles)

	viper.Set("auth.scim.enabled", false)
	o = extauth.RequestOverlay(WithRecordedGroups(context.Background(), user), user)
	require.NotNil(t, o)
	require.Empty(t, o.Workspaces)
}
