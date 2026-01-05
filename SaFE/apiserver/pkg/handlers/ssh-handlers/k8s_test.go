/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package ssh_handlers

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestParseUserInfoFunc tests the ParseUserInfo function from ssh.go
func TestParseUserInfoFunc(t *testing.T) {
	tests := []struct {
		name      string
		user      string
		expectOk  bool
		expectPod string
	}{
		{
			name:      "valid user string",
			user:      "root.pod-1.container.bash.namespace",
			expectOk:  true,
			expectPod: "pod-1",
		},
		{
			name:     "invalid user string",
			user:     "invalid",
			expectOk: false,
		},
		{
			name:     "empty string",
			user:     "",
			expectOk: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info, ok := ParseUserInfo(tt.user)
			assert.Equal(t, tt.expectOk, ok)
			if tt.expectOk {
				assert.NotNil(t, info)
				if tt.expectPod != "" {
					assert.Equal(t, tt.expectPod, info.Pod)
				}
			}
		})
	}
}

// TestIsShellCommand tests the IsShellCommand function
func TestIsShellCommand(t *testing.T) {
	tests := []struct {
		name     string
		cmd      string
		expected bool
	}{
		{
			name:     "sh command",
			cmd:      "sh",
			expected: true,
		},
		{
			name:     "bash command",
			cmd:      "bash",
			expected: true,
		},
		{
			name:     "zsh command",
			cmd:      "zsh",
			expected: true,
		},
		{
			name:     "ash command",
			cmd:      "ash",
			expected: true,
		},
		{
			name:     "ksh command",
			cmd:      "ksh",
			expected: true,
		},
		{
			name:     "csh command",
			cmd:      "csh",
			expected: true,
		},
		{
			name:     "tcsh command",
			cmd:      "tcsh",
			expected: true,
		},
		{
			name:     "bash with login",
			cmd:      "bash --login -c bash",
			expected: true,
		},
		{
			name:     "non-shell command",
			cmd:      "python",
			expected: false,
		},
		{
			name:     "empty command",
			cmd:      "",
			expected: false,
		},
		{
			name:     "bash with different flags",
			cmd:      "bash -c",
			expected: false,
		},
		{
			name:     "partial match should fail",
			cmd:      "bash  ",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsShellCommand(tt.cmd)
			assert.Equal(t, tt.expected, result, "IsShellCommand(%q) should return %v", tt.cmd, tt.expected)
		})
	}
}
