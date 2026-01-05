/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package cluster

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apis/pkg/client/clientset/versioned/scheme"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
)

// TestGetEndpoint tests the GetEndpoint function
func TestGetEndpoint(t *testing.T) {
	tests := []struct {
		name      string
		cluster   *v1.Cluster
		service   *corev1.Service
		wantErr   bool
		wantValue string
		errMsg    string
	}{
		{
			name:    "nil cluster",
			cluster: nil,
			wantErr: true,
			errMsg:  "cluster is not ready",
		},
		{
			name: "cluster not ready",
			cluster: &v1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-cluster",
				},
				Status: v1.ClusterStatus{
					ControlPlaneStatus: v1.ControlPlaneStatus{
						Phase: v1.CreatingPhase,
					},
				},
			},
			wantErr: true,
			errMsg:  "cluster is not ready",
		},
		{
			name: "ready cluster with service",
			cluster: &v1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-cluster",
				},
				Status: v1.ClusterStatus{
					ControlPlaneStatus: v1.ControlPlaneStatus{
						Phase: v1.ReadyPhase,
					},
				},
			},
			service: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cluster",
					Namespace: common.PrimusSafeNamespace,
				},
				Spec: corev1.ServiceSpec{
					ClusterIP: "10.96.100.50",
					Ports: []corev1.ServicePort{
						{
							Name:       "https",
							Port:       6443,
							TargetPort: intstr.FromInt(6443),
							Protocol:   corev1.ProtocolTCP,
						},
					},
				},
			},
			wantErr:   false,
			wantValue: "10.96.100.50:6443",
		},
		{
			name: "ready cluster with service but no ports",
			cluster: &v1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-cluster",
				},
				Status: v1.ClusterStatus{
					ControlPlaneStatus: v1.ControlPlaneStatus{
						Phase: v1.ReadyPhase,
					},
				},
			},
			service: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cluster",
					Namespace: common.PrimusSafeNamespace,
				},
				Spec: corev1.ServiceSpec{
					ClusterIP: "10.96.100.50",
					Ports:     []corev1.ServicePort{},
				},
			},
			wantErr: true,
			errMsg:  "service ports are empty",
		},
		{
			name: "ready cluster without service, with endpoint in status",
			cluster: &v1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-cluster",
				},
				Status: v1.ClusterStatus{
					ControlPlaneStatus: v1.ControlPlaneStatus{
						Phase: v1.ReadyPhase,
						Endpoints: []string{
							"https://192.168.1.100:6443",
						},
					},
				},
			},
			wantErr:   false,
			wantValue: "https://192.168.1.100:6443",
		},
		{
			name: "ready cluster without service and without endpoint",
			cluster: &v1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-cluster",
				},
				Status: v1.ClusterStatus{
					ControlPlaneStatus: v1.ControlPlaneStatus{
						Phase:     v1.ReadyPhase,
						Endpoints: []string{},
					},
				},
			},
			wantErr: true,
			errMsg:  "either the Service address or the Endpoint is empty",
		},
		{
			name: "ready cluster with service and multiple ports",
			cluster: &v1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "multi-port-cluster",
				},
				Status: v1.ClusterStatus{
					ControlPlaneStatus: v1.ControlPlaneStatus{
						Phase: v1.ReadyPhase,
					},
				},
			},
			service: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "multi-port-cluster",
					Namespace: common.PrimusSafeNamespace,
				},
				Spec: corev1.ServiceSpec{
					ClusterIP: "10.96.200.100",
					Ports: []corev1.ServicePort{
						{
							Name:       "https",
							Port:       6443,
							TargetPort: intstr.FromInt(6443),
							Protocol:   corev1.ProtocolTCP,
						},
						{
							Name:       "http",
							Port:       8080,
							TargetPort: intstr.FromInt(8080),
							Protocol:   corev1.ProtocolTCP,
						},
					},
				},
			},
			wantErr:   false,
			wantValue: "10.96.200.100:6443", // Should use first port
		},
		{
			name: "ready cluster with multiple endpoints",
			cluster: &v1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "multi-endpoint-cluster",
				},
				Status: v1.ClusterStatus{
					ControlPlaneStatus: v1.ControlPlaneStatus{
						Phase: v1.ReadyPhase,
						Endpoints: []string{
							"https://192.168.1.100:6443",
							"https://192.168.1.101:6443",
							"https://192.168.1.102:6443",
						},
					},
				},
			},
			wantErr:   false,
			wantValue: "https://192.168.1.100:6443", // Should use first endpoint
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock scheme
			mockScheme := scheme.Scheme
			_ = corev1.AddToScheme(mockScheme)
			_ = v1.AddToScheme(mockScheme)

			// Build fake client with or without service
			clientBuilder := fake.NewClientBuilder().WithScheme(mockScheme)
			if tt.service != nil {
				clientBuilder = clientBuilder.WithObjects(tt.service)
			}
			mockClient := clientBuilder.Build()

			// Execute test
			result, err := GetEndpoint(context.Background(), mockClient, tt.cluster)

			// Validate results
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.wantValue, result)
			}
		})
	}
}

// TestGetEndpointWithDifferentPhases tests GetEndpoint with various cluster phases
func TestGetEndpointWithDifferentPhases(t *testing.T) {
	phases := []struct {
		name  string
		phase v1.ClusterPhase
	}{
		{"Pending", v1.PendingPhase},
		{"Creating", v1.CreatingPhase},
		{"Deleting", v1.DeletingPhase},
		{"Deleted", v1.DeletedPhase},
	}

	for _, tt := range phases {
		t.Run(tt.name, func(t *testing.T) {
			cluster := &v1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-cluster",
				},
				Status: v1.ClusterStatus{
					ControlPlaneStatus: v1.ControlPlaneStatus{
						Phase: tt.phase,
					},
				},
			}

			mockScheme := scheme.Scheme
			_ = v1.AddToScheme(mockScheme)
			mockClient := fake.NewClientBuilder().WithScheme(mockScheme).Build()

			_, err := GetEndpoint(context.Background(), mockClient, cluster)
			assert.Error(t, err, "Should fail for phase: %s", tt.phase)
			assert.Contains(t, err.Error(), "cluster is not ready")
		})
	}
}

// TestGetEndpointServicePriority tests that service endpoint has priority over status endpoint
func TestGetEndpointServicePriority(t *testing.T) {
	cluster := &v1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-cluster",
		},
		Status: v1.ClusterStatus{
			ControlPlaneStatus: v1.ControlPlaneStatus{
				Phase: v1.ReadyPhase,
				Endpoints: []string{
					"https://status-endpoint:6443", // This should NOT be used
				},
			},
		},
	}

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: common.PrimusSafeNamespace,
		},
		Spec: corev1.ServiceSpec{
			ClusterIP: "10.96.100.50",
			Ports: []corev1.ServicePort{
				{
					Name:       "https",
					Port:       6443,
					TargetPort: intstr.FromInt(6443),
					Protocol:   corev1.ProtocolTCP,
				},
			},
		},
	}

	mockScheme := scheme.Scheme
	_ = corev1.AddToScheme(mockScheme)
	_ = v1.AddToScheme(mockScheme)
	mockClient := fake.NewClientBuilder().WithScheme(mockScheme).WithObjects(service).Build()

	result, err := GetEndpoint(context.Background(), mockClient, cluster)
	assert.NoError(t, err)
	assert.Equal(t, "10.96.100.50:6443", result)
	assert.NotContains(t, result, "status-endpoint", "Should use service endpoint, not status endpoint")
}
