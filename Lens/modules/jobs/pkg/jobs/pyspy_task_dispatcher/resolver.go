package pyspy_task_dispatcher

import (
	"context"
	"fmt"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// DefaultNodeExporterPort is the default port for node-exporter
	DefaultNodeExporterPort = 9100

	// NodeExporterDaemonSetName is the name of the node-exporter DaemonSet
	NodeExporterDaemonSetName = "lens-node-exporter"

	// NodeExporterNamespace is the namespace where node-exporter runs
	NodeExporterNamespace = "primus"
)

// NodeExporterResolver resolves node-exporter addresses
type NodeExporterResolver struct {
	k8sClient *clientsets.K8SClientSet
	port      int
}

// NewNodeExporterResolver creates a new resolver
func NewNodeExporterResolver(k8sClient *clientsets.K8SClientSet) *NodeExporterResolver {
	return &NodeExporterResolver{
		k8sClient: k8sClient,
		port:      DefaultNodeExporterPort,
	}
}

// GetNodeExporterAddress returns the node-exporter address for a given node
func (r *NodeExporterResolver) GetNodeExporterAddress(ctx context.Context, nodeName string) (string, error) {
	if r.k8sClient == nil || r.k8sClient.Clientsets == nil {
		return "", fmt.Errorf("k8s client not available")
	}

	// Get the node to find its IP
	node, err := r.k8sClient.Clientsets.CoreV1().Nodes().Get(ctx, nodeName, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to get node %s: %w", nodeName, err)
	}

	// Find the node's internal IP
	nodeIP := r.getNodeInternalIP(node)
	if nodeIP == "" {
		return "", fmt.Errorf("no internal IP found for node %s", nodeName)
	}

	address := fmt.Sprintf("%s:%d", nodeIP, r.port)
	log.Debugf("Resolved node-exporter address for node %s: %s", nodeName, address)

	return address, nil
}

// getNodeInternalIP extracts the internal IP from node status
func (r *NodeExporterResolver) getNodeInternalIP(node *corev1.Node) string {
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

// GetAllNodeExporterAddresses returns addresses for all nodes running node-exporter
func (r *NodeExporterResolver) GetAllNodeExporterAddresses(ctx context.Context) (map[string]string, error) {
	if r.k8sClient == nil || r.k8sClient.Clientsets == nil {
		return nil, fmt.Errorf("k8s client not available")
	}

	// List all nodes
	nodes, err := r.k8sClient.Clientsets.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list nodes: %w", err)
	}

	addresses := make(map[string]string)
	for _, node := range nodes.Items {
		nodeIP := r.getNodeInternalIP(&node)
		if nodeIP != "" {
			addresses[node.Name] = fmt.Sprintf("%s:%d", nodeIP, r.port)
		}
	}

	return addresses, nil
}

// ValidateNodeExporter checks if node-exporter is accessible on a node
func (r *NodeExporterResolver) ValidateNodeExporter(ctx context.Context, nodeName string) error {
	address, err := r.GetNodeExporterAddress(ctx, nodeName)
	if err != nil {
		return err
	}

	// For now, just check if we can resolve the address
	// In production, you might want to do a health check
	log.Debugf("Node-exporter address validated for node %s: %s", nodeName, address)
	return nil
}

