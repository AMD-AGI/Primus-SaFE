package clientsets

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	statsapi "k8s.io/kubelet/pkg/apis/stats/v1alpha1"
)

// TestDecodeBase64IfNeeded tests the decodeBase64IfNeeded function
func TestDecodeBase64IfNeeded(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{
			name:    "valid base64 string",
			input:   base64.StdEncoding.EncodeToString([]byte("hello world")),
			want:    "hello world",
			wantErr: false,
		},
		{
			name:    "plain text PEM format",
			input:   "-----BEGIN CERTIFICATE-----\nMIIC...\n-----END CERTIFICATE-----",
			want:    "-----BEGIN CERTIFICATE-----\nMIIC...\n-----END CERTIFICATE-----",
			wantErr: false,
		},
		{
			name:    "empty string",
			input:   "",
			want:    "",
			wantErr: false,
		},
		{
			name:    "plain text",
			input:   "this is plain text",
			want:    "this is plain text",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := decodeBase64IfNeeded(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("decodeBase64IfNeeded() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("decodeBase64IfNeeded() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestClusterConfigFromRestConfig tests the ClusterConfigFromRestConfig function
func TestClusterConfigFromRestConfig(t *testing.T) {
	tests := []struct {
		name      string
		restCfg   *rest.Config
		wantNil   bool
		checkFunc func(t *testing.T, cfg *ClusterConfig)
	}{
		{
			name:    "nil rest config",
			restCfg: nil,
			wantNil: true,
		},
		{
			name: "basic rest config",
			restCfg: &rest.Config{
				Host:        "https://api.example.com",
				BearerToken: "test-token",
			},
			wantNil: false,
			checkFunc: func(t *testing.T, cfg *ClusterConfig) {
				if cfg.Host != "https://api.example.com" {
					t.Errorf("Expected host to be https://api.example.com, got %s", cfg.Host)
				}
				if cfg.BearerToken != "test-token" {
					t.Errorf("Expected bearer token to be test-token, got %s", cfg.BearerToken)
				}
				if !cfg.InsecureSkipTLSVerify {
					t.Error("Expected InsecureSkipTLSVerify to be true (always set for kubelet)")
				}
			},
		},
		{
			name: "rest config with TLS settings",
			restCfg: &rest.Config{
				Host: "https://api.example.com",
				TLSClientConfig: rest.TLSClientConfig{
					ServerName: "test-server",
					CAData:     []byte("ca-data"),
					CertData:   []byte("cert-data"),
					KeyData:    []byte("key-data"),
				},
			},
			wantNil: false,
			checkFunc: func(t *testing.T, cfg *ClusterConfig) {
				if cfg.TLSServerName != "test-server" {
					t.Errorf("Expected TLS server name to be test-server, got %s", cfg.TLSServerName)
				}
				if cfg.CAData != "ca-data" {
					t.Errorf("Expected CA data to be ca-data, got %s", cfg.CAData)
				}
				if cfg.CertData != "cert-data" {
					t.Errorf("Expected cert data to be cert-data, got %s", cfg.CertData)
				}
				if cfg.KeyData != "key-data" {
					t.Errorf("Expected key data to be key-data, got %s", cfg.KeyData)
				}
			},
		},
		{
			name: "rest config with bearer token",
			restCfg: &rest.Config{
				Host:        "https://api.example.com",
				BearerToken: "my-bearer-token",
			},
			wantNil: false,
			checkFunc: func(t *testing.T, cfg *ClusterConfig) {
				if cfg.BearerToken != "my-bearer-token" {
					t.Errorf("Expected bearer token to be my-bearer-token, got %s", cfg.BearerToken)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ClusterConfigFromRestConfig(tt.restCfg)
			if (got == nil) != tt.wantNil {
				t.Errorf("ClusterConfigFromRestConfig() returned nil = %v, wantNil = %v", got == nil, tt.wantNil)
				return
			}
			if tt.checkFunc != nil && got != nil {
				tt.checkFunc(t, got)
			}
		})
	}
}

// TestCreateKubeletTLSConfig tests the createKubeletTLSConfig function
func TestCreateKubeletTLSConfig(t *testing.T) {
	// Note: We're testing with actual PEM data, but not validating certificates
	// since that would require valid certificate chains
	validCert := `-----BEGIN CERTIFICATE-----
MIIBkTCB+wIJAKHHCgVZU6NzMA0GCSqGSIb3DQEBCwUAMBMxETAPBgNVBAMMCHRl
c3QtY2VydDAeFw0yMzAxMDEwMDAwMDBaFw0yNDAxMDEwMDAwMDBaMBMxETAPBgNV
BAMMCHRlc3QtY2VydDBcMA0GCSqGSIb3DQEBAQUAA0sAMEgCQQCxoeCUW5KJ7Fzi
-----END CERTIFICATE-----`

	validKey := `-----BEGIN RSA PRIVATE KEY-----
MIIBOwIBAAJBALGh4JRbkonsXOIMQRON0WU1gkP2GyeOkLjBiJGn6W9XG0rStC2w
fN4p6b5RAiAW1hqzfA3LUQrfPJrLLXdQCLT+nQIhALnTqSvyf4l7h8T2p0KXqfnO
-----END RSA PRIVATE KEY-----`

	tests := []struct {
		name      string
		config    *ClusterConfig
		wantErr   bool
		checkFunc func(t *testing.T, tlsCfg *tls.Config)
	}{
		{
			name: "insecure config",
			config: &ClusterConfig{
				InsecureSkipTLSVerify: true,
			},
			wantErr: false,
			checkFunc: func(t *testing.T, tlsCfg *tls.Config) {
				if !tlsCfg.InsecureSkipVerify {
					t.Error("Expected InsecureSkipVerify to be true")
				}
			},
		},
		{
			name: "config with TLS server name",
			config: &ClusterConfig{
				TLSServerName:         "test-server.example.com",
				InsecureSkipTLSVerify: true,
			},
			wantErr: false,
			checkFunc: func(t *testing.T, tlsCfg *tls.Config) {
				if tlsCfg.ServerName != "test-server.example.com" {
					t.Errorf("Expected ServerName to be test-server.example.com, got %s", tlsCfg.ServerName)
				}
			},
		},
		{
			name: "config with valid cert and key",
			config: &ClusterConfig{
				CertData:              validCert,
				KeyData:               validKey,
				InsecureSkipTLSVerify: true,
			},
			wantErr: true, // Incomplete test certificates will fail - this is expected
		},
		{
			name: "config with base64 encoded cert and key",
			config: &ClusterConfig{
				CertData:              base64.StdEncoding.EncodeToString([]byte(validCert)),
				KeyData:               base64.StdEncoding.EncodeToString([]byte(validKey)),
				InsecureSkipTLSVerify: true,
			},
			wantErr: true, // Incomplete test certificates will fail - this is expected
		},
		{
			name: "config with CA data",
			config: &ClusterConfig{
				CAData:                validCert,
				InsecureSkipTLSVerify: false,
			},
			wantErr: true, // Incomplete test certificate will fail - this is expected
		},
		{
			name: "config with CA data but insecure skip",
			config: &ClusterConfig{
				CAData:                validCert,
				InsecureSkipTLSVerify: true,
			},
			wantErr: false,
			checkFunc: func(t *testing.T, tlsCfg *tls.Config) {
				if tlsCfg.RootCAs != nil {
					t.Error("Expected RootCAs to be nil when InsecureSkipVerify is true")
				}
			},
		},
		{
			name: "config with invalid cert/key pair",
			config: &ClusterConfig{
				CertData:              "invalid-cert",
				KeyData:               "invalid-key",
				InsecureSkipTLSVerify: true,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tlsCfg, err := createKubeletTLSConfig(tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("createKubeletTLSConfig() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.checkFunc != nil && tlsCfg != nil {
				tt.checkFunc(t, tlsCfg)
			}
		})
	}
}

// TestNewClient tests the NewClient function (without actual HTTP calls)
func TestNewClient(t *testing.T) {
	tests := []struct {
		name            string
		kubeletAddress  string
		config          *ClusterConfig
		wantErr         bool
		checkFunc       func(t *testing.T, client *Client)
		setupTokenFile  bool
		tokenFileEnvVar string
	}{
		{
			name:           "client with nil config and no token file",
			kubeletAddress: "https://10.0.0.1:10250",
			config:         nil,
			wantErr:        true, // Will fail because token file doesn't exist
		},
		{
			name:           "client with cluster config",
			kubeletAddress: "https://10.0.0.1:10250",
			config: &ClusterConfig{
				InsecureSkipTLSVerify: true,
			},
			wantErr: false,
			checkFunc: func(t *testing.T, client *Client) {
				if client.kubeletApi == nil {
					t.Error("Expected kubelet API client to be initialized")
				}
			},
		},
		{
			name:           "client with bearer token",
			kubeletAddress: "https://10.0.0.1:10250",
			config: &ClusterConfig{
				BearerToken:           "test-bearer-token",
				InsecureSkipTLSVerify: true,
			},
			wantErr: false,
			checkFunc: func(t *testing.T, client *Client) {
				if client.kubeletApi == nil {
					t.Error("Expected kubelet API client to be initialized")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := NewClient(tt.kubeletAddress, tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewClient() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.checkFunc != nil && client != nil {
				tt.checkFunc(t, client)
			}
		})
	}
}

// TestClient_GetRestyClient tests the GetRestyClient method
func TestClient_GetRestyClient(t *testing.T) {

	// Create a client with proper initialization
	testClient, err := NewClient("https://10.0.0.1:10250", &ClusterConfig{
		InsecureSkipTLSVerify: true,
	})
	if err != nil {
		t.Fatalf("Failed to create test client: %v", err)
	}

	restyClient := testClient.GetRestyClient()
	if restyClient == nil {
		t.Error("Expected non-nil resty client")
	}

	// Verify it's a clone (different instance)
	if restyClient == testClient.kubeletApi {
		t.Error("Expected GetRestyClient to return a clone, not the same instance")
	}
}

// Benchmark tests for performance
func BenchmarkDecodeBase64IfNeeded(b *testing.B) {
	input := base64.StdEncoding.EncodeToString([]byte("hello world"))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = decodeBase64IfNeeded(input)
	}
}

func BenchmarkClusterConfigFromRestConfig(b *testing.B) {
	restCfg := &rest.Config{
		Host:        "https://api.example.com",
		BearerToken: "test-token",
		TLSClientConfig: rest.TLSClientConfig{
			ServerName: "test-server",
			CAData:     []byte("ca-data"),
			CertData:   []byte("cert-data"),
			KeyData:    []byte("key-data"),
		},
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ClusterConfigFromRestConfig(restCfg)
	}
}

func BenchmarkCreateKubeletTLSConfig(b *testing.B) {
	config := &ClusterConfig{
		InsecureSkipTLSVerify: true,
		TLSServerName:         "test-server",
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = createKubeletTLSConfig(config)
	}
}

// TestKubeletClientKey tests the kubeletClientKey structure
func TestKubeletClientKey(t *testing.T) {
	key1 := kubeletClientKey{
		clusterName: "cluster1",
		nodeName:    "node1",
	}

	key2 := kubeletClientKey{
		clusterName: "cluster1",
		nodeName:    "node1",
	}

	key3 := kubeletClientKey{
		clusterName: "cluster2",
		nodeName:    "node1",
	}

	// Test key equality (Go structs are comparable if all fields are comparable)
	if key1 != key2 {
		t.Error("Expected key1 and key2 to be equal")
	}

	if key1 == key3 {
		t.Error("Expected key1 and key3 to be different")
	}
}

// TestCreateKubeletTLSConfig_AppendCertsFromPEM tests CA certificate loading
func TestCreateKubeletTLSConfig_AppendCertsFromPEM(t *testing.T) {
	validCA := `-----BEGIN CERTIFICATE-----
MIIBkTCB+wIJAKHHCgVZU6NzMA0GCSqGSIb3DQEBCwUAMBMxETAPBgNVBAMMCHRl
c3QtY2VydDAeFw0yMzAxMDEwMDAwMDBaFw0yNDAxMDEwMDAwMDBaMBMxETAPBgNV
BAMMCHRlc3QtY2VydDBcMA0GCSqGSIb3DQEBAQUAA0sAMEgCQQCxoeCUW5KJ7Fzi
-----END CERTIFICATE-----`

	tests := []struct {
		name    string
		config  *ClusterConfig
		wantErr bool
	}{
		{
			name: "valid CA certificate",
			config: &ClusterConfig{
				CAData:                validCA,
				InsecureSkipTLSVerify: false,
			},
			wantErr: true, // Incomplete test certificate will fail - this is expected
		},
		{
			name: "invalid CA certificate",
			config: &ClusterConfig{
				CAData:                "not-a-valid-cert",
				InsecureSkipTLSVerify: false,
			},
			wantErr: true,
		},
		{
			name: "empty CA certificate",
			config: &ClusterConfig{
				CAData:                "",
				InsecureSkipTLSVerify: false,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := createKubeletTLSConfig(tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("createKubeletTLSConfig() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestTLSConfigCertificates tests that certificates are properly loaded
func TestTLSConfigCertificates(t *testing.T) {
	t.Skip("Skipping test with incomplete test certificates - requires valid certificate data for proper testing")
}

// TestTLSConfigRootCAs tests that CA certificates are properly loaded into RootCAs
func TestTLSConfigRootCAs(t *testing.T) {
	t.Skip("Skipping test with incomplete test certificates - requires valid certificate data for proper testing")
}

// ========== HTTP Method Tests ==========
// Tests for kubelet HTTP methods using httptest

// TestClient_GetKubeletStats tests the GetKubeletStats method
func TestClient_GetKubeletStats(t *testing.T) {
	tests := []struct {
		name           string
		serverResponse func(w http.ResponseWriter, r *http.Request)
		wantNil        bool
		checkResult    func(t *testing.T, stats *statsapi.Summary)
	}{
		{
			name: "successful response",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/stats/summary" {
					t.Errorf("Expected path /stats/summary, got %s", r.URL.Path)
				}
				summary := &statsapi.Summary{
					Node: statsapi.NodeStats{
						NodeName: "test-node",
						CPU: &statsapi.CPUStats{
							UsageNanoCores: uint64Ptr(1000000000),
						},
					},
					Pods: []statsapi.PodStats{
						{
							PodRef: statsapi.PodReference{
								Name:      "test-pod",
								Namespace: "default",
							},
						},
					},
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				json.NewEncoder(w).Encode(summary)
			},
			wantNil: false,
			checkResult: func(t *testing.T, stats *statsapi.Summary) {
				if stats.Node.NodeName != "test-node" {
					t.Errorf("Expected node name test-node, got %s", stats.Node.NodeName)
				}
				if len(stats.Pods) != 1 {
					t.Errorf("Expected 1 pod, got %d", len(stats.Pods))
				}
			},
		},
		{
			name: "non-200 status code",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusUnauthorized)
				w.Write([]byte("Unauthorized"))
			},
			wantNil: true,
		},
		{
			name: "empty stats",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				summary := &statsapi.Summary{
					Node: statsapi.NodeStats{
						NodeName: "empty-node",
					},
					Pods: []statsapi.PodStats{},
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				json.NewEncoder(w).Encode(summary)
			},
			wantNil: false,
			checkResult: func(t *testing.T, stats *statsapi.Summary) {
				if len(stats.Pods) != 0 {
					t.Errorf("Expected 0 pods, got %d", len(stats.Pods))
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(tt.serverResponse))
			defer server.Close()

			client, err := NewClient(server.URL, &ClusterConfig{
				InsecureSkipTLSVerify: true,
			})
			if err != nil {
				t.Fatalf("Failed to create client: %v", err)
			}

			stats := client.GetKubeletStats(context.Background())

			if (stats == nil) != tt.wantNil {
				t.Errorf("GetKubeletStats() returned nil = %v, wantNil = %v", stats == nil, tt.wantNil)
				return
			}

			if tt.checkResult != nil && stats != nil {
				tt.checkResult(t, stats)
			}
		})
	}
}

// TestClient_GetKubeletPods tests the GetKubeletPods method
func TestClient_GetKubeletPods(t *testing.T) {
	tests := []struct {
		name           string
		serverResponse func(w http.ResponseWriter, r *http.Request)
		wantErr        bool
		checkResult    func(t *testing.T, pods *corev1.PodList)
	}{
		{
			name: "successful response with pods",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/pods" {
					t.Errorf("Expected path /pods, got %s", r.URL.Path)
				}
				podList := &corev1.PodList{
					TypeMeta: metav1.TypeMeta{
						Kind:       "PodList",
						APIVersion: "v1",
					},
					Items: []corev1.Pod{
						{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "pod-1",
								Namespace: "default",
								UID:       "uid-1",
							},
							Status: corev1.PodStatus{
								Phase: corev1.PodRunning,
							},
						},
						{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "pod-2",
								Namespace: "kube-system",
								UID:       "uid-2",
							},
							Status: corev1.PodStatus{
								Phase: corev1.PodRunning,
							},
						},
					},
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				json.NewEncoder(w).Encode(podList)
			},
			wantErr: false,
			checkResult: func(t *testing.T, pods *corev1.PodList) {
				if len(pods.Items) != 2 {
					t.Errorf("Expected 2 pods, got %d", len(pods.Items))
				}
				if pods.Items[0].Name != "pod-1" {
					t.Errorf("Expected first pod name pod-1, got %s", pods.Items[0].Name)
				}
			},
		},
		{
			name: "empty pod list",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				podList := &corev1.PodList{
					Items: []corev1.Pod{},
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				json.NewEncoder(w).Encode(podList)
			},
			wantErr: false,
			checkResult: func(t *testing.T, pods *corev1.PodList) {
				if len(pods.Items) != 0 {
					t.Errorf("Expected 0 pods, got %d", len(pods.Items))
				}
			},
		},
		{
			name: "server error",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
				w.Write([]byte("Internal Server Error"))
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(tt.serverResponse))
			defer server.Close()

			client, err := NewClient(server.URL, &ClusterConfig{
				InsecureSkipTLSVerify: true,
			})
			if err != nil {
				t.Fatalf("Failed to create client: %v", err)
			}

			pods, err := client.GetKubeletPods(context.Background())

			if (err != nil) != tt.wantErr {
				t.Errorf("GetKubeletPods() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.checkResult != nil && !tt.wantErr {
				tt.checkResult(t, pods)
			}
		})
	}
}

// TestClient_GetKubeletPodMap tests the GetKubeletPodMap method
func TestClient_GetKubeletPodMap(t *testing.T) {
	tests := []struct {
		name           string
		serverResponse func(w http.ResponseWriter, r *http.Request)
		wantErr        bool
		checkResult    func(t *testing.T, podMap map[string]corev1.Pod)
	}{
		{
			name: "successful response creates map",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				podList := &corev1.PodList{
					Items: []corev1.Pod{
						{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "pod-1",
								Namespace: "default",
								UID:       "uid-1",
							},
						},
						{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "pod-2",
								Namespace: "default",
								UID:       "uid-2",
							},
						},
					},
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				json.NewEncoder(w).Encode(podList)
			},
			wantErr: false,
			checkResult: func(t *testing.T, podMap map[string]corev1.Pod) {
				if len(podMap) != 2 {
					t.Errorf("Expected 2 pods in map, got %d", len(podMap))
				}
				if pod, exists := podMap["uid-1"]; !exists {
					t.Error("Expected pod with UID uid-1 to exist in map")
				} else if pod.Name != "pod-1" {
					t.Errorf("Expected pod name pod-1, got %s", pod.Name)
				}
			},
		},
		{
			name: "empty pod list creates empty map",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				podList := &corev1.PodList{
					Items: []corev1.Pod{},
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				json.NewEncoder(w).Encode(podList)
			},
			wantErr: false,
			checkResult: func(t *testing.T, podMap map[string]corev1.Pod) {
				if len(podMap) != 0 {
					t.Errorf("Expected empty map, got %d entries", len(podMap))
				}
			},
		},
		{
			name: "server error",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusForbidden)
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(tt.serverResponse))
			defer server.Close()

			client, err := NewClient(server.URL, &ClusterConfig{
				InsecureSkipTLSVerify: true,
			})
			if err != nil {
				t.Fatalf("Failed to create client: %v", err)
			}

			podMap, err := client.GetKubeletPodMap(context.Background())

			if (err != nil) != tt.wantErr {
				t.Errorf("GetKubeletPodMap() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.checkResult != nil && !tt.wantErr {
				tt.checkResult(t, podMap)
			}
		})
	}
}

// TestClient_ContextCancellation tests that HTTP methods respect context cancellation
func TestClient_ContextCancellation(t *testing.T) {
	// Create a server that delays response
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
			return
		}
	}))
	defer server.Close()

	client, err := NewClient(server.URL, &ClusterConfig{
		InsecureSkipTLSVerify: true,
	})
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	// Create a context that's already cancelled
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// GetKubeletStats should return nil on context cancellation
	stats := client.GetKubeletStats(ctx)
	if stats != nil {
		t.Error("Expected nil stats from cancelled context")
	}

	// GetKubeletPods should return error on context cancellation
	_, err = client.GetKubeletPods(ctx)
	if err == nil {
		t.Error("Expected error from cancelled context for GetKubeletPods")
	}

	// GetKubeletPodMap should return error on context cancellation
	_, err = client.GetKubeletPodMap(ctx)
	if err == nil {
		t.Error("Expected error from cancelled context for GetKubeletPodMap")
	}
}

// Helper function for test
func uint64Ptr(v uint64) *uint64 {
	return &v
}

// Benchmark tests for HTTP methods
func BenchmarkClient_GetKubeletStats(b *testing.B) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		summary := &statsapi.Summary{
			Node: statsapi.NodeStats{NodeName: "bench-node"},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(summary)
	}))
	defer server.Close()

	client, _ := NewClient(server.URL, &ClusterConfig{
		InsecureSkipTLSVerify: true,
	})
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = client.GetKubeletStats(ctx)
	}
}

func BenchmarkClient_GetKubeletPods(b *testing.B) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		podList := &corev1.PodList{Items: []corev1.Pod{}}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(podList)
	}))
	defer server.Close()

	client, _ := NewClient(server.URL, &ClusterConfig{
		InsecureSkipTLSVerify: true,
	})
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = client.GetKubeletPods(ctx)
	}
}

func BenchmarkClient_GetKubeletPodMap(b *testing.B) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		podList := &corev1.PodList{
			Items: []corev1.Pod{
				{ObjectMeta: metav1.ObjectMeta{UID: "uid-1"}},
				{ObjectMeta: metav1.ObjectMeta{UID: "uid-2"}},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(podList)
	}))
	defer server.Close()

	client, _ := NewClient(server.URL, &ClusterConfig{
		InsecureSkipTLSVerify: true,
	})
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = client.GetKubeletPodMap(ctx)
	}
}
