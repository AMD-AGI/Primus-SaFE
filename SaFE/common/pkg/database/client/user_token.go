/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package client

import (
	"context"
	"time"

	sqrl "github.com/Masterminds/squirrel"
	"k8s.io/klog/v2"

	dbutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/utils"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
)

const (
	TUserToken            = "user_token"
	upsertUserTokenFormat = `
INSERT INTO ` + TUserToken + ` (%s)
VALUES (%s)
ON CONFLICT (user_id) 
DO UPDATE SET
    session_id = EXCLUDED.session_id,
    token = EXCLUDED.token,
    refresh_token = EXCLUDED.refresh_token,
    creation_time = EXCLUDED.creation_time,
    expire_time = EXCLUDED.expire_time
`
)

// UpsertUserToken inserts or updates a user token in the database
// Uses PostgreSQL's ON CONFLICT clause to ensure atomic upsert operation
// Prevents race conditions in concurrent environments
func (c *Client) UpsertUserToken(ctx context.Context, userToken *UserToken) error {
	if userToken == nil {
		return commonerrors.NewBadRequest("the input is empty")
	}
	db, err := c.getDB()
	if err != nil {
		return err
	}
	upsertUserTokenCmd := generateCommand(*userToken, upsertUserTokenFormat, "")
	_, err = db.NamedExecContext(ctx, upsertUserTokenCmd, userToken)
	if err != nil {
		klog.ErrorS(err, "failed to upsert user_token db", "user_id", userToken.UserId)
		return err
	}
	return nil
}

// SelectUserTokens retrieves multiple userTokens records.
func (c *Client) SelectUserTokens(ctx context.Context, query sqrl.Sqlizer, orderBy []string, limit, offset int) ([]*UserToken, error) {
	startTime := time.Now().UTC()
	defer func() {
		if query != nil {
			strQuery := dbutils.CvtToSqlStr(query)
			klog.Infof("select userToken, query: %s, limit: %d, offset: %d, cost (%v)",
				strQuery, limit, offset, time.Since(startTime))
		}
	}()
	db, err := c.getDB()
	if err != nil {
		return nil, err
	}

	sql, args, err := sqrl.Select("*").PlaceholderFormat(sqrl.Dollar).
		From(TUserToken).
		Where(query).
		OrderBy(orderBy...).
		Limit(uint64(limit)).
		Offset(uint64(offset)).ToSql()
	if err != nil {
		return nil, err
	}

	var userTokens []*UserToken
	if c.RequestTimeout > 0 {
		ctx2, cancel := context.WithTimeout(ctx, c.RequestTimeout)
		defer cancel()
		err = db.SelectContext(ctx2, &userTokens, sql, args...)
	} else {
		err = db.SelectContext(ctx, &userTokens, sql, args...)
	}
	return userTokens, err
}
