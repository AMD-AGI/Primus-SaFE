package collector

import (
	"context"
	"encoding/json"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/constant"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model"
	pb "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/pb/exporter"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/utils/mapUtil"
	"github.com/AMD-AGI/Primus-SaFE/Lens/node-exporter/pkg/collector/containerd"
	"github.com/AMD-AGI/Primus-SaFE/Lens/node-exporter/pkg/collector/report"
	"github.com/containerd/containerd/api/events"
	"github.com/containerd/containerd/namespaces"
	"github.com/containerd/typeurl/v2"
	"google.golang.org/protobuf/types/known/structpb"
	"strings"
)

func GetContainerInfo(ctx context.Context) ([]model.Container, error) {
	return snapShotContainers(ctx)
}

func runEventListener(ctx context.Context) {
	_ = reportSnapshot(ctx)
	startContainerdWatcher(ctx)
}

func reportSnapshot(ctx context.Context) error {
	containers, err := snapShotContainers(ctx)
	if err != nil {
		log.Errorf("Failed to snapshot containers: %v", err)
		return err
	}

	for _, container := range containers {
		if !container.HasGpu() {
			log.Debugf("Container %s(pod name %s) does not have GPU, skipping", container.Id, container.PodName)
			continue
		}
		containerMap, err := mapUtil.EncodeMap(container)
		if err != nil {
			log.Errorf("Failed to encode container info: %v", err)
			continue
		}
		pbStruct, err := structpb.NewStruct(containerMap)
		if err != nil {
			log.Errorf("Failed to encode container info: %v", err)
			continue
		}
		err = report.GetStreamClient().Send(&pb.ContainerEvent{
			Type:        model.ContainerEventTypeSnapshot,
			ContainerId: container.Id,
			Data:        pbStruct,
		})
		if err != nil {
			log.Errorf("Failed to send container event: %v", err)
			continue
		}
		log.Infof("Container %s(pod name %s) snapshot reported", container.Id, container.PodName)
	}
	return nil
}

func readContainerInfoFromContainerd(ctx context.Context, containerId string) (*model.Container, error) {
	result := &model.Container{
		Devices: &model.ContainerDevices{
			GPU:        make([]*model.DeviceInfo, 0),
			Infiniband: make([]*model.DeviceInfo, 0),
		},
	}
	status, err := containerd.ContainerStatus(ctx, containerId)
	if err != nil {
		return nil, err
	}
	result.PodName = status.Status.Labels[constant.ContainerdK8SPodName]
	result.PodNamespace = status.Status.Labels[constant.ContainerdK8SPodNamespace]
	result.PodUuid = status.Status.Labels[constant.ContainerdK8SPodUid]
	info := &model.ContainerInfo{}
	if infoStr, ok := status.Info["info"]; ok {
		err := json.Unmarshal([]byte(infoStr), info)
		if err != nil {
			info = nil
		} else {
			if info.RuntimeSpec != nil && info.RuntimeSpec.Linux != nil {
				for _, device := range info.RuntimeSpec.Linux.Devices {
					if driInfo, ok := cardDriDeviceMapping[device.Path]; ok {
						// gpu
						cardInfo := driCardInfoMapping[device.Path]
						result.Devices.GPU = append(result.Devices.GPU, &model.DeviceInfo{
							Name:   cardInfo.Asic.MarketName,
							Id:     cardInfo.GPU,
							Path:   driInfo.Card,
							Type:   device.Type,
							Kind:   "GPU",
							UUID:   cardInfo.Asic.DeviceID,
							Serial: cardInfo.Asic.AsicSerial,
							Slot:   cardInfo.Bus.BDF,
						})
					}
					if !strings.Contains(device.Path, "uverbs") {
						continue
					}
					deviceIdStr := strings.Split(device.Path, "uverbs")[1]
					if rdmaInfo, ok := rdmaDeviceMapping[deviceIdStr]; ok {
						result.Devices.Infiniband = append(result.Devices.Infiniband, &model.DeviceInfo{
							Name:   rdmaInfo.IfName,
							Id:     rdmaInfo.IfIndex,
							Path:   device.Path,
							Type:   "",
							Kind:   constant.DeviceTypeRDMA,
							UUID:   rdmaInfo.NodeGUID,
							Serial: rdmaInfo.IfName,
							Slot:   deviceIdStr,
						})
					}
				}
			}
		}
	} else {
		info = nil
	}
	result.ContainerStatus = *status.Status
	result.Info = info
	return result, nil
}

func startContainerdWatcher(ctx context.Context) {
	ctx = namespaces.WithNamespace(ctx, "k8s.io")
	ch, errCh := containerd.EventService().Subscribe(ctx)

	go func() {
		for {
			select {
			case evt := <-ch:
				ev, err := typeurl.UnmarshalAny(evt.Event)
				if err != nil {
					log.Errorf("unmarshal error: %v", err)
					continue
				}
				switch e := ev.(type) {
				case *events.ContainerCreate:
					_, err = getAndReportContainerInfo(ctx, e.ID, func(container *model.Container) {
						container.Status = constant.ContainerStatusCreated
					})
					if err != nil {
						log.Errorf("Failed to get and report container info for %s: %v", e.ID, err)
					} else {
						log.Infof("Container %s created and reported", e.ID)
					}
				case *events.ContainerDelete:
					_, err = getAndReportContainerInfo(ctx, e.ID, func(container *model.Container) {
						container.Status = constant.ContainerStatusDeleted
					})
					if err != nil {
						log.Errorf("Failed to get and report container info for %s: %v", e.ID, err)
					} else {
						log.Infof("Container %s created and reported", e.ID)
					}

				case *events.TaskCreate:
					_, err = getAndReportContainerInfo(ctx, e.ContainerID, func(container *model.Container) {
						container.Status = constant.ContainerStatusCreated
					})
					if err != nil {
						log.Errorf("Failed to get and report container info for %s: %v", e.ContainerID, err)
					} else {
						log.Infof("Container %s created and reported", e.ContainerID)
					}

				case *events.TaskStart:
					_, err = getAndReportContainerInfo(ctx, e.ContainerID, func(container *model.Container) {
						container.Status = constant.ContainerStatusRunning
					})
					if err != nil {
						log.Errorf("Failed to get and report container info for %s: %v", e.ContainerID, err)
					} else {
						log.Infof("Container %s created and reported", e.ContainerID)
					}

				case *events.TaskExit:
					_, err = getAndReportContainerInfo(ctx, e.ContainerID, func(container *model.Container) {
						container.Status = constant.ContainerStatusExit
						container.ExitCode = int32(e.ExitStatus)
						container.ExitTime = e.ExitedAt.AsTime()
					})
					if err != nil {
						log.Errorf("Failed to get and report container info for %s: %v", e.ContainerID, err)
					} else {
						log.Infof("Container %s created and reported", e.ContainerID)
					}
				case *events.TaskDelete:
					_, err = getAndReportContainerInfo(ctx, e.ContainerID, func(container *model.Container) {
						container.Status = constant.ContainerStatusDeleted
						container.ExitCode = int32(e.ExitStatus)
						container.ExitTime = e.ExitedAt.AsTime()
					})
					if err != nil {
						log.Errorf("Failed to get and report container info for %s: %v", e.ContainerID, err)
					} else {
						log.Infof("Container %s created and reported", e.ContainerID)
					}
				case *events.TaskOOM:
					_, err = getAndReportContainerInfo(ctx, e.ContainerID, func(container *model.Container) {
						container.Status = constant.ContainerStatusOOMKilled
						container.OOMKilled = true
					})
					if err != nil {
						log.Errorf("Failed to get and report container info for %s: %v", e.ContainerID, err)
					} else {
						log.Infof("Container %s created and reported", e.ContainerID)
					}
				default:
				}

			case err := <-errCh:
				log.Errorf("event stream error: %v", err)
				return
			}
		}
	}()
}

func getAndReportContainerInfo(ctx context.Context, containerId string, updateHook func(container *model.Container)) (*model.Container, error) {
	container, err := readContainerInfoFromContainerd(ctx, containerId)
	if err != nil {
		log.Errorf("Failed to read container info for %s: %v", containerId, err)
		return nil, err
	}
	if container == nil {
		log.Warnf("Container %s not found in containerd", containerId)
		return nil, err
	}
	if !container.HasGpu() {
		return nil, nil
	}
	if updateHook != nil {
		updateHook(container)
	}

	reportContainerUpdate(container, container.Status)
	return container, nil
}

func snapShotContainers(ctx context.Context) ([]model.Container, error) {
	existContainers, err := containerd.ListContainers(ctx)
	if err != nil {
		log.Errorf("Failed to list containers: %v", err)
		return nil, err
	}
	result := []model.Container{}
	for _, container := range existContainers {
		containerInfo, err := readContainerInfoFromContainerd(ctx, container.Id)
		if err != nil {
			log.Errorf("Failed to read container info: %v", err)
			continue
		}
		if containerInfo == nil {
			log.Warnf("Container %s not found in containerd", container.Id)
			continue
		}
		result = append(result, *containerInfo)
	}
	return result, nil
}

func reportContainerUpdate(container *model.Container, typ string) {
	go func() {
		containerMap, err := mapUtil.EncodeMap(container)
		if err != nil {
			log.Errorf("Failed to encode container info: %v", err)
			return
		}
		pbStruct, err := structpb.NewStruct(containerMap)
		if err != nil {
			log.Errorf("Failed to encode container info: %v", err)
			return
		}
		err = report.GetStreamClient().Send(&pb.ContainerEvent{
			Type:        typ,
			ContainerId: container.Id,
			Data:        pbStruct,
			Node:        nodeName,
		})
		if err != nil {
			log.Errorf("Failed to send container event: %v", err)
		}
	}()
}
