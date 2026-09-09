/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resources

import (
	"context"
	"fmt"
	"strings"
	"unicode"

	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
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
func (h *Handler) createCICDSecret(ctx context.Context, workload *v1.Workload,
	requestUser *v1.User, auth *view.GitHubAuthRequest, proxyAuth *view.ProxyAuthRequest) (*corev1.Secret, error) {
	if err := validateCICDGitHubAuth(auth); err != nil {
		return nil, err
	}
	if err := validateCICDProxyAuth(proxyAuth); err != nil {
		return nil, err
	}
	name := commonutils.GenerateName(v1.GetDisplayName(workload))
	createSecretReq := &view.CreateSecretRequest{
		Name:         name,
		WorkspaceIds: []string{workload.Spec.Workspace},
		Type:         v1.SecretGeneral,
		Owner:        workload.Name,
		Params: []map[view.SecretParam]string{
			buildCICDSecretParams(auth, proxyAuth),
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

// cicdSecretRotation describes a CICD GitHub auth secret that updateCICDSecret created
// but whose workload annotation the caller has not persisted yet.
//
// SupersededSecretId is empty when the workload had no usable secret before, which is
// why this is a struct and not a bare id: "no previous secret to drop" and "no rotation
// happened at all" need different handling and a single empty string cannot tell them
// apart.
type cicdSecretRotation struct {
	// NewSecretId is the replacement secret, already created and referenced by the
	// in-memory workload annotation.
	NewSecretId string
	// SupersededSecretId is the secret the replacement displaced, still present in the
	// cluster so a failed persist can fall back to it. Empty when there was none.
	SupersededSecretId string
}

// updateCICDSecret rotates the CICD GitHub auth secret: it creates the replacement
// secret and repoints the workload annotation at it, both in memory.
//
// It deliberately does NOT delete the superseded secret. The annotation is only
// persisted by the caller, after this returns; deleting the old secret here would mean
// a failed persist leaves the stored annotation pointing at a secret that no longer
// exists, which breaks the ARC githubConfigSecret with no way back. Instead the caller
// gets both ids and settles them once the annotation is durable -- see
// deleteSupersededCICDSecret and discardRolledBackCICDSecret.
//
// A nil rotation means the submitted credentials already match the current secret and
// nothing was created or changed.
func (h *Handler) updateCICDSecret(ctx context.Context, workload *v1.Workload, requestUser *v1.User,
	auth *view.GitHubAuthRequest, proxyAuth *view.ProxyAuthRequest) (*cicdSecretRotation, error) {
	if err := validateCICDProxyAuth(proxyAuth); err != nil {
		return nil, err
	}
	oldSecretId := v1.GetGithubSecretId(workload)
	if oldSecretId != "" {
		oldSecret, err := h.getAdminSecret(ctx, oldSecretId)
		if err != nil {
			if apierrors.IsNotFound(err) {
				// The annotation is stale; there is nothing to fall back to or delete.
				oldSecretId = ""
			} else {
				return nil, fmt.Errorf("failed to get existing CICD GitHub secret %q: %w", oldSecretId, err)
			}
		} else {
			// One Secret holds both credentials, so rotating either one has to write
			// the other back unchanged rather than drop it.
			auth, proxyAuth = carryForwardCICDAuth(oldSecret, auth, proxyAuth)
			if cicdSecretDataMatchesAuth(oldSecret, auth) && cicdSecretDataMatchesProxyAuth(oldSecret, proxyAuth) {
				return nil, nil
			}
		}
	}
	if err := validateCICDGitHubAuth(auth); err != nil {
		return nil, err
	}

	newSecret, err := h.createCICDSecret(ctx, workload, requestUser, auth, proxyAuth)
	if err != nil {
		return nil, err
	}

	v1.SetAnnotation(workload, v1.GithubSecretIdAnnotation, newSecret.Name)
	// Both references name the same Secret, so both have to move: the superseded
	// one is deleted once the rotation lands, and an env left pointing at it
	// sends ARC to a Secret that is no longer there.
	if proxyAuth != nil {
		if workload.Spec.Env == nil {
			workload.Spec.Env = map[string]string{}
		}
		workload.Spec.Env[common.ProxyCredentialSecret] = newSecret.Name
	}
	return &cicdSecretRotation{NewSecretId: newSecret.Name, SupersededSecretId: oldSecretId}, nil
}

// carryForwardCICDAuth fills whichever credential the request omitted from the
// Secret it is replacing, so a caller rotating one does not erase the other.
func carryForwardCICDAuth(oldSecret *corev1.Secret, auth *view.GitHubAuthRequest,
	proxyAuth *view.ProxyAuthRequest) (*view.GitHubAuthRequest, *view.ProxyAuthRequest) {
	if auth == nil && oldSecret != nil {
		if appId := string(oldSecret.Data[GitHubAppId]); appId != "" {
			auth = &view.GitHubAuthRequest{
				Type:           GitHubAuthTypeApp,
				AppId:          appId,
				InstallationId: string(oldSecret.Data[GitHubAppInstallationId]),
				PrivateKey:     string(oldSecret.Data[GitHubAppPrivateKey]),
			}
		} else if token := string(oldSecret.Data[GitHubToken]); token != "" {
			auth = &view.GitHubAuthRequest{Type: GitHubAuthTypePAT, Token: token}
		}
	}
	if proxyAuth == nil && oldSecret != nil {
		username := string(oldSecret.Data[string(view.UserNameParam)])
		pw := string(oldSecret.Data[string(view.PasswordParam)])
		if username != "" && pw != "" {
			proxyAuth = &view.ProxyAuthRequest{Username: username, Password: pw}
		}
	}
	return auth, proxyAuth
}

func cicdSecretDataMatchesProxyAuth(secret *corev1.Secret, proxyAuth *view.ProxyAuthRequest) bool {
	if proxyAuth == nil {
		return len(secret.Data[string(view.UserNameParam)]) == 0 && len(secret.Data[string(view.PasswordParam)]) == 0
	}
	return string(secret.Data[string(view.UserNameParam)]) == strings.TrimSpace(proxyAuth.Username) &&
		string(secret.Data[string(view.PasswordParam)]) == strings.TrimSpace(proxyAuth.Password)
}

// deleteSupersededCICDSecret drops the secret a rotation replaced, once the new
// annotation has been persisted. A failure here leaks a secret but does not invalidate
// the rotation that already succeeded, so it is logged rather than returned: reporting
// an error would push the caller into retrying a rotation that is already live. The
// leaked secret still carries the workload's owner label, so the scheduler's
// owner-label sweep collects it when the workload is deleted.
func (h *Handler) deleteSupersededCICDSecret(ctx context.Context,
	rotation *cicdSecretRotation, requestUser *v1.User) {
	if rotation == nil || rotation.SupersededSecretId == "" {
		return
	}
	if err := h.deleteSecretImpl(ctx, rotation.SupersededSecretId, requestUser); err != nil {
		klog.ErrorS(err, "failed to delete superseded CICD GitHub secret", "secret", rotation.SupersededSecretId)
	}
}

// discardRolledBackCICDSecret undoes a rotation whose annotation the caller could not
// persist. The stored workload still refers to the superseded secret, so the
// replacement is unreachable and is deleted, and the in-memory annotation is put back
// to match what is actually stored.
func (h *Handler) discardRolledBackCICDSecret(ctx context.Context,
	workload *v1.Workload, rotation *cicdSecretRotation, requestUser *v1.User) {
	if rotation == nil {
		return
	}
	if rotation.NewSecretId != "" {
		if err := h.deleteSecretImpl(ctx, rotation.NewSecretId, requestUser); err != nil {
			klog.ErrorS(err, "failed to delete unreferenced CICD GitHub secret", "secret", rotation.NewSecretId)
		}
	}
	if rotation.SupersededSecretId != "" {
		v1.SetAnnotation(workload, v1.GithubSecretIdAnnotation, rotation.SupersededSecretId)
		return
	}
	delete(workload.Annotations, v1.GithubSecretIdAnnotation)
}

// cleanupCICDSecrets deletes secrets created for CICD scaling runner set workloads.
// This is called when workload creation fails to ensure orphaned secrets are cleaned up.
//
// Secrets are selected by owner label rather than by name: createCICDSecret names them
// with commonutils.GenerateName, which appends a random suffix, so the workload's display
// name never matches the object that was actually created. This mirrors how the
// job-manager scheduler sweeps the same secrets when a workload is deleted -- but that
// sweep only runs for a Workload that reached the API server, which is not the case for
// every path that lands here.
func (h *Handler) cleanupCICDSecrets(ctx context.Context, workload *v1.Workload) {
	if workload == nil || !commonworkload.IsCICDScalingRunnerSet(workload) {
		return
	}
	secrets, err := h.clientSet.CoreV1().Secrets(common.PrimusSafeNamespace).List(ctx, metav1.ListOptions{
		LabelSelector: labels.SelectorFromSet(labels.Set{v1.OwnerLabel: workload.Name}).String(),
	})
	if err != nil {
		klog.ErrorS(err, "failed to list CICD secrets", "workload", workload.Name)
		return
	}
	for i := range secrets.Items {
		name := secrets.Items[i].Name
		if err = h.clientSet.CoreV1().Secrets(common.PrimusSafeNamespace).Delete(
			ctx, name, metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
			klog.ErrorS(err, "failed to delete secret", "name", name)
			continue
		}
		klog.Infof("cleaned up CICD secret %s after workload %s creation failure", name, workload.Name)
	}
}

// generateCICDScaleRunnerSet configures a workload for CICD scaling runner set.
// It validates CICD settings and creates a GitHub auth secret.
func (h *Handler) generateCICDScaleRunnerSet(ctx context.Context, workload *v1.Workload,
	requestUser *v1.User, auth *view.GitHubAuthRequest, proxyAuth *view.ProxyAuthRequest) error {
	if !commonconfig.IsCICDEnable() {
		return commonerrors.NewNotImplemented("the CICD is not enabled")
	}
	auth = normalizeCICDGitHubAuth(auth, workload.Spec.Env)
	if err := validateCICDGitHubAuth(auth); err != nil {
		return err
	}
	if err := validateCICDProxyAuth(proxyAuth); err != nil {
		return err
	}
	// A submitted credential and a named Secret are alternatives, not a pair: the
	// name would be overwritten below, so a caller sending both is telling us two
	// different things about where the credential lives.
	if proxyAuth != nil && workload.Spec.Env[common.ProxyCredentialSecret] != "" {
		return commonerrors.NewBadRequest(
			"proxyAuth: cannot be combined with env.PROXY_CREDENTIAL_SECRET; send the credential or name a Secret, not both")
	}
	if err := h.validateCICDProxyReference(ctx, workload, requestUser); err != nil {
		return err
	}
	secret, err := h.createCICDSecret(ctx, workload, requestUser, auth, proxyAuth)
	if err != nil {
		return err
	}
	delete(workload.Spec.Env, GithubPAT)
	if proxyAuth != nil {
		if workload.Spec.Env == nil {
			workload.Spec.Env = map[string]string{}
		}
		workload.Spec.Env[common.ProxyCredentialSecret] = secret.Name
	}
	v1.SetAnnotation(workload, v1.GithubSecretIdAnnotation, secret.Name)
	return nil
}

func (h *Handler) validateCICDProxyReference(ctx context.Context, workload *v1.Workload, requestUser *v1.User) error {
	if !commonworkload.IsCICDProxyRoot(workload) {
		return nil
	}
	config, err := commonworkload.ParseCICDProxy(workload.Spec.Env)
	if err != nil || config == nil || config.CredentialSecret == "" {
		return err
	}
	if h.Client == nil || h.clientSet == nil || h.accessController == nil {
		return commonerrors.NewInternalError(commonworkload.CICDProxySecretUnavailable)
	}
	secret, err := h.getAndAuthorizeSecret(ctx, config.CredentialSecret, workload.Spec.Workspace, requestUser, v1.GetVerb)
	if err != nil {
		if commonerrors.IsForbidden(err) {
			return commonerrors.NewForbidden(commonworkload.CICDProxySecretForbidden)
		}
		return commonworkload.CICDProxySecretLookupError(err)
	}
	return commonworkload.ValidateCICDProxySecret(secret, workload.Spec.Workspace)
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

// cicdGitHubAuthType normalizes the submitted auth discriminator. The wire value is
// whatever the caller typed, so "GITHUB_APP" and " Pat " have to select the same branch
// as the canonical lowercase constants -- otherwise a GitHub App request is rejected as
// an unsupported type. Every switch on the auth type goes through here so validation,
// secret construction, and the idempotency check can never disagree about the branch.
func cicdGitHubAuthType(auth *view.GitHubAuthRequest) string {
	if auth == nil {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(auth.Type))
}

func validateCICDGitHubAuth(auth *view.GitHubAuthRequest) error {
	if auth == nil {
		return commonerrors.NewBadRequest("the github authentication is empty")
	}
	switch cicdGitHubAuthType(auth) {
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

func buildCICDSecretParams(auth *view.GitHubAuthRequest, proxyAuth *view.ProxyAuthRequest) map[view.SecretParam]string {
	var params map[view.SecretParam]string
	switch cicdGitHubAuthType(auth) {
	case GitHubAuthTypeApp:
		params = map[view.SecretParam]string{
			GitHubAppId:             stringutil.Base64Encode(strings.TrimSpace(auth.AppId)),
			GitHubAppInstallationId: stringutil.Base64Encode(strings.TrimSpace(auth.InstallationId)),
			GitHubAppPrivateKey:     stringutil.Base64Encode(strings.TrimSpace(auth.PrivateKey)),
		}
	default:
		params = map[view.SecretParam]string{
			GitHubToken: stringutil.Base64Encode(strings.TrimSpace(auth.Token)),
		}
	}
	// The proxy credential rides in the same Secret as the GitHub one: ARC reads
	// each through its own reference and ignores the keys it was not asked for,
	// so one Secret keeps one rotation and one lifetime for both.
	if proxyAuth != nil {
		params[view.UserNameParam] = stringutil.Base64Encode(strings.TrimSpace(proxyAuth.Username))
		params[view.PasswordParam] = stringutil.Base64Encode(strings.TrimSpace(proxyAuth.Password))
	}
	return params
}

// validateCICDProxyAuth rejects what ValidateCICDProxySecret would reject later,
// while the caller can still be told which field was wrong.
func validateCICDProxyAuth(proxyAuth *view.ProxyAuthRequest) error {
	if proxyAuth == nil {
		return nil
	}
	for field, value := range map[string]string{
		"username": proxyAuth.Username,
		"password": proxyAuth.Password,
	} {
		// Checked before trimming, because trimming would hide exactly the
		// characters ValidateCICDProxySecret later rejects in the stored value.
		if strings.IndexFunc(value, unicode.IsControl) >= 0 {
			return commonerrors.NewBadRequest(fmt.Sprintf("proxyAuth.%s: must not contain a control character", field))
		}
		if strings.TrimSpace(value) == "" {
			return commonerrors.NewBadRequest(fmt.Sprintf("proxyAuth.%s: must not be empty", field))
		}
	}
	return nil
}

// cicdSecretDataMatchesAuth is an idempotency check that avoids rotating
// credentials when the submitted values already match the existing ARC secret.
func cicdSecretDataMatchesAuth(secret *corev1.Secret, auth *view.GitHubAuthRequest) bool {
	switch cicdGitHubAuthType(auth) {
	case GitHubAuthTypeApp:
		return string(secret.Data[GitHubAppId]) == strings.TrimSpace(auth.AppId) &&
			string(secret.Data[GitHubAppInstallationId]) == strings.TrimSpace(auth.InstallationId) &&
			string(secret.Data[GitHubAppPrivateKey]) == strings.TrimSpace(auth.PrivateKey)
	default:
		return string(secret.Data[GitHubToken]) == strings.TrimSpace(auth.Token)
	}
}
