/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package ops_job

import (
	"context"

	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

func CleanupJobRelatedInfo(ctx context.Context, cli client.Client, opsJobId string) error {
	labelSelector := labels.SelectorFromSet(map[string]string{v1.OpsJobIdLabel: opsJobId})

	workloadList := &v1.WorkloadList{}
	if err := cli.List(ctx, workloadList, &client.ListOptions{LabelSelector: labelSelector}); err != nil {
		return err
	}
	for _, workload := range workloadList.Items {
		if err := cli.Delete(ctx, &workload); err != nil {
			klog.ErrorS(err, "failed to delete workload")
		}
	}

	faultList := &v1.FaultList{}
	if err := cli.List(ctx, faultList, &client.ListOptions{LabelSelector: labelSelector}); err != nil {
		return err
	}
	for _, fault := range faultList.Items {
		if err := cli.Delete(ctx, &fault); err != nil {
			klog.ErrorS(err, "failed to delete fault")
		}
	}
	return nil
}
