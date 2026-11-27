/*
 * Copyright (C) 2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package workspaceaccess

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"net/url"
	"strconv"
	"time"

	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	dbmodel "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/model"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/notification/model"
	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
)

// The events of a workspace access request that are notified.
const (
	EventCreated    = "created"
	EventApproved   = "approved"
	EventRejected   = "rejected"
	EventAutoClosed = "auto_closed"
)

// TopicData is the notification data of one event of a workspace access request. To holds the
// e-mail addresses of the recipients, resolved when the event is submitted.
type TopicData struct {
	Event         string   `json:"event"`
	RequestId     int64    `json:"requestId"`
	Workspace     string   `json:"workspace"`
	WorkspaceName string   `json:"workspaceName,omitempty"`
	UserId        string   `json:"userId"`
	UserName      string   `json:"userName,omitempty"`
	UserEmail     string   `json:"userEmail,omitempty"`
	Reason        string   `json:"reason,omitempty"`
	DecidedByName string   `json:"decidedByName,omitempty"`
	Comment       string   `json:"comment,omitempty"`
	URL           string   `json:"url,omitempty"`
	To            []string `json:"to"`
}

// notificationSubmitter stores a notification for the notification manager to send.
type notificationSubmitter interface {
	SubmitNotification(ctx context.Context, data *dbmodel.Notification) error
}

// Topic builds the e-mails of the workspace access request events.
type Topic struct{}

// Name returns the topic name.
func (t *Topic) Name() string {
	return model.TopicWorkspaceAccess
}

// Filter accepts data that names a known event and at least one recipient.
func (t *Topic) Filter(data map[string]interface{}) bool {
	topicData := &TopicData{}
	if err := commonutils.TransMapToStruct(data, topicData); err != nil {
		return false
	}
	return isKnownEvent(topicData.Event) && len(topicData.To) > 0
}

// BuildMessage renders the e-mail of the event to its recipients.
func (t *Topic) BuildMessage(_ context.Context, data map[string]interface{}) ([]*model.Message, error) {
	topicData := &TopicData{}
	if err := commonutils.TransMapToStruct(data, topicData); err != nil {
		return nil, fmt.Errorf("failed to convert data to TopicData: %w", err)
	}
	if !isKnownEvent(topicData.Event) || len(topicData.To) == 0 {
		return nil, nil
	}
	content, err := renderEmail(topicData)
	if err != nil {
		return nil, fmt.Errorf("failed to render email template: %w", err)
	}
	return []*model.Message{{
		Email: &model.EmailMessage{
			To:      topicData.To,
			Title:   emailTitle(topicData),
			Content: content,
		},
	}}, nil
}

// requestPagePath is the console page of a request, followed by its id.
const requestPagePath = "/workspace-access/requests/"

// RequestURL returns the console page of the request: absolute when the console address is configured,
// the path otherwise.
func RequestURL(requestId int64) string {
	return commonconfig.GetConsoleOrigin() + requestPagePath + strconv.FormatInt(requestId, 10)
}

// UID returns the notification uid of an event, unique per request and event.
func UID(requestId int64, event string) string {
	return "workspace-access-" + strconv.FormatInt(requestId, 10) + "-" + event
}

// Submit stores the notification of the event; data without recipients is not stored.
func Submit(ctx context.Context, db notificationSubmitter, data *TopicData) error {
	if db == nil || data == nil || len(data.To) == 0 {
		return nil
	}
	raw, err := json.Marshal(data)
	if err != nil {
		return err
	}
	ext := dbmodel.ExtType{}
	if err = json.Unmarshal(raw, &ext); err != nil {
		return err
	}
	return db.SubmitNotification(ctx, &dbmodel.Notification{
		Topic:     model.TopicWorkspaceAccess,
		UID:       UID(data.RequestId, data.Event),
		Data:      ext,
		CreatedAt: time.Now(),
	})
}

// isKnownEvent reports whether the event is one of the notified events.
func isKnownEvent(event string) bool {
	switch event {
	case EventCreated, EventApproved, EventRejected, EventAutoClosed:
		return true
	}
	return false
}

// workspaceLabel returns the display name of the workspace, or its id.
func workspaceLabel(data *TopicData) string {
	if data.WorkspaceName != "" {
		return data.WorkspaceName
	}
	return data.Workspace
}

// requesterLabel returns the name of the requester, or the id.
func requesterLabel(data *TopicData) string {
	if data.UserName != "" {
		return data.UserName
	}
	return data.UserId
}

// emailTitle returns the subject of the e-mail of the event.
func emailTitle(data *TopicData) string {
	switch data.Event {
	case EventCreated:
		return fmt.Sprintf("[Primus SaFE] %s requests access to workspace %s", requesterLabel(data), workspaceLabel(data))
	case EventApproved:
		return fmt.Sprintf("[Primus SaFE] Your access request to workspace %s is approved", workspaceLabel(data))
	case EventRejected:
		return fmt.Sprintf("[Primus SaFE] Your access request to workspace %s is rejected", workspaceLabel(data))
	default:
		return fmt.Sprintf("[Primus SaFE] Your access request to workspace %s is closed", workspaceLabel(data))
	}
}

// emailData is the data of the e-mail template.
type emailData struct {
	Summary       string
	Workspace     string
	Requester     string
	RequesterMail string
	Reason        string
	DecidedBy     string
	Comment       string
	URL           string
}

// absoluteURL returns raw when it is an http or https URL with a host, and "" otherwise.
func absoluteURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return ""
	}
	return raw
}

// renderEmail renders the HTML body of the e-mail of the event.
func renderEmail(data *TopicData) (string, error) {
	view := emailData{
		Workspace:     workspaceLabel(data),
		Requester:     requesterLabel(data),
		RequesterMail: data.UserEmail,
		Reason:        data.Reason,
		DecidedBy:     data.DecidedByName,
		Comment:       data.Comment,
		URL:           absoluteURL(data.URL),
	}
	switch data.Event {
	case EventCreated:
		view.Summary = "A user requests member access to a workspace you manage. Open the link to approve or reject it."
	case EventApproved:
		view.Summary = "Your request for member access to the workspace is approved. The workspace is available now."
	case EventRejected:
		view.Summary = "Your request for member access to the workspace is rejected."
	default:
		view.Summary = "Your request for member access to the workspace was not decided in time and is closed. " +
			"You may submit a new request."
	}
	tmpl, err := template.New("workspace_access").Parse(emailTemplate)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err = tmpl.Execute(&buf, view); err != nil {
		return "", err
	}
	return buf.String(), nil
}
