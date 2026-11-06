/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package authority

import (
	"testing"

	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apis/pkg/client/clientset/versioned/scheme"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority/authtest"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
)

// AuthorizeWorkloadCreate makes the create path's three checks with the built-in roles as the
// chart ships them: workload create in the workspace, the priority, and workload/privileged for a
// privileged workload.
func TestAuthorizeWorkloadCreate(t *testing.T) {
	user := func(name string, workspaces, managed []string, roles ...v1.UserRole) *v1.User {
		u := &v1.User{
			ObjectMeta: metav1.ObjectMeta{Name: name},
			Spec:       v1.UserSpec{Type: v1.DefaultUserType, Roles: roles, Resources: map[string][]string{}},
		}
		if len(workspaces) > 0 {
			u.Spec.Resources[common.UserWorkspaces] = workspaces
		}
		if len(managed) > 0 {
			u.Spec.Resources[common.UserManagedWorkspaces] = managed
		}
		return u
	}
	ws := []string{"ws"}
	objs := append(authtest.ChartRoles(t),
		user("member", ws, nil, v1.DefaultRole),
		user("outsider", []string{"other"}, nil, v1.DefaultRole),
		user("manager", ws, ws, v1.DefaultRole),
		user("admin", nil, nil, v1.SystemAdminRole),
	)
	a := &AccessController{Client: fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(objs...).Build()}
	workload := func(priority int, privileged bool) *v1.Workload {
		w := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "w"},
			Spec: v1.WorkloadSpec{Workspace: "ws", Priority: priority}}
		if privileged {
			v1.SetAnnotation(w, v1.WorkloadPrivilegedAnnotation, v1.TrueStr)
		}
		return w
	}

	for _, tc := range []struct {
		user       string
		priority   int
		privileged bool
		refusedFor string
	}{
		{"member", common.LowPriorityInt, false, ""},
		{"member", common.MedPriorityInt, false, ""},
		{"member", common.HighPriorityInt, false, "workload/high-priority"},
		{"member", common.MedPriorityInt, true, WorkloadPrivilegedKind},
		{"outsider", common.LowPriorityInt, false, "create workload,"},
		{"manager", common.HighPriorityInt, true, ""},
		{"admin", common.HighPriorityInt, true, ""},
	} {
		err := a.AuthorizeWorkloadCreate(t.Context(), tc.user, workload(tc.priority, tc.privileged))
		if tc.refusedFor == "" {
			require.NoError(t, err, "%+v", tc)
			continue
		}
		require.True(t, apierrors.IsForbidden(err), "%+v: want 403, got %v", tc, err)
		require.Contains(t, err.Error(), tc.refusedFor, "%+v", tc)
	}

	err := a.AuthorizeWorkloadCreate(t.Context(), "nobody", workload(common.LowPriorityInt, false))
	require.Error(t, err)
}
