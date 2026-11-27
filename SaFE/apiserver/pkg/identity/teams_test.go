/*
 * Copyright (C) 2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package identity

import (
	"testing"

	"github.com/stretchr/testify/require"
)

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
