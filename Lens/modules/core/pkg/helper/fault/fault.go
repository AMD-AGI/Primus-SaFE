// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package fault

import (
	"context"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/utils/k8sUtil"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

// GetFaultyNodes gets the faulty nodes
// It returns the faulty nodes and an error if any
func GetFaultyNodes(ctx context.Context, clientsets *clientsets.K8SClientSet, nodes []string) ([]string, error) {
	faulty := []string{}
	for _, nodeName := range nodes {
		log.Infof("Checking node %s", nodeName)
		node := corev1.Node{}
		err := clientsets.ControllerRuntimeClient.Get(ctx, types.NamespacedName{Name: nodeName}, &node)
		if err != nil {
			log.Errorf("Get node %s error: %v", nodeName, err)
			return nil, err
		}
		if len(node.Spec.Taints) > 0 {
			faulty = append(faulty, nodeName)
			log.Infof("Node %s is faulty", nodeName)
			continue
		}
		if !k8sUtil.NodeReady(node) {
			faulty = append(faulty, nodeName)
			log.Infof("Node %s is faulty", nodeName)
			continue
		}
		log.Infof("Node %s is not faulty", nodeName)
	}
	return faulty, nil
}
