/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
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
)

func (c *Client) UpsertFault(ctx context.Context, fault *Fault) error {
	if fault == nil {
		return nil
	}
	db := c.db.Unsafe()
	var faults []*Fault
	var err error
	if err = db.SelectContext(ctx, &faults, getFaultCmd, fault.Uid); err != nil {
		return err
	}
	if len(faults) > 0 && faults[0] != nil {
		if _, err = db.NamedExecContext(ctx, updateFaultCmd, fault); err != nil {
			klog.ErrorS(err, "failed to upsert fault db")
		}
	} else {
		_, err = db.NamedExecContext(ctx, genInsertCommand(*fault, insertFaultFormat, "id"), fault)
		if err != nil {
			klog.ErrorS(err, "failed to insert fault db")
		}
	}

	return err
}

func (c *Client) SelectFaults(ctx context.Context, query sqrl.Sqlizer, sortBy, order string, limit, offset int) ([]*Fault, error) {
	if c.db == nil {
		return nil, commonerrors.NewInternalError("The client of db has not been initialized")
	}
	db := c.db.Unsafe()
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
		ctx2, cancel := context.WithTimeout(ctx, time.Duration(c.RequestTimeout)*time.Second)
		defer cancel()
		err = db.SelectContext(ctx2, &faults, sql, args...)
	} else {
		err = db.SelectContext(ctx, &faults, sql, args...)
	}
	return faults, err
}

func (c *Client) CountFaults(ctx context.Context, query sqrl.Sqlizer) (int, error) {
	if c.db == nil {
		return 0, commonerrors.NewInternalError("The client of db has not been initialized")
	}
	db := c.db.Unsafe()
	sql, args, err := sqrl.Select("COUNT(*)").PlaceholderFormat(sqrl.Dollar).From(TFault).Where(query).ToSql()
	if err != nil {
		return 0, err
	}
	var cnt int
	err = db.GetContext(ctx, &cnt, sql, args...)
	return cnt, err
}
