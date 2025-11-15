/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package utils

import (
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

// MockObject is a mock implementation of the Object interface for testing
type MockObject struct {
	id           string
	released     bool
	releaseError error
	mu           sync.Mutex
}

func (m *MockObject) Release() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.released = true
	return m.releaseError
}

func (m *MockObject) IsReleased() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.released
}

// TestNewObjectManager tests ObjectManager creation
func TestNewObjectManager(t *testing.T) {
	om := NewObjectManager()
	assert.NotNil(t, om)
	assert.NotNil(t, om.objects)
	assert.Equal(t, 0, om.Len())
}

// TestObjectManagerAdd tests adding objects
func TestObjectManagerAdd(t *testing.T) {
	om := NewObjectManager()

	obj1 := &MockObject{id: "obj1"}
	err := om.Add("obj1", obj1)
	assert.NoError(t, err)
	assert.Equal(t, 1, om.Len())

	// Try to add duplicate
	obj2 := &MockObject{id: "obj1"}
	err = om.Add("obj1", obj2)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")
	assert.Equal(t, 1, om.Len())

	// Add different object
	obj3 := &MockObject{id: "obj3"}
	err = om.Add("obj3", obj3)
	assert.NoError(t, err)
	assert.Equal(t, 2, om.Len())
}

// TestObjectManagerAddOrReplace tests adding or replacing objects
func TestObjectManagerAddOrReplace(t *testing.T) {
	om := NewObjectManager()

	obj1 := &MockObject{id: "obj1"}
	om.AddOrReplace("obj1", obj1)
	assert.Equal(t, 1, om.Len())

	// Replace with new object
	obj2 := &MockObject{id: "obj1-new"}
	om.AddOrReplace("obj1", obj2)
	assert.Equal(t, 1, om.Len())
	assert.True(t, obj1.IsReleased(), "Old object should be released")

	// Verify new object is stored
	retrieved, exists := om.Get("obj1")
	assert.True(t, exists)
	assert.Equal(t, obj2, retrieved)
}

// TestObjectManagerAddOrReplaceWithError tests replacement when release fails
func TestObjectManagerAddOrReplaceWithError(t *testing.T) {
	om := NewObjectManager()

	obj1 := &MockObject{id: "obj1", releaseError: fmt.Errorf("release failed")}
	om.AddOrReplace("obj1", obj1)

	obj2 := &MockObject{id: "obj1-new"}
	om.AddOrReplace("obj1", obj2) // Should still replace despite error

	assert.True(t, obj1.IsReleased())
	retrieved, _ := om.Get("obj1")
	assert.Equal(t, obj2, retrieved)
}

// TestObjectManagerGet tests retrieving objects
func TestObjectManagerGet(t *testing.T) {
	om := NewObjectManager()

	obj1 := &MockObject{id: "obj1"}
	om.AddOrReplace("obj1", obj1)

	// Get existing object
	retrieved, exists := om.Get("obj1")
	assert.True(t, exists)
	assert.Equal(t, obj1, retrieved)

	// Get non-existing object
	retrieved, exists = om.Get("nonexistent")
	assert.False(t, exists)
	assert.Nil(t, retrieved)
}

// TestObjectManagerHas tests checking object existence
func TestObjectManagerHas(t *testing.T) {
	om := NewObjectManager()

	obj1 := &MockObject{id: "obj1"}
	om.AddOrReplace("obj1", obj1)

	assert.True(t, om.Has("obj1"))
	assert.False(t, om.Has("nonexistent"))
}

// TestObjectManagerDelete tests deleting objects
func TestObjectManagerDelete(t *testing.T) {
	om := NewObjectManager()

	obj1 := &MockObject{id: "obj1"}
	om.AddOrReplace("obj1", obj1)
	assert.Equal(t, 1, om.Len())

	// Delete existing object
	err := om.Delete("obj1")
	assert.NoError(t, err)
	assert.Equal(t, 0, om.Len())
	assert.True(t, obj1.IsReleased())

	// Delete non-existing object
	err = om.Delete("nonexistent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "does not exist")
}

// TestObjectManagerDeleteWithError tests deletion when release fails
func TestObjectManagerDeleteWithError(t *testing.T) {
	om := NewObjectManager()

	obj1 := &MockObject{id: "obj1", releaseError: fmt.Errorf("release failed")}
	om.AddOrReplace("obj1", obj1)

	err := om.Delete("obj1")
	assert.NoError(t, err) // Delete should succeed even if release fails
	assert.Equal(t, 0, om.Len())
	assert.True(t, obj1.IsReleased())
}

// TestObjectManagerClear tests clearing all objects
func TestObjectManagerClear(t *testing.T) {
	om := NewObjectManager()

	obj1 := &MockObject{id: "obj1"}
	obj2 := &MockObject{id: "obj2"}
	obj3 := &MockObject{id: "obj3"}

	om.AddOrReplace("obj1", obj1)
	om.AddOrReplace("obj2", obj2)
	om.AddOrReplace("obj3", obj3)
	assert.Equal(t, 3, om.Len())

	om.Clear()
	assert.Equal(t, 0, om.Len())
	assert.True(t, obj1.IsReleased())
	assert.True(t, obj2.IsReleased())
	assert.True(t, obj3.IsReleased())
}

// TestObjectManagerClearWithError tests clearing when some releases fail
func TestObjectManagerClearWithError(t *testing.T) {
	om := NewObjectManager()

	obj1 := &MockObject{id: "obj1"}
	obj2 := &MockObject{id: "obj2", releaseError: fmt.Errorf("release failed")}
	obj3 := &MockObject{id: "obj3"}

	om.AddOrReplace("obj1", obj1)
	om.AddOrReplace("obj2", obj2)
	om.AddOrReplace("obj3", obj3)

	om.Clear()
	assert.Equal(t, 0, om.Len())
	assert.True(t, obj1.IsReleased())
	assert.True(t, obj2.IsReleased())
	assert.True(t, obj3.IsReleased())
}

// TestObjectManagerGetAll tests retrieving all objects
func TestObjectManagerGetAll(t *testing.T) {
	om := NewObjectManager()

	obj1 := &MockObject{id: "obj1"}
	obj2 := &MockObject{id: "obj2"}
	obj3 := &MockObject{id: "obj3"}

	om.AddOrReplace("obj1", obj1)
	om.AddOrReplace("obj2", obj2)
	om.AddOrReplace("obj3", obj3)

	keys, objs := om.GetAll()
	assert.Equal(t, 3, len(keys))
	assert.Equal(t, 3, len(objs))

	// Verify all keys are present
	keySet := make(map[string]bool)
	for _, key := range keys {
		keySet[key] = true
	}
	assert.True(t, keySet["obj1"])
	assert.True(t, keySet["obj2"])
	assert.True(t, keySet["obj3"])

	// Verify all objects are present
	objSet := make(map[Object]bool)
	for _, obj := range objs {
		objSet[obj] = true
	}
	assert.True(t, objSet[obj1])
	assert.True(t, objSet[obj2])
	assert.True(t, objSet[obj3])
}

// TestObjectManagerGetAllEmpty tests GetAll on empty manager
func TestObjectManagerGetAllEmpty(t *testing.T) {
	om := NewObjectManager()

	keys, objs := om.GetAll()
	assert.Empty(t, keys)
	assert.Empty(t, objs)
}

// TestObjectManagerLen tests length tracking
func TestObjectManagerLen(t *testing.T) {
	om := NewObjectManager()
	assert.Equal(t, 0, om.Len())

	obj1 := &MockObject{id: "obj1"}
	om.AddOrReplace("obj1", obj1)
	assert.Equal(t, 1, om.Len())

	obj2 := &MockObject{id: "obj2"}
	om.AddOrReplace("obj2", obj2)
	assert.Equal(t, 2, om.Len())

	om.Delete("obj1")
	assert.Equal(t, 1, om.Len())

	om.Clear()
	assert.Equal(t, 0, om.Len())
}

// TestObjectManagerConcurrency tests thread safety
func TestObjectManagerConcurrency(t *testing.T) {
	om := NewObjectManager()
	var wg sync.WaitGroup
	iterations := 100

	// Concurrent adds
	for i := 0; i < iterations; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			obj := &MockObject{id: fmt.Sprintf("obj%d", id)}
			om.AddOrReplace(fmt.Sprintf("obj%d", id), obj)
		}(i)
	}
	wg.Wait()
	assert.Equal(t, iterations, om.Len())

	// Concurrent reads
	for i := 0; i < iterations; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			_, exists := om.Get(fmt.Sprintf("obj%d", id))
			assert.True(t, exists)
		}(i)
	}
	wg.Wait()

	// Concurrent deletes
	for i := 0; i < iterations; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			om.Delete(fmt.Sprintf("obj%d", id))
		}(i)
	}
	wg.Wait()
	assert.Equal(t, 0, om.Len())
}

// TestNewObjectManagerSingleton tests singleton creation
func TestNewObjectManagerSingleton(t *testing.T) {
	// Note: This test might interfere with other tests due to singleton nature
	// In production code, consider using dependency injection instead
	om1 := NewObjectManagerSingleton()
	om2 := NewObjectManagerSingleton()

	assert.NotNil(t, om1)
	assert.NotNil(t, om2)
	assert.Same(t, om1, om2, "Singleton should return same instance")
}

// TestGetK8sClientFactory tests K8s client factory retrieval
func TestGetK8sClientFactory(t *testing.T) {
	om := NewObjectManager()

	// Test with non-existent cluster
	_, err := GetK8sClientFactory(om, "nonexistent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "is not found")

	// Test with wrong object type
	wrongObj := &MockObject{id: "cluster1"}
	om.AddOrReplace("cluster1", wrongObj)
	_, err = GetK8sClientFactory(om, "cluster1")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "type is not matched")

	// Note: Testing with actual ClientFactory would require mocking the entire
	// ClientFactory type, which is complex. The above tests cover the error paths.
}
