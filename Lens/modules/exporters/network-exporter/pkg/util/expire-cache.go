// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package util

import (
	"sync"
	"time"
)

var (
	defaultDuration = 5 * time.Minute
)

type Item[T any] struct {
	Value         T
	Expire        time.Time
	defaultExpire time.Duration
}

type Cache[T any] struct {
	Items          map[string]Item[T]
	lock           sync.RWMutex
	expireCallback func(key string, value T)
	defaultExpire  time.Duration
}

func NewCache[T any](expireCallback func(key string, value T), defaultExpire time.Duration) *Cache[T] {
	c := &Cache[T]{
		Items:          make(map[string]Item[T]),
		expireCallback: expireCallback,
		defaultExpire:  defaultDuration,
	}

	if defaultExpire >= 0 {
		c.defaultExpire = defaultExpire
	}
	return c
}

func (c *Cache[T]) Set(key string, value T, expire time.Duration) {
	if expire == 0 {
		expire = defaultDuration
	}
	c.lock.Lock()
	defer c.lock.Unlock()
	c.Items[key] = Item[T]{
		Value:  value,
		Expire: time.Now().Add(expire),
	}
}

func (c *Cache[T]) Upsert(key string, updateFunc func(old Item[T]) Item[T]) {
	c.lock.Lock()
	defer c.lock.Unlock()
	old, ok := c.Items[key]
	if !ok {
		old = Item[T]{}
	}
	newItem := updateFunc(old)
	newItem.Expire = time.Now().Add(c.defaultExpire)
	c.Items[key] = newItem
}

func (c *Cache[T]) Get(key string) (T, bool) {
	c.lock.RLock()
	defer c.lock.RUnlock()
	item, ok := c.Items[key]
	if !ok || c.isExpired(item) {
		t := Item[T]{}
		return t.Value, false
	}
	return item.Value, true
}

func (c *Cache[T]) Delete(key string) {
	c.lock.Lock()
	defer c.lock.Unlock()
	delete(c.Items, key)
}

func (c *Cache[T]) Len() int {
	c.lock.RLock()
	defer c.lock.RUnlock()
	return len(c.Items)
}

func (c *Cache[T]) Range(fn func(key string, value T) bool) {
	c.lock.Lock()
	defer c.lock.Unlock()

	for key, item := range c.Items {
		if c.isExpired(item) {
			if c.expireCallback != nil {
				c.expireCallback(key, item.Value)
			}
			delete(c.Items, key)
			continue
		}
		if !fn(key, item.Value) {
			break
		}
	}
}

func (c *Cache[T]) isExpired(item Item[T]) bool {
	if c.defaultExpire <= 0 {
		return false
	}
	return item.Expire.Before(time.Now())
}
