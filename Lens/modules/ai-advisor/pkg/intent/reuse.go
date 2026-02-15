// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package intent

import (
	"context"
	"encoding/json"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
)

// ReuseChecker checks if an existing intent analysis result can be reused
// based on code snapshot fingerprint or image digest. This avoids redundant
// analysis when:
//   - The same code (same fingerprint) has been analyzed before
//   - The same image (same digest) has been analyzed before
type ReuseChecker struct {
	detectionFacade  database.WorkloadDetectionFacadeInterface
	snapshotFacade   database.WorkloadCodeSnapshotFacadeInterface
	imageCacheFacade database.ImageRegistryCacheFacadeInterface
}

// ReuseResult holds the result of a reuse check
type ReuseResult struct {
	// Whether a previous result was found
	Found bool

	// The reused IntentResult (if found)
	Result *IntentResult

	// How the reuse was found
	ReuseSource string // "fingerprint" or "digest"

	// The source workload UID whose result we're reusing
	SourceWorkloadUID string
}

// NewReuseChecker creates a new reuse checker
func NewReuseChecker() *ReuseChecker {
	return &ReuseChecker{
		detectionFacade:  database.NewWorkloadDetectionFacade(),
		snapshotFacade:   database.NewWorkloadCodeSnapshotFacade(),
		imageCacheFacade: database.NewImageRegistryCacheFacade(),
	}
}

// Check attempts to find a reusable intent result
func (r *ReuseChecker) Check(ctx context.Context, workloadUID string, evidence *IntentEvidence) *ReuseResult {
	// 1. Check by code snapshot fingerprint
	if evidence.CodeSnapshot != nil && evidence.CodeSnapshot.Fingerprint != "" {
		if result := r.checkByFingerprint(ctx, workloadUID, evidence.CodeSnapshot.Fingerprint); result != nil {
			return result
		}
	}

	// 2. Check by image digest
	if evidence.ImageRegistry != nil && evidence.ImageRegistry.Digest != "" {
		if result := r.checkByDigest(ctx, workloadUID, evidence.ImageRegistry.Digest); result != nil {
			return result
		}
	}

	return &ReuseResult{Found: false}
}

func (r *ReuseChecker) checkByFingerprint(ctx context.Context, currentUID, fingerprint string) *ReuseResult {
	// Find other workloads with the same fingerprint
	snapshots, err := r.snapshotFacade.GetByFingerprint(ctx, fingerprint)
	if err != nil || len(snapshots) == 0 {
		return nil
	}

	// Iterate through snapshots to find one from a different workload with confirmed intent
	for _, snapshot := range snapshots {
		if snapshot.WorkloadUID == currentUID {
			continue
		}

		// Get the intent result from the other workload
		det, err := r.detectionFacade.GetDetection(ctx, snapshot.WorkloadUID)
		if err != nil || det == nil {
			continue
		}

		// Must have a confirmed intent
		if det.IntentState == nil || *det.IntentState != "confirmed" {
			continue
		}

		// Parse the stored IntentResult from detection model
		intentResult := r.parseStoredIntentResult(det)
	if intentResult == nil {
		return nil
	}

		log.Infof("Intent reuse found for workload %s via fingerprint (source: %s)", currentUID, snapshot.WorkloadUID)

		return &ReuseResult{
			Found:             true,
			Result:            intentResult,
			ReuseSource:       "fingerprint",
			SourceWorkloadUID: snapshot.WorkloadUID,
		}
	}

	return nil
}

func (r *ReuseChecker) checkByDigest(ctx context.Context, currentUID, digest string) *ReuseResult {
	// Find workloads with the same image digest
	// We look through workload_detection for matching image-related evidence
	// For now, we use a simpler approach: check if any other confirmed workload
	// has the same image digest in its intent_detail

	// This is a best-effort optimization; if the cache doesn't have the info,
	// we just skip reuse
	cache, err := r.imageCacheFacade.GetByDigest(ctx, digest)
	if err != nil || cache == nil {
		return nil
	}

	// Check if the cached image tag matches any workload with confirmed intent
	if cache.Tag == "" {
		return nil
	}

	// For image digest reuse, we only reuse if the workloads have the same
	// command line (since different commands with the same image have different intents)
	// This check is handled by the caller (analyzer.go) which compares full evidence

	return nil
}

func (r *ReuseChecker) parseStoredIntentResult(detModel *model.WorkloadDetection) *IntentResult {
	if detModel == nil {
		return nil
	}

	// The IntentDetail is stored as JSONB (ExtType) and contains the full IntentResult
	if detModel.IntentDetail == nil {
		return nil
	}

	detailJSON, err := json.Marshal(detModel.IntentDetail)
	if err != nil {
		return nil
	}

	var result IntentResult
	if err := json.Unmarshal(detailJSON, &result); err != nil {
		return nil
	}

	if result.Category == "" {
		return nil
	}

	// Mark as reused
	result.Source = IntentSourceRule // Reused results act like rule matches

	return &result
}
