// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package collector

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/constant"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/node-exporter/pkg/collector/containerd"
	"github.com/AMD-AGI/Primus-SaFE/Lens/node-exporter/pkg/collector/report"
	"github.com/containerd/containerd/api/events"
	containerdEvents "github.com/containerd/containerd/events"
	"github.com/containerd/containerd/namespaces"
	"github.com/containerd/typeurl/v2"
)

func GetContainerInfo(ctx context.Context) ([]model.Container, error) {
	return snapShotContainers(ctx)
}

func runEventListener(ctx context.Context) {
	_ = reportSnapshot(ctx)
	startContainerdWatcher(ctx)
	startPeriodicSnapshot(ctx)
}

const snapshotInterval = 5 * time.Minute

func startPeriodicSnapshot(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(snapshotInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				log.Infof("periodic snapshot stopped")
				return
			case <-ticker.C:
				if err := reportSnapshot(ctx); err != nil {
					log.Errorf("periodic snapshot failed: %v", err)
				} else {
					log.Infof("periodic snapshot completed")
				}
			}
		}
	}()
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
		
		// Use HTTP reporter to send snapshot event
		err = report.ReportContainer(ctx, &container, model.ContainerEventTypeSnapshot)
		if err != nil {
			log.Errorf("Failed to report container snapshot: container=%s, pod=%s, error=%v", 
				container.Id, container.PodName, err)
			continue
		}
		log.Infof("Container %s(pod name %s) snapshot reported", container.Id, container.PodName)
	}
	
	// Flush any buffered events to ensure snapshot completes
	report.FlushEvents()
	
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
	go func() {
		const (
			minBackoff = 2 * time.Second
			maxBackoff = 60 * time.Second
		)
		backoff := minBackoff

		for {
			select {
			case <-ctx.Done():
				log.Infof("containerd watcher stopped: context cancelled")
				return
			default:
			}

			nsCtx := namespaces.WithNamespace(ctx, "k8s.io")
			ch, errCh := containerd.EventService().Subscribe(nsCtx)
			log.Infof("containerd event watcher (re)started")
			backoff = minBackoff

			if err := watchContainerdEvents(nsCtx, ch, errCh); err != nil {
				log.Errorf("containerd event stream error: %v, reconnecting in %v", err, backoff)
			}

			_ = reportSnapshot(ctx)

			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
			}
			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
		}
	}()
}

func watchContainerdEvents(ctx context.Context, ch <-chan *containerdEvents.Envelope, errCh <-chan error) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-errCh:
			return err
		case evt := <-ch:
			ev, err := typeurl.UnmarshalAny(evt.Event)
			if err != nil {
				log.Errorf("unmarshal error: %v", err)
				continue
			}
			handleContainerdEvent(ctx, ev)
		}
	}
}

func handleContainerdEvent(ctx context.Context, ev interface{}) {
	switch e := ev.(type) {
	case *events.ContainerCreate:
		_, err := getAndReportContainerInfo(ctx, e.ID, func(container *model.Container) {
			container.Status = constant.ContainerStatusCreated
		})
		if err != nil {
			log.Errorf("Failed to get and report container info for %s: %v", e.ID, err)
		} else {
			log.Infof("Container %s created and reported", e.ID)
		}
	case *events.ContainerDelete:
		_, err := getAndReportContainerInfo(ctx, e.ID, func(container *model.Container) {
			container.Status = constant.ContainerStatusDeleted
		})
		if err != nil {
			log.Errorf("Failed to get and report container info for %s: %v", e.ID, err)
		} else {
			log.Infof("Container %s deleted and reported", e.ID)
		}
	case *events.TaskCreate:
		_, err := getAndReportContainerInfo(ctx, e.ContainerID, func(container *model.Container) {
			container.Status = constant.ContainerStatusCreated
		})
		if err != nil {
			log.Errorf("Failed to get and report container info for %s: %v", e.ContainerID, err)
		} else {
			log.Infof("Container %s task created and reported", e.ContainerID)
		}
	case *events.TaskStart:
		_, err := getAndReportContainerInfo(ctx, e.ContainerID, func(container *model.Container) {
			container.Status = constant.ContainerStatusRunning
		})
		if err != nil {
			log.Errorf("Failed to get and report container info for %s: %v", e.ContainerID, err)
		} else {
			log.Infof("Container %s task started and reported", e.ContainerID)
		}
	case *events.TaskExit:
		_, err := getAndReportContainerInfo(ctx, e.ContainerID, func(container *model.Container) {
			container.Status = constant.ContainerStatusExit
			container.ExitCode = int32(e.ExitStatus)
			container.ExitTime = e.ExitedAt.AsTime()
		})
		if err != nil {
			log.Errorf("Failed to get and report container info for %s: %v", e.ContainerID, err)
		} else {
			log.Infof("Container %s task exited and reported", e.ContainerID)
		}
	case *events.TaskDelete:
		_, err := getAndReportContainerInfo(ctx, e.ContainerID, func(container *model.Container) {
			container.Status = constant.ContainerStatusDeleted
			container.ExitCode = int32(e.ExitStatus)
			container.ExitTime = e.ExitedAt.AsTime()
		})
		if err != nil {
			log.Errorf("Failed to get and report container info for %s: %v", e.ContainerID, err)
		} else {
			log.Infof("Container %s task deleted and reported", e.ContainerID)
		}
	case *events.TaskOOM:
		_, err := getAndReportContainerInfo(ctx, e.ContainerID, func(container *model.Container) {
			container.Status = constant.ContainerStatusOOMKilled
			container.OOMKilled = true
		})
		if err != nil {
			log.Errorf("Failed to get and report container info for %s: %v", e.ContainerID, err)
		} else {
			log.Infof("Container %s OOM killed and reported", e.ContainerID)
		}
	default:
	}
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
		err := report.ReportContainer(context.Background(), container, typ)
		if err != nil {
			log.Errorf("Failed to report container event: container=%s, pod=%s, type=%s, error=%v", 
				container.Id, container.PodName, typ, err)
		} else {
			log.Debugf("Successfully reported container event: container=%s, pod=%s, type=%s", 
				container.Id, container.PodName, typ)
		}
	}()
}
