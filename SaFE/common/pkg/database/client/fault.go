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
	TFault = "fault"
)

var (
	getFaultCmd       = fmt.Sprintf(`SELECT * FROM %s WHERE uid = $1 LIMIT 1`, TFault)
	insertFaultFormat = `INSERT INTO ` + TFault + ` (%s) VALUES (%s)`
	updateFaultCmd    = fmt.Sprintf(`UPDATE %s 
		SET phase = :phase,
		    create_time = :create_time,
		    update_time = :update_time,
		    delete_time = :delete_time 
		WHERE uid = :uid`, TFault)
	deleteFaultCmd = fmt.Sprintf(`DELETE FROM %s WHERE uid = $1`, TFault)
)

// UpsertFault performs the UpsertFault operation.
func (c *Client) UpsertFault(ctx context.Context, fault *Fault) error {
	if fault == nil {
		return commonerrors.NewBadRequest("input is empty")
	}
	db, err := c.getDB()
	if err != nil {
		return err
	}
	if _, err = c.GetFault(ctx, fault.Uid); err == nil {
		if _, err = db.NamedExecContext(ctx, updateFaultCmd, fault); err != nil {
			klog.ErrorS(err, "failed to upsert fault db")
			return err
		}
	} else {
		_, err = db.NamedExecContext(ctx, genInsertCommand(*fault, insertFaultFormat, "id"), fault)
		if err != nil {
			klog.ErrorS(err, "failed to insert fault db")
			return err
		}
	}
	return nil
}

// SelectFaults performs the SelectFaults operation.
func (c *Client) SelectFaults(ctx context.Context, query sqrl.Sqlizer, sortBy, order string, limit, offset int) ([]*Fault, error) {
	db, err := c.getDB()
	if err != nil {
		return nil, err
	}
	orderBy := func() []string {
		var results []string
		if sortBy == "" || order == "" {
			return results
		}
		if order == DESC {
			results = append(results, fmt.Sprintf("%s desc", sortBy))
		} else {
			results = append(results, fmt.Sprintf("%s asc", sortBy))
		}
		return results
	}()
	sql, args, err := sqrl.Select("*").PlaceholderFormat(sqrl.Dollar).
		From(TFault).
		Where(query).
		OrderBy(orderBy...).
		Limit(uint64(limit)).
		Offset(uint64(offset)).ToSql()
	if err != nil {
		return nil, err
	}

	var faults []*Fault
	if c.RequestTimeout > 0 {
		ctx2, cancel := context.WithTimeout(ctx, c.RequestTimeout)
		defer cancel()
		err = db.SelectContext(ctx2, &faults, sql, args...)
	} else {
		err = db.SelectContext(ctx, &faults, sql, args...)
	}
	return faults, err
}

// CountFaults returns the count of resources.
func (c *Client) CountFaults(ctx context.Context, query sqrl.Sqlizer) (int, error) {
	db, err := c.getDB()
	if err != nil {
		return 0, err
	}
	sql, args, err := sqrl.Select("COUNT(*)").PlaceholderFormat(sqrl.Dollar).From(TFault).Where(query).ToSql()
	if err != nil {
		return 0, err
	}
	var cnt int
	err = db.GetContext(ctx, &cnt, sql, args...)
	return cnt, err
}

// GetFault returns the Fault value.
func (c *Client) GetFault(ctx context.Context, uid string) (*Fault, error) {
	if uid == "" {
		return nil, commonerrors.NewBadRequest("the faultUId is empty")
	}
	db, err := c.getDB()
	if err != nil {
		return nil, err
	}
	var faults []*Fault
	if err = db.SelectContext(ctx, &faults, getFaultCmd, uid); err != nil {
		return nil, err
	}
	if len(faults) > 0 && faults[0] != nil {
		return faults[0], nil
	}
	return nil, commonerrors.NewNotFoundWithMessage(fmt.Sprintf("fault %s not found", uid))
}

// DeleteFault removes the specified item.
func (c *Client) DeleteFault(ctx context.Context, uid string) error {
	if uid == "" {
		return commonerrors.NewBadRequest("the faultUId is empty")
	}
	db, err := c.getDB()
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, deleteFaultCmd, uid)
	return err
}
