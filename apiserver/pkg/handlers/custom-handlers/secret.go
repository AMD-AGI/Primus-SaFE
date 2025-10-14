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
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/backoff"
	jsonutils "github.com/AMD-AIG-AIMA/SAFE/utils/pkg/json"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/stringutil"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/timeutil"
)

func (h *Handler) CreateSecret(c *gin.Context) {
	handle(c, h.createSecret)
}

func (h *Handler) ListSecret(c *gin.Context) {
	handle(c, h.listSecret)
}

func (h *Handler) GetSecret(c *gin.Context) {
	handle(c, h.getSecret)
}

func (h *Handler) PatchSecret(c *gin.Context) {
	handle(c, h.patchSecret)
}

func (h *Handler) DeleteSecret(c *gin.Context) {
	handle(c, h.deleteSecret)
}

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
	body, err := getBodyFromRequest(c.Request, req)
	if err != nil {
		klog.ErrorS(err, "failed to parse request", "body", string(body))
		return nil, commonerrors.NewBadRequest(err.Error())
	}

	secret, err := generateSecret(c, req)
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
	secret, err := h.getAdminSecret(c.Request.Context(), c.GetString(types.Name))
	if err != nil {
		return nil, err
	}
	return cvtToSecretResponseItem(secret), nil
}

func (h *Handler) patchSecret(c *gin.Context) (interface{}, error) {
	req := &types.PatchSecretRequest{}
	body, err := getBodyFromRequest(c.Request, req)
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

	secret, err := h.getAdminSecret(c.Request.Context(), c.GetString(types.Name))
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

func updateSecret(secret *corev1.Secret, req *types.PatchSecretRequest) error {
	if len(req.Params) > 0 {
		reqType := v1.SecretType(v1.GetSecretType(secret))
		if err := buildSecretData(reqType, req.Params, secret); err != nil {
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

func (h *Handler) updateWorkspaceSecret(ctx context.Context, inputSecret *corev1.Secret) error {
	maxRetry := 3
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
	}, maxRetry, time.Millisecond*100); err != nil {
		klog.ErrorS(err, "failed to update workspace secret", "secret", inputSecret.Name)
		return err
	}
	return nil
}

func (h *Handler) deleteSecret(c *gin.Context) (interface{}, error) {
	name := c.GetString(types.Name)
	if name == "" {
		return nil, commonerrors.NewBadRequest("the secretId is not found")
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

func (h *Handler) deleteWorkspaceSecret(ctx context.Context, secretId string) error {
	maxRetry := 3
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
	}, maxRetry, time.Millisecond*100); err != nil {
		klog.ErrorS(err, "failed to update workspace secret", "secret", secretId)
		return err
	}
	return nil
}

func (h *Handler) getAdminSecret(ctx context.Context, name string) (*corev1.Secret, error) {
	secret, err := h.clientSet.CoreV1().Secrets(common.PrimusSafeNamespace).Get(
		ctx, name, metav1.GetOptions{})
	if err != nil {
		klog.ErrorS(err, "failed to get secret")
	}
	return secret, err
}

func generateSecret(c *gin.Context, req *types.CreateSecretRequest) (*corev1.Secret, error) {
	if req.Name == "" {
		return nil, commonerrors.NewBadRequest("the secretName is empty")
	}
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      req.Name,
			Namespace: common.PrimusSafeNamespace,
			Labels: map[string]string{
				v1.SecretTypeLabel: string(req.Type),
				v1.UserIdLabel:     c.GetString(common.UserId),
			},
		},
	}
	if req.BindAllWorkspaces {
		v1.SetLabel(secret, v1.SecretAllWorkspaceLabel, v1.TrueStr)
	}
	if err := buildSecretData(req.Type, req.Params, secret); err != nil {
		return nil, err
	}
	if req.Name != "" {
		v1.SetLabel(secret, v1.DisplayNameLabel, req.Name)
	}
	return secret, nil
}

func buildSecretData(reqType v1.SecretType, reqParams map[types.SecretParam]string, secret *corev1.Secret) error {
	var secretType corev1.SecretType
	params := make(map[string][]byte)

	switch reqType {
	case v1.SecretImage:
		keys := []types.SecretParam{types.PasswordParam, types.UserNameParam, types.ServerParam}
		for _, key := range keys {
			if !existKey(reqParams, key) {
				return fmt.Errorf("the %s is empty", key)
			}
		}
		secretType = corev1.SecretTypeDockerConfigJson
		auth := stringutil.Base64Encode(fmt.Sprintf("%s:%s",
			reqParams[types.UserNameParam], reqParams[types.PasswordParam]))
		dockerConf := types.DockerConfig{
			Auths: map[string]types.DockerConfigItem{
				reqParams[types.ServerParam]: {
					UserName: reqParams[types.UserNameParam],
					Password: stringutil.Base64Decode(reqParams[types.PasswordParam]),
					Auth:     auth,
				},
			},
		}
		params[types.DockerConfigJson] = jsonutils.MarshalSilently(dockerConf)
	case v1.SecretSSH:
		if !existKey(reqParams, types.UserNameParam) {
			return fmt.Errorf("the %s is empty", types.UserNameParam)
		}
		secretType = corev1.SecretTypeOpaque
		params[string(types.UserNameParam)] = []byte(reqParams[types.UserNameParam])
		if val, _ := reqParams[types.PasswordParam]; val != "" {
			params[string(types.PasswordParam)] = []byte(reqParams[types.PasswordParam])
		} else if existKey(reqParams, types.PublicKeyParam) && existKey(reqParams, types.PrivateKeyParam) {
			params[types.SSHAuthKey] = []byte(stringutil.Base64Decode(reqParams[types.PrivateKeyParam]))
			params[types.SSHAuthPubKey] = []byte(stringutil.Base64Decode(reqParams[types.PublicKeyParam]))
		} else {
			return fmt.Errorf("the password or keypair is empty")
		}
	default:
		return fmt.Errorf("the secret type %s is not supported", reqType)
	}
	secret.Data = params
	secret.Type = secretType
	return nil
}

func existKey(params map[types.SecretParam]string, key types.SecretParam) bool {
	val, _ := params[key]
	return val != ""
}

func parseListSecretQuery(c *gin.Context) (*types.ListSecretRequest, error) {
	query := &types.ListSecretRequest{}
	if err := c.ShouldBindWith(&query, binding.Query); err != nil {
		return nil, commonerrors.NewBadRequest("invalid query: " + err.Error())
	}
	return query, nil
}

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

func cvtToSecretResponseItem(secret *corev1.Secret) types.SecretResponseItem {
	result := types.SecretResponseItem{
		SecretId:          secret.Name,
		SecretName:        v1.GetDisplayName(secret),
		Type:              v1.GetSecretType(secret),
		CreationTime:      timeutil.FormatRFC3339(&secret.CreationTimestamp.Time),
		BindAllWorkspaces: v1.IsSecretBindAllWorkspaces(secret),
	}
	result.Params = make(map[types.SecretParam]string)
	switch result.Type {
	case string(v1.SecretImage):
		dockerConf := &types.DockerConfig{}
		if json.Unmarshal(secret.Data[types.DockerConfigJson], dockerConf) == nil {
			for k, v := range dockerConf.Auths {
				result.Params[types.ServerParam] = k
				result.Params[types.UserNameParam] = v.UserName
				result.Params[types.PasswordParam] = stringutil.Base64Encode(v.Password)
				break
			}
		}
	case string(v1.SecretSSH):
		result.Params[types.UserNameParam] = string(secret.Data[string(types.UserNameParam)])
		result.Params[types.PrivateKeyParam] = stringutil.Base64Encode(string(secret.Data[types.SSHAuthKey]))
		result.Params[types.PublicKeyParam] = stringutil.Base64Encode(string(secret.Data[types.SSHAuthPubKey]))
	}
	return result
}
