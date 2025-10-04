/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package cluster

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
)

func GetEndpoint(ctx context.Context, cli client.Client, cluster *v1.Cluster) (string, error) {
	if cluster == nil || !cluster.IsReady() {
		return "", fmt.Errorf("cluster is not ready")
	}
	service := &corev1.Service{}
	err := cli.Get(ctx, client.ObjectKey{Name: cluster.Name, Namespace: common.PrimusSafeNamespace}, service)
	result := ""
	if err == nil {
		// result = fmt.Sprintf("https://%s.%s.svc", clusterName, common.PrimusSafeNamespace)
		result = fmt.Sprintf("%s:%d", service.Spec.ClusterIP, service.Spec.Ports[0].Port)
	} else {
		if len(cluster.Status.ControlPlaneStatus.Endpoints) == 0 {
			return "", fmt.Errorf("either the Service address or the Endpoint is empty")
		}
		result = cluster.Status.ControlPlaneStatus.Endpoints[0]
	}
	return result, nil
}
