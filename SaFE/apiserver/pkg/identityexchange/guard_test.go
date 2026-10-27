/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package identityexchange_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/identityexchange"
)

// Every capability an exchanged token must never have, each with the routes
// that carry it. A route missing from the guard turns one of these red.
func TestForbiddenCapabilities(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		path       string
		capability string
	}{
		// API keys: create, delete, list, current.
		{"create api key", http.MethodPost, "/api/v1/apikeys", identityexchange.CapabilityApiKey},
		{"delete api key", http.MethodDelete, "/api/v1/apikeys/7", identityexchange.CapabilityApiKey},
		{"list api keys", http.MethodGet, "/api/v1/apikeys", identityexchange.CapabilityApiKey},
		{"current api key", http.MethodGet, "/api/v1/apikeys/current", identityexchange.CapabilityApiKey},
		// Secrets: every access.
		{"list secrets", http.MethodGet, "/api/v1/secrets", identityexchange.CapabilitySecret},
		{"get secret", http.MethodGet, "/api/v1/secrets/my-secret", identityexchange.CapabilitySecret},
		{"create secret", http.MethodPost, "/api/v1/secrets", identityexchange.CapabilitySecret},
		{"patch secret", http.MethodPatch, "/api/v1/secrets/my-secret", identityexchange.CapabilitySecret},
		{"delete secret", http.MethodDelete, "/api/v1/secrets/my-secret", identityexchange.CapabilitySecret},
		// Users and roles: management, not reading one's own record.
		{"list users", http.MethodGet, "/api/v1/users", identityexchange.CapabilityUserManagement},
		{"patch user (roles, status)", http.MethodPatch, "/api/v1/users/u1", identityexchange.CapabilityUserManagement},
		{"delete user", http.MethodDelete, "/api/v1/users/u1", identityexchange.CapabilityUserManagement},
		{"update user settings", http.MethodPut, "/api/v1/users/u1/settings", identityexchange.CapabilityUserManagement},
		{"create user", http.MethodPost, "/api/v1/users", identityexchange.CapabilityUserManagement},
		// Standing credentials.
		{"add ssh public key", http.MethodPost, "/api/v1/publickeys", identityexchange.CapabilityCredentialMint},
		{"delete ssh public key", http.MethodDelete, "/api/v1/publickeys/3", identityexchange.CapabilityCredentialMint},
		// The exchange itself: bindings and clients.
		{"create binding", http.MethodPost, "/api/v1/identity-bindings", identityexchange.CapabilityExchangeManager},
		{"list bindings", http.MethodGet, "/api/v1/identity-bindings", identityexchange.CapabilityExchangeManager},
		{"delete binding", http.MethodDelete, "/api/v1/identity-bindings/3", identityexchange.CapabilityExchangeManager},
		{"create service client", http.MethodPost, "/api/v1/service-clients", identityexchange.CapabilityExchangeManager},
		{"list service clients", http.MethodGet, "/api/v1/service-clients", identityexchange.CapabilityExchangeManager},
		{"disable service client", http.MethodPost, "/api/v1/service-clients/c1/disable", identityexchange.CapabilityExchangeManager},
		// Spellings that must not slip past the match.
		{"upper case", http.MethodPost, "/API/V1/ApiKeys", identityexchange.CapabilityApiKey},
		{"trailing slash", http.MethodGet, "/api/v1/secrets/", identityexchange.CapabilitySecret},
		{"double slash", http.MethodPost, "/api/v1//apikeys", identityexchange.CapabilityApiKey},
		{"dot segments", http.MethodGet, "/api/v1/workloads/../secrets/x", identityexchange.CapabilitySecret},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			capability, forbidden := identityexchange.Forbidden(tt.method, tt.path)
			require.True(t, forbidden, "%s %s must be refused", tt.method, tt.path)
			require.Equal(t, tt.capability, capability)
		})
	}
}

func TestForbiddenLeavesOrdinaryAccessAlone(t *testing.T) {
	for _, tt := range []struct{ method, path string }{
		{http.MethodGet, "/api/v1/workloads"},
		{http.MethodPost, "/api/v1/workloads"},
		{http.MethodDelete, "/api/v1/workloads/w1"},
		{http.MethodGet, "/api/v1/users/self"},
		{http.MethodGet, "/api/v1/users/u1/settings"},
		{http.MethodGet, "/api/v1/publickeys"},
		{http.MethodGet, "/api/v1/workspaces"},
		{http.MethodPost, "/api/v1/login"},
		{http.MethodGet, "/lens/v1/secrets"}, // not the platform API
		{http.MethodGet, "/"},
	} {
		_, forbidden := identityexchange.Forbidden(tt.method, tt.path)
		require.False(t, forbidden, "%s %s", tt.method, tt.path)
	}
}

func TestScopeAllows(t *testing.T) {
	tests := []struct {
		scopes []string
		method string
		want   bool
	}{
		{[]string{"safe.api"}, http.MethodPost, true},
		{[]string{"safe.api"}, http.MethodGet, true},
		{[]string{"safe.read"}, http.MethodGet, true},
		{[]string{"safe.read"}, http.MethodPost, false},
		{[]string{"safe.read"}, http.MethodDelete, false},
		{[]string{"safe.read", "safe.api"}, http.MethodPatch, true},
		{[]string{"other.api"}, http.MethodGet, false},
		{nil, http.MethodGet, false},
	}
	for _, tt := range tests {
		require.Equal(t, tt.want, identityexchange.ScopeAllows(tt.scopes, tt.method), "%v %s", tt.scopes, tt.method)
	}
}
