/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package identityexchange

import (
	"net/http"
	"path"
	"regexp"
	"strings"

	"github.com/gin-gonic/gin"

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
	CapabilityCredentialRead  = "credential-read"
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
	case "llm-gateway":
		// The gateway binding holds the user's gateway key, which can be revealed,
		// replaced and removed there. Usage, budget and the proxy itself stay open.
		if len(rest) > 1 && rest[1] == "binding" {
			return CapabilityCredentialRead, true
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

// credentialWords are path fragments that suggest a route handles a credential.
var credentialWords = []string{"key", "secret", "credential", "token", "password", "reveal"}

// CredentialRouteGaps returns the routes ("METHOD path") of an engine whose path
// mentions a credential but which an exchanged token may call. It exists so that
// the route tables of the packages that register routes can pin the exclusion
// list in a test: a new route must be classified in Forbidden, or reviewed and
// listed in reviewed, instead of being allowed by default.
func CredentialRouteGaps(routes []gin.RouteInfo, reviewed map[string]bool) []string {
	var gaps []string
	for _, r := range routes {
		path := routeParam.ReplaceAllString(r.Path, "x")
		if _, forbidden := Forbidden(r.Method, path); forbidden || reviewed[r.Method+" "+r.Path] {
			continue
		}
		lower := strings.ToLower(r.Path)
		for _, w := range credentialWords {
			if strings.Contains(lower, w) {
				gaps = append(gaps, r.Method+" "+r.Path)
				break
			}
		}
	}
	return gaps
}

var routeParam = regexp.MustCompile(`:[A-Za-z]+`)
