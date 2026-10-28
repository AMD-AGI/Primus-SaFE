/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package ssh_handlers

import (
	"bytes"
	"context"
	"testing"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority"
	"github.com/spf13/viper"
	testifyassert "github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

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

// --- merged from k8s_auth_test.go ---

func TestSendError(t *testing.T) {
	var buf bytes.Buffer
	sendError(&buf, "boom")
	assert.Equal(t, "boom\n", buf.String())
}

func TestAuthUser(t *testing.T) {
	adminUser := &v1.User{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "admin",
			Labels:      map[string]string{v1.UserIdLabel: "admin"},
			Annotations: map[string]string{v1.UserNameAnnotation: "admin"},
		},
		Spec: v1.UserSpec{Type: v1.DefaultUserType, Roles: []v1.UserRole{v1.SystemAdminRole}},
	}
	adminRole := &v1.Role{
		ObjectMeta: metav1.ObjectMeta{Name: string(v1.SystemAdminRole)},
		Rules: []v1.PolicyRule{{
			Resources:    []string{authority.AllResource},
			Verbs:        []v1.RoleVerb{v1.AllVerb},
			GrantedUsers: []string{authority.GrantedAllUser},
		}},
	}

	scheme := runtime.NewScheme()
	_ = v1.AddToScheme(scheme)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(adminUser, adminRole).Build()

	h := &SshHandler{accessController: authority.NewAccessController(fakeClient)}
	workload := &v1.Workload{
		ObjectMeta: metav1.ObjectMeta{Name: "wl-1"},
		Spec:       v1.WorkloadSpec{Workspace: "ws-1"},
	}

	// Admin user is authorized.
	err := h.authUser(context.Background(), &UserInfo{User: "admin"}, workload)
	testifyassert.NoError(t, err)

	// Unknown user is rejected.
	err = h.authUser(context.Background(), &UserInfo{User: "nobody"}, workload)
	testifyassert.Error(t, err)
}

// An SSH session is authenticated by public key, not by an identity provider. In external mode a
// user whose stored record lists system-admin is therefore an ordinary user there, as for a built-in
// session or API key: the platform role is the identity provider's to grant. In builtin mode, the
// default, the stored role is the role.
func TestAuthUserHoldsNoStoredPlatformRoleInExternalMode(t *testing.T) {
	adminRole := &v1.Role{
		ObjectMeta: metav1.ObjectMeta{Name: string(v1.SystemAdminRole)},
		Rules: []v1.PolicyRule{{
			Resources:    []string{authority.AllResource},
			Verbs:        []v1.RoleVerb{v1.AllVerb},
			GrantedUsers: []string{authority.GrantedAllUser},
		}},
	}
	defaultRole := &v1.Role{
		ObjectMeta: metav1.ObjectMeta{Name: string(v1.DefaultRole)},
		Rules: []v1.PolicyRule{{
			Resources:    []string{authority.AllResource},
			Verbs:        []v1.RoleVerb{v1.GetVerb},
			GrantedUsers: []string{authority.GrantedOwner},
		}},
	}
	newUser := func(name string, roles ...v1.UserRole) *v1.User {
		return &v1.User{
			ObjectMeta: metav1.ObjectMeta{Name: name, Labels: map[string]string{v1.UserIdLabel: name}},
			Spec:       v1.UserSpec{Type: v1.DefaultUserType, Roles: roles},
		}
	}
	scheme := runtime.NewScheme()
	_ = v1.AddToScheme(scheme)
	cli := fake.NewClientBuilder().WithScheme(scheme).WithObjects(adminRole, defaultRole,
		newUser("admin", v1.SystemAdminRole), newUser("owner", v1.DefaultRole)).Build()
	// Not NewAccessController: that is a singleton, and another test's client would win.
	h := &SshHandler{accessController: &authority.AccessController{Client: cli}}
	workload := &v1.Workload{
		ObjectMeta: metav1.ObjectMeta{Name: "wl-1", Labels: map[string]string{v1.UserIdLabel: "owner"}},
		Spec:       v1.WorkloadSpec{Workspace: "ws-1"},
	}
	authorize := func(user string) error {
		return h.authUser(context.Background(), &UserInfo{User: user}, workload)
	}

	// Builtin mode: the stored role is the role.
	testifyassert.NoError(t, authorize("admin"))

	viper.Set("auth.mode", "external")
	viper.Set("auth.authenticators", []map[string]any{{"type": "builtin"}})
	require.NoError(t, authority.InitExternalAuth(nil))
	defer func() {
		viper.Set("auth.mode", nil)
		viper.Set("auth.authenticators", nil)
		require.NoError(t, authority.InitExternalAuth(nil))
	}()

	testifyassert.Error(t, authorize("admin"), "the stored platform role does not count over SSH")
	// The owner of the workload is still its owner: only the platform role went, not the account.
	testifyassert.NoError(t, authorize("owner"))
}
