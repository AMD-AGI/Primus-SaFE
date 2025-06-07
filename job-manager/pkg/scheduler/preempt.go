/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package scheduler

import (
	"context"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/quantity"
	commonworkload "github.com/AMD-AIG-AIMA/SAFE/common/pkg/workload"
)

type WorkloadWrapper struct {
	workload  *v1.Workload
	resources corev1.ResourceList
	// score: Gpu * 10 + Cpu/MaxCpu + Mem/MaxMem
	resourceScore float64
}

// Perform preemption when the requested resources exceed the workspace left quota.
// target: lower-priority tasks within the same workspace.
// Returns true if preemption is successful, false otherwise.
func (r *SchedulerReconciler) preempt(ctx context.Context, requestWorkload *v1.Workload,
	runningWorkloads []*v1.Workload, leftAvailResources corev1.ResourceList) (bool, error) {
	targetWorkloads, err := r.preemptLowPriorityWorkloads(ctx, requestWorkload, leftAvailResources, runningWorkloads)
	if err != nil || len(targetWorkloads) == 0 {
		return false, err
	}
	for _, w := range targetWorkloads {
		patch := client.MergeFrom(w.DeepCopy())
		v1.SetAnnotation(w, v1.WorkloadPreemptedAnnotation, time.Now().UTC().String())
		if err = r.Patch(ctx, w, patch); err != nil {
			klog.ErrorS(err, "failed to patch workload")
			return false, err
		}
		klog.Infof("the workload(%s) is preempted due to workload(%s)", w.Name, requestWorkload.Name)
	}
	return true, nil
}

// Preemption may occur when the requested resources exceed the remaining quota of the workspace.
// The preemption policy is: preemption must be enabled in the workspace,
// and the sum of resources from all lower-priority tasks plus the remaining available resource can meet the high-priority task.
func (r *SchedulerReconciler) preemptLowPriorityWorkloads(ctx context.Context, requestWorkload *v1.Workload,
	leftResources corev1.ResourceList, runningWorkloads []*v1.Workload) ([]*v1.Workload, error) {
	if !v1.IsWorkloadEnablePreempt(requestWorkload) {
		return nil, nil
	}
	sortedRunningWorkloads, err := r.sortRunningWorkloads(ctx, requestWorkload, runningWorkloads)
	if err != nil {
		klog.Error(err.Error())
		return nil, err
	}
	if len(sortedRunningWorkloads) == 0 {
		return nil, nil
	}

	var totalResources corev1.ResourceList
	if len(leftResources) == 0 {
		totalResources = make(corev1.ResourceList)
	} else {
		totalResources = leftResources.DeepCopy()
	}
	requestResources, _ := commonworkload.CvtToResourceList(requestWorkload)
	result := make([]*v1.Workload, 0, len(sortedRunningWorkloads))
	for i := range sortedRunningWorkloads {
		w := sortedRunningWorkloads[i].workload
		if w.Spec.Priority >= requestWorkload.Spec.Priority {
			break
		}
		if v1.IsWorkloadPreempted(w) {
			continue
		}
		totalResources = quantity.AddResource(totalResources, sortedRunningWorkloads[i].resources)
		if ok, _ := quantity.IsSubResource(requestResources, totalResources); ok {
			for j := 0; j <= i; j++ {
				result = append(result, sortedRunningWorkloads[j].workload)
			}
			return result, nil
		}
	}
	return nil, nil
}

func (r *SchedulerReconciler) sortRunningWorkloads(ctx context.Context,
	reqWorkload *v1.Workload, runningWorkloads []*v1.Workload) (WorkloadWrapperSlice, error) {
	if len(runningWorkloads) == 0 {
		return nil, nil
	}
	nf := &v1.NodeFlavor{}
	err := r.Get(ctx, client.ObjectKey{Name: v1.GetNodeFlavorId(reqWorkload)}, nf)
	if err != nil {
		return nil, err
	}
	var result []*WorkloadWrapper
	for i, w := range runningWorkloads {
		resources, _ := commonworkload.CvtToResourceList(w)
		result = append(result, &WorkloadWrapper{
			workload:      runningWorkloads[i],
			resources:     resources,
			resourceScore: buildResourceWeight(reqWorkload, resources, nf),
		})
	}
	sort.Sort(WorkloadWrapperSlice(result))
	return result, nil
}

type WorkloadWrapperSlice []*WorkloadWrapper

func (ws WorkloadWrapperSlice) Len() int {
	return len(ws)
}

func (ws WorkloadWrapperSlice) Swap(i, j int) {
	ws[i], ws[j] = ws[j], ws[i]
}

func (ws WorkloadWrapperSlice) Less(i, j int) bool {
	if ws[i].workload.Spec.Priority < ws[j].workload.Spec.Priority {
		return true
	} else if ws[i].workload.Spec.Priority > ws[j].workload.Spec.Priority {
		return false
	}
	if ws[i].resourceScore > ws[j].resourceScore {
		return true
	} else if ws[i].resourceScore < ws[j].resourceScore {
		return false
	}
	if ws[i].workload.CreationTimestamp.Time.After(ws[j].workload.CreationTimestamp.Time) {
		return true
	} else if ws[i].workload.CreationTimestamp.Time.Before(ws[j].workload.CreationTimestamp.Time) {
		return false
	}
	return ws[i].workload.Name < ws[j].workload.Name
}
