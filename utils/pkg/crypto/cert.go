/*
 * Copyright © AMD. 2025-2026. All rights reserved.
 */

package crypto

import (
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
)

// ParseCertificate parses a PEM encoded certificate.
func ParseCertificate(content string) (*x509.Certificate, error) {
	cert, err := base64.StdEncoding.DecodeString(content)
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(cert)
	if block == nil {
		return nil, fmt.Errorf("failed to decode certificate")
	}
	certificate, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, err
	}

	return certificate, nil
}

func DecodeCertificate(content string) ([]byte, error) {
	if content == "" {
		return []byte{}, nil
	}
	return base64.StdEncoding.DecodeString(content)
}
