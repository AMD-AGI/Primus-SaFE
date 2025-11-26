/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package image_handlers

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"

	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"

	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/crypto"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/model"
)

func (h *ImageHandler) refreshImageImportSecrets(ctx context.Context) error {
	registries, err := h.dbClient.ListRegistryInfos(ctx, 1, -1)
	if err != nil {
		klog.ErrorS(err, "List registry info from db error")
		return err
	}
	secret, err := h.getDesiredImageImportSecret(registries)
	if err != nil {
		klog.ErrorS(err, "Get desired image pull secret error")
		return err
	}
	// create or update
	existSecret, err := h.clientSet.CoreV1().Secrets(secret.Namespace).Get(ctx, secret.Name, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			_, err = h.clientSet.CoreV1().Secrets(secret.Namespace).Create(ctx, secret, metav1.CreateOptions{})
			if err != nil {
				klog.ErrorS(err, "Create image pull secret error")
				return err
			}
			klog.Info("Create image pull secret success")
			return nil
		}
		klog.ErrorS(err, "Get image pull secret error")
		return err
	}
	existSecret.Data = secret.Data
	existSecret.StringData = secret.StringData
	existSecret.Type = secret.Type
	_, err = h.clientSet.CoreV1().Secrets(secret.Namespace).Update(ctx, existSecret, metav1.UpdateOptions{})
	if err != nil {
		klog.ErrorS(err, "Update image pull secret error")
		return err
	}
	klog.Info("Update image pull secret success")
	return nil
}

func (h *ImageHandler) getDesiredImageImportSecret(registries []*model.RegistryInfo) (*corev1.Secret, error) {
	auths := RegistryAuth{Auths: map[string]RegistryAuthItem{}}
	for _, registry := range registries {
		if registry.Username == "" {
			continue
		}
		userName := ""
		password := ""
		if registry.Username != "" {
			u, err := crypto.NewCrypto().Decrypt(registry.Username)
			if err != nil {
				klog.ErrorS(err, "Decrypt registry username Fail", registry.Name)
				continue
			}
			userName = u
		}
		if registry.Password != "" {
			p, err := crypto.NewCrypto().Decrypt(registry.Password)
			if err != nil {
				klog.ErrorS(err, "Decrypt registry password Fail", registry.Name)
				continue
			}
			password = p

		}
		auths.Auths[registry.URL] = RegistryAuthItem{Auth: generateAuthValue(userName, password)}
	}
	jsonByte, err := json.Marshal(auths)
	if err != nil {
		klog.ErrorS(err, "Generate registry auth json error")
		return nil, err
	}
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      common.ImageImportSecretName,
			Namespace: common.PrimusSafeNamespace,
		},
		StringData: map[string]string{
			"config.json": string(jsonByte),
		},
		Type: corev1.SecretTypeOpaque,
	}, nil
}

func (h *ImageHandler) refreshImagePullSecrets(ctx context.Context) error {
	registries, err := h.dbClient.ListRegistryInfos(ctx, 1, -1)
	if err != nil {
		klog.ErrorS(err, "List registry info from db error")
		return err
	}
	secret, err := h.getDesiredImagePullSecret(registries)
	if err != nil {
		klog.ErrorS(err, "Get desired image pull secret error")
		return err
	}
	// create or update
	existSecret, err := h.clientSet.CoreV1().Secrets(secret.Namespace).Get(ctx, secret.Name, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			_, err = h.clientSet.CoreV1().Secrets(secret.Namespace).Create(ctx, secret, metav1.CreateOptions{})
			if err != nil {
				klog.ErrorS(err, "Create image pull secret error")
				return err
			}
			klog.Info("Create image pull secret success")
			return nil
		}
		klog.ErrorS(err, "Get image pull secret error")
		return err
	}
	existSecret.Data = secret.Data
	existSecret.StringData = secret.StringData
	existSecret.Type = secret.Type
	_, err = h.clientSet.CoreV1().Secrets(secret.Namespace).Update(ctx, existSecret, metav1.UpdateOptions{})
	if err != nil {
		klog.ErrorS(err, "Update image pull secret error")
		return err
	}
	klog.Info("Update image pull secret success")
	return nil
}

func (h *ImageHandler) getDesiredImagePullSecret(registries []*model.RegistryInfo) (*corev1.Secret, error) {
	auths := RegistryAuth{Auths: map[string]RegistryAuthItem{}}
	for _, registry := range registries {
		if registry.Username == "" {
			continue
		}
		userName := ""
		password := ""
		if registry.Username != "" {
			u, err := crypto.NewCrypto().Decrypt(registry.Username)
			if err != nil {
				klog.ErrorS(err, "Decrypt registry username Fail", registry.Name)
				continue
			}
			userName = u
		}
		if registry.Password != "" {
			p, err := crypto.NewCrypto().Decrypt(registry.Password)
			if err != nil {
				klog.ErrorS(err, "Decrypt registry password Fail", registry.Name)
				continue
			}
			password = p

		}
		auths.Auths[registry.URL] = RegistryAuthItem{Auth: generateAuthValue(userName, password)}
	}
	jsonByte, err := json.Marshal(auths)
	if err != nil {
		klog.ErrorS(err, "Generate registry auth json error")
		return nil, err
	}
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ImagePullSecretName,
			Namespace: common.PrimusSafeNamespace,
		},
		StringData: map[string]string{
			".dockerconfigjson": string(jsonByte),
		},
		Type: corev1.SecretTypeDockerConfigJson,
	}, nil
}

func generateAuthValue(username, password string) string {
	return base64.URLEncoding.EncodeToString([]byte(fmt.Sprintf("%s:%s", username, password)))
}
