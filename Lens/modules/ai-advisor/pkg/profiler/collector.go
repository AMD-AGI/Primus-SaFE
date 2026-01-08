package profiler

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/ai-advisor/pkg/profiler/storage"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/node-exporter/pkg/client"
	"github.com/AMD-AGI/Primus-SaFE/Lens/node-exporter/pkg/types"
)

// CollectorConfig represents profiler collector configuration
type CollectorConfig struct {
	AutoCollect bool                   `yaml:"auto_collect"`
	Interval    int                    `yaml:"interval"` // Collection interval in seconds
	Filter      *FilterConfig          `yaml:"filter"`
	Storage     *storage.StorageConfig `yaml:"storage"`
}

// FilterConfig represents file filtering configuration
type FilterConfig struct {
	MinConfidence    string   `yaml:"min_confidence"`    // "high", "medium", "low"
	MaxFileSize      int64    `yaml:"max_file_size"`     // Maximum file size in bytes
	AllowedTypes     []string `yaml:"allowed_types"`     // Allowed file types
	RequireFramework bool     `yaml:"require_framework"` // Require framework detection
}

// Collector collects and archives profiler files
type Collector struct {
	config         *CollectorConfig
	storageBackend storage.StorageBackend
	nodeClient     *client.Client // Use official node-exporter client directly
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

	// Create official node-exporter client with extended timeout for large files
	clientConfig := client.DefaultConfig(nodeExporterURL)
	clientConfig.Timeout = 5 * time.Minute // Extended timeout for large profiler files

	collector := &Collector{
		config:         config,
		storageBackend: storageBackend,
		nodeClient:     client.NewClient(clientConfig),
	}

	log.Infof("Initialized profiler collector: auto_collect=%v, interval=%ds, storage=%s",
		config.AutoCollect, config.Interval, storageBackend.GetStorageType())

	return collector, nil
}

// ProfilerFileInfo represents discovered profiler file information
type ProfilerFileInfo struct {
	PID        int       `json:"pid"`
	FD         string    `json:"fd"`
	FilePath   string    `json:"file_path"`
	FileName   string    `json:"file_name"`
	FileType   string    `json:"file_type"`
	FileSize   int64     `json:"file_size"`
	Confidence string    `json:"confidence"` // high/medium/low
	DetectedAt time.Time `json:"detected_at"`
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
	FileName           string               `json:"file_name"`
	FilePath           string               `json:"file_path"`
	FileType           string               `json:"file_type"`
	FileSize           int64                `json:"file_size"`
	StorageType        string               `json:"storage_type"`
	StoragePath        string               `json:"storage_path"`
	DownloadURL        string               `json:"download_url"`
	CollectedAt        time.Time            `json:"collected_at"`
	Skipped            bool                 `json:"skipped,omitempty"`
	SkipReason         string               `json:"skip_reason,omitempty"`
	// Workload matching fields
	MatchedWorkloads   []FileWorkloadMatch  `json:"matched_workloads,omitempty"`
	PrimaryWorkloadUID string               `json:"primary_workload_uid,omitempty"`
	MatchConfidence    string               `json:"match_confidence,omitempty"`
	HasConflict        bool                 `json:"has_conflict,omitempty"`
	ConflictReason     string               `json:"conflict_reason,omitempty"`
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

	// Step 1: Read file from node-exporter using the official client
	content, err := c.readFileFromContainer(ctx, req, fileInfo)
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
			"pod_uid":       req.PodUID,
			"pod_name":      req.PodName,
			"pod_namespace": req.PodNamespace,
			"confidence":    fileInfo.Confidence,
			"detected_at":   fileInfo.DetectedAt.Format(time.RFC3339),
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

// readFileFromContainer reads a file from container using the official node-exporter client
func (c *Collector) readFileFromContainer(
	ctx context.Context,
	req *CollectionRequest,
	fileInfo *ProfilerFileInfo,
) ([]byte, error) {
	// Use chunked reading for large files (> 50MB)
	chunkThreshold := int64(50 * 1024 * 1024)

	if fileInfo.FileSize > chunkThreshold {
		log.Debugf("Using chunked reading for large file: %s (%d bytes)", fileInfo.FileName, fileInfo.FileSize)
		return c.readFileChunked(ctx, req.PodUID, fileInfo.FilePath, 10*1024*1024)
	}

	// Read entire file at once for smaller files
	readReq := &types.ContainerFileReadRequest{
		PodUID: req.PodUID,
		Path:   fileInfo.FilePath,
	}

	resp, err := c.nodeClient.ReadContainerFile(ctx, readReq)
	if err != nil {
		return nil, err
	}

	log.Infof("Successfully read profiler file: path=%s, size=%d bytes", fileInfo.FilePath, len(resp.Content))
	return []byte(resp.Content), nil
}

// readFileChunked reads a large file in chunks using the official client
func (c *Collector) readFileChunked(
	ctx context.Context,
	podUID string,
	filePath string,
	chunkSize int64,
) ([]byte, error) {
	var fullContent []byte
	offset := int64(0)

	for {
		readReq := &types.ContainerFileReadRequest{
			PodUID: podUID,
			Path:   filePath,
			Offset: offset,
			Length: chunkSize,
		}

		resp, err := c.nodeClient.ReadContainerFile(ctx, readReq)
		if err != nil {
			return nil, err
		}

		fullContent = append(fullContent, []byte(resp.Content)...)
		offset += resp.BytesRead

		log.Debugf("Read chunk: offset=%d, bytes=%d, eof=%v", offset, resp.BytesRead, resp.EOF)

		if resp.EOF {
			break
		}
	}

	log.Infof("Successfully read profiler file in chunks: path=%s, total_size=%d bytes", filePath, len(fullContent))
	return fullContent, nil
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

// GetNodeClient returns the node-exporter client for reuse in other components
func (c *Collector) GetNodeClient() *client.Client {
	return c.nodeClient
}

// LocationCollectionRequest represents a request to collect profiler files from locations
type LocationCollectionRequest struct {
	WorkloadUID        string              `json:"workload_uid"`
	PodUID             string              `json:"pod_uid"`
	PodName            string              `json:"pod_name"`
	PodNamespace       string              `json:"pod_namespace"`
	ContainerName      string              `json:"container_name,omitempty"`
	Framework          string              `json:"framework,omitempty"`
	Locations          []ProfilerLocation  `json:"locations"`
	NodeClient         *client.Client      `json:"-"` // Node-exporter client (injected)
	WorkingDir         string              `json:"working_dir,omitempty"` // Pod's working directory for resolving relative paths
	EnableFileMatcher  bool                `json:"enable_file_matcher,omitempty"` // Enable file-workload timestamp matching
}

// CollectProfilerFilesFromLocations collects profiler files from specified locations
func (c *Collector) CollectProfilerFilesFromLocations(
	ctx context.Context,
	req *LocationCollectionRequest,
) (*CollectionResult, error) {
	result := &CollectionResult{
		WorkloadUID: req.WorkloadUID,
		CollectedAt: time.Now(),
		Files:       make([]*ArchivedFileInfo, 0),
		Errors:      make([]string, 0),
	}

	log.Infof("Starting profiler file collection from %d locations for workload %s",
		len(req.Locations), req.WorkloadUID)

	// Use provided nodeClient or fallback to collector's default client
	nodeClient := req.NodeClient
	if nodeClient == nil {
		nodeClient = c.nodeClient
	}

	// Initialize file-workload matcher if enabled
	var fileMatcher *FileWorkloadMatcher
	if req.EnableFileMatcher {
		fileMatcher = NewFileWorkloadMatcher()
		log.Infof("File-workload timestamp matching enabled for workload %s", req.WorkloadUID)
	}

	// Track already collected files to avoid duplicates
	collectedFiles := make(map[string]bool)

	for _, location := range req.Locations {
		// Resolve relative paths to absolute paths using working directory
		scanPath := location.Directory
		if !filepath.IsAbs(scanPath) && req.WorkingDir != "" {
			scanPath = filepath.Join(req.WorkingDir, scanPath)
			log.Debugf("Resolved relative path %s to absolute path %s", location.Directory, scanPath)
		}

		log.Debugf("Scanning location: %s (patterns: %v)", scanPath, location.Patterns)

		// List files in directory
		listReq := &types.ContainerDirectoryListRequest{
			PodUID:        req.PodUID,
			PodName:       req.PodName,
			PodNamespace:  req.PodNamespace,
			ContainerName: req.ContainerName,
			Path:          scanPath,
			Recursive:     location.Recursive,
		}

		listResp, err := nodeClient.ListContainerDirectory(ctx, listReq)
		if err != nil {
			errMsg := fmt.Sprintf("failed to list directory %s: %v", location.Directory, err)
			log.Warnf(errMsg)
			result.Errors = append(result.Errors, errMsg)
			continue
		}

		log.Debugf("Found %d files in %s", listResp.Total, location.Directory)

		// Filter files by patterns
		for _, fileInfo := range listResp.Files {
			// Skip if already collected
			if collectedFiles[fileInfo.Path] {
				continue
			}

			// Check if file matches any pattern
			if !c.matchesAnyPattern(fileInfo.Path, location.Patterns) {
				continue
			}

			// Skip if file is too large
			if fileInfo.Size > c.config.Filter.MaxFileSize {
				log.Debugf("Skipping file %s: too large (%d > %d bytes)",
					fileInfo.Path, fileInfo.Size, c.config.Filter.MaxFileSize)
				result.SkippedFiles++
				result.Files = append(result.Files, &ArchivedFileInfo{
					FileName:   filepath.Base(fileInfo.Path),
					FilePath:   fileInfo.Path,
					FileSize:   fileInfo.Size,
					Skipped:    true,
					SkipReason: fmt.Sprintf("file too large (%d bytes)", fileInfo.Size),
				})
				continue
			}

			log.Debugf("Found matching profiler file: %s (%d bytes)", fileInfo.Path, fileInfo.Size)

			// Apply file-workload timestamp matching BEFORE collection to filter out files
			// that don't belong to the current workload
			var matchResult *FileMatchResult
			if fileMatcher != nil {
				var matchErr error
				matchResult, matchErr = fileMatcher.MatchFileToWorkloads(ctx, fileInfo.Path, req.PodNamespace)
				if matchErr != nil {
					log.Warnf("Failed to match file %s to workloads: %v", fileInfo.Path, matchErr)
				} else if matchResult != nil {
					// Check if the file matches the current workload
					fileMatchesCurrentWorkload := false
					for _, match := range matchResult.Matches {
						if match.WorkloadUID == req.WorkloadUID {
							fileMatchesCurrentWorkload = true
							break
						}
					}
					
					// If file doesn't match current workload based on timestamp, skip it
					if !fileMatchesCurrentWorkload {
						log.Debugf("Skipping file %s: timestamp doesn't match workload %s (file time: %s)",
							fileInfo.Path, req.WorkloadUID, matchResult.FileTime)
						result.SkippedFiles++
						result.Files = append(result.Files, &ArchivedFileInfo{
							FileName:   filepath.Base(fileInfo.Path),
							FilePath:   fileInfo.Path,
							FileSize:   fileInfo.Size,
							Skipped:    true,
							SkipReason: fmt.Sprintf("timestamp doesn't match workload (file time: %s)", matchResult.FileTime),
						})
						continue
					}
				}
			}

			// Check if file is already stored to avoid duplicates
			fileName := filepath.Base(fileInfo.Path)
			exists, existErr := c.storageBackend.ExistsByWorkloadAndFilename(ctx, req.WorkloadUID, fileName)
			if existErr != nil {
				log.Warnf("Failed to check file existence for %s: %v", fileInfo.Path, existErr)
			} else if exists {
				log.Debugf("Skipping file %s: already stored for workload %s", fileInfo.Path, req.WorkloadUID)
				collectedFiles[fileInfo.Path] = true
				continue
			}

			log.Infof("Collecting profiler file: %s (%d bytes)", fileInfo.Path, fileInfo.Size)

			// Check if file is stable (not being written to)
			// Wait up to 5 seconds for file size to stabilize
			stableSize, isStable := c.waitForFileStability(ctx, nodeClient, req.PodUID, req.PodName, req.PodNamespace, req.ContainerName, fileInfo.Path, fileInfo.Size)
			if !isStable {
				log.Warnf("Skipping file %s: file is still being written (size changed from %d to %d)",
					fileInfo.Path, fileInfo.Size, stableSize)
				result.SkippedFiles++
				result.Files = append(result.Files, &ArchivedFileInfo{
					FileName:   filepath.Base(fileInfo.Path),
					FilePath:   fileInfo.Path,
					FileSize:   fileInfo.Size,
					Skipped:    true,
					SkipReason: fmt.Sprintf("file still being written (size: %d -> %d)", fileInfo.Size, stableSize),
				})
				continue
			}
			// Update file size if it changed during stability check
			if stableSize != fileInfo.Size {
				log.Infof("File size stabilized: %s (%d -> %d bytes)", fileInfo.Path, fileInfo.Size, stableSize)
				fileInfo.Size = stableSize
			}

			// Mark as collected to avoid duplicates
			collectedFiles[fileInfo.Path] = true
			result.TotalFiles++

			// Convert to ProfilerFileInfo for collection
			profilerFile := &ProfilerFileInfo{
				FilePath:   fileInfo.Path,
				FileName:   fileName,
				FileSize:   fileInfo.Size,
				FileType:   c.detectFileType(fileInfo.Path),
				Confidence: "high", // Files matching patterns have high confidence
				DetectedAt: time.Now(),
			}

			// Collect the file
			collectionReq := &CollectionRequest{
				WorkloadUID:  req.WorkloadUID,
				PodUID:       req.PodUID,
				PodName:      req.PodName,
				PodNamespace: req.PodNamespace,
				Framework:    req.Framework,
			}

			archived, err := c.collectSingleFileWithClient(ctx, nodeClient, collectionReq, profilerFile)
			if err != nil {
				result.FailedFiles++
				result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", fileInfo.Path, err))
				log.Errorf("Failed to collect file %s: %v", fileInfo.Path, err)
				continue
			}

			// Apply match result if available
			if matchResult != nil {
				archived.MatchedWorkloads = matchResult.Matches
				archived.MatchConfidence = matchResult.GetConfidence()
				archived.HasConflict = matchResult.HasConflict
				archived.ConflictReason = matchResult.ConflictReason
				
				if matchResult.PrimaryMatch != nil {
					archived.PrimaryWorkloadUID = matchResult.PrimaryMatch.WorkloadUID
					
					// Log conflict warning
					if matchResult.HasConflict {
						workloadUIDs := matchResult.GetAllMatchedWorkloadUIDs()
						log.Warnf("File %s has conflict: matched to %d workloads: %v (primary: %s)",
							fileInfo.Path, len(workloadUIDs), workloadUIDs, archived.PrimaryWorkloadUID)
					}
				}
			}

			result.ArchivedFiles++
			result.Files = append(result.Files, archived)
		}
	}

	log.Infof("Profiler file collection completed: workload=%s, total=%d, archived=%d, skipped=%d, failed=%d",
		req.WorkloadUID, result.TotalFiles, result.ArchivedFiles, result.SkippedFiles, result.FailedFiles)

	return result, nil
}

// matchesAnyPattern checks if a file path matches any of the given patterns
func (c *Collector) matchesAnyPattern(filePath string, patterns []string) bool {
	fileName := filepath.Base(filePath)
	
	for _, pattern := range patterns {
		// Try matching both full path and filename
		if matched, _ := filepath.Match(pattern, fileName); matched {
			return true
		}
		if matched, _ := filepath.Match(pattern, filePath); matched {
			return true
		}
		// Handle patterns with brackets (like primus-megatron-exp[...])
		// filepath.Match treats [...] as character class, so we need special handling
		if strings.Contains(pattern, "[") && strings.Contains(pattern, "]") {
			// Simple containment check for bracket patterns
			patternParts := strings.Split(pattern, "*")
			allMatch := true
			remaining := fileName
			for _, part := range patternParts {
				if part == "" {
					continue
				}
				idx := strings.Index(remaining, part)
				if idx == -1 {
					allMatch = false
					break
				}
				remaining = remaining[idx+len(part):]
			}
			if allMatch {
				return true
			}
		}
	}
	return false
}

// waitForFileStability checks if a file is stable (not being written to)
// Returns the final stable size and whether the file is stable
func (c *Collector) waitForFileStability(
	ctx context.Context,
	nodeClient *client.Client,
	podUID, podName, podNamespace, containerName string,
	filePath string,
	initialSize int64,
) (int64, bool) {
	const maxRetries = 3
	const retryInterval = 2 * time.Second

	previousSize := initialSize

	for i := 0; i < maxRetries; i++ {
		// Wait before checking
		select {
		case <-ctx.Done():
			return previousSize, false
		case <-time.After(retryInterval):
		}

		// Get current file info
		currentInfo, err := nodeClient.GetContainerFileInfo(ctx, podUID, podName, podNamespace, containerName, filePath)
		if err != nil {
			log.Warnf("Failed to get file info for stability check: %v", err)
			// If we can't get file info, assume it's stable (best effort)
			return previousSize, true
		}

		currentSize := currentInfo.Size

		// Check if size is stable
		if currentSize == previousSize {
			log.Debugf("File %s is stable at %d bytes (checked %d times)", filePath, currentSize, i+1)
			return currentSize, true
		}

		log.Debugf("File %s size changed: %d -> %d bytes, waiting...", filePath, previousSize, currentSize)
		previousSize = currentSize
	}

	// Size still changing after all retries
	return previousSize, false
}

// detectFileType detects the profiler file type based on filename
func (c *Collector) detectFileType(filePath string) string {
	fileName := strings.ToLower(filepath.Base(filePath))
	
	if strings.HasSuffix(fileName, ".pt.trace.json.gz") || strings.HasSuffix(fileName, ".pt.trace.json") {
		return "pytorch_trace"
	}
	if strings.Contains(fileName, "kineto") {
		return "kineto"
	}
	if strings.HasSuffix(fileName, ".json") || strings.HasSuffix(fileName, ".json.gz") {
		return "chrome_trace"
	}
	return "unknown"
}

// collectSingleFileWithClient collects a single profiler file using provided client
func (c *Collector) collectSingleFileWithClient(
	ctx context.Context,
	nodeClient *client.Client,
	req *CollectionRequest,
	fileInfo *ProfilerFileInfo,
) (*ArchivedFileInfo, error) {
	log.Infof("Collecting file: %s (type=%s, size=%d bytes)",
		fileInfo.FileName, fileInfo.FileType, fileInfo.FileSize)

	// Step 1: Read file from node-exporter using the provided client
	content, err := c.readFileWithClient(ctx, nodeClient, req.PodUID, fileInfo)
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
		Compressed:  strings.HasSuffix(fileInfo.FileName, ".gz"),
		Metadata: map[string]string{
			"pod_uid":       req.PodUID,
			"pod_name":      req.PodName,
			"pod_namespace": req.PodNamespace,
			"file_type":     fileInfo.FileType,
			"detected_at":   fileInfo.DetectedAt.Format(time.RFC3339),
			"original_path": fileInfo.FilePath,
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

// readFileWithClient reads a file from container using provided node-exporter client
func (c *Collector) readFileWithClient(
	ctx context.Context,
	nodeClient *client.Client,
	podUID string,
	fileInfo *ProfilerFileInfo,
) ([]byte, error) {
	// Use chunked reading for large files (> 50MB)
	chunkThreshold := int64(50 * 1024 * 1024)

	if fileInfo.FileSize > chunkThreshold {
		log.Debugf("Using chunked reading for large file: %s (%d bytes)", fileInfo.FileName, fileInfo.FileSize)
		return c.readFileChunkedWithClient(ctx, nodeClient, podUID, fileInfo.FilePath, 10*1024*1024)
	}

	// Read entire file at once for smaller files
	readReq := &types.ContainerFileReadRequest{
		PodUID: podUID,
		Path:   fileInfo.FilePath,
	}

	resp, err := nodeClient.ReadContainerFile(ctx, readReq)
	if err != nil {
		return nil, err
	}

	log.Infof("Successfully read profiler file: path=%s, size=%d bytes", fileInfo.FilePath, len(resp.Content))
	return []byte(resp.Content), nil
}

// readFileChunkedWithClient reads a large file in chunks using provided client
func (c *Collector) readFileChunkedWithClient(
	ctx context.Context,
	nodeClient *client.Client,
	podUID string,
	filePath string,
	chunkSize int64,
) ([]byte, error) {
	var fullContent []byte
	offset := int64(0)

	for {
		readReq := &types.ContainerFileReadRequest{
			PodUID: podUID,
			Path:   filePath,
			Offset: offset,
			Length: chunkSize,
		}

		resp, err := nodeClient.ReadContainerFile(ctx, readReq)
		if err != nil {
			return nil, err
		}

		fullContent = append(fullContent, []byte(resp.Content)...)
		offset += resp.BytesRead

		log.Debugf("Read chunk: offset=%d, bytes=%d, eof=%v", offset, resp.BytesRead, resp.EOF)

		if resp.EOF {
			break
		}
	}

	log.Infof("Successfully read profiler file in chunks: path=%s, total_size=%d bytes", filePath, len(fullContent))
	return fullContent, nil
}
