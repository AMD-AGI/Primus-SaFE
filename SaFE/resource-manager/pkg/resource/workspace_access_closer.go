/*
 * Copyright (C) 2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resource

import (
	"context"
	"time"

	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	dbmodel "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/model"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/notification/topic/workspaceaccess"
)

const (
	// workspaceAccessCloseInterval is how often the pending workspace access requests are checked.
	workspaceAccessCloseInterval = 24 * time.Hour
	// workspaceAccessRetryInterval is how soon a run is tried again when the database was not reachable
	// or the query failed.
	workspaceAccessRetryInterval = time.Minute
)

// workspaceAccessStore is the part of the database the closer uses.
type workspaceAccessStore interface {
	AutoCloseWorkspaceAccessRequests(ctx context.Context, before time.Time) ([]*dbclient.WorkspaceAccessRequest, error)
	SubmitNotification(ctx context.Context, data *dbmodel.Notification) error
}

// WorkspaceAccessCloser closes the workspace access requests that stayed pending longer than
// workspace_access.pending_ttl and notifies their requesters.
type WorkspaceAccessCloser struct {
	client.Client
	connect  func() workspaceAccessStore
	store    workspaceAccessStore
	interval time.Duration
	now      func() time.Time
}

// SetupWorkspaceAccessCloser registers the closer. It is skipped when the database is not enabled,
// that is when connect is nil. connect returns the store, or nil while the database cannot be reached.
func SetupWorkspaceAccessCloser(mgr manager.Manager, connect func() workspaceAccessStore) error {
	if connect == nil {
		klog.Info("[workspace-access-closer] database not enabled, skipping")
		return nil
	}
	c := &WorkspaceAccessCloser{
		Client:   mgr.GetClient(),
		connect:  connect,
		interval: workspaceAccessCloseInterval,
		now:      time.Now,
	}
	if err := mgr.Add(c); err != nil {
		return err
	}
	klog.Info("[workspace-access-closer] registered")
	return nil
}

// NeedLeaderElection makes the closer run only on the elected leader.
func (c *WorkspaceAccessCloser) NeedLeaderElection() bool { return true }

// Start closes the expired requests at once and then at every interval until ctx ends. A run without
// the database, or whose query failed, is tried again after workspaceAccessRetryInterval.
func (c *WorkspaceAccessCloser) Start(ctx context.Context) error {
	for {
		wait := c.interval
		if !c.closeExpired(ctx) {
			wait = workspaceAccessRetryInterval
		}
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil
		case <-timer.C:
		}
	}
}

// closeExpired closes the requests created more than the pending TTL ago and notifies each requester.
// The close is one conditional update, so a request is closed and notified once even when two runs overlap.
// An invalid pending TTL is logged and the default is used. It returns false when the database is not
// reachable or the query failed.
func (c *WorkspaceAccessCloser) closeExpired(ctx context.Context) bool {
	if c.store == nil {
		if c.store = c.connect(); c.store == nil {
			klog.Warningf("[workspace-access-closer] database not reachable, retrying in %s", workspaceAccessRetryInterval)
			return false
		}
	}
	ttl, err := commonconfig.GetWorkspaceAccessPendingTTL()
	if err != nil {
		klog.ErrorS(err, "[workspace-access-closer] invalid pending TTL")
	}
	cutoff := c.now().UTC().Add(-ttl)
	closed, err := c.store.AutoCloseWorkspaceAccessRequests(ctx, cutoff)
	if err != nil {
		klog.ErrorS(err, "[workspace-access-closer] failed to close the expired workspace access requests, retrying in "+
			workspaceAccessRetryInterval.String())
		return false
	}
	if len(closed) > 0 {
		klog.Infof("[workspace-access-closer] closed %d workspace access requests created before %s",
			len(closed), cutoff.Format(time.RFC3339))
	}
	if commonconfig.IsNotificationEnable() {
		for _, req := range closed {
			c.notifyRequester(ctx, req)
		}
	}
	return true
}

// notifyRequester tells the requester that the request is closed and may be submitted again. Failures are
// only logged.
func (c *WorkspaceAccessCloser) notifyRequester(ctx context.Context, req *dbclient.WorkspaceAccessRequest) {
	user := &v1.User{}
	if err := c.Get(ctx, client.ObjectKey{Name: req.UserID}, user); err != nil {
		klog.V(4).InfoS("[workspace-access-closer] requester not found", "id", req.ID, "user", req.UserID, "err", err)
		return
	}
	email := v1.GetUserEmail(user)
	if email == "" {
		return
	}
	data := &workspaceaccess.TopicData{
		Event:     workspaceaccess.EventAutoClosed,
		RequestId: req.ID,
		Workspace: req.Workspace,
		UserId:    req.UserID,
		UserName:  req.UserName,
		UserEmail: email,
		Reason:    req.Reason,
		URL:       workspaceaccess.RequestURL(req.ID),
		To:        []string{email},
	}
	workspace := &v1.Workspace{}
	if err := c.Get(ctx, client.ObjectKey{Name: req.Workspace}, workspace); err == nil {
		data.WorkspaceName = v1.GetDisplayName(workspace)
	}
	if err := workspaceaccess.Submit(ctx, c.store, data); err != nil {
		klog.ErrorS(err, "[workspace-access-closer] failed to submit the notification", "id", req.ID)
	}
}
