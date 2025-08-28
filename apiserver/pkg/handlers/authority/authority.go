/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package authority

import (
	"fmt"
	"strings"

	"github.com/gin-gonic/gin"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	apiutils "github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/utils"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
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
	GinContext    *gin.Context
	ResourceKind  string
	ResourceOwner string
	Resource      client.Object
	Verb          v1.RoleVerb
	Workspaces    []string
	User          *v1.User
	Roles         []*v1.Role
}

func NewAuthorizer(cli client.Client) *Authorizer {
	instance = &Authorizer{
		Client: cli,
	}
	return instance
}

func (a *Authorizer) Authorize(in Input) error {
	if !commonconfig.IsEnableUserAuthority() {
		return nil
	}
	if in.User == nil {
		var err error
		in.User, err = a.GetRequestUser(in.GinContext)
		if err != nil {
			return err
		}
	}
	if len(in.Roles) == 0 {
		in.Roles = apiutils.GetRoles(in.GinContext.Request.Context(), a.Client, in.User)
	}
	return a.authorize(in)
}

func (a *Authorizer) AuthorizeSystemAdmin(c *gin.Context) error {
	if !commonconfig.IsEnableUserAuthority() {
		return nil
	}
	user, err := a.GetRequestUser(c)
	if err != nil {
		return err
	}
	if !user.IsSystemAdmin() {
		return commonerrors.NewForbidden(SystemAdminRequired)
	}
	return nil
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
	for _, r := range in.Roles {
		rules := getPolicyRules(r, resourceKind, resourceName, isOwner, isWorkspaceUser)
		if isMatchVerb(rules, in.Verb) {
			return nil
		}
	}
	return commonerrors.NewForbidden(
		fmt.Sprintf("The user is not allowed to %s %s", in.Verb, resourceKind))
}

func (a *Authorizer) GetRequestUser(c *gin.Context) (*v1.User, error) {
	userId := c.GetString(common.UserId)
	if userId == "" {
		return nil, nil
	}
	user := &v1.User{}
	err := a.Get(c.Request.Context(), client.ObjectKey{Name: userId}, user)
	if err != nil {
		return nil, commonerrors.NewUserNotRegistered(userId)
	}
	c.Set(common.UserName, v1.GetUserName(user))
	return user, nil
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
		if len(r.Resources) == 0 || isMatch {
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
