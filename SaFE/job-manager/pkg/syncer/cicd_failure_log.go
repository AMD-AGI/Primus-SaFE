/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package syncer

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog/v2"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/cicdlog"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/controller"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/opensearch"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/robustclient"
	commonworkload "github.com/AMD-AIG-AIMA/SAFE/common/pkg/workload"
	jmmetrics "github.com/AMD-AIG-AIMA/SAFE/job-manager/pkg/metrics"
	jobutils "github.com/AMD-AIG-AIMA/SAFE/job-manager/pkg/utils"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/timeutil"
)

type cicdFailureSnapshot struct {
	workload  *v1.Workload
	condition metav1.Condition
	since     time.Time
	until     time.Time
}

func cicdFailureKey(snapshot *cicdFailureSnapshot) string {
	return fmt.Sprintf("%s|%d|%s", snapshot.workload.UID, v1.GetWorkloadDispatchCnt(snapshot.workload),
		v1.GetAnnotation(snapshot.workload, v1.WorkloadDispatchedAnnotation))
}

type cicdFailureWorker struct {
	reconciler *SyncerReconciler
}

func (worker *cicdFailureWorker) Start(ctx context.Context) error {
	r := worker.reconciler
	if commonconfig.IsOpenSearchEnable() {
		rc := robustclient.NewClient(robustclient.DefaultClientConfig())
		opensearch.InitRobustClient(rc)
		robustclient.NewDiscovery(r.Client, rc, 30*time.Second).Start(ctx)
	}
	for i := 0; i < 2; i++ {
		r.cicdFailureLogs.Run(ctx)
	}
	<-ctx.Done()
	r.cicdFailureLogs.ShutDown()
	return nil
}

func (worker *cicdFailureWorker) Do(ctx context.Context, snapshot *cicdFailureSnapshot) (ctrlruntime.Result, error) {
	if err := worker.reconciler.enrichCICDFailureMessage(ctx, snapshot); err != nil {
		klog.V(2).Info("ARC failure enrichment unavailable; retaining the persisted diagnostic")
	}
	return ctrlruntime.Result{}, nil
}

func (r *SyncerReconciler) newCICDFailureWorker() *cicdFailureWorker {
	worker := &cicdFailureWorker{reconciler: r}
	r.cicdFailureLogs = controller.NewKeyedController[*cicdFailureSnapshot](worker, cicdFailureKey, nil, 2)
	return worker
}

func (r *SyncerReconciler) enqueueCICDFailureEnrichment(workload *v1.Workload) {
	if r.cicdFailureLogs == nil || !commonworkload.IsCICD(workload) || workload.Status.Phase != v1.WorkloadFailed || workload.Status.EndTime == nil {
		return
	}
	since, err := timeutil.CvtStrToRFC3339Milli(v1.GetAnnotation(workload, v1.WorkloadDispatchedAnnotation))
	index := commonworkload.WorkloadFailureConditionIndex(workload.Status.Conditions, v1.GetWorkloadDispatchCnt(workload))
	if err != nil || index < 0 || workload.Status.EndTime.Before(&metav1.Time{Time: since}) {
		return
	}
	snapshot := &cicdFailureSnapshot{
		workload: workload.DeepCopy(), condition: workload.Status.Conditions[index], since: since, until: workload.Status.EndTime.Time,
	}
	attempts, _ := r.cicdFailureAttempts.LoadOrStore(workload.Name, &sync.Map{})
	if _, exists := attempts.(*sync.Map).LoadOrStore(cicdFailureKey(snapshot), struct{}{}); !exists {
		r.cicdFailureLogs.Add(snapshot)
	}
}

func (r *SyncerReconciler) enrichCICDFailureMessage(ctx context.Context, snapshot *cicdFailureSnapshot) error {
	scope := cicdlog.ScopeFromWorkload(snapshot.workload)
	queries, err := cicdlog.ARCControllerQueries(scope, cicdlog.Query{
		ListLogInput: cicdlog.ListLogInput{Limit: 20, Order: "desc"}, SinceTime: snapshot.since, UntilTime: snapshot.until,
	})
	if err != nil {
		return err
	}
	queryCtx, cancel := context.WithTimeout(ctx, commonconfig.GetCICDFailureEnrichTimeout())
	defer cancel()
	responses, err := cicdlog.SearchARCControllerLogs(queryCtx, scope, cicdlog.SearchOptions{
		Queries: queries, ErrorsOnly: true, Observe: jmmetrics.ObserveCICDFailureEnrichment,
	})
	if err != nil {
		return err
	}
	detail := cicdlog.NewestARCControllerError(scope, responses, snapshot.since, snapshot.until)
	if detail == "" {
		return nil
	}
	suffix := []rune(" ARC controller: " + detail)
	if len(suffix) > 2048 {
		suffix = suffix[:2048]
	}
	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		return r.patchCICDFailureMessage(ctx, snapshot, string(suffix))
	})
}

func (r *SyncerReconciler) patchCICDFailureMessage(ctx context.Context, snapshot *cicdFailureSnapshot, suffix string) error {
	current := &v1.Workload{}
	if err := r.Get(ctx, client.ObjectKeyFromObject(snapshot.workload), current); err != nil {
		return client.IgnoreNotFound(err)
	}
	if current.UID != snapshot.workload.UID || current.Status.Phase != v1.WorkloadFailed ||
		v1.GetWorkloadDispatchCnt(current) != v1.GetWorkloadDispatchCnt(snapshot.workload) ||
		v1.GetAnnotation(current, v1.WorkloadDispatchedAnnotation) != v1.GetAnnotation(snapshot.workload, v1.WorkloadDispatchedAnnotation) {
		return nil
	}
	index := commonworkload.WorkloadFailureConditionIndex(current.Status.Conditions, v1.GetWorkloadDispatchCnt(current))
	if index < 0 {
		return nil
	}
	condition := &current.Status.Conditions[index]
	if condition.Type != snapshot.condition.Type || condition.Reason != snapshot.condition.Reason ||
		!condition.LastTransitionTime.Equal(&snapshot.condition.LastTransitionTime) ||
		condition.Message != snapshot.condition.Message || strings.Contains(condition.Message, " ARC controller: ") {
		return nil
	}
	condition.Message = strings.TrimSpace(condition.Message) + suffix
	current.Status.Message = condition.Message
	return jobutils.PatchWorkloadStatusFields(ctx, r.Client, current, map[string]any{
		"conditions": current.Status.Conditions, "message": current.Status.Message,
	})
}
