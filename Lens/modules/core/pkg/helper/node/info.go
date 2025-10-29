package node

import (
	"context"

	"github.com/AMD-AGI/primus-lens/core/pkg/clientsets"
	"github.com/AMD-AGI/primus-lens/core/pkg/database"
	"github.com/AMD-AGI/primus-lens/core/pkg/helper/gpu"
	"github.com/AMD-AGI/primus-lens/core/pkg/helper/metadata"
	"github.com/AMD-AGI/primus-lens/core/pkg/model"
	"github.com/AMD-AGI/primus-lens/core/pkg/utils/k8sUtil"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

var (
	statusColorMap = map[string]string{
		k8sUtil.NodeStatusReady:    "green",
		k8sUtil.NodeStatusNotReady: "red",
		k8sUtil.NodeStatusUnknown:  "yellow",
	}
)

func GetNodeGpuModel(ctx context.Context, nodeName string) (string, error) {
	node, err := database.GetNodeByName(ctx, nodeName)
	if err != nil {
		return "", err
	}
	if node == nil {
		return "", nil
	}
	return node.GpuName, nil
}

func GetGpuNodeInfo(ctx context.Context, nodeName string, clientSets *clientsets.K8SClientSet, storageClientSet *clientsets.StorageClientSet, vendor metadata.GpuVendor) (*model.GPUNode, error) {
	node := &corev1.Node{}
	err := clientSets.ControllerRuntimeClient.Get(ctx, types.NamespacedName{Name: nodeName}, node)
	if err != nil {
		return nil, err
	}
	gpuDeviceName := GetGpuDeviceName(*node, vendor)
	alloc, capacity, err := gpu.GetNodeGpuAllocation(ctx, clientSets, nodeName, vendor)
	if err != nil {
		return nil, err
	}
	gpuUsage, _ := gpu.CalculateNodeGpuUsage(ctx, nodeName, storageClientSet, vendor)
	nodeStatus := k8sUtil.NodeStatus(*node)
	return &model.GPUNode{
		Name:           nodeName,
		Ip:             node.Status.Addresses[0].Address,
		GpuName:        gpuDeviceName,
		GpuCount:       capacity,
		GpuAllocation:  alloc,
		GpuUtilization: gpuUsage,
		Status:         nodeStatus,
		StatusColor:    GetStatusColor(nodeStatus),
	}, nil
}

func GetStatusColor(nodeStatus string) string {
	return statusColorMap[nodeStatus]
}

func GetGpuDeviceName(node corev1.Node, vendor metadata.GpuVendor) string {
	if node.Labels == nil {
		return "Unknown"
	}
	return node.Labels[metadata.GetDeviceTagNames(vendor)]
}

func GetMemorySize(node corev1.Node) int64 {
	return node.Status.Capacity.Memory().Value()
}

func GetMemorySizeHumanReadable(node corev1.Node) string {
	return node.Status.Capacity.Memory().String()
}

func GetCPUCount(node corev1.Node) int64 {
	return node.Status.Capacity.Cpu().Value()
}
