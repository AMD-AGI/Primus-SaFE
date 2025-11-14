package context

import (
	"context"
	"sync"
	"testing"
)

// TestWithObject tests the WithObject function
func TestWithObject(t *testing.T) {
	tests := []struct {
		name  string
		setup func() context.Context
		key   string
		value interface{}
		check func(t *testing.T, ctx context.Context)
	}{
		{
			name: "add single object to new context",
			setup: func() context.Context {
				return context.Background()
			},
			key:   "test-key",
			value: "test-value",
			check: func(t *testing.T, ctx context.Context) {
				val, ok := GetValue(ctx, "test-key")
				if !ok {
					t.Error("Expected to find test-key in context")
				}
				if val != "test-value" {
					t.Errorf("Expected value to be test-value, got %v", val)
				}
			},
		},
		{
			name: "add multiple objects",
			setup: func() context.Context {
				ctx := context.Background()
				ctx = WithObject(ctx, "key1", "value1")
				return ctx
			},
			key:   "key2",
			value: "value2",
			check: func(t *testing.T, ctx context.Context) {
				// Check both keys exist
				val1, ok1 := GetValue(ctx, "key1")
				val2, ok2 := GetValue(ctx, "key2")
				
				if !ok1 {
					t.Error("Expected to find key1 in context")
				}
				if !ok2 {
					t.Error("Expected to find key2 in context")
				}
				if val1 != "value1" {
					t.Errorf("Expected value1, got %v", val1)
				}
				if val2 != "value2" {
					t.Errorf("Expected value2, got %v", val2)
				}
			},
		},
		{
			name: "store complex object",
			setup: func() context.Context {
				return context.Background()
			},
			key: "complex",
			value: map[string]interface{}{
				"nested": "value",
				"count":  42,
			},
			check: func(t *testing.T, ctx context.Context) {
				val, ok := GetValue(ctx, "complex")
				if !ok {
					t.Error("Expected to find complex key in context")
				}
				m, ok := val.(map[string]interface{})
				if !ok {
					t.Error("Expected value to be a map")
				}
				if m["nested"] != "value" {
					t.Errorf("Expected nested value, got %v", m["nested"])
				}
				if m["count"] != 42 {
					t.Errorf("Expected count to be 42, got %v", m["count"])
				}
			},
		},
		{
			name: "overwrite existing key",
			setup: func() context.Context {
				ctx := context.Background()
				return WithObject(ctx, "key", "old-value")
			},
			key:   "key",
			value: "new-value",
			check: func(t *testing.T, ctx context.Context) {
				val, ok := GetValue(ctx, "key")
				if !ok {
					t.Error("Expected to find key in context")
				}
				if val != "new-value" {
					t.Errorf("Expected new-value, got %v", val)
				}
			},
		},
		{
			name: "store nil value",
			setup: func() context.Context {
				return context.Background()
			},
			key:   "nil-key",
			value: nil,
			check: func(t *testing.T, ctx context.Context) {
				val, ok := GetValue(ctx, "nil-key")
				if !ok {
					t.Error("Expected to find nil-key in context")
				}
				if val != nil {
					t.Errorf("Expected nil value, got %v", val)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := tt.setup()
			ctx = WithObject(ctx, tt.key, tt.value)
			tt.check(t, ctx)
		})
	}
}

// TestWithoutObject tests the WithoutObject function
func TestWithoutObject(t *testing.T) {
	tests := []struct {
		name  string
		setup func() context.Context
		key   string
		check func(t *testing.T, ctx context.Context)
	}{
		{
			name: "delete existing key",
			setup: func() context.Context {
				ctx := context.Background()
				return WithObject(ctx, "key-to-delete", "value")
			},
			key: "key-to-delete",
			check: func(t *testing.T, ctx context.Context) {
				_, ok := GetValue(ctx, "key-to-delete")
				if ok {
					t.Error("Expected key to be deleted")
				}
			},
		},
		{
			name: "delete non-existing key",
			setup: func() context.Context {
				return context.Background()
			},
			key: "non-existing",
			check: func(t *testing.T, ctx context.Context) {
				// Should not panic or error
				_, ok := GetValue(ctx, "non-existing")
				if ok {
					t.Error("Expected key to not exist")
				}
			},
		},
		{
			name: "delete one key but keep others",
			setup: func() context.Context {
				ctx := context.Background()
				ctx = WithObject(ctx, "keep", "value1")
				ctx = WithObject(ctx, "delete", "value2")
				ctx = WithObject(ctx, "also-keep", "value3")
				return ctx
			},
			key: "delete",
			check: func(t *testing.T, ctx context.Context) {
				// Deleted key should not exist
				_, ok := GetValue(ctx, "delete")
				if ok {
					t.Error("Expected delete key to be removed")
				}
				
				// Other keys should still exist
				val1, ok1 := GetValue(ctx, "keep")
				val2, ok2 := GetValue(ctx, "also-keep")
				
				if !ok1 {
					t.Error("Expected keep key to still exist")
				}
				if !ok2 {
					t.Error("Expected also-keep key to still exist")
				}
				if val1 != "value1" {
					t.Errorf("Expected value1, got %v", val1)
				}
				if val2 != "value3" {
					t.Errorf("Expected value3, got %v", val2)
				}
			},
		},
		{
			name: "delete from empty context",
			setup: func() context.Context {
				return context.Background()
			},
			key: "any-key",
			check: func(t *testing.T, ctx context.Context) {
				// Should not panic
				_, ok := GetValue(ctx, "any-key")
				if ok {
					t.Error("Expected key to not exist")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := tt.setup()
			ctx = WithoutObject(ctx, tt.key)
			tt.check(t, ctx)
		})
	}
}

// TestGetValue tests the GetValue function
func TestGetValue(t *testing.T) {
	tests := []struct {
		name      string
		setup     func() context.Context
		key       string
		wantValue interface{}
		wantOk    bool
	}{
		{
			name: "get existing value",
			setup: func() context.Context {
				ctx := context.Background()
				return WithObject(ctx, "existing", "value")
			},
			key:       "existing",
			wantValue: "value",
			wantOk:    true,
		},
		{
			name: "get non-existing value",
			setup: func() context.Context {
				return context.Background()
			},
			key:       "non-existing",
			wantValue: nil,
			wantOk:    false,
		},
		{
			name: "get from empty context",
			setup: func() context.Context {
				return context.Background()
			},
			key:       "any-key",
			wantValue: nil,
			wantOk:    false,
		},
		{
			name: "get nil value",
			setup: func() context.Context {
				ctx := context.Background()
				return WithObject(ctx, "nil-key", nil)
			},
			key:       "nil-key",
			wantValue: nil,
			wantOk:    true,
		},
		{
			name: "get integer value",
			setup: func() context.Context {
				ctx := context.Background()
				return WithObject(ctx, "int-key", 42)
			},
			key:       "int-key",
			wantValue: 42,
			wantOk:    true,
		},
		{
			name: "get struct value",
			setup: func() context.Context {
				ctx := context.Background()
				type testStruct struct {
					Name string
					Age  int
				}
				return WithObject(ctx, "struct-key", testStruct{Name: "Test", Age: 25})
			},
			key: "struct-key",
			wantValue: struct {
				Name string
				Age  int
			}{Name: "Test", Age: 25},
			wantOk: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := tt.setup()
			gotValue, gotOk := GetValue(ctx, tt.key)
			
			if gotOk != tt.wantOk {
				t.Errorf("GetValue() ok = %v, want %v", gotOk, tt.wantOk)
			}
			
			// Only check value if we expected to find it
			if tt.wantOk {
				if gotValue != tt.wantValue {
					t.Errorf("GetValue() value = %v, want %v", gotValue, tt.wantValue)
				}
			}
		})
	}
}

// TestShallowCopyCtx tests the ShallowCopyCtx function
func TestShallowCopyCtx(t *testing.T) {
	tests := []struct {
		name  string
		setup func() context.Context
		check func(t *testing.T, original, copied context.Context)
	}{
		{
			name: "copy context with values",
			setup: func() context.Context {
				ctx := context.Background()
				ctx = WithObject(ctx, "key1", "value1")
				ctx = WithObject(ctx, "key2", "value2")
				return ctx
			},
			check: func(t *testing.T, original, copied context.Context) {
				// Values should be copied
				val1, ok1 := GetValue(copied, "key1")
				val2, ok2 := GetValue(copied, "key2")
				
				if !ok1 || !ok2 {
					t.Error("Expected copied context to have all values")
				}
				if val1 != "value1" || val2 != "value2" {
					t.Error("Expected copied values to match original")
				}
			},
		},
		{
			name: "copy empty context",
			setup: func() context.Context {
				return context.Background()
			},
			check: func(t *testing.T, original, copied context.Context) {
				// Should not panic
				_, ok := GetValue(copied, "any-key")
				if ok {
					t.Error("Expected copied empty context to have no values")
				}
			},
		},
		{
			name: "modifications to copied context don't affect original",
			setup: func() context.Context {
				ctx := context.Background()
				return WithObject(ctx, "shared", "original-value")
			},
			check: func(t *testing.T, original, copied context.Context) {
				// Modify copied context
				copied = WithObject(copied, "shared", "modified-value")
				copied = WithObject(copied, "new-key", "new-value")
				
				// Original should be unchanged
				val, _ := GetValue(original, "shared")
				if val != "original-value" {
					t.Error("Expected original context to be unchanged")
				}
				
				_, ok := GetValue(original, "new-key")
				if ok {
					t.Error("Expected new-key to not exist in original")
				}
			},
		},
		{
			name: "copy context with ignore keys",
			setup: func() context.Context {
				ctx := context.Background()
				ctx = WithObject(ctx, "normal-key", "normal-value")
				
				// Add a key to ignore list
				contextCopyIgnoreKey.Store("ignore-key", true)
				ctx = WithObject(ctx, "ignore-key", "ignored-value")
				
				return ctx
			},
			check: func(t *testing.T, original, copied context.Context) {
				// Normal key should be copied
				val, ok := GetValue(copied, "normal-key")
				if !ok || val != "normal-value" {
					t.Error("Expected normal-key to be copied")
				}
				
				// Ignored key should not be copied
				_, ok = GetValue(copied, "ignore-key")
				if ok {
					t.Error("Expected ignore-key to not be copied")
				}
				
				// Cleanup
				contextCopyIgnoreKey.Delete("ignore-key")
			},
		},
		{
			name: "copy returns new background context",
			setup: func() context.Context {
				ctx := context.Background()
				return WithObject(ctx, "key", "value")
			},
			check: func(t *testing.T, original, copied context.Context) {
				// Copied context should be a new background context
				// (Not the same instance as original)
				if original == copied {
					t.Error("Expected copied context to be a new instance")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			original := tt.setup()
			copied := ShallowCopyCtx(original)
			tt.check(t, original, copied)
		})
	}
}

// TestConcurrentAccess tests thread safety of context operations
func TestConcurrentAccess(t *testing.T) {
	t.Run("concurrent writes", func(t *testing.T) {
		ctx := context.Background()
		var wg sync.WaitGroup
		numGoroutines := 100
		
		wg.Add(numGoroutines)
		for i := 0; i < numGoroutines; i++ {
			go func(id int) {
				defer wg.Done()
				// Each goroutine gets its own context to avoid data races
				localCtx := ctx
				key := "key-" + string(rune(id))
				value := "value-" + string(rune(id))
				_ = WithObject(localCtx, key, value)
			}(i)
		}
		
		wg.Wait()
		// Test should complete without panic or race condition
	})
	
	t.Run("concurrent reads and writes", func(t *testing.T) {
		ctx := context.Background()
		ctx = WithObject(ctx, "shared", "value")
		
		var wg sync.WaitGroup
		numOperations := 50
		
		// Writers
		wg.Add(numOperations)
		for i := 0; i < numOperations; i++ {
			go func(id int) {
				defer wg.Done()
				// Each goroutine gets its own context to avoid data races
				localCtx := ctx
				key := "key-" + string(rune(id))
				_ = WithObject(localCtx, key, id)
			}(i)
		}
		
		// Readers
		wg.Add(numOperations)
		for i := 0; i < numOperations; i++ {
			go func() {
				defer wg.Done()
				GetValue(ctx, "shared")
			}()
		}
		
		wg.Wait()
		// Test should complete without panic or race condition
	})
}

// TestFindOrCreateContextMap tests the findOrCreateContextMap function
func TestFindOrCreateContextMap(t *testing.T) {
	t.Run("create new map for empty context", func(t *testing.T) {
		ctx := context.Background()
		ctxMap, newCtx := findOrCreateContextMap(ctx)
		
		if ctxMap == nil {
			t.Error("Expected non-nil context map")
		}
		
		if newCtx == nil {
			t.Error("Expected non-nil context")
		}
		
		// Verify the map is stored in context
		storedMap, ok := findContextMap(newCtx)
		if !ok {
			t.Error("Expected to find context map in new context")
		}
		if storedMap != ctxMap {
			t.Error("Expected stored map to be the same instance")
		}
	})
	
	t.Run("return existing map", func(t *testing.T) {
		ctx := context.Background()
		ctxMap1, ctx := findOrCreateContextMap(ctx)
		ctxMap2, _ := findOrCreateContextMap(ctx)
		
		if ctxMap1 != ctxMap2 {
			t.Error("Expected to get the same map instance")
		}
	})
}

// TestFindContextMap tests the findContextMap function
func TestFindContextMap(t *testing.T) {
	t.Run("find existing map", func(t *testing.T) {
		ctx := context.Background()
		ctx = WithObject(ctx, "key", "value")
		
		ctxMap, ok := findContextMap(ctx)
		if !ok {
			t.Error("Expected to find context map")
		}
		if ctxMap == nil {
			t.Error("Expected non-nil context map")
		}
	})
	
	t.Run("map not found in empty context", func(t *testing.T) {
		ctx := context.Background()
		
		_, ok := findContextMap(ctx)
		if ok {
			t.Error("Expected to not find context map in empty context")
		}
	})
}

// TestContextIntegration tests integration of multiple context operations
func TestContextIntegration(t *testing.T) {
	t.Run("complete workflow", func(t *testing.T) {
		// Start with background context
		ctx := context.Background()
		
		// Add multiple values
		ctx = WithObject(ctx, "user", "john")
		ctx = WithObject(ctx, "role", "admin")
		ctx = WithObject(ctx, "session", "abc123")
		
		// Verify all values
		user, ok1 := GetValue(ctx, "user")
		role, ok2 := GetValue(ctx, "role")
		session, ok3 := GetValue(ctx, "session")
		
		if !ok1 || !ok2 || !ok3 {
			t.Error("Expected all values to exist")
		}
		if user != "john" || role != "admin" || session != "abc123" {
			t.Error("Expected correct values")
		}
		
		// Remove one value
		ctx = WithoutObject(ctx, "session")
		_, ok := GetValue(ctx, "session")
		if ok {
			t.Error("Expected session to be removed")
		}
		
		// Copy context
		copiedCtx := ShallowCopyCtx(ctx)
		
		// Modify copy
		copiedCtx = WithObject(copiedCtx, "extra", "data")
		
		// Verify original unchanged
		_, ok = GetValue(ctx, "extra")
		if ok {
			t.Error("Expected original to not have extra key")
		}
		
		// Verify copy has all data
		_, ok = GetValue(copiedCtx, "user")
		if !ok {
			t.Error("Expected copied context to have user")
		}
		_, ok = GetValue(copiedCtx, "extra")
		if !ok {
			t.Error("Expected copied context to have extra")
		}
	})
}

// Benchmark tests
func BenchmarkWithObject(b *testing.B) {
	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ctx = WithObject(ctx, "key", "value")
	}
}

func BenchmarkGetValue(b *testing.B) {
	ctx := context.Background()
	ctx = WithObject(ctx, "key", "value")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		GetValue(ctx, "key")
	}
}

func BenchmarkWithoutObject(b *testing.B) {
	ctx := context.Background()
	ctx = WithObject(ctx, "key", "value")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ctx = WithoutObject(ctx, "key")
		ctx = WithObject(ctx, "key", "value") // Re-add for next iteration
	}
}

func BenchmarkShallowCopyCtx(b *testing.B) {
	ctx := context.Background()
	ctx = WithObject(ctx, "key1", "value1")
	ctx = WithObject(ctx, "key2", "value2")
	ctx = WithObject(ctx, "key3", "value3")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ShallowCopyCtx(ctx)
	}
}

func BenchmarkConcurrentWrites(b *testing.B) {
	ctx := context.Background()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			key := "key-" + string(rune(i))
			ctx = WithObject(ctx, key, i)
			i++
		}
	})
}

