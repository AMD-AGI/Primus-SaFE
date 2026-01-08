package github_workflow_collector

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/node-exporter/pkg/client"
	"github.com/AMD-AGI/Primus-SaFE/Lens/node-exporter/pkg/types"
)

// PVCReader reads files from Pod PVC via node-exporter
type PVCReader struct {
	// Database facades
	nodeFacade    database.NodeFacadeInterface
	podFacade     database.PodFacadeInterface
	workloadFacade database.WorkloadFacadeInterface

	// Client cache for node-specific clients (thread-safe)
	clientCache sync.Map // nodeName -> *client.Client
}

// PodInfo contains pod information needed for file access
type PodInfo struct {
	UID           string
	Name          string
	Namespace     string
	NodeName      string
	ContainerName string
}

// PVCFile represents a file read from Pod PVC
type PVCFile struct {
	Name    string
	Path    string
	Content []byte
	Size    int64
}

// NewPVCReader creates a new PVC file reader
func NewPVCReader() *PVCReader {
	return &PVCReader{
		nodeFacade:     database.NewNodeFacade(),
		podFacade:      database.NewPodFacade(),
		workloadFacade: database.NewWorkloadFacade(),
	}
}

// GetPodInfoByWorkloadUID gets pod information by workload UID
// Returns the first available pod for the workload
func (r *PVCReader) GetPodInfoByWorkloadUID(ctx context.Context, workloadUID string) (*PodInfo, error) {
	// Get workload-pod references
	refs, err := r.workloadFacade.ListWorkloadPodReferenceByWorkloadUid(ctx, workloadUID)
	if err != nil {
		return nil, fmt.Errorf("failed to get workload-pod references: %w", err)
	}

	if len(refs) == 0 {
		return nil, fmt.Errorf("no pods found for workload %s", workloadUID)
	}

	// Get the first pod's details
	podUID := refs[0].PodUID
	pod, err := r.podFacade.GetGpuPodsByPodUid(ctx, podUID)
	if err != nil {
		return nil, fmt.Errorf("failed to get pod %s: %w", podUID, err)
	}

	if pod == nil {
		return nil, fmt.Errorf("pod %s not found", podUID)
	}

	// Get container name from process tree
	containerName, err := r.getContainerName(ctx, pod)
	if err != nil {
		log.Warnf("PVCReader: failed to get container name for pod %s, using default: %v", podUID, err)
		containerName = "" // Will use default container
	}

	return &PodInfo{
		UID:           pod.UID,
		Name:          pod.Name,
		Namespace:     pod.Namespace,
		NodeName:      pod.NodeName,
		ContainerName: containerName,
	}, nil
}

// getContainerName gets the main container name for a pod
func (r *PVCReader) getContainerName(ctx context.Context, pod *model.GpuPods) (string, error) {
	if pod.NodeName == "" {
		return "", fmt.Errorf("pod has no node name")
	}

	nodeClient, err := r.getOrCreateClient(ctx, pod.NodeName)
	if err != nil {
		return "", err
	}

	processTreeReq := &types.ProcessTreeRequest{
		PodName:        pod.Name,
		PodNamespace:   pod.Namespace,
		PodUID:         pod.UID,
		IncludeCmdline: false,
		IncludeEnv:     false,
	}

	processTree, err := nodeClient.GetPodProcessTree(ctx, processTreeReq)
	if err != nil {
		return "", fmt.Errorf("failed to get process tree: %w", err)
	}

	if len(processTree.Containers) == 0 {
		return "", fmt.Errorf("no containers found in pod")
	}

	// Use the first container
	return processTree.Containers[0].ContainerName, nil
}

// ListMatchingFiles lists files in the pod that match the given patterns
func (r *PVCReader) ListMatchingFiles(ctx context.Context, podInfo *PodInfo, basePaths []string, patterns []string) ([]*types.ContainerFileInfo, error) {
	nodeClient, err := r.getOrCreateClient(ctx, podInfo.NodeName)
	if err != nil {
		return nil, fmt.Errorf("failed to get node client: %w", err)
	}

	var matchingFiles []*types.ContainerFileInfo

	for _, basePath := range basePaths {
		// List directory recursively
		listReq := &types.ContainerDirectoryListRequest{
			PodUID:        podInfo.UID,
			PodName:       podInfo.Name,
			PodNamespace:  podInfo.Namespace,
			ContainerName: podInfo.ContainerName,
			Path:          basePath,
			Recursive:     true,
		}

		listResp, err := nodeClient.ListContainerDirectory(ctx, listReq)
		if err != nil {
			log.Warnf("PVCReader: failed to list directory %s: %v", basePath, err)
			continue
		}

		// Filter files by patterns
		for _, file := range listResp.Files {
			if file.IsDir {
				continue
			}

			if matchesAnyPattern(file.Path, patterns) {
				matchingFiles = append(matchingFiles, file)
			}
		}
	}

	return matchingFiles, nil
}

// ReadFile reads a file from the pod
func (r *PVCReader) ReadFile(ctx context.Context, podInfo *PodInfo, filePath string) (*PVCFile, error) {
	nodeClient, err := r.getOrCreateClient(ctx, podInfo.NodeName)
	if err != nil {
		return nil, fmt.Errorf("failed to get node client: %w", err)
	}

	readReq := &types.ContainerFileReadRequest{
		PodUID:        podInfo.UID,
		PodName:       podInfo.Name,
		PodNamespace:  podInfo.Namespace,
		ContainerName: podInfo.ContainerName,
		Path:          filePath,
	}

	resp, err := nodeClient.ReadContainerFile(ctx, readReq)
	if err != nil {
		return nil, fmt.Errorf("failed to read file %s: %w", filePath, err)
	}

	return &PVCFile{
		Name:    filepath.Base(filePath),
		Path:    filePath,
		Content: []byte(resp.Content),
		Size:    resp.BytesRead,
	}, nil
}

// ReadFiles reads multiple files from the pod
func (r *PVCReader) ReadFiles(ctx context.Context, podInfo *PodInfo, files []*types.ContainerFileInfo, maxSizeBytes int64) ([]*PVCFile, error) {
	var result []*PVCFile

	for _, fileInfo := range files {
		// Skip files that are too large
		if fileInfo.Size > maxSizeBytes {
			log.Warnf("PVCReader: skipping file %s (size %d exceeds limit %d)", fileInfo.Path, fileInfo.Size, maxSizeBytes)
			continue
		}

		file, err := r.ReadFile(ctx, podInfo, fileInfo.Path)
		if err != nil {
			log.Warnf("PVCReader: failed to read file %s: %v", fileInfo.Path, err)
			continue
		}

		result = append(result, file)
	}

	return result, nil
}

// getOrCreateClient gets or creates a node-exporter client for a node
func (r *PVCReader) getOrCreateClient(ctx context.Context, nodeName string) (*client.Client, error) {
	// Check cache first
	if cached, ok := r.clientCache.Load(nodeName); ok {
		return cached.(*client.Client), nil
	}

	// Get node-exporter client using K8s clientsets
	k8sClient := clientsets.GetClusterManager().GetCurrentClusterClients().K8SClientSet.ControllerRuntimeClient

	nodeExporterK8sClient, err := clientsets.GetOrInitNodeExportersClient(ctx, nodeName, k8sClient)
	if err != nil {
		return nil, fmt.Errorf("failed to get node-exporter client for node %s: %w", nodeName, err)
	}

	// Convert to our client type
	baseURL := nodeExporterK8sClient.GetRestyClient().BaseURL
	nodeClient := client.NewClient(client.DefaultConfig(baseURL))

	// Cache the client
	r.clientCache.Store(nodeName, nodeClient)

	log.Infof("PVCReader: created node-exporter client for node %s at %s", nodeName, baseURL)
	return nodeClient, nil
}

// matchesAnyPattern checks if a file path matches any of the given glob patterns
func matchesAnyPattern(path string, patterns []string) bool {
	if len(patterns) == 0 {
		// If no patterns specified, match all files
		return true
	}

	for _, pattern := range patterns {
		// Try matching against full path
		if matched, _ := filepath.Match(pattern, path); matched {
			return true
		}
		// Try matching against base name
		if matched, _ := filepath.Match(pattern, filepath.Base(path)); matched {
			return true
		}
		// Try with double-star pattern simulation (match any directory)
		if strings.Contains(pattern, "**") {
			// Replace ** with * for simple matching
			simplePattern := strings.ReplaceAll(pattern, "**/", "")
			if matched, _ := filepath.Match(simplePattern, filepath.Base(path)); matched {
				return true
			}
		}
	}

	return false
}

