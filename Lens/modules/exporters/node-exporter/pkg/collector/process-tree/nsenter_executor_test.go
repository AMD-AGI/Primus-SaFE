package processtree

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewNsenterExecutor(t *testing.T) {
	executor := NewNsenterExecutor()
	assert.NotNil(t, executor)
}

func TestCheckNsenterAvailable(t *testing.T) {
	executor := NewNsenterExecutor()

	// Just check that it doesn't panic
	available := executor.CheckNsenterAvailable()

	// On Linux systems with nsenter, this should be true
	// On other systems or in containers without nsenter, it may be false
	t.Logf("nsenter available: %v", available)
}

func TestGetContainerPID_Integration(t *testing.T) {
	if os.Getenv("SKIP_PROC_TESTS") != "" {
		t.Skip("Skipping /proc filesystem tests")
	}

	executor := NewNsenterExecutor()
	pid := os.Getpid()

	t.Run("success for self", func(t *testing.T) {
		containerPID, err := executor.GetContainerPID(pid)

		if err != nil {
			// This is expected if not in a container namespace
			t.Logf("GetContainerPID error (expected if not in container): %v", err)
			return
		}

		// If successful, the container PID should be positive
		assert.Greater(t, containerPID, 0)
	})

	t.Run("non-existent pid", func(t *testing.T) {
		_, err := executor.GetContainerPID(999999)
		assert.Error(t, err)
	})

	t.Run("invalid pid", func(t *testing.T) {
		_, err := executor.GetContainerPID(-1)
		assert.Error(t, err)
	})
}

func TestExecuteInContainer_Integration(t *testing.T) {
	if os.Getenv("SKIP_NSENTER_TESTS") != "" {
		t.Skip("Skipping nsenter tests")
	}

	executor := NewNsenterExecutor()

	// Check if nsenter is available
	if !executor.CheckNsenterAvailable() {
		t.Skip("nsenter not available on this system")
	}

	pid := os.Getpid()

	t.Run("simple command", func(t *testing.T) {
		output, err := executor.ExecuteInContainer(pid, "echo hello")

		// This will likely fail with permission errors unless running as root
		if err != nil {
			t.Logf("ExecuteInContainer error (expected without proper permissions): %v", err)
			return
		}

		require.NoError(t, err)
		assert.Contains(t, output, "hello")
	})

	t.Run("non-existent pid", func(t *testing.T) {
		_, err := executor.ExecuteInContainer(999999, "echo test")
		assert.Error(t, err)
	})
}

func TestGetContainerProcessList_Integration(t *testing.T) {
	if os.Getenv("SKIP_NSENTER_TESTS") != "" {
		t.Skip("Skipping nsenter tests")
	}

	executor := NewNsenterExecutor()

	if !executor.CheckNsenterAvailable() {
		t.Skip("nsenter not available on this system")
	}

	pid := os.Getpid()

	t.Run("get process list", func(t *testing.T) {
		lines, err := executor.GetContainerProcessList(pid)

		// This will likely fail with permission errors unless running as root
		if err != nil {
			t.Logf("GetContainerProcessList error (expected without proper permissions): %v", err)
			return
		}

		require.NoError(t, err)
		assert.NotEmpty(t, lines)
	})
}

func TestGetContainerEnvironment_Integration(t *testing.T) {
	if os.Getenv("SKIP_NSENTER_TESTS") != "" {
		t.Skip("Skipping nsenter tests")
	}

	executor := NewNsenterExecutor()

	if !executor.CheckNsenterAvailable() {
		t.Skip("nsenter not available on this system")
	}

	pid := os.Getpid()

	t.Run("get environment", func(t *testing.T) {
		env, err := executor.GetContainerEnvironment(pid)

		// This will likely fail with permission errors unless running as root
		if err != nil {
			t.Logf("GetContainerEnvironment error (expected without proper permissions): %v", err)
			return
		}

		require.NoError(t, err)
		assert.NotEmpty(t, env)
	})
}

// Benchmark tests
func BenchmarkGetContainerPID(b *testing.B) {
	if os.Getenv("SKIP_PROC_TESTS") != "" {
		b.Skip("Skipping /proc filesystem tests")
	}

	executor := NewNsenterExecutor()
	pid := os.Getpid()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		executor.GetContainerPID(pid)
	}
}

func BenchmarkCheckNsenterAvailable(b *testing.B) {
	executor := NewNsenterExecutor()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		executor.CheckNsenterAvailable()
	}
}
