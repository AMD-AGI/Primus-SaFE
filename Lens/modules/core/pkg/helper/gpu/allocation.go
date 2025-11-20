package gpu

import (
	"context"
	"fmt"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/constant"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	dbmodel "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/helper/kubelet"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/helper/metadata"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model"
	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	promModel "github.com/prometheus/common/model"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func GetClusterGpuAllocationRate(ctx context.Context, clientsets *clientsets.K8SClientSet, clusterName string, vendor metadata.GpuVendor) (float64, error) {
	nodes, err := GetGpuNodes(ctx, clientsets, vendor)
	if err != nil {
		return 0, err
	}
	totalCapacity := 0.0
	totalAllocated := 0.0
	for _, node := range nodes {
		allocated, capacity, err := GetNodeGpuAllocation(ctx, clientsets, node, clusterName, vendor)
		if err != nil {
			return 0, err
		}
		totalCapacity += float64(capacity)
		totalAllocated += float64(allocated)
	}
	if totalCapacity == 0 {
		return 0, nil
	}
	return totalAllocated / totalCapacity * 100, nil
}

// GetClusterGpuAllocationRateFromDB calculates cluster GPU allocation rate from database
// It queries GPU nodes and active GPU pods from database and calculates the overall allocation rate
func GetClusterGpuAllocationRateFromDB(ctx context.Context, podFacade database.PodFacadeInterface, nodeFacade database.NodeFacadeInterface) (float64, error) {
	// Get all GPU nodes from database
	nodes, err := nodeFacade.ListGpuNodes(ctx)
	if err != nil {
		return 0, err
	}

	totalCapacity := 0.0
	totalAllocated := 0.0

	for _, node := range nodes {
		capacity := int(node.GpuCount)
		if capacity == 0 {
			continue
		}

		// Skip nodes with taints
		if hasTaints(node.Taints) {
			continue
		}

		// Get active GPU pods from database
		gpuPods, err := podFacade.GetActiveGpuPodByNodeName(ctx, node.Name)
		if err != nil {
			log.Errorf("GetActiveGpuPodByNodeName for node %s failed: %v", node.Name, err)
			return 0, err
		}

		// Calculate total allocated GPUs from pods
		allocated := 0
		for _, pod := range gpuPods {
			allocated += int(pod.GpuAllocated)
		}

		totalCapacity += float64(capacity)
		totalAllocated += float64(allocated)
	}

	if totalCapacity == 0 {
		return 0, nil
	}

	return totalAllocated / totalCapacity * 100, nil
}

func GetNodeGpuAllocation(ctx context.Context, clientSets *clientsets.K8SClientSet, nodeName string, clusterName string, vendor metadata.GpuVendor) (int, int, error) {
	node := &corev1.Node{}
	err := clientSets.ControllerRuntimeClient.Get(ctx, types.NamespacedName{Name: nodeName}, node)
	if err != nil {
		return 0, 0, err
	}
	allocated, err := GetAllocatedGpuResources(ctx, node, clusterName, vendor)
	if err != nil {
		return 0, 0, err
	}
	return allocated, GetGpuCapacity(node, vendor), nil

}

func GetAllocatedGpuResources(ctx context.Context, node *corev1.Node, clusterName string, vendor metadata.GpuVendor) (int, error) {
	gpuResource := metadata.GetResourceName(vendor)
	gpuPods, err := kubelet.GetGpuPods(ctx, node, clusterName, vendor)
	if err != nil {
		return 0, err
	}
	return GetAllocatedGpuResourceFromPods(ctx, gpuPods, gpuResource), nil
}

func GetGpuCapacity(node *corev1.Node, vendor metadata.GpuVendor) int {
	gpuResource := metadata.GetResourceName(vendor)
	capacity, _ := node.Status.Capacity.Name(corev1.ResourceName(gpuResource), resource.BinarySI).AsInt64()
	return int(capacity)
}

func GetAllocatedGpuResourceFromPods(ctx context.Context, pods []corev1.Pod, gpuResource string) int {
	allocated := 0

	for _, pod := range pods {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		allocated += GetAllocatedGpuResourceFromPod(&pod, gpuResource)
	}
	return allocated
}

func GetAllocatedGpuResourceFromPod(pod *corev1.Pod, gpuResource string) int {
	allocated := 0
	for _, container := range pod.Spec.Containers {
		if quantity, ok := container.Resources.Requests[corev1.ResourceName(gpuResource)]; ok {
			val := int(quantity.Value())
			allocated += val
		}
	}
	return allocated
}

func GetGpuNodes(ctx context.Context, clientsets *clientsets.K8SClientSet, vendor metadata.GpuVendor) ([]string, error) {
	nodeList := &corev1.NodeList{}
	err := clientsets.ControllerRuntimeClient.List(ctx, nodeList, client.HasLabels{metadata.GetNodeFilter(vendor)})
	if err != nil {
		return nil, err
	}
	result := make([]string, 0, len(nodeList.Items))
	for _, node := range nodeList.Items {
		result = append(result, node.Name)
	}
	return result, nil
}

func GetGpuNodesAllocation(ctx context.Context, clientsets *clientsets.K8SClientSet, clusterName string, vendor metadata.GpuVendor) ([]model.GpuAllocation, error) {
	nodes, err := GetGpuNodes(ctx, clientsets, vendor)
	if err != nil {
		return nil, err
	}
	results := make([]model.GpuAllocation, 0, len(nodes))
	for _, node := range nodes {
		allocated, capacity, err := GetNodeGpuAllocation(ctx, clientsets, node, clusterName, vendor)
		if err != nil {
			return nil, err
		}
		if capacity == 0 {
			continue
		}
		results = append(results, model.GpuAllocation{
			Node:           node,
			Vendor:         string(vendor),
			Capacity:       capacity,
			Allocated:      allocated,
			AllocationRate: float64(allocated) / float64(capacity),
		})
	}
	return results, nil
}

func GetHistoryGpuAllocationRate(ctx context.Context, clientsets *clientsets.StorageClientSet, vendor metadata.GpuVendor, start, end time.Time, step int) ([]model.TimePoint, error) {
	promClient := clientsets.PrometheusRead
	if promClient == nil {
		return nil, fmt.Errorf("Prometheus client is not initialized")
	}

	promAPI := v1.NewAPI(promClient)

	query := "avg(gpu_allocation_rate)"

	rangeQuery := v1.Range{
		Start: start,
		End:   end,
		Step:  time.Duration(step) * time.Second,
	}

	result, warnings, err := promAPI.QueryRange(ctx, query, rangeQuery)
	if err != nil {
		return nil, fmt.Errorf("prometheus query range failed: %w", err)
	}
	if len(warnings) > 0 {
		fmt.Printf("Prometheus query range warnings: %v\n", warnings)
	}

	matrixVal, ok := result.(promModel.Matrix)
	if !ok || len(matrixVal) == 0 {
		log.Warnf("No data returned for metric %s", query)
		return []model.TimePoint{}, nil
	}

	var timeSeries []model.TimePoint
	for _, stream := range matrixVal {
		for _, point := range stream.Values {
			timeSeries = append(timeSeries, model.TimePoint{
				Timestamp: point.Timestamp.Unix(),
				Value:     float64(point.Value),
			})
		}
	}

	return timeSeries, nil
}

func GetNodeGpuVramUsageHistory(ctx context.Context, clientsets *clientsets.StorageClientSet, vendor metadata.GpuVendor, start, end time.Time, step int) ([]model.TimePoint, error) {
	promClient := clientsets.PrometheusRead
	if promClient == nil {
		return nil, fmt.Errorf("Prometheus client is not initialized")
	}

	promAPI := v1.NewAPI(promClient)

	query := fmt.Sprintf(`avg by (%s) (gpu_used_vram/gpu_total_vram)`, constant.PrimusLensNodeLabelName)

	rangeQuery := v1.Range{
		Start: start,
		End:   end,
		Step:  time.Duration(step) * time.Second,
	}

	result, warnings, err := promAPI.QueryRange(ctx, query, rangeQuery)
	if err != nil {
		return nil, fmt.Errorf("prometheus query range failed: %w", err)
	}
	if len(warnings) > 0 {
		fmt.Printf("Prometheus query range warnings: %v\n", warnings)
	}

	matrixVal, ok := result.(promModel.Matrix)
	if !ok || len(matrixVal) == 0 {
		log.Warnf("No data returned for metric %s", query)
		return []model.TimePoint{}, nil
	}

	var timeSeries []model.TimePoint
	for _, stream := range matrixVal {
		for _, point := range stream.Values {
			timeSeries = append(timeSeries, model.TimePoint{
				Timestamp: point.Timestamp.Unix(),
				Value:     float64(point.Value),
			})
		}
	}

	return timeSeries, nil
}

func GetGpuNodeIdleInfo(ctx context.Context, clientsets *clientsets.K8SClientSet, clusterName string, vendor metadata.GpuVendor) (fullyIdle int, partiallyIdle int, busy int, err error) {
	nodeNames, err := GetGpuNodes(ctx, clientsets, vendor)
	if err != nil {
		return 0, 0, 0, err
	}
	for _, name := range nodeNames {
		allocated, capacity, err := GetNodeGpuAllocation(ctx, clientsets, name, clusterName, vendor)
		if err != nil {
			log.Errorf("GetNodeGpuAllocation for node %s failed: %v", name, err)
			return 0, 0, 0, err
		}
		if capacity == 0 {
			continue
		}
		if allocated == capacity {
			busy++
			continue
		}
		if allocated == 0 {
			fullyIdle++
			continue
		}
		partiallyIdle++
	}
	return fullyIdle, partiallyIdle, busy, nil
}

// GetGpuNodeIdleInfoFromDB gets GPU node idle information from database
// It queries active GPU pods from database and calculates allocation based on node capacity
func GetGpuNodeIdleInfoFromDB(ctx context.Context, podFacade database.PodFacadeInterface, nodeFacade database.NodeFacadeInterface) (fullyIdle int, partiallyIdle int, busy int, err error) {
	// Get all GPU nodes from database
	nodes, err := nodeFacade.ListGpuNodes(ctx)
	if err != nil {
		return 0, 0, 0, err
	}

	for _, node := range nodes {
		capacity := int(node.GpuCount)
		if capacity == 0 {
			continue
		}

		// Skip nodes with taints
		if hasTaints(node.Taints) {
			continue
		}

		// Get active GPU pods from database
		gpuPods, err := podFacade.GetActiveGpuPodByNodeName(ctx, node.Name)
		if err != nil {
			log.Errorf("GetActiveGpuPodByNodeName for node %s failed: %v", node.Name, err)
			return 0, 0, 0, err
		}

		// Calculate total allocated GPUs from pods
		allocated := 0
		for _, pod := range gpuPods {
			allocated += int(pod.GpuAllocated)
		}

		// Classify node based on allocation
		if allocated == capacity {
			busy++
			continue
		}
		if allocated == 0 {
			fullyIdle++
			continue
		}
		partiallyIdle++
	}

	return fullyIdle, partiallyIdle, busy, nil
}

// hasTaints checks if a node has any taints
func hasTaints(taints dbmodel.ExtType) bool {
	if taints == nil {
		return false
	}

	// Check if "taints" key exists and has values
	if taintsVal, ok := taints["taints"]; ok {
		// Try to convert to slice
		if taintsSlice, ok := taintsVal.([]interface{}); ok {
			return len(taintsSlice) > 0
		}
	}

	return false
}
