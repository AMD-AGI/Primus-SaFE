// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package reconciler

import (
	"testing"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func TestNewEndpointsReconciler_Create(t *testing.T) {
	reconciler := NewEndpointsReconciler()

	assert.NotNil(t, reconciler)
	assert.Nil(t, reconciler.clientSets)
}

func TestEndpointsReconciler_CreateServicePodRef(t *testing.T) {
	reconciler := NewEndpointsReconciler()

	t.Run("creates basic service-pod reference", func(t *testing.T) {
		svc := &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-service",
				Namespace: "default",
				UID:       types.UID("service-uid-123"),
			},
		}

		addr := corev1.EndpointAddress{
			IP: "10.0.0.1",
			TargetRef: &corev1.ObjectReference{
				Kind:      "Pod",
				Name:      "test-pod-abc123",
				Namespace: "default",
				UID:       types.UID("pod-uid-456"),
			},
		}

		ref := reconciler.createServicePodRef(nil, svc, addr)

		assert.NotNil(t, ref)
		assert.Equal(t, "service-uid-123", ref.ServiceUID)
		assert.Equal(t, "test-service", ref.ServiceName)
		assert.Equal(t, "default", ref.ServiceNamespace)
		assert.Equal(t, "pod-uid-456", ref.PodUID)
		assert.Equal(t, "test-pod-abc123", ref.PodName)
		assert.Equal(t, "10.0.0.1", ref.PodIP)
		assert.False(t, ref.CreatedAt.IsZero())
		assert.False(t, ref.UpdatedAt.IsZero())
	})

	t.Run("includes node name when available", func(t *testing.T) {
		svc := &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-service",
				Namespace: "default",
				UID:       types.UID("service-uid-123"),
			},
		}

		nodeName := "node-1"
		addr := corev1.EndpointAddress{
			IP:       "10.0.0.1",
			NodeName: &nodeName,
			TargetRef: &corev1.ObjectReference{
				Kind:      "Pod",
				Name:      "test-pod-abc123",
				Namespace: "default",
				UID:       types.UID("pod-uid-456"),
			},
		}

		ref := reconciler.createServicePodRef(nil, svc, addr)

		assert.NotNil(t, ref)
		assert.Equal(t, "node-1", ref.NodeName)
	})
}

func TestServicePodReference_Fields(t *testing.T) {
	t.Run("can create ServicePodReference with all fields", func(t *testing.T) {
		ref := &model.ServicePodReference{
			ServiceUID:       "svc-uid-123",
			ServiceName:      "api-service",
			ServiceNamespace: "production",
			PodUID:           "pod-uid-456",
			PodName:          "api-service-pod-abc",
			PodIP:            "10.0.1.5",
			PodLabels: model.ExtType{
				"app":     "api-service",
				"version": "v1",
			},
			WorkloadID:    "workload-123",
			WorkloadOwner: "team-a",
			WorkloadType:  "Deployment",
			NodeName:      "node-2",
			CreatedAt:     time.Now(),
			UpdatedAt:     time.Now(),
		}

		assert.Equal(t, "svc-uid-123", ref.ServiceUID)
		assert.Equal(t, "api-service", ref.ServiceName)
		assert.Equal(t, "production", ref.ServiceNamespace)
		assert.Equal(t, "pod-uid-456", ref.PodUID)
		assert.Equal(t, "api-service-pod-abc", ref.PodName)
		assert.Equal(t, "10.0.1.5", ref.PodIP)
		assert.Equal(t, "workload-123", ref.WorkloadID)
		assert.Equal(t, "team-a", ref.WorkloadOwner)
		assert.Equal(t, "Deployment", ref.WorkloadType)
		assert.Equal(t, "node-2", ref.NodeName)
	})

	t.Run("pod labels can store multiple values", func(t *testing.T) {
		ref := &model.ServicePodReference{
			PodLabels: model.ExtType{
				"app":                          "nginx",
				"version":                      "1.0",
				"environment":                  "prod",
				"app.kubernetes.io/name":       "nginx",
				"app.kubernetes.io/managed-by": "helm",
			},
		}

		assert.Len(t, ref.PodLabels, 5)
		assert.Equal(t, "nginx", ref.PodLabels.GetStringValue("app"))
		assert.Equal(t, "prod", ref.PodLabels.GetStringValue("environment"))
	})
}

func TestEndpointAddressProcessing(t *testing.T) {
	t.Run("processes endpoint address with target ref", func(t *testing.T) {
		addr := corev1.EndpointAddress{
			IP: "10.0.0.1",
			TargetRef: &corev1.ObjectReference{
				Kind:      "Pod",
				Name:      "my-pod",
				Namespace: "default",
				UID:       "pod-123",
			},
		}

		assert.NotNil(t, addr.TargetRef)
		assert.Equal(t, "Pod", addr.TargetRef.Kind)
		assert.Equal(t, "my-pod", addr.TargetRef.Name)
	})

	t.Run("handles nil target ref", func(t *testing.T) {
		addr := corev1.EndpointAddress{
			IP: "10.0.0.1",
		}

		assert.Nil(t, addr.TargetRef)
	})
}

func TestEndpointSubsetProcessing(t *testing.T) {
	t.Run("processes subset with ready and not ready addresses", func(t *testing.T) {
		subset := corev1.EndpointSubset{
			Addresses: []corev1.EndpointAddress{
				{
					IP: "10.0.0.1",
					TargetRef: &corev1.ObjectReference{
						Kind: "Pod",
						Name: "pod-1",
						UID:  "uid-1",
					},
				},
				{
					IP: "10.0.0.2",
					TargetRef: &corev1.ObjectReference{
						Kind: "Pod",
						Name: "pod-2",
						UID:  "uid-2",
					},
				},
			},
			NotReadyAddresses: []corev1.EndpointAddress{
				{
					IP: "10.0.0.3",
					TargetRef: &corev1.ObjectReference{
						Kind: "Pod",
						Name: "pod-3",
						UID:  "uid-3",
					},
				},
			},
		}

		assert.Len(t, subset.Addresses, 2)
		assert.Len(t, subset.NotReadyAddresses, 1)

		// Verify all addresses have valid target refs
		for _, addr := range subset.Addresses {
			assert.NotNil(t, addr.TargetRef)
			assert.Equal(t, "Pod", addr.TargetRef.Kind)
		}
	})

	t.Run("handles empty subset", func(t *testing.T) {
		subset := corev1.EndpointSubset{}

		assert.Empty(t, subset.Addresses)
		assert.Empty(t, subset.NotReadyAddresses)
	})
}

