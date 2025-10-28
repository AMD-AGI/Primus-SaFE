package client

import (
	"context"

	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/dal"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/model"
)

func (c *Client) SubmitNotification(ctx context.Context, data *model.Notification) error {
	q := dal.Use(c.gorm).Notification
	return q.WithContext(ctx).Create(data)
}

func (c *Client) ListUnprocessedNotifications(ctx context.Context) ([]*model.Notification, error) {
	q := dal.Use(c.gorm).Notification
	return q.WithContext(ctx).Where(q.SentAt.IsNull()).Find()
}
