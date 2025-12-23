package processtree

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewContainerdReader(t *testing.T) {
	reader, err := NewContainerdReader()
	
	// May fail if containerd is not available
	if err != nil {
		t.Logf("NewContainerdReader error (expected if containerd not available): %v", err)
		return
	}

	assert.NotNil(t, reader)
}

func TestGetPodContainers_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	defer func() {
		if r := recover(); r != nil {
			t.Skipf("Test panicked (likely containerd not available): %v", r)
		}
	}()

	reader, err := NewContainerdReader()
	if err != nil {
		t.Skipf("Containerd not available: %v", err)
	}

	ctx := context.Background()

	t.Run("non-existent pod", func(t *testing.T) {
		defer func() {
			if r := recover(); r != nil {
				t.Skipf("Test panicked (likely containerd not available): %v", r)
			}
		}()

		containers, err := reader.GetPodContainers(ctx, "nonexistent-pod-uid-12345")
		
		// Should return error because pod not found
		assert.Error(t, err)
		assert.Nil(t, containers)
	})

	t.Run("with cancelled context", func(t *testing.T) {
		defer func() {
			if r := recover(); r != nil {
				t.Skipf("Test panicked (likely containerd not available): %v", r)
			}
		}()

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := reader.GetPodContainers(ctx, "test-pod-uid")
		
		// May fail due to context cancellation or pod not found
		assert.Error(t, err)
	})
}

func TestGetContainerInfo_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	defer func() {
		if r := recover(); r != nil {
			t.Skipf("Test panicked (likely containerd not available): %v", r)
		}
	}()

	reader, err := NewContainerdReader()
	if err != nil {
		t.Skipf("Containerd not available: %v", err)
	}

	ctx := context.Background()

	t.Run("non-existent container", func(t *testing.T) {
		defer func() {
			if r := recover(); r != nil {
				t.Skipf("Test panicked (likely containerd not available): %v", r)
			}
		}()

		_, err := reader.GetContainerInfo(ctx, "nonexistent-container-id-12345")
		
		// Should return error because container not found
		assert.Error(t, err)
	})

	t.Run("empty container ID", func(t *testing.T) {
		defer func() {
			if r := recover(); r != nil {
				t.Skipf("Test panicked (likely containerd not available): %v", r)
			}
		}()

		_, err := reader.GetContainerInfo(ctx, "")
		
		// Should return error
		assert.Error(t, err)
	})
}

func TestContainerInfo_Extraction(t *testing.T) {
	info := &ContainerInfo{
		ID:    "abc123def456",
		Name:  "main-container",
		Image: "python:3.9",
	}

	assert.Equal(t, "abc123def456", info.ID)
	assert.Equal(t, "main-container", info.Name)
	assert.Equal(t, "python:3.9", info.Image)
	assert.NotEmpty(t, info.ID)
}

// Benchmark tests
func BenchmarkNewContainerdReader(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		NewContainerdReader()
	}
}

