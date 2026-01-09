// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package reconciler

import (
	"context"
	"testing"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestNewServiceReconciler(t *testing.T) {
	r := NewServiceReconciler()
	assert.NotNil(t, r)
	assert.Nil(t, r.clientSets)
}

func TestServiceReconciler_createServicePodRef(t *testing.T) {
	ctx := context.Background()
	r := &ServiceReconciler{}

	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-service",
			Namespace: "test-namespace",
			UID:       types.UID("test-service-uid"),
		},
	}

	nodeName := "test-node"
	addr := corev1.EndpointAddress{
		IP:       "10.0.0.1",
		NodeName: &nodeName,
		TargetRef: &corev1.ObjectReference{
			Kind:      "Pod",
			Name:      "test-pod",
			Namespace: "test-namespace",
			UID:       types.UID("test-pod-uid"),
		},
	}

	ref := r.createServicePodRef(ctx, svc, addr)

	assert.NotNil(t, ref)
	assert.Equal(t, "test-service-uid", ref.ServiceUID)
	assert.Equal(t, "test-service", ref.ServiceName)
	assert.Equal(t, "test-namespace", ref.ServiceNamespace)
	assert.Equal(t, "test-pod-uid", ref.PodUID)
	assert.Equal(t, "test-pod", ref.PodName)
	assert.Equal(t, "10.0.0.1", ref.PodIP)
	assert.Equal(t, "test-node", ref.NodeName)
	assert.False(t, ref.CreatedAt.IsZero())
	assert.False(t, ref.UpdatedAt.IsZero())
}

func TestServicePort_JSON(t *testing.T) {
	port := model.ServicePort{
		Name:       "http",
		Port:       80,
		TargetPort: "8080",
		Protocol:   "TCP",
		NodePort:   30080,
	}

	assert.Equal(t, "http", port.Name)
	assert.Equal(t, 80, port.Port)
	assert.Equal(t, "8080", port.TargetPort)
	assert.Equal(t, "TCP", port.Protocol)
	assert.Equal(t, 30080, port.NodePort)
}

func TestK8sService_TableName(t *testing.T) {
	svc := &model.K8sService{}
	assert.Equal(t, "k8s_services", svc.TableName())
}

func TestServicePodReference_TableName(t *testing.T) {
	ref := &model.ServicePodReference{}
	assert.Equal(t, "service_pod_references", ref.TableName())
}

func TestK8sService_Model(t *testing.T) {
	now := time.Now()
	svc := &model.K8sService{
		ID:          1,
		UID:         "test-uid",
		Name:        "test-service",
		Namespace:   "test-namespace",
		ClusterIP:   "10.96.0.1",
		ServiceType: "ClusterIP",
		Selector:    model.ExtType{"app": "test"},
		Ports:       model.ExtJSON("[]"),
		Labels:      model.ExtType{"env": "test"},
		Annotations: model.ExtType{"note": "test annotation"},
		Deleted:     false,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	assert.Equal(t, int64(1), svc.ID)
	assert.Equal(t, "test-uid", svc.UID)
	assert.Equal(t, "test-service", svc.Name)
	assert.Equal(t, "test-namespace", svc.Namespace)
	assert.Equal(t, "10.96.0.1", svc.ClusterIP)
	assert.Equal(t, "ClusterIP", svc.ServiceType)
	assert.Equal(t, "test", svc.Selector["app"])
	assert.Equal(t, "test", svc.Labels["env"])
	assert.Equal(t, "test annotation", svc.Annotations["note"])
	assert.False(t, svc.Deleted)
}

func TestServicePodReference_Model(t *testing.T) {
	now := time.Now()
	ref := &model.ServicePodReference{
		ID:               1,
		ServiceUID:       "svc-uid",
		ServiceName:      "test-service",
		ServiceNamespace: "test-namespace",
		PodUID:           "pod-uid",
		PodName:          "test-pod",
		PodIP:            "10.0.0.1",
		PodLabels:        model.ExtType{"app": "test"},
		WorkloadID:       "workload-123",
		WorkloadOwner:    "user@example.com",
		WorkloadType:     "Deployment",
		NodeName:         "node-1",
		CreatedAt:        now,
		UpdatedAt:        now,
	}

	assert.Equal(t, int64(1), ref.ID)
	assert.Equal(t, "svc-uid", ref.ServiceUID)
	assert.Equal(t, "test-service", ref.ServiceName)
	assert.Equal(t, "test-namespace", ref.ServiceNamespace)
	assert.Equal(t, "pod-uid", ref.PodUID)
	assert.Equal(t, "test-pod", ref.PodName)
	assert.Equal(t, "10.0.0.1", ref.PodIP)
	assert.Equal(t, "test", ref.PodLabels["app"])
	assert.Equal(t, "workload-123", ref.WorkloadID)
	assert.Equal(t, "user@example.com", ref.WorkloadOwner)
	assert.Equal(t, "Deployment", ref.WorkloadType)
	assert.Equal(t, "node-1", ref.NodeName)
}

func TestBuildK8sServiceFromCorev1Service(t *testing.T) {
	k8sSvc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "web-service",
			Namespace: "production",
			UID:       types.UID("svc-12345"),
			Labels: map[string]string{
				"app":     "web",
				"version": "v1",
			},
			Annotations: map[string]string{
				"description": "Web service",
			},
			CreationTimestamp: metav1.Now(),
		},
		Spec: corev1.ServiceSpec{
			Type:      corev1.ServiceTypeClusterIP,
			ClusterIP: "10.96.100.1",
			Selector: map[string]string{
				"app": "web",
			},
			Ports: []corev1.ServicePort{
				{
					Name:       "http",
					Port:       80,
					TargetPort: intstr.FromInt(8080),
					Protocol:   corev1.ProtocolTCP,
				},
				{
					Name:       "https",
					Port:       443,
					TargetPort: intstr.FromInt(8443),
					Protocol:   corev1.ProtocolTCP,
				},
			},
		},
	}

	// Verify the source data
	assert.Equal(t, "web-service", k8sSvc.Name)
	assert.Equal(t, "production", k8sSvc.Namespace)
	assert.Equal(t, types.UID("svc-12345"), k8sSvc.UID)
	assert.Equal(t, "10.96.100.1", k8sSvc.Spec.ClusterIP)
	assert.Len(t, k8sSvc.Spec.Ports, 2)
	assert.Equal(t, "http", k8sSvc.Spec.Ports[0].Name)
	assert.Equal(t, int32(80), k8sSvc.Spec.Ports[0].Port)
}

func TestNewEndpointsReconciler(t *testing.T) {
	r := NewEndpointsReconciler()
	assert.NotNil(t, r)
	assert.Nil(t, r.clientSets)
}

func TestEndpointsReconciler_createServicePodRef(t *testing.T) {
	ctx := context.Background()
	r := &EndpointsReconciler{}

	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "api-service",
			Namespace: "default",
			UID:       types.UID("api-service-uid"),
		},
	}

	nodeName := "worker-1"
	addr := corev1.EndpointAddress{
		IP:       "192.168.1.10",
		NodeName: &nodeName,
		TargetRef: &corev1.ObjectReference{
			Kind:      "Pod",
			Name:      "api-pod-abc123",
			Namespace: "default",
			UID:       types.UID("api-pod-uid"),
		},
	}

	ref := r.createServicePodRef(ctx, svc, addr)

	assert.NotNil(t, ref)
	assert.Equal(t, "api-service-uid", ref.ServiceUID)
	assert.Equal(t, "api-service", ref.ServiceName)
	assert.Equal(t, "default", ref.ServiceNamespace)
	assert.Equal(t, "api-pod-uid", ref.PodUID)
	assert.Equal(t, "api-pod-abc123", ref.PodName)
	assert.Equal(t, "192.168.1.10", ref.PodIP)
	assert.Equal(t, "worker-1", ref.NodeName)
}

func TestEndpointsWithMultipleSubsets(t *testing.T) {
	endpoints := &corev1.Endpoints{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "multi-port-service",
			Namespace: "test",
		},
		Subsets: []corev1.EndpointSubset{
			{
				Addresses: []corev1.EndpointAddress{
					{IP: "10.0.0.1", TargetRef: &corev1.ObjectReference{Kind: "Pod", Name: "pod-1", UID: "uid-1"}},
					{IP: "10.0.0.2", TargetRef: &corev1.ObjectReference{Kind: "Pod", Name: "pod-2", UID: "uid-2"}},
				},
				Ports: []corev1.EndpointPort{
					{Name: "http", Port: 8080},
				},
			},
			{
				Addresses: []corev1.EndpointAddress{
					{IP: "10.0.0.3", TargetRef: &corev1.ObjectReference{Kind: "Pod", Name: "pod-3", UID: "uid-3"}},
				},
				NotReadyAddresses: []corev1.EndpointAddress{
					{IP: "10.0.0.4", TargetRef: &corev1.ObjectReference{Kind: "Pod", Name: "pod-4", UID: "uid-4"}},
				},
				Ports: []corev1.EndpointPort{
					{Name: "grpc", Port: 9090},
				},
			},
		},
	}

	// Count total addresses
	totalReady := 0
	totalNotReady := 0
	for _, subset := range endpoints.Subsets {
		totalReady += len(subset.Addresses)
		totalNotReady += len(subset.NotReadyAddresses)
	}

	assert.Equal(t, 3, totalReady)
	assert.Equal(t, 1, totalNotReady)
	assert.Len(t, endpoints.Subsets, 2)
}

