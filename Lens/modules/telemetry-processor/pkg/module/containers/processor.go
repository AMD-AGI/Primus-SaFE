package containers

import (
	"context"
	"encoding/json"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/constant"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	dbModel "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/errors"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model"
)

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
	if existContainer == nil {
		existContainer = &dbModel.NodeContainer{
			ContainerID:   req.ContainerID,
			ContainerName: containerData.ID,
			PodUID:        containerData.PodUUID,
			PodName:       containerData.PodName,
			PodNamespace:  containerData.PodNamespace,
			CreatedAt:     time.Unix(0, containerData.CreatedAt),
			UpdatedAt:     time.Now(),
			NodeName:      req.Node,
			Source:        constant.ContainerSourceK8S,
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
	if containerData.Devices != nil {
		// Create a context with timeout for device operations
		deviceCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		// Save GPU devices
		for _, gpu := range containerData.Devices.GPU {
			if err := saveContainerDevice(deviceCtx, req.ContainerID, req.Node, gpu.Name, int32(gpu.Id), gpu.Serial, constant.DeviceTypeGPU); err != nil {
				log.Warnf("Failed to save GPU device for container %s (device=%s): %v - continuing anyway", req.ContainerID, gpu.Name, err)
				containerEventErrorCnt.WithLabelValues(req.Source, req.Node, "device_save_error").Inc()
				// Continue processing other devices - don't fail the entire event
			}
		}

		// Save InfiniBand devices
		for _, ib := range containerData.Devices.Infiniband {
			if err := saveContainerDevice(deviceCtx, req.ContainerID, req.Node, ib.Name, int32(ib.Id), ib.Serial, constant.DeviceTypeIB); err != nil {
				log.Warnf("Failed to save IB device for container %s (device=%s): %v - continuing anyway", req.ContainerID, ib.Name, err)
				containerEventErrorCnt.WithLabelValues(req.Source, req.Node, "device_save_error").Inc()
				// Continue processing other devices - don't fail the entire event
			}
		}
	}

	// Save container event (except for snapshots)
	if req.Type != model.ContainerEventTypeSnapshot {
		event := &dbModel.NodeContainerEvent{
			ContainerID: req.ContainerID,
			EventType:   req.Type,
			CreatedAt:   time.Now(),
		}
		if err := database.GetFacade().GetContainer().CreateNodeContainerEvent(ctx, event); err != nil {
			log.Errorf("Failed to create container event for %s: %v", req.ContainerID, err)
			containerEventErrorCnt.WithLabelValues(req.Source, req.Node, "event_save_error").Inc()
			// Non-critical error, continue
		}
	}

	log.Infof("Successfully processed K8s container event: container=%s, type=%s, node=%s", req.ContainerID, req.Type, req.Node)
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
			// Continue processing other devices - don't fail the entire event
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
		log.Debugf("Created container device association: container=%s, device=%s, type=%s", containerID, deviceName, deviceType)
	} else {
		log.Debugf("Container device association already exists: container=%s, device=%s", containerID, deviceName)
	}

	return nil
}
