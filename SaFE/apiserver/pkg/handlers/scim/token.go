/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package scim

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
)

const tokenKey = "token"

// EnsureToken returns the SCIM bearer token, creating the Secret when it is absent.
// The token is 32 random bytes, stored hex-encoded.
func EnsureToken(ctx context.Context, cli client.Client) (string, error) {
	name := commonconfig.GetSCIMTokenSecret()
	key := client.ObjectKey{Namespace: common.PrimusSafeNamespace, Name: name}
	secret := &corev1.Secret{}
	err := cli.Get(ctx, key, secret)
	if apierrors.IsNotFound(err) {
		raw := make([]byte, 32)
		if _, err = rand.Read(raw); err != nil {
			return "", err
		}
		secret = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: common.PrimusSafeNamespace},
			Type:       corev1.SecretTypeOpaque,
			Data:       map[string][]byte{tokenKey: []byte(hex.EncodeToString(raw))},
		}
		if err = cli.Create(ctx, secret); err != nil {
			if !apierrors.IsAlreadyExists(err) {
				return "", err
			}
			if err = cli.Get(ctx, key, secret); err != nil {
				return "", err
			}
		} else {
			klog.Infof("created scim token secret %s/%s", common.PrimusSafeNamespace, name)
		}
	} else if err != nil {
		return "", err
	}
	token := strings.TrimSpace(string(secret.Data[tokenKey]))
	if token == "" {
		return "", fmt.Errorf("secret %s/%s has no %s", common.PrimusSafeNamespace, name, tokenKey)
	}
	return token, nil
}

// bearerMatches reports whether the Authorization header carries token.
// The comparison does not stop early on the token bytes.
func bearerMatches(header, token string) bool {
	got := ""
	fields := strings.Fields(header)
	if len(fields) == 2 && strings.EqualFold(fields[0], "bearer") {
		got = fields[1]
	}
	sum := sha256.Sum256([]byte(got))
	want := sha256.Sum256([]byte(token))
	equal := subtle.ConstantTimeCompare(sum[:], want[:]) == 1
	if token == "" || got == "" {
		return false
	}
	return equal
}
