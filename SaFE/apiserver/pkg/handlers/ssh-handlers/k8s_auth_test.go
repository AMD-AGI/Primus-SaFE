/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package ssh_handlers

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority"
)

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
	assert.NoError(t, err)

	// Unknown user is rejected.
	err = h.authUser(context.Background(), &UserInfo{User: "nobody"}, workload)
	assert.Error(t, err)
}
