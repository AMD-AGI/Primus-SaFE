package clientsets

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"k8s.io/client-go/rest"
)

// TestTruncateString tests the truncateString function
func TestTruncateString(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		maxLen int
		want   string
	}{
		{
			name:   "string shorter than maxLen",
			input:  "hello",
			maxLen: 10,
			want:   "hello",
		},
		{
			name:   "string equal to maxLen",
			input:  "hello",
			maxLen: 5,
			want:   "hello",
		},
		{
			name:   "string longer than maxLen",
			input:  "hello world",
			maxLen: 5,
			want:   "hello...",
		},
		{
			name:   "empty string",
			input:  "",
			maxLen: 10,
			want:   "",
		},
		{
			name:   "maxLen is 0",
			input:  "hello",
			maxLen: 0,
			want:   "...",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncateString(tt.input, tt.maxLen)
			if got != tt.want {
				t.Errorf("truncateString() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestDecodeIfBase64 tests the decodeIfBase64 function
func TestDecodeIfBase64(t *testing.T) {
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
			name:    "plain text string (not base64)",
			input:   "-----BEGIN CERTIFICATE-----",
			want:    "-----BEGIN CERTIFICATE-----",
			wantErr: false,
		},
		{
			name:    "empty string",
			input:   "",
			want:    "",
			wantErr: false,
		},
		{
			name:    "plain text with spaces",
			input:   "this is plain text",
			want:    "this is plain text",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := decodeIfBase64(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("decodeIfBase64() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("decodeIfBase64() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestMultiClusterConfig_LoadFromSecret tests MultiClusterConfig.LoadFromSecret
func TestMultiClusterConfig_LoadFromSecret(t *testing.T) {
	tests := []struct {
		name    string
		data    map[string][]byte
		wantErr bool
		check   func(t *testing.T, cfg MultiClusterConfig)
	}{
		{
			name: "valid single cluster config",
			data: map[string][]byte{
				"cluster1": []byte(`{
					"host": "https://api.cluster1.example.com",
					"bearerToken": "token123",
					"insecureSkipTLSVerify": true
				}`),
			},
			wantErr: false,
			check: func(t *testing.T, cfg MultiClusterConfig) {
				if len(cfg) != 1 {
					t.Errorf("Expected 1 cluster, got %d", len(cfg))
				}
				if cfg["cluster1"].Host != "https://api.cluster1.example.com" {
					t.Errorf("Expected host to be https://api.cluster1.example.com, got %s", cfg["cluster1"].Host)
				}
				if cfg["cluster1"].BearerToken != "token123" {
					t.Errorf("Expected bearerToken to be token123, got %s", cfg["cluster1"].BearerToken)
				}
			},
		},
		{
			name: "multiple clusters config",
			data: map[string][]byte{
				"cluster1": []byte(`{"host": "https://api.cluster1.example.com"}`),
				"cluster2": []byte(`{"host": "https://api.cluster2.example.com"}`),
			},
			wantErr: false,
			check: func(t *testing.T, cfg MultiClusterConfig) {
				if len(cfg) != 2 {
					t.Errorf("Expected 2 clusters, got %d", len(cfg))
				}
			},
		},
		{
			name: "empty data",
			data: map[string][]byte{
				"cluster1": []byte{},
			},
			wantErr: false,
			check: func(t *testing.T, cfg MultiClusterConfig) {
				if len(cfg) != 0 {
					t.Errorf("Expected 0 clusters (empty data should be skipped), got %d", len(cfg))
				}
			},
		},
		{
			name: "invalid JSON",
			data: map[string][]byte{
				"cluster1": []byte(`{invalid json}`),
			},
			wantErr: true,
			check:   nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := MultiClusterConfig{}
			err := cfg.LoadFromSecret(tt.data)
			if (err != nil) != tt.wantErr {
				t.Errorf("LoadFromSecret() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.check != nil {
				tt.check(t, cfg)
			}
		})
	}
}

// TestClusterConfig_ToRestConfig tests ClusterConfig.ToRestConfig
func TestClusterConfig_ToRestConfig(t *testing.T) {
	// Create a temporary kubeconfig file for testing
	tmpDir := t.TempDir()
	kubeconfigPath := filepath.Join(tmpDir, "config")
	kubeconfigContent := `
apiVersion: v1
kind: Config
clusters:
- cluster:
    server: https://test-server:6443
  name: test-cluster
contexts:
- context:
    cluster: test-cluster
    user: test-user
  name: test-context
current-context: test-context
users:
- name: test-user
  user:
    token: test-token
`
	if err := os.WriteFile(kubeconfigPath, []byte(kubeconfigContent), 0644); err != nil {
		t.Fatalf("Failed to create test kubeconfig: %v", err)
	}

	tests := []struct {
		name    string
		config  ClusterConfig
		wantErr bool
		check   func(t *testing.T, cfg *rest.Config)
	}{
		{
			name: "valid config with host and certs",
			config: ClusterConfig{
				Host:                  "https://api.example.com",
				InsecureSkipTLSVerify: true,
				CertData:              "cert-data",
				KeyData:               "key-data",
			},
			wantErr: false,
			check: func(t *testing.T, cfg *rest.Config) {
				if cfg.Host != "https://api.example.com" {
					t.Errorf("Expected host to be https://api.example.com, got %s", cfg.Host)
				}
				if !cfg.TLSClientConfig.Insecure {
					t.Errorf("Expected Insecure to be true")
				}
			},
		},
		{
			name: "valid config with kubeconfig file",
			config: ClusterConfig{
				Kubeconfig: kubeconfigPath,
			},
			wantErr: false,
			check: func(t *testing.T, cfg *rest.Config) {
				if cfg.Host != "https://test-server:6443" {
					t.Errorf("Expected host from kubeconfig, got %s", cfg.Host)
				}
			},
		},
		{
			name: "kubeconfig file not found",
			config: ClusterConfig{
				Kubeconfig: "/non/existent/path",
			},
			wantErr: true,
			check:   nil,
		},
		{
			name:    "missing host when no kubeconfig",
			config:  ClusterConfig{},
			wantErr: true,
			check:   nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := tt.config.ToRestConfig()
			if (err != nil) != tt.wantErr {
				t.Errorf("ToRestConfig() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.check != nil && cfg != nil {
				tt.check(t, cfg)
			}
		})
	}
}

// TestPrimusLensClientConfig_LoadFromSecret tests PrimusLensClientConfig.LoadFromSecret
func TestPrimusLensClientConfig_LoadFromSecret(t *testing.T) {
	tests := []struct {
		name    string
		data    map[string][]byte
		wantErr bool
		check   func(t *testing.T, cfg *PrimusLensClientConfig)
	}{
		{
			name: "valid opensearch config",
			data: map[string][]byte{
				"opensearch": []byte(`{
					"service": "opensearch-service",
					"namespace": "primus-lens",
					"port": 9200,
					"scheme": "https",
					"username": "admin",
					"password": "admin123"
				}`),
			},
			wantErr: false,
			check: func(t *testing.T, cfg *PrimusLensClientConfig) {
				if cfg.Opensearch == nil {
					t.Fatal("Expected Opensearch config to be set")
				}
				if cfg.Opensearch.Service != "opensearch-service" {
					t.Errorf("Expected service to be opensearch-service, got %s", cfg.Opensearch.Service)
				}
				if cfg.Opensearch.Port != 9200 {
					t.Errorf("Expected port to be 9200, got %d", cfg.Opensearch.Port)
				}
			},
		},
		{
			name: "valid prometheus config",
			data: map[string][]byte{
				"prometheus": []byte(`{
					"write_service": "vminsert",
					"write_port": 8480,
					"read_service": "vmselect",
					"read_port": 8481,
					"namespace": "primus-lens"
				}`),
			},
			wantErr: false,
			check: func(t *testing.T, cfg *PrimusLensClientConfig) {
				if cfg.Prometheus == nil {
					t.Fatal("Expected Prometheus config to be set")
				}
				if cfg.Prometheus.WriteService != "vminsert" {
					t.Errorf("Expected write_service to be vminsert, got %s", cfg.Prometheus.WriteService)
				}
			},
		},
		{
			name: "valid postgres config",
			data: map[string][]byte{
				"postgres": []byte(`{
					"service": "postgres-service",
					"namespace": "primus-lens",
					"port": 5432,
					"username": "admin",
					"password": "password",
					"db_name": "primus_lens",
					"ssl_mode": "disable"
				}`),
			},
			wantErr: false,
			check: func(t *testing.T, cfg *PrimusLensClientConfig) {
				if cfg.Postgres == nil {
					t.Fatal("Expected Postgres config to be set")
				}
				if cfg.Postgres.Service != "postgres-service" {
					t.Errorf("Expected service to be postgres-service, got %s", cfg.Postgres.Service)
				}
				if cfg.Postgres.DBName != "primus_lens" {
					t.Errorf("Expected db_name to be primus_lens, got %s", cfg.Postgres.DBName)
				}
			},
		},
		{
			name: "all configs",
			data: map[string][]byte{
				"opensearch": []byte(`{"service": "opensearch", "port": 9200}`),
				"prometheus": []byte(`{"write_service": "vminsert", "write_port": 8480}`),
				"postgres":   []byte(`{"service": "postgres", "port": 5432}`),
			},
			wantErr: false,
			check: func(t *testing.T, cfg *PrimusLensClientConfig) {
				if cfg.Opensearch == nil || cfg.Prometheus == nil || cfg.Postgres == nil {
					t.Error("Expected all configs to be set")
				}
			},
		},
		{
			name: "invalid JSON in opensearch",
			data: map[string][]byte{
				"opensearch": []byte(`{invalid}`),
			},
			wantErr: true,
			check:   nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &PrimusLensClientConfig{}
			err := cfg.LoadFromSecret(tt.data)
			if (err != nil) != tt.wantErr {
				t.Errorf("LoadFromSecret() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.check != nil {
				tt.check(t, cfg)
			}
		})
	}
}

// TestPrimusLensClientConfig_Equals tests PrimusLensClientConfig.Equals
func TestPrimusLensClientConfig_Equals(t *testing.T) {
	tests := []struct {
		name  string
		cfg1  *PrimusLensClientConfig
		cfg2  *PrimusLensClientConfig
		equal bool
	}{
		{
			name: "equal configs with all fields",
			cfg1: &PrimusLensClientConfig{
				Opensearch: &PrimusLensClientConfigOpensearch{Service: "os", Port: 9200},
				Prometheus: &PrimusLensClientConfigPrometheus{WriteService: "vm", WritePort: 8480},
				Postgres:   &PrimusLensClientConfigPostgres{Service: "pg", Port: 5432},
			},
			cfg2: &PrimusLensClientConfig{
				Opensearch: &PrimusLensClientConfigOpensearch{Service: "os", Port: 9200},
				Prometheus: &PrimusLensClientConfigPrometheus{WriteService: "vm", WritePort: 8480},
				Postgres:   &PrimusLensClientConfigPostgres{Service: "pg", Port: 5432},
			},
			equal: true,
		},
		{
			name: "different opensearch config",
			cfg1: &PrimusLensClientConfig{
				Opensearch: &PrimusLensClientConfigOpensearch{Service: "os1", Port: 9200},
			},
			cfg2: &PrimusLensClientConfig{
				Opensearch: &PrimusLensClientConfigOpensearch{Service: "os2", Port: 9200},
			},
			equal: false,
		},
		{
			name: "one has opensearch, other doesn't",
			cfg1: &PrimusLensClientConfig{
				Opensearch: &PrimusLensClientConfigOpensearch{Service: "os", Port: 9200},
			},
			cfg2:  &PrimusLensClientConfig{},
			equal: false,
		},
		{
			name:  "both empty configs",
			cfg1:  &PrimusLensClientConfig{},
			cfg2:  &PrimusLensClientConfig{},
			equal: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.cfg1.Equals(tt.cfg2)
			if got != tt.equal {
				t.Errorf("Equals() = %v, want %v", got, tt.equal)
			}
		})
	}
}

// TestPrimusLensClientConfigOpensearch_Equals tests Opensearch config equality
func TestPrimusLensClientConfigOpensearch_Equals(t *testing.T) {
	tests := []struct {
		name  string
		cfg1  PrimusLensClientConfigOpensearch
		cfg2  PrimusLensClientConfigOpensearch
		equal bool
	}{
		{
			name: "equal configs",
			cfg1: PrimusLensClientConfigOpensearch{
				Service:   "opensearch",
				Namespace: "primus-lens",
				Port:      9200,
				NodePort:  30920,
				Scheme:    "https",
				Username:  "admin",
				Password:  "admin123",
			},
			cfg2: PrimusLensClientConfigOpensearch{
				Service:   "opensearch",
				Namespace: "primus-lens",
				Port:      9200,
				NodePort:  30920,
				Scheme:    "https",
				Username:  "admin",
				Password:  "admin123",
			},
			equal: true,
		},
		{
			name: "different service",
			cfg1: PrimusLensClientConfigOpensearch{Service: "opensearch1"},
			cfg2: PrimusLensClientConfigOpensearch{Service: "opensearch2"},
			equal: false,
		},
		{
			name: "different port",
			cfg1: PrimusLensClientConfigOpensearch{Port: 9200},
			cfg2: PrimusLensClientConfigOpensearch{Port: 9201},
			equal: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.cfg1.Equals(tt.cfg2)
			if got != tt.equal {
				t.Errorf("Equals() = %v, want %v", got, tt.equal)
			}
		})
	}
}

// TestPrimusLensClientConfigPrometheus_Equals tests Prometheus config equality
func TestPrimusLensClientConfigPrometheus_Equals(t *testing.T) {
	tests := []struct {
		name  string
		cfg1  PrimusLensClientConfigPrometheus
		cfg2  PrimusLensClientConfigPrometheus
		equal bool
	}{
		{
			name: "equal configs",
			cfg1: PrimusLensClientConfigPrometheus{
				WriteService:  "vminsert",
				WritePort:     8480,
				ReadService:   "vmselect",
				ReadPort:      8481,
				WriteNodePort: 30480,
				ReadNodePort:  30481,
				Namespace:     "primus-lens",
			},
			cfg2: PrimusLensClientConfigPrometheus{
				WriteService:  "vminsert",
				WritePort:     8480,
				ReadService:   "vmselect",
				ReadPort:      8481,
				WriteNodePort: 30480,
				ReadNodePort:  30481,
				Namespace:     "primus-lens",
			},
			equal: true,
		},
		{
			name: "different write service",
			cfg1: PrimusLensClientConfigPrometheus{WriteService: "vminsert1"},
			cfg2: PrimusLensClientConfigPrometheus{WriteService: "vminsert2"},
			equal: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.cfg1.Equals(tt.cfg2)
			if got != tt.equal {
				t.Errorf("Equals() = %v, want %v", got, tt.equal)
			}
		})
	}
}

// TestPrimusLensClientConfigPostgres_Equals tests Postgres config equality
func TestPrimusLensClientConfigPostgres_Equals(t *testing.T) {
	tests := []struct {
		name  string
		cfg1  PrimusLensClientConfigPostgres
		cfg2  PrimusLensClientConfigPostgres
		equal bool
	}{
		{
			name: "equal configs",
			cfg1: PrimusLensClientConfigPostgres{
				Service:   "postgres",
				Namespace: "primus-lens",
				Port:      5432,
				NodePort:  30432,
				Username:  "admin",
				Password:  "password",
				DBName:    "primus_lens",
				SSLMode:   "disable",
			},
			cfg2: PrimusLensClientConfigPostgres{
				Service:   "postgres",
				Namespace: "primus-lens",
				Port:      5432,
				NodePort:  30432,
				Username:  "admin",
				Password:  "password",
				DBName:    "primus_lens",
				SSLMode:   "disable",
			},
			equal: true,
		},
		{
			name: "different database name",
			cfg1: PrimusLensClientConfigPostgres{DBName: "db1"},
			cfg2: PrimusLensClientConfigPostgres{DBName: "db2"},
			equal: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.cfg1.Equals(tt.cfg2)
			if got != tt.equal {
				t.Errorf("Equals() = %v, want %v", got, tt.equal)
			}
		})
	}
}

// TestPrimusLensMultiClusterClientConfig_LoadFromSecret tests multi-cluster config loading
func TestPrimusLensMultiClusterClientConfig_LoadFromSecret(t *testing.T) {
	// Create base64 encoded configs
	opensearchConfig := base64.StdEncoding.EncodeToString([]byte(`{"service": "opensearch", "port": 9200}`))
	prometheusConfig := base64.StdEncoding.EncodeToString([]byte(`{"write_service": "vminsert", "write_port": 8480}`))
	postgresConfig := base64.StdEncoding.EncodeToString([]byte(`{"service": "postgres", "port": 5432}`))

	tests := []struct {
		name    string
		data    map[string][]byte
		wantErr bool
		check   func(t *testing.T, cfg PrimusLensMultiClusterClientConfig)
	}{
		{
			name: "valid single cluster config",
			data: map[string][]byte{
				"cluster1": []byte(createMultiClusterConfigJSON(opensearchConfig, prometheusConfig, postgresConfig)),
			},
			wantErr: false,
			check: func(t *testing.T, cfg PrimusLensMultiClusterClientConfig) {
				if len(cfg) != 1 {
					t.Errorf("Expected 1 cluster, got %d", len(cfg))
				}
				if cfg["cluster1"].Opensearch == nil {
					t.Error("Expected Opensearch config to be set")
				}
			},
		},
		{
			name: "multiple clusters config",
			data: map[string][]byte{
				"cluster1": []byte(createMultiClusterConfigJSON(opensearchConfig, "", "")),
				"cluster2": []byte(createMultiClusterConfigJSON(opensearchConfig, "", "")),
			},
			wantErr: false,
			check: func(t *testing.T, cfg PrimusLensMultiClusterClientConfig) {
				if len(cfg) != 2 {
					t.Errorf("Expected 2 clusters, got %d", len(cfg))
				}
			},
		},
		{
			name: "invalid base64 encoding",
			data: map[string][]byte{
				"cluster1": []byte(`{"opensearch": "not-base64!!!"}`),
			},
			wantErr: true,
			check:   nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := PrimusLensMultiClusterClientConfig{}
			err := cfg.LoadFromSecret(tt.data)
			if (err != nil) != tt.wantErr {
				t.Errorf("LoadFromSecret() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.check != nil {
				tt.check(t, cfg)
			}
		})
	}
}

// Helper function to create multi-cluster config JSON
func createMultiClusterConfigJSON(opensearch, prometheus, postgres string) string {
	config := make(map[string]string)
	if opensearch != "" {
		config["opensearch"] = opensearch
	}
	if prometheus != "" {
		config["prometheus"] = prometheus
	}
	if postgres != "" {
		config["postgres"] = postgres
	}
	bytes, _ := json.Marshal(config)
	return string(bytes)
}

// TestCreateRestConfig tests the createRestConfig function
func TestCreateRestConfig(t *testing.T) {
	certData := "-----BEGIN CERTIFICATE-----\ntest-cert\n-----END CERTIFICATE-----"
	keyData := "-----BEGIN RSA PRIVATE KEY-----\ntest-key\n-----END RSA PRIVATE KEY-----"
	caData := "-----BEGIN CERTIFICATE-----\ntest-ca\n-----END CERTIFICATE-----"

	tests := []struct {
		name     string
		endpoint string
		certData string
		keyData  string
		caData   string
		insecure bool
		wantErr  bool
		check    func(t *testing.T, cfg *rest.Config)
	}{
		{
			name:     "insecure config",
			endpoint: "https://api.example.com",
			certData: "",
			keyData:  "",
			caData:   "",
			insecure: true,
			wantErr:  false,
			check: func(t *testing.T, cfg *rest.Config) {
				if cfg.Host != "https://api.example.com" {
					t.Errorf("Expected host to be https://api.example.com, got %s", cfg.Host)
				}
				if !cfg.TLSClientConfig.Insecure {
					t.Error("Expected Insecure to be true")
				}
			},
		},
		{
			name:     "secure config with certs - not valid certs so will work but data is set",
			endpoint: "https://api.example.com",
			certData: certData,
			keyData:  keyData,
			caData:   caData,
			insecure: false,
			wantErr:  false,
			check: func(t *testing.T, cfg *rest.Config) {
				if cfg.TLSClientConfig.Insecure {
					t.Error("Expected Insecure to be false")
				}
				if len(cfg.TLSClientConfig.CertData) == 0 {
					t.Error("Expected CertData to be set")
				}
				if len(cfg.TLSClientConfig.KeyData) == 0 {
					t.Error("Expected KeyData to be set")
				}
				if len(cfg.TLSClientConfig.CAData) == 0 {
					t.Error("Expected CAData to be set")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := createRestConfig(tt.endpoint, tt.certData, tt.keyData, tt.caData, tt.insecure)
			if (err != nil) != tt.wantErr {
				t.Errorf("createRestConfig() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.check != nil && cfg != nil {
				tt.check(t, cfg)
			}
		})
	}
}

