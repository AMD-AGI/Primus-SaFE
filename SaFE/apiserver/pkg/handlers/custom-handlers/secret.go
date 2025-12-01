/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package custom_handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/custom-handlers/types"
	apiutils "github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/utils"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	commonsecret "github.com/AMD-AIG-AIMA/SAFE/common/pkg/secret"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/backoff"
	jsonutils "github.com/AMD-AIG-AIMA/SAFE/utils/pkg/json"
	sliceutil "github.com/AMD-AIG-AIMA/SAFE/utils/pkg/slice"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/stringutil"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/timeutil"
)

// CreateSecret handles the creation of a new secret resource.
// It authorizes the request, parses the creation request, generates a secret object,
// creates it in the Kubernetes cluster, and updates associated workspace secrets.
// Returns the created secret ID on success.
func (h *Handler) CreateSecret(c *gin.Context) {
	handle(c, h.createSecret)
}

// ListSecret handles listing secret resources with filtering capabilities.
// It retrieves secrets based on query parameters, applies authorization filtering,
// and returns them in a sorted list.
func (h *Handler) ListSecret(c *gin.Context) {
	handle(c, h.listSecret)
}

// GetSecret retrieves detailed information about a specific secret.
// It authorizes the request and returns the secret's complete information.
func (h *Handler) GetSecret(c *gin.Context) {
	handle(c, h.getSecret)
}

// PatchSecret handles partial updates to a secret resource.
// It authorizes the request, parses update parameters, applies changes,
// and updates the secret along with associated cluster and workspace resources.
func (h *Handler) PatchSecret(c *gin.Context) {
	handle(c, h.patchSecret)
}

// DeleteSecret handles deletion of a secret resource.
// It authorizes the request, removes the secret from the Kubernetes cluster,
// and cleans up references in associated clusters and workspaces.
func (h *Handler) DeleteSecret(c *gin.Context) {
	handle(c, h.deleteSecret)
}

// createSecret handles the HTTP request for creating a new secret.
// It extracts the user context, parses the creation request, and delegates to createSecretImpl
// to perform the actual secret creation logic.
func (h *Handler) createSecret(c *gin.Context) (interface{}, error) {
	requestUser, err := h.getAndSetUsername(c)
	if err != nil {
		return nil, err
	}
	req, err := parseCreateSecretRequest(c)
	if err != nil {
		klog.ErrorS(err, "failed to parse request")
		return nil, commonerrors.NewBadRequest(err.Error())
	}
	secret, err := h.createSecretImpl(c.Request.Context(), req, requestUser)
	if err != nil {
		return nil, err
	}
	return &types.CreateSecretResponse{
		SecretId: secret.Name,
	}, nil
}

// createSecretImpl performs the core logic for secret creation.
// It authorizes the user, generates the secret object based on request parameters,
// creates the secret in the Kubernetes cluster, and returns the created secret ID.
func (h *Handler) createSecretImpl(ctx context.Context, req *types.CreateSecretRequest, requestUser *v1.User) (*corev1.Secret, error) {
	if err := h.accessController.Authorize(authority.AccessInput{
		Context:      ctx,
		ResourceKind: authority.SecretResourceKind,
		Verb:         v1.CreateVerb,
		User:         requestUser,
		Workspaces:   req.WorkspaceIds,
	}); err != nil {
		return nil, err
	}

	secret, err := generateSecret(req, requestUser)
	if err != nil {
		klog.ErrorS(err, "failed to generate secret")
		return nil, err
	}
	if secret, err = h.clientSet.CoreV1().Secrets(common.PrimusSafeNamespace).Create(
		ctx, secret, metav1.CreateOptions{}); err != nil {
		klog.ErrorS(err, "failed to create secret")
		return nil, err
	}
	klog.Infof("created secret %s", secret.Name)
	return secret, nil
}

// listSecret implements the secret listing logic.
// Parses query parameters, builds label selectors, retrieves secrets from the cluster,
// applies authorization filtering, sorts them, and converts to response format.
func (h *Handler) listSecret(c *gin.Context) (interface{}, error) {
	requestUser, err := h.getAndSetUsername(c)
	if err != nil {
		return nil, err
	}

	query := &types.ListSecretRequest{}
	if err = c.ShouldBindWith(&query, binding.Query); err != nil {
		return nil, commonerrors.NewBadRequest("invalid query: " + err.Error())
	}
	labelSelector := buildSecretLabelSelector(query)
	secretList := &corev1.SecretList{}
	if err = h.List(c.Request.Context(), secretList,
		&client.ListOptions{LabelSelector: labelSelector, Namespace: common.PrimusSafeNamespace}); err != nil {
		return nil, err
	}
	result := &types.ListSecretResponse{}
	roles := h.accessController.GetRoles(c.Request.Context(), requestUser)
	for _, item := range secretList.Items {
		workspaceIds := commonsecret.GetSecretWorkspaces(&item)
		if query.WorkspaceId != nil {
			if *query.WorkspaceId == "" {
				if len(workspaceIds) > 0 {
					continue
				}
			} else if !sliceutil.Contains(workspaceIds, *query.WorkspaceId) {
				continue
			}
			workspaceIds = []string{*query.WorkspaceId}
		}
		if err = h.accessController.Authorize(authority.AccessInput{
			Context:      c.Request.Context(),
			Resource:     &item,
			ResourceKind: authority.SecretResourceKind,
			Verb:         v1.ListVerb,
			User:         requestUser,
			Roles:        roles,
			Workspaces:   workspaceIds,
		}); err != nil {
			continue
		}
		result.Items = append(result.Items, cvtToSecretResponseItem(&item))
	}
	sort.Slice(result.Items, func(i, j int) bool {
		return result.Items[i].SecretId < result.Items[j].SecretId
	})
	result.TotalCount = len(result.Items)
	return result, nil
}

// getSecret implements the logic for retrieving a single secret's information.
// Authorizes the request and retrieves the secret by name from the cluster.
func (h *Handler) getSecret(c *gin.Context) (interface{}, error) {
	requestUser, err := h.getAndSetUsername(c)
	if err != nil {
		return nil, err
	}
	secret, err := h.getAndAuthorizeSecret(c.Request.Context(),
		c.GetString(common.Name), "", requestUser, v1.GetVerb)
	if err != nil {
		return nil, err
	}
	return cvtToGetSecretResponse(secret), nil
}

// patchSecret implements partial update logic for a secret.
// Parses the patch request, applies specified changes, updates the secret in the cluster,
// and synchronizes changes with associated cluster and workspace resources.
func (h *Handler) patchSecret(c *gin.Context) (interface{}, error) {
	req := &types.PatchSecretRequest{}
	body, err := apiutils.ParseRequestBody(c.Request, req)
	if err != nil {
		klog.ErrorS(err, "failed to parse request", "body", string(body))
		return nil, commonerrors.NewBadRequest(err.Error())
	}
	name := c.GetString(common.Name)
	secret, err := h.getAdminSecret(c.Request.Context(), name)
	if err != nil {
		return nil, err
	}
	if err = h.authSecretUpdate(c, req, secret); err != nil {
		return nil, err
	}

	if err = backoff.ConflictRetry(func() error {
		var innerError error
		if innerError = modifySecret(secret, req); innerError != nil {
			return innerError
		}
		if innerError = h.Update(c.Request.Context(), secret); innerError == nil {
			return nil
		} else {
			if apierrors.IsConflict(innerError) {
				if secret, _ = h.getAdminSecret(c.Request.Context(), name); secret == nil {
					return commonerrors.NewNotFoundWithMessage(fmt.Sprintf("secret %s not found", name))
				}
			}
			return innerError
		}
	}, defaultRetryCount, defaultRetryDelay); err != nil {
		klog.ErrorS(err, "failed to update secret", "name", secret.Name)
		return nil, err
	}
	return nil, nil
}

// authSecretUpdate validates user authorization for updating a secret.
// Checks both update permission on the secret and create permission if workspace IDs are being modified.
func (h *Handler) authSecretUpdate(c *gin.Context, req *types.PatchSecretRequest, secret *corev1.Secret) error {
	requestUser, err := h.getAndSetUsername(c)
	if err != nil {
		return err
	}

	roles := h.accessController.GetRoles(c.Request.Context(), requestUser)
	if err = h.accessController.Authorize(authority.AccessInput{
		Context:      c.Request.Context(),
		Resource:     secret,
		ResourceKind: authority.SecretResourceKind,
		Verb:         v1.UpdateVerb,
		Workspaces:   commonsecret.GetSecretWorkspaces(secret),
		User:         requestUser,
		Roles:        roles,
	}); err != nil {
		return err
	}
	if req.WorkspaceIds != nil {
		currentWorkspaceIds := commonsecret.GetSecretWorkspaces(secret)
		workspaceIdsToAdd := sliceutil.Difference(*req.WorkspaceIds, currentWorkspaceIds)
		if err = h.accessController.Authorize(authority.AccessInput{
			Context:      c.Request.Context(),
			ResourceKind: authority.SecretResourceKind,
			Verb:         v1.CreateVerb,
			Workspaces:   workspaceIdsToAdd,
			User:         requestUser,
			Roles:        roles,
		}); err != nil {
			return err
		}
	}
	return nil
}

// modifySecret applies updates to a secret based on the patch request.
func modifySecret(secret *corev1.Secret, req *types.PatchSecretRequest) error {
	if req.Params != nil {
		reqType := v1.SecretType(v1.GetSecretType(secret))
		if err := buildSecretData(reqType, *req.Params, secret); err != nil {
			return commonerrors.NewBadRequest(err.Error())
		}
	}
	if req.WorkspaceIds != nil {
		v1.SetAnnotation(secret, v1.WorkspaceIdsAnnotation, string(jsonutils.MarshalSilently(*req.WorkspaceIds)))
	}
	return nil
}

// deleteSecret implements secret deletion logic.
// Removes the secret from the Kubernetes cluster and cleans up references
// in associated clusters and workspaces.
func (h *Handler) deleteSecret(c *gin.Context) (interface{}, error) {
	name := c.GetString(common.Name)
	secret, err := h.getAdminSecret(c.Request.Context(), name)
	if err != nil {
		return nil, err
	}
	if err = h.accessController.Authorize(authority.AccessInput{
		Context:      c.Request.Context(),
		Resource:     secret,
		ResourceKind: authority.SecretResourceKind,
		Verb:         v1.DeleteVerb,
		UserId:       c.GetString(common.UserId),
		Workspaces:   commonsecret.GetSecretWorkspaces(secret),
	}); err != nil {
		return nil, err
	}
	if err = h.clientSet.CoreV1().Secrets(common.PrimusSafeNamespace).Delete(
		c.Request.Context(), name, metav1.DeleteOptions{}); err != nil {
		return nil, err
	}
	klog.Infof("delete secret %s", name)
	return nil, nil
}

// getAdminSecret retrieves a secret resource by name without authorization.
// Returns the secret object or an error if retrieval fails.
func (h *Handler) getAdminSecret(ctx context.Context, name string) (*corev1.Secret, error) {
	if name == "" {
		return nil, commonerrors.NewBadRequest("the secretId is empty")
	}
	secret, err := h.clientSet.CoreV1().Secrets(common.PrimusSafeNamespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		klog.ErrorS(err, "failed to get secret")
		return nil, err
	}
	return secret, err
}

// getAndAuthorizeSecret retrieves a secret by name and performs authorization check with specified verb
// If a workspace is set, validate permissions only on that workspace; otherwise, validate across all workspaces the secret belongs to.
func (h *Handler) getAndAuthorizeSecret(ctx context.Context,
	name, workspaceId string, requestUser *v1.User, verb v1.RoleVerb) (*corev1.Secret, error) {
	secret, err := h.getAdminSecret(ctx, name)
	if err != nil {
		return nil, err
	}
	var workspaceIds []string
	if workspaceId != "" {
		workspaceIds = []string{workspaceId}
	} else {
		workspaceIds = commonsecret.GetSecretWorkspaces(secret)
	}
	if err = h.accessController.Authorize(authority.AccessInput{
		Context:      ctx,
		Resource:     secret,
		ResourceKind: authority.SecretResourceKind,
		Verb:         verb,
		User:         requestUser,
		Workspaces:   workspaceIds,
	}); err != nil {
		return nil, err
	}
	return secret, nil
}

// generateSecret creates a new secret object based on the creation request.
// Validates the request parameters and populates the secret metadata and data.
func generateSecret(req *types.CreateSecretRequest, requestUser *v1.User) (*corev1.Secret, error) {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      req.Name,
			Namespace: common.PrimusSafeNamespace,
			Labels: map[string]string{
				v1.SecretTypeLabel: string(req.Type),
				v1.UserIdLabel:     requestUser.Name,
			},
			Annotations: map[string]string{
				v1.UserNameAnnotation: v1.GetUserName(requestUser),
			},
		},
	}
	if req.Owner != "" {
		v1.SetLabel(secret, v1.OwnerLabel, req.Owner)
	}
	if err := buildSecretData(req.Type, req.Params, secret); err != nil {
		return nil, commonerrors.NewBadRequest(err.Error())
	}
	if req.Name != "" {
		v1.SetLabel(secret, v1.DisplayNameLabel, req.Name)
	}
	if len(req.WorkspaceIds) > 0 {
		v1.SetAnnotation(secret, v1.WorkspaceIdsAnnotation, string(jsonutils.MarshalSilently(req.WorkspaceIds)))
	}
	controllerutil.AddFinalizer(secret, v1.SecretFinalizer)
	return secret, nil
}

// buildSecretData constructs the secret data based on the secret type and parameters.
// Handles different secret types (image, SSH) and formats the data appropriately.
func buildSecretData(reqType v1.SecretType, allParams []map[types.SecretParam]string, secret *corev1.Secret) error {
	var secretType corev1.SecretType
	data := make(map[string][]byte)

	switch reqType {
	case v1.SecretImage:
		keys := []types.SecretParam{types.PasswordParam, types.UserNameParam, types.ServerParam}
		secretType = corev1.SecretTypeDockerConfigJson
		dockerConf := types.DockerConfig{}
		dockerConf.Auths = make(map[string]types.DockerConfigItem)
		for _, params := range allParams {
			for _, key := range keys {
				if !existKey(params, key) {
					return fmt.Errorf("the %s is empty", key)
				}
			}
			password := stringutil.Base64Decode(params[types.PasswordParam])
			auth := stringutil.Base64Encode(fmt.Sprintf("%s:%s", params[types.UserNameParam], password))
			dockerConf.Auths[params[types.ServerParam]] = types.DockerConfigItem{
				UserName: params[types.UserNameParam],
				Password: password,
				Auth:     auth,
			}
		}
		data[types.DockerConfigJson] = jsonutils.MarshalSilently(dockerConf)
	case v1.SecretSSH:
		if len(allParams) == 0 {
			return fmt.Errorf("the input params are empty")
		}
		params := allParams[0]
		if !existKey(params, types.UserNameParam) {
			return fmt.Errorf("the %s is empty", types.UserNameParam)
		}
		secretType = corev1.SecretTypeOpaque
		data[string(types.UserNameParam)] = []byte(params[types.UserNameParam])
		if val, _ := params[types.PasswordParam]; val != "" {
			data[string(types.PasswordParam)] = []byte(stringutil.Base64Decode(params[types.PasswordParam]))
		} else if existKey(params, types.PublicKeyParam) && existKey(params, types.PrivateKeyParam) {
			data[types.SSHAuthKey] = []byte(stringutil.Base64Decode(params[types.PrivateKeyParam]))
			data[types.SSHAuthPubKey] = []byte(stringutil.Base64Decode(params[types.PublicKeyParam]))
		} else {
			return fmt.Errorf("the password or keypair is empty")
		}
	case v1.SecretGeneral:
		secretType = corev1.SecretTypeOpaque
		if len(allParams) == 0 {
			return fmt.Errorf("the input params are empty")
		}
		params := allParams[0]
		for k, v := range params {
			data[string(k)] = []byte(stringutil.Base64Decode(v))
		}
	default:
		return fmt.Errorf("the secret type %s is not supported", reqType)
	}
	secret.Data = data
	secret.Type = secretType
	return nil
}

// existKey checks if a key exists in the parameters map and has a non-empty value.
func existKey(params map[types.SecretParam]string, key types.SecretParam) bool {
	val, _ := params[key]
	return val != ""
}

// buildSecretLabelSelector constructs a label selector based on query parameters.
// Used to filter secrets by type criteria.
func buildSecretLabelSelector(query *types.ListSecretRequest) labels.Selector {
	var labelSelector = labels.NewSelector()
	if query.Type != "" {
		typeList := strings.Split(query.Type, ",")
		req, _ := labels.NewRequirement(v1.SecretTypeLabel, selection.In, typeList)
		labelSelector = labelSelector.Add(*req)
	}
	return labelSelector
}

// cvtToSecretResponseItem converts a secret object to a response item format.
func cvtToSecretResponseItem(secret *corev1.Secret) types.SecretResponseItem {
	result := types.SecretResponseItem{
		SecretId:     secret.Name,
		SecretName:   v1.GetDisplayName(secret),
		WorkspaceIds: commonsecret.GetSecretWorkspaces(secret),
		Type:         v1.GetSecretType(secret),
		CreationTime: timeutil.FormatRFC3339(secret.CreationTimestamp.Time),
		UserId:       v1.GetUserId(secret),
		UserName:     v1.GetUserName(secret),
	}
	return result
}

// cvtToGetSecretResponse converts a secret object to a response format.
// Maps the secret data to the appropriate response structure with proper value handling.
func cvtToGetSecretResponse(secret *corev1.Secret) types.GetSecretResponse {
	result := types.GetSecretResponse{
		SecretResponseItem: cvtToSecretResponseItem(secret),
	}
	switch result.Type {
	case string(v1.SecretImage):
		dockerConf := &types.DockerConfig{}
		if json.Unmarshal(secret.Data[types.DockerConfigJson], dockerConf) == nil {
			result.Params = make([]map[types.SecretParam]string, 0, len(dockerConf.Auths))
			for k, v := range dockerConf.Auths {
				params := make(map[types.SecretParam]string)
				params[types.ServerParam] = k
				params[types.UserNameParam] = v.UserName
				params[types.PasswordParam] = stringutil.Base64Encode(v.Password)
				result.Params = append(result.Params, params)
			}
		}
	case string(v1.SecretSSH):
		result.Params = make([]map[types.SecretParam]string, 0, 1)
		params := make(map[types.SecretParam]string)
		params[types.UserNameParam] = string(secret.Data[string(types.UserNameParam)])
		if pwd, ok := secret.Data[string(types.PasswordParam)]; ok && len(pwd) > 0 {
			params[types.PasswordParam] = stringutil.Base64Encode(string(pwd))
		} else {
			if priv := secret.Data[types.SSHAuthKey]; len(priv) > 0 {
				params[types.PrivateKeyParam] = stringutil.Base64Encode(string(priv))
			}
			if pub := secret.Data[types.SSHAuthPubKey]; len(pub) > 0 {
				params[types.PublicKeyParam] = stringutil.Base64Encode(string(pub))
			}
		}
		result.Params = append(result.Params, params)
	case string(v1.SecretGeneral):
		result.Params = make([]map[types.SecretParam]string, 0, len(secret.Data))
		params := make(map[types.SecretParam]string)
		for k, v := range secret.Data {
			params[types.SecretParam(k)] = string(v)
		}
		result.Params = append(result.Params, params)
	}
	return result
}

// parseCreateSecretRequest parses and validates the request for creating a secret.
// It ensures required fields like name, type, and inputs are provided.
func parseCreateSecretRequest(c *gin.Context) (*types.CreateSecretRequest, error) {
	req := &types.CreateSecretRequest{}
	_, err := apiutils.ParseRequestBody(c.Request, req)
	if err != nil {
		return nil, commonerrors.NewBadRequest(err.Error())
	}
	if req.Name == "" || req.Type == "" || len(req.Params) == 0 {
		return nil, commonerrors.NewBadRequest("the name, type and params of request are required")
	}
	return req, nil
}
