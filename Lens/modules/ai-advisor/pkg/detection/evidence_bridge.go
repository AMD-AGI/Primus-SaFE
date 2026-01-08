// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package detection

import (
	"context"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/constant"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/framework"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	coreModel "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model"
)

// EvidenceBridge bridges the legacy detection system to the new evidence-based system
// It listens for detection events from FrameworkDetectionManager and stores them as evidence
type EvidenceBridge struct {
	evidenceFacade database.WorkloadDetectionEvidenceFacadeInterface
	coverageFacade database.DetectionCoverageFacadeInterface
}

// NewEvidenceBridge creates a new evidence bridge
func NewEvidenceBridge() *EvidenceBridge {
	return &EvidenceBridge{
		evidenceFacade: database.NewWorkloadDetectionEvidenceFacade(),
		coverageFacade: database.NewDetectionCoverageFacade(),
	}
}

// OnDetectionEvent implements DetectionEventListener interface
// It converts legacy detection events to evidence records
func (b *EvidenceBridge) OnDetectionEvent(
	ctx context.Context,
	event *framework.DetectionEvent,
) error {
	// Only handle updated events (which contain new detection data)
	if event.Type != framework.DetectionEventTypeUpdated &&
		event.Type != framework.DetectionEventTypeCompleted {
		return nil
	}

	if event.Detection == nil {
		return nil
	}

	// Convert detection sources to evidence records
	return b.storeDetectionAsEvidence(ctx, event.WorkloadUID, event.Detection)
}

// storeDetectionAsEvidence converts a FrameworkDetection to evidence records
func (b *EvidenceBridge) storeDetectionAsEvidence(
	ctx context.Context,
	workloadUID string,
	detection *coreModel.FrameworkDetection,
) error {
	if detection == nil || len(detection.Sources) == 0 {
		return nil
	}

	// Process each source as a separate evidence record
	for _, source := range detection.Sources {
		// Map source type to DetectionSource constant
		detectionSource := mapSourceToConstant(source.Source)
		if detectionSource == "" {
			log.Debugf("Unknown detection source: %s, skipping evidence storage", source.Source)
			continue
		}

		// Get primary framework from the source
		framework := ""
		if len(source.Frameworks) > 0 {
			framework = source.Frameworks[0]
		}
		if framework == "" {
			continue
		}

		// Build evidence data
		evidenceData := model.ExtType{
			"source_type": source.Source,
			"method":      "legacy_detection_manager",
			"frameworks":  source.Frameworks,
		}
		if source.Evidence != nil {
			for k, v := range source.Evidence {
				evidenceData[k] = v
			}
		}
		if source.WrapperFramework != "" {
			evidenceData["wrapper_framework"] = source.WrapperFramework
		}
		if source.BaseFramework != "" {
			evidenceData["base_framework"] = source.BaseFramework
		}

		// Create evidence record
		evidence := &model.WorkloadDetectionEvidence{
			WorkloadUID: workloadUID,
			Source:      detectionSource,
			SourceType:  "passive", // Legacy detection is passive
			Framework:   framework,
			Confidence:  source.Confidence,
			DetectedAt:  source.DetectedAt,
			Evidence:    evidenceData,
		}

		// Upsert evidence (avoid duplicates by source + framework)
		if err := b.evidenceFacade.UpsertEvidence(ctx, evidence); err != nil {
			log.Warnf("Failed to store evidence for workload %s source %s: %v",
				workloadUID, detectionSource, err)
			// Continue with other sources
			continue
		}

		log.Debugf("Stored evidence for workload %s: source=%s, framework=%s, confidence=%.2f",
			workloadUID, detectionSource, framework, source.Confidence)

		// Update coverage status if applicable
		b.updateCoverageFromEvidence(ctx, workloadUID, detectionSource)
	}

	return nil
}

// updateCoverageFromEvidence updates detection coverage based on evidence
func (b *EvidenceBridge) updateCoverageFromEvidence(
	ctx context.Context,
	workloadUID string,
	source string,
) {
	// Check if coverage record exists for this source
	coverage, err := b.coverageFacade.GetCoverage(ctx, workloadUID, source)
	if err != nil || coverage == nil {
		// Coverage not initialized yet, skip
		return
	}

	// Mark as collected if not already
	if coverage.Status != constant.DetectionStatusCollected {
		// Pass evidence count as 1 (we're adding one piece of evidence)
		if err := b.coverageFacade.MarkCollected(ctx, workloadUID, source, 1); err != nil {
			log.Warnf("Failed to update coverage status for workload %s source %s: %v",
				workloadUID, source, err)
		}
	}
}

// mapSourceToConstant maps legacy source names to DetectionSource constants
func mapSourceToConstant(source string) string {
	switch source {
	case "log", "log_pattern", "telemetry":
		return constant.DetectionSourceLog
	case "process", "cmdline", "env":
		return constant.DetectionSourceProcess
	case "image", "container", "component":
		return constant.DetectionSourceImage
	case "label", "annotation":
		return constant.DetectionSourceLabel
	case "wandb":
		return constant.DetectionSourceWandb
	case "import", "python_import":
		return constant.DetectionSourceImport
	case "user", "manual":
		// User/manual detection - map to process as generic active source
		return constant.DetectionSourceProcess
	case "reuse":
		// Reuse detection - map to import as it's based on code similarity
		return constant.DetectionSourceImport
	default:
		// Unknown source, log and skip
		return ""
	}
}

// RegisterEvidenceBridge registers the evidence bridge with the detection manager
func RegisterEvidenceBridge(detectionMgr *framework.FrameworkDetectionManager) *EvidenceBridge {
	bridge := NewEvidenceBridge()
	detectionMgr.RegisterListener(bridge)
	log.Info("EvidenceBridge registered with DetectionManager - legacy detections will be stored as evidence")
	return bridge
}

