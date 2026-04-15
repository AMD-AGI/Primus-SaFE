/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resource

import (
	"context"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"k8s.io/klog/v2"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/robustclient"
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
)

// WorkloadRobustSyncer reconciles Workload CRDs and pushes sync events
// to data-plane robust-api instances. Sync failures are best-effort and
// do NOT block the Workload lifecycle.
type WorkloadRobustSyncer struct {
	client.Client
	robustClient *robustclient.Client
}

func SetupWorkloadRobustSyncer(mgr manager.Manager, rc *robustclient.Client) error {
	if rc == nil {
		klog.Info("[workload-robust-syncer] robust client not configured, skipping")
		return nil
	}

	r := &WorkloadRobustSyncer{
		Client:       mgr.GetClient(),
		robustClient: rc,
	}

	err := ctrlruntime.NewControllerManagedBy(mgr).
		Named("workload-robust-syncer").
		For(&v1.Workload{}).
		WithOptions(controller.Options{MaxConcurrentReconciles: 4}).
		Complete(r)
	if err != nil {
		return err
	}

	go r.startCatchUpSyncer(context.Background(), 30*time.Second)

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

	if !wl.GetDeletionTimestamp().IsZero() {
		r.syncDeleteToRobust(ctx, wl)
		return ctrlruntime.Result{}, nil
	}

	r.syncToRobust(ctx, wl)
	return ctrlruntime.Result{}, nil
}

func (r *WorkloadRobustSyncer) syncToRobust(ctx context.Context, wl *v1.Workload) {
	cluster := v1.GetClusterId(wl)
	if cluster == "" {
		return
	}

	cc := r.robustClient.ForCluster(cluster)
	if cc == nil {
		klog.V(4).Infof("[workload-robust-syncer] cluster %s not available, skip workload %s", cluster, wl.Name)
		return
	}

	payload := buildWorkloadSyncPayload(wl)
	var resp robustclient.WorkloadSyncResponse
	err := cc.Post(ctx, "/api/v1/workloads/sync", payload, &resp)
	if err != nil {
		klog.Warningf("[workload-robust-syncer] sync failed, will retry in catch-up: cluster=%s workload=%s err=%v",
			cluster, wl.Name, err)
		robustSyncErrors.WithLabelValues(cluster, "workload").Inc()
		return
	}
	robustSyncSuccess.WithLabelValues(cluster, "workload").Inc()
}

func (r *WorkloadRobustSyncer) syncDeleteToRobust(ctx context.Context, wl *v1.Workload) {
	cluster := v1.GetClusterId(wl)
	if cluster == "" {
		return
	}

	cc := r.robustClient.ForCluster(cluster)
	if cc == nil {
		return
	}

	err := cc.Delete(ctx, "/api/v1/workloads/sync/"+string(wl.UID), nil)
	if err != nil {
		klog.Warningf("[workload-robust-syncer] delete sync failed: cluster=%s workload=%s err=%v",
			cluster, wl.Name, err)
		robustSyncErrors.WithLabelValues(cluster, "workload_delete").Inc()
		return
	}
	robustSyncSuccess.WithLabelValues(cluster, "workload_delete").Inc()
}

func (r *WorkloadRobustSyncer) startCatchUpSyncer(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.runCatchUp(ctx)
		}
	}
}

func (r *WorkloadRobustSyncer) runCatchUp(ctx context.Context) {
	wlList := &v1.WorkloadList{}
	if err := r.List(ctx, wlList); err != nil {
		klog.Warningf("[workload-robust-syncer] catch-up list failed: %v", err)
		return
	}

	clusterBatches := make(map[string][]robustclient.WorkloadSyncPayload)
	for i := range wlList.Items {
		wl := &wlList.Items[i]
		if wl.IsEnd() {
			continue
		}
		cluster := v1.GetClusterId(wl)
		if cluster == "" {
			continue
		}
		clusterBatches[cluster] = append(clusterBatches[cluster], buildWorkloadSyncPayload(wl))
	}

	for cluster, payloads := range clusterBatches {
		cc := r.robustClient.ForCluster(cluster)
		if cc == nil {
			continue
		}

		batch := robustclient.WorkloadSyncBatchPayload{Workloads: payloads}
		var resp robustclient.WorkloadSyncBatchResponse
		err := cc.Post(ctx, "/api/v1/workloads/sync/batch", batch, &resp)
		if err != nil {
			klog.Warningf("[workload-robust-syncer] catch-up batch failed: cluster=%s count=%d err=%v",
				cluster, len(payloads), err)
			robustSyncErrors.WithLabelValues(cluster, "catch_up").Inc()
			continue
		}
		if resp.Synced > 0 {
			klog.V(4).Infof("[workload-robust-syncer] catch-up: cluster=%s synced=%d failed=%d",
				cluster, resp.Synced, resp.Failed)
		}
	}
}

func buildWorkloadSyncPayload(wl *v1.Workload) robustclient.WorkloadSyncPayload {
	gpuRequest := 0
	for _, res := range wl.Spec.Resources {
		if res.GPU != "" {
			if g, err := strconv.Atoi(res.GPU); err == nil {
				gpuRequest += g * res.Replica
			}
		}
	}

	payload := robustclient.WorkloadSyncPayload{
		UID:         string(wl.UID),
		Name:        wl.Name,
		Workspace:   wl.Spec.Workspace,
		Labels:      wl.Labels,
		Annotations: wl.Annotations,
		Phase:       string(wl.Status.Phase),
		GPURequest:  gpuRequest,
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
