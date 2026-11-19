/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package posttraining

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

// A finished run with gaps or a failed run's directory still needs the end-of-run backfill.
func TestBackfillPending(t *testing.T) {
	failed := testRunRow("Failed", "train")
	failed.OutputPath = testRunDir
	require.True(t, backfillPending(failed, nil), "a failed run's directory is counted again")
	require.False(t, backfillPending(failed, []stageEntry{{Event: backfilledEvent}}), "once")
	done := testRunRow("Succeeded", "done")
	done.OutputPath = testRunDir
	require.False(t, backfillPending(done, nil))
	done.MetricCursor = `{"order":["a1"],"attempts":{"a1":{"maxSeq":9,"gaps":[{"from":4,"to":5,"since":1}]}}}`
	require.True(t, backfillPending(done, nil), "a finished run with gaps is backfilled")

	// A run whose workload the controller never saw, as one that was never created, wrote no directory.
	unseen := testRunRow("Failed", "queued")
	unseen.OutputPath, unseen.WorkloadUID = testRunDir, ""
	require.False(t, backfillPending(unseen, nil))
}

// The backfill workload has a DNS display name and runs the backfill script on the run directory.
func TestBackfillWorkload(t *testing.T) {
	run := testRunRow("Failed", "train")
	run.OutputPath, run.DisplayName = testRunDir, "Qwen3 SFT demo"
	run.MetricCursor = `{"order":["a1","a2"],"attempts":{"a1":{"maxSeq":9,"gaps":[{"from":4,"to":5,"since":1}]},"a2":{"maxSeq":3}}}`
	r, _, _ := newTestReconciler(t, run)
	ctx := context.Background()
	require.Equal(t, pendingRequeue, reconcileRun(t, r).RequeueAfter)
	w := &v1.Workload{}
	require.NoError(t, r.Get(ctx, client.ObjectKey{Name: backfillWorkloadName(testRun)}, w))
	require.Equal(t, "Job", w.Spec.Kind)
	require.Equal(t, "backfill-qwen3-sft-demo", w.Labels[v1.DisplayNameLabel], "the workload webhook needs a DNS name")
	require.Equal(t, v1.PostTrainingRoleBackfill, w.Labels[v1.PostTrainingRoleLabel])
	raw, err := base64.StdEncoding.DecodeString(w.Spec.Env["PT_BACKFILL"])
	require.NoError(t, err)
	var spec backfillSpec
	require.NoError(t, json.Unmarshal(raw, &spec))
	require.Equal(t, backfillSpec{Run: testRun, Dir: testRunDir, Attempt: "a2", Stage: "train",
		Ranges: map[string][][2]int64{"a1": {{4, 5}}}}, spec)

	w.Status.Phase = v1.WorkloadSucceeded
	require.NoError(t, r.Status().Update(ctx, w))
	reconcileRun(t, r)
	require.Error(t, r.Get(ctx, client.ObjectKey{Name: backfillWorkloadName(testRun)}, &v1.Workload{}),
		"the finished backfill is removed even when its log cannot be read")
}

// A queued backfill lets exports register and is given up when it does not end in time.
func TestAPendingBackfillDoesNotHoldExportsAndEndsInTime(t *testing.T) {
	run := testRunRow("Stopped", "train")
	run.OutputPath = testRunDir
	backfill := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: backfillWorkloadName(testRun),
		CreationTimestamp: metav1.NewTime(time.Now().Add(-time.Minute)),
		Labels:            map[string]string{v1.PostTrainingRunIdLabel: testRun, v1.PostTrainingRoleLabel: v1.PostTrainingRoleBackfill}}}
	backfill.Status.Phase = v1.WorkloadPending
	export := exportJob("pt-x20-g", v1.WorkloadSucceeded)
	r, db, updates := newTestReconciler(t, run, backfill, export)
	db.EXPECT().ListWorkloadPods(gomock.Any(), export.Name, string(export.UID)).Return(exportPods(export,
		`[SUCCESS] {"run":"`+testRun+`","attempt":"x20","pv":1,"stage":"export","output":"`+testRunDir+
			`/exports/step-20"}`), nil)
	ctx := context.Background()
	require.Equal(t, pendingRequeue, reconcileRun(t, r).RequeueAfter)
	models := &v1.ModelList{}
	require.NoError(t, r.List(ctx, models))
	require.Len(t, models.Items, 1, "the export of a stopped run registers while its backfill waits in the queue")

	require.NoError(t, r.Get(ctx, client.ObjectKeyFromObject(backfill), backfill))
	backfill.CreationTimestamp = metav1.NewTime(time.Now().Add(-2 * backfillWait))
	require.NoError(t, r.Update(ctx, backfill))
	reconcileRun(t, r)
	require.Error(t, r.Get(ctx, client.ObjectKeyFromObject(backfill), &v1.Workload{}), "a backfill that never ran is given up")
	require.Contains(t, (*updates)[len(*updates)-1]["stage_history"], "did not finish within")
}
