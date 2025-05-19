/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package cluster

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
)

func GetClusterEndpoint(ctx context.Context, cli client.Client, clusterName string, endpoints []string) (string, error) {
	service := new(corev1.Service)
	err := cli.Get(ctx, types.NamespacedName{
		Name:      clusterName,
		Namespace: common.PrimusSafeNamespace,
	}, service)
	serviceUrl := ""
	if err == nil {
		serviceUrl = fmt.Sprintf("https://%s.%s.svc", clusterName, common.PrimusSafeNamespace)
	} else {
		if len(endpoints) == 0 {
			return "", fmt.Errorf("cluster %s has no endpoints", clusterName)
		}
		serviceUrl = endpoints[0]
	}
	return serviceUrl, nil
}
