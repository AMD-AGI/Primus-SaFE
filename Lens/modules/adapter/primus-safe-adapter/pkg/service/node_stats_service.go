/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package service

import (
	"context"
	"time"

	"gorm.io/gorm"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	safedal "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/dal"
	safemodel "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/model"
)

// NodeStatsService provides node GPU utilization statistics collection service
type NodeStatsService struct {
	safeDB *gorm.DB
}

// NewNodeStatsService creates a new node statistics service
func NewNodeStatsService(safeDB *gorm.DB) *NodeStatsService {
	return &NodeStatsService{
		safeDB: safeDB,
	}
}

// Name returns the task name
func (s *NodeStatsService) Name() string {
	return "node-stats-collector"
}

// Run executes the node statistics collection task
func (s *NodeStatsService) Run(ctx context.Context) error {
	startTime := time.Now()
	log.Info("Starting node stats collection for all clusters")

	clusterManager := clientsets.GetClusterManager()

	// Get all cluster names
	clusterNames := clusterManager.GetClusterNames()
	if len(clusterNames) == 0 {
		log.Warn("No clusters found")
		return nil
	}

	log.Infof("Found %d cluster(s) to process", len(clusterNames))

	// Process each cluster
	totalSuccessCount := 0
	totalFailCount := 0
	clustersProcessed := 0
	clustersFailed := 0

	for _, clusterName := range clusterNames {
		if clusterName == "default"{
			continue
		}
		log.Infof("Processing cluster: %s", clusterName)

		successCount, failCount, err := s.processCluster(ctx, clusterName)
		if err != nil {
			log.Errorf("Failed to process cluster %s: %v", clusterName, err)
			clustersFailed++
			continue
		}

		totalSuccessCount += successCount
		totalFailCount += failCount
		clustersProcessed++

		log.Infof("Cluster %s processed: success=%d, failed=%d", clusterName, successCount, failCount)
	}

	duration := time.Since(startTime)
	log.Infof("Node stats collection completed: clusters=%d/%d, total_nodes_success=%d, total_nodes_failed=%d, duration=%v",
		clustersProcessed, len(clusterNames), totalSuccessCount, totalFailCount, duration)

	return nil
}

// processCluster processes a single cluster's node statistics
func (s *NodeStatsService) processCluster(ctx context.Context, clusterName string) (successCount int, failCount int, err error) {
	// Get nodes from the specific cluster's Lens database
	nodes, err := database.GetFacadeForCluster(clusterName).GetNode().ListGpuNodes(ctx)
	if err != nil {
		return 0, 0, err
	}

	if len(nodes) == 0 {
		log.Infof("No GPU nodes found in cluster: %s", clusterName)
		return 0, 0, nil
	}

	log.Infof("Found %d GPU nodes in cluster: %s", len(nodes), clusterName)

	// Process each node and save to SaFE node_statistic table
	successCount = 0
	failCount = 0
	for _, node := range nodes {
		if err := s.saveNodeStatistic(ctx, clusterName, node.Name, node.GpuUtilization); err != nil {
			log.Errorf("Failed to save node statistic for node %s in cluster %s: %v", node.Name, clusterName, err)
			failCount++
		} else {
			successCount++
		}
	}

	return successCount, failCount, nil
}

// saveNodeStatistic saves or updates node GPU utilization statistic in SaFE database
func (s *NodeStatsService) saveNodeStatistic(ctx context.Context, cluster, nodeName string, gpuUtilization float64) error {
	dal := safedal.Use(s.safeDB)
	ns := dal.NodeStatistic

	// Check if record already exists
	existing, err := ns.WithContext(ctx).
		Where(ns.Cluster.Eq(cluster)).
		Where(ns.NodeName.Eq(nodeName)).
		First()

	now := time.Now()

	if err != nil && err != gorm.ErrRecordNotFound {
		return err
	}

	if existing != nil {
		// Update existing record
		existing.GpuUtilization = gpuUtilization
		existing.UpdatedAt = now
		err = ns.WithContext(ctx).Save(existing)
		if err != nil {
			return err
		}
		log.Debugf("Updated node statistic for node %s: gpu_utilization=%.2f", nodeName, gpuUtilization)
	} else {
		// Create new record
		statistic := &safemodel.NodeStatistic{
			Cluster:        cluster,
			NodeName:       nodeName,
			GpuUtilization: gpuUtilization,
			CreatedAt:      now,
			UpdatedAt:      now,
		}
		err = ns.WithContext(ctx).Create(statistic)
		if err != nil {
			return err
		}
		log.Debugf("Created node statistic for node %s: gpu_utilization=%.2f", nodeName, gpuUtilization)
	}

	return nil
}
