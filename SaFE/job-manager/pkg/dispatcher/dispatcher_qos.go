/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 */

package dispatcher

import (
	"context"
	"encoding/json"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	commonqos "github.com/AMD-AIG-AIMA/SAFE/common/pkg/qos"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/quantity"
	commonworkload "github.com/AMD-AIG-AIMA/SAFE/common/pkg/workload"
)

// prepareScaleQuota decides whether a stored scale may be synced.
// It returns true when the running-size annotation should be cleared after a successful sync.
// A scale that does not fit is written back to the running size and is not cleared again.
// Caller syncs whatever spec remains.
func (r *DispatcherReconciler) prepareScaleQuota(ctx context.Context, workload *v1.Workload) (bool, error) {
	raw := v1.GetAnnotation(workload, v1.QosRunningResourcesAnnotation)
	if raw == "" || workload == nil {
		return false, nil
	}
	running, err := decodeRunningResources(raw)
	if err != nil {
		return false, err
	}
	runningTotal, err := commonworkload.GetTotalResourceList(&v1.Workload{Spec: v1.WorkloadSpec{Resources: running}})
	if err != nil {
		return false, err
	}
	requested, err := commonworkload.GetTotalResourceList(workload)
	if err != nil {
		return false, err
	}
	cpu, memory, gpu := commonqos.ResourceGrew(runningTotal, requested)
	if !cpu && !memory && !gpu {
		return true, nil
	}
	userID := v1.GetUserId(workload)
	workspaceID := commonqos.WorkspaceID(workload)
	if userID == "" || workspaceID == "" {
		return true, nil
	}
	r.scaleMu.Lock()
	defer r.scaleMu.Unlock()
	limits, err := commonqos.Ceiling(ctx, r.workloadReader(), userID, workspaceID, string(commonworkload.GetScope(workload)))
	if err != nil {
		return false, err
	}
	if limits.CPU == "" && limits.Memory == "" && limits.GPU == "" {
		r.rememberScaleHeld(workload)
		return true, nil
	}
	used, err := r.scaleUsage(ctx, workload)
	if err != nil {
		return false, err
	}
	reason, err := commonqos.Exceeded(userID, workspaceID, used, requested, limits, cpu, memory, gpu)
	if err != nil {
		return false, err
	}
	if reason != "" {
		delete(r.scaleHeld, scaleHoldKey(userID, workspaceID, workload.Name))
		return false, r.restoreRunningResources(ctx, workload, running)
	}
	r.rememberScaleHeld(workload)
	return true, nil
}

// clearRunningResources drops the running-size annotation after the new size has been synced.
func (r *DispatcherReconciler) clearRunningResources(ctx context.Context, workload *v1.Workload) error {
	if workload == nil || v1.GetAnnotation(workload, v1.QosRunningResourcesAnnotation) == "" {
		r.dropScaleHeld(workload)
		return nil
	}
	base := workload.DeepCopy()
	v1.RemoveAnnotation(workload, v1.QosRunningResourcesAnnotation)
	if err := r.Patch(ctx, workload, client.MergeFrom(base)); err != nil {
		return err
	}
	r.dropScaleHeld(workload)
	return nil
}

// scaleUsage sums the sizes this scale must fit beside. Caller holds scaleMu.
// A scale already accepted in this process counts at its new size. A queued workload
// counts when it is admitted ahead of this one. A scale still waiting counts at its
// new size when it is ahead, and at its running size when it is behind.
func (r *DispatcherReconciler) scaleUsage(ctx context.Context, workload *v1.Workload) (corev1.ResourceList, error) {
	used := corev1.ResourceList{}
	listed := &v1.WorkloadList{}
	if err := r.workloadReader().List(ctx, listed, client.MatchingLabels{
		v1.WorkspaceIdLabel: commonqos.WorkspaceID(workload),
		v1.UserIdLabel:      v1.GetUserId(workload),
	}); err != nil {
		return nil, err
	}
	for i := range listed.Items {
		item := &listed.Items[i]
		if item.Name == workload.Name || item.IsEnd() {
			continue
		}
		resources, count, err := r.resourcesCountedForScale(item, workload)
		if err != nil || !count {
			if err != nil {
				return nil, err
			}
			continue
		}
		part, err := commonworkload.GetTotalResourceList(&v1.Workload{Spec: v1.WorkloadSpec{Resources: resources}})
		if err != nil {
			return nil, err
		}
		used = quantity.AddResource(used, part)
	}
	return used, nil
}

// resourcesCountedForScale returns the size of item that counts against workload.
// Caller holds scaleMu.
func (r *DispatcherReconciler) resourcesCountedForScale(item, workload *v1.Workload) ([]v1.WorkloadResource, bool, error) {
	key := scaleHoldKey(v1.GetUserId(item), commonqos.WorkspaceID(item), item.Name)
	if held, ok := r.scaleHeld[key]; ok {
		return held, true, nil
	}
	if !v1.IsWorkloadScheduled(item) {
		if !commonqos.AdmissionBefore(item, workload) {
			return nil, false, nil
		}
		return item.Spec.Resources, true, nil
	}
	raw := v1.GetAnnotation(item, v1.QosRunningResourcesAnnotation)
	if raw != "" && !commonqos.AdmissionBefore(item, workload) {
		running, err := decodeRunningResources(raw)
		return running, true, err
	}
	return item.Spec.Resources, true, nil
}

// restoreRunningResources writes the running resource list back onto the workload.
func (r *DispatcherReconciler) restoreRunningResources(ctx context.Context, workload *v1.Workload, running []v1.WorkloadResource) error {
	base := workload.DeepCopy()
	workload.Spec.Resources = running
	v1.RemoveAnnotation(workload, v1.QosRunningResourcesAnnotation)
	return r.Patch(ctx, workload, client.MergeFrom(base))
}

// rememberScaleHeld records a scale this process will sync. Caller holds scaleMu.
func (r *DispatcherReconciler) rememberScaleHeld(workload *v1.Workload) {
	if workload == nil {
		return
	}
	if r.scaleHeld == nil {
		r.scaleHeld = map[string][]v1.WorkloadResource{}
	}
	copied := workload.DeepCopy()
	r.scaleHeld[scaleHoldKey(v1.GetUserId(workload), commonqos.WorkspaceID(workload), workload.Name)] = copied.Spec.Resources
}

// dropScaleHeld forgets a scale decision.
func (r *DispatcherReconciler) dropScaleHeld(workload *v1.Workload) {
	if workload == nil {
		return
	}
	r.scaleMu.Lock()
	defer r.scaleMu.Unlock()
	delete(r.scaleHeld, scaleHoldKey(v1.GetUserId(workload), commonqos.WorkspaceID(workload), workload.Name))
}

func (r *DispatcherReconciler) workloadReader() client.Reader {
	if r.apiReader != nil {
		return r.apiReader
	}
	return r.Client
}

func scaleHoldKey(userID, workspaceID, name string) string {
	return userID + "|" + workspaceID + "|" + name
}

func decodeRunningResources(raw string) ([]v1.WorkloadResource, error) {
	var running []v1.WorkloadResource
	if err := json.Unmarshal([]byte(raw), &running); err != nil {
		return nil, err
	}
	return running, nil
}
