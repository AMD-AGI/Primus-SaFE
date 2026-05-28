/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resources

import (
	"context"
	"fmt"
	"strings"

	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/resources/view"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
	commonworkload "github.com/AMD-AIG-AIMA/SAFE/common/pkg/workload"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/stringutil"
)

const (
	GithubPAT               = "GITHUB_PAT"
	GitHubAuthTypePAT       = "pat"
	GitHubAuthTypeApp       = "github_app"
	GitHubToken             = "github_token"
	GitHubAppId             = "github_app_id"
	GitHubAppInstallationId = "github_app_installation_id"
	GitHubAppPrivateKey     = "github_app_private_key"
)

// createCICDSecret creates a new secret for CICD scaling runner workloads.
// The secret contains ARC-compatible GitHub authentication keys.
// It returns the created secret or an error if the creation fails.
func (h *Handler) createCICDSecret(ctx context.Context,
	workload *v1.Workload, requestUser *v1.User, auth *view.GitHubAuthRequest) (*corev1.Secret, error) {
	if err := validateCICDGitHubAuth(auth); err != nil {
		return nil, err
	}
	name := commonutils.GenerateName(v1.GetDisplayName(workload))
	createSecretReq := &view.CreateSecretRequest{
		Name:         name,
		WorkspaceIds: []string{workload.Spec.Workspace},
		Type:         v1.SecretGeneral,
		Owner:        workload.Name,
		Params: []map[view.SecretParam]string{
			buildCICDSecretParams(auth),
		},
		Labels: map[string]string{
			"secret.usage": "cicd",
		},
	}
	secret, err := h.createSecretImpl(ctx, createSecretReq, requestUser)
	if err != nil {
		klog.ErrorS(err, "failed to create secret", "name", createSecretReq.Name)
		return nil, err
	}
	return secret, nil
}

// updateCICDSecret updates the CICD secret by creating a new secret and deleting the old one.
// This replaces the existing GitHub auth secret with a new one for CICD scaling runner workloads.
func (h *Handler) updateCICDSecret(ctx context.Context,
	workload *v1.Workload, requestUser *v1.User, auth *view.GitHubAuthRequest) error {
	if err := validateCICDGitHubAuth(auth); err != nil {
		return err
	}
	oldSecretId := v1.GetGithubSecretId(workload)
	if oldSecretId != "" {
		oldSecret, err := h.getAdminSecret(ctx, oldSecretId)
		if err != nil {
			if apierrors.IsNotFound(err) {
				oldSecretId = ""
			} else {
				return fmt.Errorf("failed to get existing CICD GitHub secret %q: %w", oldSecretId, err)
			}
		} else if cicdSecretDataMatchesAuth(oldSecret, auth) {
			return nil
		}
	}

	newSecret, err := h.createCICDSecret(ctx, workload, requestUser, auth)
	if err != nil {
		return err
	}
	if oldSecretId != "" {
		if err = h.deleteSecretImpl(ctx, oldSecretId, requestUser); err != nil {
			h.deleteSecretImpl(ctx, newSecret.Name, requestUser)
			return err
		}
	}

	v1.SetAnnotation(workload, v1.GithubSecretIdAnnotation, newSecret.Name)
	return nil
}

// cleanupCICDSecrets deletes secrets created for CICD scaling runner set workloads.
// This is called when workload creation fails to ensure orphaned secrets are cleaned up.
func (h *Handler) cleanupCICDSecrets(ctx context.Context, workload *v1.Workload) {
	if !commonworkload.IsCICDScalingRunnerSet(workload) {
		return
	}
	if err := h.clientSet.CoreV1().Secrets(common.PrimusSafeNamespace).Delete(
		ctx, v1.GetDisplayName(workload), metav1.DeleteOptions{}); err != nil {
		if !apierrors.IsNotFound(err) {
			klog.ErrorS(err, "failed to delete secret", "name", v1.GetDisplayName(workload))
		}
	}
	klog.Infof("cleaned up CICD secret %s after workload %s creation failure", v1.GetDisplayName(workload), workload.Name)
}

// generateCICDScaleRunnerSet configures a workload for CICD scaling runner set.
// It validates CICD settings and creates a GitHub auth secret.
func (h *Handler) generateCICDScaleRunnerSet(ctx context.Context, workload *v1.Workload,
	requestUser *v1.User, auth *view.GitHubAuthRequest) error {
	if !commonconfig.IsCICDEnable() {
		return commonerrors.NewNotImplemented("the CICD is not enabled")
	}
	auth = normalizeCICDGitHubAuth(auth, workload.Spec.Env)
	if err := validateCICDGitHubAuth(auth); err != nil {
		return err
	}
	secret, err := h.createCICDSecret(ctx, workload, requestUser, auth)
	if err != nil {
		return err
	}
	delete(workload.Spec.Env, GithubPAT)
	v1.SetAnnotation(workload, v1.GithubSecretIdAnnotation, secret.Name)
	return nil
}

func normalizeCICDGitHubAuth(auth *view.GitHubAuthRequest, env map[string]string) *view.GitHubAuthRequest {
	if auth != nil {
		return auth
	}
	if env == nil {
		return nil
	}
	if token := strings.TrimSpace(env[GithubPAT]); token != "" {
		return &view.GitHubAuthRequest{
			Type:  GitHubAuthTypePAT,
			Token: token,
		}
	}
	return nil
}

func validateCICDGitHubAuth(auth *view.GitHubAuthRequest) error {
	if auth == nil {
		return commonerrors.NewBadRequest("the github authentication is empty")
	}
	switch strings.TrimSpace(auth.Type) {
	case GitHubAuthTypeApp:
		if strings.TrimSpace(auth.AppId) == "" ||
			strings.TrimSpace(auth.InstallationId) == "" ||
			strings.TrimSpace(auth.PrivateKey) == "" {
			return commonerrors.NewBadRequest("github app authentication requires appId, installationId, and privateKey")
		}
	case GitHubAuthTypePAT:
		if strings.TrimSpace(auth.Token) == "" {
			return commonerrors.NewBadRequest("the github pat(token) is empty")
		}
	default:
		return commonerrors.NewBadRequest("unsupported github authentication type")
	}
	return nil
}

func buildCICDSecretParams(auth *view.GitHubAuthRequest) map[view.SecretParam]string {
	switch strings.TrimSpace(auth.Type) {
	case GitHubAuthTypeApp:
		return map[view.SecretParam]string{
			GitHubAppId:             stringutil.Base64Encode(strings.TrimSpace(auth.AppId)),
			GitHubAppInstallationId: stringutil.Base64Encode(strings.TrimSpace(auth.InstallationId)),
			GitHubAppPrivateKey:     stringutil.Base64Encode(strings.TrimSpace(auth.PrivateKey)),
		}
	default:
		return map[view.SecretParam]string{
			GitHubToken: stringutil.Base64Encode(strings.TrimSpace(auth.Token)),
		}
	}
}

// cicdSecretDataMatchesAuth is an idempotency check that avoids rotating
// credentials when the submitted values already match the existing ARC secret.
func cicdSecretDataMatchesAuth(secret *corev1.Secret, auth *view.GitHubAuthRequest) bool {
	switch strings.TrimSpace(auth.Type) {
	case GitHubAuthTypeApp:
		return string(secret.Data[GitHubAppId]) == strings.TrimSpace(auth.AppId) &&
			string(secret.Data[GitHubAppInstallationId]) == strings.TrimSpace(auth.InstallationId) &&
			string(secret.Data[GitHubAppPrivateKey]) == strings.TrimSpace(auth.PrivateKey)
	default:
		return string(secret.Data[GitHubToken]) == strings.TrimSpace(auth.Token)
	}
}
