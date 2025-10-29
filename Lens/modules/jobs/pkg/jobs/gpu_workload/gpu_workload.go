package gpu_workload

import (
	"context"
	"sync"

	"github.com/AMD-AGI/primus-lens/core/pkg/clientsets"
	"github.com/AMD-AGI/primus-lens/core/pkg/database"
	dbModel "github.com/AMD-AGI/primus-lens/core/pkg/database/model"
	"github.com/AMD-AGI/primus-lens/core/pkg/helper/gpu"
	"github.com/AMD-AGI/primus-lens/core/pkg/helper/metadata"
	"github.com/AMD-AGI/primus-lens/core/pkg/helper/workload"
	"github.com/AMD-AGI/primus-lens/core/pkg/logger/log"
	"github.com/AMD-AGI/primus-lens/core/pkg/utils/k8sUtil"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type GpuWorkloadJob struct {
}

func (g *GpuWorkloadJob) Run(ctx context.Context, clientSets *clientsets.K8SClientSet, storageClientSet *clientsets.StorageClientSet) error {
	workloadsNotEnd, err := database.GetWorkloadNotEnd(ctx)
	if err != nil {
		return err
	}
	wg := &sync.WaitGroup{}
	for i := range workloadsNotEnd {
		workload := workloadsNotEnd[i]
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := g.checkWorkload(ctx, workload, clientSets)
			if err != nil {
				log.Errorf("Failed to check workload %s: %v", workload.Name, err)
			}
		}()
	}
	wg.Wait()
	return nil
}

func (g *GpuWorkloadJob) checkWorkload(ctx context.Context, dbWorkload *dbModel.GpuWorkload, clientSets *clientsets.K8SClientSet) error {
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
		podCount, err := g.fillCurrentGpuCount(ctx, dbWorkload, clientSets)
		if err != nil {
			return err
		}
		if podCount != 0 {
			dbWorkload.Status = metadata.WorkloadStatusRunning
		} else {
			dbWorkload.Status = metadata.WorkloadStatusDone
		}
	}

	err = database.UpdateGpuWorkload(ctx, dbWorkload)
	if err != nil {
		return err
	}
	return nil
}

func (g *GpuWorkloadJob) fillCurrentGpuCount(ctx context.Context, dbWorkload *dbModel.GpuWorkload, clientSets *clientsets.K8SClientSet) (podCount int, err error) {
	// Check all exist pods
	dBpods, err := workload.GetActivePodsByWorkloadUid(ctx, dbWorkload.UID)
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
