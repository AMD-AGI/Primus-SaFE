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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"

	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	dbutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/utils"
)

const (
	TPublicKey = "public_key"
)

var (
	getPublicKeyCmd       = fmt.Sprintf(`SELECT  * FROM %s WHERE public_key=$1 and user_id=$2 and delete_time=$3 LIMIT 1`, TPublicKey)
	insertPublicKeyFormat = `INSERT INTO ` + TPublicKey + ` (%s) VALUES (%s)`
)

// InsertPublicKey inserts a new public key record into the database.
func (c *Client) InsertPublicKey(ctx context.Context, publicKey *PublicKey) error {
	if publicKey == nil {
		return commonerrors.NewBadRequest("the input is empty")
	}
	db, err := c.getDB()
	if err != nil {
		return err
	}

	var publicKeys []*PublicKey
	if err = db.SelectContext(ctx, &publicKeys, getPublicKeyCmd, publicKey.PublicKey, publicKey.UserId, nil); err != nil {
		return fmt.Errorf("failed to select publicKey err: %v", err)
	}
	if len(publicKeys) > 0 {
		return nil
	}

	_, err = db.NamedExecContext(ctx, genInsertCommand(*publicKey, insertPublicKeyFormat, "id"), publicKey)
	if err != nil {
		return fmt.Errorf("failed to insert publicKey db err: %v", err)
	}
	return err
}

// SelectPublicKeys retrieves multiple public key records.
func (c *Client) SelectPublicKeys(ctx context.Context, query sqrl.Sqlizer, orderBy []string, limit, offset int) ([]*PublicKey, error) {
	startTime := time.Now().UTC()
	defer func() {
		if query != nil {
			strQuery := dbutils.CvtToSqlStr(query)
			klog.Infof("select publicKey, query: %s, limit: %d, offset: %d, cost (%v)",
				strQuery, limit, offset, time.Since(startTime))
		}
	}()
	db, err := c.getDB()
	if err != nil {
		return nil, err
	}

	builder := sqrl.Select("*").PlaceholderFormat(sqrl.Dollar).
		From(TPublicKey).
		Where(query)
	if offset > 0 || limit > 0 {
		builder = builder.Limit(uint64(limit)).
			Offset(uint64(offset))
	}
	sql, args, err := builder.OrderBy(orderBy...).ToSql()
	if err != nil {
		return nil, err
	}

	var publicKeys []*PublicKey
	if c.RequestTimeout > 0 {
		ctx2, cancel := context.WithTimeout(ctx, c.RequestTimeout)
		defer cancel()
		err = db.SelectContext(ctx2, &publicKeys, sql, args...)
	} else {
		err = db.SelectContext(ctx, &publicKeys, sql, args...)
	}
	return publicKeys, err
}

// CountPublicKeys returns the count of resources.
func (c *Client) CountPublicKeys(ctx context.Context, query sqrl.Sqlizer) (int, error) {
	db, err := c.getDB()
	if err != nil {
		return 0, err
	}
	sql, args, err := sqrl.Select("COUNT(*)").PlaceholderFormat(sqrl.Dollar).From(TPublicKey).Where(query).ToSql()
	if err != nil {
		return 0, err
	}
	var cnt int
	err = db.GetContext(ctx, &cnt, sql, args...)
	return cnt, err
}

// DeletePublicKey deletes a public key record.
func (c *Client) DeletePublicKey(ctx context.Context, userId string, id int64) error {
	db, err := c.getDB()
	if err != nil {
		return err
	}
	nowTime := dbutils.NullMetaV1Time(&metav1.Time{Time: time.Now().UTC()})
	cmd := fmt.Sprintf(`UPDATE %s SET delete_time=$1 WHERE id=$2 and user_id=$3`, TPublicKey)
	_, err = db.ExecContext(ctx, cmd, nowTime, id, userId)
	if err != nil {
		klog.ErrorS(err, "failed to update publicKey db. ", "id", id)
		return err
	}
	return nil
}

// SetPublicKeyStatus sets the PublicKeyStatus value.
func (c *Client) SetPublicKeyStatus(ctx context.Context, userId string, id int64, status bool) error {
	db, err := c.getDB()
	if err != nil {
		return err
	}
	nowTime := dbutils.NullMetaV1Time(&metav1.Time{Time: time.Now().UTC()})
	cmd := fmt.Sprintf(`UPDATE %s SET status=$1, update_time=$2 WHERE id=$3 and user_id=$4`,
		TPublicKey)
	_, err = db.ExecContext(ctx, cmd, status, nowTime, id, userId)
	if err != nil {
		klog.ErrorS(err, "failed to update publicKey db. ", "id", id)
		return err
	}
	return nil
}

// SetPublicKeyDescription sets the PublicKeyDescription value.
func (c *Client) SetPublicKeyDescription(ctx context.Context, userId string, id int64, description string) error {
	db, err := c.getDB()
	if err != nil {
		return err
	}
	nowTime := dbutils.NullMetaV1Time(&metav1.Time{Time: time.Now().UTC()})
	cmd := fmt.Sprintf(`UPDATE %s SET description=$1, update_time=$2 WHERE id=$3 and user_id=$4`,
		TPublicKey)
	_, err = db.ExecContext(ctx, cmd, description, nowTime, id, userId)
	if err != nil {
		klog.ErrorS(err, "failed to update publicKey db. ", "id", id)
		return err
	}
	return nil
}

// GetPublicKeyByUserId returns the PublicKeyByUserId value.
func (c *Client) GetPublicKeyByUserId(ctx context.Context, userId string) ([]*PublicKey, error) {
	if userId == "" {
		return nil, commonerrors.NewBadRequest("userId is empty")
	}
	dbTags := GetPublicKeyFieldTags()
	dbSql := sqrl.And{
		sqrl.Eq{GetFieldTag(dbTags, "DeleteTime"): nil},
		sqrl.Eq{GetFieldTag(dbTags, "UserId"): userId},
		sqrl.Eq{GetFieldTag(dbTags, "Status"): true},
	}
	publicKeys, err := c.SelectPublicKeys(ctx, dbSql, nil, 0, 0)
	if err != nil {
		klog.ErrorS(err, "failed to select publicKey", "sql", dbutils.CvtToSqlStr(dbSql))
		return nil, err
	}
	if len(publicKeys) == 0 {
		return nil, commonerrors.NewNotFound("userId", userId)
	}
	return publicKeys, nil
}
