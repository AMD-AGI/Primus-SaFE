package docker

import (
	"context"
	"fmt"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
)

var (
	cli *client.Client
)

func Init(dockerHost string) error {
	var err error
	cli, err = client.NewClientWithOpts(
		client.WithHost(dockerHost),
		client.WithAPIVersionNegotiation(),
	)
	if err != nil {
		return fmt.Errorf("failed to initialize docker client: %w", err)
	}
	return nil
}

func GetContainers() ([]model.DockerContainerInfo, error) {
	if cli == nil {
		return nil, fmt.Errorf("docker client not initialized")
	}
	ctx := context.Background()

	containers, err := cli.ContainerList(ctx, types.ContainerListOptions{All: true})
	if err != nil {
		return nil, fmt.Errorf("failed to list containers: %w", err)
	}

	var results []model.DockerContainerInfo
	for _, c := range containers {
		info, err := cli.ContainerInspect(ctx, c.ID)
		if err != nil {
			return nil, fmt.Errorf("failed to inspect container %s: %w", c.ID, err)
		}

		var devices []model.DockerDeviceInfo
		for _, dev := range info.HostConfig.Devices {
			devices = append(devices, model.DockerDeviceInfo{
				PathOnHost:        dev.PathOnHost,
				PathInContainer:   dev.PathInContainer,
				CgroupPermissions: dev.CgroupPermissions,
			})
		}

		var mounts []model.DockerMountInfo
		for _, m := range info.Mounts {
			mounts = append(mounts, model.DockerMountInfo{
				Type:        string(m.Type),
				Source:      m.Source,
				Destination: m.Destination,
			})
		}

		name := ""
		if len(c.Names) > 0 {
			name = c.Names[0]
		}
		start, err := time.Parse(info.State.StartedAt, time.RFC3339Nano)
		if err != nil {
			start = time.Now()
		}
		results = append(results, model.DockerContainerInfo{
			ID:      c.ID,
			Name:    name,
			Labels:  c.Labels,
			Cmd:     c.Command,
			Devices: devices,
			Mounts:  mounts,
			StartAt: start,
		})
	}

	return results, nil
}
