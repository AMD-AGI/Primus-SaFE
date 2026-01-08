// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package metadata

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/node-exporter/pkg/client"
	"github.com/AMD-AGI/Primus-SaFE/Lens/node-exporter/pkg/types"
)

var (
	collectorInstance *Collector
	once              sync.Once
)

// Collector collects and stores workload metadata
type Collector struct {
	storage  Storage
	cache    sync.Map // workloadUID -> *CollectionResult
	cacheTTL time.Duration

	// Node-specific client cache
	clientCache sync.Map // nodeName -> *client.Client

	// Node Exporter configuration
	nodeExporterPort int

	// Database facades for node and pod info
	nodeFacade      database.NodeFacadeInterface
	podFacade       database.PodFacadeInterface
	detectionFacade database.AiWorkloadMetadataFacadeInterface
}

// Storage defines the interface for metadata storage
type Storage interface {
	Store(ctx context.Context, metadata *WorkloadMetadata) error
	Get(ctx context.Context, workloadUID string) (*WorkloadMetadata, error)
	Query(ctx context.Context, query *MetadataQuery) ([]*WorkloadMetadata, error)
	Delete(ctx context.Context, workloadUID string) error
}

// InitCollector initializes the metadata collector
func InitCollector(ctx context.Context, storage Storage) error {
	var initErr error
	once.Do(func() {
		collectorInstance = &Collector{
			storage:          storage,
			cacheTTL:         10 * time.Minute,
			nodeExporterPort: 8989, // Default node-exporter port
			nodeFacade:       database.NewNodeFacade(),
			podFacade:        database.NewPodFacade(),
			detectionFacade:  database.NewAiWorkloadMetadataFacade(),
		}
		log.Info("Metadata collector initialized")
	})
	return initErr
}

// GetCollector returns the global collector instance
func GetCollector() *Collector {
	return collectorInstance
}

// CollectMetadata collects complete metadata for a training workload
func (c *Collector) CollectMetadata(ctx context.Context, req *CollectionRequest) (*CollectionResult, error) {
	startTime := time.Now()

	log.Infof("Starting metadata collection for workload %s (pod %s/%s)",
		req.WorkloadUID, req.PodNamespace, req.PodName)

	// Check cache unless forced
	if !req.Force {
		if cached, ok := c.cache.Load(req.WorkloadUID); ok {
			result := cached.(*CollectionResult)
			if time.Since(time.Now().Add(-time.Duration(result.Duration)*time.Second)) < c.cacheTTL {
				log.Debugf("Using cached metadata for workload %s", req.WorkloadUID)
				return result, nil
			}
		}
	}

	result := &CollectionResult{
		Success: false,
	}

	// Get node name and IP from database
	nodeName, nodeIP, err := c.getNodeInfoFromDB(ctx, req.PodUID)
	if err != nil {
		result.Error = fmt.Sprintf("failed to get node info from database: %v", err)
		result.Duration = time.Since(startTime).Seconds()
		return result, err
	}

	log.Infof("Pod %s/%s is on node %s (IP: %s)", req.PodNamespace, req.PodName, nodeName, nodeIP)

	// Get node-specific client
	client, err := c.getNodeExporterClientByIP(nodeIP, nodeName)
	if err != nil {
		result.Error = fmt.Sprintf("failed to get node-exporter client: %v", err)
		result.Duration = time.Since(startTime).Seconds()
		return result, err
	}

	// Step 1: Get process tree
	log.Debugf("Getting process tree for pod %s on node %s", req.PodUID, req.NodeName)
	processTree, err := client.GetPodProcessTree(ctx, &types.ProcessTreeRequest{
		PodName:          req.PodName,
		PodNamespace:     req.PodNamespace,
		PodUID:           req.PodUID,
		IncludeCmdline:   true,
		IncludeEnv:       false, // We'll get env from inspection if needed
		IncludeResources: true,
	})

	if err != nil {
		result.Error = fmt.Sprintf("failed to get process tree: %v", err)
		result.Duration = time.Since(startTime).Seconds()
		return result, err
	}

	result.ProcessCount = processTree.TotalProcesses
	result.PythonCount = processTree.TotalPython

	if processTree.TotalPython == 0 {
		result.Error = "no Python processes found in pod"
		result.Duration = time.Since(startTime).Seconds()
		return result, fmt.Errorf("%s", result.Error)
	}

	log.Infof("Found %d Python processes in pod %s", processTree.TotalPython, req.PodName)

	// Step 2: Find root Python processes from process tree
	rootPythonProcesses := c.findRootPythonProcesses(processTree)
	if len(rootPythonProcesses) == 0 {
		result.Error = "no root Python processes found in pod"
		result.Duration = time.Since(startTime).Seconds()
		return result, fmt.Errorf("%s", result.Error)
	}

	log.Infof("Found %d root Python processes in pod %s", len(rootPythonProcesses), req.PodName)

	metadata := &WorkloadMetadata{
		WorkloadUID:      req.WorkloadUID,
		PodName:          req.PodName,
		PodNamespace:     req.PodNamespace,
		NodeName:         nodeName,
		Frameworks:       []string{},
		CollectedAt:      time.Now(),
		CollectionSource: "node-exporter",
		Confidence:       0.0,
	}

	// Detect Primus by checking cmdline patterns from root processes
	if primusData := c.detectPrimusFromProcesses(rootPythonProcesses); primusData != nil {
		metadata.PrimusInfo = primusData
		if !contains(metadata.Frameworks, "primus") {
			metadata.Frameworks = append(metadata.Frameworks, "primus")
		}
		if metadata.WrapperFramework == "" {
			metadata.WrapperFramework = "primus"
		}
	}

	// Store metadata
	if err := c.storage.Store(ctx, metadata); err != nil {
		log.Errorf("Failed to store metadata: %v", err)
		result.Error = fmt.Sprintf("failed to store metadata: %v", err)
	} else {
		result.Success = true
		result.Metadata = metadata
	}

	result.InspectedCount = 0
	result.Duration = time.Since(startTime).Seconds()

	// Cache result
	c.cache.Store(req.WorkloadUID, result)

	log.Infof("Metadata collection completed for workload %s: success=%v, frameworks=%v, duration=%.2fs",
		req.WorkloadUID, result.Success, metadata.Frameworks, result.Duration)

	return result, nil
}

// detectPrimusFromProcesses detects Primus wrapper by analyzing process cmdlines
func (c *Collector) detectPrimusFromProcesses(processes []*types.ProcessInfo) *PrimusMetadata {
	for _, proc := range processes {
		cmdline := strings.ToLower(proc.Cmdline)

		// Look for Primus indicators in command line
		if strings.Contains(cmdline, "primus") ||
			strings.Contains(cmdline, "primus-train") ||
			strings.Contains(cmdline, "primus.") {

			primus := &PrimusMetadata{
				Mode:     "training",
				Features: []string{},
			}

			// Try to detect version and configuration from cmdline
			if strings.Contains(cmdline, "--") {
				// Parse command line flags
				parts := strings.Split(cmdline, "--")
				for _, part := range parts[1:] {
					trimmed := strings.TrimSpace(part)
					if strings.HasPrefix(trimmed, "version") {
						// Extract version
						fields := strings.Fields(trimmed)
						if len(fields) > 1 {
							primus.Version = fields[1]
						}
					}
				}
			}

			log.Infof("Detected Primus wrapper framework in process %d", proc.HostPID)
			return primus
		}
	}

	return nil
}

// GetMetadata retrieves stored metadata for a workload
func (c *Collector) GetMetadata(ctx context.Context, workloadUID string) (*WorkloadMetadata, error) {
	return c.storage.Get(ctx, workloadUID)
}

// QueryMetadata queries stored metadata with filters
func (c *Collector) QueryMetadata(ctx context.Context, query *MetadataQuery) ([]*WorkloadMetadata, error) {
	return c.storage.Query(ctx, query)
}

// InvalidateCache invalidates the cache for a workload
func (c *Collector) InvalidateCache(workloadUID string) {
	c.cache.Delete(workloadUID)
}

// findRootPythonProcesses finds the root (top-level) Python processes from process tree
// These are typically the main training processes
func (c *Collector) findRootPythonProcesses(processTree *types.PodProcessTree) []*types.ProcessInfo {
	var rootProcesses []*types.ProcessInfo

	// Iterate through all containers in the pod
	for _, container := range processTree.Containers {
		// Find Python processes with minimal depth (closest to root)
		pythonProcesses := c.findPythonProcessesInTree(container.RootProcess)

		if len(pythonProcesses) > 0 {
			// Find the Python process with the smallest depth (closest to root)
			minDepth := -1
			var rootPython []*types.ProcessInfo

			for _, proc := range pythonProcesses {
				depth := c.calculateProcessDepth(proc, container.RootProcess)
				if minDepth == -1 || depth < minDepth {
					minDepth = depth
					rootPython = []*types.ProcessInfo{proc}
				} else if depth == minDepth {
					rootPython = append(rootPython, proc)
				}
			}

			rootProcesses = append(rootProcesses, rootPython...)
		}
	}

	if len(rootProcesses) > 0 {
		log.Infof("Found %d root Python processes", len(rootProcesses))
		for _, proc := range rootProcesses {
			log.Debugf("Root Python process: PID=%d, cmdline=%s", proc.HostPID, proc.Cmdline)
		}
	}

	return rootProcesses
}

// findPythonProcessesInTree recursively finds all Python processes in the tree
func (c *Collector) findPythonProcessesInTree(root *types.ProcessInfo) []*types.ProcessInfo {
	var pythonProcesses []*types.ProcessInfo

	if root == nil {
		return pythonProcesses
	}

	// Check if current process is Python
	if root.IsPython {
		pythonProcesses = append(pythonProcesses, root)
	}

	// Recursively check children
	for _, child := range root.Children {
		pythonProcesses = append(pythonProcesses, c.findPythonProcessesInTree(child)...)
	}

	return pythonProcesses
}

// calculateProcessDepth calculates the depth of a process in the tree
func (c *Collector) calculateProcessDepth(target *types.ProcessInfo, root *types.ProcessInfo) int {
	if root == nil {
		return -1
	}

	if root.HostPID == target.HostPID {
		return 0
	}

	for _, child := range root.Children {
		depth := c.calculateProcessDepth(target, child)
		if depth >= 0 {
			return depth + 1
		}
	}

	return -1
}

// contains checks if a string slice contains a string
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// getNodeInfoFromDB retrieves node name and IP from database based on pod UID
func (c *Collector) getNodeInfoFromDB(ctx context.Context, podUID string) (nodeName, nodeIP string, err error) {
	// Query gpu_pods table to get node name
	gpuPod, err := c.podFacade.GetGpuPodsByPodUid(ctx, podUID)
	if err != nil {
		return "", "", fmt.Errorf("failed to query gpu_pods: %w", err)
	}

	if gpuPod == nil {
		return "", "", fmt.Errorf("pod with UID %s not found in database", podUID)
	}

	nodeName = gpuPod.NodeName
	if nodeName == "" {
		return "", "", fmt.Errorf("node name is empty for pod %s", podUID)
	}

	log.Debugf("Found pod %s on node %s from database", podUID, nodeName)

	// Query node table to get node IP
	node, err := c.nodeFacade.GetNodeByName(ctx, nodeName)
	if err != nil {
		return "", "", fmt.Errorf("failed to query node table: %w", err)
	}

	if node == nil {
		return "", "", fmt.Errorf("node %s not found in database", nodeName)
	}

	nodeIP = node.Address
	if nodeIP == "" {
		// Fallback: use nodeName as hostname
		log.Warnf("Node IP is empty for node %s, using node name as hostname", nodeName)
		nodeIP = nodeName
	}

	log.Debugf("Found node %s IP: %s from database", nodeName, nodeIP)
	return nodeName, nodeIP, nil
}

// getNodeExporterClientByIP gets or creates a node-exporter client by finding the pod on the target node
func (c *Collector) getNodeExporterClientByIP(nodeIP, nodeName string) (*client.Client, error) {
	// Use nodeName as cache key
	cacheKey := nodeName

	// Check cache first
	if cached, ok := c.clientCache.Load(cacheKey); ok {
		return cached.(*client.Client), nil
	}

	// Get node-exporter pod on the target node using existing clientsets implementation
	ctx := context.Background()
	k8sClient := clientsets.GetClusterManager().GetCurrentClusterClients().K8SClientSet.ControllerRuntimeClient

	nodeExporterK8sClient, err := clientsets.GetOrInitNodeExportersClient(ctx, nodeName, k8sClient)
	if err != nil {
		return nil, fmt.Errorf("failed to get node-exporter client for node %s: %w", nodeName, err)
	}

	// Convert to our client type by creating a new client with the baseURL
	// The address from GetOrInitNodeExportersClient already includes http:// and port
	// Client request paths include /v1 prefix, so baseURL should not include it
	baseURL := nodeExporterK8sClient.GetRestyClient().BaseURL
	nodeExporterClient := client.NewClient(client.DefaultConfig(baseURL))

	// Cache the client
	c.clientCache.Store(cacheKey, nodeExporterClient)

	log.Infof("Created node-exporter client for node %s at %s", nodeName, baseURL)
	return nodeExporterClient, nil
}

// GetNodeExporterClientForPod gets a node-exporter client for a specific node (public method)
func (c *Collector) GetNodeExporterClientForPod(ctx context.Context, nodeName string) (*client.Client, error) {
	// Query node info from database
	node, err := c.nodeFacade.GetNodeByName(ctx, nodeName)
	if err != nil {
		return nil, fmt.Errorf("failed to query node %s: %w", nodeName, err)
	}

	if node == nil {
		return nil, fmt.Errorf("node %s not found", nodeName)
	}

	nodeIP := node.Address
	if nodeIP == "" {
		nodeIP = nodeName // Fallback to nodeName as hostname
	}

	return c.getNodeExporterClientByIP(nodeIP, nodeName)
}
