/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package posttraining

import (
	"context"
	"time"

	"k8s.io/klog/v2"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	rmmetrics "github.com/AMD-AIG-AIMA/SAFE/resource-manager/pkg/metrics"
)

const unfinishedSweepInterval = time.Minute

// runUnfinishedSweeps goes over the unfinished runs every unfinishedSweepInterval until ctx ends. The first sweep
// runs once the controller leads, so runs whose workloads changed while it was down are advanced too.
func (r *RunReconciler) runUnfinishedSweeps(ctx context.Context) error {
	ticker := time.NewTicker(unfinishedSweepInterval)
	defer ticker.Stop()
	for {
		r.sweepUnfinished(ctx)
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

// sweepUnfinished lists the unfinished runs once for two uses. It counts them by status and stage, and the runs
// marked stalled, for the gauges. And it queues them: a run is otherwise reconciled on an event of its workload or
// on a requeue of its own. A pending run whose workload was never created has neither, nor does a running one
// whose workload is gone once a restart lost its requeue.
func (r *RunReconciler) sweepUnfinished(ctx context.Context) {
	counts := map[[2]string]int{}
	stalled := 0
	filter := dbclient.PostTrainingRunFilter{
		Statuses: []string{string(v1.WorkloadPending), string(v1.WorkloadRunning)}, Limit: recoverPageSize}
	for {
		runs, _, err := r.db.ListPostTrainingRuns(ctx, filter)
		if err != nil {
			klog.ErrorS(err, "failed to list the unfinished Post Training runs")
			return
		}
		for _, run := range runs {
			counts[[2]string{run.Status, run.Stage}]++
			if run.StalledSince.Valid {
				stalled++
			}
			if !r.requeue(ctx, run.RunID) {
				return
			}
		}
		if len(runs) < recoverPageSize {
			break
		}
		filter.Offset += recoverPageSize
	}
	rmmetrics.PostTrainingRuns.Reset()
	for key, n := range counts {
		rmmetrics.PostTrainingRuns.WithLabelValues(key[0], key[1]).Set(float64(n))
	}
	rmmetrics.PostTrainingRunsStalled.Set(float64(stalled))
}

// reconcileError counts a reconcile error under reason and returns it.
func reconcileError(reason string, err error) error {
	if err != nil {
		rmmetrics.PostTrainingReconcileErrorsTotal.WithLabelValues(reason).Inc()
	}
	return err
}

// countRead adds what one log read found to the counters: rejected points, skipped long lines, metric lines
// the launcher failed to parse, and a run that got a metric gap.
func countRead(in *ingestion, rejectedBefore, skippedBefore int64, gapBefore, gapNow bool) {
	if n := in.sum.Rejected - rejectedBefore; n > 0 {
		rmmetrics.PostTrainingMetricRejectedTotal.Add(float64(n))
	}
	if n := in.cur.Skipped - skippedBefore; n > 0 {
		rmmetrics.PostTrainingLogLinesSkippedTotal.Add(float64(n))
	}
	if in.parseFailures > 0 {
		rmmetrics.PostTrainingMetricParseFailuresTotal.Add(float64(in.parseFailures))
	}
	if gapNow && !gapBefore {
		rmmetrics.PostTrainingMetricGapsTotal.Inc()
	}
}
