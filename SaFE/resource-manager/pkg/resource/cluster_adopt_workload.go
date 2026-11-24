/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resource

import (
	"context"
	"errors"
	"fmt"

	"k8s.io/client-go/dynamic"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/resource-manager/pkg/adopt"
	"github.com/AMD-AIG-AIMA/SAFE/resource-manager/pkg/utils"
)

// importAdoptedWorkloads imports the unfinished Workloads of the data plane SaFE whose owner
// matches an admin plane user by id and name, then removes the import request annotation.
// Workloads of other owners are listed in the log for the import tool.
func (r *ClusterReconciler) importAdoptedWorkloads(ctx context.Context, cluster *v1.Cluster) error {
	factory, err := utils.GetK8sClientFactory(r.clientManager, cluster.Name)
	if err != nil || factory == nil || !factory.IsValid() {
		return fmt.Errorf("adopted cluster %s: %w", cluster.Name, errAdoptedClientNotReady)
	}
	var dynamicClient dynamic.Interface
	if dc := factory.DynamicClient(); dc != nil {
		dynamicClient = dc
	}
	locals, err := adopt.ListUnfinishedWorkloads(ctx, dynamicClient)
	if err != nil {
		return err
	}
	plans, err := adopt.PlanImports(ctx, r.Client, locals, nil)
	if err != nil {
		return err
	}
	var errs []error
	imported, unmatched := 0, make([]string, 0)
	for _, plan := range plans {
		if plan.Action == adopt.ActionUnmapped {
			unmatched = append(unmatched, fmt.Sprintf("%s(%s)", plan.Workload.Name, v1.GetUserName(plan.Workload)))
			continue
		}
		if plan.Action != adopt.ActionMatched {
			continue
		}
		created, err := adopt.ImportWorkload(ctx, r.Client, cluster.Name, plan.Workload, plan.User)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if created {
			imported++
		}
	}
	klog.Infof("adopted cluster %s: imported %d workloads, %d without a matching user: %v",
		cluster.Name, imported, len(unmatched), unmatched)
	if err = errors.Join(errs...); err != nil {
		return err
	}
	patch := client.MergeFrom(cluster.DeepCopy())
	delete(cluster.Annotations, v1.ClusterAdoptWorkloadsAnnotation)
	return r.Patch(ctx, cluster, patch)
}
