package clientsets

// Kubelet client implementation with multi-cluster authentication support.
//
// Usage examples:
//
// 1. Using default authentication (current cluster):
//    client, err := GetOrInitKubeletClient(nodeName, address, "")
//
// 2. Using cluster-specific authentication (auto retrieves config from ClusterManager):
//    client, err := GetOrInitKubeletClient(nodeName, address, "cluster-name")
//
// 3. Direct client creation with custom config:
//    config := &ClusterConfig{
//        Host: "https://api.cluster.example.com",
//        CertData: "...",
//        KeyData: "...",
//        CAData: "...",
//    }
//    client, err := NewClient(kubeletAddress, config)

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"fmt"
	"net/http"
	"os"
	"sync"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/go-resty/resty/v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/rest"
	statsapi "k8s.io/kubelet/pkg/apis/stats/v1alpha1"
)

const (
	kubeletStatsApi = "/stats/summary"
	podsApi         = "/pods"
)

// NewClient creates a new kubelet client with cluster-specific authentication
// If config is nil, falls back to the default token-based authentication
func NewClient(kubeletAddress string, config *ClusterConfig) (*Client, error) {
	restyC := resty.New().SetBaseURL(kubeletAddress)

	// If no config provided, use default token-based authentication
	if config == nil {
		path := os.Getenv("KUBELET_TOKEN_FILE")
		if path == "" {
			path = "/var/run/secrets/kubernetes.io/serviceaccount/token"
		}
		tokenBytes, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read token file failed.Path %s:%w", path, err)
		}
		restyC.SetHeader("Authorization", fmt.Sprintf("Bearer %s", string(tokenBytes)))
		restyC.SetTLSClientConfig(&tls.Config{InsecureSkipVerify: true})
	} else {
		// Use cluster-specific authentication from config
		tlsConfig, err := createKubeletTLSConfig(config)
		if err != nil {
			return nil, fmt.Errorf("failed to create TLS config: %w", err)
		}
		restyC.SetTLSClientConfig(tlsConfig)

		// If bearer token is provided in config, use it
		if config.BearerToken != "" {
			restyC.SetHeader("Authorization", fmt.Sprintf("Bearer %s", config.BearerToken))
		}
	}

	return &Client{
		kubeletApi: restyC,
	}, nil
}

// createKubeletTLSConfig creates TLS config from cluster config
func createKubeletTLSConfig(config *ClusterConfig) (*tls.Config, error) {
	tlsConfig := &tls.Config{
		InsecureSkipVerify: config.InsecureSkipTLSVerify,
	}

	// Set TLS server name if provided
	if config.TLSServerName != "" {
		tlsConfig.ServerName = config.TLSServerName
	}

	// Load client certificate and key if provided
	if config.CertData != "" && config.KeyData != "" {
		certPEM, err := decodeBase64IfNeeded(config.CertData)
		if err != nil {
			return nil, fmt.Errorf("failed to decode cert data: %w", err)
		}
		keyPEM, err := decodeBase64IfNeeded(config.KeyData)
		if err != nil {
			return nil, fmt.Errorf("failed to decode key data: %w", err)
		}

		cert, err := tls.X509KeyPair([]byte(certPEM), []byte(keyPEM))
		if err != nil {
			return nil, fmt.Errorf("failed to load client cert/key pair: %w", err)
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	}

	// Load CA certificate if provided and not skipping verification
	if config.CAData != "" && !config.InsecureSkipTLSVerify {
		caPEM, err := decodeBase64IfNeeded(config.CAData)
		if err != nil {
			return nil, fmt.Errorf("failed to decode CA data: %w", err)
		}

		caCertPool := x509.NewCertPool()
		if !caCertPool.AppendCertsFromPEM([]byte(caPEM)) {
			return nil, fmt.Errorf("failed to append CA certificate")
		}
		tlsConfig.RootCAs = caCertPool
	}

	return tlsConfig, nil
}

// decodeBase64IfNeeded attempts to decode base64 string, returns original if not base64 encoded
func decodeBase64IfNeeded(data string) (string, error) {
	if data == "" {
		return "", nil
	}

	// Try to decode as base64
	decoded, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		// If decode fails, assume it's already plain text (e.g., PEM format)
		return data, nil
	}

	return string(decoded), nil
}

// ClusterConfigFromRestConfig creates a ClusterConfig from rest.Config for kubelet client
// This is a helper function to convert K8S rest.Config to ClusterConfig for kubelet authentication
func ClusterConfigFromRestConfig(restConfig *rest.Config) *ClusterConfig {
	if restConfig == nil {
		return nil
	}

	return &ClusterConfig{
		Host:          restConfig.Host,
		BearerToken:   restConfig.BearerToken,
		TLSServerName: restConfig.TLSClientConfig.ServerName,
		// Always skip TLS verification for kubelet API because:
		// 1. Kubelet server certificates typically don't contain IP SANs
		// 2. Connections are made using IP addresses, not hostnames
		// 3. Client certificate authentication is sufficient for security
		InsecureSkipTLSVerify: true,
		CAData:                string(restConfig.TLSClientConfig.CAData),
		CertData:              string(restConfig.TLSClientConfig.CertData),
		KeyData:               string(restConfig.TLSClientConfig.KeyData),
	}
}

type Client struct {
	address    string
	kubeletApi *resty.Client
}

func (s *Client) GetRestyClient() *resty.Client {
	return s.kubeletApi.Clone()
}

func (s *Client) GetKubeletStats(ctx context.Context) *statsapi.Summary {
	resp, err := s.kubeletApi.R().SetResult(&statsapi.Summary{}).Get(kubeletStatsApi)
	if err != nil {
		log.GlobalLogger().WithContext(ctx).WithError(err).Errorln("Failed to get kubelet stats")
		return nil
	}
	if resp.StatusCode() != http.StatusOK {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to get kubelet stats, status code: %d", resp.StatusCode())
		return nil
	}
	return resp.Result().(*statsapi.Summary)
}

func (s *Client) GetKubeletPods(ctx context.Context) (*corev1.PodList, error) {
	resp, err := s.kubeletApi.R().SetResult(&corev1.PodList{}).Get(podsApi)
	if err != nil {
		log.GlobalLogger().WithContext(ctx).WithError(err).Errorln("Failed to get kubelet pods")
		return nil, err
	}
	if resp.StatusCode() != http.StatusOK {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to get kubelet pods, status code: %d.Resp %s", resp.StatusCode(), resp.String())
		return nil, err
	}
	return resp.Result().(*corev1.PodList), nil
}

func (s *Client) GetKubeletPodMap(ctx context.Context) (map[string]corev1.Pod, error) {
	podList, err := s.GetKubeletPods(ctx)
	if err != nil {
		return nil, err
	}
	result := map[string]corev1.Pod{}
	for i := range podList.Items {
		pod := podList.Items[i]
		result[string(pod.UID)] = pod
	}
	return result, nil
}

// kubeletClientKey is used to uniquely identify a kubelet client
type kubeletClientKey struct {
	clusterName string
	nodeName    string
}

var kubeletClients = map[kubeletClientKey]*Client{}

var kubeletLock sync.Mutex

// GetOrInitKubeletClient creates or retrieves a cached kubelet client
// It retrieves cluster config from ClusterManager based on clusterName
// If clusterName is empty or not found, falls back to default token-based authentication
func GetOrInitKubeletClient(nodeName, address, clusterName string) (*Client, error) {
	kubeletLock.Lock()
	defer kubeletLock.Unlock()

	// Create cache key
	cacheKey := kubeletClientKey{
		clusterName: clusterName,
		nodeName:    nodeName,
	}

	// Check if client already exists
	existing, ok := kubeletClients[cacheKey]
	if ok {
		if existing.address == address {
			return existing, nil
		}
		log.GlobalLogger().WithField("node", nodeName).WithField("cluster", clusterName).
			Warningf("Kubelet address changed: old=%s, new=%s. Reinitializing client.", existing.address, address)
	}

	// Get cluster config from ClusterManager
	var config *ClusterConfig
	if clusterName != "" {
		if globalClusterManager != nil {
			clientSet, err := globalClusterManager.GetClientSetByClusterName(clusterName)
			if err != nil {
				log.GlobalLogger().WithField("cluster", clusterName).
					Warningf("Failed to get cluster config from ClusterManager: %v, using default authentication", err)
			} else if clientSet.K8SClientSet != nil && clientSet.K8SClientSet.Config != nil {
				config = ClusterConfigFromRestConfig(clientSet.K8SClientSet.Config)
				log.GlobalLogger().WithField("cluster", clusterName).WithField("node", nodeName).
					Infof("Using cluster-specific config for kubelet client")
			}
		} else {
			log.GlobalLogger().Warning("ClusterManager not initialized, using default authentication")
		}
	}

	// Create new client
	newClient, err := NewClient(address, config)
	if err != nil {
		return nil, fmt.Errorf("failed to create kubelet client for node %s in cluster %s: %w", nodeName, clusterName, err)
	}

	newClient.address = address

	// Cache the client
	kubeletClients[cacheKey] = newClient
	return newClient, nil
}
