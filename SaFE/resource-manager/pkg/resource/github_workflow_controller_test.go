/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resource

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	githubpkg "github.com/AMD-AIG-AIMA/SAFE/resource-manager/pkg/github"
)

func TestIsEphemeralRunnerWorkload(t *testing.T) {
	wl := &v1.Workload{}
	wl.Spec.GroupVersionKind.Kind = common.CICDEphemeralRunnerKind
	assert.True(t, isEphemeralRunnerWorkload(wl))
	assert.False(t, isEphemeralRunnerWorkload(&v1.Workload{}))
	assert.False(t, isEphemeralRunnerWorkload(&corev1.Pod{}))
}

func TestSetupGitHubWorkflowControllerDBDisabled(t *testing.T) {
	// DB disabled by default -> returns nil without touching manager.
	assert.NoError(t, SetupGitHubWorkflowController(nil))
}

func TestGitHubWorkflowReconcileNotFound(t *testing.T) {
	scheme, _ := genMockScheme()
	cl := ctrlfake.NewClientBuilder().WithScheme(scheme).Build()
	r := &GitHubWorkflowReconciler{Client: cl}
	res, err := r.Reconcile(context.Background(), ctrlruntime.Request{NamespacedName: types.NamespacedName{Name: "missing"}})
	assert.NoError(t, err)
	assert.Equal(t, ctrlruntime.Result{}, res)
}

func TestGitHubWorkflowReconcileNonEphemeral(t *testing.T) {
	scheme, _ := genMockScheme()
	wl := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "wl1"}}
	cl := ctrlfake.NewClientBuilder().WithScheme(scheme).WithObjects(wl).Build()
	r := &GitHubWorkflowReconciler{Client: cl}
	_, err := r.Reconcile(context.Background(), ctrlruntime.Request{NamespacedName: types.NamespacedName{Name: "wl1"}})
	assert.NoError(t, err)
}

func TestGitHubWorkflowReconcileNoRunID(t *testing.T) {
	scheme, _ := genMockScheme()
	wl := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "wl1", Annotations: map[string]string{"x": "y"}}}
	wl.Spec.GroupVersionKind.Kind = common.CICDEphemeralRunnerKind
	cl := ctrlfake.NewClientBuilder().WithScheme(scheme).WithObjects(wl).Build()
	r := &GitHubWorkflowReconciler{Client: cl}
	_, err := r.Reconcile(context.Background(), ctrlruntime.Request{NamespacedName: types.NamespacedName{Name: "wl1"}})
	assert.NoError(t, err)
}

func TestGitHubWorkflowReconcileWithRunID(t *testing.T) {
	scheme, _ := genMockScheme()
	wl := &v1.Workload{ObjectMeta: metav1.ObjectMeta{
		Name: "wl1",
		Annotations: map[string]string{
			"actions.github.com/run-id":     "100",
			"actions.github.com/repository": "owner/repo",
		},
	}}
	wl.Spec.GroupVersionKind.Kind = common.CICDEphemeralRunnerKind
	cl := ctrlfake.NewClientBuilder().WithScheme(scheme).WithObjects(wl).Build()

	db, mock, _ := sqlmock.New()
	defer db.Close()
	mock.ExpectExec("INSERT INTO github_workflow_runs").WillReturnResult(sqlmock.NewResult(1, 1))

	r := &GitHubWorkflowReconciler{Client: cl, tracker: githubpkg.NewWorkflowTracker(githubpkg.NewStore(db))}
	_, err := r.Reconcile(context.Background(), ctrlruntime.Request{NamespacedName: types.NamespacedName{Name: "wl1"}})
	assert.NoError(t, err)
}
