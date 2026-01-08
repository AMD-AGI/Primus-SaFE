// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package api

import (
	"net/http"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/helper/cluster"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/helper/fault"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/helper/gpu"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/helper/metadata"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/helper/rdma"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/helper/storage"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model/rest"
	"github.com/gin-gonic/gin"
)

func getClusterOverview(c *gin.Context) {
	cm := clientsets.GetClusterManager()
	// Get cluster name from query parameter, priority: specified cluster > default cluster > current cluster
	clusterName := c.Query("cluster")
	clients, err := cm.GetClusterClientsOrDefault(clusterName)
	if err != nil {
		_ = c.Error(err)
		return
	}
	log.Infof("getClusterOverview: clusterName: %s", clients.ClusterName)
	// Try to get cached data first
	result, err := cluster.GetClusterOverviewFromCache(c, clients.ClusterName)
	if err == nil && result != nil {
		// Cache hit - return cached data immediately
		log.Infof("getClusterOverview: cache hit")
		c.JSON(http.StatusOK, rest.SuccessResp(c, result))
		return
	}
	log.Infof("getClusterOverview: cache miss")
	// Cache miss or error - fall back to real-time calculation
	// This ensures backward compatibility and handles the case where cache is not yet populated
	gpuNodes, err := gpu.GetGpuNodes(c, clients.K8SClientSet, metadata.GpuVendorAMD)
	if err != nil {
		_ = c.Error(err)
		return
	}
	faultyNodes, err := fault.GetFaultyNodes(c, clients.K8SClientSet, gpuNodes)
	if err != nil {
		_ = c.Error(err)
		return
	}
	idle, particalIdle, busy, err := gpu.GetGpuNodeIdleInfo(c, clients.K8SClientSet, clients.ClusterName, metadata.GpuVendorAMD)
	if err != nil {
		_ = c.Error(err)
		return
	}
	usage, err := gpu.CalculateGpuUsage(c, clients.StorageClientSet, metadata.GpuVendorAMD)
	if err != nil {
		_ = c.Error(err)
		return
	}
	allocationRate, err := gpu.GetClusterGpuAllocationRateFromDB(c, database.GetFacade().GetPod(), database.GetFacade().GetNode())
	if err != nil {
		_ = c.Error(err)
		return
	}
	storageStat, err := storage.GetStorageStat(c)
	if err != nil {
		_ = c.Error(err)
		return
	}
	rdmaStat, err := rdma.GetRdmaClusterStat(c, clients.StorageClientSet)
	if err != nil {
		_ = c.Error(err)
		return
	}
	result = &model.GpuClusterOverview{
		RdmaClusterStat:    rdmaStat,
		StorageStat:        *storageStat,
		TotalNodes:         len(gpuNodes),
		HealthyNodes:       len(gpuNodes) - len(faultyNodes),
		FaultyNodes:        len(faultyNodes),
		FullyIdleNodes:     idle,
		PartiallyIdleNodes: particalIdle,
		BusyNodes:          busy,
		AllocationRate:     allocationRate,
		Utilization:        usage,
	}
	c.JSON(http.StatusOK, rest.SuccessResp(c, result))
}

func getClusterGpuHeatmap(c *gin.Context) {
	k := 5
	cm := clientsets.GetClusterManager()
	// Get cluster name from query parameter, priority: specified cluster > default cluster > current cluster
	clusterName := c.Query("cluster")
	clients, err := cm.GetClusterClientsOrDefault(clusterName)
	if err != nil {
		_ = c.Error(err)
		return
	}
	storageClient := clients.StorageClientSet

	power, err := gpu.TopKGpuPowerInstant(c, k, storageClient)
	if err != nil {
		_ = c.Error(err)
		return
	}
	util, err := gpu.TopKGpuUtilizationInstant(c, k, storageClient)
	if err != nil {
		_ = c.Error(err)
		return
	}
	temp, err := gpu.TopKGpuTemperatureInstant(c, k, storageClient)
	if err != nil {
		_ = c.Error(err)
		return
	}

	c.JSON(http.StatusOK, rest.SuccessResp(c, struct {
		Power       model.Heatmap `json:"power"`
		Temperature model.Heatmap `json:"temperature"`
		Utilization model.Heatmap `json:"utilization"`
	}{
		Power: model.Heatmap{
			Serial:   2,
			Unit:     "W",
			YAxisMax: 850,
			YAxisMin: 0,
			Data:     power,
		},
		Temperature: model.Heatmap{
			Serial:   3,
			Unit:     "℃",
			YAxisMax: 110,
			YAxisMin: 20,
			Data:     temp,
		},
		Utilization: model.Heatmap{
			Serial:   1,
			Unit:     "%",
			YAxisMax: 100,
			YAxisMin: 0,
			Data:     util,
		},
	}))
}
