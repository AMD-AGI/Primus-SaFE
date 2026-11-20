/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resource

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"k8s.io/klog/v2"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/robustclient"
)

const (
	// Set to "false" on a Cluster CR to stop syncing its workloads to Robust.
	annotationRobustWorkloadSync = "primus-safe.amd.com/robust-workload-sync"

	robustSyncBatchSize  = 200
	robustSyncDBPageSize = 1000

	robustPushMissing = "missing"
	robustPushStale   = "stale"
	robustPushEnded   = "ended"
)

var (
	robustSyncErrors = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "safe_robust_sync_errors_total",
		Help: "Total number of failed robust sync operations",
	}, []string{"cluster", "type"})

	robustSyncSuccess = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "safe_robust_sync_success_total",
		Help: "Total number of successful robust sync operations",
	}, []string{"cluster", "type"})

	robustSyncPushed = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "safe_robust_sync_pushed_total",
		Help: "Total number of workloads pushed to robust by the reconcile loop",
	}, []string{"cluster", "reason"})

	robustSyncDeleted = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "safe_robust_sync_deleted_total",
		Help: "Total number of workloads deleted from robust by the reconcile loop",
	}, []string{"cluster"})

	robustSyncReconcileErrors = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "safe_robust_sync_reconcile_errors_total",
		Help: "Total number of failed robust sync reconcile rounds",
	}, []string{"cluster"})

	robustSyncFallback = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "safe_robust_sync_fallback_total",
		Help: "Total number of reconcile rounds that fell back to the catch-up push because robust has no sync digest",
	}, []string{"cluster"})
)

// workloadHistoryReader reads the workloads SaFE keeps in its database.
type workloadHistoryReader interface {
	SelectWorkloadsEndedSince(ctx context.Context, cluster string, since time.Time, limit, offset int) ([]*dbclient.Workload, error)
	SelectWorkloadsByUIDs(ctx context.Context, uids []string) ([]*dbclient.Workload, error)
}

// WorkloadRobustSyncer reconciles Workload CRDs and pushes sync events
// to data-plane robust-api instances. Sync failures are retried and
// do NOT block the Workload lifecycle.
type WorkloadRobustSyncer struct {
	client.Client
	robustClient *robustclient.Client
	// dbClient is nil when the database is disabled.
	dbClient  workloadHistoryReader
	retention time.Duration
	now       func() time.Time
}

func SetupWorkloadRobustSyncer(mgr manager.Manager, rc *robustclient.Client) error {
	if rc == nil {
		klog.Info("[workload-robust-syncer] robust client not configured, skipping")
		return nil
	}

	r := &WorkloadRobustSyncer{
		Client:       mgr.GetClient(),
		robustClient: rc,
		retention:    time.Duration(commonconfig.GetWorkloadRobustSyncRetentionDay()) * 24 * time.Hour,
		now:          time.Now,
	}
	if commonconfig.IsDBEnable() {
		if db := dbclient.NewClient(); db != nil {
			r.dbClient = db
		} else {
			klog.Warning("[workload-robust-syncer] db client not initialized, reconciling against etcd only")
		}
	}

	err := ctrlruntime.NewControllerManagedBy(mgr).
		Named("workload-robust-syncer").
		For(&v1.Workload{}).
		WithOptions(controller.Options{MaxConcurrentReconciles: 4}).
		Complete(r)
	if err != nil {
		return err
	}

	interval := time.Duration(commonconfig.GetWorkloadRobustSyncIntervalSecond()) * time.Second
	if interval <= 0 {
		interval = 60 * time.Second
	}
	if err = mgr.Add(manager.RunnableFunc(func(ctx context.Context) error {
		r.startReconcileLoop(ctx, interval)
		return nil
	})); err != nil {
		return err
	}

	klog.Info("[workload-robust-syncer] controller registered")
	return nil
}

func (r *WorkloadRobustSyncer) Reconcile(ctx context.Context, req ctrlruntime.Request) (ctrlruntime.Result, error) {
	wl := &v1.Workload{}
	if err := r.Get(ctx, req.NamespacedName, wl); err != nil {
		if client.IgnoreNotFound(err) == nil {
			return ctrlruntime.Result{}, nil
		}
		return ctrlruntime.Result{}, err
	}

	cluster := v1.GetClusterId(wl)
	if cluster == "" {
		return ctrlruntime.Result{}, nil
	}
	// Without a Robust client the periodic reconcileAll syncs the cluster once it gets one.
	if r.robustClient.ForCluster(cluster) == nil {
		klog.V(4).Infof("[workload-robust-syncer] cluster %s not available, skip workload %s", cluster, wl.Name)
		return ctrlruntime.Result{}, nil
	}
	if !r.syncEnabled(ctx, cluster) {
		return ctrlruntime.Result{}, nil
	}

	if !wl.GetDeletionTimestamp().IsZero() {
		return ctrlruntime.Result{}, r.syncDeleteToRobust(ctx, wl)
	}
	return ctrlruntime.Result{}, r.syncToRobust(ctx, wl)
}

// syncEnabled tells whether the workloads of cluster are synced to Robust.
func (r *WorkloadRobustSyncer) syncEnabled(ctx context.Context, cluster string) bool {
	c := &v1.Cluster{}
	if err := r.Get(ctx, client.ObjectKey{Name: cluster}, c); err != nil {
		return true
	}
	return c.Annotations[annotationRobustWorkloadSync] != "false"
}

func (r *WorkloadRobustSyncer) syncToRobust(ctx context.Context, wl *v1.Workload) error {
	cluster := v1.GetClusterId(wl)
	if cluster == "" {
		return nil
	}

	cc := r.robustClient.ForCluster(cluster)
	if cc == nil {
		klog.V(4).Infof("[workload-robust-syncer] cluster %s not available, skip workload %s", cluster, wl.Name)
		return nil
	}

	payload := buildWorkloadSyncPayload(wl)
	var resp robustclient.WorkloadSyncResponse
	err := cc.Post(ctx, "/api/v1/workloads/sync", payload, &resp)
	if err != nil {
		klog.Warningf("[workload-robust-syncer] sync failed, will retry: cluster=%s workload=%s err=%v",
			cluster, wl.Name, err)
		robustSyncErrors.WithLabelValues(cluster, "workload").Inc()
		return fmt.Errorf("sync workload %s to robust of cluster %s: %w", wl.Name, cluster, err)
	}
	robustSyncSuccess.WithLabelValues(cluster, "workload").Inc()
	return nil
}

func (r *WorkloadRobustSyncer) syncDeleteToRobust(ctx context.Context, wl *v1.Workload) error {
	cluster := v1.GetClusterId(wl)
	if cluster == "" {
		return nil
	}

	cc := r.robustClient.ForCluster(cluster)
	if cc == nil {
		return nil
	}

	err := cc.Delete(ctx, "/api/v1/workloads/sync/"+string(wl.UID), nil)
	if err != nil {
		klog.Warningf("[workload-robust-syncer] delete sync failed: cluster=%s workload=%s err=%v",
			cluster, wl.Name, err)
		robustSyncErrors.WithLabelValues(cluster, "workload_delete").Inc()
		return fmt.Errorf("delete workload %s from robust of cluster %s: %w", wl.Name, cluster, err)
	}
	robustSyncSuccess.WithLabelValues(cluster, "workload_delete").Inc()
	return nil
}

func (r *WorkloadRobustSyncer) startReconcileLoop(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.reconcileAll(ctx)
		}
	}
}

// reconcileAll reconciles every cluster that has a Robust client, in parallel.
func (r *WorkloadRobustSyncer) reconcileAll(ctx context.Context) {
	wlList := &v1.WorkloadList{}
	if err := r.List(ctx, wlList); err != nil {
		klog.Warningf("[workload-robust-syncer] reconcile list failed: %v", err)
		return
	}
	byCluster := make(map[string][]*v1.Workload)
	for i := range wlList.Items {
		wl := &wlList.Items[i]
		if cluster := v1.GetClusterId(wl); cluster != "" {
			byCluster[cluster] = append(byCluster[cluster], wl)
		}
	}

	var wg sync.WaitGroup
	for _, cluster := range r.robustClient.ClusterNames() {
		cc := r.robustClient.ForCluster(cluster)
		if cc == nil || !r.syncEnabled(ctx, cluster) {
			continue
		}
		wg.Add(1)
		go func(cc *robustclient.ClusterClient, workloads []*v1.Workload) {
			defer wg.Done()
			if err := r.reconcileCluster(ctx, cc, workloads); err != nil {
				klog.Warningf("[workload-robust-syncer] reconcile cluster %s failed: %v", cc.ClusterName(), err)
				robustSyncReconcileErrors.WithLabelValues(cc.ClusterName()).Inc()
			}
		}(cc, byCluster[cluster])
	}
	wg.Wait()
}

// reconcileCluster brings Robust's copy of one cluster's workloads in line with
// SaFE's: the workloads in etcd plus those the database holds as ended within the
// retention window. Against a Robust without the sync digest it pushes every
// workload that has not ended instead.
func (r *WorkloadRobustSyncer) reconcileCluster(ctx context.Context,
	cc *robustclient.ClusterClient, workloads []*v1.Workload) error {
	cluster := cc.ClusterName()
	digest, err := cc.GetSyncDigest(ctx)
	if err != nil {
		if robustclient.IsNotSupported(err) {
			robustSyncFallback.WithLabelValues(cluster).Inc()
			return r.catchUp(ctx, cc, workloads)
		}
		return fmt.Errorf("get sync digest: %w", err)
	}

	history, err := r.endedWorkloadsFromDB(ctx, cluster)
	if err != nil {
		return fmt.Errorf("list ended workloads: %w", err)
	}
	plan := planRobustSync(workloads, history, digest.Workloads)

	var errs []error
	if err = r.pushBatches(ctx, cc, plan.push, plan.reasons); err != nil {
		errs = append(errs, err)
	}

	kept, err := r.uidsInDB(ctx, plan.deleteCandidates)
	if err != nil {
		return errors.Join(append(errs, fmt.Errorf("look up workloads to delete: %w", err))...)
	}
	for _, uid := range plan.deleteCandidates {
		if kept[uid] {
			continue
		}
		if err = cc.Delete(ctx, "/api/v1/workloads/sync/"+uid, nil); err != nil {
			robustSyncErrors.WithLabelValues(cluster, "reconcile_delete").Inc()
			errs = append(errs, fmt.Errorf("delete %s: %w", uid, err))
			continue
		}
		robustSyncDeleted.WithLabelValues(cluster).Inc()
	}
	return errors.Join(errs...)
}

// catchUp pushes every workload of the cluster that has not ended, for a Robust
// that cannot tell what it holds.
func (r *WorkloadRobustSyncer) catchUp(ctx context.Context,
	cc *robustclient.ClusterClient, workloads []*v1.Workload) error {
	var payloads []robustclient.WorkloadSyncPayload
	for _, wl := range workloads {
		if !wl.IsEnd() {
			payloads = append(payloads, buildWorkloadSyncPayload(wl))
		}
	}
	return r.pushBatches(ctx, cc, payloads, nil)
}

// pushBatches sends payloads in batches. reasons, when set, runs parallel to
// payloads and labels the pushed counter.
func (r *WorkloadRobustSyncer) pushBatches(ctx context.Context, cc *robustclient.ClusterClient,
	payloads []robustclient.WorkloadSyncPayload, reasons []string) error {
	cluster := cc.ClusterName()
	var errs []error
	for start := 0; start < len(payloads); start += robustSyncBatchSize {
		end := min(start+robustSyncBatchSize, len(payloads))
		batch := robustclient.WorkloadSyncBatchPayload{Workloads: payloads[start:end]}
		var resp robustclient.WorkloadSyncBatchResponse
		if err := cc.Post(ctx, "/api/v1/workloads/sync/batch", batch, &resp); err != nil {
			klog.Warningf("[workload-robust-syncer] batch push failed: cluster=%s count=%d err=%v",
				cluster, end-start, err)
			robustSyncErrors.WithLabelValues(cluster, "catch_up").Inc()
			errs = append(errs, err)
			continue
		}
		if resp.Failed > 0 {
			robustSyncErrors.WithLabelValues(cluster, "batch_item").Add(float64(resp.Failed))
			klog.V(2).Infof("[workload-robust-syncer] batch push rejected items: cluster=%s synced=%d failed=%d",
				cluster, resp.Synced, resp.Failed)
		} else if resp.Synced > 0 {
			klog.V(4).Infof("[workload-robust-syncer] batch push: cluster=%s synced=%d failed=%d",
				cluster, resp.Synced, resp.Failed)
		}
		for i := start; i < end; i++ {
			reason := "catch_up"
			if reasons != nil {
				reason = reasons[i]
			}
			robustSyncPushed.WithLabelValues(cluster, reason).Inc()
		}
	}
	return errors.Join(errs...)
}

// endedWorkloadsFromDB returns the workloads of cluster that ended within the retention window.
func (r *WorkloadRobustSyncer) endedWorkloadsFromDB(ctx context.Context, cluster string) ([]*dbclient.Workload, error) {
	if r.dbClient == nil {
		return nil, nil
	}
	since := r.now().Add(-r.retention)
	var result []*dbclient.Workload
	for offset := 0; ; offset += robustSyncDBPageSize {
		page, err := r.dbClient.SelectWorkloadsEndedSince(ctx, cluster, since, robustSyncDBPageSize, offset)
		if err != nil {
			return nil, err
		}
		result = append(result, page...)
		if len(page) < robustSyncDBPageSize {
			return result, nil
		}
	}
}

// uidsInDB returns which of uids the database still has a row for.
func (r *WorkloadRobustSyncer) uidsInDB(ctx context.Context, uids []string) (map[string]bool, error) {
	found := make(map[string]bool)
	if r.dbClient == nil {
		return found, nil
	}
	for start := 0; start < len(uids); start += robustSyncBatchSize {
		end := min(start+robustSyncBatchSize, len(uids))
		rows, err := r.dbClient.SelectWorkloadsByUIDs(ctx, uids[start:end])
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			if row.WorkloadUId.Valid {
				found[row.WorkloadUId.String] = true
			}
		}
	}
	return found, nil
}

type robustSyncPlan struct {
	push []robustclient.WorkloadSyncPayload
	// reasons[i] says why push[i] is sent.
	reasons []string
	// deleteCandidates are uids Robust holds that neither etcd nor the
	// retention window has; they are deleted unless the database has them.
	deleteCandidates []string
}

func (p *robustSyncPlan) add(payload robustclient.WorkloadSyncPayload, reason string) {
	p.push = append(p.push, payload)
	p.reasons = append(p.reasons, reason)
}

// planRobustSync compares SaFE's workloads with what Robust holds and returns
// what to push and what may be deleted.
func planRobustSync(workloads []*v1.Workload, history []*dbclient.Workload,
	digest []robustclient.WorkloadSyncDigestEntry) robustSyncPlan {
	held := make(map[string]robustclient.WorkloadSyncDigestEntry, len(digest))
	for _, d := range digest {
		held[d.UID] = d
	}
	known := make(map[string]bool, len(workloads)+len(history))
	var plan robustSyncPlan

	for _, wl := range workloads {
		uid := string(wl.UID)
		if uid == "" {
			continue
		}
		known[uid] = true
		// Its deletion is sent by Reconcile.
		if !wl.GetDeletionTimestamp().IsZero() {
			continue
		}
		payload := buildWorkloadSyncPayload(wl)
		d, ok := held[uid]
		switch {
		case !ok || d.Deleted:
			plan.add(payload, robustPushMissing)
		case isNewerVersion(payload.ResourceVersion, d.SyncVersion):
			plan.add(payload, robustPushStale)
		case v1.IsWorkloadPhaseEnded(wl.Status.Phase) && !v1.IsWorkloadPhaseEnded(v1.WorkloadPhase(d.Phase)):
			plan.add(payload, robustPushEnded)
		}
	}

	for _, row := range history {
		if !row.WorkloadUId.Valid || row.WorkloadUId.String == "" || known[row.WorkloadUId.String] {
			continue
		}
		uid := row.WorkloadUId.String
		known[uid] = true
		d, ok := held[uid]
		switch {
		case !ok:
			plan.add(buildHistorySyncPayload(row, nextVersion("0")), robustPushMissing)
		case d.Deleted:
		case v1.IsWorkloadPhaseEnded(v1.WorkloadPhase(row.Phase.String)) &&
			!v1.IsWorkloadPhaseEnded(v1.WorkloadPhase(d.Phase)):
			plan.add(buildHistorySyncPayload(row, nextVersion(d.SyncVersion)), robustPushEnded)
		}
	}

	for _, d := range digest {
		if !d.Deleted && !known[d.UID] {
			plan.deleteCandidates = append(plan.deleteCandidates, d.UID)
		}
	}
	return plan
}

// isNewerVersion tells whether resource version a is newer than b. Versions
// that are not integers are only compared for equality.
func isNewerVersion(a, b string) bool {
	if a == "" {
		return false
	}
	na, errA := strconv.ParseUint(a, 10, 64)
	nb, errB := strconv.ParseUint(b, 10, 64)
	if errA == nil && errB == nil {
		return na > nb
	}
	return a != b
}

// nextVersion is the version a workload kept only in the database is pushed
// with: the database has no resourceVersion, and Robust drops a payload that
// is not newer than the version it holds. Robust also rejects an empty
// version, so a version that is not an integer gives "1".
func nextVersion(v string) string {
	n, err := strconv.ParseUint(v, 10, 64)
	if err != nil {
		return "1"
	}
	return strconv.FormatUint(n+1, 10)
}

func gpuRequestOf(resources []v1.WorkloadResource) int {
	gpuRequest := 0
	for _, res := range resources {
		if res.GPU != "" {
			if g, err := strconv.Atoi(res.GPU); err == nil {
				gpuRequest += g * res.Replica
			}
		}
	}
	return gpuRequest
}

func buildWorkloadSyncPayload(wl *v1.Workload) robustclient.WorkloadSyncPayload {
	payload := robustclient.WorkloadSyncPayload{
		UID:             string(wl.UID),
		Name:            wl.Name,
		Workspace:       wl.Spec.Workspace,
		Labels:          wl.Labels,
		Annotations:     wl.Annotations,
		Phase:           string(wl.Status.Phase),
		GPURequest:      gpuRequestOf(wl.Spec.Resources),
		ResourceVersion: wl.ResourceVersion,
	}

	if wl.CreationTimestamp.Time.Unix() > 0 {
		ts := wl.CreationTimestamp.Time.UTC().Format(time.RFC3339)
		payload.CreatedAt = &ts
	}
	if wl.Status.EndTime != nil {
		ts := wl.Status.EndTime.Time.UTC().Format(time.RFC3339)
		payload.EndAt = &ts
	}

	return payload
}

// buildHistorySyncPayload builds the payload of a workload kept only in the
// database, which has neither its labels nor its annotations.
func buildHistorySyncPayload(row *dbclient.Workload, resourceVersion string) robustclient.WorkloadSyncPayload {
	payload := robustclient.WorkloadSyncPayload{
		UID:             row.WorkloadUId.String,
		Name:            row.WorkloadId,
		Workspace:       row.Workspace,
		Phase:           row.Phase.String,
		ResourceVersion: resourceVersion,
	}
	if row.Resources.Valid {
		var resources []v1.WorkloadResource
		if err := json.Unmarshal([]byte(row.Resources.String), &resources); err == nil {
			payload.GPURequest = gpuRequestOf(resources)
		}
	}
	if row.CreationTime.Valid {
		ts := row.CreationTime.Time.UTC().Format(time.RFC3339)
		payload.CreatedAt = &ts
	}
	if row.EndTime.Valid {
		ts := row.EndTime.Time.UTC().Format(time.RFC3339)
		payload.EndAt = &ts
	}
	return payload
}
