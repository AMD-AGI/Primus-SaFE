package tensorboard

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/node-exporter/pkg/types"
)

// StreamReader provides streaming access to TensorBoard logs with offset management
type StreamReader struct {
	reader *Reader

	// Offset manager to track read positions
	offsetMgr *OffsetManager

	// Active streams
	streams sync.Map // workloadUID -> *StreamSession
}

// NewStreamReader creates a new stream reader
func NewStreamReader(reader *Reader) *StreamReader {
	return &StreamReader{
		reader:    reader,
		offsetMgr: NewOffsetManager(),
	}
}

// StreamSession represents an active streaming session
type StreamSession struct {
	WorkloadUID string
	PodUID      string
	LogDir      string
	EventFiles  []string // 精确的事件文件列表（如果提供）

	// File tracking
	files     []*FileTracker
	fileMap   map[string]*FileTracker
	fileMutex sync.RWMutex

	// Stream control
	ctx          context.Context
	cancel       context.CancelFunc
	updates      chan *StreamUpdate
	errors       chan error
	pollInterval time.Duration

	// Reconnection support
	lastUpdate   time.Time
	reconnectCnt int
}

// FileTracker tracks reading position for a file
type FileTracker struct {
	Path         string
	Offset       int64
	Size         int64
	LastModTime  time.Time
	LastReadTime time.Time
	EOF          bool
}

// StreamUpdate represents new data from streaming
type StreamUpdate struct {
	File      string                   `json:"file"`
	Content   string                   `json:"content"`
	Offset    int64                    `json:"offset"`
	NewOffset int64                    `json:"new_offset"`
	BytesRead int64                    `json:"bytes_read"`
	Timestamp time.Time                `json:"timestamp"`
	FileInfo  *types.ContainerFileInfo `json:"file_info,omitempty"`
}

// StreamConfig configures streaming behavior
type StreamConfig struct {
	// Poll interval for checking new data
	PollInterval time.Duration

	// Chunk size for each read
	ChunkSize int64

	// Buffer size for update channel
	BufferSize int

	// Whether to read historical data first
	ReadHistorical bool

	// Maximum historical data to read per file
	MaxHistoricalBytes int64

	// Whether to follow file rotations
	FollowRotation bool
}

// DefaultStreamConfig returns default streaming configuration
func DefaultStreamConfig() *StreamConfig {
	return &StreamConfig{
		PollInterval:       2 * time.Second,
		ChunkSize:          64 * 1024, // 64KB per read
		BufferSize:         100,
		ReadHistorical:     false,
		MaxHistoricalBytes: 10 * 1024 * 1024, // 10MB
		FollowRotation:     true,
	}
}

// StreamRequest represents a request to start streaming
type StreamRequest struct {
	WorkloadUID string        `json:"workload_uid" binding:"required"`
	PodUID      string        `json:"pod_uid" binding:"required"`
	LogDir      string        `json:"log_dir"`               // 日志目录（可选，用于发现文件）
	EventFiles  []string      `json:"event_files,omitempty"` // 精确的事件文件列表（优先使用）
	Config      *StreamConfig `json:"config,omitempty"`

	// Resume from saved state
	ResumeState *StreamState `json:"resume_state,omitempty"`
}

// StreamState represents streaming state for resumption
type StreamState struct {
	WorkloadUID string           `json:"workload_uid"`
	FileOffsets map[string]int64 `json:"file_offsets"`
	LastUpdate  time.Time        `json:"last_update"`
	SessionID   string           `json:"session_id"`
}

// StartStream starts streaming TensorBoard logs
func (s *StreamReader) StartStream(ctx context.Context, req *StreamRequest) (*StreamSession, error) {
	log.Infof("Starting stream for workload %s, log_dir=%s", req.WorkloadUID, req.LogDir)

	// Check if already streaming
	if existing, ok := s.streams.Load(req.WorkloadUID); ok {
		existingSession := existing.(*StreamSession)
		log.Warnf("Stream already exists for workload %s, stopping old stream", req.WorkloadUID)
		existingSession.Stop()
	}

	config := req.Config
	if config == nil {
		config = DefaultStreamConfig()
	}

	// Create session context
	sessionCtx, cancel := context.WithCancel(ctx)

	session := &StreamSession{
		WorkloadUID:  req.WorkloadUID,
		PodUID:       req.PodUID,
		LogDir:       req.LogDir,
		EventFiles:   req.EventFiles,
		ctx:          sessionCtx,
		cancel:       cancel,
		updates:      make(chan *StreamUpdate, config.BufferSize),
		errors:       make(chan error, 10),
		pollInterval: config.PollInterval,
		fileMap:      make(map[string]*FileTracker),
		lastUpdate:   time.Now(),
	}

	// Initialize file list
	if err := session.refreshFileList(s.reader); err != nil {
		cancel()
		return nil, fmt.Errorf("failed to initialize file list: %w", err)
	}

	// Resume from saved state if provided
	if req.ResumeState != nil {
		session.restoreState(req.ResumeState)
		log.Infof("Resuming stream from saved state, %d file offsets restored",
			len(req.ResumeState.FileOffsets))
	}

	// Read historical data if requested
	if config.ReadHistorical && req.ResumeState == nil {
		go session.readHistorical(s.reader, config.MaxHistoricalBytes)
	}

	// Start streaming loop
	go session.streamLoop(s.reader, config)

	// Register session
	s.streams.Store(req.WorkloadUID, session)

	log.Infof("Stream started for workload %s with poll interval %v",
		req.WorkloadUID, config.PollInterval)

	return session, nil
}

// GetStream retrieves an active stream session
func (s *StreamReader) GetStream(workloadUID string) (*StreamSession, bool) {
	if session, ok := s.streams.Load(workloadUID); ok {
		return session.(*StreamSession), true
	}
	return nil, false
}

// StopStream stops a streaming session
func (sr *StreamReader) StopStream(workloadUID string) error {
	if session, ok := sr.streams.Load(workloadUID); ok {
		s := session.(*StreamSession)
		s.Stop()
		sr.streams.Delete(workloadUID)
		log.Infof("Stream stopped for workload %s", workloadUID)
		return nil
	}
	return fmt.Errorf("no active stream for workload %s", workloadUID)
}

// streamLoop is the main streaming loop
func (session *StreamSession) streamLoop(reader *Reader, config *StreamConfig) {
	ticker := time.NewTicker(session.pollInterval)
	defer ticker.Stop()

	log.Debugf("Stream loop started for workload %s", session.WorkloadUID)

	for {
		select {
		case <-session.ctx.Done():
			log.Infof("Stream loop stopped for workload %s", session.WorkloadUID)
			close(session.updates)
			close(session.errors)
			return

		case <-ticker.C:
			// Refresh file list periodically (handle file rotation)
			if config.FollowRotation {
				if err := session.refreshFileList(reader); err != nil {
					log.Errorf("Failed to refresh file list: %v", err)
					session.sendError(fmt.Errorf("file list refresh failed: %w", err))
					continue
				}
			}

			// Check each file for new data
			session.fileMutex.RLock()
			files := make([]*FileTracker, len(session.files))
			copy(files, session.files)
			session.fileMutex.RUnlock()

			for _, tracker := range files {
				if session.ctx.Err() != nil {
					return
				}

				// Read new data from this file
				update, err := session.readFileIncremental(reader, tracker, config.ChunkSize)
				if err != nil {
					log.Errorf("Failed to read file %s: %v", tracker.Path, err)
					session.sendError(fmt.Errorf("read error for %s: %w", tracker.Path, err))
					continue
				}

				if update != nil {
					session.sendUpdate(update)
					session.lastUpdate = time.Now()
				}
			}
		}
	}
}

// readFileIncremental reads new data from a file incrementally
func (session *StreamSession) readFileIncremental(reader *Reader, tracker *FileTracker, chunkSize int64) (*StreamUpdate, error) {
	// Get current file info
	fileInfo, err := reader.GetFileInfo(session.ctx, session.PodUID, tracker.Path)
	if err != nil {
		return nil, fmt.Errorf("failed to get file info: %w", err)
	}

	// Update size and mod time
	tracker.Size = fileInfo.Size
	tracker.LastModTime = fileInfo.ModTime

	// Check if file was truncated (rotation)
	if fileInfo.Size < tracker.Offset {
		log.Warnf("File %s was truncated (size %d < offset %d), resetting offset",
			tracker.Path, fileInfo.Size, tracker.Offset)
		tracker.Offset = 0
		tracker.EOF = false
	}

	// Check if there's new data
	if fileInfo.Size <= tracker.Offset {
		// No new data
		tracker.EOF = true
		return nil, nil
	}

	// Calculate how much to read
	remaining := fileInfo.Size - tracker.Offset
	toRead := chunkSize
	if toRead > remaining {
		toRead = remaining
	}

	// Read the chunk
	resp, err := reader.ReadFile(session.ctx, session.PodUID, tracker.Path, tracker.Offset, toRead)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	// Update tracker
	newOffset := tracker.Offset + resp.BytesRead
	tracker.Offset = newOffset
	tracker.LastReadTime = time.Now()
	tracker.EOF = resp.EOF

	// Create update
	update := &StreamUpdate{
		File:      tracker.Path,
		Content:   resp.Content,
		Offset:    tracker.Offset - resp.BytesRead,
		NewOffset: newOffset,
		BytesRead: resp.BytesRead,
		Timestamp: time.Now(),
		FileInfo:  resp.FileInfo,
	}

	log.Debugf("Read %d bytes from %s, offset %d -> %d",
		resp.BytesRead, tracker.Path, update.Offset, newOffset)

	return update, nil
}

// refreshFileList refreshes the list of event files
func (session *StreamSession) refreshFileList(reader *Reader) error {
	var files []*types.ContainerFileInfo
	var err error

	// 如果提供了精确的事件文件列表，直接使用它们
	if len(session.EventFiles) > 0 {
		log.Infof("Using provided event files list (%d files)", len(session.EventFiles))
		// 为每个文件获取文件信息
		for _, filePath := range session.EventFiles {
			// 这里简化处理，直接创建 FileTracker
			// 在实际读取时会获取真实的文件信息
			files = append(files, &types.ContainerFileInfo{
				Path:  filePath,
				IsDir: false,
			})
		}
	} else if session.LogDir != "" {
		// 否则从目录中发现文件
		log.Infof("Discovering event files from log_dir: %s", session.LogDir)
		files, err = reader.ListEventFiles(session.ctx, &LogReadRequest{
			WorkloadUID: session.WorkloadUID,
			PodUID:      session.PodUID,
			LogDir:      session.LogDir,
		})
		if err != nil {
			return err
		}
	} else {
		return fmt.Errorf("neither EventFiles nor LogDir provided")
	}

	session.fileMutex.Lock()
	defer session.fileMutex.Unlock()

	// Add new files
	for _, file := range files {
		if file.IsDir {
			continue
		}

		if _, exists := session.fileMap[file.Path]; !exists {
			tracker := &FileTracker{
				Path:         file.Path,
				Offset:       0,
				Size:         file.Size,
				LastModTime:  file.ModTime,
				LastReadTime: time.Time{},
				EOF:          false,
			}
			session.files = append(session.files, tracker)
			session.fileMap[file.Path] = tracker

			log.Infof("Added new file to stream: %s (size: %d bytes)", file.Path, file.Size)
		} else {
			// Update existing tracker
			tracker := session.fileMap[file.Path]
			tracker.Size = file.Size
			tracker.LastModTime = file.ModTime
		}
	}

	return nil
}

// readHistorical reads historical data from all files
func (session *StreamSession) readHistorical(reader *Reader, maxBytesPerFile int64) {
	log.Infof("Reading historical data for workload %s", session.WorkloadUID)

	session.fileMutex.RLock()
	files := make([]*FileTracker, len(session.files))
	copy(files, session.files)
	session.fileMutex.RUnlock()

	for _, tracker := range files {
		if session.ctx.Err() != nil {
			return
		}

		// Determine how much to read
		toRead := tracker.Size
		if maxBytesPerFile > 0 && toRead > maxBytesPerFile {
			// Read last N bytes (most recent data)
			tracker.Offset = tracker.Size - maxBytesPerFile
			toRead = maxBytesPerFile
		}

		// Read the data
		resp, err := reader.ReadFile(session.ctx, session.PodUID, tracker.Path, tracker.Offset, toRead)
		if err != nil {
			log.Errorf("Failed to read historical data from %s: %v", tracker.Path, err)
			continue
		}

		// Send update
		update := &StreamUpdate{
			File:      tracker.Path,
			Content:   resp.Content,
			Offset:    tracker.Offset,
			NewOffset: tracker.Offset + resp.BytesRead,
			BytesRead: resp.BytesRead,
			Timestamp: time.Now(),
			FileInfo:  resp.FileInfo,
		}

		session.sendUpdate(update)

		// Update tracker
		tracker.Offset += resp.BytesRead
		tracker.LastReadTime = time.Now()

		log.Debugf("Read %d bytes of historical data from %s", resp.BytesRead, tracker.Path)
	}

	log.Infof("Historical data reading completed for workload %s", session.WorkloadUID)
}

// GetState returns the current state for resumption
func (session *StreamSession) GetState() *StreamState {
	session.fileMutex.RLock()
	defer session.fileMutex.RUnlock()

	offsets := make(map[string]int64)
	for path, tracker := range session.fileMap {
		offsets[path] = tracker.Offset
	}

	return &StreamState{
		WorkloadUID: session.WorkloadUID,
		FileOffsets: offsets,
		LastUpdate:  session.lastUpdate,
		SessionID:   fmt.Sprintf("%s-%d", session.WorkloadUID, time.Now().Unix()),
	}
}

// restoreState restores streaming state
func (session *StreamSession) restoreState(state *StreamState) {
	session.fileMutex.Lock()
	defer session.fileMutex.Unlock()

	for path, offset := range state.FileOffsets {
		if tracker, exists := session.fileMap[path]; exists {
			tracker.Offset = offset
			log.Debugf("Restored offset for %s: %d", path, offset)
		}
	}

	session.lastUpdate = state.LastUpdate
	session.reconnectCnt++
}

// Updates returns the channel for receiving updates
func (session *StreamSession) Updates() <-chan *StreamUpdate {
	return session.updates
}

// Errors returns the channel for receiving errors
func (session *StreamSession) Errors() <-chan error {
	return session.errors
}

// Stop stops the streaming session
func (session *StreamSession) Stop() {
	if session.cancel != nil {
		session.cancel()
	}
}

// sendUpdate sends an update to the channel (non-blocking)
func (session *StreamSession) sendUpdate(update *StreamUpdate) {
	select {
	case session.updates <- update:
	case <-session.ctx.Done():
	default:
		log.Warnf("Update channel full, dropping update for %s", update.File)
	}
}

// sendError sends an error to the channel (non-blocking)
func (session *StreamSession) sendError(err error) {
	select {
	case session.errors <- err:
	default:
		log.Warnf("Error channel full, dropping error: %v", err)
	}
}

// OffsetManager manages read offsets for multiple workloads
type OffsetManager struct {
	offsets sync.Map // workloadUID -> map[filePath]int64
}

// NewOffsetManager creates a new offset manager
func NewOffsetManager() *OffsetManager {
	return &OffsetManager{}
}

// GetOffset retrieves the offset for a file
func (m *OffsetManager) GetOffset(workloadUID, filePath string) int64 {
	if workloadOffsets, ok := m.offsets.Load(workloadUID); ok {
		offsets := workloadOffsets.(map[string]int64)
		if offset, exists := offsets[filePath]; exists {
			return offset
		}
	}
	return 0
}

// SetOffset sets the offset for a file
func (m *OffsetManager) SetOffset(workloadUID, filePath string, offset int64) {
	var workloadOffsets map[string]int64

	if existing, ok := m.offsets.Load(workloadUID); ok {
		workloadOffsets = existing.(map[string]int64)
	} else {
		workloadOffsets = make(map[string]int64)
	}

	workloadOffsets[filePath] = offset
	m.offsets.Store(workloadUID, workloadOffsets)
}

// GetAllOffsets retrieves all offsets for a workload
func (m *OffsetManager) GetAllOffsets(workloadUID string) map[string]int64 {
	if workloadOffsets, ok := m.offsets.Load(workloadUID); ok {
		return workloadOffsets.(map[string]int64)
	}
	return make(map[string]int64)
}

// Clear clears offsets for a workload
func (m *OffsetManager) Clear(workloadUID string) {
	m.offsets.Delete(workloadUID)
}
