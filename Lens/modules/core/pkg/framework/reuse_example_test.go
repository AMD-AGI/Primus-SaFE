package framework_test

import (
	"context"
	"fmt"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/framework"
	coreModel "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model"
)

// Example_reuseEngine demonstrates how to use the ReuseEngine for metadata reuse
func Example_reuseEngine() {
	// 1. Setup: Create database facade and reuse engine
	db := database.NewAiWorkloadMetadataFacade()
	config := coreModel.DefaultReuseConfig()
	config.MinSimilarityScore = 0.85
	config.TimeWindowDays = 30
	
	engine := framework.NewReuseEngine(db, config)

	// 2. Define a new workload
	newWorkload := &framework.Workload{
		UID:       "new-workload-123",
		Image:     "registry.example.com/pytorch-training:v1.9.0",
		Command:   []string{"python", "train.py"},
		Args:      []string{"--epochs", "100", "--batch-size", "32"},
		Env: map[string]string{
			"FRAMEWORK":   "PyTorch",
			"WORLD_SIZE":  "8",
			"MASTER_ADDR": "node-0",
		},
		Labels: map[string]string{
			"app":                    "training",
			"ai.amd.com/framework":   "pytorch",
		},
		Namespace: "ml-training",
	}

	// 3. Try to reuse metadata from similar workloads
	ctx := context.Background()
	detection, err := engine.TryReuse(ctx, newWorkload)
	
	if err != nil {
		fmt.Printf("Error trying to reuse: %v\n", err)
		return
	}

	// 4. Handle the result
	if detection != nil {
		// Metadata was successfully reused
		fmt.Printf("✓ Successfully reused metadata from workload %s\n", detection.ReuseInfo.ReusedFrom)
		fmt.Printf("  Framework: %s\n", detection.Framework)
		fmt.Printf("  Type: %s\n", detection.Type)
		fmt.Printf("  Confidence: %.2f (original: %.2f)\n", 
			detection.Confidence, 
			detection.ReuseInfo.OriginalConfidence)
		fmt.Printf("  Similarity Score: %.4f\n", detection.ReuseInfo.SimilarityScore)
		fmt.Printf("  Status: %s\n", detection.Status)
		
		// You can now use this detection without waiting for framework identification
		// Save it to the database for the new workload
		// ...
	} else {
		// No suitable candidate found, need to wait for normal framework detection
		fmt.Println("✗ No similar workload found, will use normal detection")
	}
}

// Example_signatureExtraction demonstrates signature extraction
func Example_signatureExtraction() {
	extractor := framework.NewSignatureExtractor()
	
	workload := &framework.Workload{
		UID:       "example-workload",
		Image:     "registry.example.com/pytorch:v1.9.0",
		Command:   []string{"/usr/bin/python3", "train.py"},
		Args:      []string{"--epochs", "100"},
		Env: map[string]string{
			"FRAMEWORK":    "PyTorch",
			"API_TOKEN":    "secret123", // Will be filtered out
			"DATABASE_URL": "postgresql://...",
		},
		Labels: map[string]string{
			"app":                      "training",
			"pod-template-hash":        "abc123", // Will be filtered out
		},
		Namespace: "default",
	}

	signature := extractor.ExtractSignature(workload)
	
	fmt.Printf("Image: %s\n", signature.Image)
	fmt.Printf("Command: %v\n", signature.Command) // Normalized to ["python3", "train.py"]
	fmt.Printf("Env (filtered): %v\n", signature.Env) // API_TOKEN removed
	fmt.Printf("Labels (filtered): %v\n", signature.Labels) // pod-template-hash removed
	fmt.Printf("Image Hash: %s\n", signature.ImageHash)
	fmt.Printf("Command Hash: %s\n", signature.CommandHash)
	fmt.Printf("Env Hash: %s\n", signature.EnvHash)
}

// Example_similarityCalculation demonstrates similarity calculation
func Example_similarityCalculation() {
	calc := framework.NewSimilarityCalculator()
	
	// Define two workload signatures
	workload1 := &coreModel.WorkloadSignature{
		Image:     "registry.example.com/pytorch:v1.9.0",
		Command:   []string{"python", "train.py"},
		Args:      []string{"--epochs", "100"},
		Env:       map[string]string{"FRAMEWORK": "PyTorch", "WORLD_SIZE": "8"},
		Labels:    map[string]string{"app": "training"},
		Namespace: "default",
	}
	
	workload2 := &coreModel.WorkloadSignature{
		Image:     "registry.example.com/pytorch:v1.9.1", // Different version
		Command:   []string{"python", "train.py"},
		Args:      []string{"--epochs", "100"},
		Env:       map[string]string{"FRAMEWORK": "PyTorch", "WORLD_SIZE": "8"},
		Labels:    map[string]string{"app": "training"},
		Namespace: "default",
	}
	
	result := calc.CalculateSimilarity(workload1, workload2)
	
	fmt.Printf("Overall Similarity: %.4f\n", result.Score)
	fmt.Printf("  Image Score: %.2f\n", result.Details.ImageScore)
	fmt.Printf("  Command Score: %.2f\n", result.Details.CommandScore)
	fmt.Printf("  Env Score: %.2f\n", result.Details.EnvScore)
	fmt.Printf("  Args Score: %.2f\n", result.Details.ArgsScore)
	fmt.Printf("  Label Score: %.2f\n", result.Details.LabelScore)
	
	// Typical output:
	// Overall Similarity: 0.96
	//   Image Score: 0.90 (same image, different tag)
	//   Command Score: 1.00
	//   Env Score: 1.00
	//   Args Score: 1.00
	//   Label Score: 1.00
}

// Example_workloadCreationWithReuse demonstrates complete workflow
func Example_workloadCreationWithReuse() {
	// This example shows how to integrate reuse engine in workload creation flow
	
	ctx := context.Background()
	db := database.NewAiWorkloadMetadataFacade()
	config := coreModel.DefaultReuseConfig()
	reuseEngine := framework.NewReuseEngine(db, config)
	
	// Step 1: Workload created (from K8s API or adapter)
	workload := &framework.Workload{
		UID:       "workload-456",
		Image:     "registry.example.com/pytorch:v1.9.0",
		Command:   []string{"python", "train.py"},
		Namespace: "ml-training",
	}
	
	fmt.Println("=== Workload Created ===")
	fmt.Printf("UID: %s\n", workload.UID)
	fmt.Printf("Image: %s\n", workload.Image)
	
	// Step 2: Try reuse immediately (before pod starts)
	fmt.Println("\n=== Attempting Metadata Reuse ===")
	detection, err := reuseEngine.TryReuse(ctx, workload)
	
	if err != nil {
		fmt.Printf("Reuse failed: %v\n", err)
		return
	}
	
	if detection != nil {
		// Step 3a: Reuse successful - save immediately
		fmt.Println("✓ Reuse successful!")
		fmt.Printf("  Framework: %s\n", detection.Framework)
		fmt.Printf("  Confidence: %.2f\n", detection.Confidence)
		fmt.Printf("  Time saved: ~6 minutes (no need to wait for log-based detection)\n")
		
		// Save detection to database
		metadata := &model.AiWorkloadMetadata{
			WorkloadUID: workload.UID,
			Metadata: model.ExtType{
				"framework_detection": detection,
			},
			CreatedAt: time.Now(),
		}
		
		// ... save metadata ...
		_ = metadata
		
		// Log processing can now use the framework configuration immediately
		fmt.Println("✓ Framework detection ready for log processing")
		
	} else {
		// Step 3b: Reuse not possible - use normal detection flow
		fmt.Println("✗ No reuse candidate found")
		fmt.Println("  Will wait for pod to start and logs to arrive for detection")
		fmt.Println("  Estimated time: 5-10 minutes")
	}
}

// Example_configurationOptions demonstrates different configuration scenarios
func Example_configurationOptions() {
	// Scenario 1: High precision (production)
	highPrecisionConfig := coreModel.ReuseConfig{
		Enabled:              true,
		MinSimilarityScore:   0.90, // Only reuse very similar workloads
		TimeWindowDays:       30,
		MinConfidence:        0.85,  // Only reuse high-confidence detections
		ConfidenceDecayRate:  0.95,  // Minimal decay
		MaxCandidates:        50,
		CacheTTLMinutes:      10,
	}
	
	// Scenario 2: Aggressive reuse (development)
	aggressiveConfig := coreModel.ReuseConfig{
		Enabled:              true,
		MinSimilarityScore:   0.75, // More lenient
		TimeWindowDays:       90,   // Longer window
		MinConfidence:        0.70,
		ConfidenceDecayRate:  0.85, // More decay
		MaxCandidates:        200,
		CacheTTLMinutes:      30,
	}
	
	// Scenario 3: Disabled (testing/debugging)
	disabledConfig := coreModel.ReuseConfig{
		Enabled: false,
	}
	
	fmt.Printf("High Precision Config: %+v\n", highPrecisionConfig)
	fmt.Printf("Aggressive Config: %+v\n", aggressiveConfig)
	fmt.Printf("Disabled Config: %+v\n", disabledConfig)
}

