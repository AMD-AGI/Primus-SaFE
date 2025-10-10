package client

import (
	"context"
	"fmt"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	sqrl "github.com/Masterminds/squirrel"
	"k8s.io/klog/v2"
)

const (
	TImage = "image"
)

// ===========================
// Image CRUD
// ===========================

var (
	getImageCmd       = fmt.Sprintf(`SELECT * FROM %s WHERE id = $1 LIMIT 1`, TImage)
	insertImageFormat = `INSERT INTO ` + TImage + ` (%s) VALUES (%s)`
	updateImageCmd    = fmt.Sprintf(`UPDATE %s 
		SET tag = :tag,
		    description = :description,
		    source = :source,
		    status = :status,
		    relation_digest = :relation_digest,
		    created_by = :created_by,
		    updated_at = :updated_at,
		    deleted_at = :deleted_at,
		    deleted_by = :deleted_by
		WHERE id = :id`, TImage)
)

// UpsertImage 插入或更新镜像记录
func (c *Client) UpsertImage(ctx context.Context, img *Image) error {
	if img == nil {
		return commonerrors.NewBadRequest("the input is empty")
	}
	db, err := c.getDB()
	if err != nil {
		return err
	}

	var list []*Image
	if err = db.SelectContext(ctx, &list, getImageCmd, img.ID); err != nil {
		return err
	}

	if len(list) > 0 && list[0] != nil {
		if _, err = db.NamedExecContext(ctx, updateImageCmd, img); err != nil {
			klog.ErrorS(err, "failed to upsert image", "id", img.ID)
			return err
		}
	} else {
		_, err = db.NamedExecContext(ctx, genInsertCommand(*img, insertImageFormat, "id"), img)
		if err != nil {
			klog.ErrorS(err, "failed to insert image", "id", img.ID)
			return err
		}
	}
	return nil
}

// SelectImages 查询镜像列表
func (c *Client) SelectImages(ctx context.Context, query sqrl.Sqlizer, sortBy, order string, limit, offset int) ([]*Image, error) {
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
		From(TImage).
		Where(query).
		OrderBy(orderBy...).
		Limit(uint64(limit)).
		Offset(uint64(offset)).ToSql()
	if err != nil {
		return nil, err
	}

	var items []*Image
	if c.RequestTimeout > 0 {
		ctx2, cancel := context.WithTimeout(ctx, c.RequestTimeout)
		defer cancel()
		err = db.SelectContext(ctx2, &items, sql, args...)
	} else {
		err = db.SelectContext(ctx, &items, sql, args...)
	}
	return items, err
}

// CountImages 返回镜像总数
func (c *Client) CountImages(ctx context.Context, query sqrl.Sqlizer) (int, error) {
	db, err := c.getDB()
	if err != nil {
		return 0, err
	}
	sql, args, err := sqrl.Select("COUNT(*)").
		PlaceholderFormat(sqrl.Dollar).
		From(TImage).
		Where(query).
		ToSql()
	if err != nil {
		return 0, err
	}
	var cnt int
	err = db.GetContext(ctx, &cnt, sql, args...)
	return cnt, err
}

// DeleteImage 逻辑删除镜像
func (c *Client) DeleteImage(ctx context.Context, id int64, deletedBy string) error {
	db, err := c.getDB()
	if err != nil {
		return err
	}
	cmd := fmt.Sprintf(`UPDATE %s SET deleted_at = NOW(), deleted_by = $2 WHERE id = $1`, TImage)
	_, err = db.ExecContext(ctx, cmd, id, deletedBy)
	if err != nil {
		klog.ErrorS(err, "failed to delete image", "id", id)
	}
	return err
}
