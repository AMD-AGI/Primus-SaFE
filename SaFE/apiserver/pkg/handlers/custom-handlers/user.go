/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package custom_handlers

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/custom-handlers/types"
	apiutils "github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/utils"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	commonuser "github.com/AMD-AIG-AIMA/SAFE/common/pkg/user"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/backoff"
	jsonutils "github.com/AMD-AIG-AIMA/SAFE/utils/pkg/json"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/slice"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/stringutil"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/timeutil"
)

const (
	ContentTypeForm = "application/x-www-form-urlencoded"
	// The lifecycle of the user token
	MaxCookieAgeSeconds = 3600 * 24 * 365
)

// CreateUser handles the creation of a new user resource.
// It parses the creation request, generates a user object based on the requester's permissions,
// and persists it in the k8s cluster. Returns the created user ID on success.
func (h *Handler) CreateUser(c *gin.Context) {
	handle(c, h.createUser)
}

// ListUser handles listing user resources with filtering capabilities.
// It retrieves users based on query parameters, applies authorization filtering,
// and returns them in a sorted list with information about workspaces
// that the user can access or manage.
func (h *Handler) ListUser(c *gin.Context) {
	handle(c, h.listUser)
}

// GetUser retrieves detailed information about a specific user with appropriate authorization checks.
func (h *Handler) GetUser(c *gin.Context) {
	handle(c, h.getUser)
}

// PatchUser handles partial updates to a user resource.
// It authorizes the request based on the specific fields being updated,
// parses update parameters, and applies changes to the specified user.
func (h *Handler) PatchUser(c *gin.Context) {
	handle(c, h.patchUser)
}

// DeleteUser handles deletion of a user resource.
// It authorizes the request and removes the specified user from the system.
func (h *Handler) DeleteUser(c *gin.Context) {
	handle(c, h.deleteUser)
}

// Login handles user authentication and token generation.
// Supports different login types and generates authentication tokens for successful logins.
// Sets cookies for console-based logins.
func (h *Handler) Login(c *gin.Context) {
	handle(c, h.login)
}

// Logout handles user logout by clearing authentication cookies.
// Only applicable for requests from the console interface.
func (h *Handler) Logout(c *gin.Context) {
	handle(c, h.logout)
}

// createUser implements the user creation logic.
// Parses the request, generates a user object with appropriate permissions and settings,
// and creates it in the system.
func (h *Handler) createUser(c *gin.Context) (interface{}, error) {
	if commonconfig.IsSSOEnable() {
		return nil, commonerrors.NewInternalError("the user registration is not enabled")
	}
	req := &types.CreateUserRequest{}
	body, err := apiutils.ParseRequestBody(c.Request, req)
	if err != nil {
		klog.ErrorS(err, "fail to parseRequestBody", "body", string(body))
		return nil, err
	}

	user := generateUser(req)
	if err = h.Create(c.Request.Context(), user); err != nil {
		return nil, err
	}
	return &types.CreateUserResponse{Id: user.Name}, nil
}

// generateUser creates a new user object based on the creation request.
// Sets user metadata, roles, and properties based on the requester's permissions.
// Handles password encoding and workspace assignments.
func generateUser(req *types.CreateUserRequest) *v1.User {
	user := &v1.User{
		ObjectMeta: metav1.ObjectMeta{
			Name: commonuser.GenerateUserIdByName(req.Name),
			Annotations: map[string]string{
				v1.UserNameAnnotation:      req.Name,
				v1.UserEmailAnnotation:     req.Email,
				v1.UserAvatarUrlAnnotation: req.AvatarUrl,
			},
		},
		Spec: v1.UserSpec{
			Type: v1.DefaultUserType,
		},
	}
	if req.Password != "" {
		user.Spec.Password = stringutil.Base64Encode(req.Password)
	}
	return user
}

// listUser implements the user listing logic.
// Parses query parameters, builds label selectors, retrieves users from the system,
// applies authorization filtering, sorts them, and converts to response format.
func (h *Handler) listUser(c *gin.Context) (interface{}, error) {
	requestUser, err := h.getAndSetUsername(c)
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
	roles := h.accessController.GetRoles(c.Request.Context(), requestUser)
	for _, item := range userList.Items {
		if query.WorkspaceId != "" {
			if !commonuser.HasWorkspaceRight(&item, query.WorkspaceId) {
				continue
			}
		}
		if h.authUserGet(c, query.WorkspaceId, requestUser, &item, roles, v1.ListVerb) != nil {
			continue
		}
		result.Items = append(result.Items, h.cvtToUserResponseItem(c.Request.Context(), &item))
	}
	result.TotalCount = len(result.Items)
	return result, nil
}

// authUserGet checks if requestUser has permission to list targetUser.
// System admins or workspace admins always have access. For other users, at least one shared workspace
// with ListVerb permission is required.
func (h *Handler) authUserGet(c *gin.Context, targetWorkspace string,
	requestUser, targetUser *v1.User, roles []*v1.Role, verb v1.RoleVerb) error {
	var workspaces []string
	if targetWorkspace != "" {
		workspaces = append(workspaces, targetWorkspace)
	} else {
		workspaces = commonuser.GetWorkspace(targetUser)
	}

	isAuthGranted := false
	if len(workspaces) == 0 {
		if h.authUserAction(c, requestUser, targetUser, nil, "", roles, verb) == nil {
			isAuthGranted = true
		}
	} else {
		// If the requester and target user share any workspace, info can be fetched
		for _, w := range workspaces {
			if h.authUserAction(c, requestUser, targetUser, []string{w}, "", roles, verb) != nil {
				continue
			}
			isAuthGranted = true
			break
		}
	}
	if !isAuthGranted {
		return commonerrors.NewForbidden(
			fmt.Sprintf("The user is not allowed to %s %s", verb, v1.UserKind))
	}
	return nil
}

// getUser implements the logic for retrieving a single user's information.
// Handles self-retrieval and other user retrieval with appropriate authorization checks.
func (h *Handler) getUser(c *gin.Context) (interface{}, error) {
	requestUser, err := h.getAndSetUsername(c)
	if err != nil {
		return nil, err
	}

	var targetUser *v1.User
	targetUserId := c.GetString(common.Name)
	if targetUserId == common.UserSelf {
		targetUser = requestUser
	} else {
		targetUser, err = h.getAdminUser(c.Request.Context(), targetUserId)
		if err != nil {
			return nil, err
		}
	}
	roles := h.accessController.GetRoles(c.Request.Context(), requestUser)
	if err = h.authUserGet(c, "", requestUser, targetUser, roles, v1.GetVerb); err != nil {
		return nil, err
	}
	return h.cvtToUserResponseItem(c.Request.Context(), targetUser), nil
}

// patchUser implements partial update logic for a user.
// Parses the patch request, validates authorization for the changes,
// and applies specified updates to the user.
func (h *Handler) patchUser(c *gin.Context) (interface{}, error) {
	req := &types.PatchUserRequest{}
	body, err := apiutils.ParseRequestBody(c.Request, req)
	if err != nil {
		klog.ErrorS(err, "fail to parse request data", "body", string(body))
		return nil, err
	}

	targetUserId := c.GetString(common.Name)
	targetUser, err := h.getAdminUser(c.Request.Context(), targetUserId)
	if err != nil {
		return nil, err
	}
	isChanged, err := h.authUserUpdate(c, targetUser, req)
	if !isChanged || err != nil {
		return nil, err
	}

	if err = backoff.ConflictRetry(func() error {
		modifyUser(targetUser, req)
		if innerError := h.Update(c.Request.Context(), targetUser); innerError == nil {
			return nil
		} else {
			if apierrors.IsConflict(innerError) {
				targetUser, _ = h.getAdminUser(c.Request.Context(), targetUserId)
				if targetUser == nil {
					return commonerrors.NewNotFoundWithMessage(fmt.Sprintf("user %s not found", targetUserId))
				}
			}
			return innerError
		}
	}, defaultRetryCount, defaultRetryDelay); err != nil {
		klog.ErrorS(err, "failed to update user", "name", targetUser.Name)
		return nil, err
	}
	klog.Infof("patch user, target.user: %s, request.user: %s, request: %s",
		targetUserId, c.GetString(common.UserName), string(jsonutils.MarshalSilently(req)))
	return nil, nil
}

// modifyUser updates the target user with values from the patch request.
// Only modifies fields that are present in the request.
func modifyUser(targetUser *v1.User, req *types.PatchUserRequest) {
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
	if req.Password != nil && *req.Password != "" {
		targetUser.Spec.Password = stringutil.Base64Encode(*req.Password)
	}
	if req.Email != nil {
		v1.SetLabel(targetUser, v1.UserEmailMd5Label, stringutil.MD5(*req.Email))
		v1.SetAnnotation(targetUser, v1.UserEmailAnnotation, *req.Email)
	}
}

// authUserUpdate validates authorization for user patch operations.
// Checks if the requester has permission to make the requested changes
// based on the fields being modified and the target user.
func (h *Handler) authUserUpdate(c *gin.Context, targetUser *v1.User, req *types.PatchUserRequest) (bool, error) {
	requestUser, err := h.getAndSetUsername(c)
	if err != nil {
		return false, err
	}
	roles := h.accessController.GetRoles(c.Request.Context(), requestUser)

	isChanged := false
	if req.RestrictedType != nil && *req.RestrictedType != targetUser.Spec.RestrictedType ||
		req.Roles != nil && !commonuser.IsRolesEqual(*req.Roles, targetUser.Spec.Roles) {
		if err = h.authUserAction(c, requestUser, targetUser,
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
			if err = h.authUserAction(c, requestUser, targetUser,
				workspaces, authority.UserWorkspaceResource, roles, v1.UpdateVerb); err != nil {
				return false, err
			}
			isChanged = true
		}
	}

	if req.Email != nil && *req.Email != v1.GetUserEmail(targetUser) ||
		req.AvatarUrl != nil && *req.AvatarUrl != v1.GetUserAvatarUrl(targetUser) ||
		req.Password != nil && *req.Password != stringutil.Base64Decode(targetUser.Spec.Password) {
		if err = h.authUserAction(c, requestUser, targetUser,
			commonuser.GetWorkspace(targetUser), "", roles, v1.UpdateVerb); err != nil {
			return false, err
		}
		isChanged = true
	}
	return isChanged, nil
}

// authUserAction performs authorization checks for user-related actions.
// Validates if the requesting user has permission to perform the specified action
// on the target user, considering workspaces and resource types.
func (h *Handler) authUserAction(c *gin.Context, requestUser, targetUser *v1.User,
	workspaces []string, kind string, roles []*v1.Role, verb v1.RoleVerb) error {
	if err := h.accessController.Authorize(authority.AccessInput{
		Context:      c.Request.Context(),
		ResourceKind: kind,
		Resource:     targetUser,
		Verb:         verb,
		Workspaces:   workspaces,
		User:         requestUser,
		UserId:       c.GetString(common.UserId),
		Roles:        roles,
	}); err != nil {
		return err
	}
	return nil
}

// deleteUser implements user deletion logic.
// Authorizes the request and removes the specified user from the system.
func (h *Handler) deleteUser(c *gin.Context) (interface{}, error) {
	requestUser, err := h.getAndSetUsername(c)
	if err != nil {
		return nil, err
	}

	targetUser, err := h.getAdminUser(c.Request.Context(), c.GetString(common.Name))
	if err != nil {
		return nil, err
	}
	if err = h.authUserAction(c, requestUser, targetUser,
		commonuser.GetWorkspace(targetUser), "", nil, v1.DeleteVerb); err != nil {
		return nil, err
	}
	if workspaceIds := commonuser.GetManagedWorkspace(targetUser); len(workspaceIds) > 0 {
		for _, id := range workspaceIds {
			if err = h.removeWorkspaceManager(c.Request.Context(), id, targetUser.Name); err != nil {
				if apierrors.IsNotFound(err) {
					continue
				}
				return nil, err
			}
		}
	}
	if err = h.Delete(c.Request.Context(), targetUser); err != nil {
		return nil, err
	}
	return nil, nil
}

// getAdminUser retrieves a user resource by ID from the admin data plane.
// Returns an error if the user doesn't exist or the ID is empty.
func (h *Handler) getAdminUser(ctx context.Context, userId string) (*v1.User, error) {
	if userId == "" {
		return nil, commonerrors.NewBadRequest("the userId is empty")
	}
	user := &v1.User{}
	err := h.Get(ctx, client.ObjectKey{Name: userId}, user)
	if err != nil {
		klog.ErrorS(err, "failed to get user")
		return nil, err
	}
	return user, nil
}

// login implements user authentication logic.
// Handles different user types and performs authentication based on the request type.
func (h *Handler) login(c *gin.Context) (interface{}, error) {
	query, err := parseLoginQuery(c)
	if err != nil {
		return nil, err
	}
	var tokenInstance authority.TokenInterface
	if query.Type == v1.SSOUserType {
		if !commonconfig.IsSSOEnable() {
			return nil, commonerrors.NewInternalError("SSO is not enabled")
		}
		tokenInstance = authority.SSOInstance()
	} else {
		tokenInstance = authority.DefaultTokenInstance()
	}
	if tokenInstance == nil {
		return nil, commonerrors.NewInternalError("failed to get token instance")
	}

	tokenInput := authority.TokenInput{
		Code:     query.Code,
		Username: query.Name,
		Password: query.Password,
	}
	user, resp, err := tokenInstance.Login(c.Request.Context(), tokenInput)
	if err != nil {
		klog.ErrorS(err, "user login failed", "userName", query.Name, "code", query.Code)
		return nil, err
	}
	result := &types.UserLoginResponse{
		Expire:           resp.Expire,
		Token:            resp.Token,
		UserResponseItem: h.cvtToUserResponseItem(c.Request.Context(), user),
	}
	if query.IsFromConsole {
		setCookie(c, result, query.Type)
	}
	klog.Infof("user login successfully, userName: %s, userId: %s", result.Name, result.Id)
	return result, nil
}

// setCookie sets authentication cookies for logged-in users.
// Configures cookie parameters including expiration time and domain based on user information.
func setCookie(c *gin.Context, userInfo *types.UserLoginResponse, userType v1.UserType) {
	maxAge := 0
	switch {
	case userInfo.Expire < 0:
		maxAge = MaxCookieAgeSeconds
	case userInfo.Expire > 0:
		maxAge = int(userInfo.Expire - time.Now().Unix())
	default:
	}
	domain := "." + c.Request.Host
	c.SetCookie(authority.CookieToken, userInfo.Token, maxAge, "/", domain, false, true)
	c.SetCookie(common.UserId, userInfo.Id, maxAge, "/", domain, false, true)
	c.SetCookie(common.UserType, string(userType), maxAge, "/", domain, false, true)
}

// cvtToUserResponseItem converts a user object to a response item format.
// Maps user properties to the appropriate response structure and includes
// workspace information which user can access or manage
func (h *Handler) cvtToUserResponseItem(ctx context.Context, user *v1.User) types.UserResponseItem {
	result := types.UserResponseItem{
		Id:             user.Name,
		Name:           v1.GetUserName(user),
		Email:          v1.GetUserEmail(user),
		Type:           user.Spec.Type,
		Roles:          user.Spec.Roles,
		CreationTime:   timeutil.FormatRFC3339(user.CreationTimestamp.Time),
		RestrictedType: user.Spec.RestrictedType,
		AvatarUrl:      v1.GetUserAvatarUrl(user),
	}
	if !user.IsSystemAdmin() && !user.IsSystemAdminReadonly() {
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
	}
	if !user.IsSystemAdmin() {
		workspaces := commonuser.GetManagedWorkspace(user)
		for _, id := range workspaces {
			workspace := &v1.Workspace{}
			if err := h.Get(ctx, client.ObjectKey{Name: id}, workspace); err != nil {
				continue
			}
			result.ManagedWorkspaces = append(result.ManagedWorkspaces, types.WorkspaceEntry{
				Id: id, Name: v1.GetDisplayName(workspace),
			})
		}
	}
	return result
}

// logout handles user logout by clearing authentication cookies.
// Only applicable for requests from the console interface.
func (h *Handler) logout(c *gin.Context) (interface{}, error) {
	info := &types.UserLoginResponse{}
	setCookie(c, info, "")
	return nil, nil
}

// parseLoginQuery parses and validates the user login request.
// Handles both form-encoded and JSON request formats.
func parseLoginQuery(c *gin.Context) (*types.UserLoginRequest, error) {
	req := &types.UserLoginRequest{}
	contentType := c.ContentType()
	if contentType == ContentTypeForm {
		req.Type = v1.UserType(c.PostForm("type"))
		req.Name = c.PostForm("name")
		req.Password = c.PostForm("password")
		req.Code = c.PostForm("code")
		req.IsFromConsole = true
	} else {
		_, err := apiutils.ParseRequestBody(c.Request, req)
		if err != nil {
			return nil, commonerrors.NewBadRequest("invalid query: " + err.Error())
		}
		req.IsFromConsole = false
	}
	return req, nil
}

// parseListUserQuery parses and validates the query parameters for listing users.
func parseListUserQuery(c *gin.Context) (*types.ListUserRequest, error) {
	query := &types.ListUserRequest{}
	if err := c.ShouldBindWith(&query, binding.Query); err != nil {
		return nil, commonerrors.NewBadRequest("invalid query: " + err.Error())
	}
	return query, nil
}

// buildListUserSelector constructs a label selector based on user list query parameters.
// Used to filter users by name or email criteria.
func buildListUserSelector(query *types.ListUserRequest) labels.Selector {
	var labelSelector = labels.NewSelector()
	if query.Name != "" {
		name := queryUnescape(query.Name)
		req, _ := labels.NewRequirement(v1.UserNameMd5Label, selection.Equals, []string{stringutil.MD5(name)})
		labelSelector = labelSelector.Add(*req)
	}
	if query.Email != "" {
		email := queryUnescape(query.Email)
		req, _ := labels.NewRequirement(v1.UserEmailMd5Label, selection.Equals, []string{stringutil.MD5(email)})
		labelSelector = labelSelector.Add(*req)
	}
	return labelSelector
}

// queryUnescape unescapes URL-encoded query parameters.
// Returns the unescaped string or the original string if unescaping fails.
func queryUnescape(input string) string {
	if unescape, err := url.QueryUnescape(input); err == nil {
		return unescape
	}
	return input
}
