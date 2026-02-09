// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package github

import (
	"context"
	"fmt"
	"sync"
	"time"

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

// GetClientForSecret returns a GitHub client using the token from the specified secret
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

	// Create and cache client
	m.mu.Lock()
	defer m.mu.Unlock()

	client := NewClient(token)
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

	// Try common token keys
	tokenKeys := []string{"github_token", "token", "GITHUB_TOKEN"}
	for _, key := range tokenKeys {
		if token, ok := secret.Data[key]; ok {
			return string(token), nil
		}
	}

	return "", fmt.Errorf("no GitHub token found in secret %s/%s", namespace, secretName)
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

