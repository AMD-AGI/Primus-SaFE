/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package syncer

import (
	"context"
	"testing"

	"gotest.tools/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
)

func TestSyncGithubAnnotationsWrongKind(t *testing.T) {
	c := &ClusterClientSets{}
	u := &unstructured.Unstructured{Object: map[string]interface{}{}}
	u.SetKind("Pod")
	// Wrong kind -> no-op, no panic (adminClient unused).
	c.syncGithubAnnotations(u)
}

func TestSyncGithubAnnotationsNoWorkloadId(t *testing.T) {
	c := &ClusterClientSets{}
	u := &unstructured.Unstructured{Object: map[string]interface{}{}}
	u.SetKind(common.CICDEphemeralRunnerKind)
	// No workload id label -> no-op.
	c.syncGithubAnnotations(u)
}

func TestSyncGithubAnnotationsUpdates(t *testing.T) {
	wl := &v1.Workload{ObjectMeta: metav1.ObjectMeta{
		Name:      "wl-1",
		Namespace: common.PrimusSafeNamespace,
	}}
	cl := ctrlfake.NewClientBuilder().WithScheme(syncerScheme(t)).WithObjects(wl).Build()
	c := &ClusterClientSets{adminClient: cl}

	u := &unstructured.Unstructured{Object: map[string]interface{}{
		"status": map[string]interface{}{
			"workflowRunId":     int64(42),
			"jobRepositoryName": "org/repo",
		},
	}}
	u.SetKind(common.CICDEphemeralRunnerKind)
	u.SetLabels(map[string]string{v1.WorkloadIdLabel: "wl-1"})

	c.syncGithubAnnotations(u)

	got := &v1.Workload{}
	assert.NilError(t, cl.Get(context.Background(),
		ctrlclient.ObjectKey{Name: "wl-1", Namespace: common.PrimusSafeNamespace}, got))
	assert.Equal(t, got.GetAnnotations()["actions.github.com/run-id"], "42")
	assert.Equal(t, got.GetAnnotations()["actions.github.com/repository"], "org/repo")
}

func TestSyncGithubAnnotationsWorkloadNotFound(t *testing.T) {
	cl := ctrlfake.NewClientBuilder().WithScheme(syncerScheme(t)).Build()
	c := &ClusterClientSets{adminClient: cl}

	u := &unstructured.Unstructured{Object: map[string]interface{}{
		"status": map[string]interface{}{"jobId": int64(7)},
	}}
	u.SetKind(common.CICDEphemeralRunnerKind)
	u.SetLabels(map[string]string{v1.WorkloadIdLabel: "missing"})
	// Workload not found -> no-op, no panic.
	c.syncGithubAnnotations(u)
}
