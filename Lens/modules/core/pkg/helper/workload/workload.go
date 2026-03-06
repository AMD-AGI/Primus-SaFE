// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package workload

import (
	"context"
	"fmt"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	dbModel "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/errors"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/helper/prom"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model"
)

func GetRunningTopLevelGpuWorkloadByNode(ctx context.Context, clusterName string, nodeName string) ([]*dbModel.GpuWorkload, error) {
	pods, err := database.GetFacadeForCluster(clusterName).GetPod().GetActiveGpuPodByNodeName(ctx, nodeName)
	if err != nil {
		return nil, err
	}
	topLevelWorkloads, err := GetTopLevelWorkloadsByPods(ctx, clusterName, pods)
	if err != nil {
		return nil, err
	}
	return topLevelWorkloads, nil
}

func GetTopLevelWorkloadsByPods(ctx context.Context, clusterName string, pods []*dbModel.GpuPods) ([]*dbModel.GpuWorkload, error) {
	uids := []string{}
	for _, pod := range pods {
		uids = append(uids, pod.UID)
	}
	workloadReferences, err := database.GetFacadeForCluster(clusterName).GetWorkload().ListWorkloadPodReferencesByPodUids(ctx, uids)
	if err != nil {
		return nil, err
	}
	workloadUids := []string{}
	for _, workload := range workloadReferences {
		workloadUids = append(workloadUids, workload.WorkloadUID)
	}
	workloads, err := database.GetFacadeForCluster(clusterName).GetWorkload().ListTopLevelWorkloadByUids(ctx, workloadUids)
	if err != nil {
		return nil, err
	}
	return workloads, nil
}

func GetActivePodsByWorkloadUid(ctx context.Context, clusterName string, workloadUid string) ([]*dbModel.GpuPods, error) {
	refs, err := database.GetFacadeForCluster(clusterName).GetWorkload().ListWorkloadPodReferenceByWorkloadUid(ctx, workloadUid)
	if err != nil {
		return nil, err
	}
	uids := []string{}
	for _, ref := range refs {
		uids = append(uids, ref.PodUID)
	}
	activePods, err := database.GetFacadeForCluster(clusterName).GetPod().ListActivePodsByUids(ctx, uids)
	if err != nil {
		return nil, err
	}
	return activePods, nil
}

func GetWorkloadPods(ctx context.Context, clusterName string, workloadUid string) ([]*dbModel.GpuPods, error) {
	refs, err := database.GetFacadeForCluster(clusterName).GetWorkload().ListWorkloadPodReferenceByWorkloadUid(ctx, workloadUid)
	if err != nil {
		return nil, err
	}
	uids := []string{}
	for _, ref := range refs {
		uids = append(uids, ref.PodUID)
	}
	pods, err := database.GetFacadeForCluster(clusterName).GetPod().ListPodsByUids(ctx, uids)
	if err != nil {
		return nil, err
	}
	return pods, nil
}

func GetWorkloadResource(ctx context.Context, clusterName string, workloadUid string) (model.GpuAllocationInfo, error) {
	result := model.GpuAllocationInfo{}
	pods, err := GetWorkloadPods(ctx, clusterName, workloadUid)
	if err != nil {
		return result, err
	}
	for _, pod := range pods {
		podResource, err := database.GetFacadeForCluster(clusterName).GetPod().GetPodResourceByUid(ctx, pod.UID)
		if err != nil {

		}
		if podResource == nil {
			continue
		}
		if _, ok := result[podResource.GpuModel]; !ok {
			result[podResource.GpuModel] = 0
		}
		endTime := podResource.EndAt
		if endTime.Unix() < int64(8*time.Millisecond) {
			endTime = time.Now()
		}
		result[podResource.GpuModel] += endTime.Sub(podResource.CreatedAt).Seconds() * float64(podResource.GpuAllocated)
	}
	// Convert seconds to hours
	for gpuModel := range result {
		result[gpuModel] = result[gpuModel] / 3600
	}
	return result, nil
}

func GetCurrentWorkloadGpuUtilization(ctx context.Context, workloadUid string, clientSets *clientsets.StorageClientSet) (float64, error) {
	return prom.QueryPrometheusInstant(ctx, fmt.Sprintf(`avg(gpu_utilization{primus_lens_workload_uid="%s"})`, workloadUid), clientSets)
}

func GetWorkloadGpuAllocatedCount(ctx context.Context, clusterName string, workloadUid string) (int, error) {
	pods, err := GetWorkloadPods(ctx, clusterName, workloadUid)
	if err != nil {
		return 0, err
	}
	totalAllocated := 0
	for _, pod := range pods {
		podResource, err := database.GetFacadeForCluster(clusterName).GetPod().GetPodResourceByUid(ctx, pod.UID)
		if err != nil {
			return 0, err
		}
		if podResource == nil {
			continue
		}
		totalAllocated += int(podResource.GpuAllocated)
	}
	return totalAllocated, nil
}

// ResolveWorkloadCluster finds the cluster where a workload exists.
// It searches in the following order:
// 1. If cluster is explicitly specified, use it directly
// 2. Try the default cluster first (most common case)
// 3. Search all other clusters if not found in default
// Returns the cluster name where the workload was found, or error if not found anywhere.
func ResolveWorkloadCluster(ctx context.Context, uid string, requestedCluster string) (string, error) {
	cm := clientsets.GetClusterManager()

	if requestedCluster != "" {
		return requestedCluster, nil
	}

	defaultCluster := cm.GetDefaultClusterName()
	if defaultCluster != "" {
		w, err := database.GetFacadeForCluster(defaultCluster).GetWorkload().GetGpuWorkloadByUid(ctx, uid)
		if err == nil && w != nil {
			log.Debugf("[ResolveWorkloadCluster] Workload %s found in default cluster %s", uid, defaultCluster)
			return defaultCluster, nil
		}
	}

	allClients := cm.ListAllClientSets()
	for clusterName := range allClients {
		if clusterName == defaultCluster {
			continue
		}
		w, err := database.GetFacadeForCluster(clusterName).GetWorkload().GetGpuWorkloadByUid(ctx, uid)
		if err == nil && w != nil {
			log.Infof("[ResolveWorkloadCluster] Workload %s found in cluster %s (not in default cluster %s)",
				uid, clusterName, defaultCluster)
			return clusterName, nil
		}
	}

	return "", errors.NewError().
		WithCode(errors.RequestDataNotExisted).
		WithMessagef("workload %s not found in any cluster", uid)
}
