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

// CapabilityInteractive is the capability of a request that opens an interactive or
// upgraded session: a shell, exec, attach or port-forward into a pod, or any connection
// the server upgrades (WebSocket, SPDY). Such a session runs commands and moves data in
// both directions over one GET, so it is not a read whatever its HTTP method says.
const CapabilityInteractive = "interactive-session"

// Scopes of a delegated credential: safe.api allows what the user may do (minus
// Forbidden), safe.read only reads, and a read is judged by what the request can do,
// not by its HTTP method (see ReadOnly).
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

// interactiveSegments are the last path segments of the routes that open an
// interactive session on a pod or a node.
var interactiveSegments = map[string]bool{
	"webshell": true, "shell": true, "terminal": true,
	"exec": true, "attach": true, "portforward": true, "port-forward": true,
}

// Interactive reports whether the request opens an interactive or upgraded session
// (CapabilityInteractive). It is true for any request that asks the server to switch
// protocols, whichever route it is sent to: a handler can only upgrade a connection
// when the client sent the upgrade headers, so a route added later is covered without
// being listed. It is also true for the route shapes of the known interactive routes,
// which pins their classification in the route-table tests and does not depend on the
// headers a client chooses to send.
func Interactive(r *http.Request) bool {
	return upgradeRequested(r.Header) || interactiveRoute(r.URL.Path)
}

// upgradeRequested reports whether the headers ask for a protocol switch (RFC 9110
// section 7.8): an Upgrade header, or the "upgrade" token in Connection.
func upgradeRequested(h http.Header) bool {
	if strings.TrimSpace(h.Get("Upgrade")) != "" {
		return true
	}
	for _, v := range h.Values("Connection") {
		for _, tok := range strings.Split(v, ",") {
			if strings.EqualFold(strings.TrimSpace(tok), "upgrade") {
				return true
			}
		}
	}
	return false
}

// interactiveRoute reports whether a path has the shape of an interactive route of the
// public API: a resource and its name, then (possibly through more of them) a final
// segment that names a session, such as workloads/:name/pods/:pod/webshell.
func interactiveRoute(urlPath string) bool {
	segs := splitPath(urlPath)
	prefix := strings.Split(strings.Trim(common.PrimusRouterCustomRootPath, "/"), "/")
	if len(segs) < len(prefix)+3 {
		return false
	}
	for i, p := range prefix {
		if segs[i] != p {
			return false
		}
	}
	return interactiveSegments[segs[len(segs)-1]]
}

// ReadOnly reports whether a request only reads. The HTTP method is not enough: a GET
// that upgrades into a shell reads nothing. A request is read-only when its method is
// GET or HEAD and it is not Interactive.
func ReadOnly(r *http.Request) bool {
	return (r.Method == http.MethodGet || r.Method == http.MethodHead) && !Interactive(r)
}

// ScopeAllows reports whether the scopes of a delegated credential allow the
// request. A delegated credential without a recognised scope allows nothing, and
// safe.read allows only what ReadOnly accepts.
func ScopeAllows(scopes []string, r *http.Request) bool {
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
	return has(ScopeRead) && ReadOnly(r)
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

// InteractiveRouteGaps returns the routes ("METHOD path") of an engine whose handler
// upgrades the connection (as reported by upgrading, which the route-table test derives
// from the handlers' source) but whose path is not classified as interactive. A delegated
// credential is held to safe.api for such a route whatever the client sends; this pins
// the classification of the route itself, so that a new upgrade route is classified in
// Interactive or reviewed, instead of relying on request headers alone.
func InteractiveRouteGaps(routes []gin.RouteInfo, upgrading func(handler string) bool) []string {
	var gaps []string
	for _, r := range routes {
		if !upgrading(r.Handler) {
			continue
		}
		if !interactiveRoute(routeParam.ReplaceAllString(r.Path, "x")) {
			gaps = append(gaps, r.Method+" "+r.Path)
		}
	}
	return gaps
}
