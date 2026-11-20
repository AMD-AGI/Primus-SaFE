/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package posttraining

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	mockclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/mock"
	commonclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/k8sclient"
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

// backfilled runs the end-of-run backfill of a run whose log has one point of its main metric, at step 2, that an
// earlier read missed. summary makes the stored summary of the run from the name of its main metric and its cap on
// points. It returns the columns and the points the backfill saved.
func backfilled(t *testing.T, summary func(main string, maxPoints int64) string) (map[string]interface{}, []string,
	string) {
	ctx := context.Background()
	run := testRunRow("Succeeded", "done")
	run.Framework, run.OutputPath = "slime", testRunDir
	w := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: backfillWorkloadName(testRun),
		Labels: map[string]string{v1.ClusterIdLabel: "c1"}}, Spec: v1.WorkloadSpec{Workspace: "ws1"}}
	w.Status.Phase = v1.WorkloadSucceeded
	r, _, _ := newTestReconciler(t, run, w)
	loadChartRegistry(t, r)
	fresh := r.newIngestion(ctx, newRunUpdate(run), loadCursor(""))
	main := fresh.mainName
	require.NotEmpty(t, main)
	run.MetricCursor = `{"order":["a1"],"attempts":{"a1":{"maxSeq":3,"gaps":[{"from":2,"to":2,"since":1}]}}}`
	run.MetricSummary = summary(main, fresh.maxPoints)
	log := "2026-11-20T00:00:00Z " + metricLine("a1", 2, 2, `{"`+main+`":0.9}`) + "\n"
	srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		if strings.HasSuffix(req.URL.Path, "/log") {
			_, _ = rw.Write([]byte(log))
			return
		}
		rw.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(rw).Encode(&corev1.PodList{TypeMeta: metav1.TypeMeta{Kind: "PodList", APIVersion: "v1"},
			Items: []corev1.Pod{{ObjectMeta: metav1.ObjectMeta{Name: "p1", Namespace: "ws1"},
				Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "main"}}}}}})
	}))
	defer srv.Close()
	pods, err := kubernetes.NewForConfig(&rest.Config{Host: srv.URL})
	require.NoError(t, err)
	r.clientManager.AddOrReplace("c1", commonclient.NewClientFactoryWithOnlyClient(ctx, "c1", pods))
	db := mockclient.NewMockInterface(gomock.NewController(t))
	var saved map[string]interface{}
	var keys []string
	db.EXPECT().SavePostTrainingRunMetrics(gomock.Any(), testRun, gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, _ string, points []*dbclient.PostTrainingRunMetric, fields map[string]interface{}) error {
			saved, keys = fields, pointKeys(points)
			return nil
		})
	r.db = db
	_, err = r.reconcileBackfill(ctx, run)
	require.NoError(t, err)
	return saved, keys, main
}

// The backfill reads the log of its pod, saves the point that was missing and leaves the main metric on the latest
// point of the run.
func TestTheBackfillSavesAMissingPointAndKeepsTheLatestMainMetric(t *testing.T) {
	saved, keys, main := backfilled(t, func(main string, _ int64) string {
		return `{"points":2,"main":{"name":"` + main + `","step":5,"value":0.5,` +
			`"spark":[{"step":1,"value":1},{"step":5,"value":0.5}],"stride":1,"seen":2}}`
	})
	require.Equal(t, []string{main + "@2=0.9"}, keys)
	require.NotContains(t, saved["stage_history"], "could not be read")
	sum := loadSummary(saved["metric_summary"].(string))
	require.Equal(t, int64(3), sum.Points)
	require.Equal(t, summaryPoint{Step: 5, Value: 0.5}, summaryPoint{Step: sum.Main.Step, Value: sum.Main.Value})
}

// A backfill that finds the run at its cap on points stores none of its points, and still records the cap that
// no earlier read had found.
func TestTheBackfillRecordsTheCapItFinds(t *testing.T) {
	saved, keys, _ := backfilled(t, func(_ string, maxPoints int64) string {
		return fmt.Sprintf(`{"points":%d}`, maxPoints)
	})
	require.Empty(t, keys)
	require.Contains(t, saved, "metric_summary")
	require.True(t, loadSummary(saved["metric_summary"].(string)).Capped)

	saved, keys, _ = backfilled(t, func(_ string, maxPoints int64) string {
		return fmt.Sprintf(`{"points":%d,"capped":true}`, maxPoints)
	})
	require.Empty(t, keys)
	require.NotContains(t, saved, "metric_summary", "a run whose cap is recorded keeps its summary")
}
