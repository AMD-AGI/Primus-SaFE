/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package posttraining

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/lib/pq"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/controller-runtime/pkg/event"

	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	rmmetrics "github.com/AMD-AIG-AIMA/SAFE/resource-manager/pkg/metrics"
)

// The sweep of the unfinished runs counts them by status and stage, and the stalled ones, and queues the pending
// ones, which have no event of a workload when theirs was never created.
func TestSweepUnfinished(t *testing.T) {
	r, db, _ := newTestReconciler(t, testRunRow("Running", "train"))
	r.recovered = make(chan event.GenericEvent, 4)
	stalled := testRunRow("Running", "train")
	stalled.StalledSince = pq.NullTime{Valid: true}
	pending := testRunRow("Pending", "queued")
	pending.RunID = "pt-pending"
	db.EXPECT().ListPostTrainingRuns(gomock.Any(), gomock.Any()).Return(
		[]*dbclient.PostTrainingRun{testRunRow("Running", "train"), stalled, pending}, int64(3), nil)
	r.sweepUnfinished(context.Background())
	require.Equal(t, 2.0, testutil.ToFloat64(rmmetrics.PostTrainingRuns.WithLabelValues("Running", "train")))
	require.Equal(t, 1.0, testutil.ToFloat64(rmmetrics.PostTrainingRuns.WithLabelValues("Pending", "queued")))
	require.Equal(t, 1.0, testutil.ToFloat64(rmmetrics.PostTrainingRunsStalled))
	require.Len(t, r.recovered, 1, "a run that runs has the events of its workload")
	require.Equal(t, "pt-pending", (<-r.recovered).Object.GetName())
}

// Read counters grow only by what a read added, and a known gap is not counted again.
func TestCountReadAddsGrowthOnly(t *testing.T) {
	before := testutil.ToFloat64(rmmetrics.PostTrainingMetricParseFailuresTotal)
	gaps := testutil.ToFloat64(rmmetrics.PostTrainingMetricGapsTotal)
	in := newTestIngestion(100)
	feed(in, stageLine("a1", "train", "running", ""),
		stageLine("a1", "train", "heartbeat", `{"parseFailures":3}`),
		stageLine("a1", "train", "heartbeat", `{"parseFailures":5}`),
		stageLine("a1", "train", "heartbeat", `{"parseFailures":5}`))
	countRead(in, 0, 0, false, true)
	require.Equal(t, 5.0, testutil.ToFloat64(rmmetrics.PostTrainingMetricParseFailuresTotal)-before)
	require.Equal(t, 1.0, testutil.ToFloat64(rmmetrics.PostTrainingMetricGapsTotal)-gaps)
	countRead(&ingestion{cur: in.cur, sum: in.sum}, 0, 0, true, true)
	require.Equal(t, 1.0, testutil.ToFloat64(rmmetrics.PostTrainingMetricGapsTotal)-gaps, "a known gap is not counted again")
}
