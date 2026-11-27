/*
 * Copyright (C) 2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package identity

import (
	"encoding/json"
	"slices"
	"sort"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

// GroupsAnnotation holds the platform groups the identity service last pushed to the user by SCIM,
// as a JSON array of names. It is the only record of an identity-vouched grant that a credential
// the apiserver authenticates itself, such as an SSH public key, can rely on.
const GroupsAnnotation = "primus-safe.scim.groups"

// TeamsAnnotation holds the team ids the identity service last pushed to the user by SCIM, as a JSON
// array. Credentials the apiserver authenticates itself, such as an SSH public key, read the teams
// from it, and member views use it to find the users of the teams bound to a workspace.
const TeamsAnnotation = "primus-safe.scim.teams"

// RecordedGroups returns the groups recorded by the last SCIM push, sorted. It returns nil when
// nothing was recorded or the record cannot be read, and an empty slice when the push carried none.
func RecordedGroups(user *v1.User) []string {
	return recordedList(user, GroupsAnnotation)
}

// ForgetGroups removes the recorded groups and reports whether there were any.
func ForgetGroups(user *v1.User) bool {
	return v1.RemoveAnnotation(user, GroupsAnnotation)
}

// RecordGroups stores groups on the user, sorted and without duplicates, and reports whether the
// recorded set changed.
func RecordGroups(user *v1.User, groups []string) bool {
	return recordList(user, GroupsAnnotation, groups)
}

// RecordedTeams returns the team ids recorded by the last SCIM push, sorted. It returns nil when
// nothing was recorded or the record cannot be read, and an empty slice when the push carried none.
func RecordedTeams(user *v1.User) []string {
	return recordedList(user, TeamsAnnotation)
}

// ForgetTeams removes the recorded teams and reports whether there were any.
func ForgetTeams(user *v1.User) bool {
	return v1.RemoveAnnotation(user, TeamsAnnotation)
}

// RecordTeams stores team ids on the user, sorted and without duplicates, and reports whether the
// recorded set changed.
func RecordTeams(user *v1.User, teams []string) bool {
	return recordList(user, TeamsAnnotation, teams)
}

// recordedList reads the JSON array of names stored in the annotation key of the user.
func recordedList(user *v1.User, key string) []string {
	if user == nil {
		return nil
	}
	raw := v1.GetAnnotation(user, key)
	if raw == "" {
		return nil
	}
	var names []string
	if err := json.Unmarshal([]byte(raw), &names); err != nil || names == nil {
		return nil
	}
	return names
}

// recordList stores names in the annotation key of the user as a sorted JSON array without
// duplicates, and reports whether the stored set changed.
func recordList(user *v1.User, key string, names []string) bool {
	next := append([]string{}, names...)
	sort.Strings(next)
	next = slices.Compact(next)
	if current := recordedList(user, key); current != nil && slices.Equal(current, next) {
		return false
	}
	raw, err := json.Marshal(next)
	if err != nil {
		return false
	}
	return v1.SetAnnotation(user, key, string(raw))
}
