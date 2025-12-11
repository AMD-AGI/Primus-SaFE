package processtree

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewKubeletReader(t *testing.T) {
	reader, err := NewKubeletReader()
	assert.NoError(t, err)
	assert.NotNil(t, reader)
}

func TestGetPodInfo(t *testing.T) {
	reader, err := NewKubeletReader()
	require.NoError(t, err)

	ctx := context.Background()

	t.Run("get pod info", func(t *testing.T) {
		info, err := reader.GetPodInfo(ctx, "default", "test-pod")

		require.NoError(t, err)
		assert.NotNil(t, info)
		assert.Equal(t, "test-pod", info.Name)
		assert.Equal(t, "default", info.Namespace)
	})

	t.Run("empty namespace", func(t *testing.T) {
		info, err := reader.GetPodInfo(ctx, "", "test-pod")

		require.NoError(t, err)
		assert.NotNil(t, info)
		assert.Equal(t, "test-pod", info.Name)
		assert.Equal(t, "", info.Namespace)
	})

	t.Run("empty name", func(t *testing.T) {
		info, err := reader.GetPodInfo(ctx, "default", "")

		require.NoError(t, err)
		assert.NotNil(t, info)
		assert.Equal(t, "", info.Name)
		assert.Equal(t, "default", info.Namespace)
	})
}

func TestGetPodByUID(t *testing.T) {
	reader, err := NewKubeletReader()
	require.NoError(t, err)

	ctx := context.Background()

	t.Run("get pod by UID", func(t *testing.T) {
		testUID := "12345-67890-abcdef"
		info, err := reader.GetPodByUID(ctx, testUID)

		require.NoError(t, err)
		assert.NotNil(t, info)
		assert.Equal(t, testUID, info.UID)
	})

	t.Run("empty UID", func(t *testing.T) {
		info, err := reader.GetPodByUID(ctx, "")

		require.NoError(t, err)
		assert.NotNil(t, info)
		assert.Equal(t, "", info.UID)
	})
}

func TestGetPodInfo_WithCancelledContext(t *testing.T) {
	reader, err := NewKubeletReader()
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	t.Run("with cancelled context", func(t *testing.T) {
		info, err := reader.GetPodInfo(ctx, "default", "test-pod")

		// Currently the implementation doesn't check context cancellation
		// This test documents the current behavior
		require.NoError(t, err)
		assert.NotNil(t, info)
	})
}

func TestKubeletPodInfo_Fields(t *testing.T) {
	info := &KubeletPodInfo{
		Name:       "test-pod",
		Namespace:  "default",
		UID:        "abc-123",
		NodeName:   "node-1",
		Phase:      "Running",
		Containers: []string{"container-1", "container-2"},
	}

	assert.Equal(t, "test-pod", info.Name)
	assert.Equal(t, "default", info.Namespace)
	assert.Equal(t, "abc-123", info.UID)
	assert.Equal(t, "node-1", info.NodeName)
	assert.Equal(t, "Running", info.Phase)
	assert.Len(t, info.Containers, 2)
}

// Benchmark tests
func BenchmarkGetPodInfo(b *testing.B) {
	reader, err := NewKubeletReader()
	require.NoError(b, err)

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		reader.GetPodInfo(ctx, "default", "test-pod")
	}
}

func BenchmarkGetPodByUID(b *testing.B) {
	reader, err := NewKubeletReader()
	require.NoError(b, err)

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		reader.GetPodByUID(ctx, "test-uid-12345")
	}
}

