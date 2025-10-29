/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package secure

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"

	"golang.org/x/crypto/ssh"
)

// GenerateKey generates an RSA key pair with the specified bit size.
// It returns the private key, public key, and any error encountered during generation.
func GenerateKey(bits int) (*rsa.PrivateKey, *rsa.PublicKey, error) {
	private, err := rsa.GenerateKey(rand.Reader, bits)
	if err != nil {
		return nil, nil, err
	}
	return private, &private.PublicKey, nil
}

// EncodePrivateKey encodes an RSA private key into PEM format.
// It returns the PEM-encoded byte slice of the private key.
func EncodePrivateKey(private *rsa.PrivateKey) []byte {
	return pem.EncodeToMemory(&pem.Block{
		Bytes: x509.MarshalPKCS1PrivateKey(private),
		Type:  "RSA PRIVATE KEY",
	})
}

// MakeSSHKeyPair generates an SSH key pair (2048 bits) and returns both keys in PEM format.
// It returns the private key, public key (in SSH authorized_keys format), and any error encountered.
func MakeSSHKeyPair() ([]byte, []byte, error) {
	pkey, pubkey, err := GenerateKey(2048)
	if err != nil {
		return nil, nil, err
	}

	pub, err := EncodeSSHKey(pubkey)
	if err != nil {
		return nil, nil, err
	}

	return EncodePrivateKey(pkey), pub, nil
}

// EncodeSSHKey encodes an RSA public key into SSH authorized_keys format.
// It converts the public key to SSH format and returns the marshaled authorized key bytes.
func EncodeSSHKey(public *rsa.PublicKey) ([]byte, error) {
	publicKey, err := ssh.NewPublicKey(public)
	if err != nil {
		return nil, err
	}
	return ssh.MarshalAuthorizedKey(publicKey), nil
}
