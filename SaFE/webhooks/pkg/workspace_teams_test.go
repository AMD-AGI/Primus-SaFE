/*
 * Copyright (C) 2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package webhooks

import (
	"testing"

	"github.com/stretchr/testify/require"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

func TestValidateTeams(t *testing.T) {
	for _, tc := range []struct {
		name   string
		teams  []v1.WorkspaceTeam
		policy v1.WorkspaceAccessPolicy
		errMsg string
	}{
		{name: "empty"},
		{name: "member and manager", teams: []v1.WorkspaceTeam{{Name: "t1"}, {Name: "t2", Role: v1.WorkspaceTeamManager}},
			policy: v1.WorkspaceAccessApproval},
		{name: "closed policy", policy: v1.WorkspaceAccessClosed},
		{name: "invalid name", teams: []v1.WorkspaceTeam{{Name: "Bad_Team"}}, errMsg: "invalid team name"},
		{name: "prefixed name", teams: []v1.WorkspaceTeam{{Name: "team:t1"}}, errMsg: "invalid team name"},
		{name: "empty name", teams: []v1.WorkspaceTeam{{Name: ""}}, errMsg: "invalid team name"},
		{name: "duplicate", teams: []v1.WorkspaceTeam{{Name: "t1"}, {Name: "t1", Role: v1.WorkspaceTeamManager}},
			errMsg: "more than once"},
		{name: "invalid role", teams: []v1.WorkspaceTeam{{Name: "t1", Role: "owner"}}, errMsg: "invalid role"},
		{name: "invalid policy", policy: "open", errMsg: "invalid accessPolicy"},
	} {
		ws := &v1.Workspace{Spec: v1.WorkspaceSpec{Teams: tc.teams, AccessPolicy: tc.policy}}
		err := validateTeams(ws)
		if tc.errMsg == "" {
			require.NoError(t, err, tc.name)
			continue
		}
		require.ErrorContains(t, err, tc.errMsg, tc.name)
	}
}
