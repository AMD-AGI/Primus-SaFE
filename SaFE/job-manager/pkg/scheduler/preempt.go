/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package scheduler

import (
	"context"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/quantity"
	commonworkload "github.com/AMD-AIG-AIMA/SAFE/common/pkg/workload"
	jsonutils "github.com/AMD-AIG-AIMA/SAFE/utils/pkg/json"
)

type WorkloadWrapper struct {
	// The underlying workload object
	workload *v1.Workload
	// Workload resource
	resources corev1.ResourceList
	// Resource score for sort. score = Gpu * 10 + Cpu/MaxCpu + Mem/MaxMem
	resourceScore float64
}

// preempt: perform preemption when the requested resources exceed the workspace left quota.
// target: lower-priority tasks within the same workspace.
// Returns true if preemption is successful, false otherwise.
func (r *SchedulerReconciler) preempt(ctx context.Context, requestWorkload *v1.Workload,
	scheduledWorkloads []*v1.Workload, leftAvailResources corev1.ResourceList) (bool, error) {
	targetWorkloads, err := r.preemptLowPriorityWorkloads(ctx, requestWorkload, leftAvailResources, scheduledWorkloads)
	if err != nil {
		return false, err
	}
	if len(targetWorkloads) == 0 {
		klog.Infof("workload %s: Unable to obtain sufficient workloads to preempt resources.", requestWorkload.Name)
		return false, nil
	}
	for _, w := range targetWorkloads {
		patchObj := map[string]any{
			"metadata": map[string]any{
				"resourceVersion": w.ResourceVersion,
				"annotations": map[string]any{
					v1.WorkloadPreemptedAnnotation: time.Now().UTC().String(),
				},
			},
		}
		p := jsonutils.MarshalSilently(patchObj)
		if err = r.Patch(ctx, w, client.RawPatch(types.MergePatchType, p)); err != nil {
			klog.ErrorS(err, "failed to update workload")
			return false, err
		}
		klog.Infof("the workload(%s) is preempted due to workload(%s)", w.Name, requestWorkload.Name)
	}
	return true, nil
}

// preemptLowPriorityWorkloads: preemption may occur when the requested resources exceed the remaining quota of the workspace.
// The preemption policy is: preemption must be enabled in the workspace,
// and the sum of resources from all lower-priority tasks plus the remaining available resource can meet the high-priority task.
func (r *SchedulerReconciler) preemptLowPriorityWorkloads(ctx context.Context, requestWorkload *v1.Workload,
	leftResources corev1.ResourceList, scheduledWorkloads []*v1.Workload) ([]*v1.Workload, error) {
	if !v1.IsWorkloadEnablePreempt(requestWorkload) {
		return nil, nil
	}
	sortedWorkloads, err := r.sortWorkloads(ctx, requestWorkload, scheduledWorkloads)
	if err != nil {
		klog.Error(err.Error())
		return nil, err
	}
	if len(sortedWorkloads) == 0 {
		return nil, nil
	}

	totalResources := quantity.Copy(leftResources)
	requestResources, _ := commonworkload.CvtToResourceList(requestWorkload)
	result := make([]*v1.Workload, 0, len(sortedWorkloads))
	for i := range sortedWorkloads {
		w := sortedWorkloads[i].workload
		if w.Spec.Priority >= requestWorkload.Spec.Priority {
			break
		}
		if v1.IsWorkloadPreempted(w) {
			continue
		}
		totalResources = quantity.AddResource(totalResources, sortedWorkloads[i].resources)
		if ok, _ := quantity.IsSubResource(requestResources, totalResources); ok {
			for j := 0; j <= i; j++ {
				result = append(result, sortedWorkloads[j].workload)
			}
			return result, nil
		}
	}
	return nil, nil
}

// isPreemptable checks if a workload can preempt other scheduled workloads based on priority.
func (r *SchedulerReconciler) isPreemptable(requestWorkload *v1.Workload, scheduledWorkloads []*v1.Workload) bool {
	if !v1.IsWorkloadEnablePreempt(requestWorkload) {
		return false
	}
	for _, w := range scheduledWorkloads {
		if requestWorkload.Spec.Priority > w.Spec.Priority {
			return true
		}
	}
	return false
}

// sortWorkloads sorts running workloads for preemption consideration.
func (r *SchedulerReconciler) sortWorkloads(ctx context.Context,
	requestWorkload *v1.Workload, targetWorkloads []*v1.Workload) (WorkloadWrapperSlice, error) {
	if len(targetWorkloads) == 0 {
		return nil, nil
	}
	nf := &v1.NodeFlavor{}
	err := r.Get(ctx, client.ObjectKey{Name: v1.GetNodeFlavorId(requestWorkload)}, nf)
	if err != nil {
		return nil, err
	}
	var result []*WorkloadWrapper
	for i, w := range targetWorkloads {
		resources, _ := commonworkload.CvtToResourceList(w)
		result = append(result, &WorkloadWrapper{
			workload:      targetWorkloads[i],
			resources:     resources,
			resourceScore: buildResourceWeight(requestWorkload, resources, nf),
		})
	}
	sort.Sort(WorkloadWrapperSlice(result))
	return result, nil
}

type WorkloadWrapperSlice []*WorkloadWrapper

// Len implements sort.Interface by returning the length of the slice.
func (workloads WorkloadWrapperSlice) Len() int {
	return len(workloads)
}

// Swap implements sort.Interface by swapping elements at the given indices.
func (workloads WorkloadWrapperSlice) Swap(i, j int) {
	workloads[i], workloads[j] = workloads[j], workloads[i]
}

// Less implements sort.Interface for sorting.
func (workloads WorkloadWrapperSlice) Less(i, j int) bool {
	if workloads[i].workload.Spec.Priority < workloads[j].workload.Spec.Priority {
		return true
	} else if workloads[i].workload.Spec.Priority > workloads[j].workload.Spec.Priority {
		return false
	}
	if workloads[i].resourceScore > workloads[j].resourceScore {
		return true
	} else if workloads[i].resourceScore < workloads[j].resourceScore {
		return false
	}
	if workloads[i].workload.CreationTimestamp.Time.After(workloads[j].workload.CreationTimestamp.Time) {
		return true
	} else if workloads[i].workload.CreationTimestamp.Time.Before(workloads[j].workload.CreationTimestamp.Time) {
		return false
	}
	return workloads[i].workload.Name < workloads[j].workload.Name
}
