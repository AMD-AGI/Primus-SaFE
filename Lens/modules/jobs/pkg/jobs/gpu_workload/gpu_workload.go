package gpu_workload

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/AMD-AGI/primus-lens/core/pkg/clientsets"
	"github.com/AMD-AGI/primus-lens/core/pkg/database"
	dbModel "github.com/AMD-AGI/primus-lens/core/pkg/database/model"
	"github.com/AMD-AGI/primus-lens/core/pkg/helper/gpu"
	"github.com/AMD-AGI/primus-lens/core/pkg/helper/metadata"
	"github.com/AMD-AGI/primus-lens/core/pkg/helper/workload"
	"github.com/AMD-AGI/primus-lens/core/pkg/logger/log"
	"github.com/AMD-AGI/primus-lens/core/pkg/utils/k8sUtil"
	"github.com/AMD-AGI/primus-lens/jobs/pkg/common"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type GpuWorkloadJob struct {
}

func (g *GpuWorkloadJob) Run(ctx context.Context, clientSets *clientsets.K8SClientSet, storageClientSet *clientsets.StorageClientSet) (*common.ExecutionStats, error) {
	stats := common.NewExecutionStats()
	
	// Use current cluster name for job running in current cluster
	clusterName := clientsets.GetClusterManager().GetCurrentClusterName()
	workloadsNotEnd, err := database.GetFacadeForCluster(clusterName).GetWorkload().GetWorkloadNotEnd(ctx)
	if err != nil {
		return stats, err
	}
	
	wg := &sync.WaitGroup{}
	for i := range workloadsNotEnd {
		workload := workloadsNotEnd[i]
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := g.checkWorkload(ctx, clusterName, workload, clientSets)
			if err != nil {
				atomic.AddInt64(&stats.ErrorCount, 1)
				log.Errorf("Failed to check workload %s: %v", workload.Name, err)
			} else {
				atomic.AddInt64(&stats.ItemsUpdated, 1)
			}
		}()
	}
	wg.Wait()
	
	stats.RecordsProcessed = int64(len(workloadsNotEnd))
	stats.AddCustomMetric("workloads_count", len(workloadsNotEnd))
	stats.AddMessage("GPU workload status updated successfully")
	
	return stats, nil
}

func (g *GpuWorkloadJob) checkWorkload(ctx context.Context, clusterName string, dbWorkload *dbModel.GpuWorkload, clientSets *clientsets.K8SClientSet) error {
	// Check weather already end.
	_, err := k8sUtil.GetObjectByGvk(ctx, dbWorkload.GroupVersion, dbWorkload.Kind, dbWorkload.Namespace, dbWorkload.Name, clientSets.ControllerRuntimeClient)
	if err != nil {
		if client.IgnoreNotFound(err) != nil {
			return err
		}
		dbWorkload.Status = metadata.WorkloadStatusDeleted
		dbWorkload.EndAt = dbWorkload.UpdatedAt
	}
	if dbWorkload.Status != metadata.WorkloadStatusDeleted {
		podCount, err := g.fillCurrentGpuCount(ctx, clusterName, dbWorkload, clientSets)
		if err != nil {
			return err
		}
		if podCount != 0 {
			dbWorkload.Status = metadata.WorkloadStatusRunning
		} else {
			dbWorkload.Status = metadata.WorkloadStatusDone
		}
	}

	err = database.GetFacadeForCluster(clusterName).GetWorkload().UpdateGpuWorkload(ctx, dbWorkload)
	if err != nil {
		return err
	}
	return nil
}

func (g *GpuWorkloadJob) fillCurrentGpuCount(ctx context.Context, clusterName string, dbWorkload *dbModel.GpuWorkload, clientSets *clientsets.K8SClientSet) (podCount int, err error) {
	// Check all exist pods
	dBpods, err := workload.GetActivePodsByWorkloadUid(ctx, clusterName, dbWorkload.UID)
	if err != nil {
		return 0, err
	}
	if len(dBpods) == 0 {
		return 0, nil
	}
	pods := []corev1.Pod{}
	for i := range dBpods {
		dbPod := dBpods[i]
		pod := &corev1.Pod{}
		err = clientSets.ControllerRuntimeClient.Get(ctx, types.NamespacedName{Namespace: dbPod.Namespace, Name: dbPod.Name}, pod)
		if err != nil {
			if client.IgnoreNotFound(err) != nil {
				return 0, err
			}
			continue
		}
		pods = append(pods, *pod)
	}
	podCount = len(pods)
	allocated := gpu.GetAllocatedGpuResourceFromPods(ctx, pods, metadata.GetResourceName(metadata.GpuVendorAMD))
	dbWorkload.GpuRequest = int32(allocated)
	return podCount, nil
}

func (g *GpuWorkloadJob) Schedule() string {
	return "@every 20s"
}
