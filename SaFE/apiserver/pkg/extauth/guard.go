/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package extauth

import (
	"net/http"
	"path"
	"regexp"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
)

// Capabilities a delegated credential (one that carries an actor) can never use,
// whatever its scope and whoever issued it. They are the ones that would turn a
// short-lived delegation into a long-lived or wider one: minting credentials,
// reading credentials, changing who has which role, and reading Secrets. The
// check is SaFE's own, on SaFE's routes, so it does not depend on the issuer.
const (
	CapabilityApiKey         = "api-key"
	CapabilityUserManagement = "user-management"
	CapabilitySecret         = "secret"
	CapabilityCredentialMint = "credential-mint"
	CapabilityCredentialRead = "credential-read"
	CapabilityPlatformKey    = "platform-key"
)

// Scopes of a delegated credential: safe.api allows what the user may do (minus
// Forbidden), safe.read only reads.
const (
	ScopeAPI  = "safe.api"
	ScopeRead = "safe.read"
)

// Forbidden reports whether a request must be refused for a delegated credential,
// and which capability it falls under. It matches the route shape of the public
// API, so it covers the handlers that call ParseToken directly as well.
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
		// A delegated token acts as one user, so it may read that user ("self", and
		// what hangs off it, such as the workspaces). Every other user route, a named
		// user's record or settings, the user list and every write, is user management.
		if !read || len(rest) < 2 || rest[1] != "self" {
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
	case "auth":
		// The token verification endpoint can hand back a user's platform key.
		return CapabilityPlatformKey, true
	}
	return "", false
}

func splitPath(p string) []string {
	p = strings.ToLower(path.Clean("/" + p))
	return strings.Split(strings.Trim(p, "/"), "/")
}

// ScopeAllows reports whether the scopes of a delegated credential allow the
// request. A delegated credential without a recognised scope allows nothing.
func ScopeAllows(scopes []string, method string) bool {
	has := func(s string) bool {
		for _, x := range scopes {
			if x == s {
				return true
			}
		}
		return false
	}
	if has(ScopeAPI) {
		return true
	}
	return has(ScopeRead) && (method == http.MethodGet || method == http.MethodHead)
}

// credentialWords are path fragments that suggest a route handles a credential.
var credentialWords = []string{"key", "secret", "credential", "token", "password", "reveal"}

var routeParam = regexp.MustCompile(`:[A-Za-z]+`)

// CredentialRouteGaps returns the routes ("METHOD path") of an engine whose path
// mentions a credential but which a delegated credential may call. It exists so that
// the route tables of the packages that register routes can pin the exclusion list
// in a test: a new route must be classified in Forbidden, or reviewed and listed in
// reviewed, instead of being allowed by default.
func CredentialRouteGaps(routes []gin.RouteInfo, reviewed map[string]bool) []string {
	var gaps []string
	for _, r := range routes {
		p := routeParam.ReplaceAllString(r.Path, "x")
		if _, forbidden := Forbidden(r.Method, p); forbidden || reviewed[r.Method+" "+r.Path] {
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
