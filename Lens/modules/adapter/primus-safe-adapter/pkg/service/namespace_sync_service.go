/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package service

import (
	"context"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	primusSafeV1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	corev1 "k8s.io/api/core/v1"
)

const (
	// AMDGPUResourceName is the AMD GPU resource name
	AMDGPUResourceName = "amd.com/gpu"
	// NVIDIAGPUResourceName is the NVIDIA GPU resource name
	NVIDIAGPUResourceName = "nvidia.com/gpu"
	// DefaultHistoryStartOffset is the default time offset for new history records (2 days ago)
	DefaultHistoryStartOffset = -48 * time.Hour
)

// NamespaceSyncService provides workspace to namespace_info synchronization service
type NamespaceSyncService struct {
	k8sClient client.Client
}

// NewNamespaceSyncService creates a new namespace sync service
func NewNamespaceSyncService(k8sClient client.Client) *NamespaceSyncService {
	return &NamespaceSyncService{
		k8sClient: k8sClient,
	}
}

// Name returns the task name
func (s *NamespaceSyncService) Name() string {
	return "namespace-sync"
}

// Run executes the namespace sync task
func (s *NamespaceSyncService) Run(ctx context.Context) error {
	startTime := time.Now()
	log.Info("Starting namespace sync from Workspace CRs")

	// 1. Get all Workspaces from K8s
	workspaces, err := s.listAllWorkspaces(ctx)
	if err != nil {
		log.Errorf("Failed to list workspaces: %v", err)
		return err
	}

	log.Infof("Found %d workspaces", len(workspaces))

	// Build workspace name set for quick lookup
	workspaceNames := make(map[string]*primusSafeV1.Workspace)
	for i := range workspaces {
		workspaceNames[workspaces[i].Name] = &workspaces[i]
	}

	// 2. Get all namespace_info records (including soft deleted for recovery)
	allNamespaceInfos, err := s.listAllNamespaceInfos(ctx)
	if err != nil {
		log.Errorf("Failed to list namespace infos: %v", err)
		return err
	}

	log.Infof("Found %d namespace_info records (including soft deleted)", len(allNamespaceInfos))

	// Build namespace_info name set
	namespaceInfoMap := make(map[string]*model.NamespaceInfo)
	for _, nsInfo := range allNamespaceInfos {
		namespaceInfoMap[nsInfo.Name] = nsInfo
	}

	// 3. Sync: create or update namespace_info for each workspace
	createdCount := 0
	updatedCount := 0
	recoveredCount := 0

	for _, workspace := range workspaces {
		clusterID := primusSafeV1.GetClusterId(&workspace)
		facade := s.getFacade(clusterID)

		existingInfo := namespaceInfoMap[workspace.Name]
		created, updated, recovered, err := s.syncWorkspaceToNamespaceInfo(ctx, &workspace, existingInfo, facade)
		if err != nil {
			log.Errorf("Failed to sync workspace %s: %v", workspace.Name, err)
			continue
		}

		if created {
			createdCount++
		}
		if updated {
			updatedCount++
		}
		if recovered {
			recoveredCount++
		}
	}

	// 4. Soft delete namespace_info records that no longer have corresponding workspace
	deletedCount := 0
	for name, nsInfo := range namespaceInfoMap {
		// Skip already soft deleted records
		if nsInfo.DeletedAt.Valid {
			continue
		}

		// If workspace doesn't exist, soft delete the namespace_info
		if _, exists := workspaceNames[name]; !exists {
			if err := s.softDeleteNamespaceInfo(ctx, nsInfo); err != nil {
				log.Errorf("Failed to soft delete namespace_info %s: %v", name, err)
				continue
			}
			deletedCount++
			log.Infof("Soft deleted namespace_info: %s (workspace no longer exists)", name)
		}
	}

	// 5. Sync node-namespace mappings
	nodeMappingStats, err := s.syncNodeNamespaceMappings(ctx, workspaces)
	if err != nil {
		log.Errorf("Failed to sync node namespace mappings: %v", err)
		// Continue even if mapping sync fails
	}

	duration := time.Since(startTime)
	log.Infof("Namespace sync completed: created=%d, updated=%d, recovered=%d, deleted=%d, node_mappings=%+v, duration=%v",
		createdCount, updatedCount, recoveredCount, deletedCount, nodeMappingStats, duration)

	return nil
}

// listAllWorkspaces lists all Workspace CRs from K8s
func (s *NamespaceSyncService) listAllWorkspaces(ctx context.Context) ([]primusSafeV1.Workspace, error) {
	workspaceList := &primusSafeV1.WorkspaceList{}
	err := s.k8sClient.List(ctx, workspaceList)
	if err != nil {
		return nil, err
	}
	return workspaceList.Items, nil
}

// listAllNamespaceInfos lists all namespace_info records including soft deleted ones
func (s *NamespaceSyncService) listAllNamespaceInfos(ctx context.Context) ([]*model.NamespaceInfo, error) {
	facade := database.GetFacade()
	return facade.GetNamespaceInfo().ListAllIncludingDeleted(ctx)
}

// syncWorkspaceToNamespaceInfo syncs a single workspace to namespace_info
// Returns (created, updated, recovered, error)
func (s *NamespaceSyncService) syncWorkspaceToNamespaceInfo(
	ctx context.Context,
	workspace *primusSafeV1.Workspace,
	existingInfo *model.NamespaceInfo,
	facade database.FacadeInterface,
) (created, updated, recovered bool, err error) {

	// Extract GPU resource from workspace
	gpuResource := s.extractGpuResource(workspace)
	if gpuResource == 0 {
		log.Debugf("Workspace %s has no GPU resource, skipping", workspace.Name)
		return false, false, false, nil
	}

	// Get GPU model
	gpuModel := s.getGpuModel(workspace)

	now := time.Now()

	if existingInfo == nil {
		// Create new namespace_info
		newInfo := &model.NamespaceInfo{
			Name:        workspace.Name,
			GpuModel:    gpuModel,
			GpuResource: gpuResource,
			CreatedAt:   now,
			UpdatedAt:   now,
		}

		err = facade.GetNamespaceInfo().Create(ctx, newInfo)
		if err != nil {
			return false, false, false, err
		}

		log.Infof("Created namespace_info: name=%s, gpu_model=%s, gpu_resource=%d",
			workspace.Name, gpuModel, gpuResource)
		return true, false, false, nil
	}

	// Check if record was soft deleted and needs recovery
	if existingInfo.DeletedAt.Valid {
		// Recover soft deleted record
		err = s.recoverNamespaceInfo(ctx, existingInfo, gpuModel, gpuResource)
		if err != nil {
			return false, false, false, err
		}

		log.Infof("Recovered soft deleted namespace_info: name=%s, gpu_model=%s, gpu_resource=%d",
			workspace.Name, gpuModel, gpuResource)
		return false, false, true, nil
	}

	// Update existing record if changed
	if existingInfo.GpuModel != gpuModel || existingInfo.GpuResource != gpuResource {
		existingInfo.GpuModel = gpuModel
		existingInfo.GpuResource = gpuResource
		existingInfo.UpdatedAt = now

		err = facade.GetNamespaceInfo().Update(ctx, existingInfo)
		if err != nil {
			return false, false, false, err
		}

		log.Debugf("Updated namespace_info: name=%s, gpu_model=%s, gpu_resource=%d",
			workspace.Name, gpuModel, gpuResource)
		return false, true, false, nil
	}

	return false, false, false, nil
}

// extractGpuResource extracts GPU resource count from workspace status
func (s *NamespaceSyncService) extractGpuResource(workspace *primusSafeV1.Workspace) int32 {
	if workspace.Status.TotalResources == nil {
		return 0
	}

	// Try AMD GPU resource
	if gpuQuantity, ok := workspace.Status.TotalResources[corev1.ResourceName(AMDGPUResourceName)]; ok {
		return int32(gpuQuantity.Value())
	}

	// Try NVIDIA GPU resource as fallback
	if gpuQuantity, ok := workspace.Status.TotalResources[corev1.ResourceName(NVIDIAGPUResourceName)]; ok {
		return int32(gpuQuantity.Value())
	}

	return 0
}

// getGpuModel gets GPU model from workspace
func (s *NamespaceSyncService) getGpuModel(workspace *primusSafeV1.Workspace) string {
	// Use node flavor as GPU model if available
	if workspace.Spec.NodeFlavor != "" {
		return workspace.Spec.NodeFlavor
	}
	return ""
}

// getFacade returns the appropriate facade based on cluster ID
func (s *NamespaceSyncService) getFacade(clusterID string) database.FacadeInterface {
	if clusterID != "" {
		return database.GetFacadeForCluster(clusterID)
	}
	return database.GetFacade()
}

// softDeleteNamespaceInfo performs soft delete on a namespace_info record
func (s *NamespaceSyncService) softDeleteNamespaceInfo(ctx context.Context, nsInfo *model.NamespaceInfo) error {
	facade := database.GetFacade()
	return facade.GetNamespaceInfo().DeleteByName(ctx, nsInfo.Name)
}

// recoverNamespaceInfo recovers a soft deleted namespace_info record
func (s *NamespaceSyncService) recoverNamespaceInfo(ctx context.Context, nsInfo *model.NamespaceInfo, gpuModel string, gpuResource int32) error {
	facade := database.GetFacade()
	return facade.GetNamespaceInfo().Recover(ctx, nsInfo.Name, gpuModel, gpuResource)
}

// NodeMappingSyncStats holds statistics for node-namespace mapping sync
type NodeMappingSyncStats struct {
	Added   int
	Removed int
	Updated int
}

// syncNodeNamespaceMappings syncs node-namespace mappings for all workspaces
func (s *NamespaceSyncService) syncNodeNamespaceMappings(ctx context.Context, workspaces []primusSafeV1.Workspace) (*NodeMappingSyncStats, error) {
	stats := &NodeMappingSyncStats{}

	// Get all nodes from K8s
	nodeList := &corev1.NodeList{}
	if err := s.k8sClient.List(ctx, nodeList); err != nil {
		return stats, err
	}

	// Build a map of workspace name -> nodes
	workspaceNodes := make(map[string][]corev1.Node)
	for _, node := range nodeList.Items {
		workspaceID := node.Labels[primusSafeV1.WorkspaceIdLabel]
		if workspaceID != "" {
			workspaceNodes[workspaceID] = append(workspaceNodes[workspaceID], node)
		}
	}

	// Process each workspace
	for _, workspace := range workspaces {
		clusterID := primusSafeV1.GetClusterId(&workspace)
		facade := s.getFacade(clusterID)

		// Get namespace_info for this workspace
		nsInfo, err := facade.GetNamespaceInfo().GetByName(ctx, workspace.Name)
		if err != nil {
			log.Errorf("Failed to get namespace_info for %s: %v", workspace.Name, err)
			continue
		}
		if nsInfo == nil {
			log.Debugf("No namespace_info found for workspace %s, skipping node mapping sync", workspace.Name)
			continue
		}

		// Get current nodes for this workspace
		currentNodes := workspaceNodes[workspace.Name]
		currentNodeNames := make(map[string]corev1.Node)
		for _, node := range currentNodes {
			currentNodeNames[node.Name] = node
		}

		// Get existing mappings from database
		existingMappings, err := facade.GetNodeNamespaceMapping().ListActiveByNamespaceName(ctx, workspace.Name)
		if err != nil {
			log.Errorf("Failed to list existing mappings for %s: %v", workspace.Name, err)
			continue
		}

		existingNodeNames := make(map[string]*model.NodeNamespaceMapping)
		for _, mapping := range existingMappings {
			existingNodeNames[mapping.NodeName] = mapping
		}

		now := time.Now()

		// Find nodes to add (in K8s but not in DB)
		for nodeName := range currentNodeNames {
			if _, exists := existingNodeNames[nodeName]; !exists {
				// Get node from DB to get node ID
				dbNode, err := facade.GetNode().GetNodeByName(ctx, nodeName)
				if err != nil {
					log.Errorf("Failed to get node %s from DB: %v", nodeName, err)
					continue
				}
				if dbNode == nil {
					log.Debugf("Node %s not found in DB, skipping", nodeName)
					continue
				}

				// Create new mapping
				newMapping := &model.NodeNamespaceMapping{
					NodeID:        dbNode.ID,
					NodeName:      nodeName,
					NamespaceID:   nsInfo.ID,
					NamespaceName: workspace.Name,
					CreatedAt:     now,
					UpdatedAt:     now,
				}

				if err := facade.GetNodeNamespaceMapping().Create(ctx, newMapping); err != nil {
					log.Errorf("Failed to create mapping for node %s -> namespace %s: %v", nodeName, workspace.Name, err)
					continue
				}

				// Create history record
				if err := s.createOrUpdateHistory(ctx, facade, dbNode.ID, nodeName, nsInfo.ID, workspace.Name, newMapping.ID, "added", now); err != nil {
					log.Errorf("Failed to create history for node %s -> namespace %s: %v", nodeName, workspace.Name, err)
				}

				stats.Added++
				log.Infof("Added node-namespace mapping: node=%s, namespace=%s", nodeName, workspace.Name)
			}
		}

		// Find nodes to remove (in DB but not in K8s)
		for nodeName, mapping := range existingNodeNames {
			if _, exists := currentNodeNames[nodeName]; !exists {
				// Soft delete the mapping
				if err := facade.GetNodeNamespaceMapping().SoftDelete(ctx, mapping.ID); err != nil {
					log.Errorf("Failed to soft delete mapping for node %s -> namespace %s: %v", nodeName, workspace.Name, err)
					continue
				}

				// Update history record_end
				latestHistory, err := facade.GetNodeNamespaceMapping().GetLatestHistoryByNodeAndNamespace(ctx, mapping.NodeID, mapping.NamespaceID)
				if err != nil {
					log.Errorf("Failed to get latest history for node %s -> namespace %s: %v", nodeName, workspace.Name, err)
				} else if latestHistory != nil && latestHistory.RecordEnd.IsZero() {
					if err := facade.GetNodeNamespaceMapping().UpdateHistoryRecordEnd(ctx, latestHistory.ID, now); err != nil {
						log.Errorf("Failed to update history record_end for node %s -> namespace %s: %v", nodeName, workspace.Name, err)
					}
				}

				// Create a new history record for removal
				newHistory := &model.NodeNamespaceMappingHistory{
					MappingID:     mapping.ID,
					NodeID:        mapping.NodeID,
					NodeName:      nodeName,
					NamespaceID:   mapping.NamespaceID,
					NamespaceName: workspace.Name,
					Action:        "removed",
					RecordStart:   now,
					RecordEnd:     now, // Removal record ends immediately
				}
				if err := facade.GetNodeNamespaceMapping().CreateHistory(ctx, newHistory); err != nil {
					log.Errorf("Failed to create removal history for node %s -> namespace %s: %v", nodeName, workspace.Name, err)
				}

				stats.Removed++
				log.Infof("Removed node-namespace mapping: node=%s, namespace=%s", nodeName, workspace.Name)
			}
		}
	}

	return stats, nil
}

// createOrUpdateHistory creates or updates history record for a node-namespace mapping
func (s *NamespaceSyncService) createOrUpdateHistory(
	ctx context.Context,
	facade database.FacadeInterface,
	nodeID int32,
	nodeName string,
	namespaceID int64,
	namespaceName string,
	mappingID int32,
	action string,
	now time.Time,
) error {
	// Check if there's an existing history record
	latestHistory, err := facade.GetNodeNamespaceMapping().GetLatestHistoryByNodeAndNamespace(ctx, nodeID, namespaceID)
	if err != nil {
		return err
	}

	// If there's an active history record (record_end is zero), close it first
	if latestHistory != nil && latestHistory.RecordEnd.IsZero() {
		if err := facade.GetNodeNamespaceMapping().UpdateHistoryRecordEnd(ctx, latestHistory.ID, now); err != nil {
			return err
		}
	}

	// Determine record_start time
	// If no previous history exists, assume the node joined 2 days ago
	recordStart := now
	if latestHistory == nil {
		recordStart = now.Add(DefaultHistoryStartOffset)
		log.Debugf("No previous history for node %s -> namespace %s, assuming joined at %v", nodeName, namespaceName, recordStart)
	}

	// Create new history record
	newHistory := &model.NodeNamespaceMappingHistory{
		MappingID:     mappingID,
		NodeID:        nodeID,
		NodeName:      nodeName,
		NamespaceID:   namespaceID,
		NamespaceName: namespaceName,
		Action:        action,
		RecordStart:   recordStart,
		// RecordEnd is zero (NULL) for active records
	}

	return facade.GetNodeNamespaceMapping().CreateHistory(ctx, newHistory)
}
