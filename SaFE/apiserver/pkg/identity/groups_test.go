/*
 * Copyright (C) 2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package identity

import (
	"testing"

	"github.com/stretchr/testify/require"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

// RecordGroups stores the groups sorted and unique, reports a change only when the stored set
// differs, and RecordedGroups reads them back.
func TestRecordGroupsRoundTrip(t *testing.T) {
	user := &v1.User{}
	require.Nil(t, RecordedGroups(user))

	require.True(t, RecordGroups(user, []string{"b", "a", "b"}))
	require.Equal(t, []string{"a", "b"}, RecordedGroups(user))

	require.False(t, RecordGroups(user, []string{"a", "b"}), "the same set is not a change")
	require.True(t, RecordGroups(user, []string{}))
	require.Equal(t, []string{}, RecordedGroups(user), "an empty set is recorded as empty, not as absent")
	require.False(t, RecordGroups(user, nil), "nil and empty are the same set")
}

// A record that is absent or cannot be read yields no groups, never a guess.
func TestRecordedGroupsIgnoresMalformedValues(t *testing.T) {
	user := &v1.User{}
	v1.SetAnnotation(user, GroupsAnnotation, "not json")
	require.Nil(t, RecordedGroups(user))
	v1.SetAnnotation(user, GroupsAnnotation, `{"a":1}`)
	require.Nil(t, RecordedGroups(user))
	require.Nil(t, RecordedGroups(nil))
}
