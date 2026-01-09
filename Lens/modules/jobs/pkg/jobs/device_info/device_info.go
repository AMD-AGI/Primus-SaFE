// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

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
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/trace"
	"github.com/AMD-AGI/Primus-SaFE/Lens/modules/jobs/pkg/common"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

var (
	defaultGPUVendor = metadata.GpuVendorAMD
)

type DeviceInfoJob struct {
}

func (d *DeviceInfoJob) Run(ctx context.Context, clientSets *clientsets.K8SClientSet, storageClientSet *clientsets.StorageClientSet) (*common.ExecutionStats, error) {
	// Create main trace span
	span, ctx := trace.StartSpanFromContext(ctx, "device_info_job.Run")
	defer trace.FinishSpan(span)

	// Record total job start time
	jobStartTime := time.Now()

	stats := common.NewExecutionStats()

	span.SetAttributes(
		attribute.String("job.name", "device_info"),
		attribute.String("gpu.vendor", string(defaultGPUVendor)),
	)

	// Get GPU nodes
	nodesSpan, nodesCtx := trace.StartSpanFromContext(ctx, "getGpuNodes")
	nodesSpan.SetAttributes(attribute.String("gpu.vendor", string(defaultGPUVendor)))

	queryStart := time.Now()
	nodes, err := gpu.GetGpuNodes(nodesCtx, clientSets, defaultGPUVendor)
	if err != nil {
		nodesSpan.RecordError(err)
		nodesSpan.SetAttributes(attribute.String("error.message", err.Error()))
		nodesSpan.SetStatus(codes.Error, err.Error())
		trace.FinishSpan(nodesSpan)

		span.SetStatus(codes.Error, "Failed to get GPU nodes")
		return stats, err
	}

	duration := time.Since(queryStart)
	nodesSpan.SetAttributes(
		attribute.Int("nodes.count", len(nodes)),
		attribute.Float64("duration_ms", float64(duration.Milliseconds())),
	)
	nodesSpan.SetStatus(codes.Ok, "")
	trace.FinishSpan(nodesSpan)

	span.SetAttributes(attribute.Int("nodes.total_count", len(nodes)))

	// Process nodes concurrently
	processSpan, processCtx := trace.StartSpanFromContext(ctx, "processNodes")
	processSpan.SetAttributes(attribute.Int("nodes.count", len(nodes)))

	processStart := time.Now()
	wg := &sync.WaitGroup{}
	for i := range nodes {
		nodeName := nodes[i]
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := d.getDeviceInfoForSingleNode(processCtx, clientSets, nodeName, stats)
			if err != nil {
				atomic.AddInt64(&stats.ErrorCount, 1)
				log.Errorf("Fail get device info for node %s: %v", nodeName, err)
			}
		}()
	}
	wg.Wait()

	duration = time.Since(processStart)
	processSpan.SetAttributes(
		attribute.Float64("duration_ms", float64(duration.Milliseconds())),
		attribute.Int64("errors.count", stats.ErrorCount),
	)
	processSpan.SetStatus(codes.Ok, "")
	trace.FinishSpan(processSpan)

	stats.RecordsProcessed = int64(len(nodes))
	stats.AddCustomMetric("nodes_count", len(nodes))
	stats.AddMessage("Device info updated successfully")

	// Record total job duration
	totalDuration := time.Since(jobStartTime)
	span.SetAttributes(attribute.Float64("total_duration_ms", float64(totalDuration.Milliseconds())))
	span.SetStatus(codes.Ok, "")
	return stats, nil
}

func (d *DeviceInfoJob) Schedule() string {
	return "@every 10s"
}

func (d *DeviceInfoJob) getDeviceInfoForSingleNode(ctx context.Context, clientSets *clientsets.K8SClientSet, nodeName string, stats *common.ExecutionStats) error {
	span, ctx := trace.StartSpanFromContext(ctx, "getDeviceInfoForSingleNode")
	defer trace.FinishSpan(span)

	span.SetAttributes(attribute.String("node.name", nodeName))

	// Get node from database
	nodeSpan, nodeCtx := trace.StartSpanFromContext(ctx, "getNodeByName")
	nodeSpan.SetAttributes(attribute.String("node.name", nodeName))

	dbNode, err := database.GetFacade().GetNode().GetNodeByName(nodeCtx, nodeName)
	if err != nil {
		nodeSpan.RecordError(err)
		nodeSpan.SetAttributes(attribute.String("error.message", err.Error()))
		nodeSpan.SetStatus(codes.Error, err.Error())
		trace.FinishSpan(nodeSpan)

		span.SetStatus(codes.Error, "Failed to get node from database")
		return err
	}
	if dbNode == nil {
		err := fmt.Errorf("fail to get node by name %s.Record not exist", nodeName)
		nodeSpan.RecordError(err)
		nodeSpan.SetAttributes(attribute.String("error.message", err.Error()))
		nodeSpan.SetStatus(codes.Error, err.Error())
		trace.FinishSpan(nodeSpan)

		span.SetStatus(codes.Error, "Node not found in database")
		return err
	}
	nodeSpan.SetAttributes(attribute.Int64("node.id", int64(dbNode.ID)))
	nodeSpan.SetStatus(codes.Ok, "")
	trace.FinishSpan(nodeSpan)

	span.SetAttributes(attribute.Int64("node.id", int64(dbNode.ID)))

	// Get or initialize node exporter client
	clientSpan, clientCtx := trace.StartSpanFromContext(ctx, "getOrInitNodeExportersClient")
	clientSpan.SetAttributes(attribute.String("node.name", nodeName))

	nodeExporterClient, err := clientsets.GetOrInitNodeExportersClient(clientCtx, nodeName, clientSets.ControllerRuntimeClient)
	if err != nil {
		clientSpan.RecordError(err)
		clientSpan.SetAttributes(attribute.String("error.message", err.Error()))
		clientSpan.SetStatus(codes.Error, err.Error())
		trace.FinishSpan(clientSpan)

		span.SetStatus(codes.Error, "Failed to get node exporter client")
		return err
	}
	clientSpan.SetStatus(codes.Ok, "")
	trace.FinishSpan(clientSpan)

	// Get GPU device info
	err = d.getGPUDeviceInfo(ctx, nodeExporterClient, dbNode, stats)
	if err != nil {
		span.RecordError(err)
		span.SetAttributes(attribute.String("error.step", "getGPUDeviceInfo"))
		span.SetStatus(codes.Error, "Failed to get GPU device info")
		return err
	}

	// Get RDMA device info
	err = d.getRDMADeviceInfo(ctx, nodeExporterClient, dbNode, stats)
	if err != nil {
		span.RecordError(err)
		span.SetAttributes(attribute.String("error.step", "getRDMADeviceInfo"))
		span.SetStatus(codes.Error, "Failed to get RDMA device info")
		return err
	}

	span.SetStatus(codes.Ok, "")
	return nil
}

func (d *DeviceInfoJob) getRDMADeviceInfo(ctx context.Context, nodeExporterClient *clientsets.NodeExporterClient, dbNode *model.Node, stats *common.ExecutionStats) error {
	span, ctx := trace.StartSpanFromContext(ctx, "getRDMADeviceInfo")
	defer trace.FinishSpan(span)

	span.SetAttributes(
		attribute.String("node.name", dbNode.Name),
		attribute.Int64("node.id", int64(dbNode.ID)),
	)

	// Get RDMA devices from node exporter
	getDevicesSpan, getDevicesCtx := trace.StartSpanFromContext(ctx, "getRdmaDevices")
	getDevicesSpan.SetAttributes(attribute.String("node.name", dbNode.Name))

	queryStart := time.Now()
	rdmaDevices, err := nodeExporterClient.GetRdmaDevices(getDevicesCtx)
	if err != nil {
		getDevicesSpan.RecordError(err)
		getDevicesSpan.SetAttributes(attribute.String("error.message", err.Error()))
		getDevicesSpan.SetStatus(codes.Error, err.Error())
		trace.FinishSpan(getDevicesSpan)

		span.SetStatus(codes.Error, "Failed to get RDMA devices")
		return err
	}

	duration := time.Since(queryStart)
	getDevicesSpan.SetAttributes(
		attribute.Int("devices.count", len(rdmaDevices)),
		attribute.Float64("duration_ms", float64(duration.Milliseconds())),
	)
	getDevicesSpan.SetStatus(codes.Ok, "")
	trace.FinishSpan(getDevicesSpan)

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

	span.SetAttributes(
		attribute.Int("devices.created_count", len(created)),
		attribute.Int("devices.deleted_count", len(deleted)),
	)
	span.SetStatus(codes.Ok, "")
	return nil
}

func (d *DeviceInfoJob) getGPUDeviceInfo(ctx context.Context, nodeExporterClient *clientsets.NodeExporterClient, dbNode *model.Node, stats *common.ExecutionStats) error {
	span, ctx := trace.StartSpanFromContext(ctx, "getGPUDeviceInfo")
	defer trace.FinishSpan(span)

	span.SetAttributes(
		attribute.String("node.name", dbNode.Name),
		attribute.Int64("node.id", int64(dbNode.ID)),
	)

	gpuMaps := map[int]boModel.GPUInfo{}
	cardMetricsMaps := map[int]boModel.CardMetrics{}

	// Get GPUs from node exporter
	getGPUsSpan, getGPUsCtx := trace.StartSpanFromContext(ctx, "getGPUs")
	getGPUsSpan.SetAttributes(attribute.String("node.name", dbNode.Name))

	queryStart := time.Now()
	gpus, err := nodeExporterClient.GetGPUs(getGPUsCtx)
	if err != nil {
		getGPUsSpan.RecordError(err)
		getGPUsSpan.SetAttributes(attribute.String("error.message", err.Error()))
		getGPUsSpan.SetStatus(codes.Error, err.Error())
		trace.FinishSpan(getGPUsSpan)

		span.SetStatus(codes.Error, "Failed to get GPUs")
		return err
	}

	duration := time.Since(queryStart)
	getGPUsSpan.SetAttributes(
		attribute.Int("gpus.count", len(gpus)),
		attribute.Float64("duration_ms", float64(duration.Milliseconds())),
	)
	getGPUsSpan.SetStatus(codes.Ok, "")
	trace.FinishSpan(getGPUsSpan)

	// Get card metrics from node exporter
	getMetricsSpan, getMetricsCtx := trace.StartSpanFromContext(ctx, "getCardMetrics")
	getMetricsSpan.SetAttributes(attribute.String("node.name", dbNode.Name))

	queryStart = time.Now()
	cardMetrics, err := nodeExporterClient.GetCardMetrics(getMetricsCtx)
	if err != nil {
		getMetricsSpan.RecordError(err)
		getMetricsSpan.SetAttributes(attribute.String("error.message", err.Error()))
		getMetricsSpan.SetStatus(codes.Error, err.Error())
		trace.FinishSpan(getMetricsSpan)

		span.SetStatus(codes.Error, "Failed to get card metrics")
		return err
	}

	duration = time.Since(queryStart)
	getMetricsSpan.SetAttributes(
		attribute.Int("metrics.count", len(cardMetrics)),
		attribute.Float64("duration_ms", float64(duration.Milliseconds())),
	)
	getMetricsSpan.SetStatus(codes.Ok, "")
	trace.FinishSpan(getMetricsSpan)
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

	span.SetAttributes(
		attribute.Int("devices.created_count", len(created)),
		attribute.Int("devices.deleted_count", len(deleted)),
		attribute.Int("devices.total_count", len(gpus)),
	)
	span.SetStatus(codes.Ok, "")
	return nil
}
