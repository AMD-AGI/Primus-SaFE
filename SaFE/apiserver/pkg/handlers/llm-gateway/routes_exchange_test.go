/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package llmgateway

import (
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/identityexchange"
)

// The gateway binding holds the user's gateway key (reveal, replace, remove), so
// an exchanged token must be refused for every route that touches it. This pins
// the exclusion list against the real route table, and fails on a new route that
// mentions a credential until it is classified.
func TestExchangedTokenExclusionsCoverGatewayRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	e := gin.New()
	InitRoutes(e, &Handler{})

	reviewed := map[string]bool{}
	require.Empty(t, identityexchange.CredentialRouteGaps(e.Routes(), reviewed))

	seen := 0
	for _, r := range e.Routes() {
		if len(r.Path) >= len("/api/v1/llm-gateway/binding") && r.Path[:len("/api/v1/llm-gateway/binding")] == "/api/v1/llm-gateway/binding" {
			_, forbidden := identityexchange.Forbidden(r.Method, r.Path)
			require.True(t, forbidden, "%s %s must be refused for exchanged tokens", r.Method, r.Path)
			seen++
		}
	}
	require.Equal(t, 5, seen, "binding routes (create, update, delete, get, reveal) not found: the test would pass vacuously")
}
