package containerfs

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	processtree "github.com/AMD-AGI/Primus-SaFE/Lens/node-exporter/pkg/collector/process-tree"
)

// ProfilerReader provides specialized reading for PyTorch Profiler files
// It supports automatic gzip decompression and chunked reading for large files
type ProfilerReader struct {
	fsReader *FSReader
	// Default chunk size for large file reading (10MB)
	chunkSize int64
	// Maximum uncompressed file size to read (500MB)
	maxUncompressedSize int64
}

// NewProfilerReader creates a new profiler file reader
func NewProfilerReader() *ProfilerReader {
	return &ProfilerReader{
		fsReader:            NewFSReader(),
		chunkSize:           10 * 1024 * 1024,  // 10MB
		maxUncompressedSize: 500 * 1024 * 1024, // 500MB
	}
}

// SetChunkSize sets the chunk size for reading large files
func (p *ProfilerReader) SetChunkSize(size int64) {
	p.chunkSize = size
}

// SetMaxUncompressedSize sets the maximum uncompressed file size
func (p *ProfilerReader) SetMaxUncompressedSize(size int64) {
	p.maxUncompressedSize = size
}

// ProfilerReadRequest represents a profiler file read request
type ProfilerReadRequest struct {
	// PID or Pod identification (same as ReadRequest)
	PID           int    `json:"pid,omitempty"`
	PodUID        string `json:"pod_uid,omitempty"`
	PodName       string `json:"pod_name,omitempty"`
	PodNamespace  string `json:"pod_namespace,omitempty"`
	ContainerName string `json:"container_name,omitempty"`

	// File path
	Path string `json:"path" binding:"required"`

	// Reading options
	AutoDecompress bool  `json:"auto_decompress,omitempty"` // Auto decompress .gz files
	Offset         int64 `json:"offset,omitempty"`          // Read offset (after decompression if enabled)
	Length         int64 `json:"length,omitempty"`          // Bytes to read (0 = all)
	ChunkIndex     int   `json:"chunk_index,omitempty"`     // For chunked reading (0-based)
}

// ProfilerReadResponse represents profiler file read response
type ProfilerReadResponse struct {
	Content          string                              `json:"content,omitempty"`    // File content (base64 encoded)
	FileInfo         *FileInfo                           `json:"file_info"`            // Original file metadata
	BytesRead        int64                               `json:"bytes_read"`           // Actual bytes read
	EOF              bool                                `json:"eof"`                  // End of file reached
	Compressed       bool                                `json:"compressed"`           // Original file is compressed
	Decompressed     bool                                `json:"decompressed"`         // Content was decompressed
	OriginalSize     int64                               `json:"original_size"`        // Compressed file size
	UncompressedSize int64                               `json:"uncompressed_size"`    // Decompressed size (if applicable)
	ChunkInfo        *ChunkInfo                          `json:"chunk_info,omitempty"` // Chunked reading info
	FileType         processtree.PyTorchProfilerFileType `json:"file_type"`            // Profiler file type
}

// ChunkInfo represents information about chunked reading
type ChunkInfo struct {
	ChunkIndex  int   `json:"chunk_index"`  // Current chunk index (0-based)
	ChunkSize   int64 `json:"chunk_size"`   // Configured chunk size
	TotalChunks int   `json:"total_chunks"` // Total number of chunks
	Offset      int64 `json:"offset"`       // Actual offset in file
	IsLastChunk bool  `json:"is_last_chunk"`
}

// ReadProfilerFile reads a profiler file with optional decompression and chunking
func (p *ProfilerReader) ReadProfilerFile(ctx context.Context, req *ProfilerReadRequest) (*ProfilerReadResponse, error) {
	// Resolve PID if not directly provided
	pid, err := p.fsReader.ResolvePID(ctx, req.PID, req.PodUID, req.PodName, req.PodNamespace, req.ContainerName)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve PID: %w", err)
	}

	// Get file info
	fileInfo, err := p.fsReader.GetFileInfo(ctx, pid, req.Path)
	if err != nil {
		return nil, fmt.Errorf("failed to get file info: %w", err)
	}

	if fileInfo.IsDir {
		return nil, fmt.Errorf("path is a directory")
	}

	// Detect if file is compressed
	isCompressed := strings.HasSuffix(strings.ToLower(req.Path), ".gz")

	// Identify profiler file type
	fileType := identifyProfilerFileType(req.Path)

	response := &ProfilerReadResponse{
		FileInfo:     fileInfo,
		Compressed:   isCompressed,
		Decompressed: false,
		OriginalSize: fileInfo.Size,
		FileType:     fileType,
	}

	// Decide reading strategy
	if isCompressed && req.AutoDecompress {
		// Strategy 1: Read and decompress
		return p.readAndDecompress(ctx, pid, req, fileInfo, response)
	} else if req.ChunkIndex >= 0 || req.Length > 0 {
		// Strategy 2: Chunked reading (without decompression)
		return p.readChunked(ctx, pid, req, fileInfo, response)
	} else {
		// Strategy 3: Read entire file (no decompression)
		return p.readEntire(ctx, pid, req, fileInfo, response)
	}
}

// readAndDecompress reads and decompresses a gzip file
func (p *ProfilerReader) readAndDecompress(
	ctx context.Context,
	pid int,
	req *ProfilerReadRequest,
	fileInfo *FileInfo,
	response *ProfilerReadResponse,
) (*ProfilerReadResponse, error) {
	// Build container path
	containerPath := fmt.Sprintf("/proc/%d/root%s", pid, req.Path)

	// Open compressed file
	file, err := os.Open(containerPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	// Create gzip reader
	gzReader, err := gzip.NewReader(file)
	if err != nil {
		return nil, fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer gzReader.Close()

	// Read decompressed content
	var decompressed bytes.Buffer
	written, err := io.CopyN(&decompressed, gzReader, p.maxUncompressedSize+1)
	if err != nil && err != io.EOF {
		return nil, fmt.Errorf("failed to decompress file: %w", err)
	}

	// Check size limit
	if written > p.maxUncompressedSize {
		return nil, fmt.Errorf("decompressed file too large: %d bytes (max: %d bytes)",
			written, p.maxUncompressedSize)
	}

	decompressedData := decompressed.Bytes()
	uncompressedSize := int64(len(decompressedData))

	// Apply offset and length if specified
	start := req.Offset
	if start > uncompressedSize {
		start = uncompressedSize
	}

	end := uncompressedSize
	if req.Length > 0 && start+req.Length < uncompressedSize {
		end = start + req.Length
	}

	content := decompressedData[start:end]
	bytesRead := int64(len(content))

	// Encode to base64
	encoded := base64.StdEncoding.EncodeToString(content)

	response.Content = encoded
	response.BytesRead = bytesRead
	response.EOF = (start+bytesRead >= uncompressedSize)
	response.Decompressed = true
	response.UncompressedSize = uncompressedSize

	log.Debugf("Read and decompressed profiler file: pid=%d, path=%s, original=%d bytes, decompressed=%d bytes, read=%d bytes",
		pid, req.Path, fileInfo.Size, uncompressedSize, bytesRead)

	return response, nil
}

// readChunked reads file in chunks
func (p *ProfilerReader) readChunked(
	ctx context.Context,
	pid int,
	req *ProfilerReadRequest,
	fileInfo *FileInfo,
	response *ProfilerReadResponse,
) (*ProfilerReadResponse, error) {
	// Calculate chunk parameters
	chunkSize := p.chunkSize
	if req.Length > 0 && req.Length < chunkSize {
		chunkSize = req.Length
	}

	var offset int64
	if req.ChunkIndex > 0 {
		offset = int64(req.ChunkIndex) * p.chunkSize
	} else if req.Offset > 0 {
		offset = req.Offset
	}

	// Calculate total chunks
	totalChunks := int((fileInfo.Size + p.chunkSize - 1) / p.chunkSize)
	currentChunkIndex := int(offset / p.chunkSize)
	isLastChunk := (offset+chunkSize >= fileInfo.Size)

	// Read using FSReader
	readReq := &ReadRequest{
		PID:    pid,
		Path:   req.Path,
		Offset: offset,
		Length: chunkSize,
	}

	readResp, err := p.fsReader.ReadFile(ctx, readReq)
	if err != nil {
		return nil, fmt.Errorf("failed to read chunk: %w", err)
	}

	response.Content = readResp.Content
	response.BytesRead = readResp.BytesRead
	response.EOF = readResp.EOF
	response.UncompressedSize = fileInfo.Size
	response.ChunkInfo = &ChunkInfo{
		ChunkIndex:  currentChunkIndex,
		ChunkSize:   p.chunkSize,
		TotalChunks: totalChunks,
		Offset:      offset,
		IsLastChunk: isLastChunk,
	}

	log.Debugf("Read profiler file chunk: pid=%d, path=%s, chunk=%d/%d, offset=%d, size=%d bytes",
		pid, req.Path, currentChunkIndex+1, totalChunks, offset, readResp.BytesRead)

	return response, nil
}

// readEntire reads entire file without decompression
func (p *ProfilerReader) readEntire(
	ctx context.Context,
	pid int,
	req *ProfilerReadRequest,
	fileInfo *FileInfo,
	response *ProfilerReadResponse,
) (*ProfilerReadResponse, error) {
	// Read using FSReader
	readReq := &ReadRequest{
		PID:  pid,
		Path: req.Path,
	}

	readResp, err := p.fsReader.ReadFile(ctx, readReq)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	response.Content = readResp.Content
	response.BytesRead = readResp.BytesRead
	response.EOF = readResp.EOF
	response.UncompressedSize = fileInfo.Size

	log.Debugf("Read entire profiler file: pid=%d, path=%s, size=%d bytes",
		pid, req.Path, readResp.BytesRead)

	return response, nil
}

// ReadProfilerFileByInfo is a convenience method that accepts a PyTorchProfilerFileInfo
func (p *ProfilerReader) ReadProfilerFileByInfo(
	ctx context.Context,
	fileInfo *processtree.PyTorchProfilerFileInfo,
	autoDecompress bool,
	chunkIndex int,
) (*ProfilerReadResponse, error) {
	req := &ProfilerReadRequest{
		PID:            fileInfo.PID,
		Path:           fileInfo.FilePath,
		AutoDecompress: autoDecompress,
		ChunkIndex:     chunkIndex,
	}

	return p.ReadProfilerFile(ctx, req)
}

// GetProfilerFileSize gets the actual size of profiler file (decompressed if gz)
func (p *ProfilerReader) GetProfilerFileSize(ctx context.Context, req *ProfilerReadRequest) (int64, error) {
	// Resolve PID
	pid, err := p.fsReader.ResolvePID(ctx, req.PID, req.PodUID, req.PodName, req.PodNamespace, req.ContainerName)
	if err != nil {
		return 0, fmt.Errorf("failed to resolve PID: %w", err)
	}

	// Get file info
	fileInfo, err := p.fsReader.GetFileInfo(ctx, pid, req.Path)
	if err != nil {
		return 0, fmt.Errorf("failed to get file info: %w", err)
	}

	// If not compressed, return file size directly
	if !strings.HasSuffix(strings.ToLower(req.Path), ".gz") {
		return fileInfo.Size, nil
	}

	// For compressed files, we need to decompress to get actual size
	// This is expensive, so we only do it when explicitly requested
	containerPath := fmt.Sprintf("/proc/%d/root%s", pid, req.Path)
	file, err := os.Open(containerPath)
	if err != nil {
		return 0, fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	gzReader, err := gzip.NewReader(file)
	if err != nil {
		return 0, fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer gzReader.Close()

	// Count decompressed bytes
	size, err := io.Copy(io.Discard, gzReader)
	if err != nil {
		return 0, fmt.Errorf("failed to measure decompressed size: %w", err)
	}

	log.Debugf("Measured decompressed size: %s -> %d bytes", req.Path, size)
	return size, nil
}

// identifyProfilerFileType identifies the profiler file type from path
func identifyProfilerFileType(path string) processtree.PyTorchProfilerFileType {
	path = strings.ToLower(path)
	baseName := filepath.Base(path)

	// Remove .gz extension for matching
	if strings.HasSuffix(baseName, ".gz") {
		baseName = strings.TrimSuffix(baseName, ".gz")
	}

	// Check for specific patterns
	if strings.HasSuffix(baseName, ".pt.trace.json") {
		return processtree.ProfilerTypePyTorchTrace
	}

	if strings.HasSuffix(baseName, ".stacks") {
		return processtree.ProfilerTypeStackTrace
	}

	if strings.Contains(baseName, "kineto") {
		return processtree.ProfilerTypeKineto
	}

	if strings.Contains(baseName, "memory_snapshot") {
		return processtree.ProfilerTypeMemoryDump
	}

	if strings.Contains(baseName, "profiler") || strings.Contains(baseName, "torch_profiler") {
		if strings.HasSuffix(baseName, ".json") {
			return processtree.ProfilerTypeChromeTrace
		}
	}

	return processtree.ProfilerTypeUnknown
}

// ListDirectoryRequest represents a directory listing request
type ListDirectoryRequest struct {
	// PID or Pod identification
	PID           int    `json:"pid,omitempty"`
	PodUID        string `json:"pod_uid,omitempty"`
	PodName       string `json:"pod_name,omitempty"`
	PodNamespace  string `json:"pod_namespace,omitempty"`
	ContainerName string `json:"container_name,omitempty"`

	// Directory path
	Path string `json:"path" binding:"required"`

	// Listing options
	Patterns  []string `json:"patterns,omitempty"`  // File patterns to match (glob patterns)
	Recursive bool     `json:"recursive,omitempty"` // Scan subdirectories
	MaxDepth  int      `json:"max_depth,omitempty"` // Max recursion depth (0 = no limit)
}

// ListDirectoryResponse represents directory listing response
type ListDirectoryResponse struct {
	Files       []*ProfilerFileEntry `json:"files"`
	TotalFiles  int                  `json:"total_files"`
	ScannedDirs int                  `json:"scanned_dirs"`
	Path        string               `json:"path"`
}

// ProfilerFileEntry represents a profiler file entry in directory listing
type ProfilerFileEntry struct {
	Path       string                              `json:"path"`
	Name       string                              `json:"name"`
	Size       int64                               `json:"size"`
	ModTime    string                              `json:"mod_time"`
	IsDir      bool                                `json:"is_dir"`
	FileType   processtree.PyTorchProfilerFileType `json:"file_type"`
	Confidence string                              `json:"confidence"`
}

// ListProfilerDirectory lists a directory for profiler files
func (p *ProfilerReader) ListProfilerDirectory(ctx context.Context, req *ListDirectoryRequest) (*ListDirectoryResponse, error) {
	// Resolve PID
	pid, err := p.fsReader.ResolvePID(ctx, req.PID, req.PodUID, req.PodName, req.PodNamespace, req.ContainerName)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve PID: %w", err)
	}

	// Build container path
	containerPath := fmt.Sprintf("/proc/%d/root%s", pid, req.Path)

	// Check if path exists and is a directory
	info, err := os.Stat(containerPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("directory does not exist: %s", req.Path)
		}
		return nil, fmt.Errorf("failed to stat directory: %w", err)
	}

	if !info.IsDir() {
		return nil, fmt.Errorf("path is not a directory: %s", req.Path)
	}

	// List files
	response := &ListDirectoryResponse{
		Files:       make([]*ProfilerFileEntry, 0),
		Path:        req.Path,
		ScannedDirs: 0,
	}

	maxDepth := req.MaxDepth
	if maxDepth <= 0 {
		maxDepth = 10 // Default max depth to prevent infinite recursion
	}

	err = p.scanDirectory(containerPath, req.Path, req.Patterns, req.Recursive, maxDepth, 0, response)
	if err != nil {
		return nil, fmt.Errorf("failed to scan directory: %w", err)
	}

	response.TotalFiles = len(response.Files)

	log.Infof("Listed directory for profiler files: pid=%d, path=%s, files=%d, dirs_scanned=%d",
		pid, req.Path, response.TotalFiles, response.ScannedDirs)

	return response, nil
}

// scanDirectory recursively scans a directory for profiler files
func (p *ProfilerReader) scanDirectory(
	containerPath string,
	relativePath string,
	patterns []string,
	recursive bool,
	maxDepth int,
	currentDepth int,
	response *ListDirectoryResponse,
) error {
	response.ScannedDirs++

	entries, err := os.ReadDir(containerPath)
	if err != nil {
		log.Warnf("Failed to read directory %s: %v", containerPath, err)
		return nil // Don't fail on permission errors, just skip
	}

	for _, entry := range entries {
		entryPath := filepath.Join(containerPath, entry.Name())
		relativeEntryPath := filepath.Join(relativePath, entry.Name())

		if entry.IsDir() {
			// Recurse into subdirectory if enabled and within depth limit
			if recursive && currentDepth < maxDepth {
				err := p.scanDirectory(entryPath, relativeEntryPath, patterns, recursive, maxDepth, currentDepth+1, response)
				if err != nil {
					log.Warnf("Failed to scan subdirectory %s: %v", entryPath, err)
				}
			}
			continue
		}

		// Check if file matches patterns
		if len(patterns) > 0 && !matchAnyPattern(entry.Name(), patterns) {
			continue
		}

		// Get file info
		info, err := entry.Info()
		if err != nil {
			log.Warnf("Failed to get file info for %s: %v", entryPath, err)
			continue
		}

		// Identify profiler file type and confidence
		fileType := identifyProfilerFileType(entry.Name())
		confidence := getProfilerFileConfidence(entry.Name(), fileType)

		// Only include files that look like profiler files
		if fileType == processtree.ProfilerTypeUnknown && len(patterns) == 0 {
			continue
		}

		response.Files = append(response.Files, &ProfilerFileEntry{
			Path:       relativeEntryPath,
			Name:       entry.Name(),
			Size:       info.Size(),
			ModTime:    info.ModTime().Format("2006-01-02T15:04:05Z07:00"),
			IsDir:      false,
			FileType:   fileType,
			Confidence: confidence,
		})
	}

	return nil
}

// matchAnyPattern checks if filename matches any of the glob patterns
func matchAnyPattern(filename string, patterns []string) bool {
	for _, pattern := range patterns {
		matched, err := filepath.Match(pattern, filename)
		if err != nil {
			log.Warnf("Invalid pattern %s: %v", pattern, err)
			continue
		}
		if matched {
			return true
		}
	}
	return false
}

// getProfilerFileConfidence returns confidence level for a profiler file
func getProfilerFileConfidence(filename string, fileType processtree.PyTorchProfilerFileType) string {
	lowerName := strings.ToLower(filename)

	// High confidence patterns
	if fileType == processtree.ProfilerTypePyTorchTrace ||
		fileType == processtree.ProfilerTypeKineto ||
		strings.Contains(lowerName, "primus-megatron") {
		return "high"
	}

	// Medium confidence patterns
	if fileType == processtree.ProfilerTypeStackTrace ||
		fileType == processtree.ProfilerTypeMemoryDump ||
		strings.Contains(lowerName, "profiler") ||
		strings.Contains(lowerName, "torch_profiler") {
		return "medium"
	}

	// Low confidence - generic JSON files in profiler directories
	if strings.HasSuffix(lowerName, ".json") || strings.HasSuffix(lowerName, ".json.gz") {
		return "low"
	}

	return "unknown"
}
