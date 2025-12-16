package profiler

import (
	"context"
	"fmt"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/ai-advisor/pkg/profiler/storage"
)

// CollectorConfig represents profiler collector configuration
type CollectorConfig struct {
	AutoCollect   bool                 `yaml:"auto_collect"`
	Interval      int                  `yaml:"interval"` // Collection interval in seconds
	Filter        *FilterConfig        `yaml:"filter"`
	Storage       *storage.StorageConfig `yaml:"storage"`
}

// FilterConfig represents file filtering configuration
type FilterConfig struct {
	MinConfidence  string   `yaml:"min_confidence"`   // "high", "medium", "low"
	MaxFileSize    int64    `yaml:"max_file_size"`    // Maximum file size in bytes
	AllowedTypes   []string `yaml:"allowed_types"`    // Allowed file types
	RequireFramework bool   `yaml:"require_framework"` // Require framework detection
}

// Collector collects and archives profiler files
type Collector struct {
	config         *CollectorConfig
	storageBackend storage.StorageBackend
	nodeClient     *NodeExporterClient
}

// NewCollector creates a new profiler collector
func NewCollector(config *CollectorConfig, storageBackend storage.StorageBackend, nodeExporterURL string) (*Collector, error) {
	if config == nil {
		return nil, fmt.Errorf("collector config is nil")
	}
	if storageBackend == nil {
		return nil, fmt.Errorf("storage backend is nil")
	}

	// Set default filter config
	if config.Filter == nil {
		config.Filter = &FilterConfig{
			MinConfidence: "medium",
			MaxFileSize:   1024 * 1024 * 1024, // 1GB
			AllowedTypes: []string{
				"chrome_trace",
				"pytorch_trace",
				"stack_trace",
				"kineto",
			},
			RequireFramework: true,
		}
	}

	collector := &Collector{
		config:         config,
		storageBackend: storageBackend,
		nodeClient:     NewNodeExporterClient(nodeExporterURL),
	}

	log.Infof("Initialized profiler collector: auto_collect=%v, interval=%ds, storage=%s",
		config.AutoCollect, config.Interval, storageBackend.GetStorageType())

	return collector, nil
}

// ProfilerFileInfo represents discovered profiler file information
type ProfilerFileInfo struct {
	PID          int       `json:"pid"`
	FD           string    `json:"fd"`
	FilePath     string    `json:"file_path"`
	FileName     string    `json:"file_name"`
	FileType     string    `json:"file_type"`
	FileSize     int64     `json:"file_size"`
	Confidence   string    `json:"confidence"` // high/medium/low
	DetectedAt   time.Time `json:"detected_at"`
}

// CollectionRequest represents a collection request
type CollectionRequest struct {
	WorkloadUID  string              `json:"workload_uid"`
	PodUID       string              `json:"pod_uid"`
	PodName      string              `json:"pod_name,omitempty"`
	PodNamespace string              `json:"pod_namespace,omitempty"`
	Framework    string              `json:"framework,omitempty"` // "pytorch", "tensorflow", etc.
	Files        []*ProfilerFileInfo `json:"files"`               // Discovered files from node-exporter
}

// CollectionResult represents collection result
type CollectionResult struct {
	WorkloadUID   string              `json:"workload_uid"`
	TotalFiles    int                 `json:"total_files"`
	ArchivedFiles int                 `json:"archived_files"`
	SkippedFiles  int                 `json:"skipped_files"`
	FailedFiles   int                 `json:"failed_files"`
	Files         []*ArchivedFileInfo `json:"files"`
	CollectedAt   time.Time           `json:"collected_at"`
	Errors        []string            `json:"errors,omitempty"`
}

// ArchivedFileInfo represents an archived file
type ArchivedFileInfo struct {
	FileName     string    `json:"file_name"`
	FilePath     string    `json:"file_path"`
	FileType     string    `json:"file_type"`
	FileSize     int64     `json:"file_size"`
	StorageType  string    `json:"storage_type"`
	StoragePath  string    `json:"storage_path"`
	DownloadURL  string    `json:"download_url"`
	CollectedAt  time.Time `json:"collected_at"`
	Skipped      bool      `json:"skipped,omitempty"`
	SkipReason   string    `json:"skip_reason,omitempty"`
}

// CollectFiles collects profiler files based on discovery results
func (c *Collector) CollectFiles(ctx context.Context, req *CollectionRequest) (*CollectionResult, error) {
	result := &CollectionResult{
		WorkloadUID: req.WorkloadUID,
		TotalFiles:  len(req.Files),
		CollectedAt: time.Now(),
		Files:       make([]*ArchivedFileInfo, 0),
		Errors:      make([]string, 0),
	}

	log.Infof("Starting profiler file collection: workload=%s, pod=%s, files=%d",
		req.WorkloadUID, req.PodUID, len(req.Files))

	for _, fileInfo := range req.Files {
		// Apply filtering
		if !c.shouldCollectFile(req.Framework, fileInfo) {
			result.SkippedFiles++
			result.Files = append(result.Files, &ArchivedFileInfo{
				FileName:   fileInfo.FileName,
				FilePath:   fileInfo.FilePath,
				FileType:   fileInfo.FileType,
				FileSize:   fileInfo.FileSize,
				Skipped:    true,
				SkipReason: c.getSkipReason(req.Framework, fileInfo),
			})
			continue
		}

		// Attempt to collect the file
		archived, err := c.collectSingleFile(ctx, req, fileInfo)
		if err != nil {
			result.FailedFiles++
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", fileInfo.FileName, err))
			log.Errorf("Failed to collect file %s: %v", fileInfo.FileName, err)
			continue
		}

		result.ArchivedFiles++
		result.Files = append(result.Files, archived)
	}

	log.Infof("Profiler file collection completed: workload=%s, total=%d, archived=%d, skipped=%d, failed=%d",
		req.WorkloadUID, result.TotalFiles, result.ArchivedFiles, result.SkippedFiles, result.FailedFiles)

	return result, nil
}

// shouldCollectFile determines if a file should be collected
func (c *Collector) shouldCollectFile(framework string, file *ProfilerFileInfo) bool {
	// 1. Check confidence level
	if !c.checkConfidence(file.Confidence) {
		log.Debugf("Skipping file %s: confidence too low (%s < %s)",
			file.FileName, file.Confidence, c.config.Filter.MinConfidence)
		return false
	}

	// 2. Check file size
	if file.FileSize > c.config.Filter.MaxFileSize {
		log.Debugf("Skipping file %s: file too large (%d > %d bytes)",
			file.FileName, file.FileSize, c.config.Filter.MaxFileSize)
		return false
	}

	// 3. Check file type
	if !c.isAllowedType(file.FileType) {
		log.Debugf("Skipping file %s: type not allowed (%s)",
			file.FileName, file.FileType)
		return false
	}

	// 4. Check framework requirement
	if c.config.Filter.RequireFramework && framework != "pytorch" {
		log.Debugf("Skipping file %s: framework requirement not met (%s != pytorch)",
			file.FileName, framework)
		return false
	}

	// 5. High confidence files always pass
	if file.Confidence == "high" {
		return true
	}

	// 6. Check if auto-collect is enabled for medium/low confidence
	if !c.config.AutoCollect {
		log.Debugf("Skipping file %s: auto-collect disabled and confidence is %s",
			file.FileName, file.Confidence)
		return false
	}

	return true
}

// getSkipReason returns the reason why a file was skipped
func (c *Collector) getSkipReason(framework string, file *ProfilerFileInfo) string {
	if !c.checkConfidence(file.Confidence) {
		return fmt.Sprintf("confidence too low (%s)", file.Confidence)
	}
	if file.FileSize > c.config.Filter.MaxFileSize {
		return fmt.Sprintf("file too large (%d bytes)", file.FileSize)
	}
	if !c.isAllowedType(file.FileType) {
		return fmt.Sprintf("type not allowed (%s)", file.FileType)
	}
	if c.config.Filter.RequireFramework && framework != "pytorch" {
		return fmt.Sprintf("framework requirement not met (%s)", framework)
	}
	if !c.config.AutoCollect && file.Confidence != "high" {
		return "auto-collect disabled"
	}
	return "unknown"
}

// checkConfidence checks if confidence meets minimum requirement
func (c *Collector) checkConfidence(confidence string) bool {
	confidenceLevels := map[string]int{
		"high":   3,
		"medium": 2,
		"low":    1,
	}

	fileLevel := confidenceLevels[confidence]
	minLevel := confidenceLevels[c.config.Filter.MinConfidence]

	return fileLevel >= minLevel
}

// isAllowedType checks if file type is allowed
func (c *Collector) isAllowedType(fileType string) bool {
	if len(c.config.Filter.AllowedTypes) == 0 {
		return true // No filter, allow all
	}

	for _, allowed := range c.config.Filter.AllowedTypes {
		if fileType == allowed {
			return true
		}
	}

	return false
}

// collectSingleFile collects a single profiler file
func (c *Collector) collectSingleFile(
	ctx context.Context,
	req *CollectionRequest,
	fileInfo *ProfilerFileInfo,
) (*ArchivedFileInfo, error) {
	log.Infof("Collecting file: %s (type=%s, size=%d bytes)",
		fileInfo.FileName, fileInfo.FileType, fileInfo.FileSize)

	// Step 1: Read file from node-exporter
	var content []byte
	var err error

	// Use chunked reading for large files (> 50MB)
	chunkThreshold := int64(50 * 1024 * 1024)
	if fileInfo.FileSize > chunkThreshold {
		log.Debugf("Using chunked reading for large file: %s (%d bytes)", fileInfo.FileName, fileInfo.FileSize)
		content, err = c.nodeClient.ReadProfilerFileChunked(ctx, req.PodUID, fileInfo.FilePath, 10*1024*1024)
	} else {
		content, err = c.nodeClient.ReadProfilerFile(ctx, req.PodUID, fileInfo.FilePath)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to read file from node-exporter: %w", err)
	}

	// Step 2: Generate unique file ID
	fileID := generateFileID(req.WorkloadUID, fileInfo.FileName)

	// Step 3: Store file to storage backend
	storeReq := &storage.StoreRequest{
		FileID:      fileID,
		WorkloadUID: req.WorkloadUID,
		FileName:    fileInfo.FileName,
		FileType:    fileInfo.FileType,
		Content:     content,
		Compressed:  false, // Content from node-exporter is not compressed
		Metadata: map[string]string{
			"pod_uid":      req.PodUID,
			"pod_name":     req.PodName,
			"pod_namespace": req.PodNamespace,
			"confidence":   fileInfo.Confidence,
			"detected_at":  fileInfo.DetectedAt.Format(time.RFC3339),
		},
	}

	storeResp, err := c.storageBackend.Store(ctx, storeReq)
	if err != nil {
		return nil, fmt.Errorf("failed to store file: %w", err)
	}

	// Step 4: Generate download URL
	downloadURL, err := c.storageBackend.GenerateDownloadURL(ctx, storeResp.StoragePath, 7*24*time.Hour)
	if err != nil {
		log.Warnf("Failed to generate download URL: %v", err)
		downloadURL = fmt.Sprintf("/api/v1/profiler/files/%s/download", fileID)
	}

	log.Infof("Successfully archived file: %s -> %s (%s)",
		fileInfo.FileName, storeResp.StoragePath, storeResp.StorageType)

	return &ArchivedFileInfo{
		FileName:    fileInfo.FileName,
		FilePath:    fileInfo.FilePath,
		FileType:    fileInfo.FileType,
		FileSize:    storeResp.Size,
		StorageType: storeResp.StorageType,
		StoragePath: storeResp.StoragePath,
		DownloadURL: downloadURL,
		CollectedAt: time.Now(),
	}, nil
}

// generateFileID generates a unique file ID
func generateFileID(workloadUID, fileName string) string {
	timestamp := time.Now().Unix()
	return fmt.Sprintf("%s-%d-%s", workloadUID, timestamp, fileName)
}

// GetConfig returns the collector configuration
func (c *Collector) GetConfig() *CollectorConfig {
	return c.config
}

// GetStorageBackend returns the storage backend
func (c *Collector) GetStorageBackend() storage.StorageBackend {
	return c.storageBackend
}

