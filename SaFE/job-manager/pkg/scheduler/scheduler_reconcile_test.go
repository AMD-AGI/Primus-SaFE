/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package scheduler

import (
	"context"
	"testing"

	"gotest.tools/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
)

func TestSchedulerReconcileNotFound(t *testing.T) {
	cl := ctrlfake.NewClientBuilder().WithScheme(ttlScheme(t)).Build()
	r := &SchedulerReconciler{Client: cl}
	_, err := r.Reconcile(context.Background(), ctrlruntime.Request{
		NamespacedName: ctrlclient.ObjectKey{Name: "missing"},
	})
	assert.NilError(t, err)
}

func TestSchedulerReconcileDefaultWorkspace(t *testing.T) {
	// A workload in the default namespace short-circuits to nil.
	w := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "w"}}
	w.Spec.Workspace = corev1.NamespaceDefault
	cl := ctrlfake.NewClientBuilder().WithScheme(ttlScheme(t)).WithObjects(w).Build()
	r := &SchedulerReconciler{Client: cl}
	_, err := r.Reconcile(context.Background(), ctrlruntime.Request{
		NamespacedName: ctrlclient.ObjectKey{Name: "w"},
	})
	assert.NilError(t, err)
}

func TestDeleteRelatedSecretsEmpty(t *testing.T) {
	cl := ctrlfake.NewClientBuilder().WithScheme(ttlScheme(t)).Build()
	r := &SchedulerReconciler{Client: cl}
	err := r.deleteRelatedSecrets(context.Background(), schedWorkload("w"))
	assert.NilError(t, err)
}

func TestDeleteRelatedSecretsWithSecret(t *testing.T) {
	sec := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{
		Name:      "tok",
		Namespace: common.PrimusSafeNamespace,
		Labels:    map[string]string{v1.OwnerLabel: "w"},
	}}
	cl := ctrlfake.NewClientBuilder().WithScheme(ttlScheme(t)).WithObjects(sec).Build()
	r := &SchedulerReconciler{Client: cl}
	err := r.deleteRelatedSecrets(context.Background(), schedWorkload("w"))
	assert.NilError(t, err)

	// Secret should be deleted.
	got := &corev1.Secret{}
	gErr := cl.Get(context.Background(), ctrlclient.ObjectKey{Name: "tok", Namespace: common.PrimusSafeNamespace}, got)
	assert.Assert(t, gErr != nil)
}

func TestDeleteRelatedEphemeralRunnerNoId(t *testing.T) {
	r := &SchedulerReconciler{}
	// No scale-runner id label -> no-op, no client interaction.
	err := r.deleteRelatedEphemeralRunner(context.Background(), schedWorkload("w"), nil)
	assert.NilError(t, err)
}

func TestScheduleWorkloadsWorkspaceNotFound(t *testing.T) {
	cl := ctrlfake.NewClientBuilder().WithScheme(ttlScheme(t)).Build()
	r := &SchedulerReconciler{Client: cl}
	err := r.scheduleWorkloads(context.Background(), &SchedulerMessage{WorkspaceId: "missing"})
	assert.NilError(t, err)
}

func TestSchedulerDo(t *testing.T) {
	cl := ctrlfake.NewClientBuilder().WithScheme(ttlScheme(t)).Build()
	r := &SchedulerReconciler{Client: cl}
	// Workspace not found -> scheduleWorkloads returns nil -> Do returns nil.
	_, err := r.Do(context.Background(), &SchedulerMessage{WorkspaceId: "missing"})
	assert.NilError(t, err)
}
