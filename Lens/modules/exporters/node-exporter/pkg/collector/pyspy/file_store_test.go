package pyspy

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/config"
)

func TestNewFileStore(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := &config.PySpyConfig{
		Enabled:           true,
		StoragePath:       tmpDir,
		FileRetentionDays: 7,
	}

	fs, err := NewFileStore(cfg)
	if err != nil {
		t.Fatalf("Failed to create file store: %v", err)
	}

	if fs == nil {
		t.Fatal("NewFileStore returned nil")
	}

	// Check that profiles directory was created
	profilesDir := filepath.Join(tmpDir, "profiles")
	if _, err := os.Stat(profilesDir); os.IsNotExist(err) {
		t.Error("Profiles directory was not created")
	}
}

func TestPrepareOutputFile(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := &config.PySpyConfig{
		Enabled:           true,
		StoragePath:       tmpDir,
		FileRetentionDays: 7,
	}

	fs, _ := NewFileStore(cfg)

	tests := []struct {
		taskID   string
		format   OutputFormat
		expected string
	}{
		{"task-123", FormatFlamegraph, filepath.Join(tmpDir, "profiles", "task-123", "profile.svg")},
		{"task-456", FormatSpeedscope, filepath.Join(tmpDir, "profiles", "task-456", "profile.json")},
		{"task-789", FormatRaw, filepath.Join(tmpDir, "profiles", "task-789", "profile.txt")},
	}

	for _, tc := range tests {
		filePath, err := fs.PrepareOutputFile(tc.taskID, tc.format)
		if err != nil {
			t.Errorf("PrepareOutputFile failed: %v", err)
			continue
		}

		if filePath != tc.expected {
			t.Errorf("Expected %s, got %s", tc.expected, filePath)
		}

		// Check that task directory was created
		taskDir := filepath.Dir(filePath)
		if _, err := os.Stat(taskDir); os.IsNotExist(err) {
			t.Errorf("Task directory was not created: %s", taskDir)
		}
	}
}

func TestRegisterAndGetFile(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := &config.PySpyConfig{
		Enabled:           true,
		StoragePath:       tmpDir,
		FileRetentionDays: 7,
	}

	fs, _ := NewFileStore(cfg)

	// Prepare output file
	taskID := "test-task-123"
	filePath, _ := fs.PrepareOutputFile(taskID, FormatFlamegraph)

	// Create the file
	if err := os.WriteFile(filePath, []byte("test content"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Register the file
	fs.RegisterFile(taskID, filePath, string(FormatFlamegraph))

	// Get the file
	fileInfo, ok := fs.GetFile(taskID)
	if !ok {
		t.Fatal("File not found after registration")
	}

	if fileInfo.TaskID != taskID {
		t.Errorf("Expected TaskID %s, got %s", taskID, fileInfo.TaskID)
	}

	if fileInfo.FileName != "profile.svg" {
		t.Errorf("Expected FileName profile.svg, got %s", fileInfo.FileName)
	}

	if fileInfo.Format != string(FormatFlamegraph) {
		t.Errorf("Expected Format flamegraph, got %s", fileInfo.Format)
	}
}

func TestListFiles(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := &config.PySpyConfig{
		Enabled:           true,
		StoragePath:       tmpDir,
		FileRetentionDays: 7,
	}

	fs, _ := NewFileStore(cfg)

	// Create multiple files
	taskIDs := []string{"task-1", "task-2", "task-3"}
	for _, taskID := range taskIDs {
		filePath, _ := fs.PrepareOutputFile(taskID, FormatFlamegraph)
		os.WriteFile(filePath, []byte("test"), 0644)
		fs.RegisterFile(taskID, filePath, string(FormatFlamegraph))
		time.Sleep(10 * time.Millisecond) // Ensure different timestamps
	}

	// List all files
	response := fs.ListFiles(&FileListRequest{})
	if response.TotalCount != 3 {
		t.Errorf("Expected 3 files, got %d", response.TotalCount)
	}

	// List with limit
	response = fs.ListFiles(&FileListRequest{Limit: 2})
	if len(response.Files) != 2 {
		t.Errorf("Expected 2 files with limit, got %d", len(response.Files))
	}

	// List by task ID
	response = fs.ListFiles(&FileListRequest{TaskID: "task-1"})
	if response.TotalCount != 1 {
		t.Errorf("Expected 1 file for task-1, got %d", response.TotalCount)
	}
}

func TestDeleteFile(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := &config.PySpyConfig{
		Enabled:           true,
		StoragePath:       tmpDir,
		FileRetentionDays: 7,
	}

	fs, _ := NewFileStore(cfg)

	taskID := "delete-test-task"
	filePath, _ := fs.PrepareOutputFile(taskID, FormatFlamegraph)
	os.WriteFile(filePath, []byte("test"), 0644)
	fs.RegisterFile(taskID, filePath, string(FormatFlamegraph))

	// Verify file exists
	if _, ok := fs.GetFile(taskID); !ok {
		t.Fatal("File should exist before deletion")
	}

	// Delete the file
	if err := fs.DeleteFile(taskID); err != nil {
		t.Fatalf("DeleteFile failed: %v", err)
	}

	// Verify file is gone
	if _, ok := fs.GetFile(taskID); ok {
		t.Error("File should not exist after deletion")
	}

	// Verify directory is removed
	taskDir := filepath.Dir(filePath)
	if _, err := os.Stat(taskDir); !os.IsNotExist(err) {
		t.Error("Task directory should be removed after deletion")
	}
}

func TestGetStorageStats(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := &config.PySpyConfig{
		Enabled:           true,
		StoragePath:       tmpDir,
		FileRetentionDays: 7,
	}

	fs, _ := NewFileStore(cfg)

	// Initially empty
	count, size := fs.GetStorageStats()
	if count != 0 || size != 0 {
		t.Errorf("Expected 0 files and 0 size, got %d files and %d bytes", count, size)
	}

	// Add a file
	taskID := "stats-test-task"
	filePath, _ := fs.PrepareOutputFile(taskID, FormatFlamegraph)
	content := "test content 12345"
	os.WriteFile(filePath, []byte(content), 0644)
	fs.RegisterFile(taskID, filePath, string(FormatFlamegraph))

	// Check stats
	count, size = fs.GetStorageStats()
	if count != 1 {
		t.Errorf("Expected 1 file, got %d", count)
	}
	if size != int64(len(content)) {
		t.Errorf("Expected %d bytes, got %d", len(content), size)
	}
}

func TestDetectFormat(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := &config.PySpyConfig{
		Enabled:           true,
		StoragePath:       tmpDir,
		FileRetentionDays: 7,
	}

	fs, _ := NewFileStore(cfg)

	tests := []struct {
		fileName string
		expected string
	}{
		{"profile.svg", string(FormatFlamegraph)},
		{"profile.json", string(FormatSpeedscope)},
		{"profile.txt", string(FormatRaw)},
		{"unknown.xyz", string(FormatFlamegraph)}, // default
	}

	for _, tc := range tests {
		result := fs.detectFormat(tc.fileName)
		if result != tc.expected {
			t.Errorf("For %s: expected %s, got %s", tc.fileName, tc.expected, result)
		}
	}
}

