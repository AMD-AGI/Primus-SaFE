/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package scim

import (
	"testing"

	"github.com/stretchr/testify/require"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

func TestFilterUsers(t *testing.T) {
	alice := v1.User{}
	alice.Name = "u-alice"
	alice.Annotations = map[string]string{v1.UserNameAnnotation: "alice@x"}
	bob := v1.User{}
	bob.Name = "u-bob"
	bob.Annotations = map[string]string{v1.UserNameAnnotation: "bob@x"}
	users := []v1.User{alice, bob}

	names := func(list []v1.User) []string {
		out := []string{}
		for i := range list {
			out = append(out, list[i].Name)
		}
		return out
	}

	got, err := filterUsers(users, "")
	require.NoError(t, err)
	require.Equal(t, []string{"u-alice", "u-bob"}, names(got))

	got, err = filterUsers(users, `userName eq "Bob@X"`)
	require.NoError(t, err)
	require.Equal(t, []string{"u-bob"}, names(got))

	got, err = filterUsers(users, `id eq "u-alice"`)
	require.NoError(t, err)
	require.Equal(t, []string{"u-alice"}, names(got))

	got, err = filterUsers(users, `externalId EQ "u-bob"`)
	require.NoError(t, err)
	require.Equal(t, []string{"u-bob"}, names(got))

	got, err = filterUsers(users, `userName eq "nobody@x"`)
	require.NoError(t, err)
	require.Empty(t, got)

	for _, bad := range []string{`userName co "bob"`, `emails eq "bob@x"`, `userName eq bob`, `userName eq "a" and id eq "b"`} {
		_, err = filterUsers(users, bad)
		require.Error(t, err, bad)
	}
}
