/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resources

import (
	"context"

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
	GithubPAT   = "GITHUB_PAT"
	GitHubToken = "github_token"
)

// createCICDSecret creates a new secret for CICD scaling runner workloads.
// The secret contains the GitHub token encoded in base64 format.
// It returns the created secret or an error if the creation fails.
func (h *Handler) createCICDSecret(ctx context.Context,
	workload *v1.Workload, requestUser *v1.User, token string) (*corev1.Secret, error) {
	name := commonutils.GenerateName(v1.GetDisplayName(workload))
	createSecretReq := &view.CreateSecretRequest{
		Name:         name,
		WorkspaceIds: []string{workload.Spec.Workspace},
		Type:         v1.SecretGeneral,
		Owner:        workload.Name,
		Params: []map[view.SecretParam]string{
			{
				GitHubToken: stringutil.Base64Encode(token),
			},
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
// This replaces the existing GitHub PAT secret with a new one for CICD scaling runner workloads.
func (h *Handler) updateCICDSecret(ctx context.Context,
	workload *v1.Workload, requestUser *v1.User, newToken string) error {
	// Get the old secret to compare tokens
	oldSecretId := v1.GetGithubSecretId(workload)
	if oldSecretId != "" {
		oldSecret, err := h.getAdminSecret(ctx, oldSecretId)
		if err == nil && oldSecret != nil {
			// Extract the old token from secret data
			if oldTokenBytes, ok := oldSecret.Data[GitHubToken]; ok {
				oldToken := string(oldTokenBytes)
				if oldToken == newToken {
					// Token hasn't changed, no need to update
					return nil
				}
			}
		}
	}

	newSecret, err := h.createCICDSecret(ctx, workload, requestUser, newToken)
	if err != nil {
		return err
	}
	if oldSecretId != "" {
		if err = h.deleteSecretImpl(ctx, oldSecretId, requestUser); err != nil {
			if innerErr := h.deleteSecretImpl(ctx, newSecret.Name, requestUser); innerErr != nil {
				klog.ErrorS(innerErr, "failed to delete secret", "secretId", newSecret.Name)
			}
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
// It validates CICD settings, creates a GitHub token secret, and sets the control plane IP.
func (h *Handler) generateCICDScaleRunnerSet(ctx context.Context, workload *v1.Workload, requestUser *v1.User) error {
	if !commonconfig.IsCICDEnable() {
		return commonerrors.NewNotImplemented("the CICD is not enabled")
	}
	controlPlaneIp, err := h.getAdminControlPlaneIp(ctx)
	if err != nil {
		return err
	}
	v1.SetAnnotation(workload, v1.AdminControlPlaneAnnotation, controlPlaneIp)

	val, _ := workload.Spec.Env[GithubPAT]
	if val == "" {
		return commonerrors.NewBadRequest("the github pat(token) is empty")
	}
	secret, err := h.createCICDSecret(ctx, workload, requestUser, val)
	if err != nil {
		return err
	}
	delete(workload.Spec.Env, GithubPAT)
	v1.SetAnnotation(workload, v1.GithubSecretIdAnnotation, secret.Name)
	return nil
}
