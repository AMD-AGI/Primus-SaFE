// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package clientsets

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

type MultiClusterConfig map[string]ClusterConfig

func (m *MultiClusterConfig) LoadFromSecret(data map[string][]byte) error {
	// Ensure the map is initialized
	if *m == nil {
		*m = make(MultiClusterConfig)
	}

	// Iterate through each cluster configuration in the secret
	// Each key is the cluster name, value is the ClusterConfig in JSON format
	for clusterName, configBytes := range data {
		// Skip empty data
		if len(configBytes) == 0 {
			continue
		}
		log.Infof("Loading k8s config for cluster: %s", clusterName)
		// First unmarshal into intermediate structure with string fields
		var clusterCfg ClusterConfig
		if err := json.Unmarshal(configBytes, &clusterCfg); err != nil {
			return fmt.Errorf("failed to unmarshal cluster config for cluster %s: %w", clusterName, err)
		}

		// Store the parsed configuration in the map
		(*m)[clusterName] = clusterCfg
	}

	return nil
}

type ClusterConfig struct {
	Kubeconfig            string `yaml:"kubeconfig" json:"kubeconfig"`
	Host                  string `yaml:"host" json:"host"`
	BearerToken           string `yaml:"bearerToken" json:"bearerToken"`
	TLSServerName         string `yaml:"tlsServerName" json:"tlsServerName"`
	InsecureSkipTLSVerify bool   `yaml:"insecureSkipTLSVerify" json:"insecureSkipTLSVerify"`
	CAData                string `yaml:"caData" json:"caData"`
	CertData              string `yaml:"certData" json:"certData"`
	KeyData               string `yaml:"keyData" json:"keyData"`
}

func (c *ClusterConfig) ToRestConfig() (*rest.Config, error) {
	if c.Kubeconfig != "" {
		kubeconfig := c.Kubeconfig
		if kubeconfig == "~/.kube/config" {
			kubeconfig = filepath.Join(homedir.HomeDir(), ".kube", "config")
		}
		if _, err := os.Stat(kubeconfig); err == nil {
			return clientcmd.BuildConfigFromFlags("", kubeconfig)
		}
		return nil, fmt.Errorf("kubeconfig file not found: %s", kubeconfig)
	}

	if c.Host == "" {
		return nil, fmt.Errorf("host must be set if kubeconfig is not provided")
	}

	return createRestConfig(c.Host, c.CertData, c.KeyData, c.CAData, c.InsecureSkipTLSVerify)
}

// decodeIfBase64 attempts to decode a base64 string, returns original if not base64 encoded
func decodeIfBase64(data string) (string, error) {
	if data == "" {
		return "", nil
	}

	// Try to decode as base64
	decoded, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		// If decode fails, assume it's already plain text (e.g., PEM format)
		log.Infof("Data is not base64 encoded, using as-is (first 50 chars): %s", truncateString(data, 50))
		return data, nil
	}

	decodedStr := string(decoded)
	log.Infof("Successfully decoded base64 data (first 50 chars): %s", truncateString(decodedStr, 50))
	return decodedStr, nil
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

type PrimusLensClientConfig struct {
	Opensearch *PrimusLensClientConfigOpensearch `yaml:"opensearch" json:"opensearch,omitempty"`
	Prometheus *PrimusLensClientConfigPrometheus `yaml:"prometheus" json:"prometheus,omitempty"`
	Postgres   *PrimusLensClientConfigPostgres   `yaml:"postgres" json:"postgres,omitempty"`
}

func (p *PrimusLensClientConfig) LoadFromSecret(data map[string][]byte) error {
	if opensearchData, ok := data["opensearch"]; ok {
		var opensearchConfig PrimusLensClientConfigOpensearch
		if err := json.Unmarshal(opensearchData, &opensearchConfig); err != nil {
			return fmt.Errorf("failed to unmarshal opensearch config: %w", err)
		}
		p.Opensearch = &opensearchConfig
	}
	if prometheusData, ok := data["prometheus"]; ok {
		var prometheusConfig PrimusLensClientConfigPrometheus
		if err := json.Unmarshal(prometheusData, &prometheusConfig); err != nil {
			return fmt.Errorf("failed to unmarshal prometheus config: %w", err)
		}
		p.Prometheus = &prometheusConfig
	}
	if postgresData, ok := data["postgres"]; ok {
		var postgresConfig PrimusLensClientConfigPostgres
		if err := json.Unmarshal(postgresData, &postgresConfig); err != nil {
			return fmt.Errorf("failed to unmarshal postgres config: %w", err)
		}
		p.Postgres = &postgresConfig
	}
	return nil
}

func (p *PrimusLensClientConfig) Equals(other *PrimusLensClientConfig) bool {
	if (p.Opensearch == nil) != (other.Opensearch == nil) {
		return false
	}
	if p.Opensearch != nil && !p.Opensearch.Equals(*other.Opensearch) {
		return false
	}
	if (p.Prometheus == nil) != (other.Prometheus == nil) {
		return false
	}
	if p.Prometheus != nil && !p.Prometheus.Equals(*other.Prometheus) {
		return false
	}
	if (p.Postgres == nil) != (other.Postgres == nil) {
		return false
	}
	if p.Postgres != nil && !p.Postgres.Equals(*other.Postgres) {
		return false
	}
	return true
}

type PrimusLensMultiClusterClientConfig map[string]PrimusLensClientConfig

func (p *PrimusLensMultiClusterClientConfig) LoadFromSecret(data map[string][]byte) error {
	// Ensure the map is initialized
	if *p == nil {
		*p = make(PrimusLensMultiClusterClientConfig)
	}

	for clusterName, bytes := range data {
		// Skip empty data
		if len(bytes) == 0 {
			continue
		}
		log.Infof("Loading multi-cluster client config for cluster: %s", clusterName)

		// First unmarshal into intermediate structure with base64-encoded values
		var intermediate map[string]string
		if err := json.Unmarshal(bytes, &intermediate); err != nil {
			return fmt.Errorf("failed to unmarshal multi-cluster client config for %s: %w", clusterName, err)
		}

		// Create new map for decoded data
		decodedData := make(map[string][]byte)
		for key, base64Value := range intermediate {
			// Decode base64 string
			decoded, err := base64.StdEncoding.DecodeString(base64Value)
			if err != nil {
				return fmt.Errorf("failed to decode %s for cluster %s: %w", key, clusterName, err)
			}
			decodedData[key] = decoded
		}

		// Load configuration using decoded data
		singleCfg := PrimusLensClientConfig{}
		if err := singleCfg.LoadFromSecret(decodedData); err != nil {
			return fmt.Errorf("failed to load config for cluster %s: %w", clusterName, err)
		}

		(*p)[clusterName] = singleCfg
	}
	return nil
}

type PrimusLensClientConfigOpensearch struct {
	NodePort  int32  `yaml:"nodePort" json:"node_port,omitempty"`
	Port      int32  `yaml:"port" json:"port,omitempty"`
	Service   string `yaml:"service" json:"service,omitempty"`
	Namespace string `yaml:"namespace" json:"namespace,omitempty"`
	Scheme    string `yaml:"scheme" json:"scheme,omitempty"`
	Username  string `yaml:"username" json:"username,omitempty"`
	Password  string `yaml:"password" json:"password,omitempty"`
}

func (p PrimusLensClientConfigOpensearch) Equals(other PrimusLensClientConfigOpensearch) bool {
	return p.NodePort == other.NodePort &&
		p.Service == other.Service &&
		p.Namespace == other.Namespace &&
		p.Scheme == other.Scheme &&
		p.Username == other.Username &&
		p.Password == other.Password &&
		p.Port == other.Port
}

type PrimusLensClientConfigPrometheus struct {
	WriteService  string `yaml:"writeService" json:"write_service,omitempty"`
	WritePort     int32  `yaml:"writePort" json:"write_port,omitempty"`
	ReadService   string `yaml:"readService" json:"read_service,omitempty"`
	ReadPort      int32  `yaml:"readPort" json:"read_port,omitempty"`
	WriteNodePort int32  `yaml:"writeNodePort" json:"write_node_port,omitempty"`
	ReadNodePort  int32  `yaml:"readNodePort" json:"read_node_port,omitempty"`
	Namespace     string `yaml:"namespace" json:"namespace,omitempty"`
}

func (p PrimusLensClientConfigPrometheus) Equals(other PrimusLensClientConfigPrometheus) bool {
	return p.WriteService == other.WriteService &&
		p.ReadService == other.ReadService &&
		p.WriteNodePort == other.WriteNodePort &&
		p.ReadNodePort == other.ReadNodePort &&
		p.Namespace == other.Namespace &&
		p.WritePort == other.WritePort &&
		p.ReadPort == other.ReadPort
}

type PrimusLensClientConfigPostgres struct {
	Service   string `yaml:"service" json:"service,omitempty"`
	Port      int32  `yaml:"port" json:"port,omitempty"`
	Namespace string `yaml:"namespace" json:"namespace,omitempty"`
	NodePort  int32  `yaml:"nodePort" json:"node_port,omitempty"`
	Username  string `yaml:"username" json:"username,omitempty"`
	Password  string `yaml:"password" json:"password,omitempty"`
	DBName    string `yaml:"dbName" json:"db_name,omitempty"`
	SSLMode   string `yaml:"sslMode" json:"ssl_mode,omitempty"`
}

func (p PrimusLensClientConfigPostgres) Equals(other PrimusLensClientConfigPostgres) bool {
	return p.Service == other.Service &&
		p.Namespace == other.Namespace &&
		p.Port == other.Port &&
		p.NodePort == other.NodePort &&
		p.Username == other.Username &&
		p.Password == other.Password &&
		p.DBName == other.DBName &&
		p.SSLMode == other.SSLMode
}

func createRestConfig(endpoint, certData, keyData, caData string, insecure bool) (*rest.Config, error) {
	// Decode base64-encoded certificate data
	decodedCertData, err := decodeIfBase64(certData)
	if err != nil {
		return nil, fmt.Errorf("failed to decode cert data: %w", err)
	}

	decodedKeyData, err := decodeIfBase64(keyData)
	if err != nil {
		return nil, fmt.Errorf("failed to decode key data: %w", err)
	}

	decodedCAData, err := decodeIfBase64(caData)
	if err != nil {
		return nil, fmt.Errorf("failed to decode ca data: %w", err)
	}

	log.Infof("Creating rest config for endpoint: %s (insecure: %v, cert len: %d, key len: %d, ca len: %d)",
		endpoint, insecure, len(decodedCertData), len(decodedKeyData), len(decodedCAData))
	log.Infof("Key data starts with: %s", truncateString(decodedKeyData, 100))
	log.Infof("Cert data starts with: %s", truncateString(decodedCertData, 100))

	cfg := &rest.Config{
		Host: endpoint,
		TLSClientConfig: rest.TLSClientConfig{
			Insecure: insecure,
			KeyData:  []byte(decodedKeyData),
			CertData: []byte(decodedCertData),
		},
	}
	if !insecure {
		cfg.TLSClientConfig.CAData = []byte(decodedCAData)
	}
	return cfg, nil
}
