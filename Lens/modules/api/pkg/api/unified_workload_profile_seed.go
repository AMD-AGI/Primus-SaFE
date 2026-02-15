// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package api

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	dbModel "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/errors"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/mcp/unified"
)

// ===== Seed endpoint: batch-create workload_detection from gpu_workload + image analysis =====

// WorkloadProfileSeedRequest triggers batch seeding of workload_detection records.
type WorkloadProfileSeedRequest struct {
	Cluster string `json:"cluster" query:"cluster" mcp:"cluster,description=Target cluster name (optional)"`
	Limit   int    `json:"limit" query:"limit" mcp:"limit,description=Max workloads to seed per call (default 200)"`
}

// WorkloadProfileSeedResponse returns seeding result.
type WorkloadProfileSeedResponse struct {
	Seeded    int      `json:"seeded"`
	Skipped   int      `json:"skipped"`
	Enriched  int      `json:"enriched"`
	Message   string   `json:"message"`
	Workloads []string `json:"workloads,omitempty"`
}

func init() {
	unified.Register(&unified.EndpointDef[WorkloadProfileSeedRequest, WorkloadProfileSeedResponse]{
		Name:        "workload_profile_seed",
		Description: "Seed workload_detection records for existing GPU workloads that lack detection entries. Optionally enriches with image analysis framework hints.",
		HTTPMethod:  "POST",
		HTTPPath:    "/workload-profile/seed",
		MCPToolName: "lens_workload_profile_seed",
		Handler:     handleWorkloadProfileSeed,
	})
}

func handleWorkloadProfileSeed(ctx context.Context, req *WorkloadProfileSeedRequest) (*WorkloadProfileSeedResponse, error) {
	cm := clientsets.GetClusterManager()
	clients, err := cm.GetClusterClientsOrDefault(req.Cluster)
	if err != nil {
		return nil, err
	}
	clusterName := clients.ClusterName

	limit := req.Limit
	if limit <= 0 {
		limit = 200
	}
	if limit > 1000 {
		limit = 1000
	}

	facade := database.GetFacadeForCluster(clusterName)

	// Get raw *gorm.DB via WorkloadStatistic facade (it exposes GetDB())
	db := facade.GetWorkloadStatistic().GetDB()
	if db == nil {
		return nil, errors.NewError().
			WithCode(errors.InternalError).
			WithMessage("database not available for cluster: " + clusterName)
	}

	// Step 1: Find root GPU workloads that have no workload_detection record
	type workloadRow struct {
		UID       string
		Kind      string
		Name      string
		Namespace string
		Status    string
	}
	var undetected []workloadRow
	err = db.WithContext(ctx).
		Raw(`
			SELECT gw.uid, gw.kind, gw.name, gw.namespace, gw.status
			FROM gpu_workload gw
			LEFT JOIN workload_detection wd ON gw.uid = wd.workload_uid
			WHERE gw.deleted_at IS NULL
			  AND gw.parent_uid = ''
			  AND wd.id IS NULL
			ORDER BY gw.created_at DESC
			LIMIT ?
		`, limit).
		Scan(&undetected).Error
	if err != nil {
		return nil, errors.NewError().
			WithCode(errors.InternalError).
			WithMessage("failed to query undetected workloads: " + err.Error())
	}

	if len(undetected) == 0 {
		var existingCount int64
		db.WithContext(ctx).Table("workload_detection").Count(&existingCount)
		return &WorkloadProfileSeedResponse{
			Seeded:  0,
			Skipped: 0,
			Message: fmt.Sprintf("No undetected workloads found. %d detection records already exist.", existingCount),
		}, nil
	}

	// Step 2: Build a map of workload_uid -> container_image from gpu_pods
	workloadUIDs := make([]string, len(undetected))
	for i, w := range undetected {
		workloadUIDs[i] = w.UID
	}

	type podImageRow struct {
		OwnerUID       string
		ContainerImage string
	}
	var podImages []podImageRow
	err = db.WithContext(ctx).
		Raw(`
			SELECT DISTINCT owner_uid, container_image
			FROM gpu_pods
			WHERE owner_uid IN ?
			  AND container_image != ''
		`, workloadUIDs).
		Scan(&podImages).Error
	if err != nil {
		log.Warnf("[seed] Failed to query pod images: %v", err)
	}

	wlImageMap := make(map[string]string)
	for _, pi := range podImages {
		if _, exists := wlImageMap[pi.OwnerUID]; !exists {
			wlImageMap[pi.OwnerUID] = pi.ContainerImage
		}
	}

	// Step 3: Get framework hints from image_registry_cache for matched images
	type imageHintRow struct {
		ImageRef       string
		FrameworkHints json.RawMessage
	}
	imageRefs := make([]string, 0, len(wlImageMap))
	for _, img := range wlImageMap {
		imageRefs = append(imageRefs, img)
	}

	imageHintMap := make(map[string]map[string]interface{})
	if len(imageRefs) > 0 {
		var hints []imageHintRow
		err = db.WithContext(ctx).
			Raw(`
				SELECT image_ref, framework_hints
				FROM image_registry_cache
				WHERE image_ref IN ?
				  AND framework_hints IS NOT NULL
				  AND framework_hints != '{}'::jsonb
			`, imageRefs).
			Scan(&hints).Error
		if err != nil {
			log.Warnf("[seed] Failed to query image hints: %v", err)
		} else {
			for _, h := range hints {
				var parsed map[string]interface{}
				if json.Unmarshal(h.FrameworkHints, &parsed) == nil {
					imageHintMap[h.ImageRef] = parsed
				}
			}
		}
	}

	// Step 4: Create workload_detection records
	seeded := 0
	enriched := 0
	seededUIDs := make([]string, 0, len(undetected))

	detFacade := facade.GetWorkloadDetection()
	pendingState := "pending"

	for _, w := range undetected {
		detection := &dbModel.WorkloadDetection{
			WorkloadUID:    w.UID,
			Status:         "unknown",
			DetectionState: "pending",
			IntentState:    &pendingState,
			CreatedAt:      time.Now(),
			UpdatedAt:      time.Now(),
		}

		// Try to enrich from image analysis hints
		if img, ok := wlImageMap[w.UID]; ok {
			if hints, ok := imageHintMap[img]; ok {
				if frameworks, ok := hints["frameworks"].([]interface{}); ok && len(frameworks) > 0 {
					if fw, ok := frameworks[0].(string); ok {
						detection.Framework = fw
						detection.Status = "suspected"
						detection.Confidence = 0.3
						enriched++
					}
				}
			}
		}

		err = detFacade.CreateDetection(ctx, detection)
		if err != nil {
			log.Warnf("[seed] Failed to create detection for %s: %v", w.UID, err)
			continue
		}
		seeded++
		seededUIDs = append(seededUIDs, w.UID)
	}

	return &WorkloadProfileSeedResponse{
		Seeded:    seeded,
		Skipped:   len(undetected) - seeded,
		Enriched:  enriched,
		Message:   fmt.Sprintf("Seeded %d workload detection records in cluster %s (%d enriched from image analysis)", seeded, clusterName, enriched),
		Workloads: seededUIDs,
	}, nil
}
