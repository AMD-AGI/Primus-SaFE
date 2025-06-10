/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package cluster

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
)

func GetEndpoint(ctx context.Context, cli client.Client, clusterName string, endpoints []string) (string, error) {
	service := &corev1.Service{}
	err := cli.Get(ctx, client.ObjectKey{Name: clusterName, Namespace: common.PrimusSafeNamespace}, service)
	result := ""
	if err == nil {
		result = fmt.Sprintf("https://%s.%s.svc", clusterName, common.PrimusSafeNamespace)
	} else {
		if len(endpoints) == 0 {
			return "", fmt.Errorf("either the Service address or the Endpoint is empty")
		}
		result = endpoints[0]
	}
	return result, nil
}
