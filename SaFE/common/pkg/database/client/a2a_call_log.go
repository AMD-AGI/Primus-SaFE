/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package client

import (
	"context"
	"fmt"

	sqrl "github.com/Masterminds/squirrel"
)

const (
	TA2ACallLog = "a2a_call_log"
)

var (
	insertA2ACallLogFormat = `INSERT INTO ` + TA2ACallLog + ` (%s) VALUES (%s) RETURNING id`
)

func (c *Client) InsertA2ACallLog(ctx context.Context, log *A2ACallLog) error {
	if log == nil {
		return fmt.Errorf("the input is empty")
	}
	db, err := c.getDB()
	if err != nil {
		return err
	}
	cmd := generateCommand(*log, insertA2ACallLogFormat, "id")
	rows, err := db.NamedQueryContext(ctx, cmd, log)
	if err != nil {
		return fmt.Errorf("failed to insert a2a call log: %v", err)
	}
	defer rows.Close()
	if rows.Next() {
		if err := rows.Scan(&log.Id); err != nil {
			return fmt.Errorf("failed to scan a2a call log id: %v", err)
		}
	}
	return nil
}

func (c *Client) SelectA2ACallLogs(ctx context.Context, query sqrl.Sqlizer, orderBy []string, limit, offset int) ([]*A2ACallLog, error) {
	db, err := c.getDB()
	if err != nil {
		return nil, err
	}
	builder := sqrl.Select("*").PlaceholderFormat(sqrl.Dollar).From(TA2ACallLog).Where(query)
	if offset > 0 || limit > 0 {
		builder = builder.Limit(uint64(limit)).Offset(uint64(offset))
	}
	sql, args, err := builder.OrderBy(orderBy...).ToSql()
	if err != nil {
		return nil, err
	}
	var logs []*A2ACallLog
	if c.RequestTimeout > 0 {
		ctx2, cancel := context.WithTimeout(ctx, c.RequestTimeout)
		defer cancel()
		err = db.SelectContext(ctx2, &logs, sql, args...)
	} else {
		err = db.SelectContext(ctx, &logs, sql, args...)
	}
	return logs, err
}

func (c *Client) CountA2ACallLogs(ctx context.Context, query sqrl.Sqlizer) (int, error) {
	db, err := c.getDB()
	if err != nil {
		return 0, err
	}
	sql, args, err := sqrl.Select("COUNT(*)").PlaceholderFormat(sqrl.Dollar).From(TA2ACallLog).Where(query).ToSql()
	if err != nil {
		return 0, err
	}
	var cnt int
	err = db.GetContext(ctx, &cnt, sql, args...)
	return cnt, err
}
