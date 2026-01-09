// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package kubelet

import (
	"context"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/helper/kubelet"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

var (
	kubeletClient *clientsets.Client
	nodeName      string
)

func Init(ctx context.Context, nodeNameParam string) error {
	nodeName = nodeNameParam
	node := &corev1.Node{}
	err := clientsets.GetClusterManager().GetCurrentClusterClients().K8SClientSet.ControllerRuntimeClient.Get(ctx, types.NamespacedName{Name: nodeName}, node)
	if err != nil {
		return err
	}
	// Use current cluster (empty string for default authentication)
	clusterName := clientsets.GetClusterManager().GetCurrentClusterName()
	kubeletClient, err = kubelet.GetKubeletClient(node, clusterName)
	if err != nil {
		return err
	}
	return nil
}

func GetKubeletClient() *clientsets.Client {
	return kubeletClient
}

func GetNodeName() string {
	return nodeName
}
