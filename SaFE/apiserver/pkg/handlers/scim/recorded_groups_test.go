/*
 * Copyright (C) 2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package scim

import (
	"context"
	"encoding/json"
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
	require.Nil(t, read(), "a deactivation forgets the groups")

	on := `{"Operations":[{"op":"replace","path":"active","value":true}]}`
	require.Equal(t, http.StatusOK, scimDo(engine, http.MethodPatch, "/scim/v2/Users/u-1", on, "secret").Code)
	require.Nil(t, read(), "a reactivation does not restore them: identity pushes them again")

	require.Equal(t, http.StatusOK, scimDo(engine, http.MethodPatch, "/scim/v2/Users/u-1", set, "secret").Code)
	require.Equal(t, []string{"admins", "other"}, read())

	clear := `{"op":"replace","path":"groups","value":[]}`
	require.Equal(t, http.StatusOK, scimDo(engine, http.MethodPatch, "/scim/v2/Users/u-1", clear, "secret").Code)
	require.Equal(t, []string{}, read(), "removing every group is recorded, so the grant ends")
}

// A deactivation that carries groups in the same request does not record them: a frozen user holds no grant.
func TestSCIMDeactivationInTheSamePatchRecordsNoGroups(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	engine, cli := scimEngine(t, newSCIMRecord("u-1", "alice"))
	both := `{"Operations":[{"op":"replace","path":"active","value":false},{"op":"replace","path":"groups","value":["admins"]}]}`
	require.Equal(t, http.StatusOK, scimDo(engine, http.MethodPatch, "/scim/v2/Users/u-1", both, "secret").Code)
	got := &v1.User{}
	require.NoError(t, cli.Get(context.Background(), client.ObjectKey{Name: "u-1"}, got))
	require.Nil(t, identity.RecordedGroups(got))
}

// A DELETE freezes the user and forgets the groups as a deactivation does.
func TestSCIMDeleteForgetsRecordedGroups(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	engine, cli := scimEngine(t, newSCIMRecord("u-1", "alice"))
	set := `{"op":"replace","path":"groups","value":["admins"]}`
	require.Equal(t, http.StatusOK, scimDo(engine, http.MethodPatch, "/scim/v2/Users/u-1", set, "secret").Code)
	require.Equal(t, http.StatusNoContent, scimDo(engine, http.MethodDelete, "/scim/v2/Users/u-1", "", "secret").Code)
	got := &v1.User{}
	require.NoError(t, cli.Get(context.Background(), client.ObjectKey{Name: "u-1"}, got))
	require.Nil(t, identity.RecordedGroups(got))
}

// The SCIM user reports the recorded groups as a list of names, so the identity's reconciliation sees a
// difference and pushes them again; with no record it reports an empty list.
func TestSCIMUserReportsTheRecordedGroups(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	engine, _ := scimEngine(t, newSCIMRecord("u-1", "alice"))
	groupsOf := func() []string {
		rec := scimDo(engine, http.MethodGet, "/scim/v2/Users/u-1", "", "secret")
		require.Equal(t, http.StatusOK, rec.Code)
		var body struct {
			Groups []string `json:"groups"`
		}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
		return body.Groups
	}
	require.Equal(t, []string{}, groupsOf(), "no record reports an empty list")

	set := `{"op":"replace","path":"groups","value":["other","admins"]}`
	require.Equal(t, http.StatusOK, scimDo(engine, http.MethodPatch, "/scim/v2/Users/u-1", set, "secret").Code)
	require.Equal(t, []string{"admins", "other"}, groupsOf())

	off := `{"Operations":[{"op":"replace","path":"active","value":false}]}`
	require.Equal(t, http.StatusOK, scimDo(engine, http.MethodPatch, "/scim/v2/Users/u-1", off, "secret").Code)
	require.Equal(t, []string{}, groupsOf(), "a deactivated user reports no groups")
}
