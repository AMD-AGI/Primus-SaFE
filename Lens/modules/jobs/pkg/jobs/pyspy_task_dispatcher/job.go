package pyspy_task_dispatcher

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"path/filepath"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/ai-advisor/pkg/profiler/storage"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/constant"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	coremodel "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/modules/jobs/pkg/common"
	"github.com/google/uuid"
)

const (
	// LockDuration is the duration for distributed lock
	LockDuration = 10 * time.Minute

	// JobSchedule defines when the job runs
	JobSchedule = "@every 5s"
)

// PySpyTaskDispatcherJob dispatches py-spy tasks to node-exporters
type PySpyTaskDispatcherJob struct {
	facade         *database.WorkloadTaskFacade
	instanceID     string
	client         *NodeExporterClient
	resolver       *NodeExporterResolver
	storageBackend storage.StorageBackend
}

// NewPySpyTaskDispatcherJob creates a new dispatcher job
func NewPySpyTaskDispatcherJob() *PySpyTaskDispatcherJob {
	job := &PySpyTaskDispatcherJob{
		facade:     database.NewWorkloadTaskFacade(),
		instanceID: generateInstanceID(),
		client:     NewNodeExporterClient(),
	}

	// Initialize storage backend (database storage by default)
	db := database.GetFacade().GetSystemConfig().GetDB()
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
				log.Warnf("Failed to initialize storage backend: %v, file storage will be skipped", err)
			} else {
				job.storageBackend = backend
				log.Info("Initialized database storage backend for py-spy files")
			}
		}
	}

	return job
}

// NewPySpyTaskDispatcherJobWithStorage creates a dispatcher job with custom storage
func NewPySpyTaskDispatcherJobWithStorage(sqlDB *sql.DB, storageConfig *storage.StorageConfig) *PySpyTaskDispatcherJob {
	job := &PySpyTaskDispatcherJob{
		facade:     database.NewWorkloadTaskFacade(),
		instanceID: generateInstanceID(),
		client:     NewNodeExporterClient(),
	}

	if sqlDB != nil && storageConfig != nil {
		backend, err := storage.NewStorageBackend(sqlDB, storageConfig)
		if err != nil {
			log.Warnf("Failed to initialize storage backend: %v", err)
		} else {
			job.storageBackend = backend
		}
	}

	return job
}

// generateInstanceID generates a unique instance ID for lock ownership
func generateInstanceID() string {
	return fmt.Sprintf("pyspy-dispatcher-%s", uuid.New().String()[:8])
}

// Schedule returns the cron schedule for this job
func (j *PySpyTaskDispatcherJob) Schedule() string {
	return JobSchedule
}

// Run executes the job
func (j *PySpyTaskDispatcherJob) Run(
	ctx context.Context,
	k8sClient *clientsets.K8SClientSet,
	storageClient *clientsets.StorageClientSet,
) (*common.ExecutionStats, error) {
	stats := &common.ExecutionStats{}

	// Initialize resolver with k8s client
	if j.resolver == nil {
		j.resolver = NewNodeExporterResolver(k8sClient)
	}

	// Query pending pyspy_sample tasks
	tasks, err := j.facade.ListPendingTasksByType(ctx, constant.TaskTypePySpySample)
	if err != nil {
		log.Errorf("Failed to list pending pyspy tasks: %v", err)
		return stats, err
	}

	if len(tasks) == 0 {
		return stats, nil
	}

	log.Infof("Found %d pending pyspy_sample tasks", len(tasks))

	var processedCount, failedCount int64

	// Process each task
	for _, task := range tasks {
		if err := j.processTask(ctx, task); err != nil {
			log.Errorf("Failed to process task %s: %v", task.WorkloadUID, err)
			failedCount++
		} else {
			processedCount++
		}
	}

	stats.RecordsProcessed = processedCount
	stats.ItemsUpdated = processedCount
	stats.ItemsDeleted = failedCount // Use ItemsDeleted to track failures

	log.Infof("PySpyTaskDispatcher completed: processed=%d, failed=%d, total=%d",
		processedCount, failedCount, len(tasks))

	return stats, nil
}

// processTask processes a single py-spy task
func (j *PySpyTaskDispatcherJob) processTask(ctx context.Context, task *model.WorkloadTaskState) error {
	// Try to acquire distributed lock
	acquired, err := j.facade.TryAcquireLock(ctx, task.WorkloadUID, task.TaskType, j.instanceID, LockDuration)
	if err != nil {
		return fmt.Errorf("failed to acquire lock: %w", err)
	}
	if !acquired {
		log.Debugf("Lock not acquired for task %s, another instance is handling it", task.WorkloadUID)
		return nil
	}

	// Ensure lock is released when done
	defer func() {
		if err := j.facade.ReleaseLock(ctx, task.WorkloadUID, task.TaskType, j.instanceID); err != nil {
			log.Warnf("Failed to release lock for task %s: %v", task.WorkloadUID, err)
		}
	}()

	// Parse task ext
	var ext coremodel.PySpyTaskExt
	if task.Ext != nil {
		extBytes, err := json.Marshal(task.Ext)
		if err != nil {
			return j.failTask(ctx, task, fmt.Sprintf("failed to marshal ext: %v", err))
		}
		if err := json.Unmarshal(extBytes, &ext); err != nil {
			return j.failTask(ctx, task, fmt.Sprintf("failed to parse task ext: %v", err))
		}
	}

	// Validate required fields
	if ext.TargetNodeName == "" {
		return j.failTask(ctx, task, "target_node_name is required")
	}
	if ext.HostPID == 0 {
		return j.failTask(ctx, task, "host_pid is required")
	}

	// Set task ID if not set
	if ext.TaskID == "" {
		ext.TaskID = task.WorkloadUID
	}

	// Get node-exporter address
	nodeExporterAddr, err := j.resolver.GetNodeExporterAddress(ctx, ext.TargetNodeName)
	if err != nil {
		return j.failTask(ctx, task, fmt.Sprintf("failed to get node-exporter address: %v", err))
	}

	// Update task status to running and record start time
	if err := j.facade.UpdateTaskExt(ctx, task.WorkloadUID, task.TaskType, model.ExtType{
		"started_at": time.Now().Format(time.RFC3339),
	}); err != nil {
		log.Warnf("Failed to update task ext: %v", err)
	}

	log.Infof("Dispatching task %s to node-exporter at %s (PID: %d, Duration: %d)",
		task.WorkloadUID, nodeExporterAddr, ext.HostPID, ext.Duration)

	// Call node-exporter to execute py-spy
	result, err := j.client.ExecutePySpy(ctx, nodeExporterAddr, &ext)
	if err != nil {
		return j.failTask(ctx, task, fmt.Sprintf("node-exporter call failed: %v", err))
	}

	if !result.Success {
		return j.failTask(ctx, task, result.Error)
	}

	// Download and store the profiling file
	var storagePath, storageType string
	if j.storageBackend != nil && result.OutputFile != "" {
		fileName := filepath.Base(result.OutputFile)
		
		// Download file content from node-exporter
		fileContent, err := j.client.DownloadFile(ctx, nodeExporterAddr, ext.TaskID, fileName)
		if err != nil {
			log.Errorf("Failed to download file from node-exporter: %v", err)
			// Continue without file storage, task still considered successful
		} else {
			// Determine file type based on format
			fileType := "pyspy_" + ext.Format
			
			// Store to database/S3
			storeReq := &storage.StoreRequest{
				FileID:      ext.TaskID,
				WorkloadUID: ext.TaskID,
				FileName:    fileName,
				FileType:    fileType,
				Content:     fileContent,
				Metadata: map[string]string{
					"pod_uid":       ext.PodUID,
					"pod_name":      ext.PodName,
					"pod_namespace": ext.PodNamespace,
					"node_name":     ext.TargetNodeName,
					"format":        ext.Format,
					"duration":      fmt.Sprintf("%d", ext.Duration),
					"confidence":    "high",
				},
			}
			
			storeResp, err := j.storageBackend.Store(ctx, storeReq)
			if err != nil {
				log.Errorf("Failed to store file to storage backend: %v", err)
			} else {
				storagePath = storeResp.StoragePath
				storageType = storeResp.StorageType
				log.Infof("Stored file to %s: path=%s, size=%d", storageType, storagePath, storeResp.Size)
				
				// Delete file from node-exporter after successful storage
				if err := j.client.DeleteFile(ctx, nodeExporterAddr, ext.TaskID); err != nil {
					log.Warnf("Failed to delete file from node-exporter: %v", err)
				}
			}
		}
	}

	// Update task as completed
	if err := j.facade.UpdateTaskStatus(ctx, task.WorkloadUID, task.TaskType, constant.TaskStatusCompleted); err != nil {
		return fmt.Errorf("failed to update task status: %w", err)
	}

	extUpdate := model.ExtType{
		"output_file":  result.OutputFile,
		"file_size":    result.FileSize,
		"completed_at": time.Now().Format(time.RFC3339),
	}
	if storagePath != "" {
		extUpdate["storage_path"] = storagePath
		extUpdate["storage_type"] = storageType
	}

	if err := j.facade.UpdateTaskExt(ctx, task.WorkloadUID, task.TaskType, extUpdate); err != nil {
		log.Warnf("Failed to update task ext: %v", err)
	}

	log.Infof("Task %s completed successfully: file=%s, size=%d, storage=%s", 
		task.WorkloadUID, result.OutputFile, result.FileSize, storageType)
	return nil
}

// failTask marks a task as failed
func (j *PySpyTaskDispatcherJob) failTask(ctx context.Context, task *model.WorkloadTaskState, errorMsg string) error {
	log.Errorf("Task %s failed: %s", task.WorkloadUID, errorMsg)

	if err := j.facade.UpdateTaskStatus(ctx, task.WorkloadUID, task.TaskType, constant.TaskStatusFailed); err != nil {
		log.Warnf("Failed to update task status to failed: %v", err)
	}

	if err := j.facade.UpdateTaskExt(ctx, task.WorkloadUID, task.TaskType, model.ExtType{
		"error":      errorMsg,
		"failed_at":  time.Now().Format(time.RFC3339),
	}); err != nil {
		log.Warnf("Failed to update task ext: %v", err)
	}

	return fmt.Errorf("%s", errorMsg)
}

