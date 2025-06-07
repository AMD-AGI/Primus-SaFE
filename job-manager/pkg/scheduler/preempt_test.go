/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package scheduler

import (
	"context"
	"sort"
	"testing"
	"time"

	"gotest.tools/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apis/pkg/client/clientset/versioned/scheme"
	"github.com/AMD-AIG-AIMA/SAFE/job-manager/pkg/utils"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/slice"
)

func TestPreemptLowPriority(t *testing.T) {
	nf := utils.TestNodeFlavorData.DeepCopy()
	nf.Spec.Memory = *resource.NewQuantity(1, resource.BinarySI)
	workspace := utils.TestWorkspaceData.DeepCopy()
	workspace.Spec.NodeFlavor = nf.Name
	cli := fake.NewClientBuilder().WithObjects(nf, workspace).WithScheme(scheme.Scheme).Build()

	requestWorkload := &v1.Workload{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "test-workload",
			Labels:      map[string]string{v1.NodeFlavorIdLabel: nf.Name},
			Annotations: map[string]string{v1.WorkloadEnablePreemptAnnotation: ""},
		},
		Spec: v1.WorkloadSpec{
			Resource: v1.WorkloadResource{
				CPU: "10", Memory: "1", Replica: 1,
			},
			Priority:  2,
			Workspace: workspace.Name,
		},
	}
	nowTime := time.Now()

	tests := []struct {
		name             string
		currentWorkloads []*v1.Workload
		leftResource     corev1.ResourceList
		result           bool
		ids              []int
	}{
		{
			name: "success",
			currentWorkloads: []*v1.Workload{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "w1", CreationTimestamp: metav1.NewTime(nowTime)},
					Spec:       v1.WorkloadSpec{Resource: v1.WorkloadResource{CPU: "7", Memory: "1", Replica: 1}, Priority: 1, Workspace: workspace.Name},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "w2", CreationTimestamp: metav1.NewTime(nowTime)},
					Spec:       v1.WorkloadSpec{Resource: v1.WorkloadResource{CPU: "4", Memory: "1", Replica: 1}, Priority: 1, Workspace: workspace.Name},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "w3", CreationTimestamp: metav1.NewTime(nowTime)},
					Spec:       v1.WorkloadSpec{Resource: v1.WorkloadResource{CPU: "6", Memory: "1", Replica: 1}, Priority: 1, Workspace: workspace.Name},
				},
			},
			result: true,
			ids:    []int{0, 2},
		},
		{
			name: "success2",
			currentWorkloads: []*v1.Workload{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "w1", CreationTimestamp: metav1.NewTime(nowTime)},
					Spec:       v1.WorkloadSpec{Resource: v1.WorkloadResource{CPU: "1", Memory: "1", Replica: 1}, Priority: 1, Workspace: workspace.Name},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "w2", CreationTimestamp: metav1.NewTime(nowTime)},
					Spec:       v1.WorkloadSpec{Resource: v1.WorkloadResource{CPU: "4", Memory: "1", Replica: 1}, Priority: 1, Workspace: workspace.Name},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "w3", CreationTimestamp: metav1.NewTime(nowTime)},
					Spec:       v1.WorkloadSpec{Resource: v1.WorkloadResource{CPU: "5", Memory: "1", Replica: 1}, Priority: 1, Workspace: workspace.Name},
				},
			},
			result: true,
			ids:    []int{0, 1, 2},
		},
		{
			name: "success3",
			currentWorkloads: []*v1.Workload{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "w1", CreationTimestamp: metav1.NewTime(nowTime)},
					Spec:       v1.WorkloadSpec{Resource: v1.WorkloadResource{CPU: "1", Memory: "1", Replica: 1}, Priority: 1, Workspace: workspace.Name},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "w2", CreationTimestamp: metav1.NewTime(nowTime)},
					Spec:       v1.WorkloadSpec{Resource: v1.WorkloadResource{CPU: "4", Memory: "1", Replica: 1}, Priority: 1, Workspace: workspace.Name},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "w3", CreationTimestamp: metav1.NewTime(nowTime)},
					Spec:       v1.WorkloadSpec{Resource: v1.WorkloadResource{CPU: "5", Memory: "1", Replica: 1}, Priority: 1, Workspace: workspace.Name},
				},
			},
			leftResource: map[corev1.ResourceName]resource.Quantity{
				corev1.ResourceCPU: *resource.NewQuantity(5, resource.DecimalSI),
			},
			result: true,
			ids:    []int{2},
		},
		{
			name: "insufficient resource due to priority",
			currentWorkloads: []*v1.Workload{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "w1", CreationTimestamp: metav1.NewTime(nowTime)},
					Spec:       v1.WorkloadSpec{Resource: v1.WorkloadResource{CPU: "7", Memory: "1", Replica: 1}, Priority: 1, Workspace: workspace.Name},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "w2", CreationTimestamp: metav1.NewTime(nowTime)},
					Spec:       v1.WorkloadSpec{Resource: v1.WorkloadResource{CPU: "4", Memory: "1", Replica: 1}, Priority: 3, Workspace: workspace.Name},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "w3", CreationTimestamp: metav1.NewTime(nowTime)},
					Spec:       v1.WorkloadSpec{Resource: v1.WorkloadResource{CPU: "6", Memory: "1", Replica: 1}, Priority: 3, Workspace: workspace.Name},
				},
			},
			result: false,
		},
		{
			name: "insufficient resource",
			currentWorkloads: []*v1.Workload{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "w1", CreationTimestamp: metav1.NewTime(nowTime)},
					Spec:       v1.WorkloadSpec{Resource: v1.WorkloadResource{CPU: "1", Memory: "1", Replica: 1}, Priority: 1, Workspace: workspace.Name},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "w2", CreationTimestamp: metav1.NewTime(nowTime)},
					Spec:       v1.WorkloadSpec{Resource: v1.WorkloadResource{CPU: "1", Memory: "1", Replica: 1}, Priority: 1, Workspace: workspace.Name},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "w3", CreationTimestamp: metav1.NewTime(nowTime)},
					Spec:       v1.WorkloadSpec{Resource: v1.WorkloadResource{CPU: "1", Memory: "1", Replica: 1}, Priority: 1, Workspace: workspace.Name},
				},
			},
			result: false,
		},
	}
	r := SchedulerReconciler{
		Client: cli,
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			workloads, err := r.preemptLowPriorityWorkloads(context.Background(),
				requestWorkload, test.leftResource, test.currentWorkloads)
			assert.NilError(t, err)
			assert.Equal(t, len(workloads) > 0, test.result)
			if len(workloads) > 0 {
				var names []string
				for _, id := range test.ids {
					names = append(names, test.currentWorkloads[id].Name)
				}
				for _, w := range workloads {
					assert.Equal(t, slice.ContainsString(names, w.Name), true)
				}
			}
		})
	}
}

func TestSortWorkloadWrapper(t *testing.T) {
	workload1 := utils.TestWorkloadData.DeepCopy()
	workload1.Name = "w1"
	w1 := &WorkloadWrapper{
		workload:      workload1,
		resourceScore: 1.0,
	}
	workload2 := utils.TestWorkloadData.DeepCopy()
	workload2.Name = "w2"
	w2 := &WorkloadWrapper{
		workload:      workload2,
		resourceScore: 2.0,
	}
	workloads := []*WorkloadWrapper{w1, w2}
	sort.Sort(WorkloadWrapperSlice(workloads))
	assert.Equal(t, workloads[0].workload.Name, "w2")
	assert.Equal(t, workloads[1].workload.Name, "w1")

	workloads[0].resourceScore = workloads[1].resourceScore
	workloads[0].workload.Spec.Priority = 3
	sort.Sort(WorkloadWrapperSlice(workloads))
	assert.Equal(t, workloads[0].workload.Name, "w1")
	assert.Equal(t, workloads[1].workload.Name, "w2")

	workloads[0].resourceScore = workloads[1].resourceScore
	workloads[0].workload.Spec.Priority = workloads[1].workload.Spec.Priority
	workloads[0].workload.CreationTimestamp = metav1.NewTime(time.Now().Add(-time.Hour))
	sort.Sort(WorkloadWrapperSlice(workloads))
	assert.Equal(t, workloads[0].workload.Name, "w2")
	assert.Equal(t, workloads[1].workload.Name, "w1")
}
