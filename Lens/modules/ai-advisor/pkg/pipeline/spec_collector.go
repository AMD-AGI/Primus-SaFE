// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package pipeline

import (
	"context"
	"fmt"
	"strings"

	"github.com/AMD-AGI/Primus-SaFE/Lens/ai-advisor/pkg/intent"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
)

// SpecCollector extracts intent evidence from the workload spec.
// It reads from gpu_workload (labels, annotations, GVK), ai_workload_metadata,
// and workload_detection (already-confirmed framework/type from detection system)
// without needing a running pod.
//
// This is a passive collector: it only uses data already stored in the database,
// making it fast and available even when the pod is not yet running.
type SpecCollector struct {
	workloadFacade  database.WorkloadFacadeInterface
	metadataFacade  database.AiWorkloadMetadataFacadeInterface
	detectionFacade database.WorkloadDetectionFacadeInterface
}

// NewSpecCollector creates a new SpecCollector
func NewSpecCollector() *SpecCollector {
	return &SpecCollector{
		workloadFacade:  database.GetFacade().GetWorkload(),
		metadataFacade:  database.NewAiWorkloadMetadataFacade(),
		detectionFacade: database.NewWorkloadDetectionFacade(),
	}
}

// NewSpecCollectorWithDeps creates a SpecCollector with injected dependencies (for testing)
func NewSpecCollectorWithDeps(
	workloadFacade database.WorkloadFacadeInterface,
	metadataFacade database.AiWorkloadMetadataFacadeInterface,
	detectionFacade database.WorkloadDetectionFacadeInterface,
) *SpecCollector {
	return &SpecCollector{
		workloadFacade:  workloadFacade,
		metadataFacade:  metadataFacade,
		detectionFacade: detectionFacade,
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
		evidence.WorkloadName = workload.Name
		evidence.WorkloadKind = workload.Kind
		evidence.WorkloadNamespace = workload.Namespace

		// Extract replicas hint from annotations if available
		if annotations := extractStringMap(workload.Annotations); annotations != nil {
			if replicas, ok := annotations["replicas"]; ok {
				if r := parseIntSafe(replicas); r > 0 {
					evidence.Replicas = r
				}
			}
		}
	}

	// 2. Gather from workload_detection: already-detected framework and workload_type.
	// This leverages signals from the detection_coordinator (process probe, log pattern, etc.)
	det, err := c.detectionFacade.GetDetection(ctx, workloadUID)
	if err != nil {
		log.Debugf("SpecCollector: no detection for workload %s: %v", workloadUID, err)
	}
	if det != nil {
		evidence.DetectedFramework = det.Framework
		evidence.DetectedWorkloadType = det.WorkloadType
	}

	// 3. Gather from ai_workload_metadata
	metadata, err := c.metadataFacade.GetAiWorkloadMetadata(ctx, workloadUID)
	if err != nil {
		log.Debugf("SpecCollector: no metadata for workload %s: %v", workloadUID, err)
	}

	if metadata != nil && metadata.Metadata != nil {
		c.extractFromMetadata(metadata, evidence)
	}

	return evidence, nil
}

// extractFromMetadata extracts evidence fields from the metadata JSON.
// It supports both the legacy workload_signature format and the
// current framework_detection format used by the detection system.
func (c *SpecCollector) extractFromMetadata(metadata *model.AiWorkloadMetadata, evidence *intent.IntentEvidence) {
	md := metadata.Metadata

	// Try workload_signature format (newer format, if present)
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

	// Try framework_detection format (current format in production)
	if fwDet, ok := md["framework_detection"].(map[string]interface{}); ok {
		c.extractFromFrameworkDetection(fwDet, evidence)
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

	// Extract environment variables from container_env or env_vars
	c.extractEnvVars(md, evidence)
}

// extractFromFrameworkDetection reads the framework_detection metadata format
// that the detection system produces, extracting useful signals for intent analysis.
func (c *SpecCollector) extractFromFrameworkDetection(fwDet map[string]interface{}, evidence *intent.IntentEvidence) {
	// Extract detected framework and type as fallback signals
	if fw, ok := fwDet["framework"].(string); ok && fw != "" && evidence.DetectedFramework == "" {
		evidence.DetectedFramework = fw
	}
	if wlType, ok := fwDet["type"].(string); ok && wlType != "" && evidence.DetectedWorkloadType == "" {
		evidence.DetectedWorkloadType = wlType
	}

	// Extract evidence details from sources array
	if sources, ok := fwDet["sources"].([]interface{}); ok {
		for _, srcRaw := range sources {
			src, ok := srcRaw.(map[string]interface{})
			if !ok {
				continue
			}

			// If source has evidence with cmdline, use it
			if evData, ok := src["evidence"].(map[string]interface{}); ok {
				if cmdline, ok := evData["cmdline"].(string); ok && cmdline != "" && evidence.Command == "" {
					evidence.Command = cmdline
				}
				// Extract image if present
				if img, ok := evData["image"].(string); ok && img != "" && evidence.Image == "" {
					evidence.Image = img
				}
			}
		}
	}
}

// extractEnvVars reads environment variables from metadata using various field names
func (c *SpecCollector) extractEnvVars(md model.ExtType, evidence *intent.IntentEvidence) {
	envFields := []string{"container_env", "env_vars"}
	for _, field := range envFields {
		if evidence.Env != nil && len(evidence.Env) > 0 {
			break
		}
		if envData, ok := md[field].(map[string]interface{}); ok {
			evidence.Env = make(map[string]string)
			for k, v := range envData {
				evidence.Env[k] = fmt.Sprint(v)
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
