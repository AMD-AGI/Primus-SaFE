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

// importAdoptedWorkloads imports the scheduled, unfinished Workloads of the data plane SaFE whose
// owner matches an admin plane user by id and name, then removes the import request annotation.
// Workloads that are skipped are listed in the log for the import tool.
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
	plans, err := adopt.PlanImports(ctx, r.Client, locals, nil, false)
	if err != nil {
		return err
	}
	if err = r.importPlannedWorkloads(ctx, cluster.Name, plans); err != nil {
		return err
	}
	patch := client.MergeFrom(cluster.DeepCopy())
	delete(cluster.Annotations, v1.ClusterAdoptWorkloadsAnnotation)
	return r.Patch(ctx, cluster, patch)
}

// importPlannedWorkloads imports the matched workloads of the plans and logs the skipped ones.
// Workloads that cannot be imported are skipped; only transient failures return an error.
func (r *ClusterReconciler) importPlannedWorkloads(ctx context.Context, clusterName string, plans []adopt.Plan) error {
	var errs []error
	imported := 0
	skipped := map[string][]string{}
	for _, plan := range plans {
		name := plan.Workload.Name
		switch plan.Action {
		case adopt.ActionUnmapped:
			skipped[plan.Action] = append(skipped[plan.Action], fmt.Sprintf("%s(%s)", name, v1.GetUserName(plan.Workload)))
			continue
		case adopt.ActionUnscheduled:
			skipped[plan.Action] = append(skipped[plan.Action], name)
			continue
		case adopt.ActionMatched:
		default:
			continue
		}
		created, err := adopt.ImportWorkload(ctx, r.Client, clusterName, plan.Workload, plan.User)
		switch {
		case errors.Is(err, adopt.ErrNotImportable):
			klog.Warningf("adopted cluster %s: skip workload %s: %v", clusterName, name, err)
			skipped["failed"] = append(skipped["failed"], name)
		case err != nil:
			errs = append(errs, err)
		case created:
			imported++
		}
	}
	klog.Infof("adopted cluster %s: imported %d workloads, skipped %v", clusterName, imported, skipped)
	return errors.Join(errs...)
}
