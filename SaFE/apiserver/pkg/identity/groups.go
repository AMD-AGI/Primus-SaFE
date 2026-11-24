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

// RecordedGroups returns the groups recorded by the last SCIM push, sorted. It returns nil when
// nothing was recorded or the record cannot be read, and an empty slice when the push carried none.
func RecordedGroups(user *v1.User) []string {
	if user == nil {
		return nil
	}
	raw := v1.GetAnnotation(user, GroupsAnnotation)
	if raw == "" {
		return nil
	}
	var groups []string
	if err := json.Unmarshal([]byte(raw), &groups); err != nil || groups == nil {
		return nil
	}
	return groups
}

// ForgetGroups removes the recorded groups and reports whether there were any.
func ForgetGroups(user *v1.User) bool {
	return v1.RemoveAnnotation(user, GroupsAnnotation)
}

// RecordGroups stores groups on the user, sorted and without duplicates, and reports whether the
// recorded set changed.
func RecordGroups(user *v1.User, groups []string) bool {
	next := append([]string{}, groups...)
	sort.Strings(next)
	next = slices.Compact(next)
	if current := RecordedGroups(user); current != nil && slices.Equal(current, next) {
		return false
	}
	raw, err := json.Marshal(next)
	if err != nil {
		return false
	}
	return v1.SetAnnotation(user, GroupsAnnotation, string(raw))
}
