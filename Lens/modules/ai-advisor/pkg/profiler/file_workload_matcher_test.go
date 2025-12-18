package profiler

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// ExtractTimestampFromFilename Tests
// ============================================================================

func TestExtractTimestampFromFilename(t *testing.T) {
	tests := []struct {
		name     string
		filename string
		wantTs   int64
		wantErr  bool
	}{
		{
			name:     "valid primus format",
			filename: "primus-megatron-exp[test-exp]-rank[0].1702345678901234567.pt.trace.json",
			wantTs:   1702345678901234567,
			wantErr:  false,
		},
		{
			name:     "valid primus format with gzip",
			filename: "primus-megatron-exp[llm-train]-rank[1].1702345678901234567.pt.trace.json.gz",
			wantTs:   1702345678901234567,
			wantErr:  false,
		},
		{
			name:     "different timestamp",
			filename: "primus-megatron-exp[my-exp]-rank[0].1609459200000000000.pt.trace.json",
			wantTs:   1609459200000000000,
			wantErr:  false,
		},
		{
			name:     "full path with timestamp",
			filename: "/output/tensorboard/primus-megatron-exp[exp1]-rank[0].1702345678901234567.pt.trace.json",
			wantTs:   1702345678901234567,
			wantErr:  false,
		},
		{
			name:     "kineto format (fallback)",
			filename: "kineto.1702345678901234567.json",
			wantTs:   1702345678901234567,
			wantErr:  false,
		},
		{
			name:     "no timestamp in filename",
			filename: "profiler_output.json",
			wantTs:   0,
			wantErr:  true,
		},
		{
			name:     "short timestamp (not nanoseconds)",
			filename: "trace.1702345678.json",
			wantTs:   0,
			wantErr:  true,
		},
		{
			name:     "empty filename",
			filename: "",
			wantTs:   0,
			wantErr:  true,
		},
		{
			name:     "timestamp too short",
			filename: "trace.12345.pt.trace.json",
			wantTs:   0,
			wantErr:  true,
		},
		{
			name:     "timestamp with letters",
			filename: "trace.abc1702345678901234567.pt.trace.json",
			wantTs:   0,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ts, err := ExtractTimestampFromFilename(tt.filename)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Equal(t, int64(0), ts)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.wantTs, ts)
			}
		})
	}
}

// ============================================================================
// TimestampToTime Tests
// ============================================================================

func TestTimestampToTime(t *testing.T) {
	tests := []struct {
		name        string
		timestampNs int64
		wantYear    int
		wantMonth   time.Month
		// Note: Day is not checked due to timezone differences
	}{
		{
			name:        "specific timestamp",
			timestampNs: 1702345678901234567,
			wantYear:    2023,
			wantMonth:   time.December,
		},
		{
			name:        "epoch start",
			timestampNs: 0,
			wantYear:    1970,
			wantMonth:   time.January,
		},
		{
			name:        "2021 timestamp",
			timestampNs: 1609459200000000000, // 2021-01-01 00:00:00 UTC
			wantYear:    2021,
			wantMonth:   time.January,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := TimestampToTime(tt.timestampNs)
			assert.Equal(t, tt.wantYear, result.Year())
			assert.Equal(t, tt.wantMonth, result.Month())
			// Skip day check due to timezone differences between test environments
		})
	}
}

// ============================================================================
// compareConfidence Tests
// ============================================================================

func TestCompareConfidence(t *testing.T) {
	tests := []struct {
		name     string
		a        string
		b        string
		expected int
	}{
		{
			name:     "high vs high",
			a:        "high",
			b:        "high",
			expected: 0,
		},
		{
			name:     "high vs medium",
			a:        "high",
			b:        "medium",
			expected: 1,
		},
		{
			name:     "high vs low",
			a:        "high",
			b:        "low",
			expected: 1,
		},
		{
			name:     "medium vs high",
			a:        "medium",
			b:        "high",
			expected: -1,
		},
		{
			name:     "medium vs medium",
			a:        "medium",
			b:        "medium",
			expected: 0,
		},
		{
			name:     "medium vs low",
			a:        "medium",
			b:        "low",
			expected: 1,
		},
		{
			name:     "low vs high",
			a:        "low",
			b:        "high",
			expected: -1,
		},
		{
			name:     "low vs medium",
			a:        "low",
			b:        "medium",
			expected: -1,
		},
		{
			name:     "low vs low",
			a:        "low",
			b:        "low",
			expected: 0,
		},
		{
			name:     "unknown vs high",
			a:        "unknown",
			b:        "high",
			expected: -1,
		},
		{
			name:     "unknown vs unknown",
			a:        "unknown",
			b:        "unknown",
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := compareConfidence(tt.a, tt.b)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// ============================================================================
// FileMatchResult Tests
// ============================================================================

func TestFileMatchResult_GetAllMatchedWorkloadUIDs(t *testing.T) {
	tests := []struct {
		name     string
		result   *FileMatchResult
		expected []string
	}{
		{
			name: "multiple matches",
			result: &FileMatchResult{
				Matches: []FileWorkloadMatch{
					{WorkloadUID: "uid-1"},
					{WorkloadUID: "uid-2"},
					{WorkloadUID: "uid-3"},
				},
			},
			expected: []string{"uid-1", "uid-2", "uid-3"},
		},
		{
			name: "single match",
			result: &FileMatchResult{
				Matches: []FileWorkloadMatch{
					{WorkloadUID: "uid-1"},
				},
			},
			expected: []string{"uid-1"},
		},
		{
			name: "no matches",
			result: &FileMatchResult{
				Matches: []FileWorkloadMatch{},
			},
			expected: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			uids := tt.result.GetAllMatchedWorkloadUIDs()
			assert.Equal(t, tt.expected, uids)
		})
	}
}

func TestFileMatchResult_GetConfidence(t *testing.T) {
	tests := []struct {
		name     string
		result   *FileMatchResult
		expected string
	}{
		{
			name: "has conflict returns low",
			result: &FileMatchResult{
				HasConflict: true,
				PrimaryMatch: &FileWorkloadMatch{
					Confidence: "high",
				},
			},
			expected: "low",
		},
		{
			name: "no conflict with high confidence primary",
			result: &FileMatchResult{
				HasConflict: false,
				PrimaryMatch: &FileWorkloadMatch{
					Confidence: "high",
				},
			},
			expected: "high",
		},
		{
			name: "no conflict with medium confidence primary",
			result: &FileMatchResult{
				HasConflict: false,
				PrimaryMatch: &FileWorkloadMatch{
					Confidence: "medium",
				},
			},
			expected: "medium",
		},
		{
			name: "no primary match returns none",
			result: &FileMatchResult{
				HasConflict:  false,
				PrimaryMatch: nil,
			},
			expected: "none",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			confidence := tt.result.GetConfidence()
			assert.Equal(t, tt.expected, confidence)
		})
	}
}

// ============================================================================
// NewFileWorkloadMatcher Tests
// ============================================================================

func TestNewFileWorkloadMatcher(t *testing.T) {
	// This test verifies the constructor works
	// Note: In actual environment, this requires database connection
	// For unit testing, we can skip or mock
	t.Skip("Requires database connection - integration test")

	matcher := NewFileWorkloadMatcher()
	assert.NotNil(t, matcher)
}

// ============================================================================
// FileWorkloadMatch Tests
// ============================================================================

func TestFileWorkloadMatch_Fields(t *testing.T) {
	now := time.Now()
	match := FileWorkloadMatch{
		WorkloadUID:  "test-uid",
		WorkloadName: "test-workload",
		Namespace:    "default",
		Confidence:   "high",
		MatchReason:  "File created during workload execution",
		CreatedAt:    now,
		EndAt:        now.Add(1 * time.Hour),
	}

	assert.Equal(t, "test-uid", match.WorkloadUID)
	assert.Equal(t, "test-workload", match.WorkloadName)
	assert.Equal(t, "default", match.Namespace)
	assert.Equal(t, "high", match.Confidence)
	assert.NotEmpty(t, match.MatchReason)
	assert.Equal(t, now, match.CreatedAt)
	assert.Equal(t, now.Add(1*time.Hour), match.EndAt)
}

// ============================================================================
// FileMatchResult Fields Tests
// ============================================================================

func TestFileMatchResult_Fields(t *testing.T) {
	now := time.Now()
	result := FileMatchResult{
		FilePath:      "/output/trace.json",
		FileName:      "trace.json",
		FileTimestamp: 1702345678901234567,
		FileTime:      now,
		Matches: []FileWorkloadMatch{
			{WorkloadUID: "uid-1", Confidence: "high"},
		},
		PrimaryMatch:   &FileWorkloadMatch{WorkloadUID: "uid-1"},
		HasConflict:    false,
		ConflictReason: "",
	}

	assert.Equal(t, "/output/trace.json", result.FilePath)
	assert.Equal(t, "trace.json", result.FileName)
	assert.Equal(t, int64(1702345678901234567), result.FileTimestamp)
	assert.Equal(t, now, result.FileTime)
	assert.Len(t, result.Matches, 1)
	assert.NotNil(t, result.PrimaryMatch)
	assert.False(t, result.HasConflict)
	assert.Empty(t, result.ConflictReason)
}

// ============================================================================
// Edge Cases and Error Handling Tests
// ============================================================================

func TestExtractTimestampFromFilename_EdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		filename string
		wantErr  bool
	}{
		{
			name:     "only extension",
			filename: ".json",
			wantErr:  true,
		},
		{
			name:     "dots only",
			filename: "...",
			wantErr:  true,
		},
		{
			name:     "spaces in filename",
			filename: "trace file.1702345678901234567.pt.trace.json",
			wantErr:  false, // Should still find the timestamp
		},
		{
			name:     "multiple timestamps takes first",
			filename: "trace.1702345678901234567.1609459200000000000.pt.trace.json",
			wantErr:  false,
		},
		{
			name:     "timestamp at start",
			filename: "1702345678901234567.pt.trace.json",
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ExtractTimestampFromFilename(tt.filename)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// ============================================================================
// Timestamp Regex Tests
// ============================================================================

func TestTimestampRegex(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		shouldMatch bool
	}{
		{
			name:        "valid format",
			input:       ".1702345678901234567.pt.trace.json",
			shouldMatch: true,
		},
		{
			name:        "with gzip",
			input:       ".1702345678901234567.pt.trace.json.gz",
			shouldMatch: true,
		},
		{
			name:        "full filename",
			input:       "primus-megatron-exp[test]-rank[0].1702345678901234567.pt.trace.json",
			shouldMatch: true,
		},
		{
			name:        "short timestamp",
			input:       ".12345678901234567.pt.trace.json", // 17 digits, not 19+
			shouldMatch: false,
		},
		{
			name:        "wrong extension",
			input:       ".1702345678901234567.pt.trace.txt",
			shouldMatch: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matches := timestampRegex.FindStringSubmatch(tt.input)
			if tt.shouldMatch {
				assert.NotEmpty(t, matches)
				require.Len(t, matches, 2)
				assert.Len(t, matches[1], 19) // 19 digit timestamp
			} else {
				assert.Empty(t, matches)
			}
		})
	}
}

// ============================================================================
// Benchmark Tests
// ============================================================================

func BenchmarkExtractTimestampFromFilename(b *testing.B) {
	filename := "primus-megatron-exp[llm-training]-rank[0].1702345678901234567.pt.trace.json"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = ExtractTimestampFromFilename(filename)
	}
}

func BenchmarkCompareConfidence(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = compareConfidence("high", "medium")
	}
}

func BenchmarkTimestampToTime(b *testing.B) {
	ts := int64(1702345678901234567)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = TimestampToTime(ts)
	}
}
