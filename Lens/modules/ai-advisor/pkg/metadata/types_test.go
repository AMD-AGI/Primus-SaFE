package metadata

import (
	"encoding/json"
	"testing"
	"time"
)

func TestWorkloadMetadata_Marshal(t *testing.T) {
	metadata := &WorkloadMetadata{
		WorkloadUID:      "test-uid-123",
		PodName:          "test-pod",
		PodNamespace:     "test-namespace",
		NodeName:         "test-node",
		Frameworks:       []string{"pytorch", "primus"},
		BaseFramework:    "pytorch",
		WrapperFramework: "primus",
		CollectedAt:      time.Now(),
		CollectionSource: "node-exporter",
		Confidence:       0.95,
	}

	data, err := json.Marshal(metadata)
	if err != nil {
		t.Fatalf("Failed to marshal metadata: %v", err)
	}

	var unmarshaled WorkloadMetadata
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("Failed to unmarshal metadata: %v", err)
	}

	if unmarshaled.WorkloadUID != metadata.WorkloadUID {
		t.Errorf("WorkloadUID = %v, want %v", unmarshaled.WorkloadUID, metadata.WorkloadUID)
	}
	if unmarshaled.PodName != metadata.PodName {
		t.Errorf("PodName = %v, want %v", unmarshaled.PodName, metadata.PodName)
	}
}

func TestPyTorchMetadata_Marshal(t *testing.T) {
	pytorch := &PyTorchMetadata{
		Version:         "2.0.0",
		CudaAvailable:   true,
		CudaVersion:     "11.8",
		TotalParams:     1000000,
		TrainableParams: 900000,
		Device:          "cuda:0",
		DistributedMode: "DDP",
		MixedPrecision:  true,
		Models: []ModelInfo{
			{
				Name:            "ResNet50",
				Type:            "CNN",
				Parameters:      25000000,
				TrainableParams: 25000000,
				Device:          "cuda:0",
			},
		},
	}

	data, err := json.Marshal(pytorch)
	if err != nil {
		t.Fatalf("Failed to marshal PyTorch metadata: %v", err)
	}

	var unmarshaled PyTorchMetadata
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("Failed to unmarshal PyTorch metadata: %v", err)
	}

	if unmarshaled.Version != pytorch.Version {
		t.Errorf("Version = %v, want %v", unmarshaled.Version, pytorch.Version)
	}
	if unmarshaled.CudaAvailable != pytorch.CudaAvailable {
		t.Errorf("CudaAvailable = %v, want %v", unmarshaled.CudaAvailable, pytorch.CudaAvailable)
	}
}

func TestMegatronMetadata_Marshal(t *testing.T) {
	megatron := &MegatronMetadata{
		Version:           "3.0.0",
		TensorParallel:    4,
		PipelineParallel:  2,
		DataParallel:      1,
		SequenceParallel:  true,
		MicroBatchSize:    2,
		GlobalBatchSize:   256,
		SequenceLength:    2048,
		HiddenSize:        4096,
		NumLayers:         32,
		NumAttentionHeads: 32,
		VocabSize:         50000,
		LearningRate:      0.0001,
		Optimizer:         "Adam",
	}

	data, err := json.Marshal(megatron)
	if err != nil {
		t.Fatalf("Failed to marshal Megatron metadata: %v", err)
	}

	var unmarshaled MegatronMetadata
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("Failed to unmarshal Megatron metadata: %v", err)
	}

	if unmarshaled.TensorParallel != megatron.TensorParallel {
		t.Errorf("TensorParallel = %v, want %v", unmarshaled.TensorParallel, megatron.TensorParallel)
	}
}

func TestPrimusMetadata_Marshal(t *testing.T) {
	primus := &PrimusMetadata{
		Version:          "1.0.0",
		Mode:             "training",
		BackendFramework: "pytorch",
		Configuration: map[string]interface{}{
			"batch_size": 32,
			"epochs":     100,
		},
		Features: []string{"auto-tuning", "checkpointing"},
	}

	data, err := json.Marshal(primus)
	if err != nil {
		t.Fatalf("Failed to marshal Primus metadata: %v", err)
	}

	var unmarshaled PrimusMetadata
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("Failed to unmarshal Primus metadata: %v", err)
	}

	if unmarshaled.Version != primus.Version {
		t.Errorf("Version = %v, want %v", unmarshaled.Version, primus.Version)
	}
	if unmarshaled.Mode != primus.Mode {
		t.Errorf("Mode = %v, want %v", unmarshaled.Mode, primus.Mode)
	}
}

func TestJAXMetadata_Marshal(t *testing.T) {
	jax := &JAXMetadata{
		Version:      "0.4.0",
		Backend:      "GPU",
		NumDevices:   8,
		ParallelMode: "pmap",
		JIT:          true,
	}

	data, err := json.Marshal(jax)
	if err != nil {
		t.Fatalf("Failed to marshal JAX metadata: %v", err)
	}

	var unmarshaled JAXMetadata
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("Failed to unmarshal JAX metadata: %v", err)
	}

	if unmarshaled.Backend != jax.Backend {
		t.Errorf("Backend = %v, want %v", unmarshaled.Backend, jax.Backend)
	}
}

func TestTensorBoardMetadata_Marshal(t *testing.T) {
	tensorboard := &TensorBoardMetadata{
		Enabled:    true,
		LogDir:     "/logs/tensorboard",
		Port:       6006,
		Writers:    []string{"train", "validation"},
		UpdateFreq: "batch",
	}

	data, err := json.Marshal(tensorboard)
	if err != nil {
		t.Fatalf("Failed to marshal TensorBoard metadata: %v", err)
	}

	var unmarshaled TensorBoardMetadata
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("Failed to unmarshal TensorBoard metadata: %v", err)
	}

	if unmarshaled.Port != tensorboard.Port {
		t.Errorf("Port = %v, want %v", unmarshaled.Port, tensorboard.Port)
	}
}

func TestCollectionRequest_Marshal(t *testing.T) {
	req := &CollectionRequest{
		WorkloadUID:  "test-uid",
		PodName:      "test-pod",
		PodNamespace: "default",
		PodUID:       "pod-uid-123",
		NodeName:     "node-1",
		Force:        true,
		Scripts:      []string{"detect_pytorch.py", "detect_megatron.py"},
		Timeout:      300,
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Failed to marshal CollectionRequest: %v", err)
	}

	var unmarshaled CollectionRequest
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("Failed to unmarshal CollectionRequest: %v", err)
	}

	if unmarshaled.WorkloadUID != req.WorkloadUID {
		t.Errorf("WorkloadUID = %v, want %v", unmarshaled.WorkloadUID, req.WorkloadUID)
	}
	if unmarshaled.Force != req.Force {
		t.Errorf("Force = %v, want %v", unmarshaled.Force, req.Force)
	}
}

func TestCollectionResult_Marshal(t *testing.T) {
	result := &CollectionResult{
		Success:        true,
		Duration:       1.5,
		ProcessCount:   10,
		PythonCount:    3,
		InspectedCount: 2,
		Metadata: &WorkloadMetadata{
			WorkloadUID: "test-uid",
			PodName:     "test-pod",
		},
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("Failed to marshal CollectionResult: %v", err)
	}

	var unmarshaled CollectionResult
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("Failed to unmarshal CollectionResult: %v", err)
	}

	if unmarshaled.Success != result.Success {
		t.Errorf("Success = %v, want %v", unmarshaled.Success, result.Success)
	}
}

func TestMetadataQuery_Marshal(t *testing.T) {
	now := time.Now()
	query := &MetadataQuery{
		WorkloadUID: "test-uid",
		Framework:   "pytorch",
		Type:        "training",
		StartTime:   &now,
		EndTime:     &now,
		Limit:       10,
	}

	data, err := json.Marshal(query)
	if err != nil {
		t.Fatalf("Failed to marshal MetadataQuery: %v", err)
	}

	var unmarshaled MetadataQuery
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("Failed to unmarshal MetadataQuery: %v", err)
	}

	if unmarshaled.Framework != query.Framework {
		t.Errorf("Framework = %v, want %v", unmarshaled.Framework, query.Framework)
	}
}

func TestWorkloadMetadataWithNestedStructs(t *testing.T) {
	metadata := &WorkloadMetadata{
		WorkloadUID:      "test-uid",
		PodName:          "test-pod",
		PodNamespace:     "test-namespace",
		NodeName:         "test-node",
		Frameworks:       []string{"pytorch"},
		BaseFramework:    "pytorch",
		CollectedAt:      time.Now(),
		CollectionSource: "node-exporter",
		PyTorchInfo: &PyTorchMetadata{
			Version:       "2.0.0",
			CudaAvailable: true,
		},
		MegatronInfo: &MegatronMetadata{
			Version:        "3.0.0",
			TensorParallel: 4,
		},
		PrimusInfo: &PrimusMetadata{
			Version: "1.0.0",
			Mode:    "training",
		},
	}

	data, err := json.Marshal(metadata)
	if err != nil {
		t.Fatalf("Failed to marshal metadata with nested structs: %v", err)
	}

	var unmarshaled WorkloadMetadata
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("Failed to unmarshal metadata with nested structs: %v", err)
	}

	if unmarshaled.PyTorchInfo == nil {
		t.Error("PyTorchInfo should not be nil")
	} else if unmarshaled.PyTorchInfo.Version != "2.0.0" {
		t.Errorf("PyTorchInfo.Version = %v, want 2.0.0", unmarshaled.PyTorchInfo.Version)
	}

	if unmarshaled.MegatronInfo == nil {
		t.Error("MegatronInfo should not be nil")
	} else if unmarshaled.MegatronInfo.TensorParallel != 4 {
		t.Errorf("MegatronInfo.TensorParallel = %v, want 4", unmarshaled.MegatronInfo.TensorParallel)
	}
}

