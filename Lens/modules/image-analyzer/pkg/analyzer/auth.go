// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package analyzer

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

// RegistryAuth holds credentials for a single container registry
type RegistryAuth struct {
	Username string
	Password string
}

// AuthResolver reads imagePullSecrets from Kubernetes and resolves
// registry credentials for OCI API calls.
type AuthResolver struct {
	k8sClient kubernetes.Interface
}

// NewAuthResolver creates an AuthResolver using the provided K8s clientset
func NewAuthResolver(client kubernetes.Interface) *AuthResolver {
	return &AuthResolver{k8sClient: client}
}

// dockerConfigJSON represents the .dockerconfigjson structure
type dockerConfigJSON struct {
	Auths map[string]dockerAuthEntry `json:"auths"`
}

type dockerAuthEntry struct {
	Auth     string `json:"auth"`
	Username string `json:"username"`
	Password string `json:"password"`
}

// ResolveAuth finds credentials for the given registry host by reading all
// docker-registry secrets in the specified namespace.
func (r *AuthResolver) ResolveAuth(ctx context.Context, namespace, registryHost string) (*RegistryAuth, error) {
	if r.k8sClient == nil {
		return nil, nil
	}

	authMap, err := r.loadAllDockerSecrets(ctx, namespace)
	if err != nil {
		return nil, fmt.Errorf("failed to load docker secrets from namespace %s: %w", namespace, err)
	}

	// Try exact match first
	if auth, ok := authMap[registryHost]; ok {
		return auth, nil
	}

	// Try matching with scheme prefix
	for host, auth := range authMap {
		cleanHost := strings.TrimPrefix(strings.TrimPrefix(host, "https://"), "http://")
		if cleanHost == registryHost {
			return auth, nil
		}
	}

	// Try suffix match (e.g., "harbor.example.com" matches "*.example.com")
	for host, auth := range authMap {
		cleanHost := strings.TrimPrefix(strings.TrimPrefix(host, "https://"), "http://")
		if strings.HasSuffix(registryHost, cleanHost) || strings.HasSuffix(cleanHost, registryHost) {
			return auth, nil
		}
	}

	log.Debugf("AuthResolver: no credentials found for %s in namespace %s, using anonymous", registryHost, namespace)
	return nil, nil
}

// loadAllDockerSecrets reads all kubernetes.io/dockerconfigjson secrets in the
// given namespace and merges them into a single registryHost->auth map.
func (r *AuthResolver) loadAllDockerSecrets(ctx context.Context, namespace string) (map[string]*RegistryAuth, error) {
	authMap := make(map[string]*RegistryAuth)

	secrets, err := r.k8sClient.CoreV1().Secrets(namespace).List(ctx, metav1.ListOptions{
		FieldSelector: "type=" + string(corev1.SecretTypeDockerConfigJson),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list docker secrets: %w", err)
	}

	for _, secret := range secrets.Items {
		data, ok := secret.Data[corev1.DockerConfigJsonKey]
		if !ok {
			continue
		}

		var cfg dockerConfigJSON
		if err := json.Unmarshal(data, &cfg); err != nil {
			log.Warnf("AuthResolver: failed to parse secret %s/%s: %v", namespace, secret.Name, err)
			continue
		}

		for host, entry := range cfg.Auths {
			auth := resolveAuthEntry(entry)
			if auth != nil {
				authMap[host] = auth
				log.Debugf("AuthResolver: loaded credentials for %s from secret %s", host, secret.Name)
			}
		}
	}

	return authMap, nil
}

// resolveAuthEntry extracts username/password from a docker auth entry.
// The entry can have explicit username/password fields or a base64-encoded "auth" field.
func resolveAuthEntry(entry dockerAuthEntry) *RegistryAuth {
	if entry.Username != "" && entry.Password != "" {
		return &RegistryAuth{
			Username: entry.Username,
			Password: entry.Password,
		}
	}

	if entry.Auth != "" {
		decoded, err := base64.StdEncoding.DecodeString(entry.Auth)
		if err != nil {
			return nil
		}
		parts := strings.SplitN(string(decoded), ":", 2)
		if len(parts) == 2 {
			return &RegistryAuth{
				Username: parts[0],
				Password: parts[1],
			}
		}
	}

	return nil
}
