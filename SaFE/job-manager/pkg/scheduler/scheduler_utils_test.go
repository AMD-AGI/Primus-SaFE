/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package scheduler

import (
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
)

// TestFormatResourceName tests formatting of resource names
func TestFormatResourceName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "NVIDIA GPU",
			input:    common.NvidiaGpu,
			expected: "gpu",
		},
		{
			name:     "AMD GPU",
			input:    common.AmdGpu,
			expected: "gpu",
		},
		{
			name:     "CPU",
			input:    "cpu",
			expected: "cpu",
		},
		{
			name:     "Memory",
			input:    "memory",
			expected: "memory",
		},
		{
			name:     "Custom resource",
			input:    "custom-resource",
			expected: "custom-resource",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatResourceName(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestIsMatchNodeLabel tests node label matching logic
func TestIsMatchNodeLabel(t *testing.T) {
	tests := []struct {
		name     string
		node     *v1.Node
		workload *v1.Workload
		expected bool
	}{
		{
			name: "matching labels",
			node: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node-1",
					Labels: map[string]string{
						"zone":     "us-west-1a",
						"gpu-type": "a100",
					},
				},
			},
			workload: &v1.Workload{
				Spec: v1.WorkloadSpec{
					CustomerLabels: map[string]string{
						"zone":     "us-west-1a",
						"gpu-type": "a100",
					},
				},
			},
			expected: true,
		},
		{
			name: "non-matching label value",
			node: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"zone": "us-west-1a",
					},
				},
			},
			workload: &v1.Workload{
				Spec: v1.WorkloadSpec{
					CustomerLabels: map[string]string{
						"zone": "us-east-1a",
					},
				},
			},
			expected: false,
		},
		{
			name: "missing label on node",
			node: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"zone": "us-west-1a",
					},
				},
			},
			workload: &v1.Workload{
				Spec: v1.WorkloadSpec{
					CustomerLabels: map[string]string{
						"zone":     "us-west-1a",
						"gpu-type": "a100",
					},
				},
			},
			expected: false,
		},
		{
			name: "workload with no labels",
			node: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"zone": "us-west-1a",
					},
				},
			},
			workload: &v1.Workload{
				Spec: v1.WorkloadSpec{
					CustomerLabels: map[string]string{},
				},
			},
			expected: true, // No labels to match means it matches
		},
		{
			name: "node with extra labels",
			node: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"zone":     "us-west-1a",
						"gpu-type": "a100",
						"extra":    "label",
					},
				},
			},
			workload: &v1.Workload{
				Spec: v1.WorkloadSpec{
					CustomerLabels: map[string]string{
						"zone": "us-west-1a",
					},
				},
			},
			expected: true, // Extra labels on node are okay
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isMatchNodeLabel(tt.node, tt.workload)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestIsReScheduledForFailover tests failover rescheduling detection
func TestIsReScheduledForFailover(t *testing.T) {
	tests := []struct {
		name     string
		workload *v1.Workload
		expected bool
	}{
		{
			name: "rescheduled for failover",
			workload: &v1.Workload{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						v1.WorkloadReScheduledAnnotation: "true",
					},
				},
			},
			expected: true,
		},
		{
			name: "not rescheduled",
			workload: &v1.Workload{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{},
				},
			},
			expected: false,
		},
		{
			name: "preempted (not failover)",
			workload: &v1.Workload{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						v1.WorkloadReScheduledAnnotation: "true",
						v1.WorkloadPreemptedAnnotation:   "true",
					},
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isReScheduledForFailover(tt.workload)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestIsHaveDependencies tests dependency checking
func TestIsHaveDependencies(t *testing.T) {
	tests := []struct {
		name     string
		workload *v1.Workload
		expected bool
	}{
		{
			name: "no dependencies",
			workload: &v1.Workload{
				Spec: v1.WorkloadSpec{
					Dependencies: []string{},
				},
			},
			expected: false,
		},
		{
			name: "dependency not met",
			workload: &v1.Workload{
				Spec: v1.WorkloadSpec{
					Dependencies: []string{"dep-1"},
				},
				Status: v1.WorkloadStatus{
					DependenciesPhase: map[string]v1.WorkloadPhase{
						"dep-1": v1.WorkloadRunning,
					},
				},
			},
			expected: true,
		},
		{
			name: "all dependencies succeeded",
			workload: &v1.Workload{
				Spec: v1.WorkloadSpec{
					Dependencies: []string{"dep-1", "dep-2"},
				},
				Status: v1.WorkloadStatus{
					DependenciesPhase: map[string]v1.WorkloadPhase{
						"dep-1": v1.WorkloadSucceeded,
						"dep-2": v1.WorkloadSucceeded,
					},
				},
			},
			expected: false,
		},
		{
			name: "missing dependency status",
			workload: &v1.Workload{
				Spec: v1.WorkloadSpec{
					Dependencies: []string{"dep-1"},
				},
				Status: v1.WorkloadStatus{
					DependenciesPhase: map[string]v1.WorkloadPhase{},
				},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isHaveDependencies(tt.workload)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestWorkloadListSort tests WorkloadList sorting logic
func TestWorkloadListSort(t *testing.T) {
	baseTime := time.Now()

	tests := []struct {
		name          string
		workloads     WorkloadList
		expectedOrder []string // expected order of workload names after sorting
	}{
		{
			name: "sort by priority (higher priority first)",
			workloads: WorkloadList{
				{ObjectMeta: metav1.ObjectMeta{Name: "low-priority"}, Spec: v1.WorkloadSpec{Priority: 1}},
				{ObjectMeta: metav1.ObjectMeta{Name: "high-priority"}, Spec: v1.WorkloadSpec{Priority: 10}},
				{ObjectMeta: metav1.ObjectMeta{Name: "mid-priority"}, Spec: v1.WorkloadSpec{Priority: 5}},
			},
			expectedOrder: []string{"high-priority", "mid-priority", "low-priority"},
		},
		{
			name: "sort by creation time when priority is equal (earlier first)",
			workloads: WorkloadList{
				{ObjectMeta: metav1.ObjectMeta{Name: "newest", CreationTimestamp: metav1.NewTime(baseTime.Add(2 * time.Hour))}, Spec: v1.WorkloadSpec{Priority: 5}},
				{ObjectMeta: metav1.ObjectMeta{Name: "oldest", CreationTimestamp: metav1.NewTime(baseTime)}, Spec: v1.WorkloadSpec{Priority: 5}},
				{ObjectMeta: metav1.ObjectMeta{Name: "middle", CreationTimestamp: metav1.NewTime(baseTime.Add(1 * time.Hour))}, Spec: v1.WorkloadSpec{Priority: 5}},
			},
			expectedOrder: []string{"oldest", "middle", "newest"},
		},
		{
			name: "sort by name when priority and time are equal",
			workloads: WorkloadList{
				{ObjectMeta: metav1.ObjectMeta{Name: "charlie", CreationTimestamp: metav1.NewTime(baseTime)}, Spec: v1.WorkloadSpec{Priority: 5}},
				{ObjectMeta: metav1.ObjectMeta{Name: "alpha", CreationTimestamp: metav1.NewTime(baseTime)}, Spec: v1.WorkloadSpec{Priority: 5}},
				{ObjectMeta: metav1.ObjectMeta{Name: "bravo", CreationTimestamp: metav1.NewTime(baseTime)}, Spec: v1.WorkloadSpec{Priority: 5}},
			},
			expectedOrder: []string{"alpha", "bravo", "charlie"},
		},
		{
			name: "rescheduled for failover comes first",
			workloads: WorkloadList{
				{ObjectMeta: metav1.ObjectMeta{Name: "normal"}, Spec: v1.WorkloadSpec{Priority: 10}},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:        "rescheduled",
						Annotations: map[string]string{v1.WorkloadReScheduledAnnotation: "true"},
					},
					Spec: v1.WorkloadSpec{Priority: 1},
				},
			},
			expectedOrder: []string{"rescheduled", "normal"},
		},
		{
			name: "preempted workload is not treated as failover rescheduled",
			workloads: WorkloadList{
				{ObjectMeta: metav1.ObjectMeta{Name: "normal"}, Spec: v1.WorkloadSpec{Priority: 10}},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "preempted-rescheduled",
						Annotations: map[string]string{
							v1.WorkloadReScheduledAnnotation: "true",
							v1.WorkloadPreemptedAnnotation:   "true",
						},
					},
					Spec: v1.WorkloadSpec{Priority: 1},
				},
			},
			expectedOrder: []string{"normal", "preempted-rescheduled"},
		},
		{
			name: "workload with unmet dependencies comes last (same priority, different name)",
			workloads: WorkloadList{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "z-has-unmet-dep", CreationTimestamp: metav1.NewTime(baseTime)},
					Spec:       v1.WorkloadSpec{Priority: 5, Dependencies: []string{"dep-1"}},
					Status:     v1.WorkloadStatus{DependenciesPhase: map[string]v1.WorkloadPhase{"dep-1": v1.WorkloadRunning}},
				},
				{ObjectMeta: metav1.ObjectMeta{Name: "a-no-dep", CreationTimestamp: metav1.NewTime(baseTime)}, Spec: v1.WorkloadSpec{Priority: 5}},
			},
			// a-no-dep comes first: same priority/time, but "a" < "z" and z-has-unmet-dep has deps (cannot be before a-no-dep)
			expectedOrder: []string{"a-no-dep", "z-has-unmet-dep"},
		},
		{
			name: "workload with all dependencies succeeded is not penalized",
			workloads: WorkloadList{
				{ObjectMeta: metav1.ObjectMeta{Name: "low-priority"}, Spec: v1.WorkloadSpec{Priority: 1}},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "high-priority-with-dep"},
					Spec:       v1.WorkloadSpec{Priority: 10, Dependencies: []string{"dep-1"}},
					Status:     v1.WorkloadStatus{DependenciesPhase: map[string]v1.WorkloadPhase{"dep-1": v1.WorkloadSucceeded}},
				},
			},
			expectedOrder: []string{"high-priority-with-dep", "low-priority"},
		},
		{
			name: "comprehensive test: failover first, then by priority and time",
			workloads: WorkloadList{
				{ObjectMeta: metav1.ObjectMeta{Name: "w1-normal-low", CreationTimestamp: metav1.NewTime(baseTime)}, Spec: v1.WorkloadSpec{Priority: 1}},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "w2-failover", CreationTimestamp: metav1.NewTime(baseTime), Annotations: map[string]string{v1.WorkloadReScheduledAnnotation: "true"}},
					Spec:       v1.WorkloadSpec{Priority: 1},
				},
				{ObjectMeta: metav1.ObjectMeta{Name: "w3-normal-high", CreationTimestamp: metav1.NewTime(baseTime)}, Spec: v1.WorkloadSpec{Priority: 10}},
				{ObjectMeta: metav1.ObjectMeta{Name: "w4-normal-high-later", CreationTimestamp: metav1.NewTime(baseTime.Add(time.Hour))}, Spec: v1.WorkloadSpec{Priority: 10}},
			},
			// w2-failover comes first (failover priority)
			// w3-normal-high (priority=10, time=base)
			// w4-normal-high-later (priority=10, time=base+1h)
			// w1-normal-low (priority=1)
			expectedOrder: []string{"w2-failover", "w3-normal-high", "w4-normal-high-later", "w1-normal-low"},
		},
		{
			name: "workload with deps cannot jump ahead of no-dep workload",
			workloads: WorkloadList{
				{ObjectMeta: metav1.ObjectMeta{Name: "a-no-dep", CreationTimestamp: metav1.NewTime(baseTime)}, Spec: v1.WorkloadSpec{Priority: 5}},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "b-has-dep", CreationTimestamp: metav1.NewTime(baseTime)},
					Spec:       v1.WorkloadSpec{Priority: 5, Dependencies: []string{"dep-1"}},
					Status:     v1.WorkloadStatus{DependenciesPhase: map[string]v1.WorkloadPhase{"dep-1": v1.WorkloadRunning}},
				},
			},
			// b-has-dep has unmet dependencies, so it cannot come before a-no-dep
			expectedOrder: []string{"a-no-dep", "b-has-dep"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sort.Sort(tt.workloads)
			var actualOrder []string
			for _, w := range tt.workloads {
				actualOrder = append(actualOrder, w.Name)
			}
			assert.Equal(t, tt.expectedOrder, actualOrder)
		})
	}
}
