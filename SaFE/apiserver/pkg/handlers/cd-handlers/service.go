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
	newReq := &dbclient.DeploymentRequest{
		DeployName:     username,
		Status:         StatusPendingApproval,
		EnvConfig:      envConfig,
		Description:    dbutils.NullString(fmt.Sprintf("Rollback to version from request %d", reqId)),
		RollbackFromId: sql.NullInt64{Int64: reqId, Valid: true},
	}

	return s.dbClient.CreateDeploymentRequest(ctx, newReq)
}

// mergeWithLatestSnapshot merges current request config with the latest snapshot
// This ensures all historical image versions are preserved, and only the specified ones are updated
func (s *Service) mergeWithLatestSnapshot(ctx context.Context, currentConfig DeploymentConfig) (DeploymentConfig, error) {
	// Get the latest snapshot
	snapshots, err := s.dbClient.ListEnvironmentSnapshots(ctx, nil, []string{"created_at DESC"}, 1, 0)
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

// GetCurrentEnvConfig reads the current .env file content from the latest snapshot
func (s *Service) GetCurrentEnvConfig(ctx context.Context) (content string, err error) {
	// Get from the latest successful deployment snapshot
	snapshots, err := s.dbClient.ListEnvironmentSnapshots(ctx, nil, []string{"created_at DESC"}, 1, 0)
	if err != nil {
		return "", fmt.Errorf("failed to read last .env record and no snapshot available: db_error=%v", err)
	}

	if len(snapshots) == 0 {
		return "", fmt.Errorf("failed to read last .env record and no snapshot available: %v", err)
	}

	// Parse the snapshot config
	var config DeploymentConfig
	if err := json.Unmarshal([]byte(snapshots[0].EnvConfig), &config); err != nil {
		return "", fmt.Errorf("failed to parse snapshot config: %v", err)
	}

	if config.EnvFileConfig == "" {
		return "", fmt.Errorf("snapshot does not contain env_file_config")
	}

	return config.EnvFileConfig, nil
}

// CreateSnapshot creates a backup of the current FULL state
// It merges the new request config with the previous snapshot to ensure complete state record
func (s *Service) CreateSnapshot(ctx context.Context, reqId int64, newConfigStr string) error {
	// 1. Parse new config (partial or full)
	var newConfig DeploymentConfig
	if err := json.Unmarshal([]byte(newConfigStr), &newConfig); err != nil {
		return fmt.Errorf("failed to parse new config: %v", err)
	}

	// 2. Get latest snapshot to find previous state
	var finalConfig DeploymentConfig

	snapshots, err := s.dbClient.ListEnvironmentSnapshots(ctx, nil, []string{"created_at DESC"}, 1, 0)
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

	// 4. Marshal final merged config
	finalConfigJSON, err := json.Marshal(finalConfig)
	if err != nil {
		return fmt.Errorf("failed to marshal final config: %v", err)
	}

	// 5. Save to DB
	snapshot := &dbclient.EnvironmentSnapshot{
		DeploymentRequestId: reqId,
		EnvConfig:           string(finalConfigJSON),
	}
	_, err = s.dbClient.CreateEnvironmentSnapshot(ctx, snapshot)
	return err
}

// cvtDBRequestToItem converts a database DeploymentRequest to a DeploymentRequestItem
func (s *Service) cvtDBRequestToItem(req *dbclient.DeploymentRequest) *DeploymentRequestItem {
	return &DeploymentRequestItem{
		Id:              req.Id,
		DeployName:      req.DeployName,
		Status:          req.Status,
		ApproverName:    dbutils.ParseNullString(req.ApproverName),
		ApprovalResult:  dbutils.ParseNullString(req.ApprovalResult),
		Description:     dbutils.ParseNullString(req.Description),
		RejectionReason: dbutils.ParseNullString(req.RejectionReason),
		FailureReason:   dbutils.ParseNullString(req.FailureReason),
		RollbackFromId:  req.RollbackFromId.Int64,
		CreatedAt:       dbutils.ParseNullTimeToString(req.CreatedAt),
		UpdatedAt:       dbutils.ParseNullTimeToString(req.UpdatedAt),
		ApprovedAt:      dbutils.ParseNullTimeToString(req.ApprovedAt),
	}
}
