package pyspy

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// DefaultNodeExporterPort is the default port for node-exporter
	DefaultNodeExporterPort = 9100
)

// NodeExporterClient is an HTTP client for calling node-exporter APIs
type NodeExporterClient struct {
	httpClient *http.Client
	port       int
}

// NewNodeExporterClient creates a new NodeExporterClient
func NewNodeExporterClient() *NodeExporterClient {
	return &NodeExporterClient{
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
		port: DefaultNodeExporterPort,
	}
}

// GetNodeExporterAddress returns the node-exporter address for a given node
func (c *NodeExporterClient) GetNodeExporterAddress(ctx context.Context, nodeName string) (string, error) {
	cm := clientsets.GetClusterManager()
	clients := cm.GetCurrentClusterClients()
	if clients == nil || clients.K8SClientSet == nil || clients.K8SClientSet.Clientsets == nil {
		return "", fmt.Errorf("k8s client not available")
	}

	// Get the node to find its IP
	node, err := clients.K8SClientSet.Clientsets.CoreV1().Nodes().Get(ctx, nodeName, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to get node %s: %w", nodeName, err)
	}

	// Find the node's internal IP
	nodeIP := c.getNodeInternalIP(node)
	if nodeIP == "" {
		return "", fmt.Errorf("no internal IP found for node %s", nodeName)
	}

	return fmt.Sprintf("%s:%d", nodeIP, c.port), nil
}

// getNodeInternalIP extracts the internal IP from node status
func (c *NodeExporterClient) getNodeInternalIP(node *corev1.Node) string {
	for _, addr := range node.Status.Addresses {
		if addr.Type == corev1.NodeInternalIP {
			return addr.Address
		}
	}
	// Fallback to external IP if internal is not available
	for _, addr := range node.Status.Addresses {
		if addr.Type == corev1.NodeExternalIP {
			return addr.Address
		}
	}
	return ""
}

// ProxyFileDownload proxies a file download request to node-exporter
func (c *NodeExporterClient) ProxyFileDownload(ctx context.Context, nodeExporterAddr, taskID, filename string) (*http.Response, error) {
	url := fmt.Sprintf("http://%s/api/v1/pyspy/file/%s/%s", nodeExporterAddr, taskID, filename)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	log.Debugf("Proxying file download from %s", url)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to proxy file download: %w", err)
	}

	return resp, nil
}

// ProxyFileList proxies a file list request to node-exporter
func (c *NodeExporterClient) ProxyFileList(ctx context.Context, nodeExporterAddr, taskID string) (io.ReadCloser, error) {
	url := fmt.Sprintf("http://%s/api/v1/pyspy/file/%s", nodeExporterAddr, taskID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to proxy file list: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("node-exporter returned status %d", resp.StatusCode)
	}

	return resp.Body, nil
}

