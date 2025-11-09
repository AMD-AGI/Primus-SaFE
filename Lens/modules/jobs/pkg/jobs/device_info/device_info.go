package device_info

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/constant"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/helper/gpu"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/helper/metadata"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	boModel "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/jobs/pkg/common"
)

var (
	defaultGPUVendor = metadata.GpuVendorAMD
)

type DeviceInfoJob struct {
}

func (d *DeviceInfoJob) Run(ctx context.Context, clientSets *clientsets.K8SClientSet, storageClientSet *clientsets.StorageClientSet) (*common.ExecutionStats, error) {
	stats := common.NewExecutionStats()
	
	nodes, err := gpu.GetGpuNodes(ctx, clientSets, defaultGPUVendor)
	if err != nil {
		return stats, err
	}
	
	wg := &sync.WaitGroup{}
	for i := range nodes {
		nodeName := nodes[i]
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := d.getDeviceInfoForSingleNode(ctx, clientSets, nodeName, stats)
			if err != nil {
				atomic.AddInt64(&stats.ErrorCount, 1)
				log.Errorf("Fail get device info for node %s: %v", nodeName, err)
			}
		}()
	}
	wg.Wait()
	
	stats.RecordsProcessed = int64(len(nodes))
	stats.AddCustomMetric("nodes_count", len(nodes))
	stats.AddMessage("Device info updated successfully")
	
	return stats, nil
}

func (d *DeviceInfoJob) Schedule() string {
	return "@every 10s"
}

func (d *DeviceInfoJob) getDeviceInfoForSingleNode(ctx context.Context, clientSets *clientsets.K8SClientSet, nodeName string, stats *common.ExecutionStats) error {
	dbNode, err := database.GetFacade().GetNode().GetNodeByName(ctx, nodeName)
	if err != nil {
		return err
	}
	if dbNode == nil {
		return fmt.Errorf("fail to get node by name %s.Record not exist", nodeName)
	}
	nodeExporterClient, err := clientsets.GetOrInitNodeExportersClient(ctx, nodeName, clientSets.ControllerRuntimeClient)
	if err != nil {
		return err
	}
	err = d.getGPUDeviceInfo(ctx, nodeExporterClient, dbNode, stats)
	if err != nil {
		return err
	}
	err = d.getRDMADeviceInfo(ctx, nodeExporterClient, dbNode, stats)
	if err != nil {
		return err
	}

	return nil
}

func (d *DeviceInfoJob) getRDMADeviceInfo(ctx context.Context, nodeExporterClient *clientsets.NodeExporterClient, dbNode *model.Node, stats *common.ExecutionStats) error {
	rdmaDevices, err := nodeExporterClient.GetRdmaDevices(ctx)
	if err != nil {
		return err
	}
	created := []model.RdmaDevice{}
	deleted := []model.RdmaDevice{}
	for i := range rdmaDevices {
		rdmaDevice := rdmaDevices[i]
		newRdmaInfo := &model.RdmaDevice{
			ID:        0,
			NodeID:    dbNode.ID,
			Ifname:    rdmaDevice.IfName,
			NodeGUID:  rdmaDevice.NodeGUID,
			IfIndex:   int32(rdmaDevice.IfIndex),
			Fw:        rdmaDevice.FW,
			NodeType:  rdmaDevice.NodeType,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		existInfo, err := database.GetFacade().GetNode().GetRdmaDeviceByNodeIdAndPort(ctx, rdmaDevice.NodeGUID, rdmaDevice.IfIndex)
		if err != nil {
			return err
		}
		if existInfo == nil {
			existInfo = newRdmaInfo
		}
		if existInfo.ID == 0 {
			err = database.GetFacade().GetNode().CreateRdmaDevice(ctx, existInfo)
			if err != nil {
				return err
			}
			created = append(created, *existInfo)
			atomic.AddInt64(&stats.ItemsCreated, 1)
		}
	}
	// TODO remove changed device
	nodeDevices, err := database.GetFacade().GetNode().ListRdmaDeviceByNodeId(ctx, dbNode.ID)
	if err != nil {
		return err
	}
	for i := range nodeDevices {
		found := false
		for j := range rdmaDevices {
			device := rdmaDevices[j]
			if device.IfIndex == int(nodeDevices[i].IfIndex) && device.NodeGUID == nodeDevices[i].NodeGUID {
				found = true
				break
			}
		}
		if !found {
			deleted = append(deleted, *nodeDevices[i])
			err = database.GetFacade().GetNode().DeleteRdmaDeviceById(ctx, nodeDevices[i].ID)
			if err != nil {
				return err
			}
			atomic.AddInt64(&stats.ItemsDeleted, 1)
		}
	}
	for _, device := range created {
		log.Infof("Created RDMA device: %+v", device)
		evt := &model.NodeDeviceChangelog{
			ID:         0,
			NodeID:     dbNode.ID,
			NodeName:   dbNode.Name,
			DeviceType: constant.DeviceTypeRDMA,
			DeviceName: device.Ifname,
			DeviceUUID: device.NodeGUID,
			Op:         constant.DeviceChangelogOpCreate,
			CreatedAt:  time.Now(),
		}
		err = database.GetFacade().GetNode().CreateNodeDeviceChangelog(ctx, evt)
		if err != nil {
			log.Errorf("Fail to create node device changelog: %v", err)
		}
	}
	for _, device := range deleted {
		log.Infof("Deleted RDMA device: %+v", device)
		evt := &model.NodeDeviceChangelog{
			ID:         0,
			NodeID:     dbNode.ID,
			NodeName:   dbNode.Name,
			DeviceType: constant.DeviceTypeRDMA,
			DeviceName: device.Ifname,
			DeviceUUID: device.NodeGUID,
			Op:         constant.DeviceChangelogOpDelete,
			CreatedAt:  time.Now(),
		}
		err = database.GetFacade().GetNode().CreateNodeDeviceChangelog(ctx, evt)
		if err != nil {
			log.Errorf("Fail to create node device changelog: %v", err)
		}
	}
	return nil
}

func (d *DeviceInfoJob) getGPUDeviceInfo(ctx context.Context, nodeExporterClient *clientsets.NodeExporterClient, dbNode *model.Node, stats *common.ExecutionStats) error {
	gpuMaps := map[int]boModel.GPUInfo{}
	cardMetricsMaps := map[int]boModel.CardMetrics{}
	gpus, err := nodeExporterClient.GetGPUs(ctx)
	if err != nil {
		return err
	}
	cardMetrics, err := nodeExporterClient.GetCardMetrics(ctx)
	if err != nil {
		return err
	}
	for i := range gpus {
		gpuInfo := gpus[i]
		gpuMaps[i] = gpuInfo
	}
	for i := range cardMetrics {
		gpuMetrics := cardMetrics[i]
		cardMetricsMaps[i] = gpuMetrics
	}
	created := []model.GpuDevice{}
	deleted := []model.GpuDevice{}
	for i := range gpus {
		info := gpus[i]
		cardMetric := cardMetricsMaps[i]
		newGpuInfo := &model.GpuDevice{
			NodeID:         dbNode.ID,
			GpuID:          int32(info.GPU),
			GpuModel:       info.Asic.MarketName,
			Memory:         info.VRAM.GetVramSizeMegaBytes(),
			Utilization:    cardMetric.GPUUsePercent,
			Temperature:    cardMetric.TemperatureJunction,
			Power:          cardMetric.SocketGraphicsPowerWatts,
			Serial:         info.Asic.AsicSerial,
			RdmaDeviceName: "",
			RdmaGUID:       "",
			RdmaLid:        "",
			CreatedAt:      time.Now(),
			UpdatedAt:      time.Now(),
			NumaAffinity:   int32(info.NUMA.Affinity),
			NumaNode:       int32(info.NUMA.Node),
		}
		existInfo, err := database.GetFacade().GetNode().GetGpuDeviceByNodeAndGpuId(ctx, dbNode.ID, info.GPU)
		if err != nil {
			return err
		}
		if existInfo == nil {
			existInfo = newGpuInfo
		} else {
			newGpuInfo.ID = existInfo.ID
			newGpuInfo.CreatedAt = existInfo.CreatedAt
			existInfo = newGpuInfo
		}
		if existInfo.ID == 0 {
			created = append(created, *existInfo)
			err = database.GetFacade().GetNode().CreateGpuDevice(ctx, existInfo)
			if err != nil {
				return err
			}
			atomic.AddInt64(&stats.ItemsCreated, 1)
		} else {
			err = database.GetFacade().GetNode().UpdateGpuDevice(ctx, existInfo)
			if err != nil {
				return err
			}
			atomic.AddInt64(&stats.ItemsUpdated, 1)
		}
	}
	nodeDevices, err := database.GetFacade().GetNode().ListGpuDeviceByNodeId(ctx, dbNode.ID)
	if err != nil {
		return err
	}
	for i := range nodeDevices {
		found := false
		for j := range gpus {
			info := gpus[j]
			if int32(info.GPU) == nodeDevices[i].GpuID {
				found = true
				break
			}
		}
		if !found {
			deleted = append(deleted, *nodeDevices[i])
			err = database.GetFacade().GetNode().DeleteGpuDeviceById(ctx, nodeDevices[i].ID)
			if err != nil {
				return err
			}
			atomic.AddInt64(&stats.ItemsDeleted, 1)
		}
	}
	for _, device := range created {
		log.Infof("Created GPU device: %+v", device)
		evt := &model.NodeDeviceChangelog{
			ID:         0,
			NodeID:     dbNode.ID,
			NodeName:   dbNode.Name,
			DeviceType: constant.DeviceTypeGPU,
			DeviceName: device.GpuModel,
			DeviceUUID: device.Serial,
			Op:         constant.DeviceChangelogOpCreate,
			CreatedAt:  time.Now(),
		}
		err = database.GetFacade().GetNode().CreateNodeDeviceChangelog(ctx, evt)
		if err != nil {
			log.Errorf("Fail to create node device changelog: %v", err)
		}
	}
	for _, device := range deleted {
		log.Infof("Deleted GPU device: %+v", device)
		evt := &model.NodeDeviceChangelog{
			ID:         0,
			NodeID:     dbNode.ID,
			NodeName:   dbNode.Name,
			DeviceType: constant.DeviceTypeGPU,
			DeviceName: device.GpuModel,
			DeviceUUID: device.Serial,
			Op:         constant.DeviceChangelogOpDelete,
			CreatedAt:  time.Now(),
		}
		err = database.GetFacade().GetNode().CreateNodeDeviceChangelog(ctx, evt)
		if err != nil {
			log.Errorf("Fail to create node device changelog: %v", err)
		}
	}
	return nil
}
