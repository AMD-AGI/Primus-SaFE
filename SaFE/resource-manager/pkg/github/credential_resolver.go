/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package github

import (
	"context"
	"fmt"
	"strings"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	gitHubAuthTypePAT = "pat"
	gitHubAuthTypeApp = "github_app"

	gitHubTokenKey             = "github_token"
	gitHubAppIDKey             = "github_app_id"
	gitHubAppInstallationIDKey = "github_app_installation_id"
	gitHubAppPrivateKeyKey     = "github_app_private_key"
)

var gitHubPATSecretKeys = []string{gitHubTokenKey, "token", "GITHUB_TOKEN"}

type GitHubCredential struct {
	Type           string
	Token          string
	AppID          string
	InstallationID string
	PrivateKey     string
}

type GitHubCredentialResolver struct {
	client client.Client
}

func NewGitHubCredentialResolver(client client.Client) *GitHubCredentialResolver {
	return &GitHubCredentialResolver{client: client}
}

func (r *GitHubCredentialResolver) Resolve(ctx context.Context, run *WorkflowRunRecord) (*GitHubCredential, error) {
	if r == nil || r.client == nil {
		return nil, fmt.Errorf("github credential resolver is not configured")
	}
	if run == nil || strings.TrimSpace(run.WorkloadID) == "" {
		return nil, fmt.Errorf("workflow run has no workload id")
	}

	workload := &v1.Workload{}
	if err := r.client.Get(ctx, client.ObjectKey{Name: run.WorkloadID}, workload); err != nil {
		return nil, fmt.Errorf("get workload %q: %w", run.WorkloadID, err)
	}

	secretName := strings.TrimSpace(v1.GetGithubSecretId(workload))
	if secretName == "" {
		return nil, fmt.Errorf("workload %q has no github secret annotation", run.WorkloadID)
	}

	secret := &corev1.Secret{}
	if err := r.client.Get(ctx, client.ObjectKey{Namespace: common.PrimusSafeNamespace, Name: secretName}, secret); err != nil {
		return nil, fmt.Errorf("get github secret %q: %w", secretName, err)
	}

	return credentialFromSecret(secret)
}

func credentialFromSecret(secret *corev1.Secret) (*GitHubCredential, error) {
	for _, key := range gitHubPATSecretKeys {
		if token := strings.TrimSpace(string(secret.Data[key])); token != "" {
			return &GitHubCredential{
				Type:  gitHubAuthTypePAT,
				Token: token,
			}, nil
		}
	}

	appID := strings.TrimSpace(string(secret.Data[gitHubAppIDKey]))
	installationID := strings.TrimSpace(string(secret.Data[gitHubAppInstallationIDKey]))
	privateKey := strings.TrimSpace(string(secret.Data[gitHubAppPrivateKeyKey]))
	if appID != "" && installationID != "" && privateKey != "" {
		return &GitHubCredential{
			Type:           gitHubAuthTypeApp,
			AppID:          appID,
			InstallationID: installationID,
			PrivateKey:     privateKey,
		}, nil
	}

	return nil, fmt.Errorf("github secret %q does not contain a supported credential", secret.Name)
}
