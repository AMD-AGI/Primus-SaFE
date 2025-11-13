package util

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNewCache(t *testing.T) {
	t.Run("创建缓存-使用默认过期时间", func(t *testing.T) {
		cache := NewCache[string](nil, 0)
		assert.NotNil(t, cache)
		assert.NotNil(t, cache.Items)
		assert.Equal(t, time.Duration(0), cache.defaultExpire) // 传入0时，defaultExpire为0
	})

	t.Run("创建缓存-自定义过期时间", func(t *testing.T) {
		customExpire := 10 * time.Second
		cache := NewCache[int](nil, customExpire)
		assert.NotNil(t, cache)
		assert.Equal(t, customExpire, cache.defaultExpire)
	})

	t.Run("创建缓存-带过期回调", func(t *testing.T) {
		called := false
		callback := func(key string, value string) {
			called = true
		}
		cache := NewCache[string](callback, time.Second)
		assert.NotNil(t, cache)
		assert.NotNil(t, cache.expireCallback)
		
		// 验证回调未被调用
		assert.False(t, called)
	})

	t.Run("创建缓存-负数过期时间不使用", func(t *testing.T) {
		cache := NewCache[string](nil, -1*time.Second)
		assert.Equal(t, defaultDuration, cache.defaultExpire) // 负数时使用defaultDuration
	})
}

func TestCache_SetAndGet(t *testing.T) {
	t.Run("Set和Get-基本功能", func(t *testing.T) {
		cache := NewCache[string](nil, time.Hour)
		
		cache.Set("key1", "value1", time.Hour)
		
		value, ok := cache.Get("key1")
		assert.True(t, ok)
		assert.Equal(t, "value1", value)
	})

	t.Run("Get-不存在的键", func(t *testing.T) {
		cache := NewCache[string](nil, time.Hour)
		
		value, ok := cache.Get("non-existent")
		assert.False(t, ok)
		assert.Equal(t, "", value)
	})

	t.Run("Set-覆盖已存在的键", func(t *testing.T) {
		cache := NewCache[int](nil, time.Hour)
		
		cache.Set("counter", 1, time.Hour)
		cache.Set("counter", 2, time.Hour)
		
		value, ok := cache.Get("counter")
		assert.True(t, ok)
		assert.Equal(t, 2, value)
	})

	t.Run("Set-使用零值过期时间（应使用默认）", func(t *testing.T) {
		cache := NewCache[string](nil, time.Hour)
		
		cache.Set("key", "value", 0)
		
		value, ok := cache.Get("key")
		assert.True(t, ok)
		assert.Equal(t, "value", value)
	})

	t.Run("不同类型的值", func(t *testing.T) {
		// string类型
		cacheStr := NewCache[string](nil, time.Hour)
		cacheStr.Set("k", "v", time.Hour)
		v1, ok := cacheStr.Get("k")
		assert.True(t, ok)
		assert.Equal(t, "v", v1)

		// int类型
		cacheInt := NewCache[int](nil, time.Hour)
		cacheInt.Set("k", 42, time.Hour)
		v2, ok := cacheInt.Get("k")
		assert.True(t, ok)
		assert.Equal(t, 42, v2)

		// struct类型
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
	t.Run("过期的项不可获取", func(t *testing.T) {
		cache := NewCache[string](nil, time.Hour)
		
		cache.Set("key", "value", 50*time.Millisecond)
		
		// 立即获取应该成功
		value, ok := cache.Get("key")
		assert.True(t, ok)
		assert.Equal(t, "value", value)
		
		// 等待过期
		time.Sleep(100 * time.Millisecond)
		
		// 获取应该失败
		_, ok = cache.Get("key")
		assert.False(t, ok)
	})

	t.Run("永不过期（defaultExpire<=0）", func(t *testing.T) {
		cache := NewCache[string](nil, 0) // defaultExpire为0
		
		cache.Set("key", "value", time.Hour) // 显式设置较长的过期时间
		
		time.Sleep(10 * time.Millisecond)
		
		// 由于设置了较长的过期时间，不会过期
		value, ok := cache.Get("key")
		assert.True(t, ok)
		assert.Equal(t, "value", value)
	})
}

func TestCache_Delete(t *testing.T) {
	t.Run("删除存在的项", func(t *testing.T) {
		cache := NewCache[string](nil, time.Hour)
		
		cache.Set("key", "value", time.Hour)
		cache.Delete("key")
		
		_, ok := cache.Get("key")
		assert.False(t, ok)
	})

	t.Run("删除不存在的项", func(t *testing.T) {
		cache := NewCache[string](nil, time.Hour)
		
		// 不应该panic
		cache.Delete("non-existent")
		
		assert.Equal(t, 0, cache.Len())
	})
}

func TestCache_Len(t *testing.T) {
	t.Run("Len-空缓存", func(t *testing.T) {
		cache := NewCache[string](nil, time.Hour)
		assert.Equal(t, 0, cache.Len())
	})

	t.Run("Len-添加项后", func(t *testing.T) {
		cache := NewCache[string](nil, time.Hour)
		
		cache.Set("key1", "value1", time.Hour)
		assert.Equal(t, 1, cache.Len())
		
		cache.Set("key2", "value2", time.Hour)
		assert.Equal(t, 2, cache.Len())
		
		cache.Set("key3", "value3", time.Hour)
		assert.Equal(t, 3, cache.Len())
	})

	t.Run("Len-删除项后", func(t *testing.T) {
		cache := NewCache[string](nil, time.Hour)
		
		cache.Set("key1", "value1", time.Hour)
		cache.Set("key2", "value2", time.Hour)
		assert.Equal(t, 2, cache.Len())
		
		cache.Delete("key1")
		assert.Equal(t, 1, cache.Len())
	})
}

func TestCache_Upsert(t *testing.T) {
	t.Run("Upsert-插入新项", func(t *testing.T) {
		cache := NewCache[int](nil, time.Hour)
		
		cache.Upsert("counter", func(old Item[int]) Item[int] {
			return Item[int]{Value: 1}
		})
		
		value, ok := cache.Get("counter")
		assert.True(t, ok)
		assert.Equal(t, 1, value)
	})

	t.Run("Upsert-更新已存在的项", func(t *testing.T) {
		cache := NewCache[int](nil, time.Hour)
		
		cache.Set("counter", 5, time.Hour)
		
		cache.Upsert("counter", func(old Item[int]) Item[int] {
			return Item[int]{Value: old.Value + 1}
		})
		
		value, ok := cache.Get("counter")
		assert.True(t, ok)
		assert.Equal(t, 6, value)
	})

	t.Run("Upsert-多次递增", func(t *testing.T) {
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

	t.Run("Upsert-刷新过期时间", func(t *testing.T) {
		cache := NewCache[int](nil, 100*time.Millisecond)
		
		cache.Set("key", 1, 50*time.Millisecond)
		
		time.Sleep(30 * time.Millisecond)
		
		// Upsert 会刷新过期时间
		cache.Upsert("key", func(old Item[int]) Item[int] {
			return Item[int]{Value: old.Value + 1}
		})
		
		time.Sleep(60 * time.Millisecond)
		
		// 由于Upsert刷新了过期时间，项应该仍然存在
		value, ok := cache.Get("key")
		assert.True(t, ok)
		assert.Equal(t, 2, value)
	})
}

func TestCache_Range(t *testing.T) {
	t.Run("Range-遍历所有项", func(t *testing.T) {
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

	t.Run("Range-提前终止遍历", func(t *testing.T) {
		cache := NewCache[int](nil, time.Hour)
		
		cache.Set("a", 1, time.Hour)
		cache.Set("b", 2, time.Hour)
		cache.Set("c", 3, time.Hour)
		
		count := 0
		cache.Range(func(key string, value int) bool {
			count++
			return count < 2 // 只遍历2个项
		})
		
		assert.Equal(t, 2, count)
	})

	t.Run("Range-自动清理过期项", func(t *testing.T) {
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
		
		// 只应该访问未过期的项
		assert.Equal(t, 1, len(visited))
		assert.Equal(t, "value1", visited["active"])
		
		// 过期回调应该被调用
		assert.Equal(t, 1, callbackCount)
		
		// 过期项应该被从缓存中删除
		assert.Equal(t, 1, cache.Len())
	})

	t.Run("Range-空缓存", func(t *testing.T) {
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
	t.Run("并发Set和Get", func(t *testing.T) {
		cache := NewCache[int](nil, time.Hour)
		
		var wg sync.WaitGroup
		numGoroutines := 100
		numOperations := 1000
		
		// 并发写入
		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				for j := 0; j < numOperations; j++ {
					cache.Set(fmt.Sprintf("key-%d", id), j, time.Hour)
				}
			}(i)
		}
		
		// 并发读取
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
		
		// 验证所有键都被写入
		assert.Equal(t, numGoroutines, cache.Len())
	})

	t.Run("并发Upsert", func(t *testing.T) {
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

	t.Run("并发Range和Delete", func(t *testing.T) {
		cache := NewCache[int](nil, time.Hour)
		
		// 预先填充数据
		for i := 0; i < 100; i++ {
			cache.Set(fmt.Sprintf("key-%d", i), i, time.Hour)
		}
		
		var wg sync.WaitGroup
		
		// 并发Range
		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				cache.Range(func(key string, value int) bool {
					return true
				})
			}()
		}
		
		// 并发Delete
		for i := 0; i < 50; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				cache.Delete(fmt.Sprintf("key-%d", id))
			}(i)
		}
		
		wg.Wait()
		
		// 应该剩余50个项
		assert.Equal(t, 50, cache.Len())
	})
}

func TestCache_ExpireCallback(t *testing.T) {
	t.Run("过期回调被正确调用", func(t *testing.T) {
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
		
		// 触发Range来清理过期项
		cache.Range(func(key string, value string) bool {
			return true
		})
		
		mu.Lock()
		assert.Equal(t, 1, len(expiredKeys))
		assert.Contains(t, expiredKeys, "key1")
		assert.Contains(t, expiredValues, "value1")
		mu.Unlock()
	})

	t.Run("无过期回调时不会panic", func(t *testing.T) {
		cache := NewCache[string](nil, 50*time.Millisecond)
		
		cache.Set("key", "value", 30*time.Millisecond)
		
		time.Sleep(60 * time.Millisecond)
		
		// 不应该panic
		cache.Range(func(key string, value string) bool {
			return true
		})
	})
}

