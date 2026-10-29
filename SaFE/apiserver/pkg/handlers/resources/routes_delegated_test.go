/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resources

import (
	"net/http"
	"regexp"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/extauth"
)

var delegatedRouteParam = regexp.MustCompile(`:[A-Za-z]+`)

// The delegated-token exclusion list is pinned against the real route table:
// every route that touches API keys, Secrets, user management or SSH keys must be
// refused for a delegated token, and a new route whose path mentions a credential
// has to be classified on purpose.
func TestDelegatedTokenExclusionsCoverTheRouteTable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	e := gin.New()
	InitCustomRouters(e, &Handler{})

	// Routes whose path contains a credential word but which are allowed on purpose,
	// reviewed one by one.
	reviewed := map[string]bool{
		"GET /api/v1/publickeys": true, // lists the user's SSH public keys; public material only
	}
	require.Empty(t, extauth.CredentialRouteGaps(e.Routes(), reviewed),
		"a route mentions a credential but is allowed for delegated tokens: classify it in extauth.Forbidden or review it here")

	seen := map[string]int{}
	for _, r := range e.Routes() {
		path := delegatedRouteParam.ReplaceAllString(r.Path, "x")
		segs := strings.Split(strings.Trim(path, "/"), "/")
		if len(segs) < 3 || segs[0] != "api" || segs[1] != "v1" {
			continue
		}
		res, rest := segs[2], segs[3:]
		_, forbidden := extauth.Forbidden(r.Method, path)
		switch {
		case res == "apikeys" || res == "secrets":
			require.True(t, forbidden, "%s %s must be refused for delegated tokens", r.Method, r.Path)
			seen[res]++
		case res == "users":
			// The path parameter stands for any user but the caller ("self" is spelled
			// out in the route table when it is its own route), so every users route
			// is refused here, and the caller's own record is checked below.
			if len(rest) > 0 && rest[0] == "self" {
				continue
			}
			require.True(t, forbidden, "%s %s must be refused for delegated tokens", r.Method, r.Path)
			seen[res]++
		case res == "publickeys" && r.Method != http.MethodGet:
			require.True(t, forbidden, "%s %s must be refused for delegated tokens", r.Method, r.Path)
			seen[res]++
		}
	}
	_, forbidden := extauth.Forbidden(http.MethodGet, "/api/v1/users/self")
	require.False(t, forbidden, "a delegated token reads the user it acts for")
	for _, res := range []string{"apikeys", "secrets", "users", "publickeys"} {
		require.Positive(t, seen[res], "no route found for %s: the test would pass vacuously", res)
	}
}

// There is no token verification endpoint: who a credential belongs to is decided by the
// apiserver's own authentication, or by the external authenticators, never handed to a
// caller that holds a shared token.
func TestNoTokenVerificationRoute(t *testing.T) {
	gin.SetMode(gin.TestMode)
	e := gin.New()
	InitCustomRouters(e, &Handler{})
	require.NotEmpty(t, e.Routes())
	for _, r := range e.Routes() {
		require.False(t, strings.HasPrefix(r.Path, "/api/v1/auth/"), "%s %s", r.Method, r.Path)
	}
}
