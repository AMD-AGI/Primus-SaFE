/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package client

import (
	"context"
	"fmt"
	"time"

	sqrl "github.com/Masterminds/squirrel"
	"k8s.io/klog/v2"

	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
)

const (
	TApiKey = "api_keys"
)

var (
	insertApiKeyFormat = `INSERT INTO ` + TApiKey + ` (%s) VALUES (%s) RETURNING id`
)

// InsertApiKey inserts a new API key record into the database.
func (c *Client) InsertApiKey(ctx context.Context, apiKey *ApiKey) error {
	if apiKey == nil {
		return commonerrors.NewBadRequest("the input is empty")
	}
	db, err := c.getDB()
	if err != nil {
		return err
	}

	cmd := generateCommand(*apiKey, insertApiKeyFormat, "id")
	rows, err := db.NamedQueryContext(ctx, cmd, apiKey)
	if err != nil {
		return fmt.Errorf("failed to insert api key: %v", err)
	}
	defer rows.Close()

	if rows.Next() {
		if err := rows.Scan(&apiKey.Id); err != nil {
			return fmt.Errorf("failed to scan api key id: %v", err)
		}
	}
	return nil
}

// SelectApiKeys retrieves multiple API key records.
func (c *Client) SelectApiKeys(ctx context.Context, query sqrl.Sqlizer, orderBy []string, limit, offset int) ([]*ApiKey, error) {
	db, err := c.getDB()
	if err != nil {
		return nil, err
	}

	builder := sqrl.Select("*").PlaceholderFormat(sqrl.Dollar).
		From(TApiKey).
		Where(query)
	if offset > 0 || limit > 0 {
		builder = builder.Limit(uint64(limit)).
			Offset(uint64(offset))
	}
	sql, args, err := builder.OrderBy(orderBy...).ToSql()
	if err != nil {
		return nil, err
	}

	var apiKeys []*ApiKey
	if c.RequestTimeout > 0 {
		ctx2, cancel := context.WithTimeout(ctx, c.RequestTimeout)
		defer cancel()
		err = db.SelectContext(ctx2, &apiKeys, sql, args...)
	} else {
		err = db.SelectContext(ctx, &apiKeys, sql, args...)
	}
	return apiKeys, err
}

// CountApiKeys counts API keys based on query conditions.
func (c *Client) CountApiKeys(ctx context.Context, query sqrl.Sqlizer) (int, error) {
	db, err := c.getDB()
	if err != nil {
		return 0, err
	}
	sql, args, err := sqrl.Select("COUNT(*)").PlaceholderFormat(sqrl.Dollar).From(TApiKey).Where(query).ToSql()
	if err != nil {
		return 0, err
	}
	var cnt int
	err = db.GetContext(ctx, &cnt, sql, args...)
	return cnt, err
}

// GetApiKeyById retrieves an API key by its ID.
func (c *Client) GetApiKeyById(ctx context.Context, id int64) (*ApiKey, error) {
	db, err := c.getDB()
	if err != nil {
		return nil, err
	}

	cmd := fmt.Sprintf(`SELECT * FROM %s WHERE id=$1 LIMIT 1`, TApiKey)
	var apiKey ApiKey
	err = db.GetContext(ctx, &apiKey, cmd, id)
	if err != nil {
		return nil, err
	}
	return &apiKey, nil
}

// GetApiKeyByKey retrieves an API key by the key value.
func (c *Client) GetApiKeyByKey(ctx context.Context, key string) (*ApiKey, error) {
	if key == "" {
		return nil, commonerrors.NewBadRequest("api key is empty")
	}
	db, err := c.getDB()
	if err != nil {
		return nil, err
	}

	cmd := fmt.Sprintf(`SELECT * FROM %s WHERE api_key=$1 LIMIT 1`, TApiKey)
	var apiKey ApiKey
	err = db.GetContext(ctx, &apiKey, cmd, key)
	if err != nil {
		klog.ErrorS(err, "failed to get api key from database")
		return nil, err
	}
	return &apiKey, nil
}

// SetApiKeyDeleted performs soft delete on an API key.
func (c *Client) SetApiKeyDeleted(ctx context.Context, userId string, id int64) error {
	db, err := c.getDB()
	if err != nil {
		return err
	}
	nowTime := time.Now().UTC()
	cmd := fmt.Sprintf(`UPDATE %s SET deleted=$1, deletion_time=$2 WHERE id=$3 AND user_id=$4`, TApiKey)
	result, err := db.ExecContext(ctx, cmd, true, nowTime, id, userId)
	if err != nil {
		klog.ErrorS(err, "failed to delete api key", "id", id, "userId", userId)
		return err
	}
	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return commonerrors.NewNotFoundWithMessage("API key not found or not owned by user")
	}
	return nil
}
