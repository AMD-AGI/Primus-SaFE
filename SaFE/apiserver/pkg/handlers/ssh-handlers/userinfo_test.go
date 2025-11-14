/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package ssh_handlers

import (
	"testing"

	"github.com/alexflint/go-restructure"
	"github.com/stretchr/testify/assert"
)

// TestParseUserInfo tests parsing of SSH user information in different scenarios
func TestParseUserInfo(t *testing.T) {
	tests := []struct {
		name     string
		userStr  string
		wantOk   bool
		wantUser string
		wantPod  string
		wantCont string
		wantCMD  string
		wantNS   string
	}{
		{
			name:     "valid standard format",
			userStr:  "root.primus-test-master-0.main.bash.primus-safe-dev",
			wantOk:   true,
			wantUser: "root",
			wantPod:  "primus-test-master-0",
			wantCont: "main",
			wantCMD:  "bash",
			wantNS:   "primus-safe-dev",
		},
		{
			name:     "user with dash",
			userStr:  "app-user.pod-name-123.container-1.sh.default",
			wantOk:   true,
			wantUser: "app-user",
			wantPod:  "pod-name-123",
			wantCont: "container-1",
			wantCMD:  "sh",
			wantNS:   "default",
		},
		{
			name:     "user with underscore",
			userStr:  "test_user.my_pod.my_container.bash.my_namespace",
			wantOk:   true,
			wantUser: "test_user",
			wantPod:  "my_pod",
			wantCont: "my_container",
			wantCMD:  "bash",
			wantNS:   "my_namespace",
		},
		{
			name:     "numeric pod name",
			userStr:  "admin.pod123.cont456.zsh.ns789",
			wantOk:   true,
			wantUser: "admin",
			wantPod:  "pod123",
			wantCont: "cont456",
			wantCMD:  "zsh",
			wantNS:   "ns789",
		},
		{
			name:     "container with dots",
			userStr:  "user.pod-1.container.v1.0.bash.namespace",
			wantOk:   true,
			wantUser: "user",
			wantPod:  "pod-1",
			wantCont: "container.v1.0",
			wantCMD:  "bash",
			wantNS:   "namespace",
		},
		{
			name:     "kube-system namespace",
			userStr:  "root.system-pod.sidecar.sh.kube-system",
			wantOk:   true,
			wantUser: "root",
			wantPod:  "system-pod",
			wantCont: "sidecar",
			wantCMD:  "sh",
			wantNS:   "kube-system",
		},
		{
			name:    "invalid - missing field",
			userStr: "root.pod.container.bash",
			wantOk:  false,
		},
		{
			name:    "invalid - starts with dash",
			userStr: "-user.pod.container.bash.namespace",
			wantOk:  false,
		},
		{
			name:    "invalid - empty string",
			userStr: "",
			wantOk:  false,
		},
		{
			name:    "invalid - special characters in user",
			userStr: "user@host.pod.container.bash.namespace",
			wantOk:  false,
		},
		{
			name:    "invalid - space in field",
			userStr: "root.my pod.container.bash.namespace",
			wantOk:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := &UserInfo{}
			ok, err := restructure.Find(info, tt.userStr)

			assert.Equal(t, tt.wantOk, ok)

			if tt.wantOk {
				assert.NoError(t, err)
				assert.Equal(t, tt.wantUser, info.User)
				assert.Equal(t, tt.wantPod, info.Pod)
				assert.Equal(t, tt.wantCont, info.Container)
				assert.Equal(t, tt.wantCMD, info.CMD)
				assert.Equal(t, tt.wantNS, info.Namespace)
			}
		})
	}
}

// TestUserInfoRegexpPatterns tests specific regexp patterns in UserInfo
func TestUserInfoRegexpPatterns(t *testing.T) {
	tests := []struct {
		name    string
		field   string
		value   string
		wantErr bool
	}{
		{
			name:    "alphanumeric user",
			field:   "user",
			value:   "user123",
			wantErr: false,
		},
		{
			name:    "user with underscore",
			field:   "user",
			value:   "test_user",
			wantErr: false,
		},
		{
			name:    "user with dash",
			field:   "user",
			value:   "app-user",
			wantErr: false,
		},
		{
			name:    "numeric only",
			field:   "pod",
			value:   "123456",
			wantErr: false,
		},
		{
			name:    "container with multiple dots",
			field:   "container",
			value:   "app.v1.2.3",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Build a valid user string with the test value in the appropriate position
			var userStr string
			switch tt.field {
			case "user":
				userStr = tt.value + ".pod.container.bash.namespace"
			case "pod":
				userStr = "user." + tt.value + ".container.bash.namespace"
			case "container":
				userStr = "user.pod." + tt.value + ".bash.namespace"
			case "cmd":
				userStr = "user.pod.container." + tt.value + ".namespace"
			case "namespace":
				userStr = "user.pod.container.bash." + tt.value
			}

			info := &UserInfo{}
			ok, _ := restructure.Find(info, userStr)

			if tt.wantErr {
				assert.False(t, ok, "Expected parsing to fail for %s='%s'", tt.field, tt.value)
			} else {
				assert.True(t, ok, "Expected parsing to succeed for %s='%s'", tt.field, tt.value)
			}
		})
	}
}

// TestWebShellRequestValidation tests WebShellRequest structure
func TestWebShellRequestValidation(t *testing.T) {
	tests := []struct {
		name     string
		request  WebShellRequest
		validate func(*testing.T, WebShellRequest)
	}{
		{
			name: "complete request",
			request: WebShellRequest{
				NameSpace: "default",
				Rows:      "24",
				Cols:      "80",
				Container: "app",
				CMD:       "bash",
			},
			validate: func(t *testing.T, req WebShellRequest) {
				assert.Equal(t, "default", req.NameSpace)
				assert.Equal(t, "24", req.Rows)
				assert.Equal(t, "80", req.Cols)
				assert.Equal(t, "app", req.Container)
				assert.Equal(t, "bash", req.CMD)
			},
		},
		{
			name: "minimal request",
			request: WebShellRequest{
				NameSpace: "kube-system",
			},
			validate: func(t *testing.T, req WebShellRequest) {
				assert.Equal(t, "kube-system", req.NameSpace)
				assert.Empty(t, req.Rows)
				assert.Empty(t, req.Cols)
			},
		},
		{
			name: "custom terminal size",
			request: WebShellRequest{
				NameSpace: "my-namespace",
				Rows:      "50",
				Cols:      "120",
				Container: "main",
				CMD:       "sh",
			},
			validate: func(t *testing.T, req WebShellRequest) {
				assert.Equal(t, "50", req.Rows)
				assert.Equal(t, "120", req.Cols)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.validate(t, tt.request)
		})
	}
}

// TestSshTypeConstants tests SSH type constants
func TestSshTypeConstants(t *testing.T) {
	tests := []struct {
		name     string
		sshType  SshType
		expected string
	}{
		{
			name:     "SSH type",
			sshType:  SSH,
			expected: "ssh",
		},
		{
			name:     "WebShell type",
			sshType:  WebShell,
			expected: "webShell",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, string(tt.sshType))
		})
	}
}
