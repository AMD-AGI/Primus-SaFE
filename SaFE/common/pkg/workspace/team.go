/*
 * Copyright (C) 2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package workspace

import (
	"context"
	"sync"
	"time"

	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

// teamLookupLogInterval is the least time between two logs of failed team lookups.
const teamLookupLogInterval = time.Minute

// teamLookupLog counts the failed team lookups since the last one logged.
var teamLookupLog struct {
	sync.Mutex
	last       time.Time
	suppressed int
}

// LogTeamLookupError logs a failed read of the workspaces bound to a team at most once per
// teamLookupLogInterval, with the number of failures left unlogged since the previous log.
func LogTeamLookupError(err error, team string) {
	teamLookupLog.Lock()
	now := time.Now()
	if now.Sub(teamLookupLog.last) < teamLookupLogInterval {
		teamLookupLog.suppressed++
		teamLookupLog.Unlock()
		return
	}
	suppressed := teamLookupLog.suppressed
	teamLookupLog.last, teamLookupLog.suppressed = now, 0
	teamLookupLog.Unlock()
	klog.ErrorS(err, "failed to list the workspaces bound to a team, the team grants none",
		"team", team, "suppressed", suppressed)
}

// TeamIndex is the cache field index of workspaces by the names of the teams bound to them.
const TeamIndex = "spec.teams.name"

// TeamIndexFunc returns the names of the teams bound to a workspace, the values of TeamIndex.
func TeamIndexFunc(obj client.Object) []string {
	workspace, ok := obj.(*v1.Workspace)
	if !ok || len(workspace.Spec.Teams) == 0 {
		return nil
	}
	names := make([]string, 0, len(workspace.Spec.Teams))
	seen := make(map[string]bool, len(workspace.Spec.Teams))
	for _, t := range workspace.Spec.Teams {
		if t.Name == "" || seen[t.Name] {
			continue
		}
		seen[t.Name] = true
		names = append(names, t.Name)
	}
	return names
}

// SetupTeamIndex registers TeamIndex on the field indexer. It must run before the cache starts.
func SetupTeamIndex(ctx context.Context, indexer client.FieldIndexer) error {
	return indexer.IndexField(ctx, &v1.Workspace{}, TeamIndex, TeamIndexFunc)
}

// ListByTeam returns the workspaces bound to the team, read through TeamIndex.
func ListByTeam(ctx context.Context, reader client.Reader, team string) ([]v1.Workspace, error) {
	if team == "" {
		return nil, nil
	}
	list := &v1.WorkspaceList{}
	if err := reader.List(ctx, list, client.MatchingFields{TeamIndex: team}); err != nil {
		return nil, err
	}
	return list.Items, nil
}

// TeamWorkspaces returns the workspaces bound to the team and the role of the team members in each.
func TeamWorkspaces(ctx context.Context, reader client.Reader, team string) (map[string]v1.WorkspaceTeamRole, error) {
	workspaces, err := ListByTeam(ctx, reader, team)
	if err != nil {
		return nil, err
	}
	result := make(map[string]v1.WorkspaceTeamRole, len(workspaces))
	for i := range workspaces {
		if role, ok := workspaces[i].TeamRole(team); ok {
			result[workspaces[i].Name] = role
		}
	}
	return result, nil
}
