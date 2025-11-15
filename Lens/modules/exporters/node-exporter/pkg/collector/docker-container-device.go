package collector

import (
	"context"
	"os"
	"strings"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/constant"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/node-exporter/pkg/collector/docker"
	"github.com/AMD-AGI/Primus-SaFE/Lens/node-exporter/pkg/collector/report"
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
	// Find the last occurrence of "uverbs" and extract the device ID after it
	parts := strings.Split(info.PathOnHost, "uverbs")
	if len(parts) < 2 {
		return
	}
	// Take the last part after the last "uverbs"
	deviceIdStr := parts[len(parts)-1]
	if existDevice, ok := rdmaDeviceMapping[deviceIdStr]; ok {
		info.DeviceType = constant.DeviceTypeRDMA
		info.DeviceId = existDevice.IfIndex
		info.DeviceName = existDevice.IfName
		info.DeviceSerial = existDevice.SysImageGUID
	}
}

func reportDockerContainer(ctx context.Context, info *model.DockerContainerInfo, typ string) {
	go func() {
		err := report.ReportDockerContainer(ctx, info, typ)
		if err != nil {
			log.Errorf("Failed to report docker container event: %v", err)
		} else {
			log.Debugf("Successfully reported docker container event: container=%s, type=%s", info.ID, typ)
		}
	}()
}
