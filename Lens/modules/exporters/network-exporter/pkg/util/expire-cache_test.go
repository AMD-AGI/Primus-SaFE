// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package util

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNewCache(t *testing.T) {
	t.Run("create cache - use default expiration time", func(t *testing.T) {
		cache := NewCache[string](nil, 0)
		assert.NotNil(t, cache)
		assert.NotNil(t, cache.Items)
		assert.Equal(t, time.Duration(0), cache.defaultExpire) // when passing 0, defaultExpire is 0
	})

	t.Run("create cache - custom expiration time", func(t *testing.T) {
		customExpire := 10 * time.Second
		cache := NewCache[int](nil, customExpire)
		assert.NotNil(t, cache)
		assert.Equal(t, customExpire, cache.defaultExpire)
	})

	t.Run("create cache - with expiration callback", func(t *testing.T) {
		called := false
		callback := func(key string, value string) {
			called = true
		}
		cache := NewCache[string](callback, time.Second)
		assert.NotNil(t, cache)
		assert.NotNil(t, cache.expireCallback)
		
		// verify callback is not called
		assert.False(t, called)
	})

	t.Run("create cache - negative expiration time not used", func(t *testing.T) {
		cache := NewCache[string](nil, -1*time.Second)
		assert.Equal(t, defaultDuration, cache.defaultExpire) // use defaultDuration when negative
	})
}

func TestCache_SetAndGet(t *testing.T) {
	t.Run("Set and Get - basic functionality", func(t *testing.T) {
		cache := NewCache[string](nil, time.Hour)
		
		cache.Set("key1", "value1", time.Hour)
		
		value, ok := cache.Get("key1")
		assert.True(t, ok)
		assert.Equal(t, "value1", value)
	})

	t.Run("Get - non-existent key", func(t *testing.T) {
		cache := NewCache[string](nil, time.Hour)
		
		value, ok := cache.Get("non-existent")
		assert.False(t, ok)
		assert.Equal(t, "", value)
	})

	t.Run("Set - overwrite existing key", func(t *testing.T) {
		cache := NewCache[int](nil, time.Hour)
		
		cache.Set("counter", 1, time.Hour)
		cache.Set("counter", 2, time.Hour)
		
		value, ok := cache.Get("counter")
		assert.True(t, ok)
		assert.Equal(t, 2, value)
	})

	t.Run("Set - use zero expiration time (should use default)", func(t *testing.T) {
		cache := NewCache[string](nil, time.Hour)
		
		cache.Set("key", "value", 0)
		
		value, ok := cache.Get("key")
		assert.True(t, ok)
		assert.Equal(t, "value", value)
	})

	t.Run("different value types", func(t *testing.T) {
		// string type
		cacheStr := NewCache[string](nil, time.Hour)
		cacheStr.Set("k", "v", time.Hour)
		v1, ok := cacheStr.Get("k")
		assert.True(t, ok)
		assert.Equal(t, "v", v1)

		// int type
		cacheInt := NewCache[int](nil, time.Hour)
		cacheInt.Set("k", 42, time.Hour)
		v2, ok := cacheInt.Get("k")
		assert.True(t, ok)
		assert.Equal(t, 42, v2)

		// struct type
		type TestStruct struct {
			Name string
			Age  int
		}
		cacheStruct := NewCache[TestStruct](nil, time.Hour)
		testVal := TestStruct{Name: "Alice", Age: 30}
		cacheStruct.Set("k", testVal, time.Hour)
		v3, ok := cacheStruct.Get("k")
		assert.True(t, ok)
		assert.Equal(t, testVal, v3)
	})
}

func TestCache_Expiration(t *testing.T) {
	t.Run("expired items cannot be retrieved", func(t *testing.T) {
		cache := NewCache[string](nil, time.Hour)
		
		cache.Set("key", "value", 50*time.Millisecond)
		
		// immediate get should succeed
		value, ok := cache.Get("key")
		assert.True(t, ok)
		assert.Equal(t, "value", value)
		
		// wait for expiration
		time.Sleep(100 * time.Millisecond)
		
		// get should fail
		_, ok = cache.Get("key")
		assert.False(t, ok)
	})

	t.Run("never expire (defaultExpire<=0)", func(t *testing.T) {
		cache := NewCache[string](nil, 0) // defaultExpire is 0
		
		cache.Set("key", "value", time.Hour) // explicitly set long expiration time
		
		time.Sleep(10 * time.Millisecond)
		
		// will not expire because long expiration time was set
		value, ok := cache.Get("key")
		assert.True(t, ok)
		assert.Equal(t, "value", value)
	})
}

func TestCache_Delete(t *testing.T) {
	t.Run("delete existing item", func(t *testing.T) {
		cache := NewCache[string](nil, time.Hour)
		
		cache.Set("key", "value", time.Hour)
		cache.Delete("key")
		
		_, ok := cache.Get("key")
		assert.False(t, ok)
	})

	t.Run("delete non-existent item", func(t *testing.T) {
		cache := NewCache[string](nil, time.Hour)
		
		// should not panic
		cache.Delete("non-existent")
		
		assert.Equal(t, 0, cache.Len())
	})
}

func TestCache_Len(t *testing.T) {
	t.Run("Len - empty cache", func(t *testing.T) {
		cache := NewCache[string](nil, time.Hour)
		assert.Equal(t, 0, cache.Len())
	})

	t.Run("Len - after adding items", func(t *testing.T) {
		cache := NewCache[string](nil, time.Hour)
		
		cache.Set("key1", "value1", time.Hour)
		assert.Equal(t, 1, cache.Len())
		
		cache.Set("key2", "value2", time.Hour)
		assert.Equal(t, 2, cache.Len())
		
		cache.Set("key3", "value3", time.Hour)
		assert.Equal(t, 3, cache.Len())
	})

	t.Run("Len - after deleting items", func(t *testing.T) {
		cache := NewCache[string](nil, time.Hour)
		
		cache.Set("key1", "value1", time.Hour)
		cache.Set("key2", "value2", time.Hour)
		assert.Equal(t, 2, cache.Len())
		
		cache.Delete("key1")
		assert.Equal(t, 1, cache.Len())
	})
}

func TestCache_Upsert(t *testing.T) {
	t.Run("Upsert - insert new item", func(t *testing.T) {
		cache := NewCache[int](nil, time.Hour)
		
		cache.Upsert("counter", func(old Item[int]) Item[int] {
			return Item[int]{Value: 1}
		})
		
		value, ok := cache.Get("counter")
		assert.True(t, ok)
		assert.Equal(t, 1, value)
	})

	t.Run("Upsert - update existing item", func(t *testing.T) {
		cache := NewCache[int](nil, time.Hour)
		
		cache.Set("counter", 5, time.Hour)
		
		cache.Upsert("counter", func(old Item[int]) Item[int] {
			return Item[int]{Value: old.Value + 1}
		})
		
		value, ok := cache.Get("counter")
		assert.True(t, ok)
		assert.Equal(t, 6, value)
	})

	t.Run("Upsert - multiple increments", func(t *testing.T) {
		cache := NewCache[int](nil, time.Hour)
		
		for i := 0; i < 10; i++ {
			cache.Upsert("counter", func(old Item[int]) Item[int] {
				return Item[int]{Value: old.Value + 1}
			})
		}
		
		value, ok := cache.Get("counter")
		assert.True(t, ok)
		assert.Equal(t, 10, value)
	})

	t.Run("Upsert - refresh expiration time", func(t *testing.T) {
		cache := NewCache[int](nil, 100*time.Millisecond)
		
		cache.Set("key", 1, 50*time.Millisecond)
		
		time.Sleep(30 * time.Millisecond)
		
		// Upsert will refresh expiration time
		cache.Upsert("key", func(old Item[int]) Item[int] {
			return Item[int]{Value: old.Value + 1}
		})
		
		time.Sleep(60 * time.Millisecond)
		
		// item should still exist because Upsert refreshed the expiration time
		value, ok := cache.Get("key")
		assert.True(t, ok)
		assert.Equal(t, 2, value)
	})
}

func TestCache_Range(t *testing.T) {
	t.Run("Range - iterate all items", func(t *testing.T) {
		cache := NewCache[int](nil, time.Hour)
		
		cache.Set("a", 1, time.Hour)
		cache.Set("b", 2, time.Hour)
		cache.Set("c", 3, time.Hour)
		
		visited := make(map[string]int)
		cache.Range(func(key string, value int) bool {
			visited[key] = value
			return true
		})
		
		assert.Equal(t, 3, len(visited))
		assert.Equal(t, 1, visited["a"])
		assert.Equal(t, 2, visited["b"])
		assert.Equal(t, 3, visited["c"])
	})

	t.Run("Range - early termination", func(t *testing.T) {
		cache := NewCache[int](nil, time.Hour)
		
		cache.Set("a", 1, time.Hour)
		cache.Set("b", 2, time.Hour)
		cache.Set("c", 3, time.Hour)
		
		count := 0
		cache.Range(func(key string, value int) bool {
			count++
			return count < 2 // only iterate 2 items
		})
		
		assert.Equal(t, 2, count)
	})

	t.Run("Range - auto cleanup expired items", func(t *testing.T) {
		callbackCount := 0
		cache := NewCache[string](func(key string, value string) {
			callbackCount++
		}, 50*time.Millisecond)
		
		cache.Set("active", "value1", time.Hour)
		cache.Set("expired", "value2", 30*time.Millisecond)
		
		time.Sleep(60 * time.Millisecond)
		
		visited := make(map[string]string)
		cache.Range(func(key string, value string) bool {
			visited[key] = value
			return true
		})
		
		// should only visit non-expired items
		assert.Equal(t, 1, len(visited))
		assert.Equal(t, "value1", visited["active"])
		
		// expiration callback should be called
		assert.Equal(t, 1, callbackCount)
		
		// expired items should be removed from cache
		assert.Equal(t, 1, cache.Len())
	})

	t.Run("Range - empty cache", func(t *testing.T) {
		cache := NewCache[int](nil, time.Hour)
		
		count := 0
		cache.Range(func(key string, value int) bool {
			count++
			return true
		})
		
		assert.Equal(t, 0, count)
	})
}

func TestCache_ConcurrentAccess(t *testing.T) {
	t.Run("concurrent Set and Get", func(t *testing.T) {
		cache := NewCache[int](nil, time.Hour)
		
		var wg sync.WaitGroup
		numGoroutines := 100
		numOperations := 1000
		
		// concurrent writes
		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				for j := 0; j < numOperations; j++ {
					cache.Set(fmt.Sprintf("key-%d", id), j, time.Hour)
				}
			}(i)
		}
		
		// concurrent reads
		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				for j := 0; j < numOperations; j++ {
					cache.Get(fmt.Sprintf("key-%d", id))
				}
			}(i)
		}
		
		wg.Wait()
		
		// verify all keys were written
		assert.Equal(t, numGoroutines, cache.Len())
	})

	t.Run("concurrent Upsert", func(t *testing.T) {
		cache := NewCache[int](nil, time.Hour)
		
		var wg sync.WaitGroup
		numGoroutines := 100
		
		cache.Set("counter", 0, time.Hour)
		
		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				cache.Upsert("counter", func(old Item[int]) Item[int] {
					return Item[int]{Value: old.Value + 1}
				})
			}()
		}
		
		wg.Wait()
		
		value, ok := cache.Get("counter")
		assert.True(t, ok)
		assert.Equal(t, numGoroutines, value)
	})

	t.Run("concurrent Range and Delete", func(t *testing.T) {
		cache := NewCache[int](nil, time.Hour)
		
		// pre-populate data
		for i := 0; i < 100; i++ {
			cache.Set(fmt.Sprintf("key-%d", i), i, time.Hour)
		}
		
		var wg sync.WaitGroup
		
		// concurrent Range
		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				cache.Range(func(key string, value int) bool {
					return true
				})
			}()
		}
		
		// concurrent Delete
		for i := 0; i < 50; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				cache.Delete(fmt.Sprintf("key-%d", id))
			}(i)
		}
		
		wg.Wait()
		
		// should have 50 items remaining
		assert.Equal(t, 50, cache.Len())
	})
}

func TestCache_ExpireCallback(t *testing.T) {
	t.Run("expiration callback is correctly invoked", func(t *testing.T) {
		expiredKeys := make([]string, 0)
		expiredValues := make([]string, 0)
		var mu sync.Mutex
		
		callback := func(key string, value string) {
			mu.Lock()
			defer mu.Unlock()
			expiredKeys = append(expiredKeys, key)
			expiredValues = append(expiredValues, value)
		}
		
		cache := NewCache[string](callback, 50*time.Millisecond)
		
		cache.Set("key1", "value1", 30*time.Millisecond)
		cache.Set("key2", "value2", time.Hour)
		
		time.Sleep(60 * time.Millisecond)
		
		// trigger Range to cleanup expired items
		cache.Range(func(key string, value string) bool {
			return true
		})
		
		mu.Lock()
		assert.Equal(t, 1, len(expiredKeys))
		assert.Contains(t, expiredKeys, "key1")
		assert.Contains(t, expiredValues, "value1")
		mu.Unlock()
	})

	t.Run("no panic when no expiration callback", func(t *testing.T) {
		cache := NewCache[string](nil, 50*time.Millisecond)
		
		cache.Set("key", "value", 30*time.Millisecond)
		
		time.Sleep(60 * time.Millisecond)
		
		// should not panic
		cache.Range(func(key string, value string) bool {
			return true
		})
	})
}

