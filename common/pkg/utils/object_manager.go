/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package utils

import (
	"fmt"
	"sync"

	"k8s.io/klog/v2"
)

var (
	once          sync.Once
	objectManager *ObjectManager
)

type Object interface {
	Release() error // Method to clean up resources when the object is released
}

// ObjectManager manages a collection of Objects.
type ObjectManager struct {
	objects map[string]Object
	mu      sync.RWMutex
}

func NewObjectManager() *ObjectManager {
	return &ObjectManager{
		objects: make(map[string]Object),
	}
}

// NewObjectManagerSingleton creates and returns a new ObjectManager.
// It is a singleton, with only one instance running per service
func NewObjectManagerSingleton() *ObjectManager {
	once.Do(func() {
		objectManager = &ObjectManager{
			objects: make(map[string]Object),
		}
	})
	return objectManager
}

// AddOrReplace adds a new object to the manager.
// if the object already exists, the old one will be released and replaced
func (om *ObjectManager) AddOrReplace(id string, obj Object) {
	om.mu.Lock()
	defer om.mu.Unlock()
	if oldObject, exists := om.objects[id]; exists {
		if err := oldObject.Release(); err != nil {
			klog.ErrorS(err, "failed to release object", "id", id)
		}
	}
	om.objects[id] = obj
}

// Add adds a new object to the manager.
// if the object already exists, return the error
func (om *ObjectManager) Add(id string, obj Object) error {
	om.mu.Lock()
	defer om.mu.Unlock()
	if _, exists := om.objects[id]; exists {
		return fmt.Errorf("object %s already exists", id)
	}
	om.objects[id] = obj
	return nil
}

// Delete deletes a object from the manager and calls its release method.
func (om *ObjectManager) Delete(id string) error {
	om.mu.Lock()
	defer om.mu.Unlock()
	obj, exists := om.objects[id]
	if !exists {
		return fmt.Errorf("object %s does not exist", id)
	}
	if err := obj.Release(); err != nil {
		klog.ErrorS(err, "failed to release object", "id", id)
	}
	delete(om.objects, id)
	return nil
}

// Clear clear all objects of the manager
func (om *ObjectManager) Clear() {
	om.mu.Lock()
	defer om.mu.Unlock()
	for id, obj := range om.objects {
		if err := obj.Release(); err != nil {
			klog.ErrorS(err, "failed to release object", "id", id)
		}
	}
	clear(om.objects)
}

// Get retrieves a object by id.
func (om *ObjectManager) Get(id string) (Object, bool) {
	om.mu.RLock()
	defer om.mu.RUnlock()
	obj, exists := om.objects[id]
	return obj, exists
}

// Has checks if a object exists in the manager.
func (om *ObjectManager) Has(id string) bool {
	om.mu.RLock()
	defer om.mu.RUnlock()
	_, exists := om.objects[id]
	return exists
}

// GetAll retrieves all Objects and keys
func (om *ObjectManager) GetAll() ([]string, []Object) {
	om.mu.RLock()
	defer om.mu.RUnlock()
	keys := make([]string, 0, len(om.objects))
	objs := make([]Object, 0, len(om.objects))
	for id, obj := range om.objects {
		keys = append(keys, id)
		objs = append(objs, obj)
	}
	return keys, objs
}

// Len returns the number of objects currently managed by the ObjectManager
func (om *ObjectManager) Len() int {
	om.mu.RLock()
	defer om.mu.RUnlock()
	return len(om.objects)
}
