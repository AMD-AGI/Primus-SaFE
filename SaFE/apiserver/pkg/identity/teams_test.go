/*
 * Copyright (C) 2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package identity

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

// useTeamsServer points the identity configuration at a server run by handler, signed in to as
// the introspection client svc-safe, with an empty team cache.
func useTeamsServer(t *testing.T, handler http.HandlerFunc) {
	t.Helper()
	resetIdentityConfig(t)
	ResetTeamsCache()
	t.Cleanup(ResetTeamsCache)
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	viper.Set("auth.scim.identity_url", srv.URL)
	viper.Set("auth.authenticators", []map[string]any{{
		"type": "introspection", "client_id": "svc-safe", "client_secret_file": writeFile(t, "client_secret", "s3cret\n"),
	}})
}

// The team calls sign in with HTTP Basic as the introspection client, ask for JSON, and read the
// paths of the identity service API.
func TestTeamCallsAuthenticateAsTheIntrospectionClient(t *testing.T) {
	var paths []string
	useTeamsServer(t, func(w http.ResponseWriter, r *http.Request) {
		id, secret, ok := r.BasicAuth()
		if !ok || id != "svc-safe" || secret != "s3cret" || r.Header.Get("Accept") != "application/json" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		paths = append(paths, r.URL.Path)
		switch r.URL.Path {
		case "/api/v1/teams":
			_, _ = w.Write([]byte(`{"items":[{"teamId":"t1","displayName":"Team 1","memberCount":2,"managers":["u1"]}]}`))
		case "/api/v1/teams/t1/members":
			_, _ = w.Write([]byte(`{"items":[{"userId":"u1","email":"u1@example.com","role":"manager"},{"userId":"u2","role":"member"}]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	ctx := context.Background()
	teams, err := ListTeams(ctx)
	require.NoError(t, err)
	require.Equal(t, map[string]TeamInfo{"t1": {Id: "t1", DisplayName: "Team 1", MemberCount: 2,
		Managers: []TeamMember{{UserId: "u1", Role: "manager"}}}}, teams)
	members, err := ListTeamMembers(ctx, "t1")
	require.NoError(t, err)
	require.Equal(t, []TeamMember{{UserId: "u1", Email: "u1@example.com", Role: "manager"}, {UserId: "u2", Role: "member"}}, members)
	_, err = ListTeamMembers(ctx, "gone")
	require.True(t, apierrors.IsNotFound(err), "%v", err)
	_, err = ListTeamMembers(ctx, "")
	require.Error(t, err)
	require.Equal(t, []string{"/api/v1/teams", "/api/v1/teams/t1/members", "/api/v1/teams/gone/members"}, paths)
}

// An answer is reused for its cache time and a failure for its shorter one, per path.
func TestTeamCallsAreCached(t *testing.T) {
	var calls, failing atomic.Int32
	useTeamsServer(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if failing.Load() == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte(`{"items":[]}`))
	})
	ctx := context.Background()
	for range 3 {
		_, err := ListTeams(ctx)
		require.NoError(t, err)
	}
	require.EqualValues(t, 1, calls.Load())
	_, err := ListTeamMembers(ctx, "t1")
	require.NoError(t, err)
	require.EqualValues(t, 2, calls.Load())

	ResetTeamsCache()
	failing.Store(1)
	for range 3 {
		_, err = ListTeams(ctx)
		var status apierrors.APIStatus
		require.ErrorAs(t, err, &status)
		require.Equal(t, int32(http.StatusServiceUnavailable), status.Status().Code)
	}
	require.EqualValues(t, 3, calls.Load())
}

// A refusal of the identity service or an answer of an unexpected shape is 502, a failure or an
// unreachable service 503, and a missing configuration 503.
func TestTeamCallErrors(t *testing.T) {
	for status, want := range map[int]int32{
		http.StatusUnauthorized:       http.StatusBadGateway,
		http.StatusForbidden:          http.StatusBadGateway,
		http.StatusBadGateway:         http.StatusServiceUnavailable,
		http.StatusServiceUnavailable: http.StatusServiceUnavailable,
	} {
		useTeamsServer(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(status) })
		_, err := ListTeams(context.Background())
		var apiStatus apierrors.APIStatus
		require.ErrorAs(t, err, &apiStatus, "%d", status)
		require.Equal(t, want, apiStatus.Status().Code, "%d", status)
	}

	useTeamsServer(t, func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`[]`)) })
	_, err := ListTeams(context.Background())
	var apiStatus apierrors.APIStatus
	require.ErrorAs(t, err, &apiStatus)
	require.Equal(t, int32(http.StatusBadGateway), apiStatus.Status().Code)

	resetIdentityConfig(t)
	ResetTeamsCache()
	_, err = ListTeams(context.Background())
	require.ErrorAs(t, err, &apiStatus)
	require.Equal(t, int32(http.StatusServiceUnavailable), apiStatus.Status().Code)
}

// The team list is read in the shape the identity service sends: an object with items, camel case
// fields, and the managers as user ids.
func TestParseTeams(t *testing.T) {
	got, err := parseTeams([]byte(`{"items":[
		{"teamId":"t1","displayName":"Team 1","description":"d","joinPolicy":"closed","memberCount":3,"managers":["u1"]},
		{"teamId":"t2","memberCount":0,"managers":[]},
		{"displayName":"no id"}]}`))
	require.NoError(t, err)
	require.Equal(t, map[string]TeamInfo{
		"t1": {Id: "t1", DisplayName: "Team 1", Description: "d", MemberCount: 3,
			Managers: []TeamMember{{UserId: "u1", Role: "manager"}}},
		"t2": {Id: "t2"},
	}, got)

	got, err = parseTeams([]byte(`{"items":[{"team_id":"t1","display_name":"T","member_count":2}]}`))
	require.NoError(t, err)
	require.Empty(t, got, "snake case fields are not read")

	for _, body := range []string{
		`[{"teamId":"t1"}]`,
		`{"teams":[{"teamId":"t1"}]}`,
		`{}`,
		`{"items":[{"teamId":"t1","managers":[{"userId":"u1"}]}]}`,
		`not json`,
	} {
		_, err = parseTeams([]byte(body))
		require.Error(t, err, body)
	}
}

// The member list is read in the shape the identity service sends: an object with items of member
// objects with camel case fields.
func TestParseTeamMembers(t *testing.T) {
	got, err := parseTeamMembers([]byte(`{"items":[
		{"userId":"u1","email":"u1@example.com","name":"U1","preferredName":"U","role":"manager","source":"manual"},
		{"userId":"","email":""}]}`))
	require.NoError(t, err)
	require.Equal(t, []TeamMember{{UserId: "u1", Email: "u1@example.com", Name: "U1", PreferredName: "U", Role: "manager"}}, got)

	got, err = parseTeamMembers([]byte(`{"items":[{"user_id":"u1","preferred_name":"U"}]}`))
	require.NoError(t, err)
	require.Empty(t, got, "snake case fields are not read")

	for _, body := range []string{
		`[{"userId":"u1"}]`,
		`{"members":[{"userId":"u1"}]}`,
		`{"items":["u1"]}`,
		`{}`,
	} {
		_, err = parseTeamMembers([]byte(body))
		require.Error(t, err, body)
	}
}
