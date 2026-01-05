/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package authority

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	commonuser "github.com/AMD-AIG-AIMA/SAFE/common/pkg/user"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/slice"
)

const (
	SystemAdminRequired = "System administrator privileges are required"
)

var (
	accessOnce           sync.Once
	accessControllerInst *AccessController
)

type AccessController struct {
	client.Client
}

// AccessInput represents the input parameters for access authorization
// It contains all the necessary information to perform an authorization check,
// including the resource being accessed, the action being performed,
// and the user requesting access.
type AccessInput struct {
	// Context is the context for the authorization request, used for passing request-scoped values
	Context context.Context

	// ResourceKind is the kind of target resource being accessed (e.g., "cluster", "node", "workload")
	ResourceKind string

	// ResourceOwner is the owner of target resource, typically the user ID who owns the resource
	ResourceOwner string

	// Resource is the actual resource object being accessed, can be nil if only ResourceKind is known
	Resource client.Object

	// Verb is the action being performed on the resource (e.g., create, get, list, update, delete)
	Verb v1.RoleVerb

	// Workspaces is the list of workspace IDs to which the resource belongs
	Workspaces []string

	// UserId is the ID of the user making the request, used to fetch the User object if not provided
	UserId string

	// User is the user object making the request, can be nil if UserId is provided instead
	User *v1.User

	// Roles is the list of roles assigned to the requesting user
	Roles []*v1.Role
}

// NewAccessController creates and returns a singleton instance of AccessController.
func NewAccessController(cli client.Client) *AccessController {
	accessOnce.Do(func() {
		accessControllerInst = &AccessController{
			Client: cli,
		}
	})
	return accessControllerInst
}

// Authorize performs authorization check for a user request.
// It retrieves user and roles if not provided, then validates if the user
// has permission to perform the requested action on the resource.
func (a *AccessController) Authorize(in AccessInput) error {
	if in.User == nil {
		var err error
		if in.User, err = a.GetRequestUser(in.Context, in.UserId); err != nil {
			return err
		}
	}
	if len(in.Roles) == 0 {
		in.Roles = a.GetRoles(in.Context, in.User)
	}
	return a.authorize(in)
}

// AuthorizeSystemAdmin checks if the user has system administrator privileges.
// Returns an error if the user is not a system admin.
func (a *AccessController) AuthorizeSystemAdmin(in AccessInput, readOnlyAllowed bool) error {
	if in.User == nil {
		var err error
		if in.User, err = a.GetRequestUser(in.Context, in.UserId); err != nil {
			return err
		}
	}
	if in.User.IsSystemAdmin() {
		return nil
	}
	if readOnlyAllowed && in.User.IsSystemAdminReadonly() {
		return nil
	}
	return commonerrors.NewForbidden(SystemAdminRequired)
}

// GetRequestUser retrieves a user object by userId from the k8s cluster.
// Returns an error if the userId is empty or the user doesn't exist.
func (a *AccessController) GetRequestUser(ctx context.Context, userId string) (*v1.User, error) {
	if userId == "" {
		return nil, commonerrors.NewBadRequest("the request userId is empty")
	}
	user := &v1.User{}
	err := a.Get(ctx, client.ObjectKey{Name: userId}, user)
	if err != nil {
		return nil, commonerrors.NewUserNotRegistered(userId)
	}
	return user, nil
}

// GetRoles retrieves all roles associated with a user.
// Fetches role objects based on the role names specified in the user spec.
func (a *AccessController) GetRoles(ctx context.Context, user *v1.User) []*v1.Role {
	if user == nil {
		return nil
	}
	var result []*v1.Role
	for _, r := range user.Spec.Roles {
		role := &v1.Role{}
		err := a.Get(ctx, client.ObjectKey{Name: string(r)}, role)
		if err != nil {
			klog.ErrorS(err, "failed to get user role", "user", user.Name, "role", r)
			continue
		}
		result = append(result, role)
	}
	return result
}

// authorize is the core authorization logic that checks if a user has permission
// to perform an action on a resource based on their roles and permissions.
func (a *AccessController) authorize(in AccessInput) error {
	if err := a.checkUserStatus(in.User); err != nil {
		return err
	}
	isOwner, isWorkspaceUser := a.determineOwnership(&in)
	resourceKind, resourceName := a.extractResourceInfo(in)

	for _, r := range in.Roles {
		rules := a.getPolicyRules(r, resourceKind, resourceName, isOwner, isWorkspaceUser)
		if isMatchVerb(rules, in.Verb) {
			return nil
		}
	}
	if len(commonuser.GetManagedWorkspace(in.User)) > 0 {
		isWorkspaceManager := false
		if len(in.Workspaces) > 0 && commonuser.HasWorkspaceManagedRight(in.User, in.Workspaces...) {
			isWorkspaceManager = true
		}
		if role, err := a.getWorkspaceAdminRole(in.Context); err == nil {
			rules := a.getPolicyRules(role, resourceKind, resourceName, isOwner, isWorkspaceManager)
			if isMatchVerb(rules, in.Verb) {
				return nil
			}
		}
	}

	return commonerrors.NewForbidden(
		fmt.Sprintf("The user is not allowed to %s %s, workspace: %s", in.Verb, resourceKind, in.Workspaces))
}

// checkUserStatus checks UserStatus and returns the result.
func (a *AccessController) checkUserStatus(user *v1.User) error {
	if user.IsRestricted() {
		return commonerrors.NewForbidden(
			fmt.Sprintf("The user is restricted. type: %d", user.Spec.RestrictedType))
	}
	return nil
}

// extractResourceInfo extracts resource kind and name from the input.
// Determines the resource type and identifier for authorization checks.
func (a *AccessController) extractResourceInfo(in AccessInput) (resourceKind, resourceName string) {
	resourceKind = in.ResourceKind
	if resourceKind == "" && in.Resource != nil {
		resourceKind = in.Resource.GetObjectKind().GroupVersionKind().Kind
	}
	resourceKind = strings.ToLower(resourceKind)

	if in.Resource != nil {
		resourceName = in.Resource.GetName()
	}
	return resourceKind, resourceName
}

// determineOwnership checks if the user is the owner of the resource
// or has workspace-level access to the resource.
func (a *AccessController) determineOwnership(in *AccessInput) (isOwner bool, isWorkspaceUser bool) {
	if in.ResourceOwner == "" && in.Resource != nil {
		in.ResourceOwner = v1.GetUserId(in.Resource)
	}

	if in.User.Name == in.ResourceOwner {
		isOwner = true
	}

	if len(in.Workspaces) > 0 && commonuser.HasWorkspaceRight(in.User, in.Workspaces...) {
		isWorkspaceUser = true
	}

	return isOwner, isWorkspaceUser
}

// getPolicyRules retrieves applicable policy rules from a role based on
// resource type, ownership, and workspace membership.
func (a *AccessController) getPolicyRules(role *v1.Role,
	resourceKind, resourceName string, isOwner, isWorkspaceUser bool) []*v1.PolicyRule {
	var result []*v1.PolicyRule
	for i, r := range role.Rules {
		if !slice.Contains(r.Resources, AllResource) && !slice.Contains(r.Resources, resourceKind) {
			continue
		}
		isMatch := false
		for _, n := range r.GrantedUsers {
			switch n {
			case GrantedAllUser:
				isMatch = true
			case GrantedOwner:
				if isOwner {
					isMatch = true
				}
			case GrantedWorkspaceUser:
				if isWorkspaceUser {
					isMatch = true
				}
			default:
				if resourceName != "" && n == resourceName {
					isMatch = true
				}
			}
			if isMatch {
				break
			}
		}
		if len(r.GrantedUsers) == 0 || isMatch {
			result = append(result, &role.Rules[i])
		}
	}
	return result
}

// getWorkspaceAdminRole retrieves the workspace administrator role from the system.
// Returns the role object and any error encountered during retrieval.
func (a *AccessController) getWorkspaceAdminRole(ctx context.Context) (*v1.Role, error) {
	role := &v1.Role{}
	err := a.Get(ctx, client.ObjectKey{Name: string(v1.WorkspaceAdminRole)}, role)
	return role, err
}

// isMatchVerb checks if any of the provided policy rules allow the specified verb/action.
// Returns true if the verb is permitted by any rule, false otherwise.
func isMatchVerb(rules []*v1.PolicyRule, verb v1.RoleVerb) bool {
	for _, r := range rules {
		for _, v := range r.Verbs {
			if v == v1.AllVerb || v == verb {
				return true
			}
		}
	}
	return false
}
