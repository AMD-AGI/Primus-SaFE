/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package posttraining

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	commonsearch "github.com/AMD-AIG-AIMA/SAFE/common/pkg/opensearch"
)

func TestSearchHitsFillAGap(t *testing.T) {
	in := newTestIngestion(100)
	feed(in, stageLine("a1", "train", "running", ""), metricLine("a1", 1, 1, `{"train/loss":1}`),
		metricLine("a1", 3, 3, `{"train/loss":0.8}`))
	require.Len(t, in.cur.Attempts["a1"].Gaps, 1)

	resp := &commonsearch.OpenSearchLogResponse{}
	resp.Hits.Hits = make([]commonsearch.OpenSearchLogDoc, 2)
	resp.Hits.Hits[0].Source.Log = metricLine("a1", 2, 2, `{"train/loss":0.9}`)
	resp.Hits.Hits[1].Source.Message = strings.Replace(metricLine("a1", 2, 2, `{}`), testRun, "pt-other", 1)
	ingestSearchHits(resp, in)
	require.Empty(t, in.cur.Attempts["a1"].Gaps, "the searched line fills the gap")
	require.Equal(t, []string{"train/loss@1=1", "train/loss@2=0.9", "train/loss@3=0.8"}, pointKeys(in.finish()))
}

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
}

func TestBackfillWorkload(t *testing.T) {
	run := testRunRow("Failed", "train")
	run.OutputPath = testRunDir
	run.MetricCursor = `{"order":["a1","a2"],"attempts":{"a1":{"maxSeq":9,"gaps":[{"from":4,"to":5,"since":1}]},"a2":{"maxSeq":3}}}`
	r, _, _ := newTestReconciler(t, run)
	ctx := context.Background()
	require.Equal(t, pendingRequeue, reconcileRun(t, r).RequeueAfter)
	w := &v1.Workload{}
	require.NoError(t, r.Get(ctx, client.ObjectKey{Name: backfillWorkloadName(testRun)}, w))
	require.Equal(t, "Job", w.Spec.Kind)
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
