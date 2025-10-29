package containerd

import (
	"context"
	"github.com/AMD-AGI/primus-lens/core/pkg/errors"
	"github.com/AMD-AGI/primus-lens/core/pkg/logger/log"
	"github.com/containerd/containerd"
	runtimeapi "k8s.io/cri-api/pkg/apis/runtime/v1"
)

var (
	containerdApi *containerd.Client
	criApi        runtimeapi.RuntimeServiceClient
)

func Init(ctx context.Context, path string) error {
	var err error
	containerdApi, err = containerd.New(path)
	if err != nil {
		log.Errorf("containerd init err: %v", err)
		return err
	}
	criApi = runtimeapi.NewRuntimeServiceClient(containerdApi.Conn())
	return nil
}

func ListContainers(ctx context.Context) ([]*runtimeapi.Container, error) {
	containers, err := criApi.ListContainers(ctx, &runtimeapi.ListContainersRequest{})
	if err != nil {
		log.Errorf("ListContainers error: %s", err)
		return nil, err
	}
	return containers.Containers, nil
}

func ContainerStatus(ctx context.Context, containerId string) (*runtimeapi.ContainerStatusResponse, error) {
	status, err := criApi.ContainerStatus(ctx, &runtimeapi.ContainerStatusRequest{
		ContainerId: containerId,
		Verbose:     true,
	})
	if err != nil {
		return nil, err
	}
	if status.Status == nil {
		return nil, errors.NewError().WithMessagef("Cannot get container status for %s", containerId)
	}
	if status.Status.Labels == nil {
		return nil, errors.NewError().WithMessagef("Container %s has no containerName", containerId)
	}
	return status, nil
}

func EventService() containerd.EventService {
	return containerdApi.EventService()
}
