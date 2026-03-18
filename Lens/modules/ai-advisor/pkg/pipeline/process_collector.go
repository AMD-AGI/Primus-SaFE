// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package pipeline

import (
	"context"
	"strings"

	"github.com/AMD-AGI/Primus-SaFE/Lens/ai-advisor/pkg/intent"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/constant"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
)

// ProcessEvidenceCollector reads process probe results from the detection_evidence
// table and enriches IntentEvidence with command lines, environment variables, and
// framework signals discovered by the ProcessProbeExecutor.
//
// This collector does NOT probe processes itself; it reads from evidence already
// collected by the standard process probe sub-task. The Pipeline schedules the
// sub-task and this collector reads the stored results.
type ProcessEvidenceCollector struct {
	evidenceFacade database.WorkloadDetectionEvidenceFacadeInterface
}

// NewProcessEvidenceCollector creates a new collector
func NewProcessEvidenceCollector() *ProcessEvidenceCollector {
	return &ProcessEvidenceCollector{
		evidenceFacade: database.NewWorkloadDetectionEvidenceFacade(),
	}
}

// Enrich reads process-source evidence and merges it into the given IntentEvidence
func (c *ProcessEvidenceCollector) Enrich(
	ctx context.Context,
	workloadUID string,
	evidence *intent.IntentEvidence,
) {
	records, err := c.evidenceFacade.ListEvidenceBySource(ctx, workloadUID, constant.DetectionSourceProcess)
	if err != nil {
		log.Warnf("ProcessEvidenceCollector: failed to list process evidence for %s: %v", workloadUID, err)
		return
	}

	if len(records) == 0 {
		return
	}

	// Merge all process evidence records
	for _, rec := range records {
		if rec.Evidence == nil {
			continue
		}

		// Extract cmdline from evidence JSON
		if cmdline, ok := rec.Evidence["cmdline"].(string); ok && cmdline != "" {
			// If no command from spec, use process probe cmdline
			if evidence.Command == "" {
				evidence.Command = cmdline
			} else if !strings.Contains(evidence.Command, cmdline) {
				// Append if different
				evidence.Args = append(evidence.Args, cmdline)
			}
		}

		// Extract env vars from evidence JSON
		if matchedVars, ok := rec.Evidence["matched_vars"].(map[string]interface{}); ok {
			if evidence.Env == nil {
				evidence.Env = make(map[string]string)
			}
			for k, v := range matchedVars {
				if s, ok := v.(string); ok {
					evidence.Env[k] = s
				}
			}
		}

		// Extract full env map if available
		if envMap, ok := rec.Evidence["env_vars"].(map[string]interface{}); ok {
			if evidence.Env == nil {
				evidence.Env = make(map[string]string)
			}
			for k, v := range envMap {
				if s, ok := v.(string); ok {
					// Only add training/distributed-relevant env vars
					if isIntentRelevantEnvVar(k) {
						evidence.Env[k] = s
					}
				}
			}
		}
	}
}

// isIntentRelevantEnvVar returns true if the env var is relevant for intent analysis
func isIntentRelevantEnvVar(key string) bool {
	relevantPrefixes := []string{
		"MASTER_", "WORLD_SIZE", "RANK", "LOCAL_RANK",
		"NCCL_", "CUDA_", "ROCR_",
		"DEEPSPEED_", "DS_",
		"MEGATRON_", "PRIMUS_",
		"HF_", "HUGGING_FACE_",
		"WANDB_", "MLFLOW_",
		"MODEL_", "CHECKPOINT_",
		"BATCH_SIZE", "LEARNING_RATE", "NUM_EPOCHS",
		"FSDP_", "TORCH_",
		"TRITON_", "VLLM_", "SGLANG_",
	}

	keyUpper := strings.ToUpper(key)
	for _, prefix := range relevantPrefixes {
		if strings.HasPrefix(keyUpper, prefix) {
			return true
		}
	}

	relevantExact := []string{
		"FRAMEWORK", "TRAINING_FRAMEWORK",
		"NUM_GPUS", "NUM_NODES",
	}

	for _, exact := range relevantExact {
		if keyUpper == exact {
			return true
		}
	}

	return false
}
