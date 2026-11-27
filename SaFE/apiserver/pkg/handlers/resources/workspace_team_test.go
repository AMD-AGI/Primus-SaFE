/*
 * Copyright (C) 2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resources

import (
	"context"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/extauth"
)

// GET /teams refuses a restricted user, whether the record or the request's identity restricts it.
func TestListTeamsRefusesRestrictedUsers(t *testing.T) {
	gin.SetMode(gin.TestMode)
	frozen := genUserFixture("frozen", []v1.UserRole{v1.DefaultRole}, nil, nil)
	frozen.Spec.RestrictedType = v1.UserFrozen
	h := newChartTeamsHandler(t, interceptor.Funcs{}, frozen,
		genUserFixture("u1", []v1.UserRole{v1.DefaultRole}, nil, nil))
	ctx := context.Background()

	rsp := callTeamsBody(ctx, h.ListTeams, "frozen", "", "", "")
	require.Equal(t, http.StatusForbidden, rsp.Code, rsp.Body.String())

	restricted := extauth.WithOverlay(ctx, extauth.MapGroups("u1", nil, nil, true))
	rsp = callTeamsBody(restricted, h.ListTeams, "u1", "", "", "")
	require.Equal(t, http.StatusForbidden, rsp.Code, rsp.Body.String())

	rsp = callTeamsBody(ctx, h.ListTeams, "u1", "", "", "")
	require.Equal(t, http.StatusOK, rsp.Code, rsp.Body.String())
}
