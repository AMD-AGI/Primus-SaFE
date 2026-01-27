/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package syncer

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	commonfaults "github.com/AMD-AIG-AIMA/SAFE/common/pkg/faults"
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
					Images: []string{"pytorch:latest"},
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
					Images: []string{"pytorch:latest"},
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
					Images: []string{"pytorch:latest"},
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
					Images: []string{"pytorch:latest"},
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

// setupTestScheme creates a scheme with required types for testing
func setupTestScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	_ = v1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	return scheme
}

// TestCreateStickyNodeFaults tests the createStickyNodeFaults function
func TestCreateStickyNodeFaults(t *testing.T) {
	ctx := context.Background()
	scheme := setupTestScheme()

	t.Run("sticky nodes not enabled - should skip", func(t *testing.T) {
		workload := &v1.Workload{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-workload",
			},
		}
		cli := fake.NewClientBuilder().WithScheme(scheme).Build()
		r := &SyncerReconciler{Client: cli}

		err := r.createStickyNodeFaults(ctx, workload, 1)
		assert.NoError(t, err)

		// Verify no fault was created
		faultList := &v1.FaultList{}
		err = cli.List(ctx, faultList)
		assert.NoError(t, err)
		assert.Empty(t, faultList.Items)
	})

	t.Run("count is zero - should skip", func(t *testing.T) {
		workload := &v1.Workload{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-workload",
				Annotations: map[string]string{
					v1.WorkloadStickyNodesAnnotation: "true",
				},
			},
		}
		cli := fake.NewClientBuilder().WithScheme(scheme).Build()
		r := &SyncerReconciler{Client: cli}

		err := r.createStickyNodeFaults(ctx, workload, 0)
		assert.NoError(t, err)

		// Verify no fault was created
		faultList := &v1.FaultList{}
		err = cli.List(ctx, faultList)
		assert.NoError(t, err)
		assert.Empty(t, faultList.Items)
	})

	t.Run("sticky nodes enabled with count=1 - should create faults", func(t *testing.T) {
		workload := &v1.Workload{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-workload",
				UID:  "test-uid",
				Annotations: map[string]string{
					v1.WorkloadStickyNodesAnnotation: "true",
				},
			},
			Spec: v1.WorkloadSpec{
				MaxRetry: 3,
			},
			Status: v1.WorkloadStatus{
				Nodes: [][]string{
					{"node-1", "node-2"},
				},
				Pods: []v1.WorkloadPod{
					{AdminNodeName: "node-1", K8sNodeName: "k8s-node-1"},
					{AdminNodeName: "node-2", K8sNodeName: "k8s-node-2"},
				},
			},
		}
		cli := fake.NewClientBuilder().WithScheme(scheme).Build()
		r := &SyncerReconciler{Client: cli}

		err := r.createStickyNodeFaults(ctx, workload, 1)
		assert.NoError(t, err)

		// Verify faults were created for both nodes
		faultList := &v1.FaultList{}
		err = cli.List(ctx, faultList)
		assert.NoError(t, err)
		assert.Len(t, faultList.Items, 2)

		// Verify fault IDs
		faultIds := make(map[string]bool)
		for _, f := range faultList.Items {
			faultIds[f.Name] = true
		}
		expectedFault1 := commonfaults.GenerateFaultId("node-1", v1.StickyNodesMonitorId)
		expectedFault2 := commonfaults.GenerateFaultId("node-2", v1.StickyNodesMonitorId)
		assert.True(t, faultIds[expectedFault1], "fault for node-1 should exist")
		assert.True(t, faultIds[expectedFault2], "fault for node-2 should exist")
	})

	t.Run("sticky nodes enabled with count=2 - should add new and delete old faults", func(t *testing.T) {
		workload := &v1.Workload{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-workload",
				UID:  "test-uid",
				Annotations: map[string]string{
					v1.WorkloadStickyNodesAnnotation: "true",
				},
			},
			Spec: v1.WorkloadSpec{
				MaxRetry: 3,
			},
			Status: v1.WorkloadStatus{
				Nodes: [][]string{
					{"node-1", "node-2"}, // previous nodes
					{"node-2", "node-3"}, // current nodes (node-1 removed, node-3 added)
				},
				Pods: []v1.WorkloadPod{
					{AdminNodeName: "node-2", K8sNodeName: "k8s-node-2"},
					{AdminNodeName: "node-3", K8sNodeName: "k8s-node-3"},
				},
			},
		}

		// Pre-create fault for node-1 (which should be deleted)
		existingFault := &v1.Fault{
			ObjectMeta: metav1.ObjectMeta{
				Name: commonfaults.GenerateFaultId("node-1", v1.StickyNodesMonitorId),
			},
			Spec: v1.FaultSpec{
				MonitorId: v1.StickyNodesMonitorId,
			},
		}
		cli := fake.NewClientBuilder().WithScheme(scheme).WithObjects(existingFault).Build()
		r := &SyncerReconciler{Client: cli}

		err := r.createStickyNodeFaults(ctx, workload, 2)
		assert.NoError(t, err)

		// Verify fault for node-3 was created
		expectedFault3 := commonfaults.GenerateFaultId("node-3", v1.StickyNodesMonitorId)
		fault3 := &v1.Fault{}
		err = cli.Get(ctx, client.ObjectKey{Name: expectedFault3}, fault3)
		assert.NoError(t, err, "fault for node-3 should be created")

		// Verify fault for node-1 was deleted
		expectedFault1 := commonfaults.GenerateFaultId("node-1", v1.StickyNodesMonitorId)
		fault1 := &v1.Fault{}
		err = cli.Get(ctx, client.ObjectKey{Name: expectedFault1}, fault1)
		assert.True(t, apierrors.IsNotFound(err), "fault for node-1 should be deleted")
	})
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
			expectedOrder: []string{"pod-1", "pod-3", "pod-2"}, // Sorted by IP descending
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
			expectedOrder: []string{"pod-1", "pod-2", "pod-3", "pod-4"},
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
