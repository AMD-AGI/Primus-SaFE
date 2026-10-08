/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package imagedigest

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	corev1 "k8s.io/api/core/v1"

	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
)

// ResolveFunc resolves a container image reference to a digest-pinned form.
// Tests replace it to avoid network access.
var ResolveFunc = Resolve

// Resolve returns image@sha256:… for a tag or digest reference. Already-pinned
// references are returned unchanged. keychain may be nil (anonymous pull).
func Resolve(ctx context.Context, image string, keychain authn.Keychain) (string, error) {
	image = strings.TrimSpace(image)
	if image == "" {
		return "", fmt.Errorf("empty image reference")
	}
	if IsPinned(image) {
		return image, nil
	}
	ref, err := name.ParseReference(image)
	if err != nil {
		return "", fmt.Errorf("parse image %q: %w", image, err)
	}
	opts := []remote.Option{remote.WithContext(ctx)}
	if keychain != nil {
		opts = append(opts, remote.WithAuthFromKeychain(keychain))
	}
	tr, err := registryTransport()
	if err != nil {
		return "", err
	}
	if tr != nil {
		opts = append(opts, remote.WithTransport(tr))
	}
	digest, err := resolveDigest(ref, opts...)
	if err != nil {
		return "", fmt.Errorf("resolve digest for %q: %w", image, err)
	}
	if digest == "" {
		return "", fmt.Errorf("registry returned empty digest for %q", image)
	}
	return ref.Context().Name() + "@" + digest, nil
}

func resolveDigest(ref name.Reference, opts ...remote.Option) (string, error) {
	if desc, err := remote.Head(ref, opts...); err == nil && desc != nil {
		return desc.Digest.String(), nil
	}
	got, err := remote.Get(ref, opts...)
	if err != nil {
		return "", err
	}
	return got.Digest.String(), nil
}

// registryTransport builds an HTTP transport for registry TLS. nil means use
// go-containerregistry defaults (process system roots).
func registryTransport() (*http.Transport, error) {
	skipVerify := commonconfig.IsExternalRegistryInsecureSkipVerify()
	caPath := strings.TrimSpace(commonconfig.GetExternalRegistryCAPath())
	if !skipVerify && caPath == "" {
		return nil, nil
	}

	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12} //nolint:gosec // MinVersion set
	if caPath != "" {
		// A configured CA is the site trust source; do not disable verify over it.
		pem, err := os.ReadFile(caPath)
		if err != nil {
			return nil, fmt.Errorf("read registry CA %q: %w", caPath, err)
		}
		pool, err := x509.SystemCertPool()
		if err != nil || pool == nil {
			pool = x509.NewCertPool()
		}
		if !pool.AppendCertsFromPEM(pem) {
			return nil, fmt.Errorf("registry CA %q: no certificates parsed", caPath)
		}
		tlsConfig.RootCAs = pool
	} else if skipVerify {
		tlsConfig.InsecureSkipVerify = true //nolint:gosec // explicit deployment escape hatch
	}

	base, ok := http.DefaultTransport.(*http.Transport)
	if !ok || base == nil {
		return &http.Transport{TLSClientConfig: tlsConfig, Proxy: http.ProxyFromEnvironment}, nil
	}
	tr := base.Clone()
	tr.TLSClientConfig = tlsConfig
	return tr, nil
}

// IsPinned reports whether a reference already names immutable content.
func IsPinned(image string) bool {
	at := strings.LastIndex(image, "@sha256:")
	return at > 0 && len(image) == at+len("@sha256:")+64
}

// KeychainFromDockerConfigJSON builds a keychain from a .dockerconfigjson secret payload.
func KeychainFromDockerConfigJSON(data []byte) (authn.Keychain, error) {
	if len(data) == 0 {
		return authn.DefaultKeychain, nil
	}
	var cfg dockerConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &staticKeychain{auths: cfg.Auths}, nil
}

// KeychainFromPullSecret reads .dockerconfigjson or .dockercfg from a Secret.
func KeychainFromPullSecret(secret *corev1.Secret) (authn.Keychain, error) {
	if secret == nil {
		return authn.DefaultKeychain, nil
	}
	if raw, ok := secret.Data[corev1.DockerConfigJsonKey]; ok {
		return KeychainFromDockerConfigJSON(raw)
	}
	if raw, ok := secret.Data[corev1.DockerConfigKey]; ok {
		return KeychainFromDockerConfigJSON(raw)
	}
	return authn.DefaultKeychain, nil
}

type dockerConfig struct {
	Auths map[string]dockerAuth `json:"auths"`
}

type dockerAuth struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Auth     string `json:"auth"`
}

type staticKeychain struct {
	auths map[string]dockerAuth
}

func (k *staticKeychain) Resolve(resource authn.Resource) (authn.Authenticator, error) {
	if k == nil || len(k.auths) == 0 {
		return authn.Anonymous, nil
	}
	host := resource.RegistryStr()
	if a, ok := k.auths[host]; ok {
		return authFromDocker(a)
	}
	if a, ok := k.auths["https://"+host]; ok {
		return authFromDocker(a)
	}
	if a, ok := k.auths["http://"+host]; ok {
		return authFromDocker(a)
	}
	// Docker Hub special-case keys.
	for _, key := range []string{"https://index.docker.io/v1/", "index.docker.io", "docker.io"} {
		if a, ok := k.auths[key]; ok && (host == "index.docker.io" || host == "registry-1.docker.io" || host == "docker.io") {
			return authFromDocker(a)
		}
	}
	return authn.Anonymous, nil
}

func authFromDocker(a dockerAuth) (authn.Authenticator, error) {
	user, pass := a.Username, a.Password
	if user == "" && pass == "" && a.Auth != "" {
		raw, err := base64.StdEncoding.DecodeString(a.Auth)
		if err != nil {
			return nil, err
		}
		parts := strings.SplitN(string(raw), ":", 2)
		if len(parts) == 2 {
			user, pass = parts[0], parts[1]
		}
	}
	if user == "" && pass == "" {
		return authn.Anonymous, nil
	}
	return &authn.Basic{Username: user, Password: pass}, nil
}
