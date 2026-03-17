// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package registry

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// RegistryAuth holds credentials for authenticating with a container registry.
type RegistryAuth struct {
	Username string
	Password string
}

// AuthResolver reads imagePullSecrets from Kubernetes to resolve registry credentials.
type AuthResolver struct {
	k8sClient kubernetes.Interface
}

// NewAuthResolver creates a new AuthResolver backed by the given Kubernetes client.
func NewAuthResolver(client kubernetes.Interface) *AuthResolver {
	return &AuthResolver{k8sClient: client}
}

// dockerConfigJSON mirrors the ~/.docker/config.json schema.
type dockerConfigJSON struct {
	Auths map[string]dockerAuthEntry `json:"auths"`
}

type dockerAuthEntry struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Auth     string `json:"auth"`
}

// ResolveAuth attempts to find registry credentials for the given host by
// scanning all dockerconfigjson secrets in the namespace. It tries exact match
// first, then scheme-prefix match, and finally suffix match.
func (r *AuthResolver) ResolveAuth(ctx context.Context, namespace, registryHost string) (*RegistryAuth, error) {
	if r.k8sClient == nil {
		return nil, nil
	}

	allAuths, err := r.loadAllDockerSecrets(ctx, namespace)
	if err != nil {
		return nil, fmt.Errorf("failed to load docker secrets from namespace %s: %w", namespace, err)
	}
	if len(allAuths) == 0 {
		return nil, nil
	}

	host := strings.TrimPrefix(strings.TrimPrefix(registryHost, "https://"), "http://")
	host = strings.TrimSuffix(host, "/")

	// 1. Exact match
	if entry, ok := allAuths[host]; ok {
		return resolveDockerAuthEntry(entry)
	}

	// 2. Scheme-prefix match (secret key may include https:// or http://)
	for key, entry := range allAuths {
		normalized := strings.TrimPrefix(strings.TrimPrefix(key, "https://"), "http://")
		normalized = strings.TrimSuffix(normalized, "/")
		if normalized == host {
			return resolveDockerAuthEntry(entry)
		}
	}

	// 3. Suffix match (e.g., host="harbor.example.com" matches key="*.example.com")
	for key, entry := range allAuths {
		normalized := strings.TrimPrefix(strings.TrimPrefix(key, "https://"), "http://")
		normalized = strings.TrimSuffix(normalized, "/")
		if strings.HasSuffix(host, normalized) || strings.HasSuffix(normalized, host) {
			return resolveDockerAuthEntry(entry)
		}
	}

	log.Debugf("AuthResolver: no credentials found for %s in namespace %s", registryHost, namespace)
	return nil, nil
}

// loadAllDockerSecrets reads all kubernetes.io/dockerconfigjson secrets in the
// namespace and returns a merged map of registry host -> auth entry.
func (r *AuthResolver) loadAllDockerSecrets(ctx context.Context, namespace string) (map[string]dockerAuthEntry, error) {
	secrets, err := r.k8sClient.CoreV1().Secrets(namespace).List(ctx, metav1.ListOptions{
		FieldSelector: "type=" + string(corev1.SecretTypeDockerConfigJson),
	})
	if err != nil {
		return nil, err
	}

	merged := make(map[string]dockerAuthEntry)
	for _, secret := range secrets.Items {
		data, ok := secret.Data[corev1.DockerConfigJsonKey]
		if !ok {
			continue
		}
		var cfg dockerConfigJSON
		if err := json.Unmarshal(data, &cfg); err != nil {
			log.Warnf("AuthResolver: failed to parse docker config from secret %s/%s: %v", namespace, secret.Name, err)
			continue
		}
		for host, entry := range cfg.Auths {
			merged[host] = entry
		}
	}
	return merged, nil
}

// resolveDockerAuthEntry extracts username/password from a Docker auth entry.
// It handles both explicit username/password fields and the base64-encoded auth field.
func resolveDockerAuthEntry(entry dockerAuthEntry) (*RegistryAuth, error) {
	if entry.Username != "" && entry.Password != "" {
		return &RegistryAuth{Username: entry.Username, Password: entry.Password}, nil
	}

	if entry.Auth != "" {
		decoded, err := base64.StdEncoding.DecodeString(entry.Auth)
		if err != nil {
			return nil, fmt.Errorf("failed to decode auth field: %w", err)
		}
		parts := strings.SplitN(string(decoded), ":", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid auth field format: expected username:password")
		}
		return &RegistryAuth{Username: parts[0], Password: parts[1]}, nil
	}

	return nil, fmt.Errorf("docker auth entry has neither username/password nor auth field")
}
