/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package syncer

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

// TestGetMainContainerRank tests extraction of RANK environment variable
func TestGetMainContainerRank(t *testing.T) {
	tests := []struct {
		name         string
		workload     *v1.Workload
		pod          *corev1.Pod
		expectedRank string
	}{
		{
			name: "pod with RANK env variable",
			workload: &v1.Workload{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						v1.MainContainerAnnotation: "main",
					},
				},
				Spec: v1.WorkloadSpec{
					Image: "pytorch:latest",
				},
			},
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "main",
							Env: []corev1.EnvVar{
								{Name: "RANK", Value: "0"},
								{Name: "WORLD_SIZE", Value: "4"},
							},
						},
					},
				},
			},
			expectedRank: "0",
		},
		{
			name: "pod with multiple containers",
			workload: &v1.Workload{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						v1.MainContainerAnnotation: "worker",
					},
				},
				Spec: v1.WorkloadSpec{
					Image: "pytorch:latest",
				},
			},
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "sidecar",
							Env: []corev1.EnvVar{
								{Name: "RANK", Value: "999"}, // Wrong container
							},
						},
						{
							Name: "worker",
							Env: []corev1.EnvVar{
								{Name: "RANK", Value: "2"},
							},
						},
					},
				},
			},
			expectedRank: "2",
		},
		{
			name: "pod without RANK env",
			workload: &v1.Workload{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						v1.MainContainerAnnotation: "main",
					},
				},
				Spec: v1.WorkloadSpec{
					Image: "pytorch:latest",
				},
			},
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "main",
							Env: []corev1.EnvVar{
								{Name: "OTHER_VAR", Value: "value"},
							},
						},
					},
				},
			},
			expectedRank: "",
		},
		{
			name: "empty pod",
			workload: &v1.Workload{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						v1.MainContainerAnnotation: "main",
					},
				},
				Spec: v1.WorkloadSpec{
					Image: "pytorch:latest",
				},
			},
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{},
				},
			},
			expectedRank: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rank := getMainContainerRank(tt.workload, tt.pod)
			assert.Equal(t, tt.expectedRank, rank)
		})
	}
}

// TestSortWorkloadPods tests sorting of workload pods by IP and ID
func TestSortWorkloadPods(t *testing.T) {
	tests := []struct {
		name          string
		inputPods     []v1.WorkloadPod
		expectedOrder []string // Pod IDs in expected order
	}{
		{
			name: "sort by different IPs",
			inputPods: []v1.WorkloadPod{
				{PodId: "pod-1", HostIp: "192.168.1.1"},
				{PodId: "pod-2", HostIp: "192.168.1.100"},
				{PodId: "pod-3", HostIp: "192.168.1.50"},
			},
			expectedOrder: []string{"pod-2", "pod-3", "pod-1"}, // Sorted by IP descending
		},
		{
			name: "sort by pod ID when same IP",
			inputPods: []v1.WorkloadPod{
				{PodId: "pod-c", HostIp: "192.168.1.1"},
				{PodId: "pod-a", HostIp: "192.168.1.1"},
				{PodId: "pod-b", HostIp: "192.168.1.1"},
			},
			expectedOrder: []string{"pod-a", "pod-b", "pod-c"}, // Sorted by pod ID ascending
		},
		{
			name: "mixed IPs and IDs",
			inputPods: []v1.WorkloadPod{
				{PodId: "pod-2", HostIp: "10.0.0.5"},
				{PodId: "pod-1", HostIp: "10.0.0.5"},
				{PodId: "pod-4", HostIp: "10.0.0.10"},
				{PodId: "pod-3", HostIp: "10.0.0.10"},
			},
			expectedOrder: []string{"pod-3", "pod-4", "pod-1", "pod-2"},
		},
		{
			name: "single pod",
			inputPods: []v1.WorkloadPod{
				{PodId: "pod-1", HostIp: "192.168.1.1"},
			},
			expectedOrder: []string{"pod-1"},
		},
		{
			name:          "empty pods",
			inputPods:     []v1.WorkloadPod{},
			expectedOrder: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			workload := &v1.Workload{
				Status: v1.WorkloadStatus{
					Pods: tt.inputPods,
				},
			}

			sortWorkloadPods(workload)

			assert.Equal(t, len(tt.expectedOrder), len(workload.Status.Pods))
			for i, expectedPodId := range tt.expectedOrder {
				assert.Equal(t, expectedPodId, workload.Status.Pods[i].PodId,
					"Pod at index %d should be %s", i, expectedPodId)
			}
		})
	}
}
