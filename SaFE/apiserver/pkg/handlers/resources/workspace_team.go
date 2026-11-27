/*
 * Copyright (C) 2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resources

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/klog/v2"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/extauth"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/resources/view"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/identity"
	apiutils "github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/utils"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	commonuser "github.com/AMD-AIG-AIMA/SAFE/common/pkg/user"
	commonworkspace "github.com/AMD-AIG-AIMA/SAFE/common/pkg/workspace"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/backoff"
	jsonutils "github.com/AMD-AIG-AIMA/SAFE/utils/pkg/json"
)

// sourceDirect, sourceDefault and the extauth prefixes are the values of the sources field.
const (
	sourceDirect  = "direct"
	sourceDefault = "default"
)

// teamUndoTimeout bounds the undo of a partly applied team to workspaces update, which runs after
// the request context may have ended.
const teamUndoTimeout = 30 * time.Second

// UpdateWorkspaceTeams adds and removes the team bindings of a workspace.
// A workspace manager may change member bindings; any change to a manager binding needs a system administrator.
func (h *Handler) UpdateWorkspaceTeams(c *gin.Context) {
	handle(c, h.updateWorkspaceTeams)
}

// UpdateTeamWorkspaces binds a team to several workspaces at once. Every entry is checked before any
// is written; when a write fails, the bindings already written are undone where no other request
// changed them since.
func (h *Handler) UpdateTeamWorkspaces(c *gin.Context) {
	handle(c, h.updateTeamWorkspaces)
}

// ListTeamWorkspaces lists the workspaces a team is bound to, with the role of its members in each.
func (h *Handler) ListTeamWorkspaces(c *gin.Context) {
	handle(c, h.listTeamWorkspaces)
}

// ListTeams lists the teams of the identity service for the team selector. It answers an empty
// list whenever the identity service cannot be asked.
func (h *Handler) ListTeams(c *gin.Context) {
	handle(c, h.listTeams)
}

// ListTeamMembers lists the members of a team of the identity service. It answers system
// administrators and the members of the team.
func (h *Handler) ListTeamMembers(c *gin.Context) {
	handle(c, h.listTeamMembers)
}

// ListWorkspaceMembers returns the effective member view of a workspace: direct users, bound teams
// and platform groups, each tagged with its source.
func (h *Handler) ListWorkspaceMembers(c *gin.Context) {
	handle(c, h.listWorkspaceMembers)
}

// GetUserWorkspaces returns the effective workspaces of a user with the sources of each.
// It answers system administrators and, for the workspaces they manage, workspace managers.
func (h *Handler) GetUserWorkspaces(c *gin.Context) {
	handle(c, h.getUserWorkspaces)
}

// updateWorkspaceTeams implements PUT /workspaces/:name/teams.
func (h *Handler) updateWorkspaceTeams(c *gin.Context) (interface{}, error) {
	requestUser, err := h.getAndSetUsername(c)
	if err != nil {
		return nil, err
	}
	req := &view.UpdateWorkspaceTeamsRequest{}
	body, err := apiutils.ParseRequestBody(c.Request, req)
	if err != nil {
		klog.ErrorS(err, "failed to parse request", "body", string(body))
		return nil, err
	}
	if err = validateWorkspaceTeamsRequest(req); err != nil {
		return nil, err
	}
	name := c.GetString(common.Name)
	updated, _, err := h.updateWorkspaceTeamList(c.Request.Context(), requestUser, name,
		func(current []v1.WorkspaceTeam) ([]v1.WorkspaceTeam, error) {
			next := current
			for _, team := range req.Remove {
				next = setTeamBinding(next, v1.WorkspaceTeam{Name: team}, false)
			}
			for _, team := range req.Add {
				next = setTeamBinding(next, team, true)
			}
			return next, nil
		})
	if err != nil {
		klog.ErrorS(err, "failed to update workspace teams", "name", name)
		return nil, err
	}
	klog.Infof("update workspace teams, name: %s, request: %s", name, string(jsonutils.MarshalSilently(*req)))
	teams := updated.Spec.Teams
	if teams == nil {
		teams = []v1.WorkspaceTeam{}
	}
	return &view.WorkspaceTeamsResponse{Teams: teams}, nil
}

// updateTeamWorkspaces implements PUT /teams/:name/workspaces. Every entry is checked first, so a
// refused entry leaves all workspaces untouched. When a write fails, undoTeamWorkspaces restores the
// bindings this request wrote, and the error names the bindings that stayed.
func (h *Handler) updateTeamWorkspaces(c *gin.Context) (interface{}, error) {
	requestUser, err := h.getAndSetUsername(c)
	if err != nil {
		return nil, err
	}
	team := c.GetString(common.Name)
	if err = validateTeamName(team); err != nil {
		return nil, err
	}
	var req []view.TeamWorkspaceBinding
	body, err := apiutils.ParseRequestBody(c.Request, &req)
	if err != nil {
		klog.ErrorS(err, "failed to parse request", "body", string(body))
		return nil, err
	}
	if err = validateTeamWorkspaceBindings(req); err != nil {
		return nil, err
	}
	ctx := c.Request.Context()
	bindingOf := func(entry view.TeamWorkspaceBinding) v1.WorkspaceTeam {
		return v1.WorkspaceTeam{Name: team, Role: entry.Role}
	}
	changeOf := func(entry view.TeamWorkspaceBinding) teamsChange {
		return func(current []v1.WorkspaceTeam) ([]v1.WorkspaceTeam, error) {
			return setTeamBinding(current, bindingOf(entry), true), nil
		}
	}
	for _, entry := range req {
		if _, _, err = h.planWorkspaceTeams(ctx, requestUser, entry.WorkspaceId, changeOf(entry)); err != nil {
			return nil, err
		}
	}

	var applied []teamBindingWrite
	result := &view.TeamWorkspacesResponse{TeamId: team, Items: []view.TeamWorkspaceItem{}}
	for _, entry := range req {
		updated, before, applyErr := h.updateWorkspaceTeamList(ctx, requestUser, entry.WorkspaceId, changeOf(entry))
		if applyErr != nil {
			klog.ErrorS(applyErr, "failed to bind team to workspace", "team", team, "workspace", entry.WorkspaceId)
			return nil, h.undoTeamWorkspaces(ctx, requestUser, team, applied, applyErr)
		}
		if !slices.Equal(before, updated.Spec.Teams) {
			previous := &v1.Workspace{Spec: v1.WorkspaceSpec{Teams: before}}
			role, bound := previous.TeamRole(team)
			applied = append(applied, teamBindingWrite{
				workspace: entry.WorkspaceId, role: role, bound: bound, written: bindingOf(entry).EffectiveRole(),
			})
		}
		result.Items = append(result.Items, view.TeamWorkspaceItem{
			WorkspaceId:   updated.Name,
			WorkspaceName: v1.GetDisplayName(updated),
			Role:          bindingOf(entry).EffectiveRole(),
		})
	}
	result.TotalCount = len(result.Items)
	klog.Infof("bind team to workspaces, team: %s, request: %s", team, string(jsonutils.MarshalSilently(req)))
	return result, nil
}

// teamBindingWrite is a team binding a team to workspaces update wrote: the workspace, the binding
// it had before (role and bound) and the role written.
type teamBindingWrite struct {
	workspace string
	role      v1.WorkspaceTeamRole
	bound     bool
	written   v1.WorkspaceTeamRole
}

// undoTeamWorkspaces restores, newest first, the bindings the update wrote, each only while the
// workspace still holds the binding as written; a binding another request changed since is left
// as it is. It returns applyErr, naming the workspaces where the team stayed bound otherwise than
// before the update.
func (h *Handler) undoTeamWorkspaces(ctx context.Context, requestUser *v1.User, team string,
	applied []teamBindingWrite, applyErr error) error {
	undoCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), teamUndoTimeout)
	defer cancel()
	var stayed []string
	for i := len(applied) - 1; i >= 0; i-- {
		undo := applied[i]
		var role v1.WorkspaceTeamRole
		var bound bool
		_, _, undoErr := h.updateWorkspaceTeamList(undoCtx, requestUser, undo.workspace,
			func(current []v1.WorkspaceTeam) ([]v1.WorkspaceTeam, error) {
				role, bound = (&v1.Workspace{Spec: v1.WorkspaceSpec{Teams: current}}).TeamRole(team)
				if !bound || role != undo.written {
					return current, nil
				}
				return setTeamBinding(current, v1.WorkspaceTeam{Name: team, Role: undo.role}, undo.bound), nil
			})
		switch {
		case undoErr != nil:
			klog.ErrorS(undoErr, "failed to undo team binding", "team", team, "workspace", undo.workspace)
			stayed = append(stayed, fmt.Sprintf("%s (%s)", undo.workspace, undo.written))
		case !bound || role != undo.written:
			klog.InfoS("team binding changed by another request, left as it is", "team", team,
				"workspace", undo.workspace, "bound", bound, "role", role)
			if bound && (!undo.bound || role != undo.role) {
				stayed = append(stayed, fmt.Sprintf("%s (%s)", undo.workspace, role))
			}
		}
	}
	if len(stayed) == 0 {
		return applyErr
	}
	sort.Strings(stayed)
	suffix := fmt.Sprintf("; the team %s stayed bound to: %s", team, strings.Join(stayed, ", "))
	var status apierrors.APIStatus
	if errors.As(applyErr, &status) {
		s := status.Status()
		s.Message += suffix
		return &apierrors.StatusError{ErrStatus: s}
	}
	return commonerrors.NewInternalError(applyErr.Error() + suffix)
}

// listTeamWorkspaces implements GET /teams/:name/workspaces, listing the bound workspaces the caller may list.
func (h *Handler) listTeamWorkspaces(c *gin.Context) (interface{}, error) {
	requestUser, err := h.getAndSetUsername(c)
	if err != nil {
		return nil, err
	}
	team := c.GetString(common.Name)
	if team == "" {
		return nil, commonerrors.NewBadRequest("the team is empty")
	}
	ctx := c.Request.Context()
	workspaces, err := commonworkspace.ListByTeam(ctx, h.Client, team)
	if err != nil {
		return nil, err
	}
	sort.Slice(workspaces, func(i, j int) bool { return workspaces[i].Name < workspaces[j].Name })
	roles := h.accessController.GetRoles(ctx, requestUser)
	result := &view.TeamWorkspacesResponse{TeamId: team, Items: []view.TeamWorkspaceItem{}}
	for i := range workspaces {
		role, ok := workspaces[i].TeamRole(team)
		if !ok {
			continue
		}
		allowed, err := h.accessController.Allowed(authority.AccessInput{
			Context:      ctx,
			Resource:     &workspaces[i],
			ResourceKind: v1.WorkspaceKind,
			Verb:         v1.ListVerb,
			Workspaces:   []string{workspaces[i].Name},
			User:         requestUser,
			Roles:        roles,
		})
		if err != nil {
			return nil, err
		}
		if !allowed {
			continue
		}
		result.Items = append(result.Items, view.TeamWorkspaceItem{
			WorkspaceId:   workspaces[i].Name,
			WorkspaceName: v1.GetDisplayName(&workspaces[i]),
			Role:          role,
		})
	}
	result.TotalCount = len(result.Items)
	return result, nil
}

// listTeams implements GET /teams for any signed-in user who is not restricted.
func (h *Handler) listTeams(c *gin.Context) (interface{}, error) {
	requestUser, err := h.getAndSetUsername(c)
	if err != nil {
		return nil, err
	}
	if requestUser == nil {
		return nil, commonerrors.NewUnauthorized("the request user is unknown")
	}
	if requestUser.IsRestricted() {
		return nil, commonerrors.NewForbidden(
			fmt.Sprintf("The user is restricted. type: %d", requestUser.Spec.RestrictedType))
	}
	result := &view.ListTeamResponse{Items: []view.TeamItem{}}
	if commonconfig.GetAuthMode() == commonconfig.AuthModeExternal {
		teams, err := identity.ListTeams(c.Request.Context())
		if err != nil {
			klog.V(4).InfoS("teams of the identity service are not available", "err", err)
		}
		for _, info := range teams {
			result.Items = append(result.Items, view.TeamItem{
				Id:          info.Id,
				DisplayName: info.DisplayName,
				Description: info.Description,
				MemberCount: info.MemberCount,
				Managers:    cvtTeamMembers(info.Managers),
			})
		}
		sort.Slice(result.Items, func(i, j int) bool { return result.Items[i].Id < result.Items[j].Id })
	}
	result.TotalCount = len(result.Items)
	return result, nil
}

// listTeamMembers implements GET /teams/:name/members.
func (h *Handler) listTeamMembers(c *gin.Context) (interface{}, error) {
	requestUser, err := h.getAndSetUsername(c)
	if err != nil {
		return nil, err
	}
	team := c.GetString(common.Name)
	if err = validateTeamName(team); err != nil {
		return nil, err
	}
	ctx := c.Request.Context()
	if err = h.authorizeTeamMembers(ctx, requestUser, team); err != nil {
		return nil, err
	}
	result := &view.TeamMembersResponse{TeamId: team, Items: []view.TeamMemberItem{}}
	if commonconfig.GetAuthMode() == commonconfig.AuthModeExternal {
		members, listErr := identity.ListTeamMembers(ctx, team)
		if listErr != nil {
			return nil, listErr
		}
		result.Items = cvtTeamMembers(members)
	}
	result.TotalCount = len(result.Items)
	return result, nil
}

// authorizeTeamMembers allows a system administrator, read-only included, or a member of the team:
// a user whose request overlay holds a workspace through the team.
func (h *Handler) authorizeTeamMembers(ctx context.Context, requestUser *v1.User, team string) error {
	err := h.accessController.AuthorizeSystemAdmin(authority.AccessInput{Context: ctx, User: requestUser}, true)
	if err == nil {
		return nil
	}
	if requestUser == nil || requestUser.IsRestricted() {
		return err
	}
	if slices.Contains(requestTeams(ctx, requestUser), team) {
		return nil
	}
	return commonerrors.NewForbidden("no permission to list the members of the team " + team)
}

// requestTeams returns the teams that grant the user a workspace in the overlay of the request.
func requestTeams(ctx context.Context, user *v1.User) []string {
	o := extauth.RequestOverlay(ctx, user)
	if o == nil {
		return nil
	}
	var teams []string
	for _, sources := range o.Sources {
		for _, source := range sources {
			if team, ok := strings.CutPrefix(source, extauth.TeamSourcePrefix); ok && !slices.Contains(teams, team) {
				teams = append(teams, team)
			}
		}
	}
	return teams
}

// cvtTeamMembers converts the members of the identity service to the response items, never nil.
func cvtTeamMembers(members []identity.TeamMember) []view.TeamMemberItem {
	items := make([]view.TeamMemberItem, 0, len(members))
	for _, m := range members {
		items = append(items, view.TeamMemberItem{
			UserId: m.UserId, Email: m.Email, Name: m.Name, PreferredName: m.PreferredName, Role: m.Role,
		})
	}
	return items
}

// listWorkspaceMembers implements GET /workspaces/:name/members.
func (h *Handler) listWorkspaceMembers(c *gin.Context) (interface{}, error) {
	requestUser, err := h.getAndSetUsername(c)
	if err != nil {
		return nil, err
	}
	ctx := c.Request.Context()
	workspace, err := h.getAdminWorkspace(ctx, c.GetString(common.Name))
	if err != nil {
		return nil, err
	}
	if err = h.accessController.Authorize(authority.AccessInput{
		Context:      ctx,
		ResourceKind: v1.WorkspaceKind,
		Resource:     workspace,
		Verb:         v1.GetVerb,
		Workspaces:   []string{workspace.Name},
		User:         requestUser,
	}); err != nil {
		return nil, err
	}
	users, err := h.directWorkspaceMembers(ctx, workspace)
	if err != nil {
		return nil, err
	}
	teams, err := h.boundTeamMembers(ctx, workspace)
	if err != nil {
		return nil, err
	}
	return &view.WorkspaceMembersResponse{
		Users:  users,
		Teams:  teams,
		Groups: groupWorkspaceMembers(workspace.Name),
	}, nil
}

// directWorkspaceMembers returns the users that hold the workspace through their own record or
// through the workspace managers list.
func (h *Handler) directWorkspaceMembers(ctx context.Context, workspace *v1.Workspace) ([]view.WorkspaceMemberUser, error) {
	userList := &v1.UserList{}
	if err := h.List(ctx, userList); err != nil {
		return nil, err
	}
	sort.Sort(view.UserSlice(userList.Items))
	result := []view.WorkspaceMemberUser{}
	for i := range userList.Items {
		user := &userList.Items[i]
		isManager := commonuser.HasWorkspaceManagedRight(user, workspace.Name) ||
			slices.Contains(workspace.Spec.Managers, user.Name)
		if !isManager && !commonuser.HasWorkspaceRight(user, workspace.Name) {
			continue
		}
		role := v1.WorkspaceTeamMember
		if isManager {
			role = v1.WorkspaceTeamManager
		}
		result = append(result, view.WorkspaceMemberUser{
			Id:            user.Name,
			Name:          v1.GetUserName(user),
			Email:         v1.GetUserEmail(user),
			PreferredName: v1.GetAnnotation(user, v1.UserPreferredNameAnnotation),
			Role:          role,
			Source:        sourceDirect,
		})
	}
	return result, nil
}

// boundTeamMembers returns the teams bound to the workspace with the display data the identity
// service has for them. A team the service does not know is returned with its id and role only.
func (h *Handler) boundTeamMembers(ctx context.Context, workspace *v1.Workspace) ([]view.WorkspaceMemberTeam, error) {
	ids := teamNames(workspace)
	display, err := h.lookupTeamDisplay(ctx, ids)
	if err != nil {
		return nil, err
	}
	result := []view.WorkspaceMemberTeam{}
	for _, id := range ids {
		role, _ := workspace.TeamRole(id)
		item := view.WorkspaceMemberTeam{Id: id, Role: role, Source: extauth.TeamSourcePrefix + id}
		if info, ok := display[id]; ok {
			item.DisplayName = info.DisplayName
			count := info.MemberCount
			item.MemberCount = &count
			for _, m := range info.Managers {
				item.Managers = append(item.Managers, view.WorkspaceMemberManager{
					Id: m.UserId, Name: m.Name, Email: m.Email, PreferredName: m.PreferredName,
				})
			}
		}
		result = append(result, item)
	}
	return result, nil
}

// groupWorkspaceMembers returns the platform groups whose mapping names the workspace. The mappings
// apply in external mode only, so builtin mode returns none.
func groupWorkspaceMembers(workspace string) []view.WorkspaceMemberGroup {
	if commonconfig.GetAuthMode() != commonconfig.AuthModeExternal {
		return []view.WorkspaceMemberGroup{}
	}
	mappings, err := commonconfig.GetAuthGroupMappings()
	if err != nil {
		klog.ErrorS(err, "failed to read the auth group mappings")
		return []view.WorkspaceMemberGroup{}
	}
	roles := map[string]v1.WorkspaceTeamRole{}
	for _, m := range mappings {
		if m.Group == "" || !slices.Contains(m.Workspaces, workspace) {
			continue
		}
		role := roles[m.Group]
		if role != v1.WorkspaceTeamManager {
			role = v1.WorkspaceTeamMember
			if slices.Contains(m.Roles, string(v1.WorkspaceAdminRole)) {
				role = v1.WorkspaceTeamManager
			}
		}
		roles[m.Group] = role
	}
	result := make([]view.WorkspaceMemberGroup, 0, len(roles))
	for group, role := range roles {
		result = append(result, view.WorkspaceMemberGroup{
			Name: group, Role: role, Source: extauth.GroupSourcePrefix + group,
		})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result
}

// teamDisplay is lookupTeamDisplay for a caller that shows what it can: when the identity service
// fails, it logs the failure and returns no display data.
func (h *Handler) teamDisplay(ctx context.Context, ids []string) map[string]identity.TeamInfo {
	result, err := h.lookupTeamDisplay(ctx, ids)
	if err != nil {
		klog.ErrorS(err, "teams of the identity service are not available")
		return map[string]identity.TeamInfo{}
	}
	return result
}

// lookupTeamDisplay returns the display data the identity service has for the teams. It is used
// only in external mode. A team the service does not know is left out of the result; any other
// failure of the service is returned.
func (h *Handler) lookupTeamDisplay(ctx context.Context, ids []string) (map[string]identity.TeamInfo, error) {
	result := map[string]identity.TeamInfo{}
	if len(ids) == 0 || commonconfig.GetAuthMode() != commonconfig.AuthModeExternal {
		return result, nil
	}
	listed, err := identity.ListTeams(ctx)
	if err != nil {
		return nil, err
	}
	for _, id := range ids {
		if info, ok := listed[id]; ok {
			result[id] = info
			continue
		}
		members, memberErr := identity.ListTeamMembers(ctx, id)
		if apierrors.IsNotFound(memberErr) {
			continue
		}
		if memberErr != nil {
			return nil, memberErr
		}
		info := identity.TeamInfo{Id: id, MemberCount: len(members)}
		for _, m := range members {
			if m.Role == string(v1.WorkspaceTeamManager) {
				info.Managers = append(info.Managers, m)
			}
		}
		result[id] = info
	}
	return result, nil
}

// getUserWorkspaces implements GET /users/:name/workspaces.
func (h *Handler) getUserWorkspaces(c *gin.Context) (interface{}, error) {
	requestUser, err := h.getAndSetUsername(c)
	if err != nil {
		return nil, err
	}
	if requestUser == nil {
		return nil, commonerrors.NewUnauthorized("the request user is unknown")
	}
	ctx := c.Request.Context()
	target, err := h.getAdminUser(ctx, c.GetString(common.Name))
	if err != nil {
		return nil, err
	}
	effective, overlay := h.recordedUser(ctx, target)
	workspaces, managed := h.userWorkspacesWithSources(ctx, effective, target, overlay)

	roles := h.accessController.GetRoles(ctx, requestUser)
	canSee := func(entries []view.WorkspaceEntry) []view.WorkspaceEntry {
		kept := []view.WorkspaceEntry{}
		for _, e := range entries {
			if h.authUserAction(c, requestUser, target, []string{e.Id}, authority.UserWorkspaceResource,
				roles, v1.UpdateVerb) == nil {
				kept = append(kept, e)
			}
		}
		return kept
	}
	resp := &view.UserWorkspacesResponse{
		Workspaces:        canSee(workspaces),
		ManagedWorkspaces: canSee(managed),
	}
	if len(resp.Workspaces) == 0 && len(resp.ManagedWorkspaces) == 0 {
		if err = h.authUserAction(c, requestUser, target, nil, authority.UserWorkspaceResource,
			roles, v1.UpdateVerb); err != nil {
			return nil, err
		}
	}
	return resp, nil
}

// recordedIdentityInUse reports whether the groups and teams SCIM records on users grant access: in
// external mode with SCIM enabled, as authority.WithRecordedGroups decides.
func recordedIdentityInUse() bool {
	return commonconfig.GetAuthMode() == commonconfig.AuthModeExternal && commonconfig.IsSCIMEnabled()
}

// recordedOverlay returns the overlay authority.WithRecordedGroups builds for a user from what the
// identity service pushed to the user record: the platform groups, mapped as the request overlay
// maps them, and the workspaces of the teams. It is empty unless recordedIdentityInUse.
func (h *Handler) recordedOverlay(ctx context.Context, user *v1.User) *extauth.Overlay {
	if recordedIdentityInUse() {
		if overlay := extauth.RequestOverlay(authority.WithRecordedGroups(ctx, user), user); overlay != nil {
			return overlay
		}
	}
	return extauth.MapGroups(user.Name, nil, nil, false)
}

// recordedUser returns the user as its recorded identity makes it, with recordedOverlay applied as
// the request overlay is applied, and that overlay. In builtin mode it returns the record and no
// overlay.
func (h *Handler) recordedUser(ctx context.Context, user *v1.User) (*v1.User, *extauth.Overlay) {
	if commonconfig.GetAuthMode() != commonconfig.AuthModeExternal {
		return user, nil
	}
	overlay := h.recordedOverlay(ctx, user)
	return extauth.ApplyOverlay(extauth.WithOverlay(ctx, overlay), user), overlay
}

// userWorkspacesWithSources is userWorkspaces with the sources of each workspace: direct for the
// stored record and the workspace managers list, the group and team sources of the overlay, and
// default for a default workspace. effective is the user with the overlay merged, record the
// stored user.
func (h *Handler) userWorkspacesWithSources(ctx context.Context, effective, record *v1.User,
	overlay *extauth.Overlay) (workspaces, managed []view.WorkspaceEntry) {
	joinDefault := commonconfig.IsUserJoinDefaultWorkspaces()
	return h.userWorkspaces(ctx, effective, func(workspace *v1.Workspace, isManaged bool) []string {
		id := workspace.Name
		var sources, granted []string
		if isManaged {
			if commonuser.HasWorkspaceManagedRight(record, id) || slices.Contains(workspace.Spec.Managers, record.Name) {
				sources = append(sources, sourceDirect)
			}
			if overlay != nil {
				granted = overlay.ManagedSources[id]
			}
		} else {
			if commonuser.HasWorkspaceRight(record, id) {
				sources = append(sources, sourceDirect)
			}
			if overlay != nil {
				granted = overlay.Sources[id]
			}
		}
		granted = slices.Clone(granted)
		sort.Strings(granted)
		sources = append(sources, granted...)
		if !isManaged && workspace.Spec.IsDefault && joinDefault {
			sources = append(sources, sourceDefault)
		}
		return sources
	})
}

// teamsChange computes the next team bindings of a workspace from the current ones.
type teamsChange func(current []v1.WorkspaceTeam) ([]v1.WorkspaceTeam, error)

// planWorkspaceTeams reads the workspace, checks that the caller may update it, applies change to
// the team bindings and checks that the caller may make that change: any added, removed or
// modified manager binding needs a system administrator. It returns the workspace carrying the
// next bindings and the bindings it had before, and writes nothing.
func (h *Handler) planWorkspaceTeams(ctx context.Context, requestUser *v1.User,
	name string, change teamsChange) (*v1.Workspace, []v1.WorkspaceTeam, error) {
	workspace, err := h.getAdminWorkspace(ctx, name)
	if err != nil {
		return nil, nil, err
	}
	if err = h.accessController.Authorize(authority.AccessInput{
		Context:      ctx,
		ResourceKind: v1.WorkspaceKind,
		Resource:     workspace,
		Verb:         v1.UpdateVerb,
		Workspaces:   []string{workspace.Name},
		User:         requestUser,
	}); err != nil {
		return nil, nil, err
	}
	before := slices.Clone(workspace.Spec.Teams)
	next, err := change(slices.Clone(before))
	if err != nil {
		return nil, nil, err
	}
	if err = h.authorizeTeamsChange(ctx, requestUser, before, next); err != nil {
		return nil, nil, err
	}
	workspace.Spec.Teams = next
	return workspace, before, nil
}

// updateWorkspaceTeamList plans the change and stores it, planning again on a write conflict. It
// returns the stored workspace and the bindings it had before.
func (h *Handler) updateWorkspaceTeamList(ctx context.Context, requestUser *v1.User,
	name string, change teamsChange) (*v1.Workspace, []v1.WorkspaceTeam, error) {
	var updated *v1.Workspace
	var before []v1.WorkspaceTeam
	err := backoff.ConflictRetry(func() error {
		planned, previous, err := h.planWorkspaceTeams(ctx, requestUser, name, change)
		if err != nil {
			return err
		}
		if !slices.Equal(previous, planned.Spec.Teams) {
			if err = h.Update(ctx, planned); err != nil {
				return err
			}
		}
		updated, before = planned, previous
		return nil
	}, defaultRetryCount, defaultRetryDelay)
	if err != nil {
		return nil, nil, err
	}
	return updated, before, nil
}

// authorizeTeamsChange requires a system administrator when the change from current to next adds,
// removes or modifies a binding with the manager role.
func (h *Handler) authorizeTeamsChange(ctx context.Context, requestUser *v1.User, current, next []v1.WorkspaceTeam) error {
	if !managerBindingChanged(current, next) {
		return nil
	}
	return h.accessController.AuthorizeSystemAdmin(authority.AccessInput{Context: ctx, User: requestUser}, false)
}

// managerBindingChanged reports whether a team is bound with the manager role on only one side of
// the change, or with different roles on the two sides, one of them manager.
func managerBindingChanged(current, next []v1.WorkspaceTeam) bool {
	rolesOf := func(teams []v1.WorkspaceTeam) map[string]v1.WorkspaceTeamRole {
		roles := make(map[string]v1.WorkspaceTeamRole, len(teams))
		for _, t := range teams {
			if roles[t.Name] != v1.WorkspaceTeamManager {
				roles[t.Name] = t.EffectiveRole()
			}
		}
		return roles
	}
	before, after := rolesOf(current), rolesOf(next)
	for name, role := range before {
		if after[name] != role && role == v1.WorkspaceTeamManager {
			return true
		}
	}
	for name, role := range after {
		if before[name] != role && role == v1.WorkspaceTeamManager {
			return true
		}
	}
	return false
}

// setTeamBinding returns the bindings with the team of binding bound with its effective role when
// bound is true, and unbound otherwise. An existing binding keeps its position, and is left as
// stored when its role already is the requested one.
func setTeamBinding(teams []v1.WorkspaceTeam, binding v1.WorkspaceTeam, bound bool) []v1.WorkspaceTeam {
	team, role := binding.Name, binding.EffectiveRole()
	result := make([]v1.WorkspaceTeam, 0, len(teams)+1)
	placed := false
	for _, t := range teams {
		if t.Name != team {
			result = append(result, t)
			continue
		}
		if !bound || placed {
			continue
		}
		placed = true
		if t.EffectiveRole() != role {
			t.Role = role
		}
		result = append(result, t)
	}
	if bound && !placed {
		result = append(result, v1.WorkspaceTeam{Name: team, Role: role})
	}
	return result
}

// teamNames returns the sorted, distinct names of the teams bound to the workspace.
func teamNames(workspace *v1.Workspace) []string {
	names := commonworkspace.TeamIndexFunc(workspace)
	sort.Strings(names)
	return names
}

// refuseDelegatedTeamsChange refuses a request made with a delegated credential that sets the team
// bindings or the access policy of a workspace, as extauth.Forbidden refuses the team binding routes.
func refuseDelegatedTeamsChange(c *gin.Context, setsTeams bool) error {
	if !setsTeams || !authority.IsDelegatedRequest(c) {
		return nil
	}
	return commonerrors.NewForbidden("this operation is not available with a delegated token")
}

// badRequest returns the validation error as a 400, nil when there is none.
func badRequest(err error) error {
	if err == nil {
		return nil
	}
	return commonerrors.NewBadRequest(err.Error())
}

// validateTeamName checks that the team id is a DNS-1123 label.
func validateTeamName(team string) error {
	return badRequest(commonworkspace.ValidateTeamName(team))
}

// validateTeamRole checks that the role is empty, member or manager.
func validateTeamRole(role v1.WorkspaceTeamRole) error {
	return badRequest(commonworkspace.ValidateTeamRole(role))
}

// validateWorkspaceTeamsRequest checks the add and remove lists of a team bindings update: valid
// bindings to add, no team named twice, and at least one change.
func validateWorkspaceTeamsRequest(req *view.UpdateWorkspaceTeamsRequest) error {
	if len(req.Add) == 0 && len(req.Remove) == 0 {
		return commonerrors.NewBadRequest("add or remove is required")
	}
	if err := validateWorkspaceTeams(req.Add); err != nil {
		return err
	}
	named := map[string]bool{}
	for _, team := range req.Add {
		named[team.Name] = true
	}
	for _, team := range req.Remove {
		if team == "" {
			return commonerrors.NewBadRequest("the team to remove is empty")
		}
		if named[team] {
			return commonerrors.NewBadRequest(fmt.Sprintf("the team %s is named more than once", team))
		}
		named[team] = true
	}
	return nil
}

// validateTeamWorkspaceBindings checks the entries of a team to workspaces update: at least one,
// each with a workspace and a valid role, no workspace twice.
func validateTeamWorkspaceBindings(req []view.TeamWorkspaceBinding) error {
	if len(req) == 0 {
		return commonerrors.NewBadRequest("at least one workspace is required")
	}
	seen := map[string]bool{}
	for _, entry := range req {
		if entry.WorkspaceId == "" {
			return commonerrors.NewBadRequest("the workspace is empty")
		}
		if err := validateTeamRole(entry.Role); err != nil {
			return err
		}
		if seen[entry.WorkspaceId] {
			return commonerrors.NewBadRequest(fmt.Sprintf("the workspace %s is named more than once", entry.WorkspaceId))
		}
		seen[entry.WorkspaceId] = true
	}
	return nil
}

// validateWorkspaceTeams checks the bindings of a workspace with commonworkspace.ValidateTeams, as
// the admission webhook does.
func validateWorkspaceTeams(teams []v1.WorkspaceTeam) error {
	return badRequest(commonworkspace.ValidateTeams(teams))
}

// validateAccessPolicy checks that the policy is empty, closed or approval.
func validateAccessPolicy(policy v1.WorkspaceAccessPolicy) error {
	return badRequest(commonworkspace.ValidateAccessPolicy(policy))
}
