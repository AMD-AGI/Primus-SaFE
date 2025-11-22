/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package client

import (
	"context"
	"fmt"

	sqrl "github.com/Masterminds/squirrel"
	"k8s.io/klog/v2"

	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
)

const (
	TInference = "inference"
)

var (
	getInferenceCmd       = fmt.Sprintf(`SELECT * FROM %s WHERE inference_id = $1 LIMIT 1`, TInference)
	insertInferenceFormat = `INSERT INTO ` + TInference + ` (%s) VALUES (%s)`
	updateInferenceCmd    = fmt.Sprintf(`UPDATE %s 
		SET display_name = :display_name,
		    description = :description,
		    user_name = :user_name,
		    model_form = :model_form,
		    model_name = :model_name,
		    instance = :instance,
		    resource = :resource,
		    config = :config,
		    phase = :phase,
		    events = :events,
		    message = :message,
		    update_time = :update_time,
		    deletion_time = :deletion_time
		WHERE inference_id = :inference_id`, TInference)
)

// UpsertInference performs the UpsertInference operation.
func (c *Client) UpsertInference(ctx context.Context, inference *Inference) error {
	if inference == nil {
		return commonerrors.NewBadRequest("the input is empty")
	}
	db, err := c.getDB()
	if err != nil {
		return err
	}

	var inferences []*Inference
	if err = db.SelectContext(ctx, &inferences, getInferenceCmd, inference.InferenceId); err != nil {
		klog.ErrorS(err, "failed to select inference", "id", inference.InferenceId)
		return err
	}
	if len(inferences) > 0 && inferences[0] != nil {
		_, err = db.NamedExecContext(ctx, updateInferenceCmd, inference)
		if err != nil {
			klog.ErrorS(err, "failed to update inference db", "id", inference.InferenceId)
		}
	} else {
		_, err = db.NamedExecContext(ctx, generateCommand(*inference, insertInferenceFormat, "id"), inference)
		if err != nil {
			klog.ErrorS(err, "failed to insert inference db", "id", inference.InferenceId)
		}
	}
	return err
}

// SelectInferences retrieves multiple inference records.
func (c *Client) SelectInferences(ctx context.Context, query sqrl.Sqlizer, orderBy []string, limit, offset int) ([]*Inference, error) {
	db, err := c.getDB()
	if err != nil {
		return nil, err
	}

	sql, args, err := sqrl.Select("*").PlaceholderFormat(sqrl.Dollar).
		From(TInference).
		Where(query).
		OrderBy(orderBy...).
		Limit(uint64(limit)).
		Offset(uint64(offset)).ToSql()
	if err != nil {
		return nil, err
	}

	var inferences []*Inference
	if c.RequestTimeout > 0 {
		ctx2, cancel := context.WithTimeout(ctx, c.RequestTimeout)
		defer cancel()
		err = db.SelectContext(ctx2, &inferences, sql, args...)
	} else {
		err = db.SelectContext(ctx, &inferences, sql, args...)
	}
	return inferences, err
}

// CountInferences returns the total count of inferences matching the criteria.
func (c *Client) CountInferences(ctx context.Context, query sqrl.Sqlizer) (int, error) {
	db, err := c.getDB()
	if err != nil {
		return 0, err
	}
	sql, args, err := sqrl.Select("COUNT(*)").PlaceholderFormat(sqrl.Dollar).From(TInference).Where(query).ToSql()
	if err != nil {
		return 0, err
	}
	var cnt int
	err = db.GetContext(ctx, &cnt, sql, args...)
	return cnt, err
}

// SetInferenceDeleted marks an inference as deleted in the database.
func (c *Client) SetInferenceDeleted(ctx context.Context, inferenceId string) error {
	db, err := c.getDB()
	if err != nil {
		return err
	}
	cmd := fmt.Sprintf(`UPDATE %s SET is_deleted=true WHERE inference_id=$1`, TInference)
	_, err = db.ExecContext(ctx, cmd, inferenceId)
	if err != nil {
		klog.ErrorS(err, "failed to update inference db", "InferenceId", inferenceId)
		return err
	}
	return nil
}

// GetInference retrieves an inference by ID.
func (c *Client) GetInference(ctx context.Context, inferenceId string) (*Inference, error) {
	if inferenceId == "" {
		return nil, commonerrors.NewBadRequest("inferenceId is empty")
	}
	dbTags := GetInferenceFieldTags()
	dbSql := sqrl.And{
		sqrl.Eq{GetFieldTag(dbTags, "IsDeleted"): false},
		sqrl.Eq{GetFieldTag(dbTags, "InferenceId"): inferenceId},
	}
	inferences, err := c.SelectInferences(ctx, dbSql, nil, 1, 0)
	if err != nil {
		klog.ErrorS(err, "failed to select inference", "inferenceId", inferenceId)
		return nil, err
	}
	if len(inferences) == 0 {
		return nil, commonerrors.NewNotFound("Inference", inferenceId)
	}
	return inferences[0], nil
}
