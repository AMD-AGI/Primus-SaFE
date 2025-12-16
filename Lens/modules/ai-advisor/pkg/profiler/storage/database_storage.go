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

	// Determine encoding
	encoding := "none"
	if compressed {
		encoding = "gzip"
	}

	// Store chunks (parallel or sequential)
	if totalChunks > 1 && b.maxConcurrentChunks > 1 {
		err = b.storeChunksParallel(ctx, tx, req.FileID, chunks, encoding, md5Hash)
	} else {
		err = b.storeChunksSequential(ctx, tx, req.FileID, chunks, encoding, md5Hash)
	}

	if err != nil {
		return nil, err
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return &StoreResponse{
		FileID:      req.FileID,
		StoragePath: req.FileID, // Use file_id as storage path for database
		StorageType: "database",
		Size:        originalSize,
		MD5:         md5Hash,
		Metadata: map[string]interface{}{
			"compressed":     compressed,
			"chunks":         totalChunks,
			"chunk_size":     b.chunkSize,
			"stored_size":    len(content),
			"compress_ratio": float64(len(content)) / float64(originalSize),
		},
	}, nil
}

// storeChunksSequential stores chunks sequentially
func (b *DatabaseStorageBackend) storeChunksSequential(
	ctx context.Context,
	tx *sql.Tx,
	fileID string,
	chunks [][]byte,
	encoding string,
	md5Hash string,
) error {
	for i, chunk := range chunks {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO profiler_file_content 
			(profiler_file_id, content, content_encoding, chunk_index, total_chunks, chunk_size, md5_hash)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
		`, fileID, chunk, encoding, i, len(chunks), len(chunk), md5Hash)

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
	fileID string,
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
				`, fileID, job.data, encoding, job.index, totalChunks, len(job.data), md5Hash)

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
	// Query file info (get total chunks)
	var totalChunks int
	var encoding string
	err := b.db.QueryRowContext(ctx, `
		SELECT COUNT(*), content_encoding
		FROM profiler_file_content
		WHERE profiler_file_id = $1
		GROUP BY content_encoding
	`, req.FileID).Scan(&totalChunks, &encoding)

	if err != nil {
		return nil, fmt.Errorf("file not found: %w", err)
	}

	compressed := (encoding == "gzip")

	// Choose reading strategy based on chunk count
	var fullContent []byte

	if totalChunks == 1 {
		// Single chunk file
		fullContent, err = b.retrieveSingleChunk(ctx, req.FileID)
	} else if totalChunks <= 5 {
		// Few chunks, sequential read
		fullContent, err = b.retrieveChunksSequential(ctx, req.FileID, totalChunks)
	} else {
		// Many chunks, parallel read
		fullContent, err = b.retrieveChunksParallel(ctx, req.FileID, totalChunks)
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
func (b *DatabaseStorageBackend) retrieveSingleChunk(ctx context.Context, fileID string) ([]byte, error) {
	var content []byte
	err := b.db.QueryRowContext(ctx, `
		SELECT content 
		FROM profiler_file_content
		WHERE profiler_file_id = $1 AND chunk_index = 0
	`, fileID).Scan(&content)

	return content, err
}

// retrieveChunksSequential retrieves chunks sequentially
func (b *DatabaseStorageBackend) retrieveChunksSequential(
	ctx context.Context,
	fileID string,
	totalChunks int,
) ([]byte, error) {
	rows, err := b.db.QueryContext(ctx, `
		SELECT content, chunk_index
		FROM profiler_file_content
		WHERE profiler_file_id = $1
		ORDER BY chunk_index
	`, fileID)
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
	fileID string,
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
			`, fileID, start, end)

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
	// Delete chunks (CASCADE should handle this, but explicit is safer)
	result, err := b.db.ExecContext(ctx, `
		DELETE FROM profiler_file_content
		WHERE profiler_file_id = $1
	`, fileID)

	if err != nil {
		return fmt.Errorf("failed to delete file chunks: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	log.Infof("Deleted file from database: file_id=%s, chunks=%d", fileID, rowsAffected)

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
	var count int
	err := b.db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM profiler_file_content
		WHERE profiler_file_id = $1
	`, fileID).Scan(&count)

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

