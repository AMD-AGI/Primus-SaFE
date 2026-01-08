// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package processtree

import (
	"context"
	"fmt"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/node-exporter/pkg/collector/containerd"
)

// ContainerdReader reads container information from containerd
type ContainerdReader struct{}

// NewContainerdReader creates a new containerd reader
func NewContainerdReader() (*ContainerdReader, error) {
	return &ContainerdReader{}, nil
}

// GetPodContainers retrieves all containers for a pod
func (r *ContainerdReader) GetPodContainers(ctx context.Context, podUID string) ([]*ContainerInfo, error) {
	// Get all containers using existing containerd package
	containers, err := containerd.ListContainers(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list containers: %w", err)
	}
	
	var podContainers []*ContainerInfo
	for _, container := range containers {
		// Check if container belongs to the pod
		labels := container.GetLabels()
		if labels == nil {
			continue
		}
		
		// Check pod UID in labels
		containerPodUID := labels["io.kubernetes.pod.uid"]
		if containerPodUID != podUID {
			continue
		}
		
		// Extract container information
		info := &ContainerInfo{
			ID:   container.GetId(),
			Name: labels["io.kubernetes.container.name"],
		}
		
		// Get image information
		imageRef := container.GetImageRef()
		if imageRef != "" {
			info.Image = imageRef
		} else if container.GetImage() != nil {
			info.Image = container.GetImage().GetImage()
		}
		
		podContainers = append(podContainers, info)
	}
	
	if len(podContainers) == 0 {
		return nil, fmt.Errorf("no containers found for pod %s", podUID)
	}
	
	log.Debugf("Found %d containers for pod %s", len(podContainers), podUID)
	return podContainers, nil
}

// GetContainerInfo retrieves information for a specific container
func (r *ContainerdReader) GetContainerInfo(ctx context.Context, containerID string) (*ContainerInfo, error) {
	// Get container status using existing containerd package
	status, err := containerd.ContainerStatus(ctx, containerID)
	if err != nil {
		return nil, fmt.Errorf("failed to get container status: %w", err)
	}
	
	containerStatus := status.GetStatus()
	if containerStatus == nil {
		return nil, fmt.Errorf("container status is nil")
	}
	
	labels := containerStatus.GetLabels()
	info := &ContainerInfo{
		ID: containerStatus.GetId(),
	}
	
	if labels != nil {
		info.Name = labels["io.kubernetes.container.name"]
	}
	
	// Get image
	if containerStatus.GetImageRef() != "" {
		info.Image = containerStatus.GetImageRef()
	} else if containerStatus.GetImage() != nil {
		info.Image = containerStatus.GetImage().GetImage()
	}
	
	return info, nil
}


