/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package scheduler

import (
	"context"
	"fmt"

	"k8s.io/klog/v2"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	commonworkload "github.com/AMD-AIG-AIMA/SAFE/common/pkg/workload"
)

const (
	// ExternalWaitingScaleUpReason is shown while a single Pod waits for B to provision.
	ExternalWaitingScaleUpReason = "In queue - waiting for scale-up"
	// ExternalWaitingPRReason is shown while a gang waits for Provisioned=True.
	ExternalWaitingPRReason = "In queue - waiting for capacity service"
)

// admitExternalViaScheduler admits an external workload on the kube-scheduler path.
// Single-replica workloads are admitted immediately so the dispatcher can create the Pod;
// Unschedulable pods then trigger B scale-up. Gang workloads require a ProvisioningRequest
// before the PyTorchJob is created (see ensureExternalProvisioning).
func (r *SchedulerReconciler) admitExternalViaScheduler(ctx context.Context,
	workload *v1.Workload, workspace *v1.Workspace) (bool, string, error) {
	if err := validateExternalShapeForScheduler(workload); err != nil {
		return false, ExternalUnsupportedReason, err
	}
	state, err := r.ensureExternalSchedulerState(ctx, workload)
	if err != nil {
		return false, "", err
	}
	if commonworkload.IsExternalRDMAGang(workload) {
		return r.ensureExternalProvisioning(ctx, workload, workspace, state)
	}
	klog.V(2).InfoS("admitted external workload via kube-scheduler path",
		"workload", workload.Name, "workspace", workspace.Name,
		"generation", state.DispatchGeneration)
	return true, "", nil
}

// ensureExternalSchedulerState persists PlacementMode=kube-scheduler and a dispatch
// generation before any Pod or ProvisioningRequest is created.
func (r *SchedulerReconciler) ensureExternalSchedulerState(ctx context.Context,
	workload *v1.Workload) (*v1.WorkloadExternalExecution, error) {
	generation := int32(v1.GetWorkloadDispatchCnt(workload) + 1)
	current := workload.Status.ExternalExecution
	if current != nil &&
		current.PlacementMode == v1.ExternalPlacementKubeScheduler &&
		current.DispatchGeneration == generation {
		return current.DeepCopy(), nil
	}
	state := &v1.WorkloadExternalExecution{
		PlacementMode:      v1.ExternalPlacementKubeScheduler,
		DispatchGeneration: generation,
	}
	if current != nil && current.PlacementMode == v1.ExternalPlacementKubeScheduler {
		state.ProvisioningRequest = current.ProvisioningRequest
	}
	if err := r.patchExternalState(ctx, workload, state); err != nil {
		return nil, err
	}
	return state, nil
}

// ensureExternalProvisioning creates or observes the gang ProvisioningRequest.
// Full PodTemplate+PR create/watch lands in a follow-up; until then gangs stay queued
// with a visible reason so the switch can still admit single-pod workloads.
func (r *SchedulerReconciler) ensureExternalProvisioning(ctx context.Context,
	workload *v1.Workload, workspace *v1.Workspace,
	state *v1.WorkloadExternalExecution) (bool, string, error) {
	name := externalProvisioningRequestName(workload, state.DispatchGeneration)
	if state.ProvisioningRequest != name {
		updated := state.DeepCopy()
		updated.ProvisioningRequest = name
		if err := r.patchExternalState(ctx, workload, updated); err != nil {
			return false, "", err
		}
	}
	// TODO(R7): create PodTemplate + ProvisioningRequest and wait for Provisioned=True.
	klog.V(2).InfoS("external gang waiting for ProvisioningRequest path",
		"workload", workload.Name, "workspace", workspace.Name, "pr", name)
	return false, ExternalWaitingPRReason, nil
}

func externalProvisioningRequestName(workload *v1.Workload, generation int32) string {
	uid := string(workload.UID)
	if len(uid) > 8 {
		uid = uid[:8]
	}
	return fmt.Sprintf("pr-%s-%d", uid, generation)
}

// validateExternalShapeForScheduler keeps the same shape gate the claim path used.
func validateExternalShapeForScheduler(workload *v1.Workload) error {
	if workload == nil {
		return fmt.Errorf("nil workload")
	}
	if commonworkload.IsExternalRDMAGang(workload) || commonworkload.GetTotalReplica(workload) == 1 {
		return nil
	}
	return fmt.Errorf("external kube-scheduler path supports one replica or an RDMA gang")
}
