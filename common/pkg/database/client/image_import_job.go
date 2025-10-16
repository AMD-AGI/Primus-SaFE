package client

import (
	"context"
	"errors"
	"fmt"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/dal"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/model"
	"gorm.io/gorm"
)

func (c *Client) GetImageImportJobByJobName(ctx context.Context, jobName string) (*model.ImageImportJob, error) {
	q := dal.Use(c.gorm).ImageImportJob
	item, err := q.WithContext(ctx).Where(q.JobName.Eq(jobName)).First()
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get image import job by job name %s: %w", jobName, err)
	}
	return item, nil
}

func (c *Client) GetImageImportJobByTag(ctx context.Context, tag string) (*model.ImageImportJob, error) {
	q := dal.Use(c.gorm).ImageImportJob
	item, err := q.WithContext(ctx).Where(q.DstName.Eq(tag)).First()
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get image import job by tag %s: %w", tag, err)
	}
	return item, nil
}

func (c *Client) GetImageImportJobByID(ctx context.Context, id int32) (*model.ImageImportJob, error) {
	q := dal.Use(c.gorm).ImageImportJob
	item, err := q.WithContext(ctx).Where(q.ID.Eq(id)).First()
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get image import job by id %d: %w", id, err)
	}
	return item, nil
}

func (c *Client) UpsertImageImportJob(ctx context.Context, job *model.ImageImportJob) error {
	exist, err := c.GetImageImportJobByID(ctx, job.ID)
	if err != nil {
		return err
	}
	if exist == nil {
		// insert
		if err := dal.Use(c.gorm).ImageImportJob.WithContext(ctx).Create(job); err != nil {
			return fmt.Errorf("failed to insert image import job %v: %w", job, err)
		}
	} else {
		// update
		job.ID = exist.ID
		if err := dal.Use(c.gorm).ImageImportJob.WithContext(ctx).Save(job); err != nil {
			return fmt.Errorf("failed to update image import job %v: %w", job, err)
		}
	}
	return nil
}

func (c *Client) GetImportImageByImageID(ctx context.Context, imageID int32) (*model.ImageImportJob, error) {
	q := dal.Use(c.gorm).ImageImportJob
	item, err := q.WithContext(ctx).Where(q.ImageID.Eq(imageID)).First()
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get import image by image id %d: %w", imageID, err)
	}
	return item, nil
}

func (c *Client) UpdateImageImportJob(ctx context.Context, job *model.ImageImportJob) error {
	err := dal.Use(c.gorm).ImageImportJob.WithContext(ctx).Save(job)
	if err != nil {
		return fmt.Errorf("failed to update image import job %v: %w", job, err)
	}
	return nil
}
