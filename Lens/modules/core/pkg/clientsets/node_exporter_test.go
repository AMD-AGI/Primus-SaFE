package clientsets

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model/rest"
)

// TestNewNodeExporterClient tests the constructor
func TestNewNodeExporterClient(t *testing.T) {
	address := "http://localhost:8989"
	client := NewNodeExporterClient(address)

	if client == nil {
		t.Fatal("Expected non-nil client")
	}

	if client.address != address {
		t.Errorf("Expected address to be %s, got %s", address, client.address)
	}

	if client.api == nil {
		t.Error("Expected API client to be initialized")
	}
}

// TestNodeExporterClient_GetRestyClient tests the GetRestyClient method
func TestNodeExporterClient_GetRestyClient(t *testing.T) {
	client := NewNodeExporterClient("http://localhost:8989")

	restyClient := client.GetRestyClient()
	if restyClient == nil {
		t.Error("Expected non-nil resty client")
	}

	// Verify it's a clone (different instance)
	if restyClient == client.api {
		t.Error("Expected GetRestyClient to return a clone, not the same instance")
	}
}

// TestNodeExporterClient_GetGPUs tests the GetGPUs method
func TestNodeExporterClient_GetGPUs(t *testing.T) {
	tests := []struct {
		name           string
		serverResponse func(w http.ResponseWriter, r *http.Request)
		wantErr        bool
		checkResult    func(t *testing.T, gpus []model.GPUInfo)
	}{
		{
			name: "successful response",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/v1/gpus" {
					t.Errorf("Expected path /v1/gpus, got %s", r.URL.Path)
				}
				response := rest.Response{
					Meta: rest.Meta{
						Code:    rest.CodeSuccess,
						Message: "success",
					},
					Data: []model.GPUInfo{
						{
							GPU: 0,
						},
						{
							GPU: 1,
						},
					},
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				json.NewEncoder(w).Encode(response)
			},
			wantErr: false,
			checkResult: func(t *testing.T, gpus []model.GPUInfo) {
				if len(gpus) != 2 {
					t.Errorf("Expected 2 GPUs, got %d", len(gpus))
				}
				if gpus[0].GPU != 0 {
					t.Errorf("Expected first GPU index to be 0, got %d", gpus[0].GPU)
				}
			},
		},
		{
			name: "non-200 status code",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
				w.Write([]byte("Internal Server Error"))
			},
			wantErr: true,
		},
		{
			name: "response with error code",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				response := rest.Response{
					Meta: rest.Meta{
						Code:    5000, // Use a custom error code
						Message: "Internal error",
					},
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				json.NewEncoder(w).Encode(response)
			},
			wantErr: true,
		},
		{
			name: "empty GPU list",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				response := rest.Response{
					Meta: rest.Meta{
						Code:    rest.CodeSuccess,
						Message: "success",
					},
					Data: []model.GPUInfo{},
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				json.NewEncoder(w).Encode(response)
			},
			wantErr: false,
			checkResult: func(t *testing.T, gpus []model.GPUInfo) {
				if len(gpus) != 0 {
					t.Errorf("Expected 0 GPUs, got %d", len(gpus))
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test server
			server := httptest.NewServer(http.HandlerFunc(tt.serverResponse))
			defer server.Close()

			// Create client pointing to test server
			client := NewNodeExporterClient(server.URL)

			// Call the method
			gpus, err := client.GetGPUs(context.Background())

			// Check error
			if (err != nil) != tt.wantErr {
				t.Errorf("GetGPUs() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			// Check result
			if tt.checkResult != nil && !tt.wantErr {
				tt.checkResult(t, gpus)
			}
		})
	}
}

// TestNodeExporterClient_GetDriverVersion tests the GetDriverVersion method
func TestNodeExporterClient_GetDriverVersion(t *testing.T) {
	tests := []struct {
		name           string
		serverResponse func(w http.ResponseWriter, r *http.Request)
		wantErr        bool
		expectedVer    string
	}{
		{
			name: "successful response",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/v1/gpuDriverVersion" {
					t.Errorf("Expected path /v1/gpuDriverVersion, got %s", r.URL.Path)
				}
				response := rest.Response{
					Meta: rest.Meta{
						Code:    rest.CodeSuccess,
						Message: "success",
					},
					Data: "6.0.2",
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				json.NewEncoder(w).Encode(response)
			},
			wantErr:     false,
			expectedVer: "6.0.2",
		},
		{
			name: "non-200 status code",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusNotFound)
				w.Write([]byte("Not Found"))
			},
			wantErr: true,
		},
		{
			name: "response with error code",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				response := rest.Response{
					Meta: rest.Meta{
						Code:    5001,
						Message: "Failed to get driver version",
					},
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				json.NewEncoder(w).Encode(response)
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(tt.serverResponse))
			defer server.Close()

			client := NewNodeExporterClient(server.URL)
			version, err := client.GetDriverVersion(context.Background())

			if (err != nil) != tt.wantErr {
				t.Errorf("GetDriverVersion() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && version != tt.expectedVer {
				t.Errorf("GetDriverVersion() = %v, want %v", version, tt.expectedVer)
			}
		})
	}
}

// TestNodeExporterClient_GetCardMetrics tests the GetCardMetrics method
func TestNodeExporterClient_GetCardMetrics(t *testing.T) {
	tests := []struct {
		name           string
		serverResponse func(w http.ResponseWriter, r *http.Request)
		wantErr        bool
		checkResult    func(t *testing.T, metrics []model.CardMetrics)
	}{
		{
			name: "successful response",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/v1/cardMetrics" {
					t.Errorf("Expected path /v1/cardMetrics, got %s", r.URL.Path)
				}
				response := rest.Response{
					Meta: rest.Meta{
						Code:    rest.CodeSuccess,
						Message: "success",
					},
					Data: []model.CardMetrics{
						{
							Gpu:                 0,
							TemperatureJunction: 65.5,
							GPUUsePercent:       80.5,
						},
					},
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				json.NewEncoder(w).Encode(response)
			},
			wantErr: false,
			checkResult: func(t *testing.T, metrics []model.CardMetrics) {
				if len(metrics) != 1 {
					t.Errorf("Expected 1 metric, got %d", len(metrics))
				}
				if metrics[0].TemperatureJunction != 65.5 {
					t.Errorf("Expected temperature 65.5, got %f", metrics[0].TemperatureJunction)
				}
			},
		},
		{
			name: "empty metrics",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				response := rest.Response{
					Meta: rest.Meta{
						Code:    rest.CodeSuccess,
						Message: "success",
					},
					Data: []model.CardMetrics{},
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				json.NewEncoder(w).Encode(response)
			},
			wantErr: false,
			checkResult: func(t *testing.T, metrics []model.CardMetrics) {
				if len(metrics) != 0 {
					t.Errorf("Expected 0 metrics, got %d", len(metrics))
				}
			},
		},
		{
			name: "server error",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusServiceUnavailable)
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(tt.serverResponse))
			defer server.Close()

			client := NewNodeExporterClient(server.URL)
			metrics, err := client.GetCardMetrics(context.Background())

			if (err != nil) != tt.wantErr {
				t.Errorf("GetCardMetrics() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.checkResult != nil && !tt.wantErr {
				tt.checkResult(t, metrics)
			}
		})
	}
}

// TestNodeExporterClient_GetRdmaDevices tests the GetRdmaDevices method
func TestNodeExporterClient_GetRdmaDevices(t *testing.T) {
	tests := []struct {
		name           string
		serverResponse func(w http.ResponseWriter, r *http.Request)
		wantErr        bool
		checkResult    func(t *testing.T, devices []model.RDMADevice)
	}{
		{
			name: "successful response with devices",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/v1/rdma" {
					t.Errorf("Expected path /v1/rdma, got %s", r.URL.Path)
				}
				response := rest.Response{
					Meta: rest.Meta{
						Code:    rest.CodeSuccess,
						Message: "success",
					},
					Data: []model.RDMADevice{
						{
							IfName:   "mlx5_0",
							IfIndex:  0,
							NodeGUID: "0x1234567890abcdef",
						},
						{
							IfName:   "mlx5_1",
							IfIndex:  1,
							NodeGUID: "0xfedcba0987654321",
						},
					},
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				json.NewEncoder(w).Encode(response)
			},
			wantErr: false,
			checkResult: func(t *testing.T, devices []model.RDMADevice) {
				if len(devices) != 2 {
					t.Errorf("Expected 2 devices, got %d", len(devices))
				}
				if devices[0].IfName != "mlx5_0" {
					t.Errorf("Expected device name mlx5_0, got %s", devices[0].IfName)
				}
			},
		},
		{
			name: "no RDMA devices",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				response := rest.Response{
					Meta: rest.Meta{
						Code:    rest.CodeSuccess,
						Message: "success",
					},
					Data: []model.RDMADevice{},
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				json.NewEncoder(w).Encode(response)
			},
			wantErr: false,
			checkResult: func(t *testing.T, devices []model.RDMADevice) {
				if len(devices) != 0 {
					t.Errorf("Expected 0 devices, got %d", len(devices))
				}
			},
		},
		{
			name: "server returns error",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				response := rest.Response{
					Meta: rest.Meta{
						Code:    5002,
						Message: "Failed to enumerate RDMA devices",
					},
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				json.NewEncoder(w).Encode(response)
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(tt.serverResponse))
			defer server.Close()

			client := NewNodeExporterClient(server.URL)
			devices, err := client.GetRdmaDevices(context.Background())

			if (err != nil) != tt.wantErr {
				t.Errorf("GetRdmaDevices() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.checkResult != nil && !tt.wantErr {
				tt.checkResult(t, devices)
			}
		})
	}
}

// TestNodeExporterClient_ContextCancellation tests context cancellation
func TestNodeExporterClient_ContextCancellation(t *testing.T) {
	// Create a server that delays response
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Delay to allow context cancellation
		select {
		case <-r.Context().Done():
			return
		}
	}))
	defer server.Close()

	client := NewNodeExporterClient(server.URL)

	// Create a context that's already cancelled
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// All methods should respect context cancellation
	_, err := client.GetGPUs(ctx)
	if err == nil {
		t.Error("Expected error from cancelled context for GetGPUs")
	}

	_, err = client.GetDriverVersion(ctx)
	if err == nil {
		t.Error("Expected error from cancelled context for GetDriverVersion")
	}

	_, err = client.GetCardMetrics(ctx)
	if err == nil {
		t.Error("Expected error from cancelled context for GetCardMetrics")
	}

	_, err = client.GetRdmaDevices(ctx)
	if err == nil {
		t.Error("Expected error from cancelled context for GetRdmaDevices")
	}
}

// Benchmark tests
func BenchmarkNodeExporterClient_GetGPUs(b *testing.B) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := rest.Response{
			Meta: rest.Meta{Code: rest.CodeSuccess, Message: "success"},
			Data: []model.GPUInfo{{GPU: 0}},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client := NewNodeExporterClient(server.URL)
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = client.GetGPUs(ctx)
	}
}

func BenchmarkNodeExporterClient_GetDriverVersion(b *testing.B) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := rest.Response{
			Meta: rest.Meta{Code: rest.CodeSuccess, Message: "success"},
			Data: "6.0.2",
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client := NewNodeExporterClient(server.URL)
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = client.GetDriverVersion(ctx)
	}
}
