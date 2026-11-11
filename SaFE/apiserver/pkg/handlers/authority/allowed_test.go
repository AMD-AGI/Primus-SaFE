/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package authority

import (
	"context"
	"fmt"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority/authtest"
	apimetrics "github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/metrics"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
)

// Allowed answers as Authorize does, with the built-in roles as the chart ships them, for owners,
// members, managers, administrators, a user with no access and a frozen user; unlike Authorize it
// neither counts nor logs a refusal, so a list narrowed to what a user may see does not report
// the narrowing as denials.
func TestAllowedAgreesWithAuthorizeWithoutCountingRefusals(t *testing.T) {
	const ws, other = "allowed-ws", "allowed-other-ws"
	user := func(name string, roles []v1.UserRole, workspaces, managed []string) *v1.User {
		return &v1.User{
			ObjectMeta: metav1.ObjectMeta{Name: name, Labels: map[string]string{v1.UserIdLabel: name}},
			Spec: v1.UserSpec{Type: v1.DefaultUserType, Roles: roles, Resources: map[string][]string{
				common.UserWorkspaces: workspaces, common.UserManagedWorkspaces: managed}},
		}
	}
	def := []v1.UserRole{v1.DefaultRole}
	frozen := user("frozen-owner", def, []string{ws}, nil)
	frozen.Spec.RestrictedType = v1.UserFrozen
	users := []*v1.User{
		user("owner", def, []string{ws}, nil),
		user("member", def, []string{ws}, nil),
		user("manager", def, nil, []string{ws}),
		user("outsider", def, []string{other}, nil),
		user("admin", []v1.UserRole{v1.SystemAdminRole}, nil, nil),
		user("reader", []v1.UserRole{v1.SystemAdminReadonlyRole}, nil, nil),
		user("self-read", []v1.UserRole{v1.SelfReadRole}, nil, nil),
		frozen,
	}
	objs := authtest.ChartRoles(t)
	for _, u := range users {
		objs = append(objs, u)
	}
	sch := runtime.NewScheme()
	require.NoError(t, v1.AddToScheme(sch))
	a := &AccessController{Client: fake.NewClientBuilder().WithScheme(sch).WithObjects(objs...).Build()}

	type target struct {
		owner      string
		workspaces []string
	}
	targets := []target{
		{"owner", []string{ws}},
		{"frozen-owner", []string{ws}},
		{"", []string{ws}},
		{"", []string{other}},
		{"", nil},
	}
	verbs := []v1.RoleVerb{v1.GetVerb, v1.ListVerb, v1.DeleteVerb, v1.ResumeVerb, v1.CreateVerb}
	var granted, refused int
	for _, u := range users {
		for _, tg := range targets {
			for _, verb := range verbs {
				name := fmt.Sprintf("%s %s owner=%q workspaces=%v", u.Name, verb, tg.owner, tg.workspaces)
				in := AccessInput{
					Context: context.Background(), ResourceKind: v1.WorkloadKind, ResourceOwner: tg.owner,
					Verb: verb, Workspaces: tg.workspaces, UserId: u.Name,
				}
				denied := apimetrics.AuthzDeniedTotal.WithLabelValues("workload", string(verb))
				before := testutil.ToFloat64(denied)
				ok, err := a.Allowed(in)
				require.NoError(t, err, name)
				require.Equal(t, before, testutil.ToFloat64(denied), "%s: Allowed counted a refusal", name)
				authErr := a.Authorize(in)
				require.Equal(t, authErr == nil, ok, "%s: Authorize says %v", name, authErr)
				if ok {
					granted++
				} else {
					refused++
				}
			}
		}
	}
	// Both answers occur, so the agreement is not one constant matching another.
	require.Positive(t, granted)
	require.Positive(t, refused)

	// An unknown user is the same error Authorize returns.
	_, err := a.Allowed(AccessInput{Context: context.Background(), ResourceKind: v1.WorkloadKind,
		Verb: v1.GetVerb, UserId: "nobody"})
	require.Error(t, err)
	require.Equal(t, a.Authorize(AccessInput{Context: context.Background(), ResourceKind: v1.WorkloadKind,
		Verb: v1.GetVerb, UserId: "nobody"}).Error(), err.Error())
}
