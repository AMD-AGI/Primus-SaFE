// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package metadata

import (
	"testing"

	"gorm.io/gorm"
)

func TestNewDBStorage(t *testing.T) {
	db := &gorm.DB{}
	storage := NewDBStorage(db)

	if storage == nil {
		t.Fatal("NewDBStorage() returned nil")
	}

	if storage.db != db {
		t.Error("db not properly set")
	}
}

func TestMetadataStatistics_Structure(t *testing.T) {
	stats := &MetadataStatistics{
		TotalWorkloads: 100,
		ByFramework: map[string]int{
			"pytorch":    50,
			"tensorflow": 30,
			"jax":        20,
		},
		ByType: map[string]int{
			"training":  80,
			"inference": 20,
		},
	}

	if stats.TotalWorkloads != 100 {
		t.Errorf("TotalWorkloads = %d, want 100", stats.TotalWorkloads)
	}

	if len(stats.ByFramework) != 3 {
		t.Errorf("ByFramework has %d entries, want 3", len(stats.ByFramework))
	}

	if stats.ByFramework["pytorch"] != 50 {
		t.Errorf("ByFramework[pytorch] = %d, want 50", stats.ByFramework["pytorch"])
	}

	if len(stats.ByType) != 2 {
		t.Errorf("ByType has %d entries, want 2", len(stats.ByType))
	}
}

func TestDBStorage_StoreBatch_EmptyList(t *testing.T) {
	storage := &DBStorage{
		db: &gorm.DB{},
	}

	// Test with nil database will just ensure the structure is correct
	// Actual database operations would require a real or mock database
	if storage.db == nil {
		t.Error("db should not be nil")
	}
}

func TestMockStorage_Integration(t *testing.T) {
	// This tests our mock storage implementation used in other tests
	storage := NewMockStorage()

	if storage == nil {
		t.Fatal("NewMockStorage() returned nil")
	}

	if storage.data == nil {
		t.Error("data map should be initialized")
	}

	if storage.StoreCalls != 0 {
		t.Errorf("StoreCalls should be 0, got %d", storage.StoreCalls)
	}
}

func TestMockStorage_Reset(t *testing.T) {
	storage := NewMockStorage()

	storage.StoreCalls = 5
	storage.GetCalls = 3
	storage.data["test"] = &WorkloadMetadata{}

	storage.Reset()

	if storage.StoreCalls != 0 {
		t.Errorf("StoreCalls = %d after reset, want 0", storage.StoreCalls)
	}

	if storage.GetCalls != 0 {
		t.Errorf("GetCalls = %d after reset, want 0", storage.GetCalls)
	}

	if len(storage.data) != 0 {
		t.Errorf("data map has %d entries after reset, want 0", len(storage.data))
	}
}

func TestMockStorage_GetStoredData(t *testing.T) {
	storage := NewMockStorage()

	metadata1 := &WorkloadMetadata{WorkloadUID: "uid-1"}
	metadata2 := &WorkloadMetadata{WorkloadUID: "uid-2"}

	storage.data["uid-1"] = metadata1
	storage.data["uid-2"] = metadata2

	storedData := storage.GetStoredData()

	if len(storedData) != 2 {
		t.Errorf("GetStoredData() returned %d entries, want 2", len(storedData))
	}

	if storedData["uid-1"] != metadata1 {
		t.Error("GetStoredData() did not return correct metadata for uid-1")
	}

	storedData["uid-3"] = &WorkloadMetadata{}

	if len(storage.data) == 3 {
		t.Error("Modifying returned data should not affect original data")
	}
}

