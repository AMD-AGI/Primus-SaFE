/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package exportimage

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/v1/remote"
)

// dockerConfig is the subset of a Docker config.json the keychain reads.
type dockerConfig struct {
	Auths map[string]authn.AuthConfig `json:"auths"`
}

// configKeychain answers with the credential config.json holds for a registry, and
// anonymously for any other. The credential lives only in this process: nothing is
// written into the user's container.
type configKeychain struct {
	auths map[string]authn.AuthConfig
}

// NewConfigKeychain parses a Docker config.json.
func NewConfigKeychain(configJSON []byte) (authn.Keychain, error) {
	var cfg dockerConfig
	if err := json.Unmarshal(configJSON, &cfg); err != nil {
		return nil, fmt.Errorf("parsing registry credentials: %w", err)
	}
	auths := make(map[string]authn.AuthConfig, len(cfg.Auths))
	for host, a := range cfg.Auths {
		auths[normalizeRegistryKey(host)] = a
	}
	return &configKeychain{auths: auths}, nil
}

func normalizeRegistryKey(host string) string {
	host = strings.TrimPrefix(strings.TrimPrefix(host, "https://"), "http://")
	return strings.TrimSuffix(host, "/")
}

// Resolve implements authn.Keychain.
func (k *configKeychain) Resolve(r authn.Resource) (authn.Authenticator, error) {
	if a, ok := k.auths[r.RegistryStr()]; ok {
		return authn.FromConfig(a), nil
	}
	return authn.Anonymous, nil
}

// NewTransport returns a transport that trusts the system roots plus the given PEM
// bundles (a registry signed by a private CA).
func NewTransport(extraCAPEMs ...[]byte) (http.RoundTripper, error) {
	pool, err := x509.SystemCertPool()
	if err != nil || pool == nil {
		pool = x509.NewCertPool()
	}
	for _, pem := range extraCAPEMs {
		if len(pem) == 0 {
			continue
		}
		if !pool.AppendCertsFromPEM(pem) {
			return nil, fmt.Errorf("a registry CA bundle holds no usable certificate")
		}
	}
	t := remote.DefaultTransport.(*http.Transport).Clone()
	t.TLSClientConfig = &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}
	return t, nil
}
