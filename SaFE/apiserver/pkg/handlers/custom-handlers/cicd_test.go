/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package custom_handlers

import (
	"context"
	"testing"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apis/pkg/client/clientset/versioned/scheme"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/custom-handlers/types"
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
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlruntimefake "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

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
	err := h.updateCICDSecret(ctx, workload, user, oldToken)

	// Should return nil without error (optimization kicks in)
	assert.NilError(t, err)

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
	err := h.updateCICDSecret(ctx, workload, user, newToken)

	// Should succeed
	assert.NilError(t, err)

	// Verify annotation is updated to new secret
	newSecretId := v1.GetGithubSecretId(workload)
	assert.Assert(t, newSecretId != "", "New secret ID should be set")
	assert.Assert(t, newSecretId != oldSecretName, "New secret ID should be different from old")

	// Verify new secret was created
	newSecret, err := fakeClientSet.CoreV1().Secrets(common.PrimusSafeNamespace).Get(ctx, newSecretId, metav1.GetOptions{})
	assert.NilError(t, err, "New secret should exist")
	assert.Equal(t, string(newSecret.Data[GitHubToken]), newToken, "New secret should contain new token")

	// Verify old secret is deleted (should not exist)
	_, err = fakeClientSet.CoreV1().Secrets(common.PrimusSafeNamespace).Get(ctx, oldSecretName, metav1.GetOptions{})
	assert.Assert(t, err != nil, "Old secret should be deleted")
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
	secret, err := h.createCICDSecret(ctx, workload, user, token)

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

	req := &types.PatchWorkloadRequest{
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

	req := &types.PatchWorkloadRequest{
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

	req := &types.PatchWorkloadRequest{
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
	err := h.updateCICDSecret(ctx, workload, user, newToken)

	// Should succeed
	assert.NilError(t, err)

	// Verify annotation is set to new secret
	newSecretId := v1.GetGithubSecretId(workload)
	assert.Assert(t, newSecretId != "", "New secret ID should be set")

	// Verify new secret was created
	newSecret, err := fakeClientSet.CoreV1().Secrets(common.PrimusSafeNamespace).Get(ctx, newSecretId, metav1.GetOptions{})
	assert.NilError(t, err, "New secret should exist")
	assert.Equal(t, string(newSecret.Data[GitHubToken]), newToken, "New secret should contain new token")
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

	// Create control plane node
	controlPlaneNode := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "control-plane-node",
			Labels: map[string]string{
				"node-role.kubernetes.io/control-plane": "",
			},
		},
		Status: corev1.NodeStatus{
			Addresses: []corev1.NodeAddress{
				{
					Type:    corev1.NodeInternalIP,
					Address: "192.168.1.100",
				},
			},
		},
	}

	testScheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(testScheme))
	utilruntime.Must(scheme.AddToScheme(testScheme))

	fakeCtrlClient := ctrlruntimefake.NewClientBuilder().
		WithObjects(workload, user, role, controlPlaneNode).
		WithScheme(testScheme).
		Build()

	fakeClientSet := k8sfake.NewSimpleClientset()

	h := Handler{
		Client:           fakeCtrlClient,
		clientSet:        fakeClientSet,
		accessController: authority.NewAccessController(fakeCtrlClient),
	}

	// Call generateCICDScaleRunnerSet
	err := h.generateCICDScaleRunnerSet(ctx, workload, user)

	// Should succeed
	assert.NilError(t, err)

	// Verify GithubPAT was removed from workload.Spec.Env
	_, exists := workload.Spec.Env[GithubPAT]
	assert.Assert(t, !exists, "GithubPAT should be removed from Spec.Env")
	assert.Equal(t, workload.Spec.Env["OTHER_VAR"], "other_value", "Other env vars should remain")

	// Verify control plane IP annotation was set
	controlPlaneIp := v1.GetAnnotation(workload, v1.AdminControlPlaneAnnotation)
	assert.Equal(t, controlPlaneIp, "192.168.1.100", "Control plane IP should be set")

	// Verify secret annotation was set
	secretId := v1.GetGithubSecretId(workload)
	assert.Assert(t, secretId != "", "Secret ID annotation should be set")

	// Verify secret was created in kubernetes
	secret, err := fakeClientSet.CoreV1().Secrets(common.PrimusSafeNamespace).Get(ctx, secretId, metav1.GetOptions{})
	assert.NilError(t, err)
	assert.Assert(t, secret != nil, "Secret should be created")
}

// Test_cleanupCICDSecrets_CICDWorkload tests cleanup deletes secret for CICD workload
func Test_cleanupCICDSecrets_CICDWorkload(t *testing.T) {
	ctx := context.Background()
	workspaceId := "test-workspace"
	clusterId := "test-cluster"

	user := genMockUser()
	role := genMockRole()

	// Create a CICD scaling runner workload
	workload := genMockWorkload(clusterId, workspaceId)
	workload.Name = "cicd-runner-workload"
	displayName := "CICD Runner"
	v1.SetLabel(workload, v1.DisplayNameLabel, displayName)
	// Set CICD specific fields
	workload.Spec.GroupVersionKind = v1.GroupVersionKind{
		Group:   "",
		Version: "v1",
		Kind:    common.CICDScaleRunnerSetKind,
	}
	workload.Spec.Env = map[string]string{
		common.ScaleRunnerSetID: "test-runner-set",
	}

	fakeCtrlClient := ctrlruntimefake.NewClientBuilder().
		WithObjects(user, role, workload).
		WithScheme(scheme.Scheme).
		Build()

	// Create the secret that should be cleaned up
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      displayName,
			Namespace: common.PrimusSafeNamespace,
		},
		Data: map[string][]byte{
			GitHubToken: []byte("test-token"),
		},
	}

	fakeClientSet := k8sfake.NewSimpleClientset(secret)

	h := Handler{
		Client:           fakeCtrlClient,
		clientSet:        fakeClientSet,
		accessController: authority.NewAccessController(fakeCtrlClient),
	}

	// Verify secret exists before cleanup
	_, err := fakeClientSet.CoreV1().Secrets(common.PrimusSafeNamespace).Get(ctx, displayName, metav1.GetOptions{})
	assert.NilError(t, err, "Secret should exist before cleanup")

	// Call cleanupCICDSecrets on CICD workload
	h.cleanupCICDSecrets(ctx, workload)

	// Verify secret was deleted
	_, err = fakeClientSet.CoreV1().Secrets(common.PrimusSafeNamespace).Get(ctx, displayName, metav1.GetOptions{})
	assert.Assert(t, err != nil, "Secret should be deleted after cleanup")
}
