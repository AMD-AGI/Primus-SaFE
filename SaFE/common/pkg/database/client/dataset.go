/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package client

import (
	"context"
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
