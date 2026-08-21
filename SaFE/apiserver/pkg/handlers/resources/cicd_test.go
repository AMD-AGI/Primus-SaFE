/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resources

import (
	"context"
	"errors"
	"testing"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apis/pkg/client/clientset/versioned/scheme"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/resources/view"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/stringutil"
	"gotest.tools/assert"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	k8stesting "k8s.io/client-go/testing"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlruntimefake "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func patAuth(token string) *view.GitHubAuthRequest {
	return &view.GitHubAuthRequest{
		Type:  GitHubAuthTypePAT,
		Token: token,
	}
}

func githubAppAuth(appId, installationId, privateKey string) *view.GitHubAuthRequest {
	return &view.GitHubAuthRequest{
		Type:           GitHubAuthTypeApp,
		AppId:          appId,
		InstallationId: installationId,
		PrivateKey:     privateKey,
	}
}

// Test_createCICDSecret tests the createCICDSecret function with token encoding
func Test_createCICDSecret(t *testing.T) {
	tests := []struct {
		name          string
		token         string
		expectedToken string
	}{
		{
			name:          "create secret with valid token",
			token:         "test_github_token_123",
			expectedToken: "test_github_token_123",
		},
		{
			name:          "create secret with empty token",
			token:         "",
			expectedToken: "",
		},
		{
			name:          "create secret with special characters",
			token:         "ghp_1234567890!@#$%^&*()",
			expectedToken: "ghp_1234567890!@#$%^&*()",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Verify token encoding/decoding works correctly
			encodedToken := stringutil.Base64Encode(tt.token)
			decodedToken := stringutil.Base64Decode(encodedToken)
			assert.Equal(t, decodedToken, tt.expectedToken, "Token should be encoded and decoded correctly")
		})
	}
}

// Test_updateCICDSecret_TokenUnchanged tests the optimization when token hasn't changed
func Test_updateCICDSecret_TokenUnchanged(t *testing.T) {
	ctx := context.Background()
	clusterId := "test-cluster"
	workspaceId := "test-workspace"

	workload := genMockWorkload(clusterId, workspaceId)
	user := genMockUser()
	role := genMockRole()

	oldToken := "same_token_123"
	secretName := "old-secret-id"

	// Create old secret with token
	oldSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: common.PrimusSafeNamespace,
		},
		Data: map[string][]byte{
			GitHubToken: []byte(oldToken),
		},
	}

	// Set annotation with old secret ID
	v1.SetAnnotation(workload, v1.GithubSecretIdAnnotation, secretName)

	// Create fake controller-runtime client
	fakeCtrlClient := ctrlruntimefake.NewClientBuilder().
		WithObjects(workload, user, role).
		WithScheme(scheme.Scheme).
		Build()

	// Create fake kubernetes clientset with the old secret
	fakeClientSet := k8sfake.NewSimpleClientset(oldSecret)

	h := Handler{
		Client:           fakeCtrlClient,
		clientSet:        fakeClientSet,
		accessController: authority.NewAccessController(fakeCtrlClient),
	}

	// Call updateCICDSecret with same token
	rotation, err := h.updateCICDSecret(ctx, workload, user, patAuth(oldToken))

	// Should return nil without error (optimization kicks in)
	assert.NilError(t, err)
	assert.Assert(t, rotation == nil, "matching credentials should not rotate the secret")

	// Verify the annotation is still pointing to old secret (not changed)
	assert.Equal(t, v1.GetGithubSecretId(workload), secretName)

	// Verify the old secret still exists (wasn't deleted)
	_, err = fakeClientSet.CoreV1().Secrets(common.PrimusSafeNamespace).Get(ctx, secretName, metav1.GetOptions{})
	assert.NilError(t, err, "Old secret should still exist")
}

// Test_updateCICDSecret_TokenChanged tests updating secret when token has changed
func Test_updateCICDSecret_TokenChanged(t *testing.T) {
	ctx := context.Background()
	clusterId := "test-cluster"
	workspaceId := "test-workspace"

	workload := genMockWorkload(clusterId, workspaceId)
	user := genMockUser()
	role := genMockRole()

	oldToken := "old_token_123"
	newToken := "new_token_456"
	oldSecretName := "old-secret-id"

	// Create old secret with old token
	oldSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      oldSecretName,
			Namespace: common.PrimusSafeNamespace,
			Labels: map[string]string{
				v1.SecretTypeLabel: string(v1.SecretGeneral),
				v1.UserIdLabel:     user.Name,
				v1.OwnerLabel:      workload.Name,
			},
		},
		Data: map[string][]byte{
			GitHubToken: []byte(oldToken),
		},
		Type: corev1.SecretTypeOpaque,
	}

	// Set annotation with old secret ID
	v1.SetAnnotation(workload, v1.GithubSecretIdAnnotation, oldSecretName)

	// Create fake controller-runtime client
	fakeCtrlClient := ctrlruntimefake.NewClientBuilder().
		WithObjects(workload, user, role).
		WithScheme(scheme.Scheme).
		Build()

	// Create fake kubernetes clientset with the old secret
	fakeClientSet := k8sfake.NewSimpleClientset(oldSecret)

	h := Handler{
		Client:           fakeCtrlClient,
		clientSet:        fakeClientSet,
		accessController: authority.NewAccessController(fakeCtrlClient),
	}

	// Call updateCICDSecret with new token
	rotation, err := h.updateCICDSecret(ctx, workload, user, patAuth(newToken))

	// Should succeed
	assert.NilError(t, err)
	assert.Assert(t, rotation != nil, "A changed token should produce a rotation")

	// Verify annotation is updated to new secret
	newSecretId := v1.GetGithubSecretId(workload)
	assert.Assert(t, newSecretId != "", "New secret ID should be set")
	assert.Assert(t, newSecretId != oldSecretName, "New secret ID should be different from old")
	assert.Equal(t, rotation.NewSecretId, newSecretId)
	assert.Equal(t, rotation.SupersededSecretId, oldSecretName)

	// Verify new secret was created
	newSecret, err := fakeClientSet.CoreV1().Secrets(common.PrimusSafeNamespace).Get(ctx, newSecretId, metav1.GetOptions{})
	assert.NilError(t, err, "New secret should exist")
	assert.Equal(t, string(newSecret.Data[GitHubToken]), newToken, "New secret should contain new token")

	// The old secret must survive until the caller has persisted the annotation --
	// deleting it here would strand the stored workload on a missing secret if the
	// patch fails.
	_, err = fakeClientSet.CoreV1().Secrets(common.PrimusSafeNamespace).Get(ctx, oldSecretName, metav1.GetOptions{})
	assert.NilError(t, err, "Old secret should survive until the caller settles the rotation")

	// Settling the rotation is what actually drops it.
	h.deleteSupersededCICDSecret(ctx, rotation, user)
	_, err = fakeClientSet.CoreV1().Secrets(common.PrimusSafeNamespace).Get(ctx, oldSecretName, metav1.GetOptions{})
	assert.Assert(t, apierrors.IsNotFound(err), "Old secret should be deleted once the rotation is settled")
}

// Test_updateCICDSecret_UppercaseAuthType ensures the auth type discriminator is matched
// case-insensitively: the wire value is whatever the caller typed, and a "GITHUB_APP"
// request must build GitHub App keys rather than fall through to the PAT branch.
func Test_updateCICDSecret_UppercaseAuthType(t *testing.T) {
	ctx := context.Background()
	workload := genMockWorkload("test-cluster", "test-workspace")
	user := genMockUser()
	role := genMockRole()

	fakeCtrlClient := ctrlruntimefake.NewClientBuilder().
		WithObjects(workload, user, role).
		WithScheme(scheme.Scheme).
		Build()
	fakeClientSet := k8sfake.NewSimpleClientset()

	h := Handler{
		Client:           fakeCtrlClient,
		clientSet:        fakeClientSet,
		accessController: authority.NewAccessController(fakeCtrlClient),
	}

	auth := githubAppAuth("123456", "789012", "-----BEGIN RSA PRIVATE KEY-----\nkey\n-----END RSA PRIVATE KEY-----")
	auth.Type = " GITHUB_APP "

	rotation, err := h.updateCICDSecret(ctx, workload, user, auth)
	assert.NilError(t, err)
	assert.Assert(t, rotation != nil, "Uppercase github_app should be accepted and rotate")

	secret, err := fakeClientSet.CoreV1().Secrets(common.PrimusSafeNamespace).Get(
		ctx, rotation.NewSecretId, metav1.GetOptions{})
	assert.NilError(t, err)
	assert.Equal(t, string(secret.Data[GitHubAppId]), "123456")
	assert.Equal(t, string(secret.Data[GitHubAppInstallationId]), "789012")
	assert.Equal(t, string(secret.Data[GitHubAppPrivateKey]),
		"-----BEGIN RSA PRIVATE KEY-----\nkey\n-----END RSA PRIVATE KEY-----")
	_, hasToken := secret.Data[GitHubToken]
	assert.Assert(t, !hasToken, "A github app secret must not carry a PAT key")

	// The same values resubmitted with different casing must be recognised as unchanged.
	sameAuth := githubAppAuth("123456", "789012",
		"-----BEGIN RSA PRIVATE KEY-----\nkey\n-----END RSA PRIVATE KEY-----")
	sameAuth.Type = "GitHub_App"
	again, err := h.updateCICDSecret(ctx, workload, user, sameAuth)
	assert.NilError(t, err)
	assert.Assert(t, again == nil, "Unchanged github app credentials should not rotate")
}

// Test_discardRolledBackCICDSecret verifies the rollback path taken when the caller fails
// to persist the annotation: the unreachable replacement is deleted and the in-memory
// annotation is restored to whatever is actually stored.
func Test_discardRolledBackCICDSecret(t *testing.T) {
	ctx := context.Background()
	user := genMockUser()
	role := genMockRole()

	t.Run("restores the superseded secret", func(t *testing.T) {
		workload := genMockWorkload("test-cluster", "test-workspace")
		oldSecretName := "old-secret-id"
		oldSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      oldSecretName,
				Namespace: common.PrimusSafeNamespace,
				Labels: map[string]string{
					v1.SecretTypeLabel: string(v1.SecretGeneral),
					v1.UserIdLabel:     user.Name,
					v1.OwnerLabel:      workload.Name,
				},
			},
			Data: map[string][]byte{GitHubToken: []byte("old_token_123")},
			Type: corev1.SecretTypeOpaque,
		}
		v1.SetAnnotation(workload, v1.GithubSecretIdAnnotation, oldSecretName)

		fakeCtrlClient := ctrlruntimefake.NewClientBuilder().
			WithObjects(workload, user, role).
			WithScheme(scheme.Scheme).
			Build()
		fakeClientSet := k8sfake.NewSimpleClientset(oldSecret)
		h := Handler{
			Client:           fakeCtrlClient,
			clientSet:        fakeClientSet,
			accessController: authority.NewAccessController(fakeCtrlClient),
		}

		rotation, err := h.updateCICDSecret(ctx, workload, user, patAuth("new_token_456"))
		assert.NilError(t, err)
		assert.Assert(t, rotation != nil)

		h.discardRolledBackCICDSecret(ctx, workload, rotation, user)

		assert.Equal(t, v1.GetGithubSecretId(workload), oldSecretName,
			"Annotation should be put back to the secret that is actually stored")
		_, err = fakeClientSet.CoreV1().Secrets(common.PrimusSafeNamespace).Get(
			ctx, rotation.NewSecretId, metav1.GetOptions{})
		assert.Assert(t, apierrors.IsNotFound(err), "The unreachable replacement should be deleted")
		_, err = fakeClientSet.CoreV1().Secrets(common.PrimusSafeNamespace).Get(
			ctx, oldSecretName, metav1.GetOptions{})
		assert.NilError(t, err, "The superseded secret should still be usable")
	})

	t.Run("clears the annotation when there was no previous secret", func(t *testing.T) {
		workload := genMockWorkload("test-cluster", "test-workspace")
		fakeCtrlClient := ctrlruntimefake.NewClientBuilder().
			WithObjects(workload, user, role).
			WithScheme(scheme.Scheme).
			Build()
		fakeClientSet := k8sfake.NewSimpleClientset()
		h := Handler{
			Client:           fakeCtrlClient,
			clientSet:        fakeClientSet,
			accessController: authority.NewAccessController(fakeCtrlClient),
		}

		rotation, err := h.updateCICDSecret(ctx, workload, user, patAuth("first_token"))
		assert.NilError(t, err)
		assert.Assert(t, rotation != nil)
		assert.Equal(t, rotation.SupersededSecretId, "")

		h.discardRolledBackCICDSecret(ctx, workload, rotation, user)

		assert.Equal(t, v1.GetGithubSecretId(workload), "",
			"Annotation should be cleared, there is nothing to fall back to")
		_, err = fakeClientSet.CoreV1().Secrets(common.PrimusSafeNamespace).Get(
			ctx, rotation.NewSecretId, metav1.GetOptions{})
		assert.Assert(t, apierrors.IsNotFound(err), "The unreachable replacement should be deleted")
	})

	t.Run("is a no-op without a rotation", func(t *testing.T) {
		workload := genMockWorkload("test-cluster", "test-workspace")
		v1.SetAnnotation(workload, v1.GithubSecretIdAnnotation, "untouched-secret")
		h := Handler{clientSet: k8sfake.NewSimpleClientset()}
		h.discardRolledBackCICDSecret(ctx, workload, nil, user)
		assert.Equal(t, v1.GetGithubSecretId(workload), "untouched-secret")
	})
}

// Test_createCICDSecret_Success tests successful creation of CICD secret
func Test_createCICDSecret_Success(t *testing.T) {
	ctx := context.Background()
	clusterId := "test-cluster"
	workspaceId := "test-workspace"

	workload := genMockWorkload(clusterId, workspaceId)
	user := genMockUser()
	role := genMockRole()
	token := "test_github_token_123"

	// Create fake controller-runtime client
	fakeCtrlClient := ctrlruntimefake.NewClientBuilder().
		WithObjects(workload, user, role).
		WithScheme(scheme.Scheme).
		Build()

	// Create fake kubernetes clientset
	fakeClientSet := k8sfake.NewSimpleClientset()

	h := Handler{
		Client:           fakeCtrlClient,
		clientSet:        fakeClientSet,
		accessController: authority.NewAccessController(fakeCtrlClient),
	}

	// Call createCICDSecret
	secret, err := h.createCICDSecret(ctx, workload, user, patAuth(token))

	// Should succeed
	assert.NilError(t, err)
	assert.Assert(t, secret != nil, "Secret should be created")
	assert.Assert(t, secret.Name != "", "Secret should have a name")

	// Verify secret was created in kubernetes
	createdSecret, err := fakeClientSet.CoreV1().Secrets(common.PrimusSafeNamespace).Get(ctx, secret.Name, metav1.GetOptions{})
	assert.NilError(t, err, "Secret should exist in kubernetes")
	assert.Equal(t, string(createdSecret.Data[GitHubToken]), token, "Secret should contain the token")
}

// Test_updateWorkload_GithubPATHandling tests updateWorkload with GithubPAT token update
func Test_updateWorkload_GithubPATHandling(t *testing.T) {
	ctx := context.Background()
	clusterId := "test-cluster"
	workspaceId := "test-workspace"

	// Create a CICD workload
	workload := genMockWorkload(clusterId, workspaceId)
	workload.Spec.Kind = common.CICDScaleRunnerSetKind
	user := genMockUser()
	role := genMockRole()

	oldToken := "old_token_123"
	newToken := "new_token_456"
	oldSecretName := "old-secret-id"

	// Create old secret
	oldSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      oldSecretName,
			Namespace: common.PrimusSafeNamespace,
			Labels: map[string]string{
				v1.SecretTypeLabel: string(v1.SecretGeneral),
				v1.UserIdLabel:     user.Name,
				v1.OwnerLabel:      workload.Name,
			},
		},
		Data: map[string][]byte{
			GitHubToken: []byte(oldToken),
		},
		Type: corev1.SecretTypeOpaque,
	}

	// Set annotation with old secret ID
	v1.SetAnnotation(workload, v1.GithubSecretIdAnnotation, oldSecretName)
	workload.Spec.Env = map[string]string{
		"EXISTING_VAR": "existing_value",
	}

	// Create fake clients
	testScheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(testScheme))
	utilruntime.Must(scheme.AddToScheme(testScheme))

	fakeCtrlClient := ctrlruntimefake.NewClientBuilder().
		WithObjects(workload, user, role).
		WithScheme(testScheme).
		Build()

	fakeClientSet := k8sfake.NewSimpleClientset(oldSecret)

	h := Handler{
		Client:           fakeCtrlClient,
		clientSet:        fakeClientSet,
		accessController: authority.NewAccessController(fakeCtrlClient),
	}

	// Create patch request with new token
	reqEnv := map[string]string{
		GithubPAT:      newToken,
		"EXISTING_VAR": "new_value",
		"NEW_VAR":      "new_var_value",
	}

	req := &view.PatchWorkloadRequest{
		Env: &reqEnv,
	}

	// Call updateWorkload
	err := h.updateWorkload(ctx, workload, user, req)

	// Should succeed
	assert.NilError(t, err, "updateWorkload should succeed")

	// Verify the workload was updated in etcd
	updatedWorkload := &v1.Workload{}
	err = fakeCtrlClient.Get(ctx, client.ObjectKey{Name: workload.Name}, updatedWorkload)
	assert.NilError(t, err, "should retrieve updated workload")

	// Verify annotation is updated to new secret
	newSecretId := v1.GetGithubSecretId(updatedWorkload)
	assert.Assert(t, newSecretId != "", "New secret ID should be set")
	assert.Assert(t, newSecretId != oldSecretName, "New secret ID should be different from old")

	// Verify the new secret was created (using clientSet, not controller-runtime client)
	newSecret, err := fakeClientSet.CoreV1().Secrets(common.PrimusSafeNamespace).Get(ctx, newSecretId, metav1.GetOptions{})
	assert.NilError(t, err, "new secret should be created")

	// Verify new secret contains the new token
	assert.Equal(t, string(newSecret.Data[GitHubToken]), newToken, "new secret should contain new token")

	// Verify old secret was deleted (using clientSet, not controller-runtime client)
	_, err = fakeClientSet.CoreV1().Secrets(common.PrimusSafeNamespace).Get(ctx, oldSecretName, metav1.GetOptions{})
	assert.Assert(t, apierrors.IsNotFound(err), "old secret should be deleted")
}

func Test_updateWorkload_GitHubAppAuthHandling(t *testing.T) {
	ctx := context.Background()
	clusterId := "test-cluster"
	workspaceId := "test-workspace"

	workload := genMockWorkload(clusterId, workspaceId)
	workload.Spec.Kind = common.CICDScaleRunnerSetKind
	workload.Spec.Env = map[string]string{
		"EXISTING_VAR": "existing_value",
	}
	user := genMockUser()
	role := genMockRole()

	oldSecretName := "old-secret-id"
	oldSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      oldSecretName,
			Namespace: common.PrimusSafeNamespace,
			Labels: map[string]string{
				v1.SecretTypeLabel: string(v1.SecretGeneral),
				v1.UserIdLabel:     user.Name,
				v1.OwnerLabel:      workload.Name,
			},
		},
		Data: map[string][]byte{
			GitHubToken: []byte("old_token_123"),
		},
		Type: corev1.SecretTypeOpaque,
	}
	v1.SetAnnotation(workload, v1.GithubSecretIdAnnotation, oldSecretName)

	testScheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(testScheme))
	utilruntime.Must(scheme.AddToScheme(testScheme))

	fakeCtrlClient := ctrlruntimefake.NewClientBuilder().
		WithObjects(workload, user, role).
		WithScheme(testScheme).
		Build()

	fakeClientSet := k8sfake.NewSimpleClientset(oldSecret)

	h := Handler{
		Client:           fakeCtrlClient,
		clientSet:        fakeClientSet,
		accessController: authority.NewAccessController(fakeCtrlClient),
	}

	newAuth := githubAppAuth("12345", "67890", "private-key")
	req := &view.PatchWorkloadRequest{
		GitHubAuth: newAuth,
	}

	err := h.updateWorkload(ctx, workload, user, req)

	assert.NilError(t, err, "updateWorkload should succeed")

	updatedWorkload := &v1.Workload{}
	err = fakeCtrlClient.Get(ctx, client.ObjectKey{Name: workload.Name}, updatedWorkload)
	assert.NilError(t, err, "should retrieve updated workload")

	newSecretId := v1.GetGithubSecretId(updatedWorkload)
	assert.Assert(t, newSecretId != "", "New secret ID should be set")
	assert.Assert(t, newSecretId != oldSecretName, "New secret ID should be different from old")

	newSecret, err := fakeClientSet.CoreV1().Secrets(common.PrimusSafeNamespace).Get(ctx, newSecretId, metav1.GetOptions{})
	assert.NilError(t, err, "new secret should be created")
	assert.Equal(t, string(newSecret.Data[GitHubAppId]), newAuth.AppId)
	assert.Equal(t, string(newSecret.Data[GitHubAppInstallationId]), newAuth.InstallationId)
	assert.Equal(t, string(newSecret.Data[GitHubAppPrivateKey]), newAuth.PrivateKey)
	_, hasPAT := newSecret.Data[GitHubToken]
	assert.Assert(t, !hasPAT, "GitHub App update should not contain PAT key")

	_, err = fakeClientSet.CoreV1().Secrets(common.PrimusSafeNamespace).Get(ctx, oldSecretName, metav1.GetOptions{})
	assert.Assert(t, apierrors.IsNotFound(err), "old secret should be deleted")
}

// Test_updateWorkload_NonCICDWorkload tests updateWorkload with non-CICD workload
func Test_updateWorkload_NonCICDWorkload(t *testing.T) {
	ctx := context.Background()
	clusterId := "test-cluster"
	workspaceId := "test-workspace"

	// Create a normal (non-CICD) workload
	workload := genMockWorkload(clusterId, workspaceId)
	workload.Spec.Kind = "PyTorchJob" // Not a CICD runner
	user := genMockUser()
	role := genMockRole()

	workload.Spec.Env = map[string]string{
		"EXISTING_VAR": "existing_value",
	}

	// Create fake clients
	testScheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(testScheme))
	utilruntime.Must(scheme.AddToScheme(testScheme))

	fakeCtrlClient := ctrlruntimefake.NewClientBuilder().
		WithObjects(workload, user, role).
		WithScheme(testScheme).
		Build()

	fakeClientSet := k8sfake.NewSimpleClientset()

	h := Handler{
		Client:           fakeCtrlClient,
		clientSet:        fakeClientSet,
		accessController: authority.NewAccessController(fakeCtrlClient),
	}

	// Create patch request with GithubPAT (should be ignored for non-CICD workload)
	reqEnv := map[string]string{
		GithubPAT:      "some_token",
		"EXISTING_VAR": "new_value",
		"NEW_VAR":      "new_var_value",
	}

	req := &view.PatchWorkloadRequest{
		Env: &reqEnv,
	}

	// Call updateWorkload
	err := h.updateWorkload(ctx, workload, user, req)

	// Should succeed (GithubPAT handling is skipped for non-CICD workloads)
	assert.NilError(t, err, "updateWorkload should succeed")

	// Verify the workload was updated in etcd
	updatedWorkload := &v1.Workload{}
	err = fakeCtrlClient.Get(ctx, client.ObjectKey{Name: workload.Name}, updatedWorkload)
	assert.NilError(t, err, "should retrieve updated workload")

	// Verify no GithubSecretId annotation is set (since it's not a CICD workload)
	assert.Equal(t, v1.GetGithubSecretId(updatedWorkload), "", "GithubSecretId should not be set for non-CICD workload")
}

// Test_applyWorkloadPatch_GithubPATFiltered tests that applyWorkloadPatch filters out GithubPAT
func Test_applyWorkloadPatch_GithubPATFiltered(t *testing.T) {
	clusterId := "test-cluster"
	workspaceId := "test-workspace"

	workload := genMockWorkload(clusterId, workspaceId)
	workload.Spec.Env = map[string]string{
		"EXISTING_VAR": "existing_value",
	}

	// Create patch request with GithubPAT
	reqEnv := map[string]string{
		GithubPAT:      "new_token_456",
		"EXISTING_VAR": "new_value",
		"NEW_VAR":      "new_var_value",
	}

	req := &view.PatchWorkloadRequest{
		Env: &reqEnv,
	}

	// Call applyWorkloadPatch
	err := applyWorkloadPatch(workload, req)

	// Should succeed
	assert.NilError(t, err)

	// Verify GithubPAT is filtered out from workload env
	_, hasGithubPAT := workload.Spec.Env[GithubPAT]
	assert.Equal(t, hasGithubPAT, false, "GithubPAT should be filtered out from workload env")

	// Verify other env vars are present
	assert.Equal(t, workload.Spec.Env["EXISTING_VAR"], "new_value")
	assert.Equal(t, workload.Spec.Env["NEW_VAR"], "new_var_value")
}

// Test_updateCICDSecret_NoOldSecret tests updating when there's no old secret
func Test_updateCICDSecret_NoOldSecret(t *testing.T) {
	ctx := context.Background()
	clusterId := "test-cluster"
	workspaceId := "test-workspace"

	workload := genMockWorkload(clusterId, workspaceId)
	user := genMockUser()
	role := genMockRole()

	newToken := "new_token_123"

	// No old secret annotation set

	// Create fake clients
	fakeCtrlClient := ctrlruntimefake.NewClientBuilder().
		WithObjects(workload, user, role).
		WithScheme(scheme.Scheme).
		Build()

	fakeClientSet := k8sfake.NewSimpleClientset()

	h := Handler{
		Client:           fakeCtrlClient,
		clientSet:        fakeClientSet,
		accessController: authority.NewAccessController(fakeCtrlClient),
	}

	// Call updateCICDSecret with new token
	rotation, err := h.updateCICDSecret(ctx, workload, user, patAuth(newToken))

	// Should succeed
	assert.NilError(t, err)
	assert.Assert(t, rotation != nil, "A first secret should still be reported as a rotation")
	assert.Equal(t, rotation.SupersededSecretId, "", "There is no previous secret to drop")

	// Verify annotation is set to new secret
	newSecretId := v1.GetGithubSecretId(workload)
	assert.Assert(t, newSecretId != "", "New secret ID should be set")
	assert.Equal(t, rotation.NewSecretId, newSecretId)

	// Verify new secret was created
	newSecret, err := fakeClientSet.CoreV1().Secrets(common.PrimusSafeNamespace).Get(ctx, newSecretId, metav1.GetOptions{})
	assert.NilError(t, err, "New secret should exist")
	assert.Equal(t, string(newSecret.Data[GitHubToken]), newToken, "New secret should contain new token")
}

func Test_updateCICDSecret_MissingAnnotatedOldSecret(t *testing.T) {
	ctx := context.Background()
	clusterId := "test-cluster"
	workspaceId := "test-workspace"

	workload := genMockWorkload(clusterId, workspaceId)
	user := genMockUser()
	role := genMockRole()
	newToken := "new_token_123"
	v1.SetAnnotation(workload, v1.GithubSecretIdAnnotation, "missing-secret")

	fakeCtrlClient := ctrlruntimefake.NewClientBuilder().
		WithObjects(workload, user, role).
		WithScheme(scheme.Scheme).
		Build()
	fakeClientSet := k8sfake.NewSimpleClientset()

	h := Handler{
		Client:           fakeCtrlClient,
		clientSet:        fakeClientSet,
		accessController: authority.NewAccessController(fakeCtrlClient),
	}

	rotation, err := h.updateCICDSecret(ctx, workload, user, patAuth(newToken))
	assert.NilError(t, err)
	assert.Assert(t, rotation != nil)
	assert.Equal(t, rotation.SupersededSecretId, "",
		"A stale annotation points at nothing, so there is nothing to supersede")

	newSecretId := v1.GetGithubSecretId(workload)
	assert.Assert(t, newSecretId != "", "New secret ID should be set")
	assert.Assert(t, newSecretId != "missing-secret", "New secret ID should replace stale annotation")
	newSecret, err := fakeClientSet.CoreV1().Secrets(common.PrimusSafeNamespace).Get(ctx, newSecretId, metav1.GetOptions{})
	assert.NilError(t, err, "New secret should exist")
	assert.Equal(t, string(newSecret.Data[GitHubToken]), newToken, "New secret should contain new token")
}

func Test_updateCICDSecret_OldSecretLookupError(t *testing.T) {
	ctx := context.Background()
	clusterId := "test-cluster"
	workspaceId := "test-workspace"

	workload := genMockWorkload(clusterId, workspaceId)
	user := genMockUser()
	role := genMockRole()
	v1.SetAnnotation(workload, v1.GithubSecretIdAnnotation, "old-secret-id")

	fakeCtrlClient := ctrlruntimefake.NewClientBuilder().
		WithObjects(workload, user, role).
		WithScheme(scheme.Scheme).
		Build()
	fakeClientSet := k8sfake.NewSimpleClientset()
	fakeClientSet.Fake.PrependReactor("get", "secrets", func(action k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("api unavailable")
	})

	h := Handler{
		Client:           fakeCtrlClient,
		clientSet:        fakeClientSet,
		accessController: authority.NewAccessController(fakeCtrlClient),
	}

	rotation, err := h.updateCICDSecret(ctx, workload, user, patAuth("new_token_123"))
	assert.ErrorContains(t, err, "failed to get existing CICD GitHub secret")
	assert.Assert(t, rotation == nil, "A failed lookup must not report a rotation to settle")
	assert.Equal(t, v1.GetGithubSecretId(workload), "old-secret-id")
}

// Test_generateCICDScaleRunnerSet tests generating CICD scale runner set configuration
func Test_generateCICDScaleRunnerSet(t *testing.T) {
	commonconfig.SetValue("cicd.enable", "true")
	defer commonconfig.SetValue("cicd.enable", "")

	ctx := context.Background()
	clusterId := "test-cluster"
	workspaceId := "test-workspace"
	githubToken := "ghp_test_token_123"

	workload := genMockWorkload(clusterId, workspaceId)
	workload.Spec.Env = map[string]string{
		GithubPAT:   githubToken,
		"OTHER_VAR": "other_value",
	}

	user := genMockUser()
	role := genMockRole()

	fakeCtrlClient := ctrlruntimefake.NewClientBuilder().
		WithObjects(workload, user, role).
		WithScheme(scheme.Scheme).
		Build()

	fakeClientSet := k8sfake.NewSimpleClientset()

	h := Handler{
		Client:           fakeCtrlClient,
		clientSet:        fakeClientSet,
		accessController: authority.NewAccessController(fakeCtrlClient),
	}

	// Call generateCICDScaleRunnerSet
	err := h.generateCICDScaleRunnerSet(ctx, workload, user, nil)

	// Should succeed
	assert.NilError(t, err)

	// Verify GithubPAT was removed from workload.Spec.Env
	_, exists := workload.Spec.Env[GithubPAT]
	assert.Assert(t, !exists, "GithubPAT should be removed from Spec.Env")
	assert.Equal(t, workload.Spec.Env["OTHER_VAR"], "other_value", "Other env vars should remain")

	// Verify secret annotation was set
	secretId := v1.GetGithubSecretId(workload)
	assert.Assert(t, secretId != "", "Secret ID annotation should be set")

	// Verify secret was created in kubernetes
	secret, err := fakeClientSet.CoreV1().Secrets(common.PrimusSafeNamespace).Get(ctx, secretId, metav1.GetOptions{})
	assert.NilError(t, err)
	assert.Assert(t, secret != nil, "Secret should be created")
}

func Test_generateCICDScaleRunnerSet_GitHubApp(t *testing.T) {
	commonconfig.SetValue("cicd.enable", "true")
	defer commonconfig.SetValue("cicd.enable", "")

	ctx := context.Background()
	clusterId := "test-cluster"
	workspaceId := "test-workspace"
	auth := githubAppAuth("12345", "67890", "private-key")

	workload := genMockWorkload(clusterId, workspaceId)
	workload.Spec.Env = map[string]string{
		"OTHER_VAR": "other_value",
	}

	user := genMockUser()
	role := genMockRole()

	fakeCtrlClient := ctrlruntimefake.NewClientBuilder().
		WithObjects(workload, user, role).
		WithScheme(scheme.Scheme).
		Build()

	fakeClientSet := k8sfake.NewSimpleClientset()

	h := Handler{
		Client:           fakeCtrlClient,
		clientSet:        fakeClientSet,
		accessController: authority.NewAccessController(fakeCtrlClient),
	}

	err := h.generateCICDScaleRunnerSet(ctx, workload, user, auth)

	assert.NilError(t, err)
	assert.Equal(t, workload.Spec.Env["OTHER_VAR"], "other_value", "Other env vars should remain")

	secretId := v1.GetGithubSecretId(workload)
	assert.Assert(t, secretId != "", "Secret ID annotation should be set")

	secret, err := fakeClientSet.CoreV1().Secrets(common.PrimusSafeNamespace).Get(ctx, secretId, metav1.GetOptions{})
	assert.NilError(t, err)
	assert.Equal(t, string(secret.Data[GitHubAppId]), auth.AppId)
	assert.Equal(t, string(secret.Data[GitHubAppInstallationId]), auth.InstallationId)
	assert.Equal(t, string(secret.Data[GitHubAppPrivateKey]), auth.PrivateKey)
	_, hasPAT := secret.Data[GitHubToken]
	assert.Assert(t, !hasPAT, "GitHub App secret should not contain PAT key")
}

// genMockCICDWorkload returns a workload that satisfies IsCICDScalingRunnerSet.
func genMockCICDWorkload(clusterId, workspaceId, name, displayName string) *v1.Workload {
	workload := genMockWorkload(clusterId, workspaceId)
	workload.Name = name
	v1.SetLabel(workload, v1.DisplayNameLabel, displayName)
	workload.Spec.GroupVersionKind = v1.GroupVersionKind{
		Group:   "",
		Version: "v1",
		Kind:    common.CICDScaleRunnerSetKind,
	}
	workload.Spec.Env = map[string]string{
		common.ScaleRunnerSetID: "test-runner-set",
	}
	return workload
}

// Test_cleanupCICDSecrets_CICDWorkload tests cleanup deletes the secrets a CICD workload
// owns. The secrets are found by owner label, not by name: createCICDSecret names them
// with commonutils.GenerateName, so the object in the cluster never has the workload's
// display name and a name-based lookup would silently clean up nothing.
func Test_cleanupCICDSecrets_CICDWorkload(t *testing.T) {
	ctx := context.Background()
	workspaceId := "test-workspace"
	clusterId := "test-cluster"

	user := genMockUser()
	role := genMockRole()

	workload := genMockCICDWorkload(clusterId, workspaceId, "cicd-runner-workload", "CICD Runner")

	fakeCtrlClient := ctrlruntimefake.NewClientBuilder().
		WithObjects(user, role, workload).
		WithScheme(scheme.Scheme).
		Build()

	// A secret owned by a different workload must survive the sweep.
	otherSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "other-workload-secret",
			Namespace: common.PrimusSafeNamespace,
			Labels: map[string]string{
				v1.OwnerLabel: "some-other-workload",
			},
		},
		Data: map[string][]byte{GitHubToken: []byte("other-token")},
	}

	fakeClientSet := k8sfake.NewSimpleClientset(otherSecret)

	h := Handler{
		Client:           fakeCtrlClient,
		clientSet:        fakeClientSet,
		accessController: authority.NewAccessController(fakeCtrlClient),
	}

	// Create the secret the same way the CICD path does, so its name carries the random
	// suffix that GenerateName adds.
	secret, err := h.createCICDSecret(ctx, workload, user, patAuth("test-token"))
	assert.NilError(t, err)
	assert.Assert(t, secret.Name != v1.GetDisplayName(workload),
		"GenerateName must produce a name that differs from the display name")
	assert.Equal(t, secret.Labels[v1.OwnerLabel], workload.Name)

	// Call cleanupCICDSecrets on CICD workload
	h.cleanupCICDSecrets(ctx, workload)

	// Verify the owned secret was deleted
	_, err = fakeClientSet.CoreV1().Secrets(common.PrimusSafeNamespace).Get(ctx, secret.Name, metav1.GetOptions{})
	assert.Assert(t, apierrors.IsNotFound(err), "Secret should be deleted after cleanup")

	// Verify another workload's secret was left alone
	_, err = fakeClientSet.CoreV1().Secrets(common.PrimusSafeNamespace).Get(
		ctx, otherSecret.Name, metav1.GetOptions{})
	assert.NilError(t, err, "Cleanup must not touch secrets owned by another workload")
}

// Test_cleanupCICDSecrets_Guards covers the inputs that must make cleanup a no-op: a nil
// workload (cleanUpWorkloads calls this before its own nil check) and a non-CICD workload.
func Test_cleanupCICDSecrets_Guards(t *testing.T) {
	ctx := context.Background()
	user := genMockUser()
	role := genMockRole()

	workload := genMockWorkload("test-cluster", "test-workspace")
	workload.Name = "plain-workload"
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "plain-workload-secret",
			Namespace: common.PrimusSafeNamespace,
			Labels:    map[string]string{v1.OwnerLabel: workload.Name},
		},
	}

	fakeCtrlClient := ctrlruntimefake.NewClientBuilder().
		WithObjects(user, role, workload).
		WithScheme(scheme.Scheme).
		Build()
	fakeClientSet := k8sfake.NewSimpleClientset(secret)
	h := Handler{
		Client:           fakeCtrlClient,
		clientSet:        fakeClientSet,
		accessController: authority.NewAccessController(fakeCtrlClient),
	}

	// Must not panic.
	h.cleanupCICDSecrets(ctx, nil)

	h.cleanupCICDSecrets(ctx, workload)
	_, err := fakeClientSet.CoreV1().Secrets(common.PrimusSafeNamespace).Get(ctx, secret.Name, metav1.GetOptions{})
	assert.NilError(t, err, "A non-CICD workload should not have its secrets swept")
}
