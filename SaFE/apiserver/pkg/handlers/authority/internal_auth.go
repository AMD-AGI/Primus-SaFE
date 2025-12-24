/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package authority

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"sync"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
)

const (
	// InternalAuthSecretName is the name of the secret containing the internal auth token
	InternalAuthSecretName = "apiserver-internal-auth"
	// InternalAuthSecretKey is the key in the secret data for the token
	InternalAuthSecretKey = "token"
	// InternalAuthTokenHeader is the header name for internal service authentication
	InternalAuthTokenHeader = "X-Internal-Token"
	// tokenLength is the length of the random token in bytes (will be 64 hex chars)
	tokenLength = 32
)

var (
	internalAuthOnce     sync.Once
	internalAuthInstance *InternalAuth
)

// InternalAuth manages internal service authentication using a shared secret
type InternalAuth struct {
	client.Client
	token string
}

// NewInternalAuth creates or loads the internal auth secret and returns the InternalAuth instance
func NewInternalAuth(cli client.Client) (*InternalAuth, error) {
	var initErr error
	internalAuthOnce.Do(func() {
		internalAuthInstance, initErr = initializeInternalAuth(cli)
	})
	if initErr != nil {
		return nil, initErr
	}
	return internalAuthInstance, nil
}

// InternalAuthInstance returns the singleton instance of InternalAuth
func InternalAuthInstance() *InternalAuth {
	return internalAuthInstance
}

// initializeInternalAuth initializes the InternalAuth by loading or creating the secret
func initializeInternalAuth(cli client.Client) (*InternalAuth, error) {
	auth := &InternalAuth{
		Client: cli,
	}

	ctx := context.Background()
	secret := &corev1.Secret{}
	err := cli.Get(ctx, client.ObjectKey{
		Namespace: common.PrimusSafeNamespace,
		Name:      InternalAuthSecretName,
	}, secret)

	if err != nil {
		if !apierrors.IsNotFound(err) {
			klog.ErrorS(err, "failed to get internal auth secret")
			return nil, err
		}

		// Secret not found, create a new one
		token, genErr := generateRandomToken()
		if genErr != nil {
			klog.ErrorS(genErr, "failed to generate random token")
			return nil, genErr
		}

		secret = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      InternalAuthSecretName,
				Namespace: common.PrimusSafeNamespace,
			},
			Type: corev1.SecretTypeOpaque,
			Data: map[string][]byte{
				InternalAuthSecretKey: []byte(token),
			},
		}

		if createErr := cli.Create(ctx, secret); createErr != nil {
			if !apierrors.IsAlreadyExists(createErr) {
				klog.ErrorS(createErr, "failed to create internal auth secret")
				return nil, createErr
			}
			// Secret was created by another instance, try to load it
			if getErr := cli.Get(ctx, client.ObjectKey{
				Namespace: common.PrimusSafeNamespace,
				Name:      InternalAuthSecretName,
			}, secret); getErr != nil {
				klog.ErrorS(getErr, "failed to get internal auth secret after create conflict")
				return nil, getErr
			}
		} else {
			klog.Info("created internal auth secret")
		}
	}

	tokenBytes, ok := secret.Data[InternalAuthSecretKey]
	if !ok || len(tokenBytes) == 0 {
		err := apierrors.NewBadRequest("internal auth secret does not contain token")
		klog.ErrorS(err, "internal auth secret does not contain token")
		return nil, err
	}

	auth.token = string(tokenBytes)
	klog.Info("internal auth initialized successfully")
	return auth, nil
}

// Validate checks if the provided token matches the internal auth token
func (a *InternalAuth) Validate(token string) bool {
	if a == nil || a.token == "" || token == "" {
		return false
	}
	return a.token == token
}

// generateRandomToken generates a cryptographically secure random token
func generateRandomToken() (string, error) {
	bytes := make([]byte, tokenLength)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}
