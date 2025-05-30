/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package custom_handlers

import (
	"context"
	"fmt"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/custom-handlers/types"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
	jsonutils "github.com/AMD-AIG-AIMA/SAFE/utils/pkg/json"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/stringutil"
)

func (h *Handler) CreateSecret(c *gin.Context) {
	handle(c, h.createSecret)
}

func (h *Handler) ListSecret(c *gin.Context) {
	handle(c, h.listSecret)
}

func (h *Handler) DeleteSecret(c *gin.Context) {
	handle(c, h.deleteSecret)
}

func (h *Handler) createSecret(c *gin.Context) (interface{}, error) {
	req := &types.CreateSecretRequest{}
	body, err := getBodyFromRequest(c.Request, req)
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
	return &types.CreateSecretResponse{
		SecretId: secret.Name,
	}, nil
}

func (h *Handler) listSecret(c *gin.Context) (interface{}, error) {
	query, err := parseListSecretQuery(c)
	if err != nil {
		klog.ErrorS(err, "failed to parse query")
		return nil, err
	}
	labelSelector := buildSecretLabelSelector(query)
	secretList := &corev1.SecretList{}
	if err = h.List(c.Request.Context(), secretList,
		&client.ListOptions{LabelSelector: labelSelector}); err != nil {
		return nil, err
	}
	result := &types.GetSecretResponse{}
	for _, item := range secretList.Items {
		result.Items = append(result.Items, types.GetSecretResponseItem{
			SecretId:   item.Name,
			SecretName: v1.GetDisplayName(&item),
			Type:       item.Labels[v1.SecretTypeLabel],
		})
	}
	result.TotalCount = len(result.Items)
	return result, nil
}

func (h *Handler) deleteSecret(c *gin.Context) (interface{}, error) {
	name := c.GetString(types.Name)
	if name == "" {
		return nil, commonerrors.NewBadRequest("the secretId is not found")
	}
	err := h.clientSet.CoreV1().Secrets(common.PrimusCryptoSecret).Delete(
		c.Request.Context(), name, metav1.DeleteOptions{})
	if err != nil {
		return nil, err
	}
	klog.Infof("delete secret: %s", name)
	return nil, nil
}

func (h *Handler) getSecret(ctx context.Context, name string) (*corev1.Secret, error) {
	secret, err := h.clientSet.CoreV1().Secrets(common.PrimusSafeNamespace).Get(
		ctx, name, metav1.GetOptions{})
	if err != nil {
		klog.ErrorS(err, "failed to get secret")
	}
	return secret, err
}

func generateSecret(req *types.CreateSecretRequest) (*corev1.Secret, error) {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: common.PrimusSafeNamespace,
			Labels: map[string]string{
				v1.SecretTypeLabel: string(req.Type),
			},
		},
	}
	if err := buildSecretData(req, secret); err != nil {
		return nil, err
	}
	if req.DisplayName != "" {
		secret.Labels[v1.DisplayNameLabel] = req.DisplayName
	}
	if req.Type == types.SecretSSH {
		if sshMd5 := buildSshSecretMd5(req); sshMd5 != "" {
			secret.Labels[v1.SecretMd5Label] = sshMd5
		}
	}
	return secret, nil
}

func buildSecretData(req *types.CreateSecretRequest, secret *corev1.Secret) error {
	name := ""
	var secretType corev1.SecretType
	data := make(map[string][]byte)

	switch req.Type {
	case types.SecretCrypto:
		if !req.HasParam(types.PasswordParam) {
			return fmt.Errorf("the %s is not found", types.PasswordParam)
		}
		name = common.PrimusCryptoSecret
		if req.DisplayName == "" {
			req.DisplayName = name
		}
		secretType = corev1.SecretTypeOpaque
		data[types.PasswordParam] = []byte(req.Params[types.PasswordParam])
	case types.SecretImage:
		params := []string{types.PasswordParam, types.UserNameParam, types.ServerParam}
		for _, p := range params {
			if !req.HasParam(p) {
				return fmt.Errorf("the %s is empty", p)
			}
		}
		name = common.PrimusImageSecret
		if req.DisplayName == "" {
			req.DisplayName = name
		}
		secretType = corev1.SecretTypeDockerConfigJson
		auth := stringutil.Base64Encode(fmt.Sprintf("%s:%s",
			req.Params[types.UserNameParam], req.Params[types.PasswordParam]))
		dockerConf := types.DockerConfig{
			Auth: map[string]types.DockerConfigItem{
				req.Params[types.ServerParam]: {
					UserName: req.Params[types.UserNameParam],
					Password: req.Params[types.PasswordParam],
					Auth:     auth,
				},
			},
		}
		data[types.DockerConfigJson] = jsonutils.MarshalSilently(dockerConf)
	case types.SecretSSH:
		if !req.HasParam(types.UserNameParam) {
			return fmt.Errorf("the %s is empty", types.UserNameParam)
		}
		if req.DisplayName == "" {
			req.DisplayName = "ssh-" + req.Params[types.UserNameParam]
		}
		name = commonutils.GenerateName(req.DisplayName)
		secretType = corev1.SecretTypeOpaque
		data[types.UserNameParam] = []byte(req.Params[types.UserNameParam])
		if req.HasParam(types.PasswordParam) {
			data[types.PasswordParam] = []byte(req.Params[types.PasswordParam])
		} else if req.HasParam(types.PublicKeyParam) && req.HasParam(types.PrivateKeyParam) {
			data[types.SSHAuthKey] = []byte(stringutil.Base64Decode(req.Params[types.PrivateKeyParam]))
			data[types.SSHAuthPubKey] = []byte(stringutil.Base64Decode(req.Params[types.PublicKeyParam]))
		} else {
			return fmt.Errorf("the password or keypair is empty")
		}
	}
	secret.Data = data
	secret.Name = name
	secret.Type = secretType
	return nil
}

func parseListSecretQuery(c *gin.Context) (*types.GetSecretRequest, error) {
	query := &types.GetSecretRequest{}
	if err := c.ShouldBindWith(&query, binding.Query); err != nil {
		return nil, commonerrors.NewBadRequest("invalid query: " + err.Error())
	}
	return query, nil
}

func buildSecretLabelSelector(query *types.GetSecretRequest) labels.Selector {
	var req1 *labels.Requirement
	var labelSelector = labels.NewSelector()
	if query.Type != "" {
		req1, _ = labels.NewRequirement(v1.SecretTypeLabel, selection.Equals, []string{query.Type})
		labelSelector = labelSelector.Add(*req1)
	}
	return labelSelector
}

func buildSshSecretMd5(req *types.CreateSecretRequest) string {
	result := ""
	if req.HasParam(types.PasswordParam) {
		result = req.Params[types.UserNameParam] + "-" + req.Params[types.PasswordParam]
	} else if req.HasParam(types.PublicKeyParam) && req.HasParam(types.PrivateKeyParam) {
		result = req.Params[types.UserNameParam] + "-" +
			req.Params[types.PrivateKeyParam] + "-" + req.Params[types.PublicKeyParam]
	} else {
		return ""
	}
	return stringutil.MD5(result)
}
