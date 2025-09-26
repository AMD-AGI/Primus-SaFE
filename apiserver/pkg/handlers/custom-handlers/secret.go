/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
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
		secretList, &client.ListOptions{LabelSelector: labelSelector}); err != nil {
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
	err = h.clientSet.CoreV1().Secrets(common.PrimusSafeNamespace).Delete(
		c.Request.Context(), name, metav1.DeleteOptions{})
	if err != nil {
		return nil, err
	}
	klog.Infof("delete secret %s", name)
	return nil, nil
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
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: common.PrimusSafeNamespace,
			Labels: map[string]string{
				v1.SecretTypeLabel: string(req.Type),
				v1.UserIdLabel:     c.GetString(common.UserId),
			},
		},
	}
	if err := buildSecretData(req, secret); err != nil {
		return nil, err
	}
	if req.Name != "" {
		secret.Labels[v1.DisplayNameLabel] = req.Name
	}
	return secret, nil
}

func buildSecretData(req *types.CreateSecretRequest, secret *corev1.Secret) error {
	name := ""
	var secretType corev1.SecretType
	params := make(map[string][]byte)

	switch req.Type {
	case types.SecretImage:
		keys := []types.SecretParam{types.PasswordParam, types.UserNameParam, types.ServerParam}
		for _, key := range keys {
			if !req.HasParam(key) {
				return fmt.Errorf("the %s is empty", key)
			}
		}
		name = req.Name
		secretType = corev1.SecretTypeDockerConfigJson
		dockerConf := types.DockerConfig{
			Auth: map[string]types.DockerConfigItem{
				req.Params[types.ServerParam]: {
					UserName: req.Params[types.UserNameParam],
					Password: stringutil.Base64Decode(req.Params[types.PasswordParam]),
				},
			},
		}
		params[types.DockerConfigJson] = jsonutils.MarshalSilently(dockerConf)
	case types.SecretSSH:
		if !req.HasParam(types.UserNameParam) {
			return fmt.Errorf("the %s is empty", types.UserNameParam)
		}
		if req.Name == "" {
			req.Name = "ssh-" + req.Params[types.UserNameParam]
		}
		name = commonutils.GenerateName(req.Name)
		secretType = corev1.SecretTypeOpaque
		params[string(types.UserNameParam)] = []byte(req.Params[types.UserNameParam])
		if req.HasParam(types.PasswordParam) {
			params[string(types.PasswordParam)] = []byte(req.Params[types.PasswordParam])
		} else if req.HasParam(types.PublicKeyParam) && req.HasParam(types.PrivateKeyParam) {
			params[types.SSHAuthKey] = []byte(stringutil.Base64Decode(req.Params[types.PrivateKeyParam]))
			params[types.SSHAuthPubKey] = []byte(stringutil.Base64Decode(req.Params[types.PublicKeyParam]))
		} else {
			return fmt.Errorf("the password or keypair is empty")
		}
	default:
		return fmt.Errorf("the secret type %s is not supported", req.Type)
	}
	secret.Data = params
	secret.Name = name
	secret.Type = secretType
	return nil
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
		types := strings.Split(query.Type, ",")
		req1, _ = labels.NewRequirement(v1.SecretTypeLabel, selection.In, types)
		labelSelector = labelSelector.Add(*req1)
	}
	return labelSelector
}

func cvtToSecretResponseItem(secret *corev1.Secret) types.SecretResponseItem {
	result := types.SecretResponseItem{
		SecretId:     secret.Name,
		SecretName:   v1.GetDisplayName(secret),
		Type:         v1.GetLabel(secret, v1.SecretTypeLabel),
		CreationTime: timeutil.FormatRFC3339(&secret.CreationTimestamp.Time),
	}
	result.Params = make(map[types.SecretParam]string)
	switch result.Type {
	case string(types.SecretImage):
		dockerConf := &types.DockerConfig{}
		if json.Unmarshal(secret.Data[types.DockerConfigJson], dockerConf) == nil {
			for k, v := range dockerConf.Auth {
				result.Params[types.ServerParam] = k
				result.Params[types.UserNameParam] = v.UserName
				result.Params[types.PasswordParam] = stringutil.Base64Encode(v.Password)
				break
			}
		}
	case string(types.SecretSSH):
		result.Params[types.UserNameParam] = string(secret.Data[string(types.UserNameParam)])
		result.Params[types.PrivateKeyParam] = stringutil.Base64Encode(string(secret.Data[types.SSHAuthKey]))
		result.Params[types.PublicKeyParam] = stringutil.Base64Encode(string(secret.Data[types.SSHAuthPubKey]))
	}
	return result
}
