// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package sql

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"gorm.io/gorm/logger"
)

// TestNullLogger_LogMode tests the LogMode method
func TestNullLogger_LogMode(t *testing.T) {
	nullLogger := NullLogger{}

	tests := []logger.LogLevel{
		logger.Silent,
		logger.Error,
		logger.Warn,
		logger.Info,
	}

	levelNames := []string{"Silent", "Error", "Warn", "Info"}
	
	for i, level := range tests {
		t.Run(levelNames[i], func(t *testing.T) {
			result := nullLogger.LogMode(level)
			assert.NotNil(t, result)
			// Should return itself
			assert.IsType(t, NullLogger{}, result)
		})
	}
}

// TestNullLogger_Info tests the Info method
func TestNullLogger_Info(t *testing.T) {
	nullLogger := NullLogger{}
	ctx := context.Background()

	// Should not panic
	assert.NotPanics(t, func() {
		nullLogger.Info(ctx, "test message")
		nullLogger.Info(ctx, "test %s", "formatted")
		nullLogger.Info(ctx, "test %d %s", 123, "values")
	})
}

// TestNullLogger_Warn tests the Warn method
func TestNullLogger_Warn(t *testing.T) {
	nullLogger := NullLogger{}
	ctx := context.Background()

	// Should not panic
	assert.NotPanics(t, func() {
		nullLogger.Warn(ctx, "warning message")
		nullLogger.Warn(ctx, "warning %s", "formatted")
		nullLogger.Warn(ctx, "warning %d %s", 456, "values")
	})
}

// TestNullLogger_Error tests the Error method
func TestNullLogger_Error(t *testing.T) {
	nullLogger := NullLogger{}
	ctx := context.Background()

	// Should not panic
	assert.NotPanics(t, func() {
		nullLogger.Error(ctx, "error message")
		nullLogger.Error(ctx, "error %s", "formatted")
		nullLogger.Error(ctx, "error %d %s", 789, "values")
	})
}

// TestNullLogger_Trace tests the Trace method
func TestNullLogger_Trace(t *testing.T) {
	nullLogger := NullLogger{}
	ctx := context.Background()
	begin := time.Now()

	tests := []struct {
		name string
		fc   func() (string, int64)
		err  error
	}{
		{
			name: "successful query",
			fc: func() (string, int64) {
				return "SELECT * FROM users", 10
			},
			err: nil,
		},
		{
			name: "query with error",
			fc: func() (string, int64) {
				return "SELECT * FROM users WHERE id = ?", 0
			},
			err: errors.New("record not found"),
		},
		{
			name: "insert query",
			fc: func() (string, int64) {
				return "INSERT INTO users (name) VALUES (?)", 1
			},
			err: nil,
		},
		{
			name: "update query",
			fc: func() (string, int64) {
				return "UPDATE users SET name = ? WHERE id = ?", 1
			},
			err: nil,
		},
		{
			name: "delete query",
			fc: func() (string, int64) {
				return "DELETE FROM users WHERE id = ?", 1
			},
			err: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Should not panic
			assert.NotPanics(t, func() {
				nullLogger.Trace(ctx, begin, tt.fc, tt.err)
			})
		})
	}
}

// TestNullLogger_Trace_SlowQuery tests slow query logging
func TestNullLogger_Trace_SlowQuery(t *testing.T) {
	nullLogger := NullLogger{}
	ctx := context.Background()

	// Simulate a slow query (> 5 seconds)
	begin := time.Now().Add(-6 * time.Second)

	fc := func() (string, int64) {
		return "SELECT * FROM large_table", 1000
	}

	// Should not panic even for slow queries
	assert.NotPanics(t, func() {
		nullLogger.Trace(ctx, begin, fc, nil)
	})
}

// TestNullLogger_Trace_FastQuery tests fast query logging
func TestNullLogger_Trace_FastQuery(t *testing.T) {
	nullLogger := NullLogger{}
	ctx := context.Background()

	// Simulate a fast query (< 5 seconds)
	begin := time.Now().Add(-100 * time.Millisecond)

	fc := func() (string, int64) {
		return "SELECT * FROM users WHERE id = 1", 1
	}

	// Should not panic for fast queries
	assert.NotPanics(t, func() {
		nullLogger.Trace(ctx, begin, fc, nil)
	})
}

// TestNullLogger_Trace_WithNilError tests Trace with nil error
func TestNullLogger_Trace_WithNilError(t *testing.T) {
	nullLogger := NullLogger{}
	ctx := context.Background()
	begin := time.Now()

	fc := func() (string, int64) {
		return "SELECT COUNT(*) FROM users", 100
	}

	// Should handle nil error gracefully
	assert.NotPanics(t, func() {
		nullLogger.Trace(ctx, begin, fc, nil)
	})
}

// TestNullLogger_Trace_WithContextCancelled tests Trace with cancelled context
func TestNullLogger_Trace_WithContextCancelled(t *testing.T) {
	nullLogger := NullLogger{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	begin := time.Now()
	fc := func() (string, int64) {
		return "SELECT * FROM users", 0
	}

	// Should not panic with cancelled context
	assert.NotPanics(t, func() {
		nullLogger.Trace(ctx, begin, fc, context.Canceled)
	})
}

// TestNullLogger_Trace_EmptySQL tests Trace with empty SQL
func TestNullLogger_Trace_EmptySQL(t *testing.T) {
	nullLogger := NullLogger{}
	ctx := context.Background()
	begin := time.Now()

	fc := func() (string, int64) {
		return "", 0
	}

	// Should handle empty SQL gracefully
	assert.NotPanics(t, func() {
		nullLogger.Trace(ctx, begin, fc, nil)
	})
}

// TestNullLogger_Trace_ZeroRowsAffected tests Trace with zero rows affected
func TestNullLogger_Trace_ZeroRowsAffected(t *testing.T) {
	nullLogger := NullLogger{}
	ctx := context.Background()
	begin := time.Now()

	fc := func() (string, int64) {
		return "UPDATE users SET active = true WHERE false", 0
	}

	// Should handle zero rows affected gracefully
	assert.NotPanics(t, func() {
		nullLogger.Trace(ctx, begin, fc, nil)
	})
}

// TestNullLogger_Trace_LargeRowCount tests Trace with large row count
func TestNullLogger_Trace_LargeRowCount(t *testing.T) {
	nullLogger := NullLogger{}
	ctx := context.Background()
	begin := time.Now()

	fc := func() (string, int64) {
		return "SELECT * FROM huge_table", 1000000
	}

	// Should handle large row count gracefully
	assert.NotPanics(t, func() {
		nullLogger.Trace(ctx, begin, fc, nil)
	})
}

// TestNullLogger_Interface tests that NullLogger implements logger.Interface
func TestNullLogger_Interface(t *testing.T) {
	var _ logger.Interface = NullLogger{}
	var _ logger.Interface = &NullLogger{}

	// Verify all methods are implemented
	nullLogger := NullLogger{}
	ctx := context.Background()

	// LogMode
	_ = nullLogger.LogMode(logger.Info)

	// Info, Warn, Error
	nullLogger.Info(ctx, "info")
	nullLogger.Warn(ctx, "warn")
	nullLogger.Error(ctx, "error")

	// Trace
	nullLogger.Trace(ctx, time.Now(), func() (string, int64) {
		return "SELECT 1", 1
	}, nil)
}

// TestNullLogger_ConcurrentAccess tests concurrent access to NullLogger
func TestNullLogger_ConcurrentAccess(t *testing.T) {
	nullLogger := NullLogger{}
	ctx := context.Background()
	begin := time.Now()

	done := make(chan bool)

	// Launch multiple goroutines
	for i := 0; i < 10; i++ {
		go func(id int) {
			nullLogger.Info(ctx, "info from goroutine %d", id)
			nullLogger.Warn(ctx, "warn from goroutine %d", id)
			nullLogger.Error(ctx, "error from goroutine %d", id)
			nullLogger.Trace(ctx, begin, func() (string, int64) {
				return "SELECT * FROM users", 1
			}, nil)
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Should not panic or deadlock
	assert.True(t, true)
}

// BenchmarkNullLogger_Info benchmarks the Info method
func BenchmarkNullLogger_Info(b *testing.B) {
	nullLogger := NullLogger{}
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		nullLogger.Info(ctx, "test message %d", i)
	}
}

// BenchmarkNullLogger_Trace benchmarks the Trace method
func BenchmarkNullLogger_Trace(b *testing.B) {
	nullLogger := NullLogger{}
	ctx := context.Background()
	begin := time.Now()

	fc := func() (string, int64) {
		return "SELECT * FROM users WHERE id = ?", 1
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		nullLogger.Trace(ctx, begin, fc, nil)
	}
}

// BenchmarkNullLogger_Trace_SlowQuery benchmarks slow query logging
func BenchmarkNullLogger_Trace_SlowQuery(b *testing.B) {
	nullLogger := NullLogger{}
	ctx := context.Background()
	begin := time.Now().Add(-6 * time.Second)

	fc := func() (string, int64) {
		return "SELECT * FROM large_table", 1000
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		nullLogger.Trace(ctx, begin, fc, nil)
	}
}

