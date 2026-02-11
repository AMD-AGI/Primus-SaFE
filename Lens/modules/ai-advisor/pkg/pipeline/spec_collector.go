// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package pipeline

import (
	"context"
	"strings"

	"github.com/AMD-AGI/Primus-SaFE/Lens/ai-advisor/pkg/intent"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
)

// SpecCollector extracts intent evidence from the workload spec.
// It reads from gpu_workload (labels, annotations, GVK) and ai_workload_metadata
// (container image, command line, environment) without needing a running pod.
//
// This is a passive collector: it only uses data already stored in the database,
// making it fast and available even when the pod is not yet running.
type SpecCollector struct {
	workloadFacade database.WorkloadFacadeInterface
	metadataFacade database.AiWorkloadMetadataFacadeInterface
}

// NewSpecCollector creates a new SpecCollector
func NewSpecCollector() *SpecCollector {
	return &SpecCollector{
		workloadFacade: database.GetFacade().GetWorkload(),
		metadataFacade: database.NewAiWorkloadMetadataFacade(),
	}
}

// NewSpecCollectorWithDeps creates a SpecCollector with injected dependencies (for testing)
func NewSpecCollectorWithDeps(
	workloadFacade database.WorkloadFacadeInterface,
	metadataFacade database.AiWorkloadMetadataFacadeInterface,
) *SpecCollector {
	return &SpecCollector{
		workloadFacade: workloadFacade,
		metadataFacade: metadataFacade,
	}
}

// Collect gathers spec-level evidence for a workload
func (c *SpecCollector) Collect(ctx context.Context, workloadUID string) (*intent.IntentEvidence, error) {
	evidence := &intent.IntentEvidence{}

	// 1. Gather from gpu_workload table: GVK, labels, annotations
	workload, err := c.workloadFacade.GetGpuWorkloadByUid(ctx, workloadUID)
	if err != nil {
		log.Warnf("SpecCollector: failed to get workload %s: %v", workloadUID, err)
	}

	if workload != nil {
		evidence.GVK = buildGVK(workload)
		evidence.Labels = extractStringMap(workload.Labels)

		// Extract replicas hint from annotations if available
		if annotations := extractStringMap(workload.Annotations); annotations != nil {
			if replicas, ok := annotations["replicas"]; ok {
				// Parse replicas if possible
				if r := parseIntSafe(replicas); r > 0 {
					evidence.Replicas = r
				}
			}
		}
	}

	// 2. Gather from ai_workload_metadata: image, command, env
	metadata, err := c.metadataFacade.GetAiWorkloadMetadata(ctx, workloadUID)
	if err != nil {
		log.Debugf("SpecCollector: no metadata for workload %s: %v", workloadUID, err)
	}

	if metadata != nil && metadata.Metadata != nil {
		c.extractFromMetadata(metadata, evidence)
	}

	return evidence, nil
}

// extractFromMetadata extracts evidence fields from the metadata JSON
func (c *SpecCollector) extractFromMetadata(metadata *model.AiWorkloadMetadata, evidence *intent.IntentEvidence) {
	md := metadata.Metadata

	// Extract image from workload_signature or container_image
	if signatureData, ok := md["workload_signature"].(map[string]interface{}); ok {
		if imageName, ok := signatureData["image"].(string); ok && imageName != "" {
			evidence.Image = imageName
		}
		if command, ok := signatureData["command"].(string); ok && command != "" {
			evidence.Command = command
		}
		if args, ok := signatureData["args"].([]interface{}); ok {
			for _, arg := range args {
				if s, ok := arg.(string); ok {
					evidence.Args = append(evidence.Args, s)
				}
			}
		}
	}

	// Fallback: container_image field
	if evidence.Image == "" {
		if imageName, ok := md["container_image"].(string); ok && imageName != "" {
			evidence.Image = imageName
		}
	}

	// Extract command from container_command
	if evidence.Command == "" {
		if command, ok := md["container_command"].(string); ok && command != "" {
			evidence.Command = command
		}
	}

	// Extract environment variables from container_env
	if envData, ok := md["container_env"].(map[string]interface{}); ok {
		evidence.Env = make(map[string]string)
		for k, v := range envData {
			if s, ok := v.(string); ok {
				evidence.Env[k] = s
			}
		}
	}

	// Extract environment from env_vars (alternative field)
	if evidence.Env == nil || len(evidence.Env) == 0 {
		if envData, ok := md["env_vars"].(map[string]interface{}); ok {
			evidence.Env = make(map[string]string)
			for k, v := range envData {
				if s, ok := v.(string); ok {
					evidence.Env[k] = s
				}
			}
		}
	}
}

// buildGVK constructs a GVK string from the workload record
func buildGVK(w *model.GpuWorkload) string {
	parts := []string{}
	if w.GroupVersion != "" {
		parts = append(parts, w.GroupVersion)
	}
	if w.Kind != "" {
		parts = append(parts, w.Kind)
	}
	return strings.Join(parts, "/")
}

// extractStringMap converts ExtType to map[string]string
func extractStringMap(ext model.ExtType) map[string]string {
	if ext == nil {
		return nil
	}
	result := make(map[string]string)
	for k, v := range ext {
		if s, ok := v.(string); ok {
			result[k] = s
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

// parseIntSafe attempts to parse a string as int, returning 0 on failure
func parseIntSafe(s string) int {
	var n int
	for _, c := range s {
		if c >= '0' && c <= '9' {
			n = n*10 + int(c-'0')
		} else {
			return 0
		}
	}
	return n
}
