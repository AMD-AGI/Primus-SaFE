// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package github

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"k8s.io/klog/v2"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// ClientManager manages GitHub clients with different tokens
type ClientManager struct {
	mu          sync.RWMutex
	clients     map[string]*Client // key: secretNamespace/secretName
	k8sClient   kubernetes.Interface
	tokenCache  map[string]*tokenCacheEntry
}

type tokenCacheEntry struct {
	token     string
	expiresAt time.Time
}

const (
	tokenCacheDuration = 10 * time.Minute
	defaultTokenKey    = "github_token"
	tokenPrefixLen     = 8
)

var (
	globalManager     *ClientManager
	globalManagerOnce sync.Once
)

// InitGlobalManager initializes the global ClientManager
func InitGlobalManager(k8sClient kubernetes.Interface) {
	globalManagerOnce.Do(func() {
		globalManager = NewClientManager(k8sClient)
	})
}

// GetGlobalManager returns the global ClientManager
func GetGlobalManager() *ClientManager {
	return globalManager
}

// NewClientManager creates a new ClientManager
func NewClientManager(k8sClient kubernetes.Interface) *ClientManager {
	return &ClientManager{
		clients:    make(map[string]*Client),
		k8sClient:  k8sClient,
		tokenCache: make(map[string]*tokenCacheEntry),
	}
}

// GetClientForSecret returns a GitHub client using the token from the specified secret.
// It caches clients for tokenCacheDuration and automatically re-reads the secret
// when the cache expires. If the cached token results in a 401 from GitHub,
// callers should use InvalidateCache to force a re-read on the next call.
func (m *ClientManager) GetClientForSecret(ctx context.Context, namespace, secretName string) (*Client, error) {
	key := fmt.Sprintf("%s/%s", namespace, secretName)

	// Check cache first
	m.mu.RLock()
	if entry, ok := m.tokenCache[key]; ok && time.Now().Before(entry.expiresAt) {
		if client, ok := m.clients[key]; ok {
			m.mu.RUnlock()
			return client, nil
		}
	}
	m.mu.RUnlock()

	// Fetch token from secret
	token, err := m.getTokenFromSecret(ctx, namespace, secretName)
	if err != nil {
		return nil, err
	}

	client := NewClient(token)

	valid, validateErr := client.ValidateToken(ctx)
	if validateErr != nil {
		klog.Warningf("GitHub token validation request failed for secret %s: %v (token prefix: %s)",
			key, validateErr, maskToken(token))
	} else if !valid {
		klog.Errorf("GitHub token from secret %s returned 401 Bad credentials (token prefix: %s). "+
			"The PAT may be expired, revoked, or lack required scopes. "+
			"Please rotate the token in the Kubernetes secret and restart the affected pods.",
			key, maskToken(token))
		return nil, fmt.Errorf("GitHub token from secret %s is invalid (401 Bad credentials): "+
			"the PAT may be expired, revoked, or lack required scopes", key)
	}

	// Create and cache client
	m.mu.Lock()
	defer m.mu.Unlock()

	m.clients[key] = client
	m.tokenCache[key] = &tokenCacheEntry{
		token:     token,
		expiresAt: time.Now().Add(tokenCacheDuration),
	}

	return client, nil
}

// getTokenFromSecret fetches the GitHub token from a Kubernetes secret
func (m *ClientManager) getTokenFromSecret(ctx context.Context, namespace, secretName string) (string, error) {
	if m.k8sClient == nil {
		return "", fmt.Errorf("k8s client not initialized")
	}

	secret, err := m.k8sClient.CoreV1().Secrets(namespace).Get(ctx, secretName, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to get secret %s/%s: %w", namespace, secretName, err)
	}

	tokenKeys := []string{"github_token", "token", "GITHUB_TOKEN"}
	for _, key := range tokenKeys {
		if token, ok := secret.Data[key]; ok {
			trimmed := strings.TrimSpace(string(token))
			if trimmed == "" {
				klog.Warningf("GitHub token key %q in secret %s/%s is present but empty",
					key, namespace, secretName)
				continue
			}
			return trimmed, nil
		}
	}

	availableKeys := make([]string, 0, len(secret.Data))
	for k := range secret.Data {
		availableKeys = append(availableKeys, k)
	}
	return "", fmt.Errorf("no GitHub token found in secret %s/%s (available keys: %v, expected one of: %v)",
		namespace, secretName, availableKeys, tokenKeys)
}

// maskToken returns a safe-to-log prefix of the token for diagnostics.
func maskToken(token string) string {
	if len(token) <= tokenPrefixLen {
		return "***"
	}
	return token[:tokenPrefixLen] + "***"
}

// GetTokenForSecret returns the GitHub token from a Kubernetes secret.
// This is useful when external components need the raw token (e.g. for git clone).
func (m *ClientManager) GetTokenForSecret(ctx context.Context, namespace, secretName string) (string, error) {
	return m.getTokenFromSecret(ctx, namespace, secretName)
}

// InvalidateCache removes a client from the cache
func (m *ClientManager) InvalidateCache(namespace, secretName string) {
	key := fmt.Sprintf("%s/%s", namespace, secretName)

	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.clients, key)
	delete(m.tokenCache, key)
}

// ClearCache clears all cached clients
func (m *ClientManager) ClearCache() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.clients = make(map[string]*Client)
	m.tokenCache = make(map[string]*tokenCacheEntry)
}

