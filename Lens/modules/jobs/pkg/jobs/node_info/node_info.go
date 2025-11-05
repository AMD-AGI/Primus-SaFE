package node_info

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/AMD-AGI/primus-lens/core/pkg/clientsets"
	"github.com/AMD-AGI/primus-lens/core/pkg/database"
	"github.com/AMD-AGI/primus-lens/core/pkg/database/model"
	"github.com/AMD-AGI/primus-lens/core/pkg/helper/gpu"
	"github.com/AMD-AGI/primus-lens/core/pkg/helper/metadata"
	"github.com/AMD-AGI/primus-lens/core/pkg/helper/node"
	"github.com/AMD-AGI/primus-lens/core/pkg/logger/log"
	"github.com/AMD-AGI/primus-lens/core/pkg/utils/k8sUtil"
	"github.com/AMD-AGI/primus-lens/jobs/pkg/common"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	defaultGPUVendor = metadata.GpuVendorAMD
)

type NodeInfoJob struct {
}

func (n *NodeInfoJob) Run(ctx context.Context, clientSets *clientsets.K8SClientSet, storageClientSet *clientsets.StorageClientSet) (*common.ExecutionStats, error) {
	stats := common.NewExecutionStats()
	
	gpuNodes, err := gpu.GetGpuNodes(ctx, clientSets, defaultGPUVendor)
	if err != nil {
		return stats, err
	}
	
	var created, updated int64
	wg := &sync.WaitGroup{}
	for i := range gpuNodes {
		wg.Add(1)
		gpuNode := gpuNodes[i]
		go func() {
			defer wg.Done()
			isCreate, err := n.runForSingleNode(ctx, gpuNode, clientSets, storageClientSet)
			if err != nil {
				atomic.AddInt64(&stats.ErrorCount, 1)
				log.Errorf("Fail run node info job for %s:%+v", gpuNode, err)
			} else {
				if isCreate {
					atomic.AddInt64(&created, 1)
				} else {
					atomic.AddInt64(&updated, 1)
				}
			}
		}()
	}
	wg.Wait()
	
	stats.ItemsCreated = created
	stats.ItemsUpdated = updated
	stats.RecordsProcessed = int64(len(gpuNodes))
	stats.AddCustomMetric("nodes_count", len(gpuNodes))
	stats.AddMessage("Node info updated successfully")
	
	return stats, nil
}

func (n *NodeInfoJob) Schedule() string {
	return "@every 10s"
}

func (n *NodeInfoJob) runForSingleNode(ctx context.Context, nodeName string, clientSets *clientsets.K8SClientSet, storageClientSet *clientsets.StorageClientSet) (bool, error) {
	k8sNode := &corev1.Node{}
	err := clientSets.ControllerRuntimeClient.Get(ctx, types.NamespacedName{Name: nodeName}, k8sNode)
	if err != nil {
		return false, client.IgnoreNotFound(err)
	}
	existDBNode, err := database.GetFacade().GetNode().GetNodeByName(ctx, k8sNode.Name)
	if err != nil {
		return false, err
	}

	newDBNode := &model.Node{
		ID:                0,
		Name:              k8sNode.Name,
		Address:           k8sNode.Status.Addresses[0].Address,
		GpuName:           node.GetGpuDeviceName(*k8sNode, defaultGPUVendor),
		Status:            k8sUtil.NodeStatus(*k8sNode),
		CreatedAt:         time.Now(),
		UpdatedAt:         time.Now(),
		CPU:               "", // TODO CPU information awaiting agent retrieval
		CPUCount:          int32(node.GetCPUCount(*k8sNode)),
		Memory:            node.GetMemorySizeHumanReadable(*k8sNode),
		K8sVersion:        "1.23.1",
		K8sStatus:         k8sUtil.NodeStatus(*k8sNode),
		Os:                k8sNode.Status.NodeInfo.OSImage,
		KubeletVersion:    k8sNode.Status.NodeInfo.KubeletVersion,
		ContainerdVersion: k8sNode.Status.NodeInfo.ContainerRuntimeVersion,
	}
	nodeExporterClient, err := clientsets.GetOrInitNodeExportersClient(ctx, nodeName, clientSets.ControllerRuntimeClient)
	if err != nil {
		log.Errorf("Fail to init node exporter client for nodeName:%s, err:%+v", nodeName, err)
	} else {
		driverVer, err := nodeExporterClient.GetDriverVersion(ctx)
		if err == nil {
			newDBNode.DriverVersion = driverVer
		} else {
			if existDBNode != nil {
				newDBNode.DriverVersion = existDBNode.DriverVersion
			}
			log.Errorf("Fail get driver version from %s.Error %+v", nodeName, err)
		}

	}
	
	isCreate := false
	if existDBNode == nil {
		existDBNode = newDBNode
		isCreate = true
	} else {
		if time.Now().Before(existDBNode.UpdatedAt.Add(10 * time.Second)) {
			return false, nil
		}
		newDBNode.ID = existDBNode.ID
		newDBNode.CreatedAt = existDBNode.CreatedAt
		newDBNode.UpdatedAt = time.Now()
		existDBNode = newDBNode
	}
	// Use current cluster name for job running in current cluster
	clusterName := clientsets.GetClusterManager().GetCurrentClusterName()
	allocatable, capacity, err := gpu.GetNodeGpuAllocation(ctx, clientSets, k8sNode.Name, clusterName, defaultGPUVendor)
	if err != nil {
		log.Errorf("Failed to get node gpu allocation for %s: %v", k8sNode.Name, err)
		return false, err
	}
	existDBNode.GpuCount = int32(capacity)
	existDBNode.GpuAllocation = int32(allocatable)
	usage, err := gpu.CalculateNodeGpuUsage(ctx, nodeName, storageClientSet, defaultGPUVendor)
	if err == nil {
		existDBNode.GpuUtilization = usage
	}
	if existDBNode.ID == 0 {
		return isCreate, database.GetFacade().GetNode().CreateNode(ctx, existDBNode)
	} else {
		return isCreate, database.GetFacade().GetNode().UpdateNode(ctx, existDBNode)
	}
}
