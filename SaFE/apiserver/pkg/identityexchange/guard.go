/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package identityexchange

import (
	"net/http"
	"path"
	"strings"

	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
)

// Capabilities an exchanged token can never use, whatever its scope. They are
// the ones that would turn a short-lived delegation into a long-lived or wider
// one: minting credentials, reading credentials, changing who has which role,
// reading Secrets, and managing the exchange itself.
const (
	CapabilityApiKey          = "api-key"
	CapabilityUserManagement  = "user-management"
	CapabilitySecret          = "secret"
	CapabilityCredentialMint  = "credential-mint"
	CapabilityExchangeManager = "exchange-management"
)

// Forbidden reports whether a request must be refused for an exchanged token,
// and which capability it falls under. The check is on the route shape of the
// public API, so it also covers the handlers that call ParseToken directly.
func Forbidden(method, urlPath string) (capability string, forbidden bool) {
	segs := splitPath(urlPath)
	prefix := strings.Split(strings.Trim(common.PrimusRouterCustomRootPath, "/"), "/")
	if len(segs) <= len(prefix) {
		return "", false
	}
	for i, p := range prefix {
		if segs[i] != p {
			return "", false
		}
	}
	rest := segs[len(prefix):]
	read := method == http.MethodGet || method == http.MethodHead
	switch rest[0] {
	case "apikeys": // create, delete, list and the current key's details
		return CapabilityApiKey, true
	case "secrets":
		return CapabilitySecret, true
	case "users":
		// Reading one user (including "self") is allowed; listing users and every
		// write (roles, status, settings) are user management.
		if !read || len(rest) == 1 {
			return CapabilityUserManagement, true
		}
	case "publickeys":
		// An SSH public key is a standing credential for the user.
		if !read {
			return CapabilityCredentialMint, true
		}
	case "identity-bindings", "service-clients":
		return CapabilityExchangeManager, true
	}
	return "", false
}

func splitPath(p string) []string {
	p = strings.ToLower(path.Clean("/" + p))
	return strings.Split(strings.Trim(p, "/"), "/")
}

// ScopeAllows reports whether the token's scopes allow the request: safe.api
// allows everything the user may do (minus Forbidden), safe.read only reads.
func ScopeAllows(scopes []string, method string) bool {
	if contains(scopes, ScopeAPI) {
		return true
	}
	return contains(scopes, ScopeRead) && (method == http.MethodGet || method == http.MethodHead)
}
