// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package pyspy

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/constant"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	coreModel "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model/rest"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/storage"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

var (
	storageBackend storage.StorageBackend
)

// getTaskFacadeForCluster returns the task facade for a specific cluster
// If clusterName is empty, it uses the default cluster
func getTaskFacadeForCluster(clusterName string) *database.WorkloadTaskFacade {
	if clusterName == "" {
		return database.NewWorkloadTaskFacade()
	}
	return database.NewWorkloadTaskFacadeForCluster(clusterName)
}

// getStorageBackend returns the storage backend, initializing it lazily if needed
func getStorageBackend(clusterName string) storage.StorageBackend {
	// For now, use the same database storage backend
	// In the future, could support per-cluster storage configuration
	if storageBackend == nil {
		db := database.GetFacadeForCluster(clusterName).GetSystemConfig().GetDB()
		if db != nil {
			sqlDB, err := db.DB()
			if err == nil {
				storageConfig := &storage.StorageConfig{
					Strategy: "database",
					Database: &storage.DatabaseConfig{
						Compression:         true,
						ChunkSize:           1024 * 1024, // 1MB chunks
						MaxFileSize:         50 * 1024 * 1024, // 50MB max
						MaxConcurrentChunks: 4,
					},
				}
				backend, err := storage.NewStorageBackend(sqlDB, storageConfig)
				if err != nil {
					log.Warnf("Failed to initialize storage backend: %v", err)
				} else {
					storageBackend = backend
				}
			}
		}
	}
	return storageBackend
}

// getClusterName resolves the cluster name from request, using ClusterManager
func getClusterName(c *gin.Context, requestedCluster string) (string, error) {
	cm := clientsets.GetClusterManager()
	clients, err := cm.GetClusterClientsOrDefault(requestedCluster)
	if err != nil {
		return "", err
	}
	return clients.ClusterName, nil
}

// CreateTask creates a new py-spy sampling task
// POST /api/v1/pyspy/sample
func CreateTask(c *gin.Context) {
	var req CreateTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	req.SetDefaults()

	// Resolve cluster name - task will be created in the target cluster's database
	clusterName, err := getClusterName(c, req.Cluster)
	if err != nil {
		log.Errorf("Failed to resolve cluster: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("invalid cluster: %v", err)})
		return
	}

	// Generate task ID
	taskID := fmt.Sprintf("pyspy-%s", uuid.New().String()[:8])

	// Build task ext
	ext := coreModel.PySpyTaskExt{
		TaskID:         taskID,
		TargetNodeName: req.NodeName,
		PodUID:         req.PodUID,
		PodName:        req.PodName,
		PodNamespace:   req.PodNamespace,
		HostPID:        req.PID,
		Duration:       req.Duration,
		Rate:           req.Rate,
		Format:         req.Format,
		Native:         req.Native,
		SubProcesses:   req.SubProcesses,
	}

	extBytes, err := json.Marshal(ext)
	if err != nil {
		log.Errorf("Failed to marshal task ext: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create task"})
		return
	}

	var extMap model.ExtType
	if err := json.Unmarshal(extBytes, &extMap); err != nil {
		log.Errorf("Failed to unmarshal task ext to map: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create task"})
		return
	}

	// Create task in target cluster's database
	task := &model.WorkloadTaskState{
		WorkloadUID: taskID,
		TaskType:    constant.TaskTypePySpySample,
		Status:      constant.TaskStatusPending,
		Ext:         extMap,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	taskFacade := getTaskFacadeForCluster(clusterName)
	if err := taskFacade.UpsertTask(c.Request.Context(), task); err != nil {
		log.Errorf("Failed to create task in cluster %s: %v", clusterName, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create task"})
		return
	}

	log.Infof("Created py-spy task %s for pod %s on node %s in cluster %s", taskID, req.PodUID, req.NodeName, clusterName)

	// Build response
	resp := TaskResponse{
		TaskID:       taskID,
		Status:       constant.TaskStatusPending,
		PodUID:       req.PodUID,
		PodName:      req.PodName,
		PodNamespace: req.PodNamespace,
		NodeName:     req.NodeName,
		PID:          req.PID,
		Duration:     req.Duration,
		Format:       req.Format,
		CreatedAt:    task.CreatedAt,
	}

	c.JSON(http.StatusCreated, rest.SuccessResp(c, resp))
}

// GetTask retrieves a py-spy task by ID
// GET /api/v1/pyspy/task/:id
func GetTask(c *gin.Context) {
	taskID := c.Param("id")
	if taskID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "task id is required"})
		return
	}

	// Get cluster from query parameter
	clusterName, err := getClusterName(c, c.Query("cluster"))
	if err != nil {
		log.Errorf("Failed to resolve cluster: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("invalid cluster: %v", err)})
		return
	}

	taskFacade := getTaskFacadeForCluster(clusterName)
	task, err := taskFacade.GetTask(c.Request.Context(), taskID, constant.TaskTypePySpySample)
	if err != nil {
		log.Errorf("Failed to get task %s: %v", taskID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get task"})
		return
	}

	if task == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "task not found"})
		return
	}

	resp := taskToResponse(task)
	c.JSON(http.StatusOK, rest.SuccessResp(c, resp))
}

// CancelTask cancels a py-spy task
// POST /api/v1/pyspy/task/:id/cancel
func CancelTask(c *gin.Context) {
	taskID := c.Param("id")
	if taskID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "task id is required"})
		return
	}

	var req CancelTaskRequest
	_ = c.ShouldBindJSON(&req) // Optional body

	// Get cluster from query parameter
	clusterName, err := getClusterName(c, c.Query("cluster"))
	if err != nil {
		log.Errorf("Failed to resolve cluster: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("invalid cluster: %v", err)})
		return
	}

	taskFacade := getTaskFacadeForCluster(clusterName)
	task, err := taskFacade.GetTask(c.Request.Context(), taskID, constant.TaskTypePySpySample)
	if err != nil {
		log.Errorf("Failed to get task %s: %v", taskID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get task"})
		return
	}

	if task == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "task not found"})
		return
	}

	// Check if task can be cancelled
	if task.Status == constant.TaskStatusCompleted || task.Status == constant.TaskStatusFailed || task.Status == constant.TaskStatusCancelled {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("cannot cancel task in status: %s", task.Status)})
		return
	}

	// Update task status to cancelled
	if err := taskFacade.UpdateTaskStatus(c.Request.Context(), taskID, constant.TaskTypePySpySample, constant.TaskStatusCancelled); err != nil {
		log.Errorf("Failed to cancel task %s: %v", taskID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to cancel task"})
		return
	}

	// Update ext with cancellation info
	cancelledAt := time.Now().Format(time.RFC3339)
	extUpdate := model.ExtType{
		"cancelled_at": cancelledAt,
		"cancel_reason": req.Reason,
	}
	if err := taskFacade.UpdateTaskExt(c.Request.Context(), taskID, constant.TaskTypePySpySample, extUpdate); err != nil {
		log.Warnf("Failed to update task ext for cancellation: %v", err)
	}

	log.Infof("Cancelled py-spy task %s in cluster %s", taskID, clusterName)

	c.JSON(http.StatusOK, rest.SuccessResp(c, TaskStatusResponse{
		TaskID:  taskID,
		Status:  constant.TaskStatusCancelled,
		Message: "task cancelled",
	}))
}

// ListTasks lists py-spy tasks with optional filters
// POST /api/v1/pyspy/tasks
func ListTasks(c *gin.Context) {
	var req ListTasksRequest
	if err := c.ShouldBind(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	req.SetDefaults()

	// Resolve cluster name
	clusterName, err := getClusterName(c, req.Cluster)
	if err != nil {
		log.Errorf("Failed to resolve cluster: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("invalid cluster: %v", err)})
		return
	}

	// Query tasks from database
	tasks, total, err := listTasksWithFilters(c.Request.Context(), &req, clusterName)
	if err != nil {
		log.Errorf("Failed to list tasks: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list tasks"})
		return
	}

	// Convert to response format
	taskResponses := make([]TaskResponse, 0, len(tasks))
	for _, task := range tasks {
		taskResponses = append(taskResponses, *taskToResponse(task))
	}

	resp := ListTasksResponse{
		Tasks:  taskResponses,
		Total:  total,
		Limit:  req.Limit,
		Offset: req.Offset,
	}

	c.JSON(http.StatusOK, rest.SuccessResp(c, resp))
}

// DownloadFile downloads a py-spy profiling file by proxying to node-exporter
// GET /api/v1/pyspy/file/:task_id/:filename
func DownloadFile(c *gin.Context) {
	taskID := c.Param("task_id")
	filename := c.Param("filename")

	if taskID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "task_id is required"})
		return
	}

	// Get cluster from query parameter
	clusterName, err := getClusterName(c, c.Query("cluster"))
	if err != nil {
		log.Errorf("Failed to resolve cluster: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("invalid cluster: %v", err)})
		return
	}

	// Get task to verify it exists and get storage info
	taskFacade := getTaskFacadeForCluster(clusterName)
	task, err := taskFacade.GetTask(c.Request.Context(), taskID, constant.TaskTypePySpySample)
	if err != nil {
		log.Errorf("Failed to get task %s: %v", taskID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get task"})
		return
	}

	if task == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "task not found"})
		return
	}

	// Check if file is stored in storage backend
	storagePath := database.GetExtString(task, "storage_path")
	if storagePath == "" {
		c.JSON(http.StatusNotFound, gin.H{"error": "file not found in storage"})
		return
	}

	// Get storage backend
	backend := getStorageBackend(clusterName)
	if backend == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "storage backend not available"})
		return
	}

	// Retrieve file from storage
	retrieveReq := &storage.RetrieveRequest{
		FileID:      taskID,
		StoragePath: storagePath,
	}
	
	retrieveResp, err := backend.Retrieve(c.Request.Context(), retrieveReq)
	if err != nil {
		log.Errorf("Failed to retrieve file from storage: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to retrieve file"})
		return
	}

	// Determine filename if not provided
	if filename == "" {
		outputFile := database.GetExtString(task, "output_file")
		if outputFile != "" {
			filename = filepath.Base(outputFile)
		} else {
			filename = fmt.Sprintf("%s.svg", taskID)
		}
	}

	// Set response headers
	format := database.GetExtString(task, "format")
	contentType := getContentTypeForFormat(format)
	c.Header("Content-Type", contentType)
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))
	c.Header("Content-Length", fmt.Sprintf("%d", retrieveResp.Size))

	// Write content
	c.Data(http.StatusOK, contentType, retrieveResp.Content)
}

// getContentTypeForFormat returns content type based on format
func getContentTypeForFormat(format string) string {
	switch format {
	case "flamegraph":
		return "image/svg+xml"
	case "speedscope":
		return "application/json"
	case "raw":
		return "text/plain"
	default:
		return "application/octet-stream"
	}
}

// GetTaskFiles lists files for a specific task
// GET /api/v1/pyspy/file/:task_id
func GetTaskFiles(c *gin.Context) {
	taskID := c.Param("task_id")

	if taskID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "task_id is required"})
		return
	}

	// Get cluster from query parameter
	clusterName, err := getClusterName(c, c.Query("cluster"))
	if err != nil {
		log.Errorf("Failed to resolve cluster: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("invalid cluster: %v", err)})
		return
	}

	// Get task to get file info from ext
	taskFacade := getTaskFacadeForCluster(clusterName)
	task, err := taskFacade.GetTask(c.Request.Context(), taskID, constant.TaskTypePySpySample)
	if err != nil {
		log.Errorf("Failed to get task %s: %v", taskID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get task"})
		return
	}

	if task == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "task not found"})
		return
	}

	// Build file info from task ext
	storagePath := database.GetExtString(task, "storage_path")
	storageType := database.GetExtString(task, "storage_type")
	outputFile := database.GetExtString(task, "output_file")
	format := database.GetExtString(task, "format")

	var files []map[string]interface{}
	if storagePath != "" {
		filename := filepath.Base(outputFile)
		if filename == "" || filename == "." {
			filename = fmt.Sprintf("%s.svg", taskID)
		}

		files = append(files, map[string]interface{}{
			"task_id":      taskID,
			"file_name":    filename,
			"format":       format,
			"file_size":    database.GetExtInt(task, "file_size"),
			"storage_type": storageType,
			"storage_path": storagePath,
			"download_url": fmt.Sprintf("/v1/pyspy/file/%s/%s?cluster=%s", taskID, filename, clusterName),
		})
	}

	c.JSON(http.StatusOK, rest.SuccessResp(c, gin.H{
		"task_id": taskID,
		"files":   files,
	}))
}

// Helper function to convert task to response
func taskToResponse(task *model.WorkloadTaskState) *TaskResponse {
	resp := &TaskResponse{
		TaskID:    task.WorkloadUID,
		Status:    task.Status,
		CreatedAt: task.CreatedAt,
	}

	// Extract fields from ext
	resp.PodUID = database.GetExtString(task, "pod_uid")
	resp.PodName = database.GetExtString(task, "pod_name")
	resp.PodNamespace = database.GetExtString(task, "pod_namespace")
	resp.NodeName = database.GetExtString(task, "target_node_name")
	resp.PID = database.GetExtInt(task, "host_pid")
	resp.Duration = database.GetExtInt(task, "duration")
	resp.Format = database.GetExtString(task, "format")
	resp.OutputFile = database.GetExtString(task, "output_file")
	resp.Error = database.GetExtString(task, "error")

	// Parse file size
	if val, ok := task.Ext["file_size"]; ok {
		switch v := val.(type) {
		case float64:
			resp.FileSize = int64(v)
		case int64:
			resp.FileSize = v
		}
	}

	// Parse timestamps
	if startedAtStr := database.GetExtString(task, "started_at"); startedAtStr != "" {
		if t, err := time.Parse(time.RFC3339, startedAtStr); err == nil {
			resp.StartedAt = &t
		}
	}
	if completedAtStr := database.GetExtString(task, "completed_at"); completedAtStr != "" {
		if t, err := time.Parse(time.RFC3339, completedAtStr); err == nil {
			resp.CompletedAt = &t
		}
	}

	// Set file download path if completed
	if task.Status == constant.TaskStatusCompleted && resp.OutputFile != "" {
		resp.FilePath = fmt.Sprintf("/api/v1/pyspy/file/%s/%s", task.WorkloadUID, getFilename(resp.OutputFile))
	}

	return resp
}

// getFilename extracts filename from path
func getFilename(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' {
			return path[i+1:]
		}
	}
	return path
}

// listTasksWithFilters queries tasks with filters
func listTasksWithFilters(ctx interface{ Done() <-chan struct{} }, req *ListTasksRequest, clusterName string) ([]*model.WorkloadTaskState, int64, error) {
	db := database.GetFacadeForCluster(clusterName).GetSystemConfig().GetDB()

	query := db.Model(&model.WorkloadTaskState{}).
		Where("task_type = ?", constant.TaskTypePySpySample)

	// Apply filters
	if req.Status != "" {
		query = query.Where("status = ?", req.Status)
	}
	if req.PodUID != "" {
		query = query.Where("ext->>'pod_uid' = ?", req.PodUID)
	}
	if req.PodNamespace != "" {
		query = query.Where("ext->>'pod_namespace' = ?", req.PodNamespace)
	}
	if req.NodeName != "" {
		query = query.Where("ext->>'target_node_name' = ?", req.NodeName)
	}

	// Get total count
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// Apply pagination
	var tasks []*model.WorkloadTaskState
	err := query.
		Order("created_at DESC").
		Offset(req.Offset).
		Limit(req.Limit).
		Find(&tasks).Error

	return tasks, total, err
}

// GetProcessTree retrieves the process tree for a pod by proxying to node-exporter
// POST /api/v1/workloads/:uid/process-tree
func GetProcessTree(c *gin.Context) {
	var req ProcessTreeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Resolve cluster name
	clusterName, err := getClusterName(c, req.Cluster)
	if err != nil {
		log.Errorf("Failed to resolve cluster: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("invalid cluster: %v", err)})
		return
	}

	// Get pod information from database
	podFacade := database.GetFacadeForCluster(clusterName).GetPod()
	gpuPod, err := podFacade.GetGpuPodsByPodUid(c.Request.Context(), req.PodUID)
	if err != nil {
		log.Errorf("Failed to query pod %s: %v", req.PodUID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to query pod"})
		return
	}

	if gpuPod == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": fmt.Sprintf("pod with UID %s not found", req.PodUID)})
		return
	}

	nodeName := gpuPod.NodeName
	if nodeName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "pod node name is empty"})
		return
	}

	// Get cluster clients for the target cluster
	cm := clientsets.GetClusterManager()
	clients, err := cm.GetClusterClientsOrDefault(clusterName)
	if err != nil {
		log.Errorf("Failed to get cluster clients for %s: %v", clusterName, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get cluster clients"})
		return
	}

	// Get node-exporter client for the target node
	nodeExporterClient, err := clientsets.GetOrInitNodeExportersClient(
		c.Request.Context(),
		nodeName,
		clients.K8SClientSet.ControllerRuntimeClient,
	)
	if err != nil {
		log.Errorf("Failed to get node-exporter client for node %s: %v", nodeName, err)
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": fmt.Sprintf("failed to connect to node-exporter on node %s: %v", nodeName, err)})
		return
	}

	// Build request body for node-exporter
	nodeExporterReq := map[string]interface{}{
		"pod_name":          gpuPod.Name,
		"pod_namespace":     gpuPod.Namespace,
		"pod_uid":           req.PodUID,
		"include_env":       req.IncludeEnv,
		"include_cmdline":   req.IncludeCmdline,
		"include_resources": req.IncludeResources,
		"include_gpu":       req.IncludeGPU,
	}

	// Call node-exporter process-tree API
	resp, err := nodeExporterClient.GetRestyClient().R().
		SetContext(c.Request.Context()).
		SetBody(nodeExporterReq).
		Post("/v1/process-tree/pod")

	if err != nil {
		log.Errorf("Failed to call node-exporter process-tree API: %v", err)
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "failed to get process tree from node"})
		return
	}

	if resp.StatusCode() != http.StatusOK {
		log.Errorf("Node-exporter process-tree API returned status %d: %s", resp.StatusCode(), resp.String())
		c.JSON(resp.StatusCode(), gin.H{"error": fmt.Sprintf("node-exporter returned error: %s", resp.String())})
		return
	}

	// Parse response from node-exporter
	var nodeExporterResp struct {
		Meta struct {
			Code    int    `json:"code"`
			Message string `json:"message,omitempty"`
		} `json:"meta"`
		Data json.RawMessage `json:"data"`
	}

	if err := json.Unmarshal(resp.Body(), &nodeExporterResp); err != nil {
		log.Errorf("Failed to parse node-exporter response: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to parse process tree response"})
		return
	}

	if nodeExporterResp.Meta.Code != 0 {
		log.Errorf("Node-exporter returned error code %d: %s", nodeExporterResp.Meta.Code, nodeExporterResp.Meta.Message)
		c.JSON(http.StatusInternalServerError, gin.H{"error": nodeExporterResp.Meta.Message})
		return
	}

	// Parse the process tree data
	var processTree PodProcessTree
	if err := json.Unmarshal(nodeExporterResp.Data, &processTree); err != nil {
		log.Errorf("Failed to parse process tree data: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to parse process tree data"})
		return
	}

	// Add node name to response
	processTree.NodeName = nodeName

	log.Infof("Successfully retrieved process tree for pod %s on node %s in cluster %s", req.PodUID, nodeName, clusterName)

	c.JSON(http.StatusOK, rest.SuccessResp(c, processTree))
}
