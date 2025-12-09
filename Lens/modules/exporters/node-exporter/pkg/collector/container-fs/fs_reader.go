package containerfs

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	processtree "github.com/AMD-AGI/Primus-SaFE/Lens/node-exporter/pkg/collector/process-tree"
)

// FSReader provides safe file system access to container files via /proc/[pid]/root
type FSReader struct {
	// Whitelist of allowed path prefixes for security
	allowedPrefixes []string
	// Maximum file size to read (default 100MB)
	maxFileSize int64
}

// NewFSReader creates a new file system reader with security constraints
func NewFSReader() *FSReader {
	return &FSReader{
		// Only allow reading from common ML/AI log directories and distributed storage
		allowedPrefixes: []string{
			"/workspace",
			"/data",
			"/logs",
			"/tmp",
			"/home",
			"/opt",
			"/wekafs", // WekaFS storage system
			"/gpfs",   // IBM GPFS/Spectrum Scale
			"/lustre", // Lustre parallel file system
			"/cephfs", // Ceph file system
			"/mnt",    // Common mount point for NFS and other storage
			"/nfs",    // NFS mounts
		},
		maxFileSize: 100 * 1024 * 1024, // 100MB default
	}
}

// FileInfo represents file metadata
type FileInfo struct {
	Path          string    `json:"path"`
	Size          int64     `json:"size"`
	Mode          string    `json:"mode"`
	ModTime       time.Time `json:"mod_time"`
	IsDir         bool      `json:"is_dir"`
	IsSymlink     bool      `json:"is_symlink"`
	SymlinkTarget string    `json:"symlink_target,omitempty"`
}

// ReadRequest represents a file read request
type ReadRequest struct {
	// Option 1: Specify PID directly (highest priority)
	PID int `json:"pid,omitempty"` // Process ID to access container filesystem

	// Option 2: Specify Pod (will auto-select first process in main container)
	PodUID        string `json:"pod_uid,omitempty"`        // Pod UID (alternative to PID)
	PodName       string `json:"pod_name,omitempty"`       // Pod name (for logging/identification)
	PodNamespace  string `json:"pod_namespace,omitempty"`  // Pod namespace (for logging/identification)
	ContainerName string `json:"container_name,omitempty"` // Specific container name (optional)

	// File access parameters
	Path           string `json:"path" binding:"required"`   // File path within container
	Offset         int64  `json:"offset,omitempty"`          // Read offset
	Length         int64  `json:"length,omitempty"`          // Bytes to read (0 = all, limited by maxFileSize)
	Recursive      bool   `json:"recursive,omitempty"`       // For directory listing
	FollowSymlinks bool   `json:"follow_symlinks,omitempty"` // Whether to follow symlinks
}

// ReadResponse represents file read response
type ReadResponse struct {
	Content   string    `json:"content,omitempty"`   // File content (base64 for binary)
	FileInfo  *FileInfo `json:"file_info"`           // File metadata
	BytesRead int64     `json:"bytes_read"`          // Actual bytes read
	EOF       bool      `json:"eof"`                 // Whether reached end of file
	IsBinary  bool      `json:"is_binary,omitempty"` // Whether content is binary
}

// ListRequest represents a directory listing request
type ListRequest struct {
	// Option 1: Specify PID directly (highest priority)
	PID int `json:"pid,omitempty"` // Process ID to access container filesystem

	// Option 2: Specify Pod (will auto-select first process in main container)
	PodUID        string `json:"pod_uid,omitempty"`        // Pod UID (alternative to PID)
	PodName       string `json:"pod_name,omitempty"`       // Pod name (for logging/identification)
	PodNamespace  string `json:"pod_namespace,omitempty"`  // Pod namespace (for logging/identification)
	ContainerName string `json:"container_name,omitempty"` // Specific container name (optional)

	// Directory listing parameters
	Path      string `json:"path" binding:"required"`
	Recursive bool   `json:"recursive,omitempty"`
	Pattern   string `json:"pattern,omitempty"` // Glob pattern filter
}

// ListResponse represents directory listing response
type ListResponse struct {
	Files []*FileInfo `json:"files"`
	Total int         `json:"total"`
}

// ReadFile reads a file from container filesystem
func (r *FSReader) ReadFile(ctx context.Context, req *ReadRequest) (*ReadResponse, error) {
	// Validate and sanitize path
	if err := r.validatePath(req.Path); err != nil {
		return nil, fmt.Errorf("invalid path: %w", err)
	}

	// Resolve PID if not directly provided
	pid, err := r.resolvePID(ctx, req.PID, req.PodUID, req.PodName, req.PodNamespace, req.ContainerName)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve PID: %w", err)
	}

	// Construct container filesystem path
	containerPath := fmt.Sprintf("/proc/%d/root%s", pid, req.Path)

	// Check if process exists
	if _, err := os.Stat(fmt.Sprintf("/proc/%d", pid)); err != nil {
		return nil, fmt.Errorf("process %d not found or not accessible", pid)
	}

	// Get file info
	fileInfo, err := r.getFileInfo(containerPath, req.Path)
	if err != nil {
		return nil, fmt.Errorf("failed to stat file: %w", err)
	}

	if fileInfo.IsDir {
		return nil, fmt.Errorf("path is a directory, use list endpoint instead")
	}

	// Check file size
	if fileInfo.Size > r.maxFileSize {
		return nil, fmt.Errorf("file too large: %d bytes (max: %d bytes)", fileInfo.Size, r.maxFileSize)
	}

	// Open file
	file, err := os.Open(containerPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	// Seek if offset specified
	if req.Offset > 0 {
		if _, err := file.Seek(req.Offset, 0); err != nil {
			return nil, fmt.Errorf("failed to seek: %w", err)
		}
	}

	// Determine how much to read
	bytesToRead := fileInfo.Size - req.Offset
	if req.Length > 0 && req.Length < bytesToRead {
		bytesToRead = req.Length
	}
	if bytesToRead > r.maxFileSize {
		bytesToRead = r.maxFileSize
	}

	// Read content
	buffer := make([]byte, bytesToRead)
	bytesRead, err := io.ReadFull(file, buffer)
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	// Check if binary
	isBinary := r.isBinaryContent(buffer[:bytesRead])

	// Encode binary data as base64 to prevent corruption during JSON transport
	var content string
	if isBinary {
		content = base64.StdEncoding.EncodeToString(buffer[:bytesRead])
	} else {
		content = string(buffer[:bytesRead])
	}

	response := &ReadResponse{
		Content:   content,
		FileInfo:  fileInfo,
		BytesRead: int64(bytesRead),
		EOF:       req.Offset+int64(bytesRead) >= fileInfo.Size,
		IsBinary:  isBinary,
	}

	log.Debugf("Read %d bytes from container file: pid=%d, path=%s, offset=%d, binary=%v",
		bytesRead, pid, req.Path, req.Offset, isBinary)

	return response, nil
}

// ListDirectory lists files in a directory
func (r *FSReader) ListDirectory(ctx context.Context, req *ListRequest) (*ListResponse, error) {
	// Validate path
	if err := r.validatePath(req.Path); err != nil {
		return nil, fmt.Errorf("invalid path: %w", err)
	}

	// Resolve PID if not directly provided
	pid, err := r.resolvePID(ctx, req.PID, req.PodUID, req.PodName, req.PodNamespace, req.ContainerName)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve PID: %w", err)
	}

	// Construct container filesystem path
	containerPath := fmt.Sprintf("/proc/%d/root%s", pid, req.Path)

	// Check if process exists
	if _, err := os.Stat(fmt.Sprintf("/proc/%d", pid)); err != nil {
		return nil, fmt.Errorf("process %d not found or not accessible", pid)
	}

	var files []*FileInfo

	if req.Recursive {
		// Walk directory tree
		err := filepath.Walk(containerPath, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				log.Warnf("Error accessing path %s: %v", path, err)
				return nil // Continue walking
			}

			// Convert absolute path back to container path
			relPath := strings.TrimPrefix(path, fmt.Sprintf("/proc/%d/root", pid))
			if relPath == "" {
				relPath = "/"
			}

			// Apply pattern filter if specified
			if req.Pattern != "" {
				matched, err := filepath.Match(req.Pattern, filepath.Base(relPath))
				if err != nil || !matched {
					return nil
				}
			}

			fileInfo := &FileInfo{
				Path:    relPath,
				Size:    info.Size(),
				Mode:    info.Mode().String(),
				ModTime: info.ModTime(),
				IsDir:   info.IsDir(),
			}

			// Check if symlink
			if info.Mode()&os.ModeSymlink != 0 {
				fileInfo.IsSymlink = true
				if target, err := os.Readlink(path); err == nil {
					fileInfo.SymlinkTarget = target
				}
			}

			files = append(files, fileInfo)
			return nil
		})

		if err != nil {
			return nil, fmt.Errorf("failed to walk directory: %w", err)
		}
	} else {
		// List only direct children
		entries, err := os.ReadDir(containerPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read directory: %w", err)
		}

		for _, entry := range entries {
			// Apply pattern filter
			if req.Pattern != "" {
				matched, err := filepath.Match(req.Pattern, entry.Name())
				if err != nil || !matched {
					continue
				}
			}

			info, err := entry.Info()
			if err != nil {
				log.Warnf("Failed to get info for %s: %v", entry.Name(), err)
				continue
			}

			filePath := filepath.Join(req.Path, entry.Name())
			fileInfo := &FileInfo{
				Path:    filePath,
				Size:    info.Size(),
				Mode:    info.Mode().String(),
				ModTime: info.ModTime(),
				IsDir:   entry.IsDir(),
			}

			// Check if symlink
			if entry.Type()&os.ModeSymlink != 0 {
				fileInfo.IsSymlink = true
				fullPath := filepath.Join(containerPath, entry.Name())
				if target, err := os.Readlink(fullPath); err == nil {
					fileInfo.SymlinkTarget = target
				}
			}

			files = append(files, fileInfo)
		}
	}

	log.Debugf("Listed %d files in container directory: pid=%d, path=%s, recursive=%v",
		len(files), pid, req.Path, req.Recursive)

	return &ListResponse{
		Files: files,
		Total: len(files),
	}, nil
}

// GetFileInfo gets metadata for a file or directory
func (r *FSReader) GetFileInfo(ctx context.Context, pid int, path string) (*FileInfo, error) {
	if err := r.validatePath(path); err != nil {
		return nil, fmt.Errorf("invalid path: %w", err)
	}

	containerPath := fmt.Sprintf("/proc/%d/root%s", pid, path)
	return r.getFileInfo(containerPath, path)
}

// ResolvePID resolves PID from either direct PID or pod information
func (r *FSReader) ResolvePID(ctx context.Context, pid int, podUID, podName, podNamespace, containerName string) (int, error) {
	return r.resolvePID(ctx, pid, podUID, podName, podNamespace, containerName)
}

// TensorBoardReader provides specialized reading for TensorBoard event files
type TensorBoardReader struct {
	fsReader *FSReader
}

// NewTensorBoardReader creates a new TensorBoard reader
func NewTensorBoardReader() *TensorBoardReader {
	return &TensorBoardReader{
		fsReader: NewFSReader(),
	}
}

// TensorBoardLogInfo represents TensorBoard log directory information
type TensorBoardLogInfo struct {
	LogDir       string      `json:"log_dir"`
	EventFiles   []*FileInfo `json:"event_files"`
	TotalSize    int64       `json:"total_size"`
	LatestUpdate time.Time   `json:"latest_update"`
}

// GetTensorBoardLogs retrieves TensorBoard log files from a container
func (t *TensorBoardReader) GetTensorBoardLogs(ctx context.Context, pid int, logDir string) (*TensorBoardLogInfo, error) {
	// List all event files in the log directory
	listReq := &ListRequest{
		PID:       pid,
		Path:      logDir,
		Recursive: true,
		Pattern:   "events.out.tfevents.*",
	}

	listResp, err := t.fsReader.ListDirectory(ctx, listReq)
	if err != nil {
		return nil, fmt.Errorf("failed to list TensorBoard logs: %w", err)
	}

	info := &TensorBoardLogInfo{
		LogDir:     logDir,
		EventFiles: listResp.Files,
	}

	// Calculate total size and find latest update
	for _, file := range listResp.Files {
		if !file.IsDir {
			info.TotalSize += file.Size
			if file.ModTime.After(info.LatestUpdate) {
				info.LatestUpdate = file.ModTime
			}
		}
	}

	log.Infof("Found %d TensorBoard event files in %s, total size: %d bytes",
		len(info.EventFiles), logDir, info.TotalSize)

	return info, nil
}

// ReadTensorBoardEvent reads a specific TensorBoard event file
func (t *TensorBoardReader) ReadTensorBoardEvent(ctx context.Context, pid int, eventFilePath string, offset, length int64) (*ReadResponse, error) {
	req := &ReadRequest{
		PID:    pid,
		Path:   eventFilePath,
		Offset: offset,
		Length: length,
	}

	return t.fsReader.ReadFile(ctx, req)
}

// Helper methods

func (r *FSReader) validatePath(path string) error {
	// Prevent path traversal
	cleanPath := filepath.Clean(path)
	if strings.Contains(cleanPath, "..") {
		return fmt.Errorf("path traversal not allowed")
	}

	// Check against whitelist
	allowed := false
	for _, prefix := range r.allowedPrefixes {
		if strings.HasPrefix(cleanPath, prefix) || cleanPath == "/" {
			allowed = true
			break
		}
	}

	if !allowed {
		return fmt.Errorf("path not in allowed list: %s (allowed: %v)", cleanPath, r.allowedPrefixes)
	}

	return nil
}

func (r *FSReader) getFileInfo(absolutePath string, displayPath string) (*FileInfo, error) {
	info, err := os.Lstat(absolutePath)
	if err != nil {
		return nil, err
	}

	fileInfo := &FileInfo{
		Path:    displayPath,
		Size:    info.Size(),
		Mode:    info.Mode().String(),
		ModTime: info.ModTime(),
		IsDir:   info.IsDir(),
	}

	// Check if symlink
	if info.Mode()&os.ModeSymlink != 0 {
		fileInfo.IsSymlink = true
		if target, err := os.Readlink(absolutePath); err == nil {
			fileInfo.SymlinkTarget = target
		}
	}

	return fileInfo, nil
}

func (r *FSReader) isBinaryContent(data []byte) bool {
	// Simple heuristic: if more than 30% non-printable characters, consider binary
	if len(data) == 0 {
		return false
	}

	nonPrintable := 0
	for _, b := range data {
		if b < 32 && b != '\n' && b != '\r' && b != '\t' {
			nonPrintable++
		}
	}

	return float64(nonPrintable)/float64(len(data)) > 0.3
}

// resolvePID resolves the PID to use for file access
// Priority: direct PID > pod_uid lookup
func (r *FSReader) resolvePID(ctx context.Context, pid int, podUID, podName, podNamespace, containerName string) (int, error) {
	// If PID is directly provided, use it
	if pid > 0 {
		return pid, nil
	}

	// If pod_uid is provided, lookup the PID
	if podUID != "" {
		collector := processtree.GetCollector()
		if collector == nil {
			return 0, fmt.Errorf("process tree collector not initialized")
		}

		// Get pod process tree
		treeReq := &processtree.ProcessTreeRequest{
			PodUID:       podUID,
			PodName:      podName,
			PodNamespace: podNamespace,
		}

		tree, err := collector.GetPodProcessTree(ctx, treeReq)
		if err != nil {
			return 0, fmt.Errorf("failed to get pod process tree: %w", err)
		}

		if len(tree.Containers) == 0 {
			return 0, fmt.Errorf("no containers found in pod %s", podUID)
		}

		// Find the specified container or use the first one
		var targetContainer *processtree.ContainerProcessTree
		if containerName != "" {
			for _, container := range tree.Containers {
				if container.ContainerName == containerName {
					targetContainer = container
					break
				}
			}
			if targetContainer == nil {
				return 0, fmt.Errorf("container %s not found in pod %s", containerName, podUID)
			}
		} else {
			// Use the first container (usually the main container)
			targetContainer = tree.Containers[0]
		}

		// Get the first PID from the container
		if targetContainer.RootProcess != nil && targetContainer.RootProcess.HostPID > 0 {
			log.Debugf("Resolved PID %d (root process) for pod %s, container %s",
				targetContainer.RootProcess.HostPID, podUID, targetContainer.ContainerName)
			return targetContainer.RootProcess.HostPID, nil
		}

		if len(targetContainer.AllProcesses) > 0 {
			pid := targetContainer.AllProcesses[0].HostPID
			log.Debugf("Resolved PID %d (first process) for pod %s, container %s",
				pid, podUID, targetContainer.ContainerName)
			return pid, nil
		}

		return 0, fmt.Errorf("no processes found in container %s of pod %s", targetContainer.ContainerName, podUID)
	}

	return 0, fmt.Errorf("either pid or pod_uid must be provided")
}
