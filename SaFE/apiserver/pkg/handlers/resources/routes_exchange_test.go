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

	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/identityexchange"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/identityexchange/fake"
)

var routeParam = regexp.MustCompile(`:[A-Za-z]+`)

// The exchanged-token exclusion list is pinned against the real route table:
// every route that touches API keys, Secrets, user management, SSH keys or the
// exchange itself must be refused for an exchanged token, and a new route whose
// path mentions a credential has to be classified on purpose.
func TestExchangedTokenExclusionsCoverTheRouteTable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := newTestExchangeService(t)
	identityexchange.SetDefault(svc)
	t.Cleanup(func() { identityexchange.SetDefault(nil) })

	e := gin.New()
	InitCustomRouters(e, &Handler{})

	// Routes whose path contains a credential word but which are allowed on purpose,
	// reviewed one by one.
	reviewed := map[string]bool{
		"GET /api/v1/publickeys": true, // lists the user's SSH public keys; public material only
	}
	require.Empty(t, identityexchange.CredentialRouteGaps(e.Routes(), reviewed),
		"a route mentions a credential but is allowed for exchanged tokens: classify it in identityexchange.Forbidden or review it here")

	seen := map[string]int{}
	for _, r := range e.Routes() {
		path := routeParam.ReplaceAllString(r.Path, "x")
		segs := strings.Split(strings.Trim(path, "/"), "/")
		if len(segs) < 3 || segs[0] != "api" || segs[1] != "v1" {
			continue
		}
		res, rest := segs[2], segs[3:]
		_, forbidden := identityexchange.Forbidden(r.Method, path)
		switch {
		case res == "apikeys" || res == "secrets" || res == "identity-bindings" || res == "service-clients":
			require.True(t, forbidden, "%s %s must be refused for exchanged tokens", r.Method, r.Path)
			seen[res]++
		case res == "users" && (r.Method != http.MethodGet || len(rest) == 0):
			require.True(t, forbidden, "%s %s must be refused for exchanged tokens", r.Method, r.Path)
			seen[res]++
		case res == "publickeys" && r.Method != http.MethodGet:
			require.True(t, forbidden, "%s %s must be refused for exchanged tokens", r.Method, r.Path)
			seen[res]++
		}
	}
	for _, res := range []string{"apikeys", "secrets", "identity-bindings", "service-clients", "users", "publickeys"} {
		require.Positive(t, seen[res], "no route found for %s: the test would pass vacuously", res)
	}
}

func newTestExchangeService(t *testing.T) *identityexchange.Service {
	t.Helper()
	pemBytes, err := identityexchange.GenerateSigningKey("")
	require.NoError(t, err)
	keys, err := identityexchange.NewKeySet(pemBytes, nil)
	require.NoError(t, err)
	svc, err := identityexchange.NewService(identityexchange.Config{Issuer: "https://safe.example.test", Audience: "safe-apiserver"},
		keys, fake.NewStore(), fake.NewUsers())
	require.NoError(t, err)
	return svc
}
