/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package custom_handlers

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority"
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
