/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resource

import (
	"context"
	"net/url"
	"time"

	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/robustclient"
)

// StatsRobustSyncer periodically pulls workload and node statistics from
// each data-plane robust-api and writes them to SaFE management DB.
type StatsRobustSyncer struct {
	robustClient *robustclient.Client
	dbWriter     StatsDBWriter
}

// StatsDBWriter abstracts writes to the SaFE management DB.
// Implemented by the SaFE database client.
type StatsDBWriter interface {
	UpsertWorkloadStatistic(ctx context.Context, cluster string, stats []WorkloadHourlyStats) error
	UpsertNodeStatistic(ctx context.Context, cluster string, stats []NodeStats) error
}

type WorkloadHourlyStats struct {
	WorkloadUID string    `json:"workload_uid"`
	Workspace   string    `json:"workspace"`
	GPUHours    float64   `json:"gpu_hours"`
	Timestamp   time.Time `json:"timestamp"`
}

type NodeStats struct {
	NodeName   string  `json:"node_name"`
	GPUCount   int     `json:"gpu_count"`
	GPUUsed    int     `json:"gpu_used"`
	Allocation float64 `json:"allocation_rate"`
}

func SetupStatsRobustSyncer(mgr manager.Manager, rc *robustclient.Client, dbWriter StatsDBWriter) error {
	if rc == nil || dbWriter == nil {
		klog.Info("[stats-robust-syncer] robust client or db writer not configured, skipping")
		return nil
	}

	s := &StatsRobustSyncer{
		robustClient: rc,
		dbWriter:     dbWriter,
	}

	go s.startWorkloadStatsLoop(context.Background(), 30*time.Second)
	go s.startNodeStatsLoop(context.Background(), 60*time.Second)

	klog.Info("[stats-robust-syncer] started")
	return nil
}

func (s *StatsRobustSyncer) startWorkloadStatsLoop(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.syncWorkloadStats(ctx)
		}
	}
}

func (s *StatsRobustSyncer) startNodeStatsLoop(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.syncNodeStats(ctx)
		}
	}
}

func (s *StatsRobustSyncer) syncWorkloadStats(ctx context.Context) {
	for _, clusterName := range s.robustClient.ClusterNames() {
		cc := s.robustClient.ForCluster(clusterName)
		if cc == nil {
			continue
		}

		var stats []WorkloadHourlyStats
		params := url.Values{}
		params.Set("period", "1h")
		err := cc.Get(ctx, "/api/v1/gpu-aggregation/workloads/hourly-stats", params, &stats)
		if err != nil {
			klog.V(4).Infof("[stats-robust-syncer] workload stats failed: cluster=%s err=%v", clusterName, err)
			robustSyncErrors.WithLabelValues(clusterName, "workload_stats").Inc()
			continue
		}

		if len(stats) > 0 {
			if err := s.dbWriter.UpsertWorkloadStatistic(ctx, clusterName, stats); err != nil {
				klog.Warningf("[stats-robust-syncer] workload stats DB write failed: cluster=%s err=%v", clusterName, err)
			}
		}
	}
}

func (s *StatsRobustSyncer) syncNodeStats(ctx context.Context) {
	for _, clusterName := range s.robustClient.ClusterNames() {
		cc := s.robustClient.ForCluster(clusterName)
		if cc == nil {
			continue
		}

		var stats []NodeStats
		err := cc.Get(ctx, "/api/v1/nodes", nil, &stats)
		if err != nil {
			klog.V(4).Infof("[stats-robust-syncer] node stats failed: cluster=%s err=%v", clusterName, err)
			robustSyncErrors.WithLabelValues(clusterName, "node_stats").Inc()
			continue
		}

		if len(stats) > 0 {
			if err := s.dbWriter.UpsertNodeStatistic(ctx, clusterName, stats); err != nil {
				klog.Warningf("[stats-robust-syncer] node stats DB write failed: cluster=%s err=%v", clusterName, err)
			}
		}
	}
}
