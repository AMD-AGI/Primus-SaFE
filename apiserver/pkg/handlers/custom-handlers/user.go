/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package custom_handlers

import (
	"context"
	"net/url"
	"sort"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/custom-handlers/types"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	commonuser "github.com/AMD-AIG-AIMA/SAFE/common/pkg/user"
	jsonutils "github.com/AMD-AIG-AIMA/SAFE/utils/pkg/json"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/slice"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/stringutil"
)

const (
	FormContent = "application/x-www-form-urlencoded"
	MaxTokenAge = 3600 * 24 * 365
)

func (h *Handler) CreateUser(c *gin.Context) {
	handle(c, h.createUser)
}

func (h *Handler) ListUser(c *gin.Context) {
	handle(c, h.listUser)
}

func (h *Handler) GetUser(c *gin.Context) {
	handle(c, h.getUser)
}

func (h *Handler) PatchUser(c *gin.Context) {
	handle(c, h.patchUser)
}

func (h *Handler) DeleteUser(c *gin.Context) {
	handle(c, h.deleteUser)
}

func (h *Handler) Login(c *gin.Context) {
	handle(c, h.login)
}

func (h *Handler) Logout(c *gin.Context) {
	handle(c, h.logout)
}

func (h *Handler) createUser(c *gin.Context) (interface{}, error) {
	req, err := parseCreateUserQuery(c)
	if err != nil {
		return nil, err
	}

	if err = h.authUserCreation(c, req); err != nil {
		return nil, err
	}
	user := generateUser(req)
	if err = h.Create(c.Request.Context(), user); err != nil {
		return nil, err
	}
	return nil, nil
}

func (h *Handler) authUserCreation(c *gin.Context, req *types.CreateUserRequest) error {
	requestUser, err := h.auth.GetRequestUser(c)
	if err != nil {
		return err
	}
	if requestUser != nil && !requestUser.IsSystemAdmin() {
		if req.Type != requestUser.Spec.Type {
			return commonerrors.NewBadRequest("invalid user type")
		}
	}
	if err = h.auth.Authorize(authority.Input{
		GinContext:   c,
		ResourceKind: v1.UserKind,
		Verb:         v1.CreateVerb,
		User:         requestUser,
	}); err != nil {
		return err
	}
	return nil
}

func generateUser(req *types.CreateUserRequest) *v1.User {
	user := &v1.User{
		ObjectMeta: metav1.ObjectMeta{
			Name: req.Id,
			Annotations: map[string]string{
				v1.UserNameAnnotation:      req.Name,
				v1.UserEmailAnnotation:     req.Email,
				v1.UserAvatarUrlAnnotation: req.AvatarUrl,
			},
		},
		Spec: v1.UserSpec{
			Type:     req.Type,
			Roles:    []v1.UserRole{v1.DefaultRole},
			Password: req.Password,
		},
	}
	if req.Password != "" {
		user.Spec.Password = stringutil.Base64Encode(req.Password)
	}
	commonuser.AssignWorkspace(user, req.Workspaces...)
	return user
}

func (h *Handler) listUser(c *gin.Context) (interface{}, error) {
	requestUser, err := h.auth.GetRequestUser(c)
	if err != nil {
		return nil, err
	}
	query, err := parseListUserQuery(c)
	if err != nil {
		return nil, err
	}

	labelSelector := buildListUserSelector(query)
	userList := &v1.UserList{}
	err = h.List(c.Request.Context(), userList, &client.ListOptions{LabelSelector: labelSelector})
	if err != nil {
		return nil, err
	}
	result := types.ListUserResponse{}
	if len(userList.Items) > 0 {
		sort.Sort(types.UserSlice(userList.Items))
	}
	roles := authority.GetRoles(c.Request.Context(), h.Client, requestUser)
	for _, item := range userList.Items {
		if query.WorkspaceId != "" && !commonuser.HasWorkspaceRight(&item, query.WorkspaceId) {
			continue
		}
		if err = h.authUser(c, requestUser, &item, []string{query.WorkspaceId}, "", roles, v1.ListVerb); err != nil {
			continue
		}
		result.Items = append(result.Items, h.cvtToUserResponseItem(c.Request.Context(), &item))
	}
	result.TotalCount = len(result.Items)
	return result, nil
}

func (h *Handler) getUser(c *gin.Context) (interface{}, error) {
	targetUserId := c.GetString(types.Name)
	if targetUserId == common.UserSelf {
		targetUserId = c.GetString(common.UserId)
	}
	targetUser, err := h.getTargetUser(c.Request.Context(), targetUserId)
	if err != nil {
		return nil, err
	}
	if err = h.auth.Authorize(authority.Input{
		GinContext: c,
		Resource:   targetUser,
		Verb:       v1.GetVerb,
	}); err != nil {
		return nil, err
	}
	return h.cvtToUserResponseItem(c.Request.Context(), targetUser), nil
}

func (h *Handler) patchUser(c *gin.Context) (interface{}, error) {
	req := &types.PatchUserRequest{}
	body, err := getBodyFromRequest(c.Request, req)
	if err != nil {
		klog.ErrorS(err, "fail to parse request data", "body", string(body))
		return nil, err
	}

	targetUserId := c.GetString(types.Name)
	targetUser, err := h.getTargetUser(c.Request.Context(), targetUserId)
	if err != nil {
		return nil, err
	}

	isChanged, err := h.checkPatchUser(c, targetUser, req)
	if !isChanged || err != nil {
		return nil, err
	}

	patch := client.MergeFrom(targetUser.DeepCopy())
	if req.Workspaces != nil {
		commonuser.AssignWorkspace(targetUser, *req.Workspaces...)
	}
	if req.Roles != nil {
		targetUser.Spec.Roles = *req.Roles
	}
	if req.RestrictedType != nil {
		targetUser.Spec.RestrictedType = *req.RestrictedType
	}
	if req.AvatarUrl != nil {
		metav1.SetMetaDataAnnotation(&targetUser.ObjectMeta, v1.UserAvatarUrlAnnotation, *req.AvatarUrl)
	}
	if req.Password != nil {
		targetUser.Spec.Password = stringutil.Base64Encode(*req.Password)
	}

	if req.Name != nil {
		v1.SetLabel(targetUser, v1.UserNameMd5Label, stringutil.MD5(*req.Name))
		v1.SetAnnotation(targetUser, v1.UserNameAnnotation, *req.Name)
	}
	if req.Email != nil {
		v1.SetLabel(targetUser, v1.UserEmailMd5Label, stringutil.MD5(*req.Email))
		v1.SetAnnotation(targetUser, v1.UserEmailAnnotation, *req.Email)
	}
	if err = h.Patch(c.Request.Context(), targetUser, patch); err != nil {
		klog.ErrorS(err, "fail to patch user", "body", string(body))
		return nil, err
	}
	klog.Infof("patch user, target.user: %s, request.user: %s, request: %s",
		targetUserId, c.GetString(common.UserName), string(jsonutils.MarshalSilently(req)))
	return nil, nil
}

func (h *Handler) checkPatchUser(c *gin.Context, targetUser *v1.User, req *types.PatchUserRequest) (bool, error) {
	requestUser, err := h.auth.GetRequestUser(c)
	if err != nil {
		return false, err
	}
	roles := authority.GetRoles(c.Request.Context(), h.Client, requestUser)

	isChanged := false
	if req.RestrictedType != nil && *req.RestrictedType != targetUser.Spec.RestrictedType ||
		req.Roles != nil && !commonuser.IsRolesEqual(*req.Roles, targetUser.Spec.Roles) {
		if err = h.authUser(c, requestUser, targetUser,
			commonuser.GetWorkspace(targetUser), authority.UserIdentityResource, roles, v1.UpdateVerb); err != nil {
			return false, err
		}
		isChanged = true
	}

	if req.Workspaces != nil {
		currentWorkspaces := commonuser.GetWorkspace(targetUser)
		var workspaces []string
		if workspaces2 := slice.Difference(*req.Workspaces, currentWorkspaces); len(workspaces2) > 0 {
			workspaces = append(workspaces, workspaces2...)
		}
		if workspaces2 := slice.Difference(currentWorkspaces, *req.Workspaces); len(workspaces2) > 0 {
			workspaces = append(workspaces, workspaces2...)
		}
		if len(workspaces) > 0 {
			if err = h.authUser(c, requestUser, targetUser,
				workspaces, authority.UserWorkspaceResource, roles, v1.UpdateVerb); err != nil {
				return false, err
			}
			isChanged = true
		}
	}

	if req.Email != nil && *req.Email != v1.GetUserEmail(targetUser) ||
		req.Name != nil && *req.Name != v1.GetUserName(targetUser) ||
		req.AvatarUrl != nil && *req.AvatarUrl != v1.GetUserAvatarUrl(targetUser) ||
		req.Password != nil && *req.Password != stringutil.Base64Decode(targetUser.Spec.Password) {
		if err = h.authUser(c, requestUser, targetUser,
			commonuser.GetWorkspace(targetUser), "", roles, v1.UpdateVerb); err != nil {
			return false, err
		}
		isChanged = true
	}
	return isChanged, nil
}

func (h *Handler) authUser(c *gin.Context, requestUser, targetUser *v1.User,
	workspaces []string, kind string, roles []*v1.Role, verb v1.RoleVerb) error {
	if err := h.auth.Authorize(authority.Input{
		GinContext:   c,
		ResourceKind: kind,
		Resource:     targetUser,
		Verb:         verb,
		Workspaces:   workspaces,
		User:         requestUser,
		Roles:        roles,
	}); err != nil {
		return err
	}
	return nil
}

func (h *Handler) deleteUser(c *gin.Context) (interface{}, error) {
	targetUser, err := h.getTargetUser(c.Request.Context(), c.GetString(types.Name))
	if err != nil {
		return nil, err
	}
	if err = h.auth.Authorize(authority.Input{
		GinContext: c,
		Resource:   targetUser,
		Verb:       v1.DeleteVerb,
	}); err != nil {
		return nil, err
	}
	if err = h.Delete(c.Request.Context(), targetUser); err != nil {
		return nil, err
	}
	return nil, nil
}

func (h *Handler) getTargetUser(ctx context.Context, name string) (*v1.User, error) {
	if name == "" {
		return nil, commonerrors.NewBadRequest("the userId is empty")
	}
	user := &v1.User{}
	err := h.Get(ctx, client.ObjectKey{Name: name}, user)
	if err != nil {
		klog.ErrorS(err, "failed to get user")
		return nil, err
	}
	return user, nil
}

func (h *Handler) login(c *gin.Context) (interface{}, error) {
	query, err := parseLoginQuery(c)
	if err != nil {
		return nil, err
	}
	var result *types.UserLoginResponse
	switch query.Type {
	case v1.TeamsUser:
	default:
		result, err = h.loginByPassword(c, query)
	}
	if err != nil {
		return nil, err
	}
	if result != nil {
		klog.Infof("user login successfully, userName: %s, userId: %s", result.Name, result.Id)
	}
	return result, nil
}

func (h *Handler) loginByPassword(c *gin.Context, query *types.UserLoginRequest) (*types.UserLoginResponse, error) {
	if query.Id == "" {
		return nil, commonerrors.NewBadRequest("the userId is empty")
	}
	if query.Password == "" {
		return nil, commonerrors.NewBadRequest("the password is empty")
	}

	user := &v1.User{}
	var err error
	if err = h.Get(c.Request.Context(), client.ObjectKey{Name: query.Id}, user); err != nil {
		return nil, commonerrors.NewUserNotRegistered(query.Id)
	}
	if user.Spec.Password != stringutil.Base64Encode(query.Password) {
		return nil, commonerrors.NewUnauthorized("the password is incorrect")
	}

	userInfo := &types.UserLoginResponse{
		UserResponseItem: types.UserResponseItem{
			Id:        query.Id,
			Name:      v1.GetUserName(user),
			Roles:     user.Spec.Roles,
			AvatarUrl: v1.GetUserAvatarUrl(user),
			Type:      v1.DefaultUser,
			Email:     v1.GetUserEmail(user),
		},
	}
	if commonconfig.GetUserTokenExpire() < 0 {
		userInfo.Expire = -1
	} else {
		userInfo.Expire = time.Now().Unix() + int64(commonconfig.GetUserTokenExpire())
	}
	userInfo.Token, err = authority.GenerateToken(authority.TokenItem{
		UserId:   userInfo.Id,
		Expire:   userInfo.Expire,
		UserType: string(userInfo.Type),
	})
	if err != nil {
		klog.ErrorS(err, "failed to build user token")
		return nil, err
	}
	userInfo.Token = stringutil.Base64Encode(userInfo.Token)
	if query.IsFromConsole {
		setCookie(c, userInfo)
	}
	return userInfo, nil
}

func setCookie(c *gin.Context, userInfo *types.UserLoginResponse) {
	maxAge := 0
	switch {
	case userInfo.Expire < 0:
		maxAge = MaxTokenAge
	case userInfo.Expire > 0:
		maxAge = int(userInfo.Expire - time.Now().Unix())
	default:
	}
	domain := commonconfig.GetDomain()
	c.SetCookie(authority.CookieToken, userInfo.Token, maxAge, "/", domain, false, true)
}

func (h *Handler) cvtToUserResponseItem(ctx context.Context, user *v1.User) types.UserResponseItem {
	result := types.UserResponseItem{
		Id:             user.Name,
		Name:           v1.GetUserName(user),
		Email:          v1.GetUserEmail(user),
		Type:           user.Spec.Type,
		Roles:          user.Spec.Roles,
		CreatedTime:    user.CreationTimestamp.Format(time.DateTime),
		RestrictedType: user.Spec.RestrictedType,
		AvatarUrl:      v1.GetUserAvatarUrl(user),
	}
	if !user.IsSystemAdmin() {
		workspaces := commonuser.GetWorkspace(user)
		for _, id := range workspaces {
			workspace := &v1.Workspace{}
			if err := h.Get(ctx, client.ObjectKey{Name: id}, workspace); err != nil {
				continue
			}
			result.Workspaces = append(result.Workspaces, types.WorkspaceEntry{
				Id: id, Name: v1.GetDisplayName(workspace),
			})
		}
		workspaces = commonuser.GetManagedWorkspace(user)
		for _, id := range workspaces {
			workspace := &v1.Workspace{}
			if err := h.Get(ctx, client.ObjectKey{Name: id}, workspace); err != nil {
				continue
			}
			result.ManagedWorkspaces = append(result.ManagedWorkspaces, v1.GetDisplayName(workspace))
		}
	}
	return result
}

// only for request from console
func (h *Handler) logout(c *gin.Context) (interface{}, error) {
	info := &types.UserLoginResponse{}
	setCookie(c, info)
	return nil, nil
}

func parseCreateUserQuery(c *gin.Context) (*types.CreateUserRequest, error) {
	req := &types.CreateUserRequest{}
	body, err := getBodyFromRequest(c.Request, req)
	if err != nil {
		klog.ErrorS(err, "fail to getBodyFromRequest", "body", string(body))
		return nil, err
	}
	if req.Type == "" {
		req.Type = v1.DefaultUser
	}
	return req, nil
}

func parseLoginQuery(c *gin.Context) (*types.UserLoginRequest, error) {
	req := &types.UserLoginRequest{}
	contentType := c.ContentType()
	if contentType == FormContent {
		req.Type = v1.UserType(c.PostForm("userType"))
		req.Id = c.PostForm("userId")
		req.Password = c.PostForm("password")
		req.IsFromConsole = true
	} else {
		_, err := getBodyFromRequest(c.Request, req)
		if err != nil {
			return nil, commonerrors.NewBadRequest("invalid query: " + err.Error())
		}
		req.IsFromConsole = false
	}
	return req, nil
}

func parseListUserQuery(c *gin.Context) (*types.ListUserRequest, error) {
	query := &types.ListUserRequest{}
	if err := c.ShouldBindWith(&query, binding.Query); err != nil {
		return nil, commonerrors.NewBadRequest("invalid query: " + err.Error())
	}
	return query, nil
}

func buildListUserSelector(query *types.ListUserRequest) labels.Selector {
	var labelSelector = labels.NewSelector()
	if query.Name != "" {
		name := query.Name
		if unescape, err := url.QueryUnescape(name); err == nil {
			name = unescape
		}
		nameMd5 := stringutil.MD5(name)
		req, _ := labels.NewRequirement(v1.UserNameMd5Label, selection.Equals, []string{nameMd5})
		labelSelector = labelSelector.Add(*req)
	}
	if query.Email != "" {
		email := query.Email
		if unescape, err := url.QueryUnescape(email); err == nil {
			email = unescape
		}
		emailMd5 := stringutil.MD5(email)
		req, _ := labels.NewRequirement(v1.UserEmailMd5Label, selection.Equals, []string{emailMd5})
		labelSelector = labelSelector.Add(*req)
	}
	return labelSelector
}
