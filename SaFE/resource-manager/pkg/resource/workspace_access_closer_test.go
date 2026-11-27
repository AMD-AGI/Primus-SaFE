/*
 * Copyright (C) 2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resource

import (
	"context"
	"testing"
	"time"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	dbmodel "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/model"
)

// fakeAccessStore closes the pending requests it holds once, as the conditional update does.
type fakeAccessStore struct {
	pending       []*dbclient.WorkspaceAccessRequest
	cutoffs       []time.Time
	notifications []*dbmodel.Notification
}

// AutoCloseWorkspaceAccessRequests returns the pending requests created before the cutoff and drops them.
func (s *fakeAccessStore) AutoCloseWorkspaceAccessRequests(_ context.Context,
	before time.Time) ([]*dbclient.WorkspaceAccessRequest, error) {
	s.cutoffs = append(s.cutoffs, before)
	var closed, kept []*dbclient.WorkspaceAccessRequest
	for _, req := range s.pending {
		if req.CreatedAt.Before(before) {
			req.Status = dbclient.WorkspaceAccessAutoClosed
			closed = append(closed, req)
		} else {
			kept = append(kept, req)
		}
	}
	s.pending = kept
	return closed, nil
}

// SubmitNotification records the notification.
func (s *fakeAccessStore) SubmitNotification(_ context.Context, data *dbmodel.Notification) error {
	s.notifications = append(s.notifications, data)
	return nil
}

// Expired requests are closed once with the configured TTL and their requesters are notified.
func TestWorkspaceAccessCloserClosesExpiredRequests(t *testing.T) {
	viper.Set("workspace_access.pending_ttl", "48h")
	viper.Set("notification.enable", true)
	t.Cleanup(viper.Reset)

	now := time.Date(2026, 11, 27, 0, 0, 0, 0, time.UTC)
	store := &fakeAccessStore{pending: []*dbclient.WorkspaceAccessRequest{
		{ID: 1, Workspace: "ws", UserID: "alice", Status: dbclient.WorkspaceAccessPending, CreatedAt: now.Add(-72 * time.Hour)},
		{ID: 2, Workspace: "ws", UserID: "bob", Status: dbclient.WorkspaceAccessPending, CreatedAt: now.Add(-time.Hour)},
	}}
	scheme := runtime.NewScheme()
	require.NoError(t, v1.AddToScheme(scheme))
	alice := &v1.User{ObjectMeta: metav1.ObjectMeta{Name: "alice",
		Annotations: map[string]string{v1.UserEmailAnnotation: "alice@example.com"}}}
	workspace := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws",
		Labels: map[string]string{v1.DisplayNameLabel: "Team WS"}}}
	c := &WorkspaceAccessCloser{
		Client:   ctrlfake.NewClientBuilder().WithScheme(scheme).WithObjects(alice, workspace).Build(),
		connect:  func() workspaceAccessStore { return store },
		interval: time.Hour,
		now:      func() time.Time { return now },
	}

	require.True(t, c.closeExpired(context.Background()))
	require.Equal(t, []time.Time{now.Add(-48 * time.Hour)}, store.cutoffs)
	require.Len(t, store.pending, 1)
	require.Len(t, store.notifications, 1)
	n := store.notifications[0]
	require.Equal(t, "workspace-access-1-auto_closed", n.UID)
	require.Equal(t, "auto_closed", n.Data["event"])
	require.Equal(t, "Team WS", n.Data["workspaceName"])
	require.Equal(t, []interface{}{"alice@example.com"}, n.Data["to"])

	require.True(t, c.closeExpired(context.Background()))
	require.Len(t, store.notifications, 1, "a closed request is not notified again")
}

// Without the database a run reports that it must be tried again.
func TestWorkspaceAccessCloserWithoutDatabase(t *testing.T) {
	c := &WorkspaceAccessCloser{
		connect: func() workspaceAccessStore { return nil },
		now:     time.Now,
	}
	require.False(t, c.closeExpired(context.Background()))
	require.True(t, c.NeedLeaderElection())
}
