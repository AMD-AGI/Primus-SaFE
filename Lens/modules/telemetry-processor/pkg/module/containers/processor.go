// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package containers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/constant"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	dbModel "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/errors"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model"
	"github.com/opensearch-project/opensearch-go/opensearchapi"
)

const indexDateFormat = "2006.01.02"

// ProcessContainerEvent processes a single container event
func ProcessContainerEvent(ctx context.Context, req *ContainerEventRequest) error {
	start := time.Now()
	defer func() {
		containerEventProcessingDuration.WithLabelValues(req.Source, req.Node).Observe(time.Since(start).Seconds())
	}()

	containerEventRecvCnt.WithLabelValues(req.Source, req.Node).Inc()

	switch req.Source {
	case "k8s":
		return processK8sContainerEvent(ctx, req)
	case "docker":
		return processDockerContainerEvent(ctx, req)
	default:
		log.Errorf("Unknown container event source: %s", req.Source)
		containerEventErrorCnt.WithLabelValues(req.Source, req.Node, "unknown_source").Inc()
		return errors.NewError().WithCode(errors.CodeInvalidArgument).WithMessagef("unknown source: %s", req.Source)
	}
}

// processK8sContainerEvent processes a Kubernetes container event
func processK8sContainerEvent(ctx context.Context, req *ContainerEventRequest) error {
	// Parse container data
	containerData := &K8sContainerData{}
	dataBytes, err := json.Marshal(req.Data)
	if err != nil {
		log.Errorf("Failed to marshal container data: %v", err)
		containerEventErrorCnt.WithLabelValues(req.Source, req.Node, "marshal_error").Inc()
		return errors.NewError().WithCode(errors.CodeInvalidArgument).WithMessagef("failed to marshal data: %v", err)
	}

	err = json.Unmarshal(dataBytes, containerData)
	if err != nil {
		log.Errorf("Failed to unmarshal container data: %v", err)
		containerEventErrorCnt.WithLabelValues(req.Source, req.Node, "unmarshal_error").Inc()
		return errors.NewError().WithCode(errors.CodeInvalidArgument).WithMessagef("failed to unmarshal data: %v", err)
	}

	// Skip containers without GPU devices (unless it's a snapshot)
	if containerData.Devices == nil || len(containerData.Devices.GPU) == 0 {
		if req.Type != model.ContainerEventTypeSnapshot {
			log.Debugf("Container %s has no GPU devices, skipping", req.ContainerID)
			return nil
		}
	}

	// Check if container exists
	existContainer, err := database.GetFacade().GetContainer().GetNodeContainerByContainerId(ctx, req.ContainerID)
	if err != nil {
		log.Errorf("Failed to get container by id %s: %v", req.ContainerID, err)
		containerEventErrorCnt.WithLabelValues(req.Source, req.Node, "db_query_error").Inc()
		return errors.NewError().WithCode(errors.CodeDatabaseError).WithMessagef("failed to get container by id %s", req.ContainerID)
	}

	// Create or update container record
	// If container doesn't exist (nil) or ID is 0 (empty object), need to set all fields
	if existContainer == nil || existContainer.ID == 0 {
		// If nil, create new object; if ID=0, reset all fields
		if existContainer == nil {
			existContainer = &dbModel.NodeContainer{}
		}

		existContainer.ContainerID = req.ContainerID
		existContainer.ContainerName = containerData.ID
		existContainer.PodUID = containerData.PodUUID
		existContainer.PodName = containerData.PodName
		existContainer.PodNamespace = containerData.PodNamespace
		existContainer.CreatedAt = time.Unix(0, containerData.CreatedAt)
		existContainer.UpdatedAt = time.Now()
		existContainer.NodeName = req.Node
		existContainer.Source = constant.ContainerSourceK8S
		existContainer.Status = containerData.Status
	} else {
		existContainer.Status = containerData.Status
		existContainer.UpdatedAt = time.Now()
	}

	// Save container
	if existContainer.ID == 0 {
		err = database.GetFacade().GetContainer().CreateNodeContainer(ctx, existContainer)
	} else {
		err = database.GetFacade().GetContainer().UpdateNodeContainer(ctx, existContainer)
	}
	if err != nil {
		log.Errorf("Failed to save container %s: %v", req.ContainerID, err)
		containerEventErrorCnt.WithLabelValues(req.Source, req.Node, "db_save_error").Inc()
		return errors.NewError().WithCode(errors.CodeDatabaseError).WithMessagef("failed to save container %s", req.ContainerID)
	}

	// Save device associations (with timeout protection)
	if containerData.Devices != nil {
		// Create a context with timeout for device operations
		deviceCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		// Save GPU devices
		for _, gpu := range containerData.Devices.GPU {
			if err := saveContainerDevice(deviceCtx, req.ContainerID, req.Node, gpu.Name, int32(gpu.Id), gpu.Serial, constant.DeviceTypeGPU); err != nil {
				log.Warnf("Failed to save GPU device for container %s (device=%s): %v - continuing anyway", req.ContainerID, gpu.Name, err)
				containerEventErrorCnt.WithLabelValues(req.Source, req.Node, "device_save_error").Inc()
			}
		}

		// Save InfiniBand devices
		for _, ib := range containerData.Devices.Infiniband {
			if err := saveContainerDevice(deviceCtx, req.ContainerID, req.Node, ib.Name, int32(ib.Id), ib.Serial, constant.DeviceTypeIB); err != nil {
				log.Warnf("Failed to save IB device for container %s (device=%s): %v - continuing anyway", req.ContainerID, ib.Name, err)
				containerEventErrorCnt.WithLabelValues(req.Source, req.Node, "device_save_error").Inc()
			}
		}
	}

	// Write container event to OpenSearch (replaces PG node_container_event INSERT).
	// Enriched with pod context that was not available in the old PG schema.
	if req.Type != model.ContainerEventTypeSnapshot {
		now := time.Now()
		doc := map[string]interface{}{
			"container_id": req.ContainerID,
			"event_type":   req.Type,
			"node":         req.Node,
			"pod_uid":      containerData.PodUUID,
			"pod_name":     containerData.PodName,
			"namespace":    containerData.PodNamespace,
			"@timestamp":   now.Format(time.RFC3339),
		}
		if err := indexContainerEvent(ctx, doc); err != nil {
			log.Errorf("Failed to write container event to OpenSearch for %s: %v", req.ContainerID, err)
			containerEventErrorCnt.WithLabelValues(req.Source, req.Node, "event_save_error").Inc()
		}
	}

	return nil
}

// processDockerContainerEvent processes a Docker container event
func processDockerContainerEvent(ctx context.Context, req *ContainerEventRequest) error {
	// Parse container data
	containerData := &DockerContainerData{}
	dataBytes, err := json.Marshal(req.Data)
	if err != nil {
		log.Errorf("Failed to marshal container data: %v", err)
		containerEventErrorCnt.WithLabelValues(req.Source, req.Node, "marshal_error").Inc()
		return errors.NewError().WithCode(errors.CodeInvalidArgument).WithMessagef("failed to marshal data: %v", err)
	}

	err = json.Unmarshal(dataBytes, containerData)
	if err != nil {
		log.Errorf("Failed to unmarshal container data: %v", err)
		containerEventErrorCnt.WithLabelValues(req.Source, req.Node, "unmarshal_error").Inc()
		return errors.NewError().WithCode(errors.CodeInvalidArgument).WithMessagef("failed to unmarshal data: %v", err)
	}

	// Check if container exists
	existContainer, err := database.GetFacade().GetContainer().GetNodeContainerByContainerId(ctx, req.ContainerID)
	if err != nil {
		log.Errorf("Failed to get container by id %s: %v", req.ContainerID, err)
		containerEventErrorCnt.WithLabelValues(req.Source, req.Node, "db_query_error").Inc()
		return errors.NewError().WithCode(errors.CodeDatabaseError).WithMessagef("failed to get container by id %s", req.ContainerID)
	}

	// Create or update container record
	if existContainer == nil {
		existContainer = &dbModel.NodeContainer{
			ContainerID:   containerData.ID,
			ContainerName: containerData.Name,
			PodUID:        "",
			PodName:       "",
			PodNamespace:  "",
			CreatedAt:     containerData.StartAt,
			UpdatedAt:     time.Now(),
			NodeName:      req.Node,
			Source:        constant.ContainerSourceDocker,
			Status:        containerData.Status,
		}
	} else {
		existContainer.Status = containerData.Status
		existContainer.UpdatedAt = time.Now()
	}

	// Save container
	if existContainer.ID == 0 {
		err = database.GetFacade().GetContainer().CreateNodeContainer(ctx, existContainer)
	} else {
		err = database.GetFacade().GetContainer().UpdateNodeContainer(ctx, existContainer)
	}
	if err != nil {
		log.Errorf("Failed to save container %s: %v", req.ContainerID, err)
		containerEventErrorCnt.WithLabelValues(req.Source, req.Node, "db_save_error").Inc()
		return errors.NewError().WithCode(errors.CodeDatabaseError).WithMessagef("failed to save container %s", req.ContainerID)
	}

	// Save device associations (with timeout protection)
	deviceCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	for _, device := range containerData.Devices {
		deviceType := device.DeviceType
		if deviceType == "" {
			deviceType = constant.DeviceTypeGPU
		}
		if err := saveContainerDevice(deviceCtx, req.ContainerID, req.Node, device.DeviceName, int32(device.DeviceId), device.DeviceSerial, deviceType); err != nil {
			log.Warnf("Failed to save device for container %s (device=%s): %v - continuing anyway", req.ContainerID, device.DeviceName, err)
			containerEventErrorCnt.WithLabelValues(req.Source, req.Node, "device_save_error").Inc()
		}
	}

	log.Infof("Successfully processed Docker container event: container=%s, type=%s, node=%s", req.ContainerID, req.Type, req.Node)
	return nil
}

// saveContainerDevice saves a container-device association
func saveContainerDevice(ctx context.Context, containerID, node, deviceName string, deviceNo int32, deviceUUID, deviceType string) error {
	existRecord, err := database.GetFacade().GetContainer().GetNodeContainerDeviceByContainerIdAndDeviceUid(ctx, containerID, deviceUUID)
	if err != nil {
		log.Errorf("Failed to get container device by container id %s and device uid %s: %v", containerID, deviceUUID, err)
		return errors.NewError().WithCode(errors.CodeDatabaseError).WithMessagef("failed to get container device")
	}

	if existRecord == nil {
		existRecord = &dbModel.NodeContainerDevices{
			ContainerID: containerID,
			DeviceType:  deviceType,
			DeviceName:  deviceName,
			DeviceNo:    deviceNo,
			DeviceUUID:  deviceUUID,
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}
		err = database.GetFacade().GetContainer().CreateNodeContainerDevice(ctx, existRecord)
		if err != nil {
			log.Errorf("Failed to create node container device: %v", err)
			return errors.NewError().WithCode(errors.CodeDatabaseError).WithMessagef("failed to create node container device")
		}
	}

	return nil
}

// indexContainerEvent writes a single container event document to OpenSearch.
// Index name follows the pattern: container-event-YYYY.MM.DD
func indexContainerEvent(ctx context.Context, doc map[string]interface{}) error {
	osClient := clientsets.GetClusterManager().GetCurrentClusterClients().StorageClientSet.OpenSearch

	body, err := json.Marshal(doc)
	if err != nil {
		return fmt.Errorf("marshal doc: %w", err)
	}

	ts, _ := doc["@timestamp"].(string)
	t, parseErr := time.Parse(time.RFC3339, ts)
	if parseErr != nil {
		t = time.Now()
	}
	indexName := fmt.Sprintf("container-event-%s", t.Format(indexDateFormat))

	req := opensearchapi.IndexRequest{
		Index: indexName,
		Body:  bytes.NewReader(body),
	}
	res, err := req.Do(ctx, osClient)
	if err != nil {
		return fmt.Errorf("index request: %w", err)
	}
	defer res.Body.Close()

	if res.IsError() {
		return fmt.Errorf("index response error: %s", res.String())
	}
	return nil
}
