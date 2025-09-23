/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package authority

import (
	"context"
	"fmt"
	"strings"

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
	instance *Authorizer
)

type Authorizer struct {
	client.Client
}

type Input struct {
	Context       context.Context
	ResourceKind  string
	ResourceOwner string
	Resource      client.Object
	Verb          v1.RoleVerb
	// the workspace to which the resource belongs.
	Workspaces []string
	// user and userid are optional; only one needs to be provided.
	// it is from the requesting end.
	UserId string
	User   *v1.User
	// the request user's roles
	Roles []*v1.Role
}

func NewAuthorizer(cli client.Client) *Authorizer {
	instance = &Authorizer{
		Client: cli,
	}
	return instance
}

func (a *Authorizer) Authorize(in Input) error {
	if in.User == nil {
		var err error
		in.User, err = a.GetRequestUser(in.Context, in.UserId)
		if err != nil {
			return err
		}
	}
	if len(in.Roles) == 0 {
		in.Roles = a.GetRoles(in.Context, in.User)
	}
	return a.authorize(in)
}

func (a *Authorizer) AuthorizeSystemAdmin(in Input) error {
	user, err := a.GetRequestUser(in.Context, in.UserId)
	if err != nil {
		return err
	}
	if !user.IsSystemAdmin() {
		return commonerrors.NewForbidden(SystemAdminRequired)
	}
	return nil
}

func (a *Authorizer) GetRequestUser(ctx context.Context, userId string) (*v1.User, error) {
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

func (a *Authorizer) GetRoles(ctx context.Context, user *v1.User) []*v1.Role {
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

func (a *Authorizer) authorize(in Input) error {
	if in.User.IsRestricted() {
		return commonerrors.NewForbidden(
			fmt.Sprintf("The user is restricted. type: %d", in.User.Spec.RestrictedType))
	}
	if in.ResourceOwner == "" {
		in.ResourceOwner = v1.GetUserId(in.Resource)
	}
	isOwner := false
	if in.User.Name == in.ResourceOwner {
		isOwner = true
	}
	isWorkspaceUser := false
	if len(in.Workspaces) > 0 && commonuser.HasWorkspaceRight(in.User, in.Workspaces...) {
		isWorkspaceUser = true
	}
	resourceKind := in.ResourceKind
	if resourceKind == "" {
		resourceKind = in.Resource.GetObjectKind().GroupVersionKind().Kind
	}
	resourceKind = strings.ToLower(resourceKind)
	resourceName := ""
	if in.Resource != nil {
		resourceName = in.Resource.GetName()
	}

	roles := make([]*v1.Role, 0, len(in.Roles)+1)
	roles = append(roles, in.Roles...)
	if len(in.Workspaces) > 0 && commonuser.HasWorkspaceManagedRight(in.User, in.Workspaces...) {
		role := &v1.Role{}
		if err := a.Get(in.Context, client.ObjectKey{Name: string(v1.WorkspaceAdminRole)}, role); err == nil {
			roles = append(roles, role)
		}
	}
	for _, r := range roles {
		rules := getPolicyRules(r, resourceKind, resourceName, isOwner, isWorkspaceUser)
		if isMatchVerb(rules, in.Verb) {
			return nil
		}
	}
	return commonerrors.NewForbidden(
		fmt.Sprintf("The user is not allowed to %s %s", in.Verb, resourceKind))
}

func getPolicyRules(role *v1.Role, resourceKind, resourceName string, isOwner, isWorkspaceUser bool) []*v1.PolicyRule {
	var result []*v1.PolicyRule
	for i, r := range role.Rules {
		if !slice.Contains(r.Resources, AllResource) && !slice.Contains(r.Resources, resourceKind) {
			continue
		}
		isMatch := false
		for _, n := range r.GrantedUsers {
			switch n {
			case AllResource:
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
