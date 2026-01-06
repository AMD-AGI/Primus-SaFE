package pyspy

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/constant"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	coreModel "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model/rest"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

var (
	taskFacade          *database.WorkloadTaskFacade
	nodeExporterClient  *NodeExporterClient
)

func init() {
	taskFacade = database.NewWorkloadTaskFacade()
	nodeExporterClient = NewNodeExporterClient()
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

	// Create task in database
	task := &model.WorkloadTaskState{
		WorkloadUID: taskID,
		TaskType:    constant.TaskTypePySpySample,
		Status:      constant.TaskStatusPending,
		Ext:         extMap,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	if err := taskFacade.UpsertTask(c.Request.Context(), task); err != nil {
		log.Errorf("Failed to create task: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create task"})
		return
	}

	log.Infof("Created py-spy task %s for pod %s on node %s", taskID, req.PodUID, req.NodeName)

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

	log.Infof("Cancelled py-spy task %s", taskID)

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

	// Query tasks from database
	tasks, total, err := listTasksWithFilters(c.Request.Context(), &req)
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

	if taskID == "" || filename == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "task_id and filename are required"})
		return
	}

	// Get task to find target node
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

	// Get target node from ext
	nodeName := database.GetExtString(task, "target_node_name")
	if nodeName == "" {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "target node not found in task"})
		return
	}

	// Get node-exporter address
	nodeExporterAddr, err := nodeExporterClient.GetNodeExporterAddress(c.Request.Context(), nodeName)
	if err != nil {
		log.Errorf("Failed to get node-exporter address for node %s: %v", nodeName, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to resolve node-exporter address"})
		return
	}

	// Proxy the file download
	resp, err := nodeExporterClient.ProxyFileDownload(c.Request.Context(), nodeExporterAddr, taskID, filename)
	if err != nil {
		log.Errorf("Failed to proxy file download: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to download file"})
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		log.Errorf("Node-exporter returned status %d: %s", resp.StatusCode, string(bodyBytes))
		c.JSON(resp.StatusCode, gin.H{"error": "failed to download file from node-exporter"})
		return
	}

	// Copy response headers
	for key, values := range resp.Header {
		for _, value := range values {
			c.Header(key, value)
		}
	}

	// Stream the file content
	c.Status(http.StatusOK)
	if _, err := io.Copy(c.Writer, resp.Body); err != nil {
		log.Errorf("Failed to stream file: %v", err)
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

	// Get task to find target node
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

	// Get target node from ext
	nodeName := database.GetExtString(task, "target_node_name")
	if nodeName == "" {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "target node not found in task"})
		return
	}

	// Get node-exporter address
	nodeExporterAddr, err := nodeExporterClient.GetNodeExporterAddress(c.Request.Context(), nodeName)
	if err != nil {
		log.Errorf("Failed to get node-exporter address for node %s: %v", nodeName, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to resolve node-exporter address"})
		return
	}

	// Proxy the file list request
	body, err := nodeExporterClient.ProxyFileList(c.Request.Context(), nodeExporterAddr, taskID)
	if err != nil {
		log.Errorf("Failed to proxy file list: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list files"})
		return
	}
	defer body.Close()

	// Read and forward the response
	respBytes, err := io.ReadAll(body)
	if err != nil {
		log.Errorf("Failed to read file list response: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read file list"})
		return
	}

	c.Data(http.StatusOK, "application/json", respBytes)
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
func listTasksWithFilters(ctx interface{ Done() <-chan struct{} }, req *ListTasksRequest) ([]*model.WorkloadTaskState, int64, error) {
	db := database.GetFacade().GetSystemConfig().GetDB()

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

