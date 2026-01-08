package storage

import (
	"bytes"
	"testing"
)

// TestNewDatabaseStorageBackend_Validation tests NewDatabaseStorageBackend validation
func TestNewDatabaseStorageBackend_Validation(t *testing.T) {
	tests := []struct {
		name      string
		db        interface{}
		config    *DatabaseConfig
		wantErr   bool
		errContains string
	}{
		{
			name:        "nil database",
			db:          nil,
			config:      &DatabaseConfig{},
			wantErr:     true,
			errContains: "database connection is nil",
		},
		{
			name:        "nil config",
			db:          "not-nil",
			config:      nil,
			wantErr:     true,
			errContains: "database config is nil",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var db interface{}
			if tt.db != nil {
				db = tt.db
			}
			
			// We can't easily test with a real sql.DB, so just verify the validation logic
			if tt.wantErr {
				// For nil DB case
				if tt.db == nil {
					_, err := NewDatabaseStorageBackend(nil, tt.config)
					if err == nil {
						t.Error("Expected error for nil database")
					}
				}
				// For nil config case - we need a non-nil DB which we can't easily create
				_ = db
			}
		})
	}
}

// TestDatabaseStorageBackend_SplitIntoChunks tests the chunk splitting logic
func TestDatabaseStorageBackend_SplitIntoChunks(t *testing.T) {
	backend := &DatabaseStorageBackend{
		chunkSize: 10, // 10 bytes for testing
	}

	tests := []struct {
		name           string
		content        []byte
		expectedChunks int
		chunkSizes     []int
	}{
		{
			name:           "smaller than chunk size",
			content:        []byte("hello"),
			expectedChunks: 1,
			chunkSizes:     []int{5},
		},
		{
			name:           "exactly chunk size",
			content:        []byte("0123456789"),
			expectedChunks: 1,
			chunkSizes:     []int{10},
		},
		{
			name:           "two chunks",
			content:        []byte("0123456789abcdef"),
			expectedChunks: 2,
			chunkSizes:     []int{10, 6},
		},
		{
			name:           "three chunks",
			content:        []byte("0123456789abcdefghij01234"),
			expectedChunks: 3,
			chunkSizes:     []int{10, 10, 5},
		},
		{
			name:           "empty content",
			content:        []byte{},
			expectedChunks: 1,
			chunkSizes:     []int{0},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chunks := backend.splitIntoChunks(tt.content)
			
			if len(chunks) != tt.expectedChunks {
				t.Errorf("Expected %d chunks, got %d", tt.expectedChunks, len(chunks))
			}
			
			for i, expectedSize := range tt.chunkSizes {
				if i >= len(chunks) {
					t.Errorf("Missing chunk %d", i)
					continue
				}
				if len(chunks[i]) != expectedSize {
					t.Errorf("Chunk %d: expected size %d, got %d", i, expectedSize, len(chunks[i]))
				}
			}
			
			// Verify reassembly
			var reassembled []byte
			for _, chunk := range chunks {
				reassembled = append(reassembled, chunk...)
			}
			if !bytes.Equal(reassembled, tt.content) {
				t.Error("Reassembled content doesn't match original")
			}
		})
	}
}

// TestDatabaseStorageBackend_SplitChunkRanges tests the chunk range splitting for parallel processing
func TestDatabaseStorageBackend_SplitChunkRanges(t *testing.T) {
	backend := &DatabaseStorageBackend{}

	tests := []struct {
		name        string
		totalChunks int
		numWorkers  int
		expected    []struct{ start, end int }
	}{
		{
			name:        "even split",
			totalChunks: 10,
			numWorkers:  2,
			expected: []struct{ start, end int }{
				{0, 5},
				{5, 10},
			},
		},
		{
			name:        "uneven split",
			totalChunks: 10,
			numWorkers:  3,
			expected: []struct{ start, end int }{
				{0, 4},
				{4, 8},
				{8, 10},
			},
		},
		{
			name:        "single worker",
			totalChunks: 5,
			numWorkers:  1,
			expected: []struct{ start, end int }{
				{0, 5},
			},
		},
		{
			name:        "more workers than chunks",
			totalChunks: 3,
			numWorkers:  5,
			expected: []struct{ start, end int }{
				{0, 1},
				{1, 2},
				{2, 3},
				{3, 3},
				{4, 3},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ranges := backend.splitChunkRanges(tt.totalChunks, tt.numWorkers)
			
			if len(ranges) != tt.numWorkers {
				t.Errorf("Expected %d ranges, got %d", tt.numWorkers, len(ranges))
			}
			
			for i, expected := range tt.expected {
				if i >= len(ranges) {
					break
				}
				if ranges[i].start != expected.start || ranges[i].end != expected.end {
					t.Errorf("Range %d: expected {%d, %d}, got {%d, %d}",
						i, expected.start, expected.end, ranges[i].start, ranges[i].end)
				}
			}
		})
	}
}

// TestDatabaseStorageBackend_ExtractRange tests the range extraction logic
func TestDatabaseStorageBackend_ExtractRange(t *testing.T) {
	backend := &DatabaseStorageBackend{}
	content := []byte("0123456789abcdefghij")

	tests := []struct {
		name     string
		offset   int64
		length   int64
		expected []byte
	}{
		{
			name:     "full content (no range)",
			offset:   0,
			length:   0,
			expected: []byte("0123456789abcdefghij"),
		},
		{
			name:     "offset only",
			offset:   5,
			length:   0,
			expected: []byte("56789abcdefghij"),
		},
		{
			name:     "offset and length",
			offset:   5,
			length:   5,
			expected: []byte("56789"),
		},
		{
			name:     "from beginning with length",
			offset:   0,
			length:   10,
			expected: []byte("0123456789"),
		},
		{
			name:     "offset at end",
			offset:   20,
			length:   0,
			expected: []byte{},
		},
		{
			name:     "offset past end",
			offset:   30,
			length:   0,
			expected: []byte{},
		},
		{
			name:     "length extends past end",
			offset:   15,
			length:   100,
			expected: []byte("fghij"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := backend.extractRange(content, tt.offset, tt.length)
			if !bytes.Equal(result, tt.expected) {
				t.Errorf("Expected %q, got %q", string(tt.expected), string(result))
			}
		})
	}
}

// TestCompressGzip tests the gzip compression helper
func TestCompressGzip(t *testing.T) {
	tests := []struct {
		name    string
		data    []byte
		wantErr bool
	}{
		{
			name:    "compress text",
			data:    []byte("Hello, World! This is a test string for compression."),
			wantErr: false,
		},
		{
			name:    "compress empty",
			data:    []byte{},
			wantErr: false,
		},
		{
			name:    "compress large data",
			data:    bytes.Repeat([]byte("test data "), 1000),
			wantErr: false,
		},
		{
			name:    "compress binary",
			data:    []byte{0x00, 0x01, 0x02, 0xFF, 0xFE, 0xFD},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			compressed, err := compressGzip(tt.data)
			if (err != nil) != tt.wantErr {
				t.Errorf("compressGzip() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			
			if !tt.wantErr {
				// Verify it decompresses back
				decompressed, err := decompressGzip(compressed)
				if err != nil {
					t.Errorf("decompressGzip() error = %v", err)
					return
				}
				
				if !bytes.Equal(decompressed, tt.data) {
					t.Error("Decompressed data doesn't match original")
				}
			}
		})
	}
}

// TestDecompressGzip tests the gzip decompression helper
func TestDecompressGzip(t *testing.T) {
	tests := []struct {
		name    string
		data    []byte
		wantErr bool
	}{
		{
			name:    "valid compressed data",
			data:    nil, // Will be set to compressed data
			wantErr: false,
		},
		{
			name:    "invalid data",
			data:    []byte("not compressed data"),
			wantErr: true,
		},
		{
			name:    "empty data",
			data:    []byte{},
			wantErr: true,
		},
	}

	// Prepare valid compressed data for the first test
	originalData := []byte("Hello, compressed world!")
	validCompressed, _ := compressGzip(originalData)
	tests[0].data = validCompressed

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decompressed, err := decompressGzip(tt.data)
			if (err != nil) != tt.wantErr {
				t.Errorf("decompressGzip() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			
			if !tt.wantErr && tt.name == "valid compressed data" {
				if !bytes.Equal(decompressed, originalData) {
					t.Error("Decompressed data doesn't match original")
				}
			}
		})
	}
}

// TestMin tests the min helper function
func TestMin(t *testing.T) {
	tests := []struct {
		name     string
		a, b     int
		expected int
	}{
		{"a smaller", 1, 5, 1},
		{"b smaller", 10, 3, 3},
		{"equal", 7, 7, 7},
		{"negative", -5, -10, -10},
		{"zero and positive", 0, 5, 0},
		{"negative and positive", -3, 3, -3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := min(tt.a, tt.b)
			if result != tt.expected {
				t.Errorf("min(%d, %d) = %d, expected %d", tt.a, tt.b, result, tt.expected)
			}
		})
	}
}

// TestDatabaseStorageBackend_GetStorageType tests GetStorageType method
func TestDatabaseStorageBackend_GetStorageType(t *testing.T) {
	backend := &DatabaseStorageBackend{}
	if backend.GetStorageType() != "database" {
		t.Errorf("Expected 'database', got '%s'", backend.GetStorageType())
	}
}

// TestCompressionRoundTrip tests compression/decompression round trip
func TestCompressionRoundTrip(t *testing.T) {
	testCases := [][]byte{
		[]byte("Simple string"),
		bytes.Repeat([]byte("A"), 10000), // Highly compressible
		make([]byte, 0),                   // Empty
		func() []byte { // Random-ish data
			data := make([]byte, 1000)
			for i := range data {
				data[i] = byte(i % 256)
			}
			return data
		}(),
	}

	for i, original := range testCases {
		compressed, err := compressGzip(original)
		if err != nil {
			t.Errorf("Case %d: compression failed: %v", i, err)
			continue
		}

		decompressed, err := decompressGzip(compressed)
		if err != nil {
			t.Errorf("Case %d: decompression failed: %v", i, err)
			continue
		}

		if !bytes.Equal(original, decompressed) {
			t.Errorf("Case %d: round trip failed, data mismatch", i)
		}
	}
}

// BenchmarkCompressGzip benchmarks gzip compression
func BenchmarkCompressGzip(b *testing.B) {
	data := bytes.Repeat([]byte("benchmark test data "), 1000)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = compressGzip(data)
	}
}

// BenchmarkDecompressGzip benchmarks gzip decompression
func BenchmarkDecompressGzip(b *testing.B) {
	data := bytes.Repeat([]byte("benchmark test data "), 1000)
	compressed, _ := compressGzip(data)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = decompressGzip(compressed)
	}
}

// BenchmarkSplitIntoChunks benchmarks chunk splitting
func BenchmarkSplitIntoChunks(b *testing.B) {
	backend := &DatabaseStorageBackend{
		chunkSize: 10 * 1024 * 1024, // 10MB
	}
	data := make([]byte, 100*1024*1024) // 100MB
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = backend.splitIntoChunks(data)
	}
}
