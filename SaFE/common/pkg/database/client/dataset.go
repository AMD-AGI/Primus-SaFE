/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package client

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	sqrl "github.com/Masterminds/squirrel"
	"k8s.io/klog/v2"

	dbutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/utils"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
)

const (
	TDataset = "dataset"
)

var (
	getDatasetCmd       = fmt.Sprintf(`SELECT * FROM %s WHERE dataset_id = $1 LIMIT 1`, TDataset)
	insertDatasetFormat = `INSERT INTO ` + TDataset + ` (%s) VALUES (%s)`
	updateDatasetCmd    = fmt.Sprintf(`UPDATE %s 
		SET display_name = :display_name,
		    description = :description,
		    dataset_type = :dataset_type,
		    status = :status,
		    s3_path = :s3_path,
		    total_size = :total_size,
		    file_count = :file_count,
		    message = :message,
		    update_time = :update_time,
		    deletion_time = :deletion_time
		WHERE dataset_id = :dataset_id`, TDataset)
)

// Dataset status constants
const (
	DatasetStatusPending     = "Pending"     // Upload completed, waiting for download to workspace
	DatasetStatusDownloading = "Downloading" // Download in progress
	DatasetStatusReady       = "Ready"       // Download completed successfully
	DatasetStatusFailed      = "Failed"      // Download failed
)

// Dataset label for OpsJob
const (
	DatasetIdLabel = "dataset-id"
)

// UpsertDataset performs the UpsertDataset operation.
func (c *Client) UpsertDataset(ctx context.Context, dataset *Dataset) error {
	if dataset == nil {
		return commonerrors.NewBadRequest("the input is empty")
	}
	db, err := c.getDB()
	if err != nil {
		return err
	}

	var datasets []*Dataset
	if err = db.SelectContext(ctx, &datasets, getDatasetCmd, dataset.DatasetId); err != nil {
		klog.ErrorS(err, "failed to select dataset", "id", dataset.DatasetId)
		return err
	}
	if len(datasets) > 0 && datasets[0] != nil {
		_, err = db.NamedExecContext(ctx, updateDatasetCmd, dataset)
		if err != nil {
			klog.ErrorS(err, "failed to upsert dataset db", "id", dataset.DatasetId)
		}
	} else {
		_, err = db.NamedExecContext(ctx, generateCommand(*dataset, insertDatasetFormat, "id"), dataset)
		if err != nil {
			klog.ErrorS(err, "failed to insert dataset db", "id", dataset.DatasetId)
		}
	}
	return err
}

// SelectDatasets retrieves multiple dataset records.
func (c *Client) SelectDatasets(ctx context.Context, query sqrl.Sqlizer, orderBy []string, limit, offset int) ([]*Dataset, error) {
	startTime := time.Now().UTC()
	defer func() {
		if query != nil {
			strQuery := dbutils.CvtToSqlStr(query)
			klog.Infof("select dataset, query: %s, orderBy: %v, limit: %d, offset: %d, cost (%v)",
				strQuery, orderBy, limit, offset, time.Since(startTime))
		}
	}()
	db, err := c.getDB()
	if err != nil {
		return nil, err
	}

	sql, args, err := sqrl.Select("*").PlaceholderFormat(sqrl.Dollar).
		From(TDataset).
		Where(query).
		OrderBy(orderBy...).
		Limit(uint64(limit)).
		Offset(uint64(offset)).ToSql()
	if err != nil {
		return nil, err
	}

	var datasets []*Dataset
	if c.RequestTimeout > 0 {
		ctx2, cancel := context.WithTimeout(ctx, c.RequestTimeout)
		defer cancel()
		err = db.SelectContext(ctx2, &datasets, sql, args...)
	} else {
		err = db.SelectContext(ctx, &datasets, sql, args...)
	}
	return datasets, err
}

// CountDatasets returns the total count of datasets matching the criteria.
func (c *Client) CountDatasets(ctx context.Context, query sqrl.Sqlizer) (int, error) {
	db, err := c.getDB()
	if err != nil {
		return 0, err
	}
	sql, args, err := sqrl.Select("COUNT(*)").PlaceholderFormat(sqrl.Dollar).From(TDataset).Where(query).ToSql()
	if err != nil {
		return 0, err
	}
	var cnt int
	err = db.GetContext(ctx, &cnt, sql, args...)
	return cnt, err
}

// GetDataset retrieves a dataset by ID.
func (c *Client) GetDataset(ctx context.Context, datasetId string) (*Dataset, error) {
	if datasetId == "" {
		return nil, commonerrors.NewBadRequest("datasetId is empty")
	}
	dbTags := GetDatasetFieldTags()
	dbSql := sqrl.And{
		sqrl.Eq{GetFieldTag(dbTags, "IsDeleted"): false},
		sqrl.Eq{GetFieldTag(dbTags, "DatasetId"): datasetId},
	}
	datasets, err := c.SelectDatasets(ctx, dbSql, nil, 1, 0)
	if err != nil {
		klog.ErrorS(err, "failed to select dataset", "sql", dbutils.CvtToSqlStr(dbSql))
		return nil, err
	}
	if len(datasets) == 0 {
		return nil, commonerrors.NewNotFoundWithMessage(fmt.Sprintf("dataset %s not found", datasetId))
	}
	return datasets[0], nil
}

// SetDatasetDeleted marks a dataset as deleted in the database.
func (c *Client) SetDatasetDeleted(ctx context.Context, datasetId string) error {
	db, err := c.getDB()
	if err != nil {
		return err
	}
	cmd := fmt.Sprintf(`UPDATE %s SET is_deleted=true, deletion_time=$2 WHERE dataset_id=$1`, TDataset)
	_, err = db.ExecContext(ctx, cmd, datasetId, time.Now().UTC())
	if err != nil {
		klog.ErrorS(err, "failed to update dataset db", "DatasetId", datasetId)
		return err
	}
	return nil
}

// UpdateDatasetStatus updates the status of a dataset.
func (c *Client) UpdateDatasetStatus(ctx context.Context, datasetId, status, message string) error {
	db, err := c.getDB()
	if err != nil {
		return err
	}
	cmd := fmt.Sprintf(`UPDATE %s SET status=$1, message=$2, update_time=$3 WHERE dataset_id=$4`, TDataset)
	_, err = db.ExecContext(ctx, cmd, status, message, time.Now().UTC(), datasetId)
	if err != nil {
		klog.ErrorS(err, "failed to update dataset status", "DatasetId", datasetId)
		return err
	}
	return nil
}

// UpdateDatasetFileInfo updates the file information of a dataset.
func (c *Client) UpdateDatasetFileInfo(ctx context.Context, datasetId string, totalSize int64, fileCount int) error {
	db, err := c.getDB()
	if err != nil {
		return err
	}
	cmd := fmt.Sprintf(`UPDATE %s SET total_size=$1, file_count=$2, update_time=$3 WHERE dataset_id=$4`, TDataset)
	_, err = db.ExecContext(ctx, cmd, totalSize, fileCount, time.Now().UTC(), datasetId)
	if err != nil {
		klog.ErrorS(err, "failed to update dataset file info", "DatasetId", datasetId)
		return err
	}
	return nil
}

// UpdateDatasetLocalPath updates a specific workspace's download status in local_paths.
// It also recalculates the overall status based on all workspaces.
// Logic: Any Ready -> Ready, Any Downloading -> Downloading, All Failed -> Failed
func (c *Client) UpdateDatasetLocalPath(ctx context.Context, datasetId, workspace, status, message string) error {
	db, err := c.getDB()
	if err != nil {
		return err
	}

	// Get current dataset to read existing local_paths
	var datasets []*Dataset
	if err = db.SelectContext(ctx, &datasets, getDatasetCmd, datasetId); err != nil {
		klog.ErrorS(err, "failed to get dataset for local path update", "datasetId", datasetId)
		return err
	}
	if len(datasets) == 0 {
		return commonerrors.NewNotFoundWithMessage(fmt.Sprintf("dataset %s not found", datasetId))
	}
	dataset := datasets[0]

	// Parse existing local_paths
	var localPaths []DatasetLocalPathDB
	if dataset.LocalPaths != "" {
		if err = json.Unmarshal([]byte(dataset.LocalPaths), &localPaths); err != nil {
			klog.ErrorS(err, "failed to parse local_paths JSON", "datasetId", datasetId)
			// Continue with empty slice if parsing fails
			localPaths = []DatasetLocalPathDB{}
		}
	}

	// Update or add the workspace entry
	found := false
	for i := range localPaths {
		if localPaths[i].Workspace == workspace {
			localPaths[i].Status = status
			localPaths[i].Message = message
			found = true
			break
		}
	}
	if !found {
		localPaths = append(localPaths, DatasetLocalPathDB{
			Workspace: workspace,
			Status:    status,
			Message:   message,
		})
	}

	// Calculate overall status
	overallStatus := calculateOverallStatus(localPaths)

	// Marshal local_paths back to JSON
	localPathsJSON, err := json.Marshal(localPaths)
	if err != nil {
		klog.ErrorS(err, "failed to marshal local_paths", "datasetId", datasetId)
		return err
	}

	// Update database - update both local_paths and status
	cmd := fmt.Sprintf(`UPDATE %s SET local_paths=$1, status=$2, update_time=$3 WHERE dataset_id=$4`, TDataset)
	_, err = db.ExecContext(ctx, cmd, string(localPathsJSON), overallStatus, time.Now().UTC(), datasetId)
	if err != nil {
		klog.ErrorS(err, "failed to update dataset local path", "DatasetId", datasetId, "Workspace", workspace)
		return err
	}

	klog.InfoS("updated dataset local path", "datasetId", datasetId, "workspace", workspace, "status", status, "overallStatus", overallStatus)
	return nil
}

// calculateOverallStatus calculates the overall status from all workspace statuses.
// Logic: Any Ready -> Ready, Any Downloading -> Downloading, All Failed -> Failed
func calculateOverallStatus(localPaths []DatasetLocalPathDB) string {
	if len(localPaths) == 0 {
		return DatasetStatusPending
	}

	hasReady := false
	hasDownloading := false
	allFailed := true

	for _, lp := range localPaths {
		switch lp.Status {
		case DatasetStatusReady:
			hasReady = true
			allFailed = false
		case DatasetStatusDownloading:
			hasDownloading = true
			allFailed = false
		case DatasetStatusPending:
			allFailed = false
		}
	}

	// Priority: Ready > Downloading > Failed > Pending
	if hasReady {
		return DatasetStatusReady
	}
	if hasDownloading {
		return DatasetStatusDownloading
	}
	if allFailed {
		return DatasetStatusFailed
	}
	return DatasetStatusPending
}
