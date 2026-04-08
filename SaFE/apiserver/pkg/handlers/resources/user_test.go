/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resources

import (
	"context"
	"testing"

	"gotest.tools/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/resources/view"
)

func genMockUser() *v1.User {
	return &v1.User{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-user",
			Labels: map[string]string{
				v1.UserIdLabel: "test-user",
			},
			Annotations: map[string]string{
				v1.UserNameAnnotation: "test-user",
			},
		},
		Spec: v1.UserSpec{
			Type:  v1.DefaultUserType,
			Roles: []v1.UserRole{v1.SystemAdminRole},
		},
	}
}

func genMockRole() *v1.Role {
	return &v1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name: string(v1.SystemAdminRole),
		},
		Rules: []v1.PolicyRule{{
			Resources:    []string{authority.AllResource},
			Verbs:        []v1.RoleVerb{v1.AllVerb},
			GrantedUsers: []string{authority.GrantedAllUser},
		}},
	}
}

func createMockUser() (*v1.User, client.WithWatch) {
	mockUser := genMockUser()
	mockRole := genMockRole()
	scheme := runtime.NewScheme()
	_ = v1.AddToScheme(scheme)

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(mockUser, mockRole).Build()
	return mockUser, fakeClient
}

func TestIsUserEnableNotification(t *testing.T) {
	t.Run("default is false", func(t *testing.T) {
		user := genMockUser()
		assert.Equal(t, v1.IsUserEnableNotification(user), false)
	})

	t.Run("returns true when annotation set", func(t *testing.T) {
		user := genMockUser()
		v1.SetAnnotation(user, v1.UserEnableNotificationAnnotation, v1.TrueStr)
		assert.Equal(t, v1.IsUserEnableNotification(user), true)
	})

	t.Run("returns false after annotation removed", func(t *testing.T) {
		user := genMockUser()
		v1.SetAnnotation(user, v1.UserEnableNotificationAnnotation, v1.TrueStr)
		v1.RemoveAnnotation(user, v1.UserEnableNotificationAnnotation)
		assert.Equal(t, v1.IsUserEnableNotification(user), false)
	})
}

func TestUserSettingsResponse(t *testing.T) {
	t.Run("response reflects annotation off", func(t *testing.T) {
		user := genMockUser()
		resp := &view.UserSettingsResponse{
			EnableNotification: v1.IsUserEnableNotification(user),
		}
		assert.Equal(t, resp.EnableNotification, false)
	})

	t.Run("response reflects annotation on", func(t *testing.T) {
		user := genMockUser()
		v1.SetAnnotation(user, v1.UserEnableNotificationAnnotation, v1.TrueStr)
		resp := &view.UserSettingsResponse{
			EnableNotification: v1.IsUserEnableNotification(user),
		}
		assert.Equal(t, resp.EnableNotification, true)
	})
}

func TestUserSettingsAnnotationPersistence(t *testing.T) {
	user := genMockUser()
	s := runtime.NewScheme()
	_ = v1.AddToScheme(s)
	fakeClient := fake.NewClientBuilder().WithScheme(s).WithObjects(user).Build()
	ctx := context.Background()

	t.Run("enable persists to store", func(t *testing.T) {
		v1.SetAnnotation(user, v1.UserEnableNotificationAnnotation, v1.TrueStr)
		err := fakeClient.Update(ctx, user)
		assert.NilError(t, err)

		fetched := &v1.User{}
		err = fakeClient.Get(ctx, client.ObjectKeyFromObject(user), fetched)
		assert.NilError(t, err)
		assert.Equal(t, v1.IsUserEnableNotification(fetched), true)
	})

	t.Run("disable persists to store", func(t *testing.T) {
		v1.RemoveAnnotation(user, v1.UserEnableNotificationAnnotation)
		err := fakeClient.Update(ctx, user)
		assert.NilError(t, err)

		fetched := &v1.User{}
		err = fakeClient.Get(ctx, client.ObjectKeyFromObject(user), fetched)
		assert.NilError(t, err)
		assert.Equal(t, v1.IsUserEnableNotification(fetched), false)
	})
}
