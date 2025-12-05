package tensorboard

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

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

	// Node Exporter configuration
	nodeExporterPort int

	// Client cache for node-specific clients
	clientCache map[string]*client.Client
}

// NewReader creates a new TensorBoard reader
func NewReader() *Reader {
	return &Reader{
		nodeFacade:       database.NewNodeFacade(),
		podFacade:        database.NewPodFacade(),
		nodeExporterPort: 8989,
		clientCache:      make(map[string]*client.Client),
	}
}

// LogReadRequest represents a request to read TensorBoard logs
type LogReadRequest struct {
	WorkloadUID  string `json:"workload_uid" binding:"required"`
	PodUID       string `json:"pod_uid" binding:"required"`
	LogDir       string `json:"log_dir" binding:"required"`
	IncludeFiles bool   `json:"include_files,omitempty"` // Whether to include file list
}

// LogReadResponse represents TensorBoard log information
type LogReadResponse struct {
	WorkloadUID  string                  `json:"workload_uid"`
	LogDir       string                  `json:"log_dir"`
	EventFiles   []*types.ContainerFileInfo `json:"event_files,omitempty"`
	TotalSize    int64                   `json:"total_size"`
	LatestUpdate time.Time               `json:"latest_update"`
	FileCount    int                     `json:"file_count"`
	AccessMethod string                  `json:"access_method"` // "proc_fs"
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
	EventFile   string                  `json:"event_file"`
	Content     string                  `json:"content"`
	FileInfo    *types.ContainerFileInfo `json:"file_info"`
	BytesRead   int64                   `json:"bytes_read"`
	EOF         bool                    `json:"eof"`
	IsBinary    bool                    `json:"is_binary"`
}

// ReadLogs retrieves TensorBoard log files information
func (r *Reader) ReadLogs(ctx context.Context, req *LogReadRequest) (*LogReadResponse, error) {
	log.Infof("Reading TensorBoard logs for workload %s, log_dir=%s", req.WorkloadUID, req.LogDir)

	// Get node and PID information
	nodeClient, pid, err := r.getNodeClientAndPID(ctx, req.PodUID)
	if err != nil {
		return nil, fmt.Errorf("failed to get node client: %w", err)
	}

	// Get TensorBoard logs via node-exporter
	logInfo, err := nodeClient.GetTensorBoardLogs(ctx, pid, req.LogDir)
	if err != nil {
		return nil, fmt.Errorf("failed to get TensorBoard logs: %w", err)
	}

	response := &LogReadResponse{
		WorkloadUID:  req.WorkloadUID,
		LogDir:       req.LogDir,
		TotalSize:    logInfo.TotalSize,
		LatestUpdate: logInfo.LatestUpdate,
		FileCount:    len(logInfo.EventFiles),
		AccessMethod: "proc_fs",
	}

	if req.IncludeFiles {
		response.EventFiles = logInfo.EventFiles
	}

	log.Infof("Found %d TensorBoard event files, total size: %d bytes",
		response.FileCount, response.TotalSize)

	return response, nil
}

// ReadEvent reads a specific TensorBoard event file
func (r *Reader) ReadEvent(ctx context.Context, req *EventReadRequest) (*EventReadResponse, error) {
	log.Infof("Reading TensorBoard event file: workload=%s, file=%s, offset=%d, length=%d",
		req.WorkloadUID, req.EventFile, req.Offset, req.Length)

	// Get node and PID information
	nodeClient, pid, err := r.getNodeClientAndPID(ctx, req.PodUID)
	if err != nil {
		return nil, fmt.Errorf("failed to get node client: %w", err)
	}

	// Read event file via node-exporter
	fileResp, err := nodeClient.ReadTensorBoardEvent(ctx, pid, req.EventFile, req.Offset, req.Length)
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
func (r *Reader) ListEventFiles(ctx context.Context, req *LogReadRequest) ([]*types.ContainerFileInfo, error) {
	log.Infof("Listing TensorBoard event files: workload=%s, log_dir=%s",
		req.WorkloadUID, req.LogDir)

	// Get node and PID information
	nodeClient, pid, err := r.getNodeClientAndPID(ctx, req.PodUID)
	if err != nil {
		return nil, fmt.Errorf("failed to get node client: %w", err)
	}

	// List directory via node-exporter
	listReq := &types.ContainerDirectoryListRequest{
		PID:       pid,
		Path:      req.LogDir,
		Recursive: true,
		Pattern:   "events.out.tfevents.*",
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

	// Get node and PID information
	nodeClient, pid, err := r.getNodeClientAndPID(ctx, podUID)
	if err != nil {
		return nil, fmt.Errorf("failed to get node client: %w", err)
	}

	// Read file via node-exporter
	readReq := &types.ContainerFileReadRequest{
		PID:    pid,
		Path:   filePath,
		Offset: offset,
		Length: length,
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

	nodeClient, pid, err := r.getNodeClientAndPID(ctx, podUID)
	if err != nil {
		return nil, fmt.Errorf("failed to get node client: %w", err)
	}

	fileInfo, err := nodeClient.GetContainerFileInfo(ctx, pid, filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to get file info: %w", err)
	}

	return fileInfo, nil
}

// Helper methods

// getNodeClientAndPID gets the node-exporter client and a representative PID for the pod
func (r *Reader) getNodeClientAndPID(ctx context.Context, podUID string) (*client.Client, int, error) {
	// Get node information from database
	gpuPod, err := r.podFacade.GetGpuPodsByPodUid(ctx, podUID)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query pod: %w", err)
	}

	if gpuPod == nil {
		return nil, 0, fmt.Errorf("pod with UID %s not found", podUID)
	}

	nodeName := gpuPod.NodeName
	if nodeName == "" {
		return nil, 0, fmt.Errorf("node name is empty for pod %s", podUID)
	}

	// Get node IP
	node, err := r.nodeFacade.GetNodeByName(ctx, nodeName)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query node: %w", err)
	}

	if node == nil {
		return nil, 0, fmt.Errorf("node %s not found", nodeName)
	}

	nodeIP := node.Address
	if nodeIP == "" {
		nodeIP = nodeName // Fallback to node name as hostname
	}

	// Get or create client for this node
	nodeClient := r.getOrCreateClient(nodeIP, nodeName)

	// Get process tree to find a representative PID
	// We need a PID from this pod to access its filesystem via /proc/[pid]/root
	processTreeReq := &types.ProcessTreeRequest{
		PodName:        gpuPod.Name,
		PodNamespace:   gpuPod.Namespace,
		PodUID:         podUID,
		IncludeCmdline: false,
		IncludeEnv:     false,
	}

	processTree, err := nodeClient.GetPodProcessTree(ctx, processTreeReq)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get process tree: %w", err)
	}

	if len(processTree.Containers) == 0 {
		return nil, 0, fmt.Errorf("no containers found in pod")
	}

	// Use the first container's root process PID
	firstContainer := processTree.Containers[0]
	if firstContainer.RootProcess == nil {
		return nil, 0, fmt.Errorf("no root process in container")
	}

	pid := firstContainer.RootProcess.HostPID
	if pid == 0 {
		return nil, 0, fmt.Errorf("invalid PID for container")
	}

	log.Debugf("Using PID %d from container %s to access pod filesystem",
		pid, firstContainer.ContainerName)

	return nodeClient, pid, nil
}

// getOrCreateClient gets or creates a node-exporter client for a node
func (r *Reader) getOrCreateClient(nodeIP, nodeName string) *client.Client {
	cacheKey := nodeName

	if cached, exists := r.clientCache[cacheKey]; exists {
		return cached
	}

	baseURL := fmt.Sprintf("http://%s:%d", nodeIP, r.nodeExporterPort)
	nodeClient := client.NewClient(client.DefaultConfig(baseURL))

	r.clientCache[cacheKey] = nodeClient
	log.Infof("Created node-exporter client for node %s at %s", nodeName, baseURL)

	return nodeClient
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

