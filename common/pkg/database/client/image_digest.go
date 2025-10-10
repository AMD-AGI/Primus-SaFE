package client

import (
	"context"
	"fmt"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	sqrl "github.com/Masterminds/squirrel"
	"k8s.io/klog/v2"
)

const (
	TImageDigest = "image_digest"
)

// ===========================
// ImageDigest CRUD
// ===========================

var (
	getImageDigestCmd       = fmt.Sprintf(`SELECT * FROM %s WHERE id = $1 LIMIT 1`, TImageDigest)
	insertImageDigestFormat = `INSERT INTO ` + TImageDigest + ` (%s) VALUES (%s)`
	updateImageDigestCmd    = fmt.Sprintf(`UPDATE %s 
		SET size = :size,
		    os = :os,
		    architecture = :architecture,
		    digest = :digest,
		    type = :type,
		    updated_at = :updated_at,
		    deleted_at = :deleted_at
		WHERE id = :id`, TImageDigest)
)

// UpsertImageDigest 插入或更新镜像摘要记录
func (c *Client) UpsertImageDigest(ctx context.Context, d *ImageDigest) error {
	if d == nil {
		return commonerrors.NewBadRequest("the input is empty")
	}
	db, err := c.getDB()
	if err != nil {
		return err
	}

	var list []*ImageDigest
	if err = db.SelectContext(ctx, &list, getImageDigestCmd, d.ID); err != nil {
		return err
	}

	if len(list) > 0 && list[0] != nil {
		if _, err = db.NamedExecContext(ctx, updateImageDigestCmd, d); err != nil {
			klog.ErrorS(err, "failed to upsert image_digest", "id", d.ID)
			return err
		}
	} else {
		_, err = db.NamedExecContext(ctx, genInsertCommand(*d, insertImageDigestFormat, "id"), d)
		if err != nil {
			klog.ErrorS(err, "failed to insert image_digest", "id", d.ID)
			return err
		}
	}
	return nil
}

// SelectImageDigests 按条件查询 image_digest
func (c *Client) SelectImageDigests(ctx context.Context, query sqrl.Sqlizer, sortBy, order string, limit, offset int) ([]*ImageDigest, error) {
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
		From(TImageDigest).
		Where(query).
		OrderBy(orderBy...).
		Limit(uint64(limit)).
		Offset(uint64(offset)).ToSql()
	if err != nil {
		return nil, err
	}

	var items []*ImageDigest
	if c.RequestTimeout > 0 {
		ctx2, cancel := context.WithTimeout(ctx, c.RequestTimeout)
		defer cancel()
		err = db.SelectContext(ctx2, &items, sql, args...)
	} else {
		err = db.SelectContext(ctx, &items, sql, args...)
	}
	return items, err
}

// CountImageDigests 返回满足条件的条数
func (c *Client) CountImageDigests(ctx context.Context, query sqrl.Sqlizer) (int, error) {
	db, err := c.getDB()
	if err != nil {
		return 0, err
	}
	sql, args, err := sqrl.Select("COUNT(*)").
		PlaceholderFormat(sqrl.Dollar).
		From(TImageDigest).
		Where(query).
		ToSql()
	if err != nil {
		return 0, err
	}
	var cnt int
	err = db.GetContext(ctx, &cnt, sql, args...)
	return cnt, err
}

// DeleteImageDigest 逻辑删除
func (c *Client) DeleteImageDigest(ctx context.Context, id int64) error {
	db, err := c.getDB()
	if err != nil {
		return err
	}
	cmd := fmt.Sprintf(`UPDATE %s SET deleted_at = NOW() WHERE id = $1`, TImageDigest)
	_, err = db.ExecContext(ctx, cmd, id)
	if err != nil {
		klog.ErrorS(err, "failed to delete image_digest", "id", id)
	}
	return err
}
