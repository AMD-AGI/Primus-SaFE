/*
 * Copyright (C) 2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package scim

import (
	"context"
	"net/http"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/identity"
)

// A groups patch records the groups the identity pushed, whatever the mappings grant from them,
// and a patch that carries no groups leaves the record as it was.
func TestSCIMPatchRecordsTheGroupsThePushCarried(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	viper.Set("auth.group_mappings", []map[string]any{{"group": "admins", "roles": []string{"system-admin"}}})
	engine, cli := scimEngine(t, newSCIMRecord("u-1", "alice"))
	got := &v1.User{}
	read := func() []string {
		require.NoError(t, cli.Get(context.Background(), client.ObjectKey{Name: "u-1"}, got))
		return identity.RecordedGroups(got)
	}
	require.Nil(t, read())

	set := `{"op":"replace","path":"groups","value":["other","admins"]}`
	require.Equal(t, http.StatusOK, scimDo(engine, http.MethodPatch, "/scim/v2/Users/u-1", set, "secret").Code)
	require.Equal(t, []string{"admins", "other"}, read())

	off := `{"Operations":[{"op":"replace","path":"active","value":false}]}`
	require.Equal(t, http.StatusOK, scimDo(engine, http.MethodPatch, "/scim/v2/Users/u-1", off, "secret").Code)
	require.Equal(t, []string{"admins", "other"}, read(), "a patch without groups keeps what was recorded")

	clear := `{"op":"replace","path":"groups","value":[]}`
	require.Equal(t, http.StatusOK, scimDo(engine, http.MethodPatch, "/scim/v2/Users/u-1", clear, "secret").Code)
	require.Equal(t, []string{}, read(), "removing every group is recorded, so the grant ends")
}
