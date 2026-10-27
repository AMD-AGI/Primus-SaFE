/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package llmgateway

import (
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/extauth"
)

// The gateway binding holds the user's gateway key (reveal, replace, remove), so a
// delegated token must be refused for every route that touches it. This pins the
// exclusion list against the real route table, and fails on a new route that
// mentions a credential until it is classified.
func TestDelegatedTokenExclusionsCoverGatewayRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	e := gin.New()
	InitRoutes(e, &Handler{})

	require.Empty(t, extauth.CredentialRouteGaps(e.Routes(), map[string]bool{}))

	seen := 0
	for _, r := range e.Routes() {
		if strings.HasPrefix(r.Path, "/api/v1/llm-gateway/binding") {
			_, forbidden := extauth.Forbidden(r.Method, r.Path)
			require.True(t, forbidden, "%s %s must be refused for delegated tokens", r.Method, r.Path)
			seen++
		}
	}
	require.Positive(t, seen, "binding routes not found: the test would pass vacuously")
}
