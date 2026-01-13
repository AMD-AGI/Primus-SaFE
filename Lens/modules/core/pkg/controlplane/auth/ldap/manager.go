// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package ldap

import (
	"context"
	"fmt"
	"sync"

	cpdb "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/controlplane/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
)

// Manager manages LDAP providers loaded from database
type Manager struct {
	mu        sync.RWMutex
	providers map[string]*Provider // providerID -> Provider
	facade    cpdb.FacadeInterface
}

// NewManager creates a new LDAP manager
func NewManager() *Manager {
	return &Manager{
		providers: make(map[string]*Provider),
		facade:    cpdb.GetFacade(),
	}
}

// LoadProviders loads all enabled LDAP providers from database
func (m *Manager) LoadProviders(ctx context.Context) error {
	providers, err := m.facade.GetAuthProvider().ListEnabled(ctx)
	if err != nil {
		return fmt.Errorf("failed to list auth providers: %w", err)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Close existing providers
	for _, p := range m.providers {
		p.Close()
	}
	m.providers = make(map[string]*Provider)

	// Load LDAP providers
	for _, ap := range providers {
		if ap.Type != "ldap" {
			continue
		}

		provider, err := NewProviderFromMap(ap.Config)
		if err != nil {
			log.Warnf("Failed to create LDAP provider %s: %v", ap.Name, err)
			continue
		}

		m.providers[ap.ID] = provider
		log.Infof("Loaded LDAP provider: %s (%s)", ap.Name, ap.ID)
	}

	log.Infof("Loaded %d LDAP providers", len(m.providers))
	return nil
}

// GetProvider returns a specific LDAP provider by ID
func (m *Manager) GetProvider(id string) (*Provider, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	p, ok := m.providers[id]
	return p, ok
}

// GetFirstProvider returns the first available LDAP provider
func (m *Manager) GetFirstProvider() (*Provider, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, p := range m.providers {
		return p, true
	}
	return nil, false
}

// Authenticate authenticates using all available LDAP providers
// Returns on first successful authentication
func (m *Manager) Authenticate(ctx context.Context, creds *Credentials) (*AuthResult, string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if len(m.providers) == 0 {
		return nil, "", fmt.Errorf("no LDAP providers configured")
	}

	var lastError error
	for id, provider := range m.providers {
		result, err := provider.Authenticate(ctx, creds)
		if err != nil {
			lastError = err
			log.Debugf("LDAP provider %s authentication error: %v", id, err)
			continue
		}

		if result.Success {
			return result, id, nil
		}
	}

	if lastError != nil {
		return nil, "", lastError
	}

	return &AuthResult{
		Success:    false,
		FailReason: "authentication failed",
	}, "", nil
}

// TestProvider tests a specific provider
func (m *Manager) TestProvider(ctx context.Context, id string) (*TestResult, error) {
	m.mu.RLock()
	provider, ok := m.providers[id]
	m.mu.RUnlock()

	if !ok {
		// Try to load from database and test
		ap, err := m.facade.GetAuthProvider().GetByID(ctx, id)
		if err != nil {
			return nil, fmt.Errorf("provider not found: %w", err)
		}

		if ap.Type != "ldap" {
			return nil, fmt.Errorf("provider is not LDAP type")
		}

		provider, err = NewProviderFromMap(ap.Config)
		if err != nil {
			return &TestResult{
				Success: false,
				Message: fmt.Sprintf("failed to create provider: %v", err),
			}, nil
		}
		defer provider.Close()
	}

	return provider.TestConnection(ctx)
}

// Close closes all providers
func (m *Manager) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, p := range m.providers {
		p.Close()
	}
	m.providers = make(map[string]*Provider)
}

// Global manager instance
var (
	globalManager     *Manager
	globalManagerOnce sync.Once
)

// GetManager returns the global LDAP manager instance
func GetManager() *Manager {
	globalManagerOnce.Do(func() {
		globalManager = NewManager()
	})
	return globalManager
}

// InitManager initializes the global LDAP manager and loads providers
func InitManager(ctx context.Context) error {
	manager := GetManager()
	return manager.LoadProviders(ctx)
}
