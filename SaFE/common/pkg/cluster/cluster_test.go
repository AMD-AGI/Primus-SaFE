/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package cluster

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"math/big"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apis/pkg/client/clientset/versioned/scheme"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/k8sclient"
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

func TestClientFactoryNeedsRefresh(t *testing.T) {
	ctx := context.Background()
	mockScheme := scheme.Scheme
	_ = corev1.AddToScheme(mockScheme)
	_ = v1.AddToScheme(mockScheme)

	readyCluster := func(endpoints ...string) *v1.Cluster {
		return &v1.Cluster{
			ObjectMeta: metav1.ObjectMeta{Name: "c1"},
			Status: v1.ClusterStatus{
				ControlPlaneStatus: v1.ControlPlaneStatus{
					Phase:     v1.ReadyPhase,
					Endpoints: endpoints,
				},
			},
		}
	}
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "c1", Namespace: common.PrimusSafeNamespace},
		Spec: corev1.ServiceSpec{
			ClusterIP: "10.96.1.1",
			Ports:     []corev1.ServicePort{{Port: 6443}},
		},
	}
	serviceClient := fake.NewClientBuilder().WithScheme(mockScheme).WithObjects(service).Build()
	directClient := fake.NewClientBuilder().WithScheme(mockScheme).Build()

	assert.True(t, ClientFactoryNeedsRefresh(ctx, serviceClient, readyCluster(), nil))

	invalid := commonclient.NewClientFactoryWithOnlyClient(ctx, "c1", nil)
	invalid.SetValid(false, "down")
	assert.True(t, ClientFactoryNeedsRefresh(ctx, serviceClient, readyCluster(), invalid))

	// Direct mode: factory on second endpoint should not refresh when status fingerprint matches.
	cluster := readyCluster("https://10.0.0.1:6443", "https://10.0.0.2:6443")
	factory := commonclient.NewClientFactoryForTest("c1", "https://10.0.0.2:6443")
	factory.SetBackendFingerprint(StatusEndpointsFingerprint(cluster))
	assert.False(t, ClientFactoryNeedsRefresh(ctx, directClient, cluster, factory))

	// Service mode: ClusterIP change triggers refresh.
	factorySvc := commonclient.NewClientFactoryForTest("c1", "10.96.1.1:6443")
	assert.False(t, ClientFactoryNeedsRefresh(ctx, serviceClient, readyCluster("https://10.0.0.1:6443"), factorySvc))
	otherService := service.DeepCopy()
	otherService.Spec.ClusterIP = "10.96.2.2"
	otherClient := fake.NewClientBuilder().WithScheme(mockScheme).WithObjects(otherService).Build()
	assert.True(t, ClientFactoryNeedsRefresh(ctx, otherClient, readyCluster("https://10.0.0.1:6443"), factorySvc))
}

func TestBackendIPsFingerprint(t *testing.T) {
	assert.Equal(t, "", BackendIPsFingerprint(nil))
	assert.Equal(t, "10.0.0.1,10.0.0.2", BackendIPsFingerprint([]string{"10.0.0.2", "10.0.0.1"}))
}

func TestGetControlPlaneBackendIPs(t *testing.T) {
	ctx := context.Background()
	mockScheme := scheme.Scheme
	_ = corev1.AddToScheme(mockScheme)
	_ = v1.AddToScheme(mockScheme)

	endpoints := &corev1.Endpoints{
		ObjectMeta: metav1.ObjectMeta{Name: "c1", Namespace: common.PrimusSafeNamespace},
		Subsets: []corev1.EndpointSubset{{
			Addresses: []corev1.EndpointAddress{{IP: "10.0.0.2"}, {IP: "10.0.0.1"}},
		}},
	}
	cl := fake.NewClientBuilder().WithScheme(mockScheme).WithObjects(endpoints).Build()
	ips, err := GetControlPlaneBackendIPs(ctx, cl, &v1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "c1"}})
	assert.NoError(t, err)
	assert.Equal(t, []string{"10.0.0.1", "10.0.0.2"}, ips)
}

func TestClientFactoryNeedsRefreshBackendIPsChanged(t *testing.T) {
	ctx := context.Background()
	mockScheme := scheme.Scheme
	_ = corev1.AddToScheme(mockScheme)
	_ = v1.AddToScheme(mockScheme)

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "c1", Namespace: common.PrimusSafeNamespace},
		Spec:       corev1.ServiceSpec{ClusterIP: "10.96.1.1", Ports: []corev1.ServicePort{{Port: 6443}}},
	}
	endpoints := &corev1.Endpoints{
		ObjectMeta: metav1.ObjectMeta{Name: "c1", Namespace: common.PrimusSafeNamespace},
		Subsets: []corev1.EndpointSubset{{
			Addresses: []corev1.EndpointAddress{{IP: "10.0.0.1"}},
		}},
	}
	cl := fake.NewClientBuilder().WithScheme(mockScheme).WithObjects(service, endpoints).Build()
	cluster := &v1.Cluster{
		ObjectMeta: metav1.ObjectMeta{Name: "c1"},
		Status:     v1.ClusterStatus{ControlPlaneStatus: v1.ControlPlaneStatus{Phase: v1.ReadyPhase}},
	}
	factory := commonclient.NewClientFactoryForTest("c1", "10.96.1.1:6443")
	factory.SetBackendFingerprint("10.0.0.1,10.0.0.2")
	assert.True(t, ClientFactoryNeedsRefresh(ctx, cl, cluster, factory))
}

func TestClientFactoryNeedsRefreshWhenBackendLookupFails(t *testing.T) {
	ctx := context.Background()
	mockScheme := scheme.Scheme
	_ = v1.AddToScheme(mockScheme)

	cluster := &v1.Cluster{
		ObjectMeta: metav1.ObjectMeta{Name: "c1"},
		Status:     v1.ClusterStatus{ControlPlaneStatus: v1.ControlPlaneStatus{Phase: v1.ReadyPhase}},
	}
	cl := fake.NewClientBuilder().WithScheme(mockScheme).Build()
	factory := commonclient.NewClientFactoryForTest("c1", "10.96.1.1:6443")
	factory.SetBackendFingerprint("10.0.0.1")
	assert.True(t, ClientFactoryNeedsRefresh(ctx, cl, cluster, factory))
}

func TestClientFactoryNeedsRefreshDirectModeUnreachable(t *testing.T) {
	ctx := context.Background()
	cluster := &v1.Cluster{
		ObjectMeta: metav1.ObjectMeta{Name: "c1"},
		Status: v1.ClusterStatus{
			ControlPlaneStatus: v1.ControlPlaneStatus{
				Phase:     v1.ReadyPhase,
				Endpoints: []string{"https://10.0.0.1:6443", "https://10.0.0.2:6443"},
			},
		},
	}
	factory := commonclient.NewClientFactoryForTest("c1", "https://10.0.0.1:6443")
	factory.SetBackendFingerprint(StatusEndpointsFingerprint(cluster))
	factory.AttachRestConfigForTest(&rest.Config{
		Host:    "https://127.0.0.1:1",
		Timeout: time.Millisecond * 100,
	})
	assert.True(t, ClientFactoryNeedsRefresh(ctx, fake.NewClientBuilder().WithScheme(scheme.Scheme).Build(), cluster, factory))
}

func TestDirectModeFactoryNeedsRefreshWithoutRestConfig(t *testing.T) {
	factory := commonclient.NewClientFactoryForTest("c1", "https://10.0.0.1:6443")
	assert.False(t, directModeFactoryNeedsRefresh(factory))
}

// clusterTestCert returns a base64-encoded self-signed cert/key pair so factory construction can
// run without reaching an apiserver.
func clusterTestCert(t *testing.T) (certData, keyData string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	assert.NoError(t, err)
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "safe-unit-test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		IsCA:         true,
	}
	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	assert.NoError(t, err)
	keyDER, err := x509.MarshalECPrivateKey(key)
	assert.NoError(t, err)

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	return base64.StdEncoding.EncodeToString(certPEM), base64.StdEncoding.EncodeToString(keyPEM)
}

func TestNewClientFactoryForClusterServiceMode(t *testing.T) {
	ctx := context.Background()
	mockScheme := scheme.Scheme
	_ = corev1.AddToScheme(mockScheme)
	_ = v1.AddToScheme(mockScheme)

	certData, keyData := clusterTestCert(t)
	cluster := &v1.Cluster{
		ObjectMeta: metav1.ObjectMeta{Name: "c1"},
		Status: v1.ClusterStatus{ControlPlaneStatus: v1.ControlPlaneStatus{
			Phase:    v1.ReadyPhase,
			CertData: certData,
			KeyData:  keyData,
		}},
	}
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "c1", Namespace: common.PrimusSafeNamespace},
		Spec:       corev1.ServiceSpec{ClusterIP: "10.96.1.1", Ports: []corev1.ServicePort{{Port: 6443}}},
	}
	endpoints := &corev1.Endpoints{
		ObjectMeta: metav1.ObjectMeta{Name: "c1", Namespace: common.PrimusSafeNamespace},
		Subsets: []corev1.EndpointSubset{{
			Addresses: []corev1.EndpointAddress{{IP: "10.0.0.2"}, {IP: "10.0.0.1"}},
		}},
	}
	cl := fake.NewClientBuilder().WithScheme(mockScheme).WithObjects(service, endpoints).Build()

	factory, err := NewClientFactoryForCluster(ctx, cl, cluster, commonclient.DisableInformer)
	assert.NoError(t, err)
	// Service mode dials the ClusterIP and records the current backend pool.
	assert.Equal(t, "https://10.96.1.1:6443", factory.Endpoint())
	assert.Equal(t, "10.0.0.1,10.0.0.2", factory.BackendFingerprint())
	assert.NoError(t, factory.Release())

	// A freshly built factory matches the cluster, so no refresh is required.
	assert.False(t, ClientFactoryNeedsRefresh(ctx, cl, cluster, factory))
}

func TestNewClientFactoryForClusterDirectModeUnreachable(t *testing.T) {
	ctx := context.Background()
	mockScheme := scheme.Scheme
	_ = corev1.AddToScheme(mockScheme)
	_ = v1.AddToScheme(mockScheme)

	certData, keyData := clusterTestCert(t)
	cluster := &v1.Cluster{
		ObjectMeta: metav1.ObjectMeta{Name: "c1"},
		Status: v1.ClusterStatus{ControlPlaneStatus: v1.ControlPlaneStatus{
			Phase:     v1.ReadyPhase,
			CertData:  certData,
			KeyData:   keyData,
			Endpoints: []string{"https://127.0.0.1:1", "https://127.0.0.1:2"},
		}},
	}
	cl := fake.NewClientBuilder().WithScheme(mockScheme).Build()

	// Direct mode probes every candidate, so an all-down control plane surfaces an error.
	_, err := NewClientFactoryForCluster(ctx, cl, cluster, commonclient.DisableInformer)
	assert.ErrorContains(t, err, "no reachable apiserver endpoint")
}

func TestNewClientFactoryForClusterNotReady(t *testing.T) {
	mockScheme := scheme.Scheme
	_ = v1.AddToScheme(mockScheme)
	cl := fake.NewClientBuilder().WithScheme(mockScheme).Build()
	_, err := NewClientFactoryForCluster(context.Background(), cl,
		&v1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "c1"}}, commonclient.DisableInformer)
	assert.Error(t, err)
}

func TestClientFactoryNeedsRefreshSkipsRebuildWhenServiceUnreachable(t *testing.T) {
	ctx := context.Background()
	mockScheme := scheme.Scheme
	_ = corev1.AddToScheme(mockScheme)
	_ = v1.AddToScheme(mockScheme)

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "c1", Namespace: common.PrimusSafeNamespace},
		Spec:       corev1.ServiceSpec{ClusterIP: "127.0.0.1", Ports: []corev1.ServicePort{{Port: 1}}},
	}
	cl := fake.NewClientBuilder().WithScheme(mockScheme).WithObjects(service).Build()
	cluster := &v1.Cluster{
		ObjectMeta: metav1.ObjectMeta{Name: "c1"},
		Status:     v1.ClusterStatus{ControlPlaneStatus: v1.ControlPlaneStatus{Phase: v1.ReadyPhase}},
	}
	factory := commonclient.NewClientFactoryForTest("c1", "127.0.0.1:1")
	factory.SetValid(false, "watch error")
	factory.AttachRestConfigForTest(&rest.Config{Host: "https://127.0.0.1:1", Timeout: time.Millisecond * 100})

	// Every backend is down: keep the current factory instead of rebuilding it every reconcile.
	assert.False(t, ClientFactoryNeedsRefresh(ctx, cl, cluster, factory))
	// The outdated check itself still reports the factory as stale.
	assert.True(t, clientFactoryOutdated(ctx, cl, cluster, factory))
}
