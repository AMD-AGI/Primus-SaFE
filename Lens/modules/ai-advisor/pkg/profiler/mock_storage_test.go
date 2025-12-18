package profiler

import (
	"context"
	"fmt"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/ai-advisor/pkg/profiler/storage"
)

// mockStorageBackend is a mock implementation of storage.StorageBackend for testing
type mockStorageBackend struct {
	files map[string][]byte
}

func newMockStorageBackend() *mockStorageBackend {
	return &mockStorageBackend{
		files: make(map[string][]byte),
	}
}

func (m *mockStorageBackend) Store(ctx context.Context, req *storage.StoreRequest) (*storage.StoreResponse, error) {
	m.files[req.FileID] = req.Content
	return &storage.StoreResponse{
		FileID:      req.FileID,
		StoragePath: req.FileID,
		StorageType: "mock",
		Size:        int64(len(req.Content)),
		MD5:         "mock-md5",
	}, nil
}

func (m *mockStorageBackend) Retrieve(ctx context.Context, req *storage.RetrieveRequest) (*storage.RetrieveResponse, error) {
	content, exists := m.files[req.FileID]
	if !exists {
		return nil, fmt.Errorf("file not found: %s", req.FileID)
	}
	return &storage.RetrieveResponse{
		Content:    content,
		Size:       int64(len(content)),
		Compressed: false,
	}, nil
}

func (m *mockStorageBackend) Delete(ctx context.Context, fileID string) error {
	delete(m.files, fileID)
	return nil
}

func (m *mockStorageBackend) GenerateDownloadURL(ctx context.Context, fileID string, expires time.Duration) (string, error) {
	return "http://mock-url/" + fileID, nil
}

func (m *mockStorageBackend) GetStorageType() string {
	return "mock"
}

func (m *mockStorageBackend) Exists(ctx context.Context, fileID string) (bool, error) {
	_, exists := m.files[fileID]
	return exists, nil
}

func (m *mockStorageBackend) ExistsByWorkloadAndFilename(ctx context.Context, workloadUID string, fileName string) (bool, error) {
	key := workloadUID + "/" + fileName
	_, exists := m.files[key]
	return exists, nil
}

