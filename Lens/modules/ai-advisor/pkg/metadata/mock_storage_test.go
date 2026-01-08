// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package metadata

import (
	"context"
	"sync"
)

// MockStorage is a mock implementation of Storage interface for testing
type MockStorage struct {
	mu       sync.RWMutex
	data     map[string]*WorkloadMetadata
	StoreErr error
	GetErr   error
	QueryErr error
	DeleteErr error
	
	StoreCalls  int
	GetCalls    int
	QueryCalls  int
	DeleteCalls int
}

// NewMockStorage creates a new mock storage
func NewMockStorage() *MockStorage {
	return &MockStorage{
		data: make(map[string]*WorkloadMetadata),
	}
}

// Store stores workload metadata
func (m *MockStorage) Store(ctx context.Context, metadata *WorkloadMetadata) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	m.StoreCalls++
	
	if m.StoreErr != nil {
		return m.StoreErr
	}
	
	m.data[metadata.WorkloadUID] = metadata
	return nil
}

// Get retrieves workload metadata
func (m *MockStorage) Get(ctx context.Context, workloadUID string) (*WorkloadMetadata, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	m.GetCalls++
	
	if m.GetErr != nil {
		return nil, m.GetErr
	}
	
	return m.data[workloadUID], nil
}

// Query queries workload metadata
func (m *MockStorage) Query(ctx context.Context, query *MetadataQuery) ([]*WorkloadMetadata, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	m.QueryCalls++
	
	if m.QueryErr != nil {
		return nil, m.QueryErr
	}
	
	var results []*WorkloadMetadata
	for _, metadata := range m.data {
		if query.WorkloadUID != "" && metadata.WorkloadUID != query.WorkloadUID {
			continue
		}
		if query.Framework != "" && metadata.BaseFramework != query.Framework {
			continue
		}
		results = append(results, metadata)
	}
	
	if query.Limit > 0 && len(results) > query.Limit {
		results = results[:query.Limit]
	}
	
	return results, nil
}

// Delete deletes workload metadata
func (m *MockStorage) Delete(ctx context.Context, workloadUID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	m.DeleteCalls++
	
	if m.DeleteErr != nil {
		return m.DeleteErr
	}
	
	delete(m.data, workloadUID)
	return nil
}

// Reset resets the mock state
func (m *MockStorage) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	m.data = make(map[string]*WorkloadMetadata)
	m.StoreErr = nil
	m.GetErr = nil
	m.QueryErr = nil
	m.DeleteErr = nil
	m.StoreCalls = 0
	m.GetCalls = 0
	m.QueryCalls = 0
	m.DeleteCalls = 0
}

// GetStoredData returns all stored data
func (m *MockStorage) GetStoredData() map[string]*WorkloadMetadata {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	dataCopy := make(map[string]*WorkloadMetadata)
	for k, v := range m.data {
		dataCopy[k] = v
	}
	return dataCopy
}

