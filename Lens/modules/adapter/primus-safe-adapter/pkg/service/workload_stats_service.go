/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package service

import (
	"context"
	"time"

	"gorm.io/gorm"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	lensmodel "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	primusSafeV1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	safedal "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/dal"
	safemodel "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/model"
)

const (
	// StatisticType3H represents the 3-hour statistics type
	StatisticType3H = "3h"

	// ThreeHours represents a 3-hour time period
	ThreeHours = 3 * time.Hour
)

// WorkloadStatsService provides workload statistics collection service
type WorkloadStatsService struct {
	k8sClient client.Client
	safeDB    *gorm.DB
}

// NewWorkloadStatsService creates a new workload statistics service
func NewWorkloadStatsService(k8sClient client.Client, safeDB *gorm.DB) *WorkloadStatsService {
	return &WorkloadStatsService{
		k8sClient: k8sClient,
		safeDB:    safeDB,
	}
}

// Name returns the task name
func (s *WorkloadStatsService) Name() string {
	return "workload-stats-collector"
}

// Run executes the workload statistics collection task
func (s *WorkloadStatsService) Run(ctx context.Context) error {
	log.Info("Starting workload stats collection")

	// Get currently running workloads
	workloads, err := s.getRunningWorkloads(ctx)
	if err != nil {
		log.Errorf("Failed to get running workloads: %v", err)
		return err
	}

	if len(workloads) == 0 {
		log.Info("No running workloads found")
		return nil
	}

	// Process statistics data for each workload
	successCount := 0
	failCount := 0
	for _, workload := range workloads {
		if err := s.processWorkloadStats(ctx, &workload); err != nil {
			log.Errorf("Failed to process workload %s/%s: %v",
				workload.Spec.Workspace, workload.Name, err)
			failCount++
		} else {
			successCount++
		}
	}

	return nil
}

// getRunningWorkloads retrieves currently running workloads
func (s *WorkloadStatsService) getRunningWorkloads(ctx context.Context) ([]primusSafeV1.Workload, error) {
	workloadList := &primusSafeV1.WorkloadList{}

	// Query all workloads
	err := s.k8sClient.List(ctx, workloadList)
	if err != nil {
		return nil, err
	}

	// Filter out running workloads
	runningWorkloads := make([]primusSafeV1.Workload, 0, len(workloadList.Items))
	for _, workload := range workloadList.Items {
		// Check if the workload is in running state
		if s.isWorkloadRunning(&workload) {
			runningWorkloads = append(runningWorkloads, workload)
		}
	}

	return runningWorkloads, nil
}

// isWorkloadRunning determines whether a workload is currently running
func (s *WorkloadStatsService) isWorkloadRunning(workload *primusSafeV1.Workload) bool {
	if workload.Status.Phase == "" {
		return false
	}

	// Running states include: Running, Pending (about to run)
	phase := workload.Status.Phase
	return phase == primusSafeV1.WorkloadRunning ||
		phase == primusSafeV1.WorkloadPending
}

// processWorkloadStats processes statistics data for a single workload
func (s *WorkloadStatsService) processWorkloadStats(ctx context.Context, workload *primusSafeV1.Workload) error {
	// Get cluster ID from workload
	clusterID := primusSafeV1.GetClusterId(workload)
	if clusterID == "" {
		log.Warnf("Workload %s/%s has no cluster ID, using default cluster",
			workload.Spec.Workspace, workload.Name)
		clusterID = "default"
	}

	// Get lens facade for the specific cluster
	lensFacade := database.GetFacade().WithCluster(clusterID)

	// Calculate the time point 3 hours ago
	endTime := time.Now()
	startTime := endTime.Add(-ThreeHours)

	// Get data from the last 3 hours from workload_gpu_hourly_stats table
	hourlyStats, err := lensFacade.GetGpuAggregation().ListWorkloadHourlyStatsByNamespace(
		ctx,
		workload.Spec.Workspace,
		startTime,
		endTime,
	)
	if err != nil {
		log.Errorf("Failed to get hourly stats from cluster %s for workload %s/%s: %v",
			clusterID, workload.Spec.Workspace, workload.Name, err)
		return err
	}

	// Filter data for the current workload
	var workloadStats []*lensmodel.WorkloadGpuHourlyStats
	for _, stat := range hourlyStats {
		if stat.WorkloadName == workload.Name {
			workloadStats = append(workloadStats, stat)
		}
	}

	if len(workloadStats) == 0 {
		return nil
	}

	// Calculate average and maximum values
	avgUtilization, maxUtilization := s.calculateUtilization(workloadStats)

	// Build statistics record
	statistic := &safemodel.WorkloadStatistic{
		WorkloadID:      string(workload.Name),
		WorkloadUID:     string(workload.UID),
		Cluster:         clusterID,
		Workspace:       workload.Spec.Workspace,
		StatisticType:   StatisticType3H,
		AvgGpuUsage3H:   avgUtilization,
		MaxGpuUsage3H:   maxUtilization,
		DataPointsCount: int32(len(workloadStats)),
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}

	// Save or update to workload_statistic table
	return s.upsertWorkloadStatistic(ctx, statistic)
}

// calculateUtilization calculates the average and maximum GPU utilization
func (s *WorkloadStatsService) calculateUtilization(stats []*lensmodel.WorkloadGpuHourlyStats) (avg, max float64) {
	if len(stats) == 0 {
		return 0, 0
	}

	sum := 0.0
	max = 0.0

	for _, stat := range stats {
		sum += stat.AvgUtilization
		if stat.MaxUtilization > max {
			max = stat.MaxUtilization
		}
	}

	avg = sum / float64(len(stats))
	return avg, max
}

// upsertWorkloadStatistic inserts or updates workload statistics data
func (s *WorkloadStatsService) upsertWorkloadStatistic(ctx context.Context, statistic *safemodel.WorkloadStatistic) error {
	dal := safedal.Use(s.safeDB)
	ws := dal.WorkloadStatistic

	// Find existing record
	existing, err := ws.WithContext(ctx).
		Where(ws.WorkloadID.Eq(statistic.WorkloadID)).
		Where(ws.StatisticType.Eq(statistic.StatisticType)).
		First()

	if err != nil && err != gorm.ErrRecordNotFound {
		return err
	}

	if existing != nil {
		// Update existing record
		statistic.ID = existing.ID
		statistic.CreatedAt = existing.CreatedAt
		err = ws.WithContext(ctx).
			Save(statistic)
		return err
	}

	// Create new record
	return ws.WithContext(ctx).Create(statistic)
}
