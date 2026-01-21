/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package cdhandlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	sqrl "github.com/Masterminds/squirrel"
	"github.com/pmezard/go-difflib/difflib"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"

	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	dbutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/utils"
)

// Service handles business logic for CD
type Service struct {
	dbClient  dbclient.Interface
	clientSet kubernetes.Interface
}

func NewService(dbClient dbclient.Interface, clientSet kubernetes.Interface) *Service {
	return &Service{
		dbClient:  dbClient,
		clientSet: clientSet,
	}
}

const (
	// Host path on the node for persistent storage (used by handler.go for OpsJob)
	HostMountPath = "/mnt/primus-safe-cd"
)

// extractBranchFromEnvFileConfig extracts deploy_branch from env file content string.
// Returns empty string if not found, which means use default branch.
func extractBranchFromEnvFileConfig(envFileConfig string) string {
	for _, line := range strings.Split(envFileConfig, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "deploy_branch=") {
			branch := strings.TrimPrefix(line, "deploy_branch=")
			branch = strings.Trim(branch, "\"'") // Remove quotes if any
			return branch
		}
	}
	return "" // Default: empty means use default branch
}

// UpdateRequestStatus updates the status of a deployment request in the database
func (s *Service) UpdateRequestStatus(ctx context.Context, reqId int64, status, failureReason string) error {
	req, err := s.dbClient.GetDeploymentRequest(ctx, reqId)
	if err != nil {
		klog.ErrorS(err, "Failed to get request for update", "id", reqId)
		return err
	}

	req.Status = status
	if failureReason != "" {
		req.FailureReason = dbutils.NullString(failureReason)
	}

	return s.dbClient.UpdateDeploymentRequest(ctx, req)
}

// Rollback creates a new request based on a previous snapshot
func (s *Service) Rollback(ctx context.Context, reqId int64, username string) (int64, error) {
	// 1. Validate target request exists and is in valid state
	targetReq, err := s.dbClient.GetDeploymentRequest(ctx, reqId)
	if err != nil {
		return 0, err
	}

	if targetReq.Status != StatusDeployed {
		return 0, fmt.Errorf("cannot rollback to a request with status %s (must be %s)",
			targetReq.Status, StatusDeployed)
	}

	// 2. Get the full config from snapshot (not from request, because request may contain partial config)
	var envConfig string
	snapshot, err := s.dbClient.GetEnvironmentSnapshotByRequestId(ctx, reqId)
	if err != nil {
		// Snapshot not found, fallback to request's EnvConfig (for backward compatibility)
		klog.Warningf("Snapshot not found for request %d, falling back to request EnvConfig", reqId)
		envConfig = targetReq.EnvConfig
	} else {
		// Use snapshot's full config
		envConfig = snapshot.EnvConfig
	}

	// 3. Create a new request that applies the old config
	// Preserve deploy_type from target request
	deployType := targetReq.DeployType
	if deployType == "" {
		deployType = DeployTypeSafe // Default for backward compatibility
	}

	newReq := &dbclient.DeploymentRequest{
		DeployName:     username,
		DeployType:     deployType,
		Status:         StatusPendingApproval,
		EnvConfig:      envConfig,
		Description:    dbutils.NullString(fmt.Sprintf("Rollback to version from request %d", reqId)),
		RollbackFromId: sql.NullInt64{Int64: reqId, Valid: true},
	}

	return s.dbClient.CreateDeploymentRequest(ctx, newReq)
}

// mergeWithLatestSnapshot merges current request config with the latest snapshot
// This ensures all historical image versions are preserved, and only the specified ones are updated
// Note: This function is only used for Safe deployments
func (s *Service) mergeWithLatestSnapshot(ctx context.Context, currentConfig DeploymentConfig) (DeploymentConfig, error) {
	// Get the latest Safe snapshot (include empty deploy_type for backward compatibility)
	query := sqrl.Or{
		sqrl.Eq{"deploy_type": DeployTypeSafe},
		sqrl.Eq{"deploy_type": ""},
		sqrl.Expr("deploy_type IS NULL"),
	}
	snapshots, err := s.dbClient.ListEnvironmentSnapshots(ctx, query, []string{"created_at DESC"}, 1, 0)
	if err != nil {
		return currentConfig, fmt.Errorf("failed to get latest snapshot: %v", err)
	}

	if len(snapshots) == 0 {
		klog.Infof("No previous snapshot found, using current config only")
		return currentConfig, nil
	}

	// Parse the snapshot config
	var snapshotConfig DeploymentConfig
	if err := json.Unmarshal([]byte(snapshots[0].EnvConfig), &snapshotConfig); err != nil {
		return currentConfig, fmt.Errorf("failed to parse snapshot config: %v", err)
	}

	// Merge image versions: start with snapshot, then override with current request
	mergedImageVersions := make(map[string]string)

	// First, copy all image versions from snapshot
	for k, v := range snapshotConfig.ImageVersions {
		mergedImageVersions[k] = v
	}

	// Then, override with current request's image versions
	for k, v := range currentConfig.ImageVersions {
		mergedImageVersions[k] = v
		klog.Infof("Updating component %s: %s -> %s", k, snapshotConfig.ImageVersions[k], v)
	}

	// Merge env_file_config: use current if provided, otherwise use snapshot
	mergedEnvFileConfig := currentConfig.EnvFileConfig
	if mergedEnvFileConfig == "" {
		mergedEnvFileConfig = snapshotConfig.EnvFileConfig
		klog.Infof("Using env_file_config from latest snapshot")
	}

	return DeploymentConfig{
		ImageVersions: mergedImageVersions,
		EnvFileConfig: mergedEnvFileConfig,
	}, nil
}

// GetLatestConfig returns the latest snapshot configuration for the specified deploy type
func (s *Service) GetLatestConfig(ctx context.Context, deployType string) (*GetLatestConfigResp, error) {
	var query sqrl.Sqlizer
	if deployType == DeployTypeLens {
		query = sqrl.Eq{"deploy_type": DeployTypeLens}
	} else {
		// Safe: include empty deploy_type for backward compatibility
		query = sqrl.Or{
			sqrl.Eq{"deploy_type": DeployTypeSafe},
			sqrl.Eq{"deploy_type": ""},
			sqrl.Expr("deploy_type IS NULL"),
		}
		deployType = DeployTypeSafe // Normalize
	}

	snapshots, err := s.dbClient.ListEnvironmentSnapshots(ctx, query, []string{"created_at DESC"}, 1, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to get latest snapshot: %v", err)
	}

	if len(snapshots) == 0 {
		return nil, fmt.Errorf("no snapshot found for deploy_type=%s", deployType)
	}

	snapshot := snapshots[0]
	var config DeploymentConfig
	if err := json.Unmarshal([]byte(snapshot.EnvConfig), &config); err != nil {
		return nil, fmt.Errorf("failed to parse snapshot config: %v", err)
	}

	resp := &GetLatestConfigResp{
		Type:       deployType,
		SnapshotId: snapshot.Id,
	}

	if snapshot.CreatedAt.Valid {
		resp.CreatedAt = snapshot.CreatedAt.Time.Format("2006-01-02T15:04:05Z")
	}

	if deployType == DeployTypeLens {
		resp.Branch = config.Branch
		resp.ControlPlaneConfig = config.ControlPlaneConfig
		resp.DataPlaneConfig = config.DataPlaneConfig
	} else {
		resp.ImageVersions = config.ImageVersions
		resp.EnvFileConfig = config.EnvFileConfig
	}

	return resp, nil
}

// ComputeUnifiedDiff computes a unified diff between old and new content
func ComputeUnifiedDiff(oldContent, newContent, oldLabel, newLabel string) string {
	if oldContent == newContent {
		return "" // No changes
	}

	diff := difflib.UnifiedDiff{
		A:        difflib.SplitLines(oldContent),
		B:        difflib.SplitLines(newContent),
		FromFile: oldLabel,
		ToFile:   newLabel,
		Context:  3,
	}

	result, err := difflib.GetUnifiedDiffString(diff)
	if err != nil {
		klog.Warningf("Failed to compute diff: %v", err)
		return ""
	}
	return result
}

// GetLensConfigDiff computes diff between request config and base snapshot
// If baseSnapshotId is valid, use that specific snapshot; otherwise use latest snapshot
func (s *Service) GetLensConfigDiff(ctx context.Context, reqConfig DeploymentConfig, baseSnapshotId int64) (cpDiff, dpDiff string, err error) {
	var oldConfig DeploymentConfig

	if baseSnapshotId > 0 {
		// Use the specific base snapshot recorded at request creation time
		snapshot, err := s.dbClient.GetEnvironmentSnapshot(ctx, baseSnapshotId)
		if err == nil && snapshot != nil {
			if err := json.Unmarshal([]byte(snapshot.EnvConfig), &oldConfig); err != nil {
				klog.Warningf("Failed to parse base snapshot config: %v", err)
			}
		}
	} else {
		// Fallback: get the latest Lens snapshot (for backward compatibility)
		query := sqrl.Eq{"deploy_type": DeployTypeLens}
		snapshots, err := s.dbClient.ListEnvironmentSnapshots(ctx, query, []string{"created_at DESC"}, 1, 0)
		if err == nil && len(snapshots) > 0 {
			if err := json.Unmarshal([]byte(snapshots[0].EnvConfig), &oldConfig); err != nil {
				klog.Warningf("Failed to parse latest snapshot config: %v", err)
			}
		}
	}

	// Compute diffs
	cpDiff = ComputeUnifiedDiff(oldConfig.ControlPlaneConfig, reqConfig.ControlPlaneConfig, "before", "after")
	dpDiff = ComputeUnifiedDiff(oldConfig.DataPlaneConfig, reqConfig.DataPlaneConfig, "before", "after")

	return cpDiff, dpDiff, nil
}

// CreateSnapshot creates a backup of the current FULL state
// It merges the new request config with the previous snapshot to ensure complete state record
func (s *Service) CreateSnapshot(ctx context.Context, reqId int64, newConfigStr string, deployType string) error {
	// 1. Parse new config (partial or full)
	var newConfig DeploymentConfig
	if err := json.Unmarshal([]byte(newConfigStr), &newConfig); err != nil {
		return fmt.Errorf("failed to parse new config: %v", err)
	}

	var finalConfig DeploymentConfig

	if deployType == DeployTypeLens {
		// Lens: store full YAML content directly, no merging needed
		finalConfig = newConfig
	} else {
		// Safe: merge with previous snapshot
		// 2. Get latest Safe snapshot to find previous state (include empty deploy_type for backward compatibility)
		query := sqrl.Or{
			sqrl.Eq{"deploy_type": DeployTypeSafe},
			sqrl.Eq{"deploy_type": ""},
			sqrl.Expr("deploy_type IS NULL"),
		}
		snapshots, err := s.dbClient.ListEnvironmentSnapshots(ctx, query, []string{"created_at DESC"}, 1, 0)
		if err == nil && len(snapshots) > 0 {
			// Parse previous config
			if err := json.Unmarshal([]byte(snapshots[0].EnvConfig), &finalConfig); err != nil {
				klog.Warningf("Failed to parse previous snapshot config: %v", err)
				// If failed to parse previous, we start fresh
				finalConfig = DeploymentConfig{
					ImageVersions: make(map[string]string),
				}
			}
		} else {
			// No previous snapshot, initialize empty
			finalConfig = DeploymentConfig{
				ImageVersions: make(map[string]string),
			}
		}

		// 3. Merge Configs
		// 3.1 Merge Image Versions
		if finalConfig.ImageVersions == nil {
			finalConfig.ImageVersions = make(map[string]string)
		}
		for component, version := range newConfig.ImageVersions {
			finalConfig.ImageVersions[component] = version
		}

		// 3.2 Merge Env File Config
		// Only update if new config provides a non-empty env file content
		if newConfig.EnvFileConfig != "" {
			finalConfig.EnvFileConfig = newConfig.EnvFileConfig
		}
		// If newConfig.EnvFileConfig is empty, we keep finalConfig.EnvFileConfig (from previous snapshot)
	}

	// 4. Marshal final merged config
	finalConfigJSON, err := json.Marshal(finalConfig)
	if err != nil {
		return fmt.Errorf("failed to marshal final config: %v", err)
	}

	// 5. Save to DB
	snapshot := &dbclient.EnvironmentSnapshot{
		DeploymentRequestId: reqId,
		DeployType:          deployType,
		EnvConfig:           string(finalConfigJSON),
	}
	_, err = s.dbClient.CreateEnvironmentSnapshot(ctx, snapshot)
	return err
}

// cvtDBRequestToItem converts a database DeploymentRequest to a DeploymentRequestItem
func (s *Service) cvtDBRequestToItem(req *dbclient.DeploymentRequest) *DeploymentRequestItem {
	deployType := req.DeployType
	if deployType == "" {
		deployType = DeployTypeSafe // Default for backward compatibility
	}
	return &DeploymentRequestItem{
		Id:              req.Id,
		DeployName:      req.DeployName,
		DeployType:      deployType,
		Status:          req.Status,
		ApproverName:    dbutils.ParseNullString(req.ApproverName),
		ApprovalResult:  dbutils.ParseNullString(req.ApprovalResult),
		Description:     dbutils.ParseNullString(req.Description),
		RejectionReason: dbutils.ParseNullString(req.RejectionReason),
		FailureReason:   dbutils.ParseNullString(req.FailureReason),
		RollbackFromId:  req.RollbackFromId.Int64,
		WorkloadId:      dbutils.ParseNullString(req.WorkloadId),
		CreatedAt:       dbutils.ParseNullTimeToString(req.CreatedAt),
		UpdatedAt:       dbutils.ParseNullTimeToString(req.UpdatedAt),
		ApprovedAt:      dbutils.ParseNullTimeToString(req.ApprovedAt),
	}
}
