// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package metadata

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
)

func TestNewFacadeStorage(t *testing.T) {
	facade := NewMockAiWorkloadMetadataFacade()
	storage := NewFacadeStorage(facade)

	if storage == nil {
		t.Fatal("NewFacadeStorage() returned nil")
	}

	if storage.facade != facade {
		t.Error("facade not properly set")
	}
}

func TestFacadeStorage_Store_Create(t *testing.T) {
	facade := NewMockAiWorkloadMetadataFacade()
	storage := NewFacadeStorage(facade)
	ctx := context.Background()

	metadata := &WorkloadMetadata{
		WorkloadUID:   "test-uid-123",
		PodName:       "test-pod",
		PodNamespace:  "default",
		NodeName:      "node-1",
		Frameworks:    []string{"pytorch"},
		BaseFramework: "pytorch",
		CollectedAt:   time.Now(),
	}

	err := storage.Store(ctx, metadata)
	if err != nil {
		t.Fatalf("Store() error = %v", err)
	}

	if facade.CreateCalls != 1 {
		t.Errorf("CreateAiWorkloadMetadata called %d times, want 1", facade.CreateCalls)
	}

	storedMetadata, exists := facade.Metadata["test-uid-123"]
	if !exists {
		t.Fatal("Metadata not stored in facade")
	}

	if storedMetadata.WorkloadUID != "test-uid-123" {
		t.Errorf("WorkloadUID = %v, want test-uid-123", storedMetadata.WorkloadUID)
	}

	if storedMetadata.Framework != "pytorch" {
		t.Errorf("Framework = %v, want pytorch", storedMetadata.Framework)
	}
}

func TestFacadeStorage_Store_Update(t *testing.T) {
	facade := NewMockAiWorkloadMetadataFacade()
	storage := NewFacadeStorage(facade)
	ctx := context.Background()

	existingMetadata := &model.AiWorkloadMetadata{
		ID:          1,
		WorkloadUID: "test-uid-123",
		Framework:   "pytorch",
		Type:        "training",
	}
	facade.Metadata["test-uid-123"] = existingMetadata

	metadata := &WorkloadMetadata{
		WorkloadUID:   "test-uid-123",
		PodName:       "test-pod",
		PodNamespace:  "default",
		Frameworks:    []string{"tensorflow"},
		BaseFramework: "tensorflow",
		CollectedAt:   time.Now(),
	}

	err := storage.Store(ctx, metadata)
	if err != nil {
		t.Fatalf("Store() error = %v", err)
	}

	if facade.UpdateCalls != 1 {
		t.Errorf("UpdateAiWorkloadMetadata called %d times, want 1", facade.UpdateCalls)
	}

	updatedMetadata := facade.Metadata["test-uid-123"]
	if updatedMetadata.Framework != "tensorflow" {
		t.Errorf("Framework = %v, want tensorflow", updatedMetadata.Framework)
	}
}

func TestFacadeStorage_Get_Success(t *testing.T) {
	facade := NewMockAiWorkloadMetadataFacade()
	storage := NewFacadeStorage(facade)
	ctx := context.Background()

	originalMetadata := &WorkloadMetadata{
		WorkloadUID:  "test-uid-123",
		PodName:      "test-pod",
		PodNamespace: "default",
		Frameworks:   []string{"pytorch"},
	}

	metadataJSON, _ := json.Marshal(originalMetadata)
	var metadataMap map[string]interface{}
	json.Unmarshal(metadataJSON, &metadataMap)

	facade.Metadata["test-uid-123"] = &model.AiWorkloadMetadata{
		WorkloadUID: "test-uid-123",
		Framework:   "pytorch",
		Metadata:    model.ExtType(metadataMap),
	}

	retrieved, err := storage.Get(ctx, "test-uid-123")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	if retrieved == nil {
		t.Fatal("Get() returned nil")
	}

	if retrieved.WorkloadUID != "test-uid-123" {
		t.Errorf("WorkloadUID = %v, want test-uid-123", retrieved.WorkloadUID)
	}

	if facade.GetCalls != 1 {
		t.Errorf("GetAiWorkloadMetadata called %d times, want 1", facade.GetCalls)
	}
}

func TestFacadeStorage_Get_NotFound(t *testing.T) {
	facade := NewMockAiWorkloadMetadataFacade()
	storage := NewFacadeStorage(facade)
	ctx := context.Background()

	retrieved, err := storage.Get(ctx, "non-existent")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	if retrieved != nil {
		t.Error("Get() should return nil for non-existent workload")
	}
}

func TestFacadeStorage_Query_ByWorkloadUID(t *testing.T) {
	facade := NewMockAiWorkloadMetadataFacade()
	storage := NewFacadeStorage(facade)
	ctx := context.Background()

	metadata := &WorkloadMetadata{
		WorkloadUID: "test-uid-123",
		Frameworks:  []string{"pytorch"},
	}

	metadataJSON, _ := json.Marshal(metadata)
	var metadataMap map[string]interface{}
	json.Unmarshal(metadataJSON, &metadataMap)

	facade.Metadata["test-uid-123"] = &model.AiWorkloadMetadata{
		WorkloadUID: "test-uid-123",
		Metadata:    model.ExtType(metadataMap),
	}

	query := &MetadataQuery{
		WorkloadUID: "test-uid-123",
	}

	results, err := storage.Query(ctx, query)
	if err != nil {
		t.Fatalf("Query() error = %v", err)
	}

	if len(results) != 1 {
		t.Errorf("Query() returned %d results, want 1", len(results))
	}

	if len(results) > 0 && results[0].WorkloadUID != "test-uid-123" {
		t.Errorf("WorkloadUID = %v, want test-uid-123", results[0].WorkloadUID)
	}
}

func TestFacadeStorage_Query_EmptyQuery(t *testing.T) {
	facade := NewMockAiWorkloadMetadataFacade()
	storage := NewFacadeStorage(facade)
	ctx := context.Background()

	query := &MetadataQuery{}

	results, err := storage.Query(ctx, query)
	if err != nil {
		t.Fatalf("Query() error = %v", err)
	}

	if results == nil {
		t.Fatal("Query() returned nil")
	}

	if len(results) != 0 {
		t.Errorf("Query() returned %d results, want 0", len(results))
	}
}

func TestFacadeStorage_Delete_Success(t *testing.T) {
	facade := NewMockAiWorkloadMetadataFacade()
	storage := NewFacadeStorage(facade)
	ctx := context.Background()

	facade.Metadata["test-uid-123"] = &model.AiWorkloadMetadata{
		WorkloadUID: "test-uid-123",
	}

	err := storage.Delete(ctx, "test-uid-123")
	if err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	if facade.DeleteCalls != 1 {
		t.Errorf("DeleteAiWorkloadMetadata called %d times, want 1", facade.DeleteCalls)
	}

	if _, exists := facade.Metadata["test-uid-123"]; exists {
		t.Error("Metadata should be deleted from facade")
	}
}

func TestFacadeStorage_Store_WithNestedStructures(t *testing.T) {
	facade := NewMockAiWorkloadMetadataFacade()
	storage := NewFacadeStorage(facade)
	ctx := context.Background()

	metadata := &WorkloadMetadata{
		WorkloadUID:   "test-uid-123",
		PodName:       "test-pod",
		Frameworks:    []string{"pytorch"},
		BaseFramework: "pytorch",
		PyTorchInfo: &PyTorchMetadata{
			Version:       "2.0.0",
			CudaAvailable: true,
			CudaVersion:   "11.8",
		},
		MegatronInfo: &MegatronMetadata{
			Version:        "3.0.0",
			TensorParallel: 4,
		},
		CollectedAt: time.Now(),
	}

	err := storage.Store(ctx, metadata)
	if err != nil {
		t.Fatalf("Store() error = %v", err)
	}

	storedMetadata, exists := facade.Metadata["test-uid-123"]
	if !exists {
		t.Fatal("Metadata not stored in facade")
	}

	if storedMetadata.Framework != "pytorch" {
		t.Errorf("Framework = %v, want pytorch", storedMetadata.Framework)
	}
}

func TestFacadeStorage_Store_EmptyFramework(t *testing.T) {
	facade := NewMockAiWorkloadMetadataFacade()
	storage := NewFacadeStorage(facade)
	ctx := context.Background()

	metadata := &WorkloadMetadata{
		WorkloadUID:   "test-uid-123",
		PodName:       "test-pod",
		Frameworks:    []string{"pytorch", "tensorflow"},
		BaseFramework: "",
		CollectedAt:   time.Now(),
	}

	err := storage.Store(ctx, metadata)
	if err != nil {
		t.Fatalf("Store() error = %v", err)
	}

	storedMetadata := facade.Metadata["test-uid-123"]
	if storedMetadata.Framework != "pytorch" {
		t.Errorf("Framework = %v, want pytorch (first framework)", storedMetadata.Framework)
	}
}
