/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package cluster

import (
	"context"
	"fmt"

	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

// GetEndpoint retrieve the endpoint address of the given cluster.
// It first tries to get the endpoint from the Kubernetes Service associated with the cluster.
// If the Service is not found or has no ports, it falls back to using the endpoint from the cluster status.
// Returns an error if the cluster is nil, not ready, or no valid endpoint can be found.
func GetEndpoint(ctx context.Context, cli client.Client, cluster *v1.Cluster) (string, error) {
	if cluster == nil || !cluster.IsReady() {
		return "", fmt.Errorf("cluster is not ready")
	}
	service := &corev1.Service{}
	err := cli.Get(ctx, client.ObjectKey{Name: cluster.Name, Namespace: common.PrimusSafeNamespace}, service)
	if err == nil {
		if len(service.Spec.Ports) == 0 {
			return "", fmt.Errorf("service ports are empty")
		}
		// return fmt.Sprintf("https://%s.%s.svc", clusterName, common.PrimusSafeNamespace), nil
		return fmt.Sprintf("%s:%d", service.Spec.ClusterIP, service.Spec.Ports[0].Port), nil
	}
	if len(cluster.Status.ControlPlaneStatus.Endpoints) == 0 {
		return "", fmt.Errorf("either the Service address or the Endpoint is empty")
	}
	return cluster.Status.ControlPlaneStatus.Endpoints[0], nil
}
