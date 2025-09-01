/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package authority

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
)

// MockClient is a mock implementation of client.Client
type MockClient struct {
	client.Client
	mock.Mock
}

func (m *MockClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object) error {
	args := m.Called(ctx, key, obj)
	return args.Error(0)
}

func TestGetPolicyRules(t *testing.T) {
	role := &v1.Role{
		Rules: []v1.PolicyRule{
			{
				Resources:    []string{"workload"},
				GrantedUsers: []string{"owner"},
				Verbs:        []v1.RoleVerb{v1.GetVerb, v1.ListVerb},
			},
			{
				Resources:    []string{"*"},
				GrantedUsers: []string{"workspace-user"},
				Verbs:        []v1.RoleVerb{v1.CreateVerb},
			},
			{
				Resources:    []string{"user"},
				GrantedUsers: []string{"testuser"},
				Verbs:        []v1.RoleVerb{v1.UpdateVerb},
			},
		},
	}

	tests := []struct {
		name             string
		resourceKind     string
		resourceName     string
		isOwner          bool
		isWorkspaceUser  bool
		expectedRulesLen int
	}{
		{
			name:             "owner access to workload",
			resourceKind:     "workload",
			resourceName:     "test-workload",
			isOwner:          true,
			isWorkspaceUser:  false,
			expectedRulesLen: 1,
		},
		{
			name:             "workspace user access to any resource",
			resourceKind:     "node",
			resourceName:     "test-node",
			isOwner:          false,
			isWorkspaceUser:  true,
			expectedRulesLen: 1,
		},
		{
			name:             "specific user access",
			resourceKind:     "user",
			resourceName:     "testuser",
			isOwner:          false,
			isWorkspaceUser:  false,
			expectedRulesLen: 1,
		},
		{
			name:             "no matching rules",
			resourceKind:     "nonexistent",
			resourceName:     "test-resource",
			isOwner:          false,
			isWorkspaceUser:  false,
			expectedRulesLen: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rules := getPolicyRules(role, tt.resourceKind, tt.resourceName, tt.isOwner, tt.isWorkspaceUser)
			assert.Len(t, rules, tt.expectedRulesLen)
		})
	}
}

func TestIsMatchVerb(t *testing.T) {
	rules := []*v1.PolicyRule{
		{
			Verbs: []v1.RoleVerb{v1.GetVerb, v1.ListVerb},
		},
		{
			Verbs: []v1.RoleVerb{v1.AllVerb},
		},
	}

	tests := []struct {
		name     string
		verb     v1.RoleVerb
		expected bool
	}{
		{
			name:     "exact match",
			verb:     v1.GetVerb,
			expected: true,
		},
		{
			name:     "all verb match",
			verb:     v1.DeleteVerb,
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isMatchVerb(rules, tt.verb)
			assert.Equal(t, tt.expected, result)
		})
	}

	rules = []*v1.PolicyRule{
		{
			Verbs: []v1.RoleVerb{v1.GetVerb, v1.ListVerb},
		},
	}
	result := isMatchVerb(rules, v1.UpdateVerb)
	assert.Equal(t, result, false)
}

func TestAuthorize(t *testing.T) {
	// Setup test scheme
	scheme := runtime.NewScheme()
	_ = v1.AddToScheme(scheme)

	// Create test roles
	systemAdminRole := &v1.Role{
		ObjectMeta: metav1.ObjectMeta{Name: string(v1.SystemAdminRole)},
		Rules: []v1.PolicyRule{
			{
				Resources:    []string{"*"},
				GrantedUsers: []string{"*"},
				Verbs:        []v1.RoleVerb{"*"},
			},
		},
	}

	workspaceAdminRole := &v1.Role{
		ObjectMeta: metav1.ObjectMeta{Name: string(v1.WorkspaceAdminRole)},
		Rules: []v1.PolicyRule{
			{
				Resources:    []string{"workload"},
				GrantedUsers: []string{"workspace-user"},
				Verbs:        []v1.RoleVerb{"*"},
			},
		},
	}

	defaultRole := &v1.Role{
		ObjectMeta: metav1.ObjectMeta{Name: string(v1.DefaultRole)},
		Rules: []v1.PolicyRule{
			{
				Resources:    []string{"workload"},
				GrantedUsers: []string{"owner"},
				Verbs:        []v1.RoleVerb{v1.GetVerb, v1.ListVerb},
			},
		},
	}

	// Create test user
	testUser := &v1.User{
		ObjectMeta: metav1.ObjectMeta{Name: "testuser"},
		Spec: v1.UserSpec{
			Type:           v1.DefaultUser,
			Roles:          []v1.UserRole{v1.DefaultRole},
			RestrictedType: v1.UserNormal,
			Resources: map[string][]string{
				common.UserWorkspaces: {"test-workspace"},
			},
		},
	}

	// Create fake client
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(systemAdminRole, workspaceAdminRole, defaultRole, testUser).
		Build()

	authorizer := NewAuthorizer(fakeClient)

	// Create test resource
	testWorkload := &v1.Workload{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-workload",
			Labels: map[string]string{
				"primus-safe.user.id": "testuser",
			},
		},
		Spec: v1.WorkloadSpec{
			Workspace: "test-workspace",
		},
	}

	tests := []struct {
		name        string
		input       Input
		expectError bool
	}{
		{
			name: "system admin access - should pass",
			input: Input{
				ResourceKind:  "workload",
				ResourceOwner: "testuser",
				Resource:      testWorkload,
				Verb:          v1.DeleteVerb,
				User: &v1.User{
					ObjectMeta: metav1.ObjectMeta{Name: "admin"},
					Spec: v1.UserSpec{
						Type:  v1.DefaultUser,
						Roles: []v1.UserRole{v1.SystemAdminRole},
					},
				},
				Roles: []*v1.Role{systemAdminRole},
			},
			expectError: false,
		},
		{
			name: "owner access with correct permissions - should pass",
			input: Input{
				ResourceKind:  "workload",
				ResourceOwner: "testuser",
				Resource:      testWorkload,
				Verb:          v1.GetVerb,
				User:          testUser,
				Roles:         []*v1.Role{defaultRole},
			},
			expectError: false,
		},
		{
			name: "owner access with incorrect permissions - should fail",
			input: Input{
				ResourceKind:  "workload",
				ResourceOwner: "testuser",
				Resource:      testWorkload,
				Verb:          v1.DeleteVerb,
				User:          testUser,
				Roles:         []*v1.Role{defaultRole},
			},
			expectError: true,
		},
		{
			name: "workspace user access - should pass",
			input: Input{
				ResourceKind:  "workload",
				ResourceOwner: "otheruser",
				Resource:      testWorkload,
				Verb:          v1.GetVerb,
				User: &v1.User{
					ObjectMeta: metav1.ObjectMeta{Name: "workspaceuser"},
					Spec: v1.UserSpec{
						Type:  v1.DefaultUser,
						Roles: []v1.UserRole{v1.DefaultRole},
						Resources: map[string][]string{
							common.UserWorkspaces:        {"test-workspace"},
							common.UserManagedWorkspaces: {"test-workspace"},
						},
					},
				},
				Roles:      []*v1.Role{defaultRole},
				Workspaces: []string{"test-workspace"},
			},
			expectError: false,
		},
		{
			name: "restricted user - should fail",
			input: Input{
				ResourceKind: "workload",
				Resource:     testWorkload,
				Verb:         v1.GetVerb,
				User: &v1.User{
					ObjectMeta: metav1.ObjectMeta{Name: "restricteduser"},
					Spec: v1.UserSpec{
						Type:           v1.DefaultUser,
						Roles:          []v1.UserRole{v1.DefaultRole},
						RestrictedType: v1.UserFrozen,
					},
				},
				Roles: []*v1.Role{defaultRole},
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.input.Context = context.Background()
			err := authorizer.authorize(tt.input)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestGetRequestUser(t *testing.T) {
	// Setup test scheme
	scheme := runtime.NewScheme()
	_ = v1.AddToScheme(scheme)

	// Create test user
	testUser := &v1.User{
		ObjectMeta: metav1.ObjectMeta{Name: "testuser"},
		Spec: v1.UserSpec{
			Type:  v1.DefaultUser,
			Roles: []v1.UserRole{v1.DefaultRole},
		},
	}

	// Create fake client
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(testUser).
		Build()

	authorizer := NewAuthorizer(fakeClient)

	user, err := authorizer.GetRequestUser(context.Background(), "testuser")
	assert.NoError(t, err)
	assert.NotNil(t, user)
	assert.Equal(t, "testuser", user.Name)
}

func TestIsMatchVerbEmptyRules(t *testing.T) {
	var emptyRules []*v1.PolicyRule
	result := isMatchVerb(emptyRules, v1.GetVerb)
	assert.False(t, result)
}
