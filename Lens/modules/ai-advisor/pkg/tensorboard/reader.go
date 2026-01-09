// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package tensorboard

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/node-exporter/pkg/client"
	"github.com/AMD-AGI/Primus-SaFE/Lens/node-exporter/pkg/types"
)

// Reader provides non-intrusive access to TensorBoard logs from training containers
type Reader struct {
	// Database facades for node and pod info
	nodeFacade database.NodeFacadeInterface
	podFacade  database.PodFacadeInterface

	// Client cache for node-specific clients (thread-safe)
	clientCache sync.Map // nodeName -> *client.Client
}

// NewReader creates a new TensorBoard reader
func NewReader() *Reader {
	return &Reader{
		nodeFacade: database.NewNodeFacade(),
		podFacade:  database.NewPodFacade(),
		// clientCache is sync.Map, no initialization needed
	}
}

// LogReadRequest represents a request to read TensorBoard logs
type LogReadRequest struct {
	WorkloadUID  string   `json:"workload_uid" binding:"required"`
	PodUID       string   `json:"pod_uid" binding:"required"`
	LogDir       string   `json:"log_dir,omitempty"`              // For reference only, not used for scanning
	EventFiles   []string `json:"event_files" binding:"required"` // Pre-discovered event files from FindTensorboardFiles
	IncludeFiles bool     `json:"include_files,omitempty"`        // Whether to include file list in response
}

// LogReadResponse represents TensorBoard log information
type LogReadResponse struct {
	WorkloadUID  string                     `json:"workload_uid"`
	LogDir       string                     `json:"log_dir"`
	EventFiles   []*types.ContainerFileInfo `json:"event_files,omitempty"`
	TotalSize    int64                      `json:"total_size"`
	LatestUpdate time.Time                  `json:"latest_update"`
	FileCount    int                        `json:"file_count"`
	AccessMethod string                     `json:"access_method"` // "proc_fs"
}

// EventReadRequest represents a request to read a specific event file
type EventReadRequest struct {
	WorkloadUID string `json:"workload_uid" binding:"required"`
	PodUID      string `json:"pod_uid" binding:"required"`
	EventFile   string `json:"event_file" binding:"required"`
	Offset      int64  `json:"offset,omitempty"`
	Length      int64  `json:"length,omitempty"` // 0 = all (limited by server)
}

// EventReadResponse represents event file content
type EventReadResponse struct {
	EventFile string                   `json:"event_file"`
	Content   string                   `json:"content"`
	FileInfo  *types.ContainerFileInfo `json:"file_info"`
	BytesRead int64                    `json:"bytes_read"`
	EOF       bool                     `json:"eof"`
	IsBinary  bool                     `json:"is_binary"`
}

// ReadLogs retrieves TensorBoard log files information
// EventFiles must be provided (from FindTensorboardFiles)
func (r *Reader) ReadLogs(ctx context.Context, req *LogReadRequest) (*LogReadResponse, error) {
	log.Infof("Reading TensorBoard logs for workload %s, files: %d",
		req.WorkloadUID, len(req.EventFiles))

	if len(req.EventFiles) == 0 {
		return nil, fmt.Errorf("EventFiles must be provided (from FindTensorboardFiles)")
	}

	var eventFiles []*types.ContainerFileInfo
	var totalSize int64
	var latestUpdate time.Time

	// Get file metadata to calculate total size and latest update
	for _, filePath := range req.EventFiles {
		fileInfo, err := r.GetFileInfo(ctx, req.PodUID, filePath)
		if err != nil {
			log.Warnf("Failed to get file info for %s: %v", filePath, err)
			continue
		}

		// Always add to eventFiles for calculation
		eventFiles = append(eventFiles, fileInfo)
		totalSize += fileInfo.Size
		if fileInfo.ModTime.After(latestUpdate) {
			latestUpdate = fileInfo.ModTime
		}
	}

	fileCount := len(req.EventFiles)
	if len(req.EventFiles) == 0 {
		fileCount = len(eventFiles)
	}

	response := &LogReadResponse{
		WorkloadUID:  req.WorkloadUID,
		LogDir:       req.LogDir,
		TotalSize:    totalSize,
		LatestUpdate: latestUpdate,
		FileCount:    fileCount,
		AccessMethod: "proc_fs",
	}

	if req.IncludeFiles {
		response.EventFiles = eventFiles
	}

	log.Infof("TensorBoard logs summary: %d files, total size: %d bytes",
		fileCount, totalSize)

	return response, nil
}

// ReadEvent reads a specific TensorBoard event file
func (r *Reader) ReadEvent(ctx context.Context, req *EventReadRequest) (*EventReadResponse, error) {
	log.Infof("Reading TensorBoard event file: workload=%s, file=%s, offset=%d, length=%d",
		req.WorkloadUID, req.EventFile, req.Offset, req.Length)

	// Get node client and pod information
	nodeClient, podInfo, err := r.getNodeClientAndPodInfo(ctx, req.PodUID)
	if err != nil {
		return nil, fmt.Errorf("failed to get node client: %w", err)
	}

	// Read event file via node-exporter using pod parameters
	fileResp, err := nodeClient.ReadTensorBoardEvent(ctx, podInfo.UID, podInfo.Name, podInfo.Namespace, podInfo.ContainerName, req.EventFile, req.Offset, req.Length)
	if err != nil {
		return nil, fmt.Errorf("failed to read event file: %w", err)
	}

	response := &EventReadResponse{
		EventFile: req.EventFile,
		Content:   fileResp.Content,
		FileInfo:  fileResp.FileInfo,
		BytesRead: fileResp.BytesRead,
		EOF:       fileResp.EOF,
		IsBinary:  fileResp.IsBinary,
	}

	log.Debugf("Read %d bytes from event file %s", response.BytesRead, req.EventFile)

	return response, nil
}

// ListEventFiles lists all TensorBoard event files in a directory
// Deprecated: Use FindTensorboardFiles instead to avoid scanning extra files
func (r *Reader) ListEventFiles(ctx context.Context, req *LogReadRequest) ([]*types.ContainerFileInfo, error) {
	log.Infof("Listing TensorBoard event files: workload=%s, log_dir=%s",
		req.WorkloadUID, req.LogDir)

	// Get node client and pod information
	nodeClient, podInfo, err := r.getNodeClientAndPodInfo(ctx, req.PodUID)
	if err != nil {
		return nil, fmt.Errorf("failed to get node client: %w", err)
	}

	// List directory via node-exporter using pod parameters
	listReq := &types.ContainerDirectoryListRequest{
		PodUID:        podInfo.UID,
		PodName:       podInfo.Name,
		PodNamespace:  podInfo.Namespace,
		ContainerName: podInfo.ContainerName,
		Path:          req.LogDir,
		Recursive:     true,
		Pattern:       "events.out.tfevents.*",
	}

	listResp, err := nodeClient.ListContainerDirectory(ctx, listReq)
	if err != nil {
		return nil, fmt.Errorf("failed to list event files: %w", err)
	}

	log.Infof("Found %d event files in %s", listResp.Total, req.LogDir)
	return listResp.Files, nil
}

// ReadFile reads any file from container (with security restrictions on node-exporter side)
func (r *Reader) ReadFile(ctx context.Context, podUID, filePath string, offset, length int64) (*types.ContainerFileReadResponse, error) {
	log.Infof("Reading container file: pod=%s, path=%s, offset=%d, length=%d",
		podUID, filePath, offset, length)

	// Get node client and pod information
	nodeClient, podInfo, err := r.getNodeClientAndPodInfo(ctx, podUID)
	if err != nil {
		return nil, fmt.Errorf("failed to get node client: %w", err)
	}

	// Read file via node-exporter using pod parameters
	readReq := &types.ContainerFileReadRequest{
		PodUID:        podInfo.UID,
		PodName:       podInfo.Name,
		PodNamespace:  podInfo.Namespace,
		ContainerName: podInfo.ContainerName,
		Path:          filePath,
		Offset:        offset,
		Length:        length,
	}

	response, err := nodeClient.ReadContainerFile(ctx, readReq)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	log.Debugf("Read %d bytes from %s", response.BytesRead, filePath)
	return response, nil
}

// GetFileInfo gets metadata for a file in container
func (r *Reader) GetFileInfo(ctx context.Context, podUID, filePath string) (*types.ContainerFileInfo, error) {
	log.Debugf("Getting file info: pod=%s, path=%s", podUID, filePath)

	nodeClient, podInfo, err := r.getNodeClientAndPodInfo(ctx, podUID)
	if err != nil {
		return nil, fmt.Errorf("failed to get node client: %w", err)
	}

	fileInfo, err := nodeClient.GetContainerFileInfo(ctx, podInfo.UID, podInfo.Name, podInfo.Namespace, podInfo.ContainerName, filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to get file info: %w", err)
	}

	return fileInfo, nil
}

// Helper methods

// PodInfo contains pod information needed for file access
type PodInfo struct {
	UID           string
	Name          string
	Namespace     string
	ContainerName string
}

// getNodeClientAndPodInfo gets the node-exporter client and pod information
func (r *Reader) getNodeClientAndPodInfo(ctx context.Context, podUID string) (*client.Client, *PodInfo, error) {
	// Get node information from database
	gpuPod, err := r.podFacade.GetGpuPodsByPodUid(ctx, podUID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to query pod: %w", err)
	}

	if gpuPod == nil {
		return nil, nil, fmt.Errorf("pod with UID %s not found", podUID)
	}

	nodeName := gpuPod.NodeName
	if nodeName == "" {
		return nil, nil, fmt.Errorf("node name is empty for pod %s", podUID)
	}

	// Get node IP
	node, err := r.nodeFacade.GetNodeByName(ctx, nodeName)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to query node: %w", err)
	}

	if node == nil {
		return nil, nil, fmt.Errorf("node %s not found", nodeName)
	}

	nodeIP := node.Address
	if nodeIP == "" {
		nodeIP = nodeName // Fallback to node name as hostname
	}

	// Get or create client for this node using K8s clientsets
	nodeClient, err := r.getOrCreateClient(nodeIP, nodeName)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get node-exporter client: %w", err)
	}

	// Get process tree to determine the main container
	processTreeReq := &types.ProcessTreeRequest{
		PodName:        gpuPod.Name,
		PodNamespace:   gpuPod.Namespace,
		PodUID:         podUID,
		IncludeCmdline: false,
		IncludeEnv:     false,
	}

	processTree, err := nodeClient.GetPodProcessTree(ctx, processTreeReq)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get process tree: %w", err)
	}

	if len(processTree.Containers) == 0 {
		return nil, nil, fmt.Errorf("no containers found in pod")
	}

	// Use the first container
	firstContainer := processTree.Containers[0]
	containerName := firstContainer.ContainerName

	podInfo := &PodInfo{
		UID:           podUID,
		Name:          gpuPod.Name,
		Namespace:     gpuPod.Namespace,
		ContainerName: containerName,
	}

	log.Debugf("Using container %s to access pod %s/%s filesystem",
		containerName, gpuPod.Namespace, gpuPod.Name)

	return nodeClient, podInfo, nil
}

// getNodeClientAndPID is deprecated, use getNodeClientAndPodInfo instead
// Kept for backward compatibility
func (r *Reader) getNodeClientAndPID(ctx context.Context, podUID string) (*client.Client, int, error) {
	nodeClient, podInfo, err := r.getNodeClientAndPodInfo(ctx, podUID)
	if err != nil {
		return nil, 0, err
	}

	// Get process tree to find PID (for backward compatibility)
	processTreeReq := &types.ProcessTreeRequest{
		PodName:        podInfo.Name,
		PodNamespace:   podInfo.Namespace,
		PodUID:         podUID,
		IncludeCmdline: false,
		IncludeEnv:     false,
	}

	processTree, err := nodeClient.GetPodProcessTree(ctx, processTreeReq)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get process tree: %w", err)
	}

	if len(processTree.Containers) == 0 || processTree.Containers[0].RootProcess == nil {
		return nil, 0, fmt.Errorf("no process found in pod")
	}

	pid := processTree.Containers[0].RootProcess.HostPID
	return nodeClient, pid, nil
}

// getOrCreateClient gets or creates a node-exporter client for a node using K8s clientsets
func (r *Reader) getOrCreateClient(nodeIP, nodeName string) (*client.Client, error) {
	cacheKey := nodeName

	// Check cache first
	if cached, ok := r.clientCache.Load(cacheKey); ok {
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
	nodeClient := client.NewClient(client.DefaultConfig(baseURL))

	// Cache the client
	r.clientCache.Store(cacheKey, nodeClient)

	log.Infof("Created node-exporter client for node %s at %s", nodeName, baseURL)
	return nodeClient, nil
}

// ParseLogDirFromMetadata extracts log directory from various sources
func ParseLogDirFromMetadata(metadata map[string]interface{}) string {
	// Try common keys
	keys := []string{
		"log_dir",
		"logdir",
		"tensorboard_log_dir",
		"tb_log_dir",
	}

	for _, key := range keys {
		if val, ok := metadata[key].(string); ok && val != "" {
			return val
		}
	}

	return ""
}

// NormalizeLogPath ensures the log path is absolute and clean
func NormalizeLogPath(logPath string) string {
	if logPath == "" {
		return ""
	}

	// Clean the path
	cleanPath := filepath.Clean(logPath)

	// Ensure absolute path
	if !filepath.IsAbs(cleanPath) {
		cleanPath = "/" + cleanPath
	}

	return cleanPath
}
