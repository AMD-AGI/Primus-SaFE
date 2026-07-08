// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.

package util

import (
	"sync"
	"testing"
	"time"
)

func TestCacheSetAndGet(t *testing.T) {
	c := NewCache[string](nil, 5*time.Minute)

	c.Set("key1", "value1", 5*time.Minute)

	val, ok := c.Get("key1")
	if !ok {
		t.Fatal("expected key1 to exist")
	}
	if val != "value1" {
		t.Errorf("expected value1, got %s", val)
	}
}

func TestCacheGetMissing(t *testing.T) {
	c := NewCache[string](nil, 5*time.Minute)

	_, ok := c.Get("nonexistent")
	if ok {
		t.Error("expected key to not exist")
	}
}

func TestCacheDelete(t *testing.T) {
	c := NewCache[string](nil, 5*time.Minute)

	c.Set("key1", "value1", 5*time.Minute)
	c.Delete("key1")

	_, ok := c.Get("key1")
	if ok {
		t.Error("expected key1 to be deleted")
	}
}

func TestCacheLen(t *testing.T) {
	c := NewCache[int](nil, 5*time.Minute)

	if c.Len() != 0 {
		t.Errorf("expected length 0, got %d", c.Len())
	}

	c.Set("a", 1, 5*time.Minute)
	c.Set("b", 2, 5*time.Minute)
	c.Set("c", 3, 5*time.Minute)

	if c.Len() != 3 {
		t.Errorf("expected length 3, got %d", c.Len())
	}
}

func TestCacheExpiration(t *testing.T) {
	c := NewCache[string](nil, 50*time.Millisecond)

	c.Set("key1", "value1", 50*time.Millisecond)

	// Should exist immediately
	val, ok := c.Get("key1")
	if !ok {
		t.Fatal("expected key1 to exist immediately")
	}
	if val != "value1" {
		t.Errorf("expected value1, got %s", val)
	}

	// Wait for expiration
	time.Sleep(100 * time.Millisecond)

	_, ok = c.Get("key1")
	if ok {
		t.Error("expected key1 to be expired")
	}
}

func TestCacheUpsert(t *testing.T) {
	c := NewCache[int](nil, 5*time.Minute)

	// Insert new
	c.Upsert("counter", func(old Item[int]) Item[int] {
		return Item[int]{Value: old.Value + 1}
	})

	val, ok := c.Get("counter")
	if !ok {
		t.Fatal("expected counter to exist")
	}
	if val != 1 {
		t.Errorf("expected 1, got %d", val)
	}

	// Update existing
	c.Upsert("counter", func(old Item[int]) Item[int] {
		return Item[int]{Value: old.Value + 10}
	})

	val, ok = c.Get("counter")
	if !ok {
		t.Fatal("expected counter to exist")
	}
	if val != 11 {
		t.Errorf("expected 11, got %d", val)
	}
}

func TestCacheRange(t *testing.T) {
	c := NewCache[string](nil, 5*time.Minute)

	c.Set("a", "alpha", 5*time.Minute)
	c.Set("b", "beta", 5*time.Minute)
	c.Set("c", "gamma", 5*time.Minute)

	visited := make(map[string]string)
	c.Range(func(key string, value string) bool {
		visited[key] = value
		return true
	})

	if len(visited) != 3 {
		t.Errorf("expected 3 items visited, got %d", len(visited))
	}
	if visited["a"] != "alpha" {
		t.Errorf("expected alpha, got %s", visited["a"])
	}
}

func TestCacheRangeStopEarly(t *testing.T) {
	c := NewCache[int](nil, 5*time.Minute)

	c.Set("a", 1, 5*time.Minute)
	c.Set("b", 2, 5*time.Minute)
	c.Set("c", 3, 5*time.Minute)

	count := 0
	c.Range(func(key string, value int) bool {
		count++
		return false // stop after first
	})

	if count != 1 {
		t.Errorf("expected 1 iteration, got %d", count)
	}
}

func TestCacheRangeExpiredCleanup(t *testing.T) {
	var expiredKeys []string
	c := NewCache[string](func(key string, value string) {
		expiredKeys = append(expiredKeys, key)
	}, 50*time.Millisecond)

	c.Set("expire-me", "value", 50*time.Millisecond)
	c.Set("keep-me", "value", 5*time.Minute)

	// Wait for expiration of first item
	time.Sleep(100 * time.Millisecond)

	c.Range(func(key string, value string) bool {
		return true
	})

	// The expired item should have been cleaned up
	_, ok := c.Get("expire-me")
	if ok {
		t.Error("expected expired item to be cleaned up")
	}

	// The non-expired item should still exist
	_, ok = c.Get("keep-me")
	if !ok {
		t.Error("expected keep-me to still exist")
	}
}

func TestCacheSetDefaultExpire(t *testing.T) {
	c := NewCache[string](nil, 5*time.Minute)

	// Set with 0 duration should use defaultDuration
	c.Set("key1", "value1", 0)

	val, ok := c.Get("key1")
	if !ok {
		t.Fatal("expected key1 to exist")
	}
	if val != "value1" {
		t.Errorf("expected value1, got %s", val)
	}
}

func TestCacheConcurrency(t *testing.T) {
	c := NewCache[int](nil, 5*time.Minute)

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			key := "key"
			c.Set(key, n, 5*time.Minute)
			c.Get(key)
		}(i)
	}
	wg.Wait()

	// Just verify no panic/race
	_, _ = c.Get("key")
}

func TestNewCacheNegativeExpire(t *testing.T) {
	// Negative defaultExpire should disable expiration
	c := NewCache[string](nil, -1)

	c.Set("key1", "value1", 0)

	val, ok := c.Get("key1")
	if !ok {
		t.Fatal("expected key1 to exist with negative expire")
	}
	if val != "value1" {
		t.Errorf("expected value1, got %s", val)
	}
}

