package storage

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/md5"
	"database/sql"
	"fmt"
	"sync"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
)

// DatabaseStorageBackend implements StorageBackend using PostgreSQL
type DatabaseStorageBackend struct {
	db                   *sql.DB
	compression          bool
	chunkSize            int64
	maxFileSize          int64
	maxConcurrentChunks  int
}

// NewDatabaseStorageBackend creates a new database storage backend
func NewDatabaseStorageBackend(db *sql.DB, config *DatabaseConfig) (*DatabaseStorageBackend, error) {
	if db == nil {
		return nil, fmt.Errorf("database connection is nil")
	}
	if config == nil {
		return nil, fmt.Errorf("database config is nil")
	}

	// Set defaults
	if config.ChunkSize == 0 {
		config.ChunkSize = 10 * 1024 * 1024 // 10MB default
	}
	if config.MaxFileSize == 0 {
		config.MaxFileSize = 200 * 1024 * 1024 // 200MB default
	}
	if config.MaxConcurrentChunks == 0 {
		config.MaxConcurrentChunks = 5
	}

	backend := &DatabaseStorageBackend{
		db:                  db,
		compression:         config.Compression,
		chunkSize:           config.ChunkSize,
		maxFileSize:         config.MaxFileSize,
		maxConcurrentChunks: config.MaxConcurrentChunks,
	}

	log.Infof("Initialized database storage backend: compression=%v, chunk_size=%d bytes, max_file_size=%d bytes, max_concurrent=%d",
		config.Compression, config.ChunkSize, config.MaxFileSize, config.MaxConcurrentChunks)

	return backend, nil
}

// Store stores a file to database
func (b *DatabaseStorageBackend) Store(ctx context.Context, req *StoreRequest) (*StoreResponse, error) {
	// Check file size
	if int64(len(req.Content)) > b.maxFileSize {
		return nil, fmt.Errorf("file too large: %d bytes (max: %d bytes)", len(req.Content), b.maxFileSize)
	}

	content := req.Content
	compressed := req.Compressed
	originalSize := int64(len(content))

	// Compress content if enabled and not already compressed
	if b.compression && !req.Compressed {
		compressedData, err := compressGzip(content)
		if err != nil {
			return nil, fmt.Errorf("compression failed: %w", err)
		}
		content = compressedData
		compressed = true
		log.Debugf("Compressed file from %d to %d bytes (%.1f%%)",
			originalSize, len(content), float64(len(content))*100/float64(originalSize))
	}

	// Calculate MD5 (use original content)
	md5Hash := fmt.Sprintf("%x", md5.Sum(req.Content))

	// Split into chunks
	chunks := b.splitIntoChunks(content)
	totalChunks := len(chunks)

	log.Infof("Storing file %s: size=%d, chunks=%d, chunk_size=%d, compressed=%v",
		req.FileName, len(content), totalChunks, b.chunkSize, compressed)

	// Begin transaction
	tx, err := b.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Step 1: Insert file metadata into profiler_files and get the generated ID
	var profilerFileID int64
	err = tx.QueryRowContext(ctx, `
		INSERT INTO profiler_files 
		(workload_uid, pod_uid, pod_name, pod_namespace, file_name, file_path, file_type, file_size, storage_type, confidence, metadata)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 'database', $9, $10)
		RETURNING id
	`,
		req.WorkloadUID,
		req.Metadata["pod_uid"],
		req.Metadata["pod_name"],
		req.Metadata["pod_namespace"],
		req.FileName,
		req.Metadata["file_path"],
		req.FileType,
		originalSize,
		req.Metadata["confidence"],
		"{}",
	).Scan(&profilerFileID)

	if err != nil {
		return nil, fmt.Errorf("failed to insert file metadata: %w", err)
	}

	log.Debugf("Created profiler_files record with ID: %d", profilerFileID)

	// Determine encoding
	encoding := "none"
	if compressed {
		encoding = "gzip"
	}

	// Step 2: Store chunks using the integer profiler_file_id
	if totalChunks > 1 && b.maxConcurrentChunks > 1 {
		err = b.storeChunksParallel(ctx, tx, profilerFileID, chunks, encoding, md5Hash)
	} else {
		err = b.storeChunksSequential(ctx, tx, profilerFileID, chunks, encoding, md5Hash)
	}

	if err != nil {
		return nil, err
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	// Use the integer ID as storage path
	storagePath := fmt.Sprintf("%d", profilerFileID)

	return &StoreResponse{
		FileID:      req.FileID,
		StoragePath: storagePath,
		StorageType: "database",
		Size:        originalSize,
		MD5:         md5Hash,
		Metadata: map[string]interface{}{
			"compressed":       compressed,
			"chunks":           totalChunks,
			"chunk_size":       b.chunkSize,
			"stored_size":      len(content),
			"compress_ratio":   float64(len(content)) / float64(originalSize),
			"profiler_file_id": profilerFileID,
		},
	}, nil
}

// storeChunksSequential stores chunks sequentially
func (b *DatabaseStorageBackend) storeChunksSequential(
	ctx context.Context,
	tx *sql.Tx,
	profilerFileID int64,
	chunks [][]byte,
	encoding string,
	md5Hash string,
) error {
	for i, chunk := range chunks {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO profiler_file_content 
			(profiler_file_id, content, content_encoding, chunk_index, total_chunks, chunk_size, md5_hash)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
		`, profilerFileID, chunk, encoding, i, len(chunks), len(chunk), md5Hash)

		if err != nil {
			return fmt.Errorf("failed to store chunk %d: %w", i, err)
		}

		log.Debugf("Stored chunk %d/%d (%d bytes)", i+1, len(chunks), len(chunk))
	}
	return nil
}

// storeChunksParallel stores chunks in parallel
func (b *DatabaseStorageBackend) storeChunksParallel(
	ctx context.Context,
	tx *sql.Tx,
	profilerFileID int64,
	chunks [][]byte,
	encoding string,
	md5Hash string,
) error {
	type chunkJob struct {
		index int
		data  []byte
	}

	totalChunks := len(chunks)
	jobs := make(chan chunkJob, totalChunks)
	errors := make(chan error, totalChunks)

	// Worker pool
	numWorkers := min(b.maxConcurrentChunks, totalChunks)
	var wg sync.WaitGroup

	// Start workers
	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for job := range jobs {
				_, err := tx.ExecContext(ctx, `
					INSERT INTO profiler_file_content 
					(profiler_file_id, content, content_encoding, chunk_index, total_chunks, chunk_size, md5_hash)
					VALUES ($1, $2, $3, $4, $5, $6, $7)
				`, profilerFileID, job.data, encoding, job.index, totalChunks, len(job.data), md5Hash)

				if err != nil {
					errors <- fmt.Errorf("worker %d failed on chunk %d: %w", workerID, job.index, err)
					return
				}

				log.Debugf("Worker %d stored chunk %d/%d (%d bytes)",
					workerID, job.index+1, totalChunks, len(job.data))
			}
		}(w)
	}

	// Send jobs
	for i, chunk := range chunks {
		jobs <- chunkJob{index: i, data: chunk}
	}
	close(jobs)

	// Wait for completion
	wg.Wait()
	close(errors)

	// Check for errors
	if len(errors) > 0 {
		return <-errors
	}

	return nil
}

// Retrieve retrieves a file from database
func (b *DatabaseStorageBackend) Retrieve(ctx context.Context, req *RetrieveRequest) (*RetrieveResponse, error) {
	// Parse storage path as integer (profiler_file_id)
	var profilerFileID int64
	if req.StoragePath != "" {
		if _, err := fmt.Sscanf(req.StoragePath, "%d", &profilerFileID); err != nil {
			return nil, fmt.Errorf("invalid storage path (expected integer): %s", req.StoragePath)
		}
	} else {
		return nil, fmt.Errorf("storage path is required for database retrieval")
	}

	// Query file info (get total chunks)
	var totalChunks int
	var encoding string
	err := b.db.QueryRowContext(ctx, `
		SELECT COUNT(*), content_encoding
		FROM profiler_file_content
		WHERE profiler_file_id = $1
		GROUP BY content_encoding
	`, profilerFileID).Scan(&totalChunks, &encoding)

	if err != nil {
		return nil, fmt.Errorf("file not found: %w", err)
	}

	compressed := (encoding == "gzip")

	// Choose reading strategy based on chunk count
	var fullContent []byte

	if totalChunks == 1 {
		// Single chunk file
		fullContent, err = b.retrieveSingleChunk(ctx, profilerFileID)
	} else if totalChunks <= 5 {
		// Few chunks, sequential read
		fullContent, err = b.retrieveChunksSequential(ctx, profilerFileID, totalChunks)
	} else {
		// Many chunks, parallel read
		fullContent, err = b.retrieveChunksParallel(ctx, profilerFileID, totalChunks)
	}

	if err != nil {
		return nil, err
	}

	originalSize := int64(len(fullContent))

	// Decompress if needed
	if compressed {
		decompressed, err := decompressGzip(fullContent)
		if err != nil {
			return nil, fmt.Errorf("decompression failed: %w", err)
		}
		fullContent = decompressed
		log.Debugf("Decompressed file from %d to %d bytes", originalSize, len(fullContent))
	}

	// Apply range if specified
	if req.Offset > 0 || req.Length > 0 {
		fullContent = b.extractRange(fullContent, req.Offset, req.Length)
	}

	// Calculate MD5
	md5Hash := fmt.Sprintf("%x", md5.Sum(fullContent))

	return &RetrieveResponse{
		Content:    fullContent,
		Size:       int64(len(fullContent)),
		Compressed: false,
		MD5:        md5Hash,
	}, nil
}

// retrieveSingleChunk retrieves a single chunk
func (b *DatabaseStorageBackend) retrieveSingleChunk(ctx context.Context, profilerFileID int64) ([]byte, error) {
	var content []byte
	err := b.db.QueryRowContext(ctx, `
		SELECT content 
		FROM profiler_file_content
		WHERE profiler_file_id = $1 AND chunk_index = 0
	`, profilerFileID).Scan(&content)

	return content, err
}

// retrieveChunksSequential retrieves chunks sequentially
func (b *DatabaseStorageBackend) retrieveChunksSequential(
	ctx context.Context,
	profilerFileID int64,
	totalChunks int,
) ([]byte, error) {
	rows, err := b.db.QueryContext(ctx, `
		SELECT content, chunk_index
		FROM profiler_file_content
		WHERE profiler_file_id = $1
		ORDER BY chunk_index
	`, profilerFileID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	// Pre-allocate capacity
	fullContent := make([]byte, 0, totalChunks*int(b.chunkSize))

	for rows.Next() {
		var chunk []byte
		var chunkIdx int
		if err := rows.Scan(&chunk, &chunkIdx); err != nil {
			return nil, err
		}
		fullContent = append(fullContent, chunk...)
		log.Debugf("Retrieved chunk %d/%d (%d bytes)", chunkIdx+1, totalChunks, len(chunk))
	}

	return fullContent, nil
}

// retrieveChunksParallel retrieves chunks in parallel
func (b *DatabaseStorageBackend) retrieveChunksParallel(
	ctx context.Context,
	profilerFileID int64,
	totalChunks int,
) ([]byte, error) {
	type chunkResult struct {
		index int
		data  []byte
		err   error
	}

	results := make(chan chunkResult, totalChunks)
	numWorkers := min(b.maxConcurrentChunks, totalChunks)

	// Worker pool
	var wg sync.WaitGroup
	chunks := make([][]byte, totalChunks)

	// Split chunk ranges for workers
	chunkRanges := b.splitChunkRanges(totalChunks, numWorkers)

	for workerID, chunkRange := range chunkRanges {
		wg.Add(1)
		go func(wid int, start, end int) {
			defer wg.Done()

			// Each worker reads a batch of chunks
			rows, err := b.db.QueryContext(ctx, `
				SELECT content, chunk_index
				FROM profiler_file_content
				WHERE profiler_file_id = $1 AND chunk_index >= $2 AND chunk_index < $3
				ORDER BY chunk_index
			`, profilerFileID, start, end)

			if err != nil {
				results <- chunkResult{err: err}
				return
			}
			defer rows.Close()

			for rows.Next() {
				var chunk []byte
				var idx int
				if err := rows.Scan(&chunk, &idx); err != nil {
					results <- chunkResult{err: err}
					return
				}
				results <- chunkResult{index: idx, data: chunk}
				log.Debugf("Worker %d retrieved chunk %d (%d bytes)", wid, idx, len(chunk))
			}
		}(workerID, chunkRange.start, chunkRange.end)
	}

	// Collect results
	go func() {
		wg.Wait()
		close(results)
	}()

	// Assemble results
	for result := range results {
		if result.err != nil {
			return nil, result.err
		}
		chunks[result.index] = result.data
	}

	// Merge all chunks
	totalSize := 0
	for _, chunk := range chunks {
		totalSize += len(chunk)
	}

	fullContent := make([]byte, 0, totalSize)
	for i, chunk := range chunks {
		if chunk == nil {
			return nil, fmt.Errorf("missing chunk %d", i)
		}
		fullContent = append(fullContent, chunk...)
	}

	return fullContent, nil
}

// Delete deletes a file from database
func (b *DatabaseStorageBackend) Delete(ctx context.Context, fileID string) error {
	// Parse fileID as integer (it's the profiler_file_id from profiler_files table)
	var profilerFileID int64
	if _, err := fmt.Sscanf(fileID, "%d", &profilerFileID); err != nil {
		return fmt.Errorf("invalid file ID (expected integer): %s", fileID)
	}

	// Delete chunks first
	result, err := b.db.ExecContext(ctx, `
		DELETE FROM profiler_file_content
		WHERE profiler_file_id = $1
	`, profilerFileID)

	if err != nil {
		return fmt.Errorf("failed to delete file chunks: %w", err)
	}

	chunksDeleted, _ := result.RowsAffected()

	// Delete file metadata
	_, err = b.db.ExecContext(ctx, `
		DELETE FROM profiler_files
		WHERE id = $1
	`, profilerFileID)

	if err != nil {
		return fmt.Errorf("failed to delete file metadata: %w", err)
	}

	log.Infof("Deleted file from database: file_id=%d, chunks=%d", profilerFileID, chunksDeleted)

	return nil
}

// GenerateDownloadURL generates a download URL (API endpoint for database storage)
func (b *DatabaseStorageBackend) GenerateDownloadURL(ctx context.Context, fileID string, expires time.Duration) (string, error) {
	// Database storage doesn't support presigned URLs, return direct download API
	return fmt.Sprintf("/api/v1/profiler/files/%s/download", fileID), nil
}

// GetStorageType returns the storage type identifier
func (b *DatabaseStorageBackend) GetStorageType() string {
	return "database"
}

// Exists checks if a file exists in database
func (b *DatabaseStorageBackend) Exists(ctx context.Context, fileID string) (bool, error) {
	// Parse fileID as integer
	var profilerFileID int64
	if _, err := fmt.Sscanf(fileID, "%d", &profilerFileID); err != nil {
		return false, fmt.Errorf("invalid file ID (expected integer): %s", fileID)
	}

	var count int
	err := b.db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM profiler_file_content
		WHERE profiler_file_id = $1
	`, profilerFileID).Scan(&count)

	if err != nil {
		return false, fmt.Errorf("failed to check file existence: %w", err)
	}

	return count > 0, nil
}

// ExistsByWorkloadAndFilename checks if a file with the same name already exists for the workload
func (b *DatabaseStorageBackend) ExistsByWorkloadAndFilename(ctx context.Context, workloadUID string, fileName string) (bool, error) {
	var count int
	err := b.db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM profiler_files
		WHERE workload_uid = $1 AND file_name = $2
	`, workloadUID, fileName).Scan(&count)

	if err != nil {
		return false, fmt.Errorf("failed to check file existence: %w", err)
	}

	return count > 0, nil
}

// Helper methods

func (b *DatabaseStorageBackend) splitIntoChunks(content []byte) [][]byte {
	size := int64(len(content))

	// Small file, no chunking
	if size <= b.chunkSize {
		return [][]byte{content}
	}

	// Calculate number of chunks
	numChunks := int((size + b.chunkSize - 1) / b.chunkSize)
	chunks := make([][]byte, 0, numChunks)

	for i := int64(0); i < size; i += b.chunkSize {
		end := i + b.chunkSize
		if end > size {
			end = size
		}
		chunks = append(chunks, content[i:end])
	}

	return chunks
}

func (b *DatabaseStorageBackend) splitChunkRanges(totalChunks, numWorkers int) []struct{ start, end int } {
	ranges := make([]struct{ start, end int }, numWorkers)
	chunkPerWorker := (totalChunks + numWorkers - 1) / numWorkers

	for i := 0; i < numWorkers; i++ {
		start := i * chunkPerWorker
		end := min((i+1)*chunkPerWorker, totalChunks)
		ranges[i] = struct{ start, end int }{start, end}
	}

	return ranges
}

func (b *DatabaseStorageBackend) extractRange(content []byte, offset, length int64) []byte {
	size := int64(len(content))

	if offset >= size {
		return []byte{}
	}

	end := size
	if length > 0 && offset+length < size {
		end = offset + length
	}

	return content[offset:end]
}

// Compression helpers

func compressGzip(data []byte) ([]byte, error) {
	var compressed bytes.Buffer
	gzWriter := gzip.NewWriter(&compressed)

	if _, err := gzWriter.Write(data); err != nil {
		return nil, err
	}

	if err := gzWriter.Close(); err != nil {
		return nil, err
	}

	return compressed.Bytes(), nil
}

func decompressGzip(data []byte) ([]byte, error) {
	gzReader, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer gzReader.Close()

	var decompressed bytes.Buffer
	if _, err := decompressed.ReadFrom(gzReader); err != nil {
		return nil, err
	}

	return decompressed.Bytes(), nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

