package cluster_overview

import (
	"context"
	"time"

	"github.com/AMD-AGI/primus-lens/core/pkg/clientsets"
	"github.com/AMD-AGI/primus-lens/core/pkg/database"
	dbmodel "github.com/AMD-AGI/primus-lens/core/pkg/database/model"
	"github.com/AMD-AGI/primus-lens/core/pkg/helper/fault"
	"github.com/AMD-AGI/primus-lens/core/pkg/helper/gpu"
	"github.com/AMD-AGI/primus-lens/core/pkg/helper/metadata"
	"github.com/AMD-AGI/primus-lens/core/pkg/helper/rdma"
	"github.com/AMD-AGI/primus-lens/core/pkg/helper/storage"
	"github.com/AMD-AGI/primus-lens/core/pkg/logger/log"
	coreModel "github.com/AMD-AGI/primus-lens/core/pkg/model"
)

type ClusterOverviewJob struct {
}

// Run executes the cluster overview caching job
func (j *ClusterOverviewJob) Run(ctx context.Context, clientSets *clientsets.K8SClientSet, storageClientSet *clientsets.StorageClientSet) error {
	// Use current cluster name for job running in current cluster
	clusterName := clientsets.GetClusterManager().GetCurrentClusterName()
	
	log.Infof("Starting cluster overview cache job for cluster: %s", clusterName)
	startTime := time.Now()
	
	// Initialize cache object
	cache := &dbmodel.ClusterOverviewCache{
		ClusterName: clusterName,
	}
	
	// 1. Get GPU nodes
	gpuNodes, err := gpu.GetGpuNodes(ctx, clientSets, metadata.GpuVendorAMD)
	if err != nil {
		log.Errorf("Failed to get GPU nodes: %v", err)
		return err
	}
	cache.TotalNodes = int32(len(gpuNodes))
	
	// 2. Get faulty nodes
	faultyNodes, err := fault.GetFaultyNodes(ctx, clientSets, gpuNodes)
	if err != nil {
		log.Errorf("Failed to get faulty nodes: %v", err)
		return err
	}
	cache.FaultyNodes = int32(len(faultyNodes))
	cache.HealthyNodes = cache.TotalNodes - cache.FaultyNodes
	
	// 3. Get GPU node idle info
	idle, partialIdle, busy, err := gpu.GetGpuNodeIdleInfo(ctx, clientSets, clusterName, metadata.GpuVendorAMD)
	if err != nil {
		log.Errorf("Failed to get GPU node idle info: %v", err)
		return err
	}
	cache.FullyIdleNodes = int32(idle)
	cache.PartiallyIdleNodes = int32(partialIdle)
	cache.BusyNodes = int32(busy)
	
	// 4. Calculate GPU usage
	usage, err := gpu.CalculateGpuUsage(ctx, storageClientSet, metadata.GpuVendorAMD)
	if err != nil {
		log.Errorf("Failed to calculate GPU usage: %v", err)
		// Don't fail the entire job for this error, just log it
		usage = 0
	}
	cache.Utilization = usage
	
	// 5. Get cluster GPU allocation rate
	allocationRate, err := gpu.GetClusterGpuAllocationRate(ctx, clientSets, clusterName, metadata.GpuVendorAMD)
	if err != nil {
		log.Errorf("Failed to get cluster GPU allocation rate: %v", err)
		return err
	}
	cache.AllocationRate = allocationRate
	
	// 6. Get storage statistics
	storageStat, err := storage.GetStorageStatWithClientSet(ctx, storageClientSet)
	if err != nil {
		log.Errorf("Failed to get storage statistics: %v", err)
		// Don't fail the entire job for this error, just log it
		storageStat = &coreModel.StorageStat{}
	}
	cache.StorageTotalSpace = storageStat.TotalSpace
	cache.StorageUsedSpace = storageStat.UsedSpace
	cache.StorageUsagePercentage = storageStat.UsagePercentage
	cache.StorageTotalInodes = storageStat.TotalInodes
	cache.StorageUsedInodes = storageStat.UsedInodes
	cache.StorageInodesUsagePercentage = storageStat.InodesUsagePercentage
	cache.StorageReadBandwidth = storageStat.ReadBandwidth
	cache.StorageWriteBandwidth = storageStat.WriteBandwidth
	
	// 7. Get RDMA cluster statistics
	rdmaStat, err := rdma.GetRdmaClusterStat(ctx, storageClientSet)
	if err != nil {
		log.Errorf("Failed to get RDMA cluster statistics: %v", err)
		// Don't fail the entire job for this error, just log it
		rdmaStat = coreModel.RdmaClusterStat{}
	}
	cache.RdmaTotalTx = rdmaStat.TotalTx
	cache.RdmaTotalRx = rdmaStat.TotalRx
	
	// 8. Save to database
	err = database.GetFacade().GetClusterOverviewCache().CreateOrUpdate(ctx, cache)
	if err != nil {
		log.Errorf("Failed to save cluster overview cache: %v", err)
		return err
	}
	
	duration := time.Since(startTime)
	log.Infof("Cluster overview cache job completed successfully for cluster: %s, took: %v", clusterName, duration)
	
	return nil
}

// Schedule returns the cron schedule for this job
func (j *ClusterOverviewJob) Schedule() string {
	// Run every 30 seconds - can be adjusted based on cluster size and requirements
	return "@every 30s"
}

