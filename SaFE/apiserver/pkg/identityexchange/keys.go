/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package identityexchange

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"

	"github.com/go-jose/go-jose/v4"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// SigningSecretName is the Secret holding the signing keys.
	SigningSecretName = "identity-exchange-signing"
	// SigningSecretCurrent holds the PEM (PKCS#8) private key used to sign.
	SigningSecretCurrent = "current.pem"
	// SigningSecretPrevious optionally holds the previous key, which is still
	// published in the JWKS (and accepted) so tokens signed before a rotation
	// stay valid until they expire.
	SigningSecretPrevious = "previous.pem"

	AlgES256 = "ES256"
	AlgRS256 = "RS256"
)

// KeySet signs tokens with the current key and publishes the public parts of
// the current and previous keys.
type KeySet struct {
	signer  jose.Signer
	current jose.JSONWebKey // private
	public  []jose.JSONWebKey
}

// GenerateSigningKey creates a new private key for the algorithm and returns it as PEM.
func GenerateSigningKey(alg string) ([]byte, error) {
	var key crypto.PrivateKey
	var err error
	switch alg {
	case AlgES256, "":
		key, err = ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	case AlgRS256:
		key, err = rsa.GenerateKey(rand.Reader, 2048)
	default:
		return nil, fmt.Errorf("unsupported signing algorithm %q", alg)
	}
	if err != nil {
		return nil, err
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return nil, err
	}
	return pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}), nil
}

// NewKeySet builds a KeySet from PEM encoded keys. previous may be empty.
func NewKeySet(currentPEM, previousPEM []byte) (*KeySet, error) {
	cur, err := parsePrivateKey(currentPEM)
	if err != nil {
		return nil, fmt.Errorf("current signing key: %w", err)
	}
	ks := &KeySet{current: *cur}
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.SignatureAlgorithm(cur.Algorithm), Key: cur},
		(&jose.SignerOptions{}).WithType("at+jwt"))
	if err != nil {
		return nil, err
	}
	ks.signer = signer
	ks.public = append(ks.public, cur.Public())
	if len(previousPEM) > 0 {
		prev, err := parsePrivateKey(previousPEM)
		if err != nil {
			return nil, fmt.Errorf("previous signing key: %w", err)
		}
		ks.public = append(ks.public, prev.Public())
	}
	return ks, nil
}

func parsePrivateKey(p []byte) (*jose.JSONWebKey, error) {
	block, _ := pem.Decode(p)
	if block == nil {
		return nil, errors.New("no PEM block")
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	jwk := &jose.JSONWebKey{Key: parsed, Use: "sig"}
	switch k := parsed.(type) {
	case *ecdsa.PrivateKey:
		if k.Curve != elliptic.P256() {
			return nil, errors.New("only P-256 EC keys are supported")
		}
		jwk.Algorithm = AlgES256
	case *rsa.PrivateKey:
		if k.N.BitLen() < 2048 {
			return nil, errors.New("RSA keys must be at least 2048 bits")
		}
		jwk.Algorithm = AlgRS256
	default:
		return nil, fmt.Errorf("unsupported key type %T", parsed)
	}
	thumb, err := jwk.Thumbprint(crypto.SHA256)
	if err != nil {
		return nil, err
	}
	jwk.KeyID = base64.RawURLEncoding.EncodeToString(thumb)
	return jwk, nil
}

// Algorithm returns the JWS algorithm of the current key.
func (k *KeySet) Algorithm() string { return k.current.Algorithm }

// KeyID returns the kid of the current key.
func (k *KeySet) KeyID() string { return k.current.KeyID }

// Sign serializes a compact JWS over the payload.
func (k *KeySet) Sign(payload []byte) (string, error) {
	obj, err := k.signer.Sign(payload)
	if err != nil {
		return "", err
	}
	return obj.CompactSerialize()
}

// JWKS returns the public keys, current first.
func (k *KeySet) JWKS() jose.JSONWebKeySet {
	return jose.JSONWebKeySet{Keys: append([]jose.JSONWebKey(nil), k.public...)}
}

// verificationKey returns the public key for a kid.
func (k *KeySet) verificationKey(kid string) (*jose.JSONWebKey, bool) {
	for i := range k.public {
		if k.public[i].KeyID == kid {
			return &k.public[i], true
		}
	}
	return nil, false
}

// LoadOrCreateKeySet reads the signing keys from the Secret, creating the
// Secret with a fresh key when it does not exist. All replicas read the same
// Secret, so they sign and verify with the same keys.
func LoadOrCreateKeySet(ctx context.Context, cli client.Client, namespace, alg string) (*KeySet, error) {
	key := client.ObjectKey{Namespace: namespace, Name: SigningSecretName}
	secret := &corev1.Secret{}
	err := cli.Get(ctx, key, secret)
	if apierrors.IsNotFound(err) {
		pemBytes, genErr := GenerateSigningKey(alg)
		if genErr != nil {
			return nil, genErr
		}
		secret = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: SigningSecretName, Namespace: namespace},
			Type:       corev1.SecretTypeOpaque,
			Data:       map[string][]byte{SigningSecretCurrent: pemBytes},
		}
		if createErr := cli.Create(ctx, secret); createErr != nil {
			if !apierrors.IsAlreadyExists(createErr) {
				return nil, createErr
			}
			// Another replica created it first: use theirs.
			if err = cli.Get(ctx, key, secret); err != nil {
				return nil, err
			}
		} else {
			klog.Info("created identity exchange signing key secret")
		}
	} else if err != nil {
		return nil, err
	}
	cur := secret.Data[SigningSecretCurrent]
	if len(cur) == 0 {
		return nil, fmt.Errorf("secret %s has no %s", SigningSecretName, SigningSecretCurrent)
	}
	return NewKeySet(cur, secret.Data[SigningSecretPrevious])
}
