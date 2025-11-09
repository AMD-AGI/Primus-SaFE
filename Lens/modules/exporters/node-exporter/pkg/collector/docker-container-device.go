package collector

import (
	"context"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/constant"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model"
	pb "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/pb/exporter"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/utils/mapUtil"
	"github.com/AMD-AGI/Primus-SaFE/Lens/node-exporter/pkg/collector/docker"
	"github.com/AMD-AGI/Primus-SaFE/Lens/node-exporter/pkg/collector/report"
	"google.golang.org/protobuf/types/known/structpb"
	"os"
	"strings"
)

func TryInitDocker(ctx context.Context, sockPath string) error {
	if _, err := os.Stat(sockPath); os.IsNotExist(err) {
		log.Warnf("The docker daemon socket file %s does not exist", sockPath)
		return nil
	}
	dockerHost := "unix://" + sockPath
	if err := docker.Init(dockerHost); err != nil {
		return err
	}

	log.Info("Docker client initialized successfully.")
	runReportSnapshot(ctx)
	return nil
}

func runReportSnapshot(ctx context.Context) {
	containers, err := snapshotDockerContainers(ctx)
	if err != nil {
		log.Errorf("Error getting docker containers %v", err)
		return
	}
	log.Infof("Got docker container %d", len(containers))
	for _, container := range containers {
		log.Infof("Report docker container %s.Name %s.Gpu count %d",
			container.ID,
			container.Name,
			container.GpuDeviceCount())
		reportDockerContainer(ctx, &container, model.ContainerEventTypeSnapshot)
	}
}

func snapshotDockerContainers(ctx context.Context) ([]model.DockerContainerInfo, error) {
	dockerContainers, err := docker.GetContainers()
	if err != nil {
		return nil, err
	}
	for i := range dockerContainers {
		for j := range (&dockerContainers[i]).Devices {
			fillGPUDeviceInfoForDockerContainerInfo(&(dockerContainers[i]).Devices[j])
			fillRDMADeviceInfoForDockerContainerInfo(&(dockerContainers[i]).Devices[j])
		}
	}
	return dockerContainers, nil
}

func fillGPUDeviceInfoForDockerContainerInfo(info *model.DockerDeviceInfo) {
	if _, ok := driCardInfoMapping[info.PathOnHost]; !ok {
		return
	}
	deviceDetail := driCardInfoMapping[info.PathOnHost]
	info.DeviceId = deviceDetail.GPU
	info.DeviceSerial = deviceDetail.Asic.AsicSerial
	info.DeviceName = deviceDetail.Asic.MarketName
	info.DeviceType = constant.DeviceTypeGPU
}

func fillRDMADeviceInfoForDockerContainerInfo(info *model.DockerDeviceInfo) {
	if !strings.Contains(info.PathOnHost, "uverbs") {
		return
	}
	deviceIdStr := strings.Split(info.PathOnHost, "uverbs")[1]
	if existDevice, ok := rdmaDeviceMapping[deviceIdStr]; ok {
		info.DeviceType = constant.DeviceTypeRDMA
		info.DeviceId = existDevice.IfIndex
		info.DeviceName = existDevice.IfName
		info.DeviceSerial = existDevice.SysImageGUID
	}
}

func reportDockerContainer(ctx context.Context, info *model.DockerContainerInfo, typ string) {
	go func() {
		containerMap, err := mapUtil.EncodeMap(info)
		if err != nil {
			log.Errorf("Failed to encode container info: %v", err)
			return
		}
		pbStruct, err := structpb.NewStruct(containerMap)
		if err != nil {
			log.Errorf("Failed to encode container info: %v", err)
			return
		}
		err = report.GetDockerStreamClient().Send(&pb.ContainerEvent{
			Type:        typ,
			ContainerId: info.ID,
			Data:        pbStruct,
			Node:        nodeName,
		})
		if err != nil {
			log.Errorf("Failed to send container event: %v", err)
		}
	}()
}
