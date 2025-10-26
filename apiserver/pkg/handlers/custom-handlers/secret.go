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
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	corev1 "k8s.io/api/core/v1"
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
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/backoff"
	jsonutils "github.com/AMD-AIG-AIMA/SAFE/utils/pkg/json"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/stringutil"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/timeutil"
)

// CreateSecret: handles the creation of a new secret resource.
// It authorizes the request, parses the creation request, generates a secret object,
// creates it in the Kubernetes cluster, and updates associated workspace secrets.
// Returns the created secret ID on success.
func (h *Handler) CreateSecret(c *gin.Context) {
	handle(c, h.createSecret)
}

// ListSecret: handles listing secret resources with filtering capabilities.
// It retrieves secrets based on query parameters, applies authorization filtering,
// and returns them in a sorted list.
func (h *Handler) ListSecret(c *gin.Context) {
	handle(c, h.listSecret)
}

// GetSecret: retrieves detailed information about a specific secret.
// It authorizes the request and returns the secret's complete information.
func (h *Handler) GetSecret(c *gin.Context) {
	handle(c, h.getSecret)
}

// PatchSecret: handles partial updates to a secret resource.
// It authorizes the request, parses update parameters, applies changes,
// and updates the secret along with associated cluster and workspace resources.
func (h *Handler) PatchSecret(c *gin.Context) {
	handle(c, h.patchSecret)
}

// DeleteSecret: handles deletion of a secret resource.
// It authorizes the request, removes the secret from the Kubernetes cluster,
// and cleans up references in associated clusters and workspaces.
func (h *Handler) DeleteSecret(c *gin.Context) {
	handle(c, h.deleteSecret)
}

// createSecret: implements the secret creation logic.
// Validates the request, generates a secret object, creates it in the cluster,
// and updates workspace secret associations.
func (h *Handler) createSecret(c *gin.Context) (interface{}, error) {
	if err := h.auth.Authorize(authority.Input{
		Context:      c.Request.Context(),
		ResourceKind: authority.SecretResourceKind,
		Verb:         v1.CreateVerb,
		UserId:       c.GetString(common.UserId),
	}); err != nil {
		return nil, err
	}

	req := &types.CreateSecretRequest{}
	body, err := apiutils.ParseRequestBody(c.Request, req)
	if err != nil {
		klog.ErrorS(err, "failed to parse request", "body", string(body))
		return nil, commonerrors.NewBadRequest(err.Error())
	}

	secret, err := generateSecret(req)
	if err != nil {
		klog.ErrorS(err, "failed to generate secret")
		return nil, err
	}

	if secret, err = h.clientSet.CoreV1().Secrets(common.PrimusSafeNamespace).Create(
		c.Request.Context(), secret, metav1.CreateOptions{}); err != nil {
		klog.ErrorS(err, "failed to create secret")
		return nil, err
	}
	klog.Infof("created secret %s", secret.Name)
	if err = h.updateWorkspaceSecret(c.Request.Context(), secret); err != nil {
		return nil, err
	}
	return &types.CreateSecretResponse{
		SecretId: secret.Name,
	}, nil
}

// listSecret: implements the secret listing logic.
// Parses query parameters, builds label selectors, retrieves secrets from the cluster,
// applies authorization filtering, sorts them, and converts to response format.
func (h *Handler) listSecret(c *gin.Context) (interface{}, error) {
	requestUser, err := h.getAndSetUsername(c)
	if err != nil {
		return nil, err
	}

	query, err := parseListSecretQuery(c)
	if err != nil {
		klog.ErrorS(err, "failed to parse query")
		return nil, err
	}
	labelSelector := buildSecretLabelSelector(query)
	secretList := &corev1.SecretList{}
	if err = h.List(c.Request.Context(),
		secretList, &client.ListOptions{LabelSelector: labelSelector, Namespace: common.PrimusSafeNamespace}); err != nil {
		return nil, err
	}
	result := &types.ListSecretResponse{}
	roles := h.auth.GetRoles(c.Request.Context(), requestUser)
	for _, item := range secretList.Items {
		if err = h.auth.Authorize(authority.Input{
			Context:      c.Request.Context(),
			Resource:     &item,
			ResourceKind: authority.SecretResourceKind,
			Verb:         v1.ListVerb,
			User:         requestUser,
			Roles:        roles,
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

// getSecret: implements the logic for retrieving a single secret's information.
// Authorizes the request and retrieves the secret by name from the cluster.
func (h *Handler) getSecret(c *gin.Context) (interface{}, error) {
	requestUser, err := h.getAndSetUsername(c)
	if err != nil {
		return nil, err
	}
	if err = h.auth.Authorize(authority.Input{
		Context:      c.Request.Context(),
		ResourceKind: authority.SecretResourceKind,
		Verb:         v1.GetVerb,
		User:         requestUser,
	}); err != nil {
		return nil, err
	}
	secret, err := h.getAdminSecret(c.Request.Context(), c.GetString(common.Name))
	if err != nil {
		return nil, err
	}
	return cvtToSecretResponseItem(secret), nil
}

// patchSecret: implements partial update logic for a secret.
// Parses the patch request, applies specified changes, updates the secret in the cluster,
// and synchronizes changes with associated cluster and workspace resources.
func (h *Handler) patchSecret(c *gin.Context) (interface{}, error) {
	req := &types.PatchSecretRequest{}
	body, err := apiutils.ParseRequestBody(c.Request, req)
	if err != nil {
		klog.ErrorS(err, "failed to parse request", "body", string(body))
		return nil, commonerrors.NewBadRequest(err.Error())
	}

	requestUser, err := h.getAndSetUsername(c)
	if err != nil {
		return nil, err
	}
	if err = h.auth.Authorize(authority.Input{
		Context:      c.Request.Context(),
		ResourceKind: authority.SecretResourceKind,
		Verb:         v1.UpdateVerb,
		User:         requestUser,
	}); err != nil {
		return nil, err
	}

	secret, err := h.getAdminSecret(c.Request.Context(), c.GetString(common.Name))
	if err != nil {
		return nil, err
	}
	if err = updateSecret(secret, req); err != nil {
		return nil, err
	}
	err = h.Update(c.Request.Context(), secret)
	if err != nil {
		return nil, err
	}
	// Update the resources associated with the secret simultaneously
	if err = h.updateClusterSecret(c.Request.Context(), secret); err != nil {
		return nil, err
	}
	if err = h.updateWorkspaceSecret(c.Request.Context(), secret); err != nil {
		return nil, err
	}
	return nil, nil
}

// updateSecret: applies updates to a secret based on the patch request.
func updateSecret(secret *corev1.Secret, req *types.PatchSecretRequest) error {
	if req.Params != nil {
		reqType := v1.SecretType(v1.GetSecretType(secret))
		if err := buildSecretData(reqType, *req.Params, secret); err != nil {
			return err
		}
	}
	if req.BindAllWorkspaces != nil {
		if *req.BindAllWorkspaces {
			v1.SetLabel(secret, v1.SecretAllWorkspaceLabel, v1.TrueStr)
		} else {
			v1.RemoveLabel(secret, v1.SecretAllWorkspaceLabel)
		}
	}
	return nil
}

// updateClusterSecret: updates cluster resources that reference the specified secret.
// If any cluster references the secret and the resource version has changed,
// it updates the cluster's reference to point to the new secret version.
func (h *Handler) updateClusterSecret(ctx context.Context, secret *corev1.Secret) error {
	clusterList := &v1.ClusterList{}
	if err := h.List(ctx, clusterList, &client.ListOptions{}); err != nil {
		return err
	}
	for _, cluster := range clusterList.Items {
		imageSecret := cluster.Spec.ControlPlane.ImageSecret
		if imageSecret == nil || imageSecret.Name != secret.Name {
			continue
		}
		if imageSecret.ResourceVersion != secret.ResourceVersion {
			cluster.Spec.ControlPlane.ImageSecret = commonutils.GenObjectReference(secret.TypeMeta, secret.ObjectMeta)
			if err := h.Update(ctx, &cluster); err != nil {
				return err
			}
		}
		break
	}
	return nil
}

// updateWorkspaceSecret: updates workspace resources to synchronize secret references.
// Ensures all workspaces that should reference the secret have up-to-date references,
// including handling the bind-all-workspaces flag.
func (h *Handler) updateWorkspaceSecret(ctx context.Context, inputSecret *corev1.Secret) error {
	isApplyAllWorkspace := v1.IsSecretBindAllWorkspaces(inputSecret)
	secretReference := commonutils.GenObjectReference(inputSecret.TypeMeta, inputSecret.ObjectMeta)

	if err := backoff.ConflictRetry(func() error {
		workspaceList := &v1.WorkspaceList{}
		if err := h.List(ctx, workspaceList, &client.ListOptions{}); err != nil {
			return err
		}
		for _, workspace := range workspaceList.Items {
			isChanged := false
			isExist := false
			for i, currentSecret := range workspace.Spec.ImageSecrets {
				if currentSecret.Name == secretReference.Name {
					isExist = true
					if currentSecret.ResourceVersion != secretReference.ResourceVersion {
						workspace.Spec.ImageSecrets[i] = secretReference
						isChanged = true
					}
					break
				}
			}
			if !isExist && isApplyAllWorkspace {
				workspace.Spec.ImageSecrets = append(workspace.Spec.ImageSecrets, secretReference)
				isChanged = true
			}
			if isChanged {
				if err := h.Update(ctx, &workspace); err != nil {
					return err
				}
			}
		}
		return nil
	}, types.MaxRetry, time.Millisecond*100); err != nil {
		klog.ErrorS(err, "failed to update workspace secret", "secret", inputSecret.Name)
		return err
	}
	return nil
}

// deleteSecret: implements secret deletion logic.
// Removes the secret from the Kubernetes cluster and cleans up references
// in associated clusters and workspaces.
func (h *Handler) deleteSecret(c *gin.Context) (interface{}, error) {
	name := c.GetString(common.Name)
	if name == "" {
		return nil, commonerrors.NewBadRequest("the secretId is empty")
	}
	secret, err := h.getAdminSecret(c.Request.Context(), name)
	if err != nil {
		return nil, err
	}
	if err = h.auth.Authorize(authority.Input{
		Context:      c.Request.Context(),
		Resource:     secret,
		ResourceKind: authority.SecretResourceKind,
		Verb:         v1.DeleteVerb,
		UserId:       c.GetString(common.UserId),
	}); err != nil {
		return nil, err
	}
	if err = h.deleteClusterSecret(c.Request.Context(), name); err != nil {
		return nil, err
	}
	if err = h.deleteWorkspaceSecret(c.Request.Context(), name); err != nil {
		return nil, err
	}
	if err = h.clientSet.CoreV1().Secrets(common.PrimusSafeNamespace).Delete(
		c.Request.Context(), name, metav1.DeleteOptions{}); err != nil {
		return nil, err
	}
	klog.Infof("delete secret %s", name)

	return nil, nil
}

// deleteClusterSecret: removes secret references from cluster resources.
// Clears image secret references in clusters that reference the deleted secret.
func (h *Handler) deleteClusterSecret(ctx context.Context, secretId string) error {
	clusterList := &v1.ClusterList{}
	if err := h.List(ctx, clusterList, &client.ListOptions{}); err != nil {
		return err
	}
	for _, cluster := range clusterList.Items {
		imageSecret := cluster.Spec.ControlPlane.ImageSecret
		if imageSecret == nil || imageSecret.Name != secretId {
			continue
		}
		cluster.Spec.ControlPlane.ImageSecret = nil
		if err := h.Update(ctx, &cluster); err != nil {
			return err
		}
		break
	}
	return nil
}

// deleteWorkspaceSecret: removes secret references from workspace resources.
// Cleans up image secret references in workspaces that reference the deleted secret.
func (h *Handler) deleteWorkspaceSecret(ctx context.Context, secretId string) error {
	if err := backoff.ConflictRetry(func() error {
		workspaceList := &v1.WorkspaceList{}
		if err := h.List(ctx, workspaceList, &client.ListOptions{}); err != nil {
			return err
		}
		for _, workspace := range workspaceList.Items {
			newSecrets := make([]*corev1.ObjectReference, 0, len(workspace.Spec.ImageSecrets))
			for i, currentSecret := range workspace.Spec.ImageSecrets {
				if currentSecret.Name == secretId {
					continue
				}
				newSecrets = append(newSecrets, workspace.Spec.ImageSecrets[i])
			}
			if len(newSecrets) != len(workspace.Spec.ImageSecrets) {
				workspace.Spec.ImageSecrets = newSecrets
				if err := h.Update(ctx, &workspace); err != nil {
					return err
				}
			}
		}
		return nil
	}, types.MaxRetry, time.Millisecond*100); err != nil {
		klog.ErrorS(err, "failed to update workspace secret", "secret", secretId)
		return err
	}
	return nil
}

// getAdminSecret: retrieves a secret resource by name from the Kubernetes cluster.
// Returns the secret object or an error if retrieval fails.
func (h *Handler) getAdminSecret(ctx context.Context, name string) (*corev1.Secret, error) {
	secret, err := h.clientSet.CoreV1().Secrets(common.PrimusSafeNamespace).Get(
		ctx, name, metav1.GetOptions{})
	if err != nil {
		klog.ErrorS(err, "failed to get secret")
	}
	return secret, err
}

// generateSecret: creates a new secret object based on the creation request.
// Validates the request parameters and populates the secret metadata and data.
func generateSecret(req *types.CreateSecretRequest) (*corev1.Secret, error) {
	if req.Name == "" {
		return nil, commonerrors.NewBadRequest("the name is empty")
	}
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      req.Name,
			Namespace: common.PrimusSafeNamespace,
			Labels: map[string]string{
				v1.SecretTypeLabel: string(req.Type),
			},
		},
	}
	if req.BindAllWorkspaces {
		v1.SetLabel(secret, v1.SecretAllWorkspaceLabel, v1.TrueStr)
	}
	if err := buildSecretData(req.Type, req.Params, secret); err != nil {
		return nil, commonerrors.NewBadRequest(err.Error())
	}
	if req.Name != "" {
		v1.SetLabel(secret, v1.DisplayNameLabel, req.Name)
	}
	return secret, nil
}

// buildSecretData: constructs the secret data based on the secret type and parameters.
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
			auth := stringutil.Base64Encode(fmt.Sprintf("%s:%s",
				params[types.UserNameParam], params[types.PasswordParam]))
			dockerConf.Auths[params[types.ServerParam]] = types.DockerConfigItem{
				UserName: params[types.UserNameParam],
				Password: stringutil.Base64Decode(params[types.PasswordParam]),
				Auth:     auth,
			}
		}
		data[types.DockerConfigJson] = jsonutils.MarshalSilently(dockerConf)
	case v1.SecretSSH:
		if len(allParams) == 0 {
			return fmt.Errorf("the input params is empty")
		}
		params := allParams[0]
		if !existKey(params, types.UserNameParam) {
			return fmt.Errorf("the %s is empty", types.UserNameParam)
		}
		secretType = corev1.SecretTypeOpaque
		data[string(types.UserNameParam)] = []byte(params[types.UserNameParam])
		if val, _ := params[types.PasswordParam]; val != "" {
			data[string(types.PasswordParam)] = []byte(params[types.PasswordParam])
		} else if existKey(params, types.PublicKeyParam) && existKey(params, types.PrivateKeyParam) {
			data[types.SSHAuthKey] = []byte(stringutil.Base64Decode(params[types.PrivateKeyParam]))
			data[types.SSHAuthPubKey] = []byte(stringutil.Base64Decode(params[types.PublicKeyParam]))
		} else {
			return fmt.Errorf("the password or keypair is empty")
		}
	default:
		return fmt.Errorf("the secret type %s is not supported", reqType)
	}
	secret.Data = data
	secret.Type = secretType
	return nil
}

// existKey: checks if a key exists in the parameters map and has a non-empty value.
func existKey(params map[types.SecretParam]string, key types.SecretParam) bool {
	val, _ := params[key]
	return val != ""
}

// parseListSecretQuery: parses and validates the query parameters for listing secrets.
func parseListSecretQuery(c *gin.Context) (*types.ListSecretRequest, error) {
	query := &types.ListSecretRequest{}
	if err := c.ShouldBindWith(&query, binding.Query); err != nil {
		return nil, commonerrors.NewBadRequest("invalid query: " + err.Error())
	}
	return query, nil
}

// buildSecretLabelSelector: constructs a label selector based on query parameters.
// Used to filter secrets by type criteria.
func buildSecretLabelSelector(query *types.ListSecretRequest) labels.Selector {
	var req1 *labels.Requirement
	var labelSelector = labels.NewSelector()
	if query.Type != "" {
		typeList := strings.Split(query.Type, ",")
		req1, _ = labels.NewRequirement(v1.SecretTypeLabel, selection.In, typeList)
		labelSelector = labelSelector.Add(*req1)
	}
	return labelSelector
}

// cvtToSecretResponseItem: converts a secret object to a response item format.
// Maps the secret data to the appropriate response structure with proper value handling.
func cvtToSecretResponseItem(secret *corev1.Secret) types.SecretResponseItem {
	result := types.SecretResponseItem{
		SecretId:          secret.Name,
		SecretName:        v1.GetDisplayName(secret),
		Type:              v1.GetSecretType(secret),
		CreationTime:      timeutil.FormatRFC3339(secret.CreationTimestamp.Time),
		BindAllWorkspaces: v1.IsSecretBindAllWorkspaces(secret),
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
		params[types.PrivateKeyParam] = stringutil.Base64Encode(string(secret.Data[types.SSHAuthKey]))
		params[types.PublicKeyParam] = stringutil.Base64Encode(string(secret.Data[types.SSHAuthPubKey]))
		result.Params = append(result.Params, params)
	}
	return result
}
