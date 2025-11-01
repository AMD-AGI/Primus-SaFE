package exporter

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/AMD-AGI/primus-lens/core/pkg/constant"
	"github.com/AMD-AGI/primus-lens/core/pkg/database"
	dbModel "github.com/AMD-AGI/primus-lens/core/pkg/database/model"
	"github.com/AMD-AGI/primus-lens/core/pkg/errors"
	"github.com/AMD-AGI/primus-lens/core/pkg/logger/log"
	"github.com/AMD-AGI/primus-lens/core/pkg/model"
	pb "github.com/AMD-AGI/primus-lens/core/pkg/pb/exporter"
	"github.com/prometheus/client_golang/prometheus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/reflection"
	"io"
	"net"
	"time"
)

var (
	containerEventRecvCnt = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "primus_lens",
		Subsystem: "jobs",
		Name:      "container_event_recv_total",
		Help:      "Total number of container events received",
	}, []string{"source"})
	containerEventErrorCnt = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "primus_lens",
		Subsystem: "jobs",
		Name:      "container_event_error_total",
		Help:      "Total number of container event errors",
	}, []string{"source"})
	upstreamConnected = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "primus_lens",
		Subsystem: "jobs",
		Name:      "upstream_connected",
		Help:      "Whether the upstream is connected",
	}, []string{"addr", "source"})
	upstreamErrorCnt = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "primus_lens",
		Subsystem: "jobs",
		Name:      "upstream_error_total",
		Help:      "Total number of upstream errors",
	})
	eventProcessingDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "primus_lens",
		Subsystem: "jobs",
		Name:      "event_processing_duration_seconds",
		Help:      "Duration of event processing in seconds",
		Buckets:   prometheus.ExponentialBuckets(0.001, 2, 15), // from 1ms to ~16s
	}, []string{"addr", "source"})
)

func init() {
	prometheus.MustRegister(containerEventRecvCnt)
	prometheus.MustRegister(containerEventErrorCnt)
	prometheus.MustRegister(upstreamConnected)
	prometheus.MustRegister(eventProcessingDuration)
	prometheus.MustRegister(upstreamErrorCnt)
}

func StartServer(ctx context.Context, port int) error {
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return err
	}
	grpcServer := grpc.NewServer()
	pb.RegisterExporterServiceServer(grpcServer, &EventServer{})
	reflection.Register(grpcServer)
	log.Infof("Listening on port %d", port)
	go func() {
		err := grpcServer.Serve(lis)
		if err != nil {
			if err != io.EOF {
				log.Fatalf("failed to serve: %v", err)
			}
			log.Errorf("failed to serve: %v", err)
		}
	}()
	return nil
}

type EventServer struct {
	pb.UnimplementedExporterServiceServer
}

func (s *EventServer) StreamDockerContainerEvents(stream pb.ExporterService_StreamDockerContainerEventsServer) error {
	p, ok := peer.FromContext(stream.Context())
	if !ok {
		log.Errorf("failed to get peer from context")
		return errors.NewError().WithCode(errors.CodeRemoteServiceError).WithMessage("peer not found")
	}

	upstreamConnected.WithLabelValues(p.Addr.String(), "docker").Add(1.0)
	defer upstreamConnected.WithLabelValues(p.Addr.String(), "docker").Add(-1.0)
	for {
		evt, err := stream.Recv()
		if err == io.EOF {
			log.Errorf("Stream closed by client")
			return nil
		}
		if err != nil {
			log.Errorf("Stream recv error: %v", err)
			return err
		}
		start := time.Now()
		err = s.solveDockerContainerEvent(stream.Context(), evt)
		if err != nil {
			log.Errorf("Failed to solve container event: %v", err)
		}
		eventProcessingDuration.WithLabelValues(p.Addr.String(), "docker").Observe(time.Since(start).Seconds())
	}
}

func (s *EventServer) solveDockerContainerEvent(ctx context.Context, evt *pb.ContainerEvent) error {
	containerEventRecvCnt.WithLabelValues("docker").Inc()
	containerInfo := &model.DockerContainerInfo{}
	err := s.getFromEvent(evt, containerInfo)
	if err != nil {
		return err
	}
	existContainer, err := database.GetFacade().GetContainer().GetNodeContainerByContainerId(ctx, evt.ContainerId)
	if err != nil {
		log.Errorf("Failed to get container by containerId: %v", err)
		containerEventErrorCnt.WithLabelValues("docker").Inc()
		return err
	}
	if existContainer == nil {
		existContainer = &dbModel.NodeContainer{
			ContainerID:   containerInfo.ID,
			ContainerName: containerInfo.Name,
			PodUID:        "",
			PodName:       "",
			PodNamespace:  "",
			CreatedAt:     containerInfo.StartAt,
			UpdatedAt:     time.Now(),
			NodeName:      evt.Node,
			Source:        constant.ContainerSourceDocker,
		}
	}
	existContainer.Status = containerInfo.Status
	if existContainer.ID == 0 {
		err = database.GetFacade().GetContainer().CreateNodeContainer(ctx, existContainer)
	} else {
		err = database.GetFacade().GetContainer().UpdateNodeContainer(ctx, existContainer)
	}
	if err != nil {
		return err
	}
	// save device reference
	for _, device := range containerInfo.Devices {
		err = s.saveContainerDevice(ctx, evt.ContainerId, device)
		if err != nil {
			log.Errorf("Failed to save container device: %v", err)
			containerEventErrorCnt.WithLabelValues("docker").Inc()
			return err
		}
	}
	return nil
}

func (s *EventServer) saveContainerDevice(ctx context.Context, containerId string, device model.DockerDeviceInfo) error {
	existRecord, err := database.GetFacade().GetContainer().GetNodeContainerDeviceByContainerIdAndDeviceUid(ctx, containerId, device.DeviceSerial)
	if err != nil {
		log.Errorf("failed to get container device by container id %s and device uid %s: %v", containerId, device.DeviceSerial, err)
		return errors.NewError().WithCode(errors.CodeDatabaseError).WithMessagef("failed to get container device by container id %s and device uid %s", containerId, device.DeviceSerial)
	}
	if existRecord == nil {
		existRecord = &dbModel.NodeContainerDevices{
			ID:          0,
			ContainerID: containerId,
			DeviceType:  "",
			DeviceName:  device.DeviceName,
			DeviceNo:    int32(device.DeviceId),
			DeviceUUID:  device.DeviceSerial,
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}
		if device.DeviceType != "" {
			existRecord.DeviceType = device.DeviceType
		} else {
			existRecord.DeviceType = constant.DeviceTypeGPU
		}
		err = database.GetFacade().GetContainer().CreateNodeContainerDevice(ctx, existRecord)
		if err != nil {
			log.Errorf("failed to create node container device: %v", err)
			return errors.NewError().WithCode(errors.CodeDatabaseError).WithMessagef("failed to create node container device: %v", err)
		}
	}
	return nil
}

func (s *EventServer) getFromEvent(evt *pb.ContainerEvent, result interface{}) error {
	if evt == nil {
		return errors.NewError().WithCode(errors.CodeInvalidArgument).WithMessage("event is nil")
	}
	if evt.ContainerId == "" {
		return errors.NewError().WithCode(errors.CodeInvalidArgument).WithMessage("container_id is empty")
	}
	if evt.Data == nil {
		return errors.NewError().WithCode(errors.CodeInvalidArgument).WithMessage("event data is nil")
	}
	jsonData, err := json.Marshal(evt.Data)
	if err != nil {
		log.Errorf("failed to marshal event data: %v", err)
		return errors.NewError().WithCode(errors.CodeInvalidArgument).WithMessagef("failed to marshal event data: %v", err)
	}
	err = json.Unmarshal(jsonData, result)
	if err != nil {
		log.Errorf("failed to unmarshal event data: %v", err)
		return errors.NewError().WithCode(errors.CodeInvalidArgument).WithMessagef("failed to unmarshal event data: %v", err)
	}
	return nil
}

func (s *EventServer) StreamContainerEvents(stream pb.ExporterService_StreamContainerEventsServer) error {
	p, ok := peer.FromContext(stream.Context())
	if !ok {
		log.Errorf("failed to get peer from context")
		return errors.NewError().WithCode(errors.CodeRemoteServiceError).WithMessage("peer not found")
	}

	upstreamConnected.WithLabelValues(p.Addr.String(), "k8s").Add(1.0)
	defer upstreamConnected.WithLabelValues(p.Addr.String(), "k8s").Add(-1.0)
	for {
		evt, err := stream.Recv()
		if err == io.EOF {
			log.Errorf("Stream closed by client")
			return nil
		}
		if err != nil {
			log.Infof("Stream recv error: %v", err)
			return err
		}
		start := time.Now()
		err = s.solveContainerEvent(stream.Context(), evt)
		if err != nil {
			log.Errorf("Failed to solve container event: %v", err)
		}
		eventProcessingDuration.WithLabelValues(p.Addr.String(), "k8s").Observe(time.Since(start).Seconds())
	}
}

func (s *EventServer) solveContainerEvent(ctx context.Context, evt *pb.ContainerEvent) error {
	containerEventRecvCnt.WithLabelValues("k8s").Inc()
	container := &model.Container{}
	err := s.getFromEvent(evt, container)
	if err != nil {
		log.Errorf("failed to get container from event: %v", err)
		containerEventErrorCnt.WithLabelValues("k8s").Inc()
		return err
	}
	if container.Devices == nil || len(container.Devices.GPU) == 0 {
		return nil
	}
	// Check weather container exists
	existContainer, err := database.GetFacade().GetContainer().GetNodeContainerByContainerId(ctx, evt.ContainerId)
	if err != nil {
		log.Errorf("failed to get container by id %s: %v", evt.ContainerId, err)
		containerEventErrorCnt.WithLabelValues("k8s").Inc()
		return errors.NewError().WithCode(errors.CodeDatabaseError).WithMessagef("failed to get container by id %s", evt.ContainerId)
	}
	if existContainer == nil {
		existContainer = &dbModel.NodeContainer{
			ID:            0,
			ContainerID:   evt.ContainerId,
			ContainerName: container.ContainerStatus.Id,
			PodUID:        container.PodUuid,
			PodName:       container.PodName,
			PodNamespace:  container.PodNamespace,
			CreatedAt:     time.Unix(0, container.CreatedAt),
			UpdatedAt:     time.Now(),
			NodeName:      evt.Node,
			Source:        constant.ContainerSourceK8S,
		}
	}
	existContainer.Status = container.Status
	if existContainer.ID == 0 {
		err = database.GetFacade().GetContainer().CreateNodeContainer(ctx, existContainer)
	} else {
		existContainer.UpdatedAt = time.Now()
		err = database.GetFacade().GetContainer().UpdateNodeContainer(ctx, existContainer)
	}
	if err != nil {
		log.Errorf("failed to save container %s. created at %d: %v", evt.ContainerId, container.CreatedAt, err)
		containerEventErrorCnt.WithLabelValues("k8s").Inc()
		return errors.NewError().WithCode(errors.CodeDatabaseError).WithMessagef("failed to save container %s", evt.ContainerId)
	}

	// Save device reference
	if container.Devices != nil {
		for i := range container.Devices.GPU {
			gpu := container.Devices.GPU[i]
			existRecord, err := database.GetFacade().GetContainer().GetNodeContainerDeviceByContainerIdAndDeviceUid(ctx, evt.ContainerId, gpu.Serial)
			if err != nil {
				log.Errorf("failed to get container device by container id %s and device uid %s: %v", evt.ContainerId, gpu.Serial, err)
				containerEventErrorCnt.WithLabelValues("k8s").Inc()
				return errors.NewError().WithCode(errors.CodeDatabaseError).WithMessagef("failed to get container device by container id %s and device uid %s", evt.ContainerId, gpu.Serial)
			}
			if existRecord == nil {
				existRecord = &dbModel.NodeContainerDevices{
					ID:          0,
					ContainerID: evt.ContainerId,
					DeviceType:  constant.DeviceTypeGPU,
					DeviceName:  gpu.Name,
					DeviceNo:    int32(gpu.Id),
					DeviceUUID:  gpu.Serial,
					CreatedAt:   time.Now(),
					UpdatedAt:   time.Now(),
				}
				err = database.GetFacade().GetContainer().CreateNodeContainerDevice(ctx, existRecord)
				if err != nil {
					log.Errorf("failed to create node container device: %v", err)
					containerEventErrorCnt.WithLabelValues("k8s").Inc()
					return errors.NewError().WithCode(errors.CodeDatabaseError).WithMessagef("failed to create node container device: %v", err)
				}
			}
		}
		for i := range container.Devices.Infiniband {
			ib := container.Devices.Infiniband[i]
			existRecord, err := database.GetFacade().GetContainer().GetNodeContainerDeviceByContainerIdAndDeviceUid(ctx, evt.ContainerId, ib.Serial)
			if err != nil {
				containerEventErrorCnt.WithLabelValues("k8s").Inc()
				log.Errorf("failed to get container device by container id %s and device uid %s: %v", evt.ContainerId, ib.Serial, err)
				return errors.NewError().WithCode(errors.CodeDatabaseError).WithMessagef("failed to get container device by container id %s and device uid %s", evt.ContainerId, ib.Serial)
			}
			if existRecord == nil {
				existRecord = &dbModel.NodeContainerDevices{
					ID:          0,
					ContainerID: evt.ContainerId,
					DeviceType:  constant.DeviceTypeIB,
					DeviceName:  ib.Name,
					DeviceNo:    int32(ib.Id),
					DeviceUUID:  ib.Serial,
					CreatedAt:   time.Now(),
					UpdatedAt:   time.Now(),
				}
				err = database.GetFacade().GetContainer().CreateNodeContainerDevice(ctx, existRecord)
				if err != nil {
					containerEventErrorCnt.WithLabelValues("k8s").Inc()
					log.Errorf("failed to create node container device: %v", err)
					return errors.NewError().WithCode(errors.CodeDatabaseError).WithMessagef("failed to create node container device: %v", err)
				}
			}
		}
	}
	// Save container event
	if evt.Type != model.ContainerEventTypeSnapshot {
		event := &dbModel.NodeContainerEvent{
			ContainerID: evt.ContainerId,
			EventType:   evt.Type,
			CreatedAt:   time.Now(),
		}
		err := database.GetFacade().GetContainer().CreateNodeContainerEvent(ctx, event)
		if err != nil {
			log.Errorf("failed to create node container event: %v", err)
			containerEventErrorCnt.WithLabelValues("k8s").Inc()
			return errors.NewError().WithCode(errors.CodeDatabaseError).WithMessagef("failed to create node container event: %v", err)
		}
	}
	return nil
}
