/*
 * Copyright (C) 2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package workspaceaccess

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	dbmodel "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/model"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/notification/model"
)

// recordingSubmitter keeps the notifications it is given.
type recordingSubmitter struct {
	got []*dbmodel.Notification
}

// SubmitNotification records the notification.
func (r *recordingSubmitter) SubmitNotification(_ context.Context, data *dbmodel.Notification) error {
	r.got = append(r.got, data)
	return nil
}

// The created e-mail goes to the recipients and carries the requester, workspace, reason and link.
func TestBuildMessageCreated(t *testing.T) {
	sub := &recordingSubmitter{}
	data := &TopicData{Event: EventCreated, RequestId: 12, Workspace: "ws", WorkspaceName: "Team WS", UserId: "u1",
		UserName: "Alice", UserEmail: "alice@example.com", Reason: "need gpus",
		URL: "https://safe.example.com/workspace-access/requests/12", To: []string{"mgr@example.com"}}
	require.NoError(t, Submit(context.Background(), sub, data))
	require.Len(t, sub.got, 1)
	require.Equal(t, model.TopicWorkspaceAccess, sub.got[0].Topic)
	require.Equal(t, "workspace-access-12-created", sub.got[0].UID)

	topic := &Topic{}
	require.True(t, topic.Filter(sub.got[0].Data))
	msgs, err := topic.BuildMessage(context.Background(), sub.got[0].Data)
	require.NoError(t, err)
	require.Len(t, msgs, 1)
	require.Equal(t, []string{"mgr@example.com"}, msgs[0].Email.To)
	require.Contains(t, msgs[0].Email.Title, "Alice")
	require.Contains(t, msgs[0].Email.Title, "Team WS")
	for _, want := range []string{"alice@example.com", "need gpus", "https://safe.example.com/workspace-access/requests/12"} {
		require.Contains(t, msgs[0].Email.Content, want)
	}
}

// Data without recipients or with an unknown event is neither stored nor sent.
func TestNoRecipientsNoMessage(t *testing.T) {
	sub := &recordingSubmitter{}
	require.NoError(t, Submit(context.Background(), sub, &TopicData{Event: EventApproved, RequestId: 1}))
	require.Empty(t, sub.got)

	topic := &Topic{}
	require.False(t, topic.Filter(map[string]interface{}{"event": EventApproved}))
	require.False(t, topic.Filter(map[string]interface{}{"event": "other", "to": []interface{}{"a@example.com"}}))
	msgs, err := topic.BuildMessage(context.Background(), map[string]interface{}{"event": EventRejected})
	require.NoError(t, err)
	require.Empty(t, msgs)
}

// The decision and auto close e-mails tell the requester the outcome.
func TestBuildMessageOutcome(t *testing.T) {
	topic := &Topic{}
	for event, word := range map[string]string{EventApproved: "approved", EventRejected: "rejected", EventAutoClosed: "closed"} {
		msgs, err := topic.BuildMessage(context.Background(), map[string]interface{}{
			"event": event, "workspace": "ws", "comment": "see you", "to": []interface{}{"alice@example.com"}})
		require.NoError(t, err)
		require.Len(t, msgs, 1)
		require.Contains(t, msgs[0].Email.Title, word, event)
	}
}

// The e-mail carries the request link only when it is an absolute http or https URL.
func TestBuildMessageOmitsARelativeURL(t *testing.T) {
	topic := &Topic{}
	for url, want := range map[string]bool{
		"https://safe.example.com/workspace-access/requests/3": true,
		"http://safe.example.com/workspace-access/requests/3":  true,
		"/workspace-access/requests/3":                         false,
		"javascript://safe.example.com/alert":                  false,
		"https:///workspace-access/requests/3":                 false,
	} {
		msgs, err := topic.BuildMessage(context.Background(), map[string]interface{}{
			"event": EventCreated, "workspace": "ws", "url": url, "to": []interface{}{"mgr@example.com"}})
		require.NoError(t, err)
		require.Len(t, msgs, 1)
		require.Equal(t, want, strings.Contains(msgs[0].Email.Content, "workspace-access/requests/3") ||
			strings.Contains(msgs[0].Email.Content, "alert"), url)
	}
}
