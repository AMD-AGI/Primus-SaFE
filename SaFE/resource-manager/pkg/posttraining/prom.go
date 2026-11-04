/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package posttraining

import (
	"context"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	rmmetrics "github.com/AMD-AIG-AIMA/SAFE/resource-manager/pkg/metrics"
)

const gaugeInterval = time.Minute

// runGauges refreshes the run gauges every gaugeInterval until ctx ends.
func (r *RunReconciler) runGauges(ctx context.Context) error {
	ticker := time.NewTicker(gaugeInterval)
	defer ticker.Stop()
	for {
		r.refreshGauges(ctx)
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

// refreshGauges counts the unfinished runs by status and stage, and the runs marked stalled.
func (r *RunReconciler) refreshGauges(ctx context.Context) {
	counts := map[[2]string]int{}
	stalled := 0
	filter := dbclient.PostTrainingRunFilter{
		Statuses: []string{string(v1.WorkloadPending), string(v1.WorkloadRunning)}, Limit: recoverPageSize}
	for {
		runs, _, err := r.db.ListPostTrainingRuns(ctx, filter)
		if err != nil {
			klog.ErrorS(err, "failed to count the unfinished Post Training runs")
			return
		}
		for _, run := range runs {
			counts[[2]string{run.Status, run.Stage}]++
			if run.StalledSince.Valid {
				stalled++
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
	r.countOrphanServices(ctx)
}

// countOrphanServices counts the evaluation inference services that still run after their evaluation ended,
// without being kept.
func (r *RunReconciler) countOrphanServices(ctx context.Context) {
	services := &v1.WorkloadList{}
	if err := r.List(ctx, services, client.MatchingLabels{v1.PostTrainingRoleLabel: v1.PostTrainingRoleEval}); err != nil {
		klog.ErrorS(err, "failed to list the Post Training evaluation services")
		return
	}
	orphans := 0
	for i := range services.Items {
		s := &services.Items[i]
		name := v1.GetAnnotation(s, evalJobAnnotation)
		if s.IsEnd() || name == "" || v1.GetAnnotation(s, evalKeepAnnotation) == v1.TrueStr {
			continue
		}
		job := &v1.OpsJob{}
		err := r.Get(ctx, client.ObjectKey{Name: name}, job)
		if apierrors.IsNotFound(err) ||
			(err == nil && (job.Status.Phase == v1.OpsJobSucceeded || job.Status.Phase == v1.OpsJobFailed)) {
			orphans++
		}
	}
	rmmetrics.PostTrainingEvalServicesOrphaned.Set(float64(orphans))
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
