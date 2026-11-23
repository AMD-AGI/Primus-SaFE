/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package optimization

import (
	"context"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveClawBearer(t *testing.T) {
	h := &Handler{}
	prevLookup, prevService := clawPlatformKeyLookup, clawServiceAPIKey
	t.Cleanup(func() {
		clawPlatformKeyLookup = prevLookup
		clawServiceAPIKey = prevService
	})
	clawServiceAPIKey = func() string { return "file-claw-key" }

	cases := []struct {
		name   string
		mode   string
		auth   string
		userID string
		lookup func(context.Context, string, string, bool) (string, error)
		want   string
	}{
		{
			name:   "explicit ak- header wins",
			mode:   "external",
			auth:   "Bearer ak-from-header",
			userID: "u1",
			lookup: func(context.Context, string, string, bool) (string, error) {
				t.Fatal("lookup must not run when a header key is present")
				return "", nil
			},
			want: "ak-from-header",
		},
		{
			name:   "existing user key is used",
			mode:   "external",
			userID: "u1",
			lookup: func(_ context.Context, userID, _ string, create bool) (string, error) {
				assert.Equal(t, "u1", userID)
				assert.False(t, create)
				return "ak-user", nil
			},
			want: "ak-user",
		},
		{
			name:   "builtin creates a missing key",
			mode:   "builtin",
			userID: "u1",
			lookup: func(_ context.Context, _ string, _ string, create bool) (string, error) {
				assert.True(t, create)
				return "ak-created", nil
			},
			want: "ak-created",
		},
		{
			name:   "builtin falls back to the file when lookup returns empty",
			mode:   "builtin",
			userID: "u1",
			lookup: func(context.Context, string, string, bool) (string, error) { return "", nil },
			want:   "file-claw-key",
		},
		{
			name:   "external with no user key returns empty and never the file",
			mode:   "external",
			userID: "u1",
			lookup: func(_ context.Context, _ string, _ string, create bool) (string, error) {
				assert.False(t, create)
				return "", nil
			},
			want: "",
		},
		{
			name:   "no user id returns the file",
			mode:   "external",
			userID: "",
			lookup: func(context.Context, string, string, bool) (string, error) {
				t.Fatal("lookup must not run without a user id")
				return "", nil
			},
			want: "file-claw-key",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			prev := viper.Get("auth.mode")
			viper.Set("auth.mode", tc.mode)
			t.Cleanup(func() { viper.Set("auth.mode", prev) })
			clawPlatformKeyLookup = tc.lookup
			got := h.resolveClawBearer(context.Background(), tc.auth, tc.userID, "alice")
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestRequireClawBearerMessages(t *testing.T) {
	prev := viper.Get("auth.mode")
	t.Cleanup(func() { viper.Set("auth.mode", prev) })

	viper.Set("auth.mode", "builtin")
	err := requireClawBearer("")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "model_optimization.secret_path")

	viper.Set("auth.mode", "external")
	err = requireClawBearer("")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "this user has no platform key")
	assert.NotContains(t, err.Error(), "log in")

	require.NoError(t, requireClawBearer("ak-present"))
}
